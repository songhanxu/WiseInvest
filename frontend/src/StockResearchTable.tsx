import { Fragment, useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { requestJson } from "./App";
import type { Market, StockQuote } from "./App";
import { ArrowDownUp, Check, ChevronDown, Columns3, Info } from "lucide-react";
import { useDismissiblePopover } from "./useDismissiblePopover";
import { assetIntroduction } from "./assetIntroductions";

export type Fundamentals = {
  description?: string;
  metrics?: Record<string, { source: string; currency?: string; as_of: string }>;
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

export const stockKey = (stock: StockQuote) => `${stock.market}:${stock.id}`;
export const numeric = (value?: number | null) => value == null || !Number.isFinite(value) ? "—" : new Intl.NumberFormat("zh-CN", { maximumFractionDigits: value !== 0 && Math.abs(value) < 1 ? 12 : 2 }).format(value);

export function useStockFundamentals(apiBase: string, stocks: StockQuote[], scope: string) {
  const [fundamentals, setFundamentals] = useState<Record<string, Fundamentals>>({});
  const [financialLoading, setFinancialLoading] = useState(false);
  const financialKeys = stocks.filter(stock => ["us_stock", "hk_stock", "a_share"].includes(stock.market)).map(stockKey).sort().join("|");
  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();
    setFinancialLoading(Boolean(financialKeys));
    setFundamentals({});
    const rows = [...new Map(stocks.filter(stock => ["us_stock", "hk_stock", "a_share"].includes(stock.market)).map(stock => [stockKey(stock), stock])).values()];
    let cursor = 0;
    async function worker() {
      while (cursor < rows.length && !cancelled) {
        const stock = rows[cursor++];
        let value: Fundamentals;
        try { value = await requestJson<Fundamentals>(apiBase, `/api/v1/stocks/fundamentals?market=${stock.market}&code=${encodeURIComponent(stock.id)}`, "", { signal: controller.signal }); }
        catch { value = { eps: null, pe: null, forward_pe: null, forward_eps: null, guidance_eps: null, guidance_eps_low: null, guidance_eps_high: null, status: "unavailable", message: "财报数据暂不可用，请检查后端与基本面数据源" }; }
        if (!cancelled) setFundamentals(previous => ({ ...previous, [stockKey(stock)]: value }));
      }
    }
    Promise.all(Array.from({ length: Math.min(4, rows.length) }, worker)).finally(() => { if (!cancelled) setFinancialLoading(false); });
    return () => { cancelled = true; controller.abort(); };
    // Quote refreshes retain the same membership and don't refetch slow financial data.
  }, [apiBase, scope, financialKeys]);

  return { fundamentals, financialLoading };
}

