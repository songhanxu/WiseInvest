import { useCallback, useEffect, useRef, useState } from "react";
import { Check, Loader2, Plus, RefreshCw } from "lucide-react";
import { fetchWatchlist, requestJson } from "./App";
import type { MarketId, StockQuote } from "./App";
import { watchlistCode, watchlistState } from "./watchlist-state";

export function StockWatchlistButton({ apiBase, token, stock, onNeedToken }: {
  apiBase: string; token: string; stock: StockQuote & { market: MarketId }; onNeedToken: () => void;
}) {
  const state = watchlistState(apiBase, token);
  const [snapshot, setSnapshot] = useState(() => state.snapshot(stock.market));
  const [loading, setLoading] = useState(() => Boolean(token && !state.snapshot(stock.market)));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const busy = useRef(false);
  const session = useRef(0);
  const inWatchlist = Boolean(snapshot?.rows.some(row => watchlistCode(row) === watchlistCode(stock)));
  const refresh = useCallback(async () => {
    if (!token) return;
    const version = session.current;
    setLoading(true); setError("");
    try {
      await fetchWatchlist(apiBase, token, stock.market);
      if (version === session.current) setSnapshot(state.snapshot(stock.market));
    } catch (err) { if (version === session.current) setError(err instanceof Error ? err.message : "自选状态读取失败"); }
    finally { if (version === session.current) setLoading(false); }
  }, [apiBase, token, stock.market, state]);

  useEffect(() => {
    session.current++; busy.current = false;
    const cached = state.snapshot(stock.market);
    setSnapshot(cached); setError(""); setSaving(false); setLoading(false);
    const unsubscribe = state.subscribe(change => { if (change.market === stock.market) setSnapshot(state.snapshot(stock.market)); });
    if (token && (!cached || Date.now() - cached.updatedAt >= 60000)) void refresh();
    return () => { session.current++; unsubscribe(); };
  }, [state, stock.market, refresh]);

  async function toggle() {
    if (!token) { onNeedToken(); return; }
    if (!snapshot) { void refresh(); return; }
    if (busy.current) return;
    busy.current = true;
    const version = session.current;
    const remove = inWatchlist;
    setSaving(true); setError("");
    try {
      await requestJson(apiBase, "/api/v1/stocks/watchlist", token, { method: remove ? "DELETE" : "POST", body: JSON.stringify({ stock_code: watchlistCode(stock), symbol: stock.symbol, name: stock.name, market: stock.market }) });
      // Synchronize all cached views even if this detail page has been closed.
      state.changed(stock, remove);
    } catch (err) { if (version === session.current) setError(err instanceof Error ? err.message : "保存自选失败"); }
    finally { if (version === session.current) { busy.current = false; setSaving(false); } }
  }

  const initialLoading = Boolean(token && !snapshot && loading);
  return <div className="stock-watchlist-control">
    <button type="button" className={`ghost-button stock-watchlist-button ${inWatchlist ? "is-watched" : ""}`} disabled={saving || initialLoading} onClick={() => void toggle()} aria-label={initialLoading ? "正在读取自选状态" : token && !snapshot ? "重试读取自选状态" : inWatchlist ? "移出自选" : "加入自选"} title={inWatchlist ? "点击移出自选列表" : "加入你的自选列表"}>
      {saving || initialLoading ? <Loader2 size={16} className="spin" /> : token && !snapshot ? <RefreshCw size={16} /> : inWatchlist ? <Check size={16} /> : <Plus size={16} />}
      {saving ? "保存中…" : initialLoading ? "读取自选…" : token && !snapshot ? "重试自选" : inWatchlist ? "移出自选" : "加入自选"}
    </button>
    {error && <small className="stock-watchlist-error" role="alert">{error}</small>}
  </div>;
}
