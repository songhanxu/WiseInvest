const { request, streamRequest } = require('../../utils/api');
const { loadConversations, upsertConversation } = require('../../utils/storage');
const { renderMarkdown } = require('../../utils/markdown');

const agents = [
  { id: 'orchestrator', name: '主持人' }, { id: 'value', name: '价值派' },
  { id: 'trend', name: '趋势派' }, { id: 'quant', name: '量化派' },
];
const createID = () => `${Date.now()}_${Math.random().toString(16).slice(2)}`;
const welcome = '欢迎来到慧投圆桌。主持人、价值派、趋势派和量化派会从不同角度讨论你的问题。选择参与角色，或输入 @ 指定发言人；@所有人 邀请全部角色。';

Page({
  data: { agents, participants: [], messages: [{ id: 'welcome', role: 'assistant', name: '慧投圆桌', content: welcome, html: renderMarkdown(welcome) }],
    input: '', busy: false, error: '', progress: '', conversationId: '', backendId: 0, scrollIntoView: 'bottom' },
  streamTask: null,
  onLoad(options) {
    const saved = loadConversations().find((row) => row.id === options.conversationId && row.agentType === 'group_chat');
    if (saved) this.setData({ conversationId: saved.id, backendId: saved.backendConversationId || 0,
      messages: (saved.messages || []).map((message) => ({ ...message, html: renderMarkdown(message.content) })) });
  },
  onUnload() { if (this.streamTask && this.streamTask.abort) this.streamTask.abort(); },
  onInput(e) { this.setData({ input: e.detail.value || '' }); },
  toggleAgent(e) {
    if (this.data.busy) return;
    const id = e.currentTarget.dataset.id;
    const old = this.data.participants;
    const participants = old.includes(id) ? old.filter((item) => item !== id) : [...old, id];
    this.setData({ participants, agents: agents.map((item) => ({ ...item, selected: participants.includes(item.id) })) });
  },
  selectAll() { if (!this.data.busy) this.setData({ participants: [], agents }); },
  async send() {
    const content = this.data.input.trim();
    if (!content || this.data.busy) return;
    const mentioned = agents.filter((agent) => content.includes(`@${agent.name}`)).map((agent) => agent.id);
    const participants = /@(?:所有人|all\b)/i.test(content) ? [] : mentioned.length ? mentioned : this.data.participants;
    this.setData({ busy: true, error: '', progress: '正在连接圆桌…', participants,
      agents: agents.map((item) => ({ ...item, selected: participants.includes(item.id) })) });
    try {
      let backendId = this.data.backendId;
      if (!backendId) {
        const response = await request({ url: '/api/v1/conversations', method: 'POST', needAuth: true,
          data: { agent_type: 'group_chat', title: content.slice(0, 24) } });
        backendId = Number(response.id);
        this.setData({ backendId });
      }
      const user = { id: createID(), role: 'user', content, html: renderMarkdown(content) };
      this.setData({ input: '', messages: [...this.data.messages, user] }, () => this.persist());
      this.streamTask = streamRequest({ url: '/api/v1/messages/group-chat/stream', data: { conversation_id: backendId, content, participants },
        onChunk: (chunk) => this.onChunk(chunk),
        onDone: () => { this.setData({ busy: false, progress: '' }); this.persist(); },
        onError: (error) => { this.setData({ busy: false, progress: '', error: error.message || '圆桌请求失败' }); this.persist(); },
      });
    } catch (error) { this.setData({ busy: false, progress: '', error: error.message || '圆桌请求失败' }); }
  },
  onChunk(chunk) {
    const messages = [...this.data.messages];
    const agent = agents.find((item) => item.id === chunk.agent_id);
    if (chunk.type === 'agent_start') {
      messages.push({ id: createID(), role: 'assistant', agentId: chunk.agent_id, name: agent ? agent.name : '慧投圆桌', content: '', html: '' });
      this.setData({ messages, progress: `${agent ? agent.name : '圆桌'}正在分析…` });
    } else if (chunk.type === 'thought') {
      this.setData({ progress: chunk.content || '正在分析…' });
    } else if (chunk.type === 'content' || chunk.type === 'agent_error') {
      const index = messages.map((item) => item.agentId).lastIndexOf(chunk.agent_id);
      if (index >= 0) {
        const text = chunk.type === 'agent_error' ? `\n\n${chunk.content || '本角色分析失败'}` : chunk.content || '';
        messages[index] = { ...messages[index], content: messages[index].content + text };
        messages[index].html = renderMarkdown(messages[index].content);
        this.setData({ messages, scrollIntoView: 'bottom' });
      }
    } else if (chunk.type === 'agent_end') {
      this.persist();
    }
  },
  persist() {
    const user = this.data.messages.find((item) => item.role === 'user');
    if (!user) return;
    const id = this.data.conversationId || createID();
    upsertConversation({ id, agentType: 'group_chat', backendConversationId: this.data.backendId,
      title: user.content.slice(0, 24), messages: this.data.messages.map(({ html, ...rest }) => rest), updatedAt: new Date().toISOString() });
    if (!this.data.conversationId) this.setData({ conversationId: id });
  },
});
