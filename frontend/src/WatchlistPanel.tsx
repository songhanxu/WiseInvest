import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, Loader2, Plus, RefreshCw, Search, Trash2, ListPlus } from "lucide-react";
import { requestJson } from "./App";
import { StockResearchTable, numeric, stockKey, useStockFundamentals } from "./StockResearchTable";
import type { Market, MarketId, StockQuote } from "./App";
import { PriceAlertSettings, TargetPriceCell, usePriceAlerts } from "./PriceAlerts";
import { useDismissiblePopover } from "./useDismissiblePopover";

export function WatchlistPanel({ apiBase, token, markets, onOpenStock, onNeedToken }: {
  apiBase: string;
  token: string;
  markets: Market[];
  onOpenStock: (market: Market, stock: StockQuote) => void;
  onNeedToken: () => void;
}) {
  const priceAlerts = usePriceAlerts(apiBase, token);
  const { rootRef: searchRef, isOpen: searchOpen, setIsOpen: setSearchOpen } = useDismissiblePopover<HTMLElement>();
  const [stocks, setStocks] = useState<StockQuote[]>([]);
  const { fundamentals, financialLoading } = useStockFundamentals(apiBase, stocks, token);
  const [loading, setLoading] = useState(false);
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
  const membership = useMemo(() => new Set(stocks.map(stockKey)), [stocks]);

  const refresh = useCallback(async () => {
    const version = ++requestVersion.current;
    const mutation = mutationVersion.current;
    if (!token) { setStocks([]); setLoading(false); return; }
    setLoading(true); setError("");
    try {
      let rows: StockQuote[];
      const response = await fetch(`${apiBase}/api/v1/stocks/watchlist/all`, { headers: { Authorization: `Bearer ${token}` } });
      if (response.status === 404) {
        // Allows the new page to read existing lists while the backend is upgraded.
        const partial = await Promise.allSettled(markets.map(market => requestJson<StockQuote[]>(apiBase, `/api/v1/stocks/watchlist?market=${market.id}`, token)));
        if (partial.every(result => result.status === "rejected")) throw new Error("无法读取自选列表，请检查登录状态和后端连接");
        rows = partial.flatMap(result => result.status === "fulfilled" ? result.value : []);
        const failures = partial.flatMap((result, i) => result.status === "rejected" ? [markets[i].displayName] : []);
        if (failures.length && version === requestVersion.current) setError(`${failures.join("、")}自选暂未加载，可稍后刷新`);
      } else {
        if (!response.ok) throw new Error(response.status === 401 ? "登录已过期，请重新登录" : "自选列表加载失败");
        rows = await response.json() as StockQuote[];
      }
      if (version !== requestVersion.current || mutation !== mutationVersion.current) return;
      if (!Array.isArray(rows)) throw new Error("后端返回的自选列表格式不正确");
      setStocks([...new Map(rows.map(stock => [stockKey(stock), stock])).values()]);
      setUpdatedAt(new Date().toLocaleTimeString("zh-CN", { hour12: false }));
    } catch (err) {
      if (version === requestVersion.current) setError(err instanceof Error ? err.message : "自选列表加载失败");
    } finally { if (version === requestVersion.current) setLoading(false); }
  }, [apiBase, token, markets]);

  useEffect(() => {
    actionSession.current++;
    setStocks([]); setBusy([]); setActionError(""); setUpdatedAt("");
    void refresh();
    const timer = window.setInterval(() => { if (!document.hidden) void refresh(); }, 60000);
    return () => { window.clearInterval(timer); requestVersion.current++; actionSession.current++; };
  }, [refresh]);

  useEffect(() => {
    let cancelled = false;
    const text = query.trim();
    setSearchError("");
    if (!text || !searchOpen) { setResults([]); setSearching(false); return; }
    setResults([]); setSearching(true);
    const timer = window.setTimeout(async () => {
      const targets = markets.filter(market => searchMarket === "all" || market.id === searchMarket);
      const responses = await Promise.allSettled(targets.map(market => requestJson<StockQuote[]>(apiBase, `/api/v1/stocks/search?market=${market.id}&q=${encodeURIComponent(text)}`, "")));
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
      if (session !== actionSession.current) return;
      mutationVersion.current++;
      setStocks(previous => remove ? previous.filter(row => stockKey(row) !== key) : membership.has(key) ? previous : [...previous, stock]);
      void refresh();
    } catch (err) { if (session === actionSession.current) setActionError(err instanceof Error ? err.message : "保存自选失败"); }
    finally { if (session === actionSession.current) setBusy(previous => previous.filter(item => item !== key)); }
  }

  const visible = stocks.filter(stock => filter === "all" || stock.market === filter);
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
          <div><strong>{stock.name}</strong><span>{stock.symbol} · {market?.displayName ?? stock.market}{stock.product_type === "quanto_perpetual" ? " · Quanto" : stock.product_type === "stock_token" ? " · 股票代币" : ""}</span></div>
          <span className="mono">{numeric(stock.current_price)} {stock.currency ?? ""}</span>
          <button className="ghost-button" disabled={saved || busy.includes(key)} onClick={() => void changeMembership(stock, false)}>{busy.includes(key) ? <Loader2 size={15} className="spin" /> : saved ? <Check size={15} /> : <Plus size={15} />}{saved ? "已添加" : "添加自选"}</button>
        </div>;
      })}</div>}
      </div>}
    </section>

    <div className="watchlist-toolbar"><div className="watchlist-filters">{[{ id: "all", displayName: "全部" }, ...markets].map(market => <button key={market.id} className={filter === market.id ? "active" : ""} onClick={() => setFilter(market.id as MarketId | "all")}>{market.displayName}<span>{market.id === "all" ? stocks.length : stocks.filter(stock => stock.market === market.id).length}</span></button>)}</div>
      <div className="watchlist-update"><span className="watchlist-note" role="status">{loading ? "行情更新中…" : updatedAt ? `更新于 ${updatedAt}` : financialLoading ? "财报加载中…" : ""}</span><button type="button" className="watchlist-update-button" aria-label="更新自选行情" title="更新自选报价和提醒状态" onClick={() => void Promise.all([refresh(), priceAlerts.refresh()])} disabled={loading || !token}><RefreshCw size={13} className={loading ? "spin" : ""} /><span>更新行情</span></button></div>
    </div>
    {error && <p className="form-note danger" role="alert">{error}</p>}
    {actionError && <p className="form-note danger" role="alert">{actionError}</p>}
    {financialUnavailable && <p className="watchlist-note">部分财报指标暂未取得，悬停单元格可查看原因；价格和自选功能不受影响。</p>}

    {token && <PriceAlertSettings alerts={priceAlerts} />}
    <StockResearchTable stocks={visible} markets={markets} fundamentals={fundamentals} onOpenStock={onOpenStock}
      emptyMessage={loading ? "正在汇总自选列表…" : token ? "暂无自选，在上方搜索并添加标的。" : "登录后展示你的自选列表。"}
      renderTarget={stock => <TargetPriceCell stock={{ ...stock, holding_id: `watchlist:${stockKey(stock)}`, account_id: "watchlist", currency: stock.currency || ({ a_share: "CNY", us_stock: "USD", hk_stock: "HKD", crypto: "USDT" } as Record<string, string>)[stock.market] }} alerts={priceAlerts} />}
      renderAction={stock => <button className="watchlist-remove" aria-label={`移除 ${stock.name}`} disabled={busy.includes(stockKey(stock))} onClick={() => void changeMembership(stock, true)}>{busy.includes(stockKey(stock)) ? <Loader2 size={15} className="spin" /> : <Trash2 size={15} />}</button>}
    />
    <p className="watchlist-data-note">美股优先显示 Bitget 股票代币行情，港股显示 Quanto 合约行情。EPS、PE 和远期 PE 属于标的公司；公司指引与分析师预测分别列示。缺失数据显示「—」，加密货币及基金的公司财报指标显示「不适用」。</p>
  </section>;
}
