import type { MarketId, StockQuote } from "./App";

export type WorkspaceView = "home" | "accounts" | "watchlist" | "chat" | "profile" | "market" | "stock";
export type StockOrigin = { view: WorkspaceView; marketId: MarketId; stock: StockQuote | null; scrollY: number };

export function enterStock(trail: StockOrigin[], origin: StockOrigin) {
  return origin.view === "stock" ? [...trail, origin] : [origin];
}

export function leaveStock(trail: StockOrigin[]) {
  return { origin: trail[trail.length - 1], trail: trail.slice(0, -1) };
}

export function stockBackLabel(origin?: StockOrigin, marketName?: string) {
  if (!origin || origin.view === "home") return "返回总览";
  if (origin.view === "market") return `返回${marketName?.replace(/\s/g, "") || "市场"}板块`;
  if (origin.view === "stock") return `返回 ${origin.stock?.symbol || "上一标的"}`;
  return { accounts: "返回实体账户", watchlist: "返回自选列表", chat: "返回 AI 分析", profile: "返回设置" }[origin.view];
}
