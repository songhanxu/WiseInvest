import type { StockQuote } from "./App";

export function watchlistCode(stock: Pick<StockQuote, "id" | "symbol" | "market">) {
  let code = stock.id.trim().toUpperCase();
  if (stock.market === "crypto") {
    if (["BTC_PRICE", "ETH_PRICE", "SOL_PRICE", "BNB_PRICE"].includes(code)) return code.split("_")[0];
    if (code.length > 4) code = code.replace(/\/?USDT$/, "");
    return code;
  }
  if (stock.market === "hk_stock") {
    const number = code.replace(/^HK|\.HK$/g, "");
    return /^\d{1,5}$/.test(number) ? number.padStart(5, "0") : code;
  }
  if (stock.market === "a_share") {
    if (/^\d{6}$/.test(code)) code = (/^(SH|SZ|BJ)/i.exec(stock.symbol)?.[0] || (/^[03]/.test(code) ? "SZ" : "SH")) + code;
    return code.toLowerCase();
  }
  return code;
}

export function applyWatchlistChange(rows: StockQuote[], stock: StockQuote, remove: boolean) {
  const code = watchlistCode(stock);
  const matches = (row: StockQuote) => row.market === stock.market && watchlistCode(row) === code;
  if (remove) return rows.filter(row => !matches(row));
  return rows.some(matches) ? rows : [...rows, { ...stock, id: code }];
}

type Snapshot = { rows: StockQuote[]; updatedAt: number };
type Change = { type: "snapshot"; market: string } | { type: "mutation"; market: string; stock: StockQuote; remove: boolean };

export class WatchlistState {
  version = 0;
  private markets = new Map<string, Snapshot>();
  private listeners = new Set<(change: Change) => void>();
  private requests = new Map<string, { version: number; promise: Promise<StockQuote[]> }>();
  snapshot(market: string) { return this.markets.get(market); }
  subscribe(listener: (change: Change) => void) { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; }
  record(market: string, rows: StockQuote[], version: number) {
    if (version !== this.version) return;
    this.markets.set(market, { rows, updatedAt: Date.now() });
    this.listeners.forEach(listener => listener({ type: "snapshot", market }));
  }
  load(market: string, loader: () => Promise<StockQuote[]>) {
    const existing = this.requests.get(market);
    if (existing?.version === this.version) return existing.promise;
    const version = this.version;
    const promise = loader().then(rows => {
      if (!Array.isArray(rows)) throw new Error("自选列表格式不正确");
      this.record(market, rows, version); return rows;
    }).finally(() => { if (this.requests.get(market)?.promise === promise) this.requests.delete(market); });
    this.requests.set(market, { version, promise });
    return promise;
  }
  changed(stock: StockQuote, remove: boolean) {
    this.version++;
    const snapshot = this.markets.get(stock.market);
    if (snapshot) this.markets.set(stock.market, { ...snapshot, rows: applyWatchlistChange(snapshot.rows, stock, remove) });
    this.listeners.forEach(listener => listener({ type: "mutation", market: stock.market, stock, remove }));
  }
}

const scopes = new Map<string, WatchlistState>();
export function watchlistState(apiBase: string, token: string) {
  const key = JSON.stringify([apiBase, token]);
  let state = scopes.get(key);
  if (!state) state = new WatchlistState();
  scopes.delete(key); scopes.set(key, state);
  if (scopes.size > 8) scopes.delete(scopes.keys().next().value!);
  return state;
}
