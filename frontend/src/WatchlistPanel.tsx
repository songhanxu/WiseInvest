import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, Loader2, Plus, RefreshCw, Search, Trash2, ListPlus } from "lucide-react";
import { requestJson } from "./App";
import type { Market, MarketId, StockQuote } from "./App";

type Fundamentals = {
  eps: number | null;
  pe: number | null;
  forward_pe: number | null;
  forward_eps: number | null;
  forward_period?: string;
  guidance_eps: number | null;
  guidance_eps_low: number | null;
  guidance_eps_high: number | null;
  guidance_period?: string;
  guidance_basis?: string;
  guidance_report_date?: string;
  guidance_source_url?: string;
  next_earnings_date?: string;
  earnings_date_status?: string;
  currency?: string;
  source?: string;
  as_of?: string;
  status: "available" | "unavailable" | "not_applicable";
  message?: string;
};

const stockKey = (stock: StockQuote) => `${stock.market}:${stock.id}`;
const numeric = (value?: number | null) => value == null || !Number.isFinite(value) ? "—" : new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 2 }).format(value);

export function WatchlistPanel({ apiBase, token, markets, onOpenStock, onNeedToken }: {
  apiBase: string;
  token: string;
  markets: Market[];
  onOpenStock: (market: Market, stock: StockQuote) => void;
  onNeedToken: () => void;
}) {
  const [stocks, setStocks] = useState<StockQuote[]>([]);
  const [fundamentals, setFundamentals] = useState<Record<string, Fundamentals>>({});
  const [loading, setLoading] = useState(false);
  const [financialLoading, setFinancialLoading] = useState(false);
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
    setStocks([]); setFundamentals({}); setBusy([]); setActionError(""); setUpdatedAt("");
    void refresh();
    const timer = window.setInterval(() => { if (!document.hidden) void refresh(); }, 60000);
    return () => { window.clearInterval(timer); requestVersion.current++; actionSession.current++; };
  }, [refresh]);

  const financialKeys = stocks.map(stockKey).sort().join("|");
  useEffect(() => {
    let cancelled = false;
    setFinancialLoading(Boolean(financialKeys));
    setFundamentals({});
    const rows = stocks.slice();
    let cursor = 0;
    async function worker() {
      while (cursor < rows.length && !cancelled) {
        const stock = rows[cursor++];
        let value: Fundamentals;
        try { value = await requestJson<Fundamentals>(apiBase, `/api/v1/stocks/fundamentals?market=${stock.market}&code=${encodeURIComponent(stock.id)}`, ""); }
        catch { value = { eps: null, pe: null, forward_pe: null, forward_eps: null, guidance_eps: null, guidance_eps_low: null, guidance_eps_high: null, status: "unavailable", message: "财报数据暂不可用，请检查后端与基本面数据源" }; }
        if (!cancelled) setFundamentals(previous => ({ ...previous, [stockKey(stock)]: value }));
      }
    }
    Promise.all(Array.from({ length: Math.min(4, rows.length) }, worker)).finally(() => { if (!cancelled) setFinancialLoading(false); });
    return () => { cancelled = true; };
    // Quote refreshes retain the same membership and don't refetch slow financial data.
  }, [apiBase, token, financialKeys]);

  useEffect(() => {
    let cancelled = false;
    const text = query.trim();
    setSearchError("");
    if (!text) { setResults([]); setSearching(false); return; }
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
  }, [apiBase, query, searchMarket, markets]);

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
      <div><p className="eyebrow">Unified Watchlist</p><h2>自选列表</h2><p>汇总 A 股、美股、港股和加密货币自选，点击名称查看行情与分析。</p></div>
      <button className="ghost-button" onClick={() => void refresh()} disabled={loading || !token}><RefreshCw size={16} className={loading ? "spin" : ""} />刷新</button>
    </div>

    {!token && <div className="auth-callout"><ListPlus size={18} /><span>登录后读取并保存你的全部自选列表。</span><button onClick={onNeedToken}>去登录</button></div>}

    <section className="surface-card watchlist-search-card">
      <div className="watchlist-search-controls">
        <label className="watchlist-search-input"><Search size={18} /><input aria-label="搜索添加自选" value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索名称或代码，如苹果、AAPL、腾讯、00700、BTC" /></label>
        <select aria-label="搜索市场" value={searchMarket} onChange={event => setSearchMarket(event.target.value as MarketId | "all")}><option value="all">所有市场</option>{markets.map(market => <option value={market.id} key={market.id}>{market.displayName}</option>)}</select>
      </div>
      {searching && <p className="watchlist-note"><Loader2 size={15} className="spin" />搜索中…</p>}
      {searchError && <p className="form-note danger" role="status">{searchError}</p>}
      {query.trim() && !searching && !results.length && !searchError && <p className="watchlist-note">未找到匹配标的，试试股票代码或切换市场。</p>}
      {results.length > 0 && <div className="watchlist-search-results">{results.map(stock => {
        const market = markets.find(item => item.id === stock.market);
        const key = stockKey(stock); const saved = membership.has(key);
        return <div className="watchlist-search-result" key={key}>
          <div><strong>{stock.name}</strong><span>{stock.symbol} · {market?.displayName ?? stock.market}{stock.product_type === "quanto_perpetual" ? " · Quanto" : stock.product_type === "stock_token" ? " · 股票代币" : ""}</span></div>
          <span className="mono">{numeric(stock.current_price)} {stock.currency ?? ""}</span>
          <button className="ghost-button" disabled={saved || busy.includes(key)} onClick={() => void changeMembership(stock, false)}>{busy.includes(key) ? <Loader2 size={15} className="spin" /> : saved ? <Check size={15} /> : <Plus size={15} />}{saved ? "已添加" : "添加自选"}</button>
        </div>;
      })}</div>}
    </section>

    <div className="watchlist-toolbar"><div className="watchlist-filters">{[{ id: "all", displayName: "全部" }, ...markets].map(market => <button key={market.id} className={filter === market.id ? "active" : ""} onClick={() => setFilter(market.id as MarketId | "all")}>{market.displayName}<span>{market.id === "all" ? stocks.length : stocks.filter(stock => stock.market === market.id).length}</span></button>)}</div><span className="watchlist-note">{financialLoading ? "财报数据加载中…" : updatedAt ? `行情更新 ${updatedAt}` : ""}</span></div>
    {error && <p className="form-note danger" role="alert">{error}</p>}
    {actionError && <p className="form-note danger" role="alert">{actionError}</p>}
    {financialUnavailable && <p className="watchlist-note">部分财报指标暂未取得，悬停单元格可查看原因；价格和自选功能不受影响。</p>}

    <div className="surface-card watchlist-table-container">
      <table className="unified-watchlist-table">
        <thead><tr><th>名称</th><th>代码</th><th>市场</th><th>价格</th><th>EPS <small>TTM</small></th><th>PE <small>TTM</small></th><th>远期 PE</th><th>最新财报预期 EPS <small>公司指引</small></th><th>下份财报时间</th><th><span className="sr-only">操作</span></th></tr></thead>
        <tbody>{visible.map(stock => {
          const market = markets.find(item => item.id === stock.market); const key = stockKey(stock); const f = fundamentals[key];
          const na = stock.market === "crypto" || f?.status === "not_applicable" || /\bETF\b|基金/i.test(stock.name);
          const empty = na ? "不适用" : "—";
          const guidance = f?.guidance_eps != null ? numeric(f.guidance_eps) : f?.guidance_eps_low != null && f.guidance_eps_high != null ? `${numeric(f.guidance_eps_low)}–${numeric(f.guidance_eps_high)}` : empty;
          const financialHint = [f?.message, f?.source, f?.currency ? `EPS 币种：${f.currency}` : "", f?.as_of ? `数据时间：${f.as_of}` : ""].filter(Boolean).join(" · ");
          return <tr key={key}>
            <td><button className="watchlist-name" onClick={() => market && onOpenStock(market, stock)}>{stock.name}</button><small>{stock.source === "bitget" ? (stock.product_type === "quanto_perpetual" ? "Bitget · Quanto 合约" : "Bitget · 股票代币") : stock.source === "tencent" ? "腾讯 · 兜底行情" : ""}</small></td>
            <td className="mono" title={stock.provider_symbol}>{stock.symbol || stock.id}</td>
            <td><span className="watchlist-market-tag">{market?.displayName ?? stock.market}</span></td>
            <td className="mono">{stock.quote_available === false || stock.current_price <= 0 ? "—" : numeric(stock.current_price)}<small>{stock.currency || (stock.market === "a_share" ? "CNY" : stock.market === "hk_stock" ? "HKD" : stock.market === "crypto" ? "USDT" : "USD")}{stock.change_basis === "24h" ? " · 24h" : ""}</small></td>
            <td className="mono" title={financialHint}>{f?.eps != null ? numeric(f.eps) : empty}{f?.eps != null && <small>{f.currency}</small>}</td>
            <td className="mono" title={financialHint}>{f?.pe != null && f.pe > 0 ? numeric(f.pe) : f?.eps != null && f.eps < 0 ? "亏损" : empty}</td>
            <td className="mono" title={financialHint}>{f?.forward_pe != null ? numeric(f.forward_pe) : empty}{f?.forward_period && <small>年度期末 {f.forward_period}</small>}</td>
            <td className="mono" title={f?.guidance_report_date ? `财报发布 ${f.guidance_report_date}` : financialHint}>{f?.guidance_source_url && /^https?:\/\//.test(f.guidance_source_url) ? <a href={f.guidance_source_url} target="_blank" rel="noreferrer">{guidance}</a> : guidance}<small>{f?.guidance_period ? `${f.guidance_period} · ${f.guidance_basis || ""}` : na ? "" : "未取得公司指引"}</small></td>
            <td className="mono">{f?.next_earnings_date || empty}{f?.next_earnings_date && <small>{f.earnings_date_status === "confirmed" ? "公司已确认" : "预计日期"}</small>}</td>
            <td><button className="watchlist-remove" aria-label={`移除 ${stock.name}`} disabled={busy.includes(key)} onClick={() => void changeMembership(stock, true)}>{busy.includes(key) ? <Loader2 size={15} className="spin" /> : <Trash2 size={15} />}</button></td>
          </tr>;
        })}</tbody>
      </table>
      {!visible.length && <div className="empty-state">{loading ? "正在汇总自选列表…" : token ? "暂无自选，在上方搜索并添加标的。" : "登录后展示你的自选列表。"}</div>}
    </div>
    <p className="watchlist-data-note">美股优先显示 Bitget 股票代币行情，港股显示 Quanto 合约行情。EPS、PE 和远期 PE 属于标的公司；公司指引与分析师预测分别列示。缺失数据显示「—」，加密货币及基金的公司财报指标显示「不适用」。</p>
  </section>;
}
