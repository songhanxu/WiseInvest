import { useEffect, useId, useRef, useState } from "react";
import { Check, Copy, Loader2, Send, Square, Users } from "lucide-react";
import { ChatMarkdown } from "./ChatMarkdown";
import { requestJson } from "./App";
import type { Message, SavedConversation } from "./conversation-history";
import { insertRoundtableMention, readResearchStream, roundtableAgents, roundtableMentionAt, roundtableParticipants, roundtableWelcome } from "./research";

const messageId = () => globalThis.crypto?.randomUUID?.() ?? `round-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;

export function RoundtableIcon({ mini = false }: { mini?: boolean }) {
  return <div className={`market-icon roundtable-icon ${mini ? "mini" : "small"}`}><Users size={mini ? 14 : 19} /></div>;
}

export function RoundtableView({ apiBase, token, conversation, draftPrompt, onDraftPromptChange, onUpdate, onNeedToken }: {
  apiBase: string; token: string; conversation: SavedConversation; draftPrompt: string;
  onDraftPromptChange: (value: string) => void; onUpdate: (next: SavedConversation) => void; onNeedToken: () => void;
}) {
  const [input, setInput] = useState(draftPrompt);
  const [participants, setParticipants] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [activeAgent, setActiveAgent] = useState("");
  const [activeMessage, setActiveMessage] = useState("");
  const [progress, setProgress] = useState("");
  const [mention, setMention] = useState<ReturnType<typeof roundtableMentionAt>>(null);
  const [mentionIndex, setMentionIndex] = useState(0);
  const [copied, setCopied] = useState("");
  const requestRef = useRef<AbortController | null>(null);
  const pendingReply = useRef<{ conversation: SavedConversation; messageId: string } | null>(null);
  const updateRef = useRef(onUpdate); updateRef.current = onUpdate;
  const textarea = useRef<HTMLTextAreaElement>(null);
  const mentionListId = useId();
  const bottom = useRef<HTMLDivElement>(null);
  useEffect(() => { setInput(draftPrompt); }, [draftPrompt]);
  useEffect(() => {
    setBusy(false); setActiveAgent("");
    return () => {
      requestRef.current?.abort(); requestRef.current = null;
      const pending = pendingReply.current;
      if (pending?.messageId) updateRef.current({ ...pending.conversation, messages: pending.conversation.messages.map(message => message.id === pending.messageId ? { ...message, interrupted: true } : message) });
      pendingReply.current = null;
    };
  }, [apiBase, token]);
  useEffect(() => { bottom.current?.scrollIntoView({ behavior: "smooth", block: "nearest" }); }, [conversation.messages]);

  function updateInput(value: string, caret: number) {
    setInput(value); onDraftPromptChange(value);
    setMention(roundtableMentionAt(value, caret)); setMentionIndex(0);
  }
  function chooseMention(name: string) {
    if (!mention) return;
    const result = insertRoundtableMention(input, mention, name);
    setInput(result.text); onDraftPromptChange(result.text); setMention(null);
    requestAnimationFrame(() => { textarea.current?.focus(); textarea.current?.setSelectionRange(result.caret, result.caret); });
  }

  async function send() {
    const text = input.trim();
    if (!text || busy || requestRef.current) return;
    if (!token) { onNeedToken(); return; }
    const requestedParticipants = roundtableParticipants(text, participants);
    setParticipants(requestedParticipants);
    const controller = new AbortController(); requestRef.current = controller;
    setInput(""); onDraftPromptChange(""); setMention(null); setBusy(true); setError(""); setProgress("正在连接圆桌…");
    const resetTimeout = () => window.setTimeout(() => controller.abort(new Error("圆桌长时间未返回内容，请重试或检查后端 AI 服务。")), 140000);
    let idleTimer = resetTimeout();
    let next = { ...conversation, title: conversation.title || text.slice(0, 24), updatedAt: new Date().toISOString(), messages: [...conversation.messages, { id: messageId(), role: "user", content: text } as Message] };
    const publish = () => { if (requestRef.current !== controller) return; next = { ...next, updatedAt: new Date().toISOString() }; pendingReply.current = { conversation: next, messageId: currentId }; onUpdate(next); };
    let currentId = "";
    publish();
    try {
      const backendId = next.backendId ?? (await requestJson<{ id: number }>(apiBase, "/api/v1/conversations", token, { method: "POST", signal: controller.signal, body: JSON.stringify({ agent_type: "group_chat", title: next.title }) })).id;
      if (controller.signal.aborted) return;
      next = { ...next, backendId }; publish();
      const response = await fetch(`${apiBase}/api/v1/messages/group-chat/stream`, { method: "POST", signal: controller.signal,
        headers: { "Content-Type": "application/json", Accept: "text/event-stream", Authorization: `Bearer ${token}` },
        body: JSON.stringify({ conversation_id: backendId, content: text, participants: requestedParticipants }) });
      await readResearchStream(response, chunk => {
        if (controller.signal.aborted) return;
        window.clearTimeout(idleTimer); idleTimer = resetTimeout();
        if (chunk.type === "agent_start") {
          currentId = messageId(); setActiveMessage(currentId); setActiveAgent(chunk.agent_id || ""); setProgress("正在分析…");
          next = { ...next, messages: [...next.messages, { id: currentId, role: "assistant", agentId: chunk.agent_id, content: "" }] };
        } else if (chunk.type === "agent_end") { currentId = ""; setActiveMessage(""); setActiveAgent(""); }
        else if (chunk.type === "thought") { setProgress(chunk.content || "正在分析…"); }
        else if (chunk.type === "agent_error" && currentId) {
          next = { ...next, messages: next.messages.map(message => message.id === currentId ? { ...message, responseError: chunk.content || "本角色分析失败，请重试。" } : message) };
        }
        else if (chunk.type === "content" && currentId) {
          next = { ...next, messages: next.messages.map(message => message.id === currentId ? { ...message, content: message.content + (chunk.content || "") } : message) };
        }
        publish();
      });
      if (!controller.signal.aborted && !next.messages.some(message => message.role === "assistant" && message.content && !conversation.messages.some(old => old.id === message.id))) setError("本轮暂未返回分析内容，可以稍后继续提问。");
    } catch (err) {
      const stopped = controller.signal.aborted;
      next = { ...next, messages: next.messages.map(message => message.id === currentId ? { ...message, interrupted: true } : message) };
      publish();
      if (stopped && controller.signal.reason instanceof Error && controller.signal.reason.name !== "AbortError") setError(controller.signal.reason.message);
      else if (!stopped) setError(err instanceof Error ? err.message : "圆桌讨论暂不可用，请稍后重试。");
    } finally {
      window.clearTimeout(idleTimer);
      if (requestRef.current === controller) { requestRef.current = null; pendingReply.current = null; setBusy(false); setActiveAgent(""); setActiveMessage(""); setProgress(""); }
    }
  }

  return <div className="conversation roundtable-conversation">
    <header className="conversation-header"><RoundtableIcon /><div><h2>慧投圆桌</h2><p>价值 · 趋势 · 量化，多角度交叉讨论</p></div></header>
    <div className="roundtable-participants" role="group" aria-label="参与角色"><span>参与角色</span><button type="button" aria-pressed={!participants.length} disabled={busy} onClick={() => setParticipants([])}>全部</button>{roundtableAgents.map(agent => <button key={agent.id} type="button" aria-pressed={participants.includes(agent.id)} disabled={busy} onClick={() => setParticipants(previous => previous.includes(agent.id) ? previous.filter(id => id !== agent.id) : [...previous, agent.id])}>{agent.name}</button>)}</div>
    <div className="messages" aria-live="polite" aria-relevant="additions text">{conversation.messages.map(message => {
      const agent = roundtableAgents.find(item => item.id === message.agentId);
      return <div className={`message ${message.role} roundtable-message`} key={message.id}>
        {message.role === "assistant" && <div className="roundtable-speaker"><span>{agent?.mark || "圆"}</span><strong>{agent?.name || "慧投圆桌"}</strong>{message.content && <button type="button" aria-label={`复制${agent?.name || "圆桌"}回复`} onClick={() => { if (!navigator.clipboard) { setError("当前浏览器不支持直接复制，请选择文本复制。"); return; } navigator.clipboard.writeText(message.content).then(() => setCopied(message.id)).catch(() => setError("复制失败，请手动选择文本复制。")); }}>{copied === message.id ? <Check size={13} /> : <Copy size={13} />}</button>}</div>}
        {message.role === "assistant" ? <ChatMarkdown content={message.content.startsWith("欢迎来到慧投圆桌。") && !message.agentId ? roundtableWelcome : message.content || (busy && message.id === activeMessage ? progress : message.responseError || (message.interrupted ? "已停止，未收到正文。" : "本轮未返回内容，可重新提问。"))} /> : <p>{message.content}</p>}
        {message.content && message.responseError && <small className="roundtable-interrupted" role="alert">{message.responseError}</small>}
        {message.interrupted && <small className="roundtable-interrupted">本条回复已中断</small>}
      </div>;
    })}{busy && <div className="roundtable-progress" role="status"><Loader2 size={13} className="spin" />{roundtableAgents.find(agent => agent.id === activeAgent)?.name || "圆桌"} · {progress}</div>}<div ref={bottom} /></div>
    {error && <div className="error-banner" role="alert">{error}</div>}
    <form className="composer roundtable-composer" onSubmit={event => { event.preventDefault(); void send(); }}>
      {mention && <div className="roundtable-mentions" id={mentionListId} role="listbox" aria-label="选择圆桌角色">{mention.options.map((agent, index) => <button key={agent.id} id={`${mentionListId}-${index}`} type="button" role="option" aria-selected={index === mentionIndex} onMouseDown={event => event.preventDefault()} onClick={() => chooseMention(agent.name)}><span>{agent.mark}</span><strong>{agent.name}</strong><small>{agent.role}</small></button>)}</div>}
      <textarea ref={textarea} aria-label="圆桌问题" role="combobox" aria-autocomplete="list" aria-expanded={Boolean(mention)} aria-controls={mention ? mentionListId : undefined} aria-activedescendant={mention ? `${mentionListId}-${mentionIndex}` : undefined} value={input}
        onChange={event => updateInput(event.target.value, event.target.selectionStart)}
        onClick={event => { setMention(roundtableMentionAt(input, event.currentTarget.selectionStart)); setMentionIndex(0); }}
        onBlur={() => setMention(null)}
        onKeyDown={event => {
          if (event.nativeEvent.isComposing) return;
          if (mention && ["ArrowDown", "ArrowUp", "Enter", "Tab", "Escape"].includes(event.key)) {
            event.preventDefault();
            if (event.key === "Escape") setMention(null);
            else if (event.key === "Enter" || event.key === "Tab") chooseMention(mention.options[mentionIndex].name);
            else setMentionIndex(index => (index + (event.key === "ArrowDown" ? 1 : -1) + mention.options.length) % mention.options.length);
          } else if ((event.ctrlKey || event.metaKey) && event.key === "Enter") { event.preventDefault(); void send(); }
        }} placeholder="提出你的问题，输入 @ 选择参与角色" rows={2} />{busy ? <button type="button" aria-label="停止圆桌讨论" title="停止讨论" onClick={() => requestRef.current?.abort()}><Square size={17} /></button> : <button type="submit" aria-label="发送圆桌问题" disabled={!input.trim()}><Send size={19} /></button>}
    </form>
  </div>;
}
