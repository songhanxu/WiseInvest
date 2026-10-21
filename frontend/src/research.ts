export const roundtableAgents = [
  { id: "orchestrator", name: "主持人", role: "综合观点，梳理分歧", mark: "主" },
  { id: "value", name: "价值派", role: "公司基本面与估值", mark: "值" },
  { id: "trend", name: "趋势派", role: "价格趋势与技术结构", mark: "势" },
  { id: "quant", name: "量化派", role: "数据、概率与风险", mark: "量" }
] as const;

export const roundtableWelcome = "欢迎来到慧投圆桌。主持人、价值派、趋势派和量化派会从不同角度讨论你的问题。可以选择参与角色，也可以在问题中 @价值派、@趋势派 或 @量化派；@主持人 或 @all 邀请全部角色。";

export function roundtableParticipants(text: string, selected: string[] = []) {
  if (/@(?:主持人|所有人|all\b)/i.test(text)) return [];
  const mentioned = roundtableAgents.filter(agent => agent.id !== "orchestrator" && text.includes(`@${agent.name}`)).map(agent => agent.id);
  return mentioned.length ? mentioned : selected.filter(id => roundtableAgents.some(agent => agent.id === id));
}

export type ResearchStreamChunk = { type?: string; agent_id?: string; content?: string; error?: string };

/** Handles split UTF-8, CRLF, heartbeat comments and a final unterminated frame. */
export async function readResearchStream(response: Response, onChunk: (chunk: ResearchStreamChunk) => void) {
  if (!response.ok || !response.body) {
    let message = `请求失败（HTTP ${response.status}）`;
    try { const body = await response.json(); if (typeof body.error === "string") message = body.error; } catch { /* Use the controlled HTTP diagnostic. */ }
    throw new Error(message);
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "", data: string[] = [], completed = false;
  const dispatch = () => {
    if (!data.length) return;
    const payload = data.join("\n");
    data = [];
    if (payload.trim() === "[DONE]") { completed = true; return; }
    let chunk: ResearchStreamChunk;
    try { chunk = JSON.parse(payload) as ResearchStreamChunk; }
    catch { throw new Error("圆桌回复格式异常，请稍后重试。"); }
    if (!chunk || typeof chunk !== "object") throw new Error("圆桌回复格式异常，请稍后重试。");
    if (chunk.error) throw new Error(chunk.error);
    onChunk(chunk);
  };
  const line = (value: string) => {
    const text = value.replace(/\r$/, "");
    if (!text) dispatch();
    else if (text.startsWith("data:")) data.push(text.slice(5).replace(/^ /, ""));
  };
  try {
    while (!completed) {
      const { value, done } = await reader.read();
      buffer += done ? decoder.decode() : decoder.decode(value, { stream: true });
      let newline: number;
      while ((newline = buffer.indexOf("\n")) >= 0 && !completed) {
        line(buffer.slice(0, newline)); buffer = buffer.slice(newline + 1);
      }
      if (done) { if (buffer) line(buffer); dispatch(); break; }
    }
    if (!completed) throw new Error("连接已中断，已收到的内容已保留，请稍后继续提问。");
  } finally { await reader.cancel().catch(() => undefined); reader.releaseLock(); }
}

type Candle = { date: string; open: number; high: number; low: number; close: number; volume: number };
export function technicalOverview(candles: Candle[]) {
  const rows = candles.filter(row => Number.isFinite(row.close) && row.close > 0);
  const last = rows[rows.length - 1];
  const average = (length: number) => rows.length >= length ? rows.slice(-length).reduce((sum, row) => sum + row.close, 0) / length : null;
  const ma5 = average(5), ma20 = average(20);
  // Exclude the current candle from its own comparison baseline.
  const previous = rows.slice(-21, -1).filter(row => Number.isFinite(row.volume) && row.volume >= 0);
  const baseline = previous.length === 20 ? previous.reduce((sum, row) => sum + row.volume, 0) / 20 : 0;
  return { ma5, ma20, last: last?.close ?? null,
    trend: !last || ma5 == null || ma20 == null ? "数据不足" : last.close > ma5 && ma5 > ma20 ? "均线偏强" : last.close < ma5 && ma5 < ma20 ? "均线偏弱" : "均线交错",
    volumeRatio: baseline > 0 && last && Number.isFinite(last.volume) && last.volume >= 0 ? last.volume / baseline : null,
    lastDate: last?.date ?? "" };
}