export function StockResearchTable<T extends StockQuote>({ stocks, markets, fundamentals, onOpenStock, getRowKey, getHoldingValue, renderHolding, renderTarget, renderAction, emptyMessage }: {
  stocks: T[];
  markets: Market[];
  fundamentals: Record<string, Fundamentals>;
  onOpenStock: (market: Market, stock: StockQuote) => void;
  getRowKey?: (stock: T) => string;
  getHoldingValue?: (stock: T) => { amount: number; currency: string } | null;
  renderHolding?: (stock: T) => ReactNode;
  renderTarget?: (stock: T) => ReactNode;
  renderAction?: (stock: T) => ReactNode;
  emptyMessage: string;
}) {
  const [sort, setSort] = useState(getHoldingValue ? "market_value" : "original");
  const [hidden, setHidden] = useState<string[]>(["guidance"]);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [menu, setMenu] = useState<"sort" | "columns">("sort");
  const { rootRef: controlsRef, isOpen, setIsOpen } = useDismissiblePopover<HTMLDivElement>();
  const sortButton = useRef<HTMLButtonElement>(null);
  const columnsButton = useRef<HTMLButtonElement>(null);
  const controlId = useId();
  const detailsRef = useRef<HTMLTableCellElement>(null);
  useLayoutEffect(() => {
    const cell = detailsRef.current;
    const code = cell?.parentElement?.previousElementSibling?.querySelector(".watchlist-name");
    const table = cell?.closest("table");
    if (!cell || !code || !table) return;
    const align = () => cell.style.setProperty("--research-detail-inset", `${Math.max(12, code.getBoundingClientRect().left - cell.getBoundingClientRect().left)}px`);
    align();
    const observer = new ResizeObserver(align);
    observer.observe(table);
    observer.observe(code);
    return () => observer.disconnect();
  }, [expanded, stocks]);
  const columnLabels: Record<string, string> = { eps: "EPS", pe: "PE", forward: "远期 PE", guidance: "公司指引", earnings: "财报日期" };
  const sortOptions = [...(getHoldingValue ? [{ value: "market_value", label: "持仓市值：高到低" }] : []), { value: "original", label: "默认顺序" }, { value: "name", label: "名称排序" }, { value: "price", label: "价格：高到低" }, { value: "pe", label: "PE：低到高" }, { value: "earnings", label: "财报日期：近到远" }];
  const toggleMenu = (next: "sort" | "columns") => { setMenu(next); setIsOpen(previous => menu === next ? !previous : true); };
  const sorted = [...stocks].sort((a, b) => {
    if (sort === "market_value" && getHoldingValue) {
      const av = getHoldingValue(a), bv = getHoldingValue(b);
      if (!av) return bv ? 1 : 0;
      if (!bv) return -1;
      // Different currencies are grouped until a verified FX conversion is available.
      if (av.currency !== bv.currency) return av.currency.localeCompare(bv.currency);
      return bv.amount - av.amount;
    }
    if (sort === "name") return a.name.localeCompare(b.name, "zh-CN");
    const value = (stock: T) => {
      if (sort === "price") return stock.quote_available === false || stock.current_price <= 0 ? null : stock.current_price;
      const row = fundamentals[stockKey(stock)];
      if (sort === "pe") return row?.pe != null && row.pe > 0 ? row.pe : null;
      return row?.next_earnings_date;
    };
    if (sort === "original") return 0;
    const av = value(a), bv = value(b);
    if (av == null || av === "") return bv == null || bv === "" ? 0 : 1;
    if (bv == null || bv === "") return -1;
    return typeof av === "number" && typeof bv === "number" ? sort === "price" ? bv - av : av - bv : String(av).localeCompare(String(bv));
  });
  const metricHint = (f: Fundamentals | undefined, key: string) => {
    const metric = f?.metrics?.[key];
    return [metric?.source || f?.source, (key === "eps" || key === "guidance") ? metric?.currency || f?.currency : "", metric?.as_of || f?.as_of].filter(Boolean).join(" · ");
  };
  const subscriptionLimited = stocks.some(stock => fundamentals[stockKey(stock)]?.message?.includes("HTTP 402"));
  const holdingCurrencies = new Set(stocks.map(stock => getHoldingValue?.(stock)?.currency).filter(Boolean));
  return <div className={`surface-card watchlist-table-container ${renderHolding ? "has-holdings" : ""} ${hidden.map(key => `hide-${key}`).join(" ")}`}>
    {subscriptionLimited && <p className="form-note danger" role="status">部分标的的财报接口未包含在当前 FMP 订阅中，相应指标暂不可用。点击标的旁的 ⓘ 可查看具体原因；持仓和行情仍可正常显示。</p>}
    <div className="research-table-controls">
      <span className="research-count">{stocks.length} 项标的</span>
      <div className="research-table-actions" ref={controlsRef} onKeyDown={event => { if (event.key === "Escape") (menu === "sort" ? sortButton : columnsButton).current?.focus(); }}>
        <div className="research-control">
          <button ref={sortButton} type="button" className="research-control-button" aria-label={`排序方式：${sortOptions.find(item => item.value === sort)?.label}`} aria-expanded={isOpen && menu === "sort"} aria-controls={`${controlId}-sort`} onClick={() => toggleMenu("sort")}><ArrowDownUp size={13} /><span>{sortOptions.find(item => item.value === sort)?.label}</span><ChevronDown size={12} /></button>
          {isOpen && menu === "sort" && <div className="research-control-popover research-sort-options" id={`${controlId}-sort`} role="group" aria-label="排序方式">{sortOptions.map(item => <button type="button" key={item.value} aria-pressed={sort === item.value} onClick={() => { setSort(item.value); setIsOpen(false); sortButton.current?.focus(); }}><span>{item.label}</span>{sort === item.value && <Check size={13} />}</button>)}</div>}
        </div>
        <div className="research-control">
          <button ref={columnsButton} type="button" className="research-control-button" aria-label="列设置" aria-expanded={isOpen && menu === "columns"} aria-controls={`${controlId}-columns`} onClick={() => toggleMenu("columns")}><Columns3 size={13} /><span>列设置</span></button>
          {isOpen && menu === "columns" && <div className="research-control-popover research-column-options" id={`${controlId}-columns`} role="group" aria-label="显示财报指标"><p>显示财报指标</p>{Object.entries(columnLabels).map(([key, label]) => <label key={key}><input type="checkbox" checked={!hidden.includes(key)} onChange={() => setHidden(previous => previous.includes(key) ? previous.filter(item => item !== key) : [...previous, key])} />{label}</label>)}<button type="button" className="research-columns-reset" onClick={() => setHidden(["guidance"])}>恢复默认</button></div>}
        </div>
      </div>
      {sort === "market_value" && holdingCurrencies.size > 1 && <span className="watchlist-note research-currency-note">按报价币种分组，组内市值降序；未换算汇率。</span>}
    </div>
    <div className="research-table-scroll">
    <table className="unified-watchlist-table">
      <colgroup><col className="research-identity-col" /><col />{renderHolding && <col className="research-holding-col" />}{renderTarget && <col className="research-target-col" />}{Object.keys(columnLabels).filter(key => !hidden.includes(key)).map(key => <col key={key} />)}{renderAction && <col className="research-action-col" />}</colgroup>
      <thead><tr><th>标的</th><th>最新价格</th>{renderHolding && <th>持仓详情</th>}{renderTarget && <th className="target-price-column">期望价格</th>}<th className="col-eps">EPS <small>TTM</small></th><th className="col-pe">PE <small>TTM</small></th><th className="col-forward">远期 PE</th><th className="col-guidance">公司指引 EPS</th><th className="col-earnings">下份财报时间</th>{renderAction && <th><span className="sr-only">操作</span></th>}</tr></thead>
      <tbody>{sorted.map(stock => {
        const market = markets.find(item => item.id === stock.market); const key = stockKey(stock); const f = fundamentals[key];
        const na = (stock.market === "crypto" || !markets.some(m=>m.id===stock.market)) || f?.status === "not_applicable" || /\bETF\b|基金/i.test(stock.name);
        const empty = na ? "不适用" : "—";
        const guidance = f?.guidance_eps != null ? numeric(f.guidance_eps) : f?.guidance_eps_low != null && f.guidance_eps_high != null ? `${numeric(f.guidance_eps_low)}–${numeric(f.guidance_eps_high)}` : empty;
        const financialHint = [f?.message, metricHint(f, "eps")].filter(Boolean).join(" · ");
        const product = stock.product_type === "quanto_perpetual" ? "Quanto 合约" : stock.product_type === "stock_perpetual" ? "股票永续" : stock.product_type === "equity" ? "股票" : stock.product_type === "crypto_spot" ? "现货资产" : stock.product_type === "crypto_perpetual" ? "加密永续" : stock.product_type === "other_perpetual" ? "永续合约" : "股票代币";
        const rowKey = getRowKey ? getRowKey(stock) : key;
        const code = stock.symbol || stock.id;
        return <Fragment key={rowKey}><tr>
          <td data-label="标的" className="research-identity"><div className="research-name-line"><button className="watchlist-name" title={code} disabled={!market} onClick={() => market && onOpenStock(market, stock)}>{code}</button><button className="source-toggle" aria-label={`${code}详情`} aria-expanded={expanded === rowKey} onClick={() => setExpanded(previous => previous === rowKey ? null : rowKey)}><Info size={10} /></button></div><small title={stock.provider_symbol}>{market?.displayName ?? "其他 / 待识别"}{["bitget","binance","gate"].includes(stock.source || "") ? ` · ${product}` : stock.source === "tencent" ? " · 股票" : ""}</small></td>
          <td data-label="价格" className="mono">{stock.quote_available === false || stock.current_price <= 0 ? "—" : numeric(stock.current_price)}<small>{stock.currency || (stock.market === "a_share" ? "CNY" : stock.market === "hk_stock" ? "HKD" : stock.market === "crypto" ? "USDT" : "USD")}{stock.change_basis === "24h" ? " · 24h" : ""}</small></td>
          {renderHolding && <td data-label="持仓详情" className="holding-detail-cell">{renderHolding(stock)}</td>}{renderTarget && <td data-label="期望价格" className="target-price-column">{renderTarget(stock)}</td>}
          <td data-label="EPS" className="mono col-eps" title={financialHint}>{f?.eps != null ? numeric(f.eps) : empty}{f?.eps != null && <small>{f.metrics?.eps?.currency || f.currency}</small>}</td>
          <td data-label="PE" className="mono col-pe" title={metricHint(f, "pe")}>{f?.pe != null && f.pe > 0 ? numeric(f.pe) : f?.eps != null && f.eps < 0 ? "亏损" : empty}</td>
          <td data-label="远期 PE" className="mono col-forward" title={metricHint(f, "forward_pe")}>{f?.forward_pe != null ? numeric(f.forward_pe) : empty}{f?.forward_period && <small>年度期末 {f.forward_period}</small>}</td>
          <td data-label="公司指引" className="mono col-guidance" title={f?.guidance_report_date ? `财报发布 ${f.guidance_report_date}` : financialHint}>{f?.guidance_source_url && /^https?:\/\//.test(f.guidance_source_url) ? <a href={f.guidance_source_url} target="_blank" rel="noreferrer">{guidance}</a> : guidance}<small>{f?.guidance_period ? `${f.guidance_period} · ${f.guidance_basis || ""} · ${f.metrics?.guidance?.currency || f.currency || ""}` : na ? "" : "未取得有效公司指引"}</small></td>
          <td data-label="财报日期" className="mono col-earnings">{f?.next_earnings_date || empty}{f?.next_earnings_date && <small>{f.earnings_date_status === "confirmed" ? "公司已确认" : "预计日期"}</small>}</td>
          {renderAction && <td data-label="操作">{renderAction(stock)}</td>}
        </tr>{expanded === rowKey && <tr className="research-expanded-row"><td ref={detailsRef} colSpan={7 + (renderHolding ? 1 : 0) + (renderTarget ? 1 : 0) + (renderAction ? 1 : 0) - hidden.length}><div className="research-provenance"><h4 className="research-asset-name">{stock.name}</h4><p className="research-asset-introduction">{assetIntroduction(stock, f?.description)}</p>{!na && <div>{Object.entries({eps:"EPS",pe:"PE",forward_pe:"远期 PE",guidance:"公司指引",next_earnings_date:"财报日期"}).map(([metric,label]) => <span key={metric}><b>{label}</b>{metricHint(f,metric) || "暂无来源数据"}</span>)}</div>}</div></td></tr>}</Fragment>;
      })}</tbody>
    </table>
    </div>
    {!stocks.length && <div className="empty-state">{emptyMessage}</div>}
  </div>;
}
