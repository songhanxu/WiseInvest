import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, Loader2, Plus, RefreshCw, Search, Trash2, ListPlus } from "lucide-react";
import { requestJson, searchStocks } from "./App";
import { StockResearchTable, numeric, stockKey, useStockFundamentals } from "./StockResearchTable";
import type { Market, MarketId, StockQuote } from "./App";
import { PriceAlertSettings, TargetPriceCell, usePriceAlerts } from "./PriceAlerts";
import { useDismissiblePopover } from "./useDismissiblePopover";
import { mergeQuoteRows } from "./quote-presentation";
import { applyWatchlistChange, watchlistState } from "./watchlist-state";

const WATCHLIST_FRESH_MS = 60000;

export function WatchlistPanel({ apiBase, token, markets, isActive = true, onOpenStock, onNeedToken }: {
  apiBase: string;
  token: string;
  markets: Market[];
  isActive?: boolean;
  onOpenStock: (market: Market, stock: StockQuote) => void;
  onNeedToken: () => void;
}) {
  const sharedWatchlist = watchlistState(apiBase, token);
  const priceAlerts = usePriceAlerts(apiBase, token, isActive);
  const { rootRef: searchRef, isOpen: searchOpen, setIsOpen: setSearchOpen } = useDismissiblePopover<HTMLElement>();
  const [stocks, setStocks] = useState<StockQuote[]>([]);
  const { fundamentals, financialLoading } = useStockFundamentals(apiBase, stocks, token);
  const [loading, setLoading] = useState(Boolean(token));
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");
  const [filter, setFilter] = useState<MarketId | "all">("all");
  const [searchMarket, setSearchMarket] = useState<MarketId | "all">("all");
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<StockQuote[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState("");
  const [busy, setBusy] = useState<string[]>([]);
  const [updatedAt, setUpdatedAt] = useState("");
  const requestVersion = useRef(0);
  const mutationVersion = useRef(0);
  const actionSession = useRef(0);
  const activeRef = useRef(isActive); activeRef.current = isActive;
  const refreshInFlight = useRef(false);
  const lastAttemptAt = useRef(0);
  const lastSuccessAt = useRef(0);
  const membershipDirty = useRef(false);
  const membership = useMemo(() => new Set(stocks.map(stockKey)), [stocks]);

  const refresh = useCallback(async (force = false) => {
    if (refreshInFlight.current && !force) return;
    const version = ++requestVersion.current;
    const mutation = mutationVersion.current;
    const sharedVersion = sharedWatchlist.version;
    if (!token) { setStocks([]); setLoading(false); return; }
    refreshInFlight.current = true;
    lastAttemptAt.current = Date.now();
    setLoading(true); setError("");
    try {
      let rows: StockQuote[];
      let loadedMarkets = new Set<string>(markets.map(market => market.id));
      const response = await fetch(`${apiBase}/api/v1/stocks/watchlist/all`, { headers: { Authorization: `Bearer ${token}` } });
      if (response.status === 404) {
        // Allows the new page to read existing lists while the backend is upgraded.
        const partial = await Promise.allSettled(markets.map(market => requestJson<StockQuote[]>(apiBase, `/api/v1/stocks/watchlist?market=${market.id}`, token)));
        if (partial.every(result => result.status === "rejected")) throw new Error("无法读取自选列表，请检查登录状态和后端连接");
        rows = partial.flatMap(result => result.status === "fulfilled" ? result.value : []);
        loadedMarkets = new Set(partial.flatMap((result, index) => result.status === "fulfilled" ? [markets[index].id] : []));
        const failures = partial.flatMap((result, i) => result.status === "rejected" ? [markets[i].displayName] : []);
        if (failures.length && version === requestVersion.current) setError(`${failures.join("、")}自选暂未加载，可稍后刷新`);
      } else {
        if (!response.ok) throw new Error(response.status === 401 ? "登录已过期，请重新登录" : "自选列表加载失败");
        rows = await response.json() as StockQuote[];
      }
      if (version !== requestVersion.current || mutation !== mutationVersion.current || sharedVersion !== sharedWatchlist.version) return;
      if (!Array.isArray(rows)) throw new Error("后端返回的自选列表格式不正确");
      markets.filter(market => loadedMarkets.has(market.id)).forEach(market => sharedWatchlist.record(market.id, rows.filter(row => row.market === market.id), sharedVersion));
      setStocks(previous => mergeQuoteRows(previous, [...new Map([...rows, ...previous.filter(row => !loadedMarkets.has(row.market)).map(row => ({ ...row, quote_stale: true }))].map(stock => [stockKey(stock), stock])).values()]));
      if (rows.some(stock => stock.quote_available === false || !(stock.current_price > 0))) setError("部分行情暂不可用，已有报价保留并标注为缓存。");
      lastSuccessAt.current = Date.now();
      membershipDirty.current = false;
      setUpdatedAt(new Date().toLocaleTimeString("zh-CN", { hour12: false }));
    } catch (err) {
      if (version === requestVersion.current) {
        setStocks(previous => previous.map(stock => ({ ...stock, quote_stale: true })));
        setError(err instanceof Error ? err.message : "自选列表加载失败");
      }
    } finally { if (version === requestVersion.current) { refreshInFlight.current = false; setLoading(false); } }
  }, [apiBase, token, markets, sharedWatchlist]);

  useEffect(() => {
    actionSession.current++;
    setStocks([]); setBusy([]); setActionError(""); setUpdatedAt("");
    lastAttemptAt.current = 0; lastSuccessAt.current = 0; membershipDirty.current = false;
    if (activeRef.current) void refresh();
    const timer = window.setInterval(() => {
      if (activeRef.current && !document.hidden && Date.now() - lastAttemptAt.current >= WATCHLIST_FRESH_MS) void refresh();
    }, WATCHLIST_FRESH_MS);
    return () => { window.clearInterval(timer); requestVersion.current++; refreshInFlight.current = false; actionSession.current++; };
  }, [refresh]);

  useEffect(() => {
    if (!isActive) { setSearchOpen(false); return; }
    // Keep rows, fundamentals, table preferences and target-price drafts when
    // navigating away. Revalidate stale quotes without an empty loading screen.
    if (membershipDirty.current || !lastAttemptAt.current || Date.now() - lastSuccessAt.current >= WATCHLIST_FRESH_MS) void refresh();
  }, [isActive, refresh, setSearchOpen]);

  useEffect(() => sharedWatchlist.subscribe(change => {
    if (change.type !== "mutation") return;
    mutationVersion.current++; requestVersion.current++; refreshInFlight.current = false;
    membershipDirty.current = true;
    setStocks(previous => applyWatchlistChange(previous, change.stock, change.remove));
    if (activeRef.current) void refresh(true);
    else setLoading(false);
  }), [sharedWatchlist, refresh]);

  useEffect(() => {
    let cancelled = false;
    const text = query.trim();
    setSearchError("");
    if (!text || !searchOpen) { setResults([]); setSearching(false); return; }
    setResults([]); setSearching(true);
    const timer = window.setTimeout(async () => {
      const targets = markets.filter(market => searchMarket === "all" || market.id === searchMarket);
      const responses = await Promise.allSettled(targets.map(market => searchStocks(apiBase, market.id, text)));
      if (cancelled) return;
      setResults(responses.flatMap(result => result.status === "fulfilled" && Array.isArray(result.value) ? result.value : []));
      const failed = responses.flatMap((result, i) => result.status === "rejected" ? [targets[i].displayName] : []);
      if (failed.length) setSearchError(`${failed.join("、")}搜索暂不可用`);
      setSearching(false);
    }, 350);
    return () => { cancelled = true; window.clearTimeout(timer); };
  }, [apiBase, query, searchMarket, markets, searchOpen]);

  async function changeMembership(stock: StockQuote, remove: boolean) {
    if (!token) { onNeedToken(); return; }
    const key = stockKey(stock);
    if (busy.includes(key)) return;
    const session = actionSession.current;
    setBusy(previous => [...previous, key]); setActionError("");
    try {
      await requestJson(apiBase, "/api/v1/stocks/watchlist", token, { method: remove ? "DELETE" : "POST", body: JSON.stringify({ stock_code: stock.id, symbol: stock.symbol, name: stock.name, market: stock.market }) });
      sharedWatchlist.changed(stock, remove);
    } catch (err) { if (session === actionSession.current) setActionError(err instanceof Error ? err.message : "保存自选失败"); }
    finally { if (session === actionSession.current) setBusy(previous => previous.filter(item => item !== key)); }
  }

  const visible = stocks.filter(stock => filter === "all" || stock.market === filter);
  const initialLoading = loading && !lastSuccessAt.current && !stocks.length;
  const financialUnavailable = stocks.some(stock => fundamentals[stockKey(stock)]?.status === "unavailable");
  return <section className="workspace watchlist-workspace">
    <div className="watchlist-heading">
      <div><p className="eyebrow">我的投资</p><h2>自选列表</h2><p>值得关注的公司与资产，汇成一份清晰的研究清单。</p></div>
    </div>

    {!token && <div className="auth-callout"><ListPlus size={18} /><span>登录后读取并保存你的全部自选列表。</span><button onClick={onNeedToken}>去登录</button></div>}

    <section className="surface-card watchlist-search-card" ref={searchRef}>
      <div className="watchlist-search-controls">
        <label className="watchlist-search-input"><Search size={18} /><input aria-label="搜索添加自选" value={query} onChange={event => { setQuery(event.target.value); setSearchOpen(true); }} onFocus={() => setSearchOpen(true)} onClick={() => setSearchOpen(true)} placeholder="搜索名称或代码，如苹果、AAPL、腾讯、00700、BTC" /></label>
        <label className="watchlist-search-scope"><select aria-label="搜索市场" value={searchMarket} onChange={event => { setSearchMarket(event.target.value as MarketId | "all"); setSearchOpen(true); }}><option value="all">全部市场</option>{markets.map(market => <option value={market.id} key={market.id}>{market.displayName}</option>)}</select><ChevronDown size={13} aria-hidden="true" /></label>
      </div>
      {searchOpen && query.trim() && <div className="watchlist-search-dropdown" role="region" aria-label="自选搜索结果">
      {searching && <p className="search-empty"><Loader2 size={15} className="spin" />搜索中…</p>}
      {searchError && <p className="search-empty danger" role="status">{searchError}</p>}
      {!searching && !results.length && !searchError && <p className="search-empty">未找到匹配标的，试试股票代码或切换市场。</p>}
      {results.length > 0 && <div className="watchlist-search-results">{results.map(stock => {
        const market = markets.find(item => item.id === stock.market);
        const key = stockKey(stock); const saved = membership.has(key);
        return <div className="watchlist-search-result" key={key}>
          <div><strong>{stock.name}</strong><span>{stock.symbol} · {market?.displayName ?? stock.market}</span></div>
          <span className="mono">{numeric(stock.current_price)} {stock.currency ?? ""}</span>
          <button className="ghost-button" disabled={saved || busy.includes(key)} onClick={() => void changeMembership(stock, false)}>{busy.includes(key) ? <Loader2 size={15} className="spin" /> : saved ? <Check size={15} /> : <Plus size={15} />}{saved ? "已添加" : "添加自选"}</button>
        </div>;
      })}</div>}
      </div>}
    </section>

    <div className="watchlist-toolbar"><div className="watchlist-filters">{[{ id: "all", displayName: "全部" }, ...markets].map(market => <button key={market.id} className={filter === market.id ? "active" : ""} onClick={() => setFilter(market.id as MarketId | "all")}>{market.displayName}<span>{initialLoading ? <span className="skeleton-block watch-count-skeleton" aria-hidden="true" /> : market.id === "all" ? stocks.length : stocks.filter(stock => stock.market === market.id).length}</span></button>)}</div>
      <div className="watchlist-update"><span className="watchlist-note" role="status">{loading ? "行情更新中…" : updatedAt ? `更新于 ${updatedAt}` : financialLoading ? "财报加载中…" : ""}</span><button type="button" className="watchlist-update-button" aria-label="更新自选行情" title="更新自选报价和提醒状态" onClick={() => void Promise.all([refresh(true), priceAlerts.refresh()])} disabled={loading || !token}><RefreshCw size={13} className={loading ? "spin" : ""} /><span>更新行情</span></button></div>
    </div>
    {error && <p className="form-note danger" role="alert">{error}</p>}
    {actionError && <p className="form-note danger" role="alert">{actionError}</p>}
    {financialUnavailable && <p className="watchlist-note">部分财报指标暂未取得，悬停单元格可查看原因；价格和自选功能不受影响。</p>}

    {token && <PriceAlertSettings alerts={priceAlerts} />}
    <StockResearchTable stocks={visible} markets={markets} fundamentals={fundamentals} onOpenStock={onOpenStock}
      loading={initialLoading}
      emptyMessage={token ? "暂无自选，在上方搜索并添加标的。" : "登录后展示你的自选列表。"}
      renderTarget={stock => <TargetPriceCell stock={{ ...stock, stale: stock.quote_stale, holding_id: `watchlist:${stockKey(stock)}`, account_id: "watchlist", currency: stock.currency || ({ a_share: "CNY", us_stock: "USD", hk_stock: "HKD", crypto: "USDT" } as Record<string, string>)[stock.market] }} alerts={priceAlerts} />}
      renderAction={stock => <button className="watchlist-remove" aria-label={`移除 ${stock.name}`} disabled={busy.includes(stockKey(stock))} onClick={() => void changeMembership(stock, true)}>{busy.includes(stockKey(stock)) ? <Loader2 size={15} className="spin" /> : <Trash2 size={15} />}</button>}
    />
    <p className="watchlist-data-note">行情优先使用 Bitget，美港股使用腾讯兜底，加密货币使用 Binance 兜底；每条价格显示实际来源和报价时间。EPS、PE 和远期 PE 属于标的公司；公司指引与分析师预测分别列示。缺失数据显示「—」，加密货币及基金的公司财报指标显示「不适用」。</p>
  </section>;
}
