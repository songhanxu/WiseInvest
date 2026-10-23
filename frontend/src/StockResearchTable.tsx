import { Fragment, useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { requestJson } from "./App";
import type { Market, StockQuote } from "./App";
import { ArrowDownUp, Check, ChevronDown, Columns3, Info, Loader2, Sparkles } from "lucide-react";
import { useDismissiblePopover } from "./useDismissiblePopover";
import { assetIntroduction } from "./assetIntroductions";
import { quoteCurrency, quoteStatusLabel, quoteTimeLabel } from "./quote-presentation";

export type Fundamentals = {
  description?: string;
  metrics?: Record<string, { source: string; source_url?: string; currency?: string; as_of: string }>;
  eps: number | null;
  eps_basis?: string;
  eps_period_start?: string;
  eps_period_end?: string;
  eps_filed_date?: string;
  trailing_eps?: number;
  trailing_eps_basis?: string;
  trailing_eps_period_start?: string;
  trailing_eps_period_end?: string;
  pe_price?: number;
  pe_price_time?: string;
  sec_cik?: string;
  financial_reports?: {
    form: string; filed_date: string; report_date: string; source_url: string; accession: string;
    metrics?: Record<string, { value: number; unit: string; period_start?: string; period_end: string; tag: string }>;
  }[];
  pe: number | null;
  forward_pe: number | null;
  forward_eps: number | null;
  forward_period?: string;
  forward_basis?: string;
  guidance_eps: number | null;
  guidance_eps_low: number | null;
  guidance_eps_high: number | null;
  guidance_period?: string;
  guidance_basis?: string;
  guidance_report_date?: string;
  guidance_source_url?: string;
  next_earnings_date?: string;
  next_earnings_date_end?: string;
  earnings_date_status?: string;
  currency?: string;
  source?: string;
  as_of?: string;
  status: "available" | "unavailable" | "not_applicable";
  message?: string;
};
export type SECReport = NonNullable<Fundamentals["financial_reports"]>[number];

export const stockKey = (stock: StockQuote) => `${stock.market}:${stock.id}`;
export const numeric = (value?: number | null) => value == null || !Number.isFinite(value) ? "—" : new Intl.NumberFormat("zh-CN", { maximumFractionDigits: value !== 0 && Math.abs(value) < 1 ? 12 : 2 }).format(value);

export function useStockFundamentals(apiBase: string, stocks: StockQuote[], scope: string) {
  const [fundamentals, setFundamentals] = useState<Record<string, Fundamentals>>({});
  const [financialLoading, setFinancialLoading] = useState(false);
  const [refreshVersion, setRefreshVersion] = useState(0);
  const refreshFundamentals = useCallback(() => setRefreshVersion(version => version + 1), []);
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
  }, [apiBase, scope, financialKeys, refreshVersion]);

  return { fundamentals, financialLoading, refreshFundamentals };
}

export function StockResearchTable<T extends StockQuote>({ stocks, markets, fundamentals, onOpenStock, onOpenReport, onOpenInsight, insightStatus, getRowKey, getHoldingValue, renderHolding, renderTarget, renderAction, emptyMessage, loading = false }: {
  stocks: T[];
  markets: Market[];
  fundamentals: Record<string, Fundamentals>;
  onOpenStock: (market: Market, stock: StockQuote) => void;
  onOpenReport?: (stock: T) => void;
  onOpenInsight?: (stock: T) => void;
  insightStatus?: Record<string, string>;
  getRowKey?: (stock: T) => string;
  getHoldingValue?: (stock: T) => { amount: number; currency: string } | null;
  renderHolding?: (stock: T) => ReactNode;
  renderTarget?: (stock: T) => ReactNode;
  renderAction?: (stock: T) => ReactNode;
  emptyMessage: string;
  loading?: boolean;
}) {
  const initialLoading = loading && stocks.length === 0;
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
  const columnLabels: Record<string, string> = { eps: "EPS", ...(onOpenReport ? { report: "当前最新财报" } : {}), pe: "PE", forward: "远期 PE", guidance: "公司指引", earnings: "下份财报时间" };
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
  const holdingCurrencies = new Set(stocks.map(stock => getHoldingValue?.(stock)?.currency).filter(Boolean));
  return <div className={`surface-card watchlist-table-container ${renderHolding ? "has-holdings" : ""} ${onOpenReport ? "has-report" : ""} ${hidden.map(key => `hide-${key}`).join(" ")}`}>
    <div className="research-table-controls">
      <span className="research-count">{initialLoading ? <span className="skeleton-block research-count-skeleton" aria-hidden="true" /> : `${stocks.length} 项标的`}</span>
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
    {initialLoading && <span className="sr-only" role="status">正在加载自选列表</span>}
    <div className="research-table-scroll" aria-busy={initialLoading}>
    <table className="unified-watchlist-table">
      <colgroup><col className="research-identity-col" /><col />{renderHolding && <col className="research-holding-col" />}{renderTarget && <col className="research-target-col" />}{Object.keys(columnLabels).filter(key => !hidden.includes(key)).map(key => <col className={key === "report" ? "research-report-col" : undefined} key={key} />)}{renderAction && <col className="research-action-col" />}</colgroup>
      <thead><tr><th>标的</th><th>最新价格</th>{renderHolding && <th>持仓详情</th>}{renderTarget && <th className="target-price-column">期望价格</th>}<th className="col-eps">EPS <small>披露口径</small></th>{onOpenReport && <th className="col-report">当前最新财报</th>}<th className="col-pe">PE <small>TTM</small></th><th className="col-forward">远期 PE</th><th className="col-guidance">公司指引 EPS</th><th className="col-earnings">下份财报时间</th>{renderAction && <th><span className="sr-only">操作</span></th>}</tr></thead>
      <tbody>{initialLoading && Array.from({ length: 6 }, (_, index) => <tr className="research-skeleton-row" key={`loading-${index}`} aria-hidden="true">
        <td data-label="标的" className="research-identity"><div className="research-skeleton-line"><span className="skeleton-block skeleton-symbol" /></div><small><span className="skeleton-block skeleton-currency" /></small></td>
        <td data-label="价格"><div className="research-skeleton-line"><span className="skeleton-block skeleton-value" /></div><small><span className="skeleton-block skeleton-currency" /></small></td>
        {renderHolding && <td data-label="持仓详情" className="holding-detail-cell"><span className="skeleton-block skeleton-value" /></td>}
        {renderTarget && <td data-label="期望价格" className="target-price-column"><span className="skeleton-block skeleton-target" /></td>}
        {Object.entries(columnLabels).map(([key, label]) => <td className={`col-${key}`} data-label={label} key={key}><span className={`skeleton-block ${key === "earnings" ? "skeleton-date" : "skeleton-metric"}`} /></td>)}
        {renderAction && <td data-label="操作"><span className="skeleton-block skeleton-action" /></td>}
      </tr>)}{sorted.map(stock => {
        const market = markets.find(item => item.id === stock.market); const key = stockKey(stock); const f = fundamentals[key];
        const na = (stock.market === "crypto" || !markets.some(m=>m.id===stock.market)) || f?.status === "not_applicable" || /\bETF\b|基金/i.test(stock.name);
        const empty = na ? "不适用" : "—";
        const guidance = f?.guidance_eps != null ? numeric(f.guidance_eps) : f?.guidance_eps_low != null && f.guidance_eps_high != null ? `${numeric(f.guidance_eps_low)}–${numeric(f.guidance_eps_high)}` : empty;
        const financialHint = [f?.message, metricHint(f, "eps")].filter(Boolean).join(" · ");
        const latestReport = f?.financial_reports?.[0];
        const rowKey = getRowKey ? getRowKey(stock) : key;
        const code = stock.symbol || stock.id;
        return <Fragment key={rowKey}><tr>
          <td data-label="标的" className="research-identity"><div className="research-name-line"><button className="watchlist-name" title={code} disabled={!market} onClick={() => market && onOpenStock(market, stock)}>{code}</button><button className="source-toggle" aria-label={`${code}详情`} aria-expanded={expanded === rowKey} onClick={() => setExpanded(previous => previous === rowKey ? null : rowKey)}><Info size={10} /></button></div><small title={stock.provider_symbol}>{market?.displayName ?? "其他 / 待识别"}</small></td>
          <td data-label="价格" className="mono"><span className="quote-amount">{stock.quote_available === false || stock.current_price <= 0 ? "—" : numeric(stock.current_price)}<span className="quote-currency">{quoteCurrency(stock)}</span></span><small className="quote-source-label" title={quoteTimeLabel(stock)}>{quoteStatusLabel(stock)}</small></td>
          {renderHolding && <td data-label="持仓详情" className="holding-detail-cell">{renderHolding(stock)}</td>}{renderTarget && <td data-label="期望价格" className="target-price-column">{renderTarget(stock)}</td>}
          <td data-label="EPS" className="mono col-eps" title={[financialHint, f?.eps_period_start && `${f.eps_period_start} 至 ${f.eps_period_end}`, f?.eps_filed_date && `提交于 ${f.eps_filed_date}`].filter(Boolean).join(" · ")}>{f?.eps != null ? numeric(f.eps) : empty}{f?.eps != null && <><small>{[f.metrics?.eps?.currency || f.currency, f.eps_basis].filter(Boolean).join(" · ")}</small>{f.eps_period_end && <small>截至 {f.eps_period_end}</small>}</>}</td>
          {onOpenReport && <td data-label="当前最新财报" className="col-report">{latestReport ? <div className="filing-cell"><div className="filing-cell-main"><button type="button" className="filing-period-link" aria-label={`${code} ${latestReport.form} 财报，报告期末 ${latestReport.report_date}，查看详情`} onClick={() => onOpenReport(stock)}><time dateTime={latestReport.report_date}>{latestReport.report_date}</time></button>{onOpenInsight && <button type="button" className="report-ai-button" aria-label={`${code} 财报 AI 解读`} title="查看 AI 财报解读" onClick={() => onOpenInsight(stock)}>{insightStatus?.[stock.id.toUpperCase()] === "pending" ? <Loader2 size={14} className="spin" /> : <Sparkles size={14} />}</button>}</div><div className="filing-cell-meta"><span className="filing-form">{latestReport.form}</span><span className="filing-filed" title={`SEC 提交于 ${latestReport.filed_date}`}>提交 <time dateTime={latestReport.filed_date}>{latestReport.filed_date.slice(0, 4) === latestReport.report_date.slice(0, 4) ? latestReport.filed_date.slice(5) : latestReport.filed_date}</time></span></div></div> : empty}</td>}
          <td data-label="PE" className="mono col-pe" title={[metricHint(f, "pe"), f?.pe_price != null && f.trailing_eps != null && `${numeric(f.pe_price)} USD ÷ ${numeric(f.trailing_eps)} USD/股`, f?.trailing_eps_basis].filter(Boolean).join(" · ")}>{f?.pe != null && f.pe > 0 ? numeric(f.pe) : f?.trailing_eps != null && f.trailing_eps <= 0 ? "亏损" : empty}{f?.pe_price != null && f.trailing_eps != null && <small>现货 {numeric(f.pe_price)} ÷ TTM {numeric(f.trailing_eps)}</small>}{f?.pe == null && f?.trailing_eps != null && f.trailing_eps > 0 && <small>缺有效美元现货报价</small>}</td>
          <td data-label="远期 PE" className="mono col-forward" title={metricHint(f, "forward_pe")}>{f?.forward_pe != null ? numeric(f.forward_pe) : empty}{f?.forward_period ? <small>年度期末 {f.forward_period}</small> : f?.forward_basis && <small>{f.forward_basis}</small>}</td>
          <td data-label="公司指引" className="mono col-guidance" title={f?.guidance_report_date ? `财报发布 ${f.guidance_report_date}` : financialHint}>{f?.guidance_source_url && /^https?:\/\//.test(f.guidance_source_url) ? <a href={f.guidance_source_url} target="_blank" rel="noreferrer">{guidance}</a> : guidance}<small>{f?.guidance_period ? `${f.guidance_period} · ${f.guidance_basis || ""} · ${f.metrics?.guidance?.currency || f.currency || ""}` : na ? "" : "未取得有效公司指引"}</small></td>
          <td data-label="下份财报时间" className="mono col-earnings" title={metricHint(f, "next_earnings_date")}>{f?.next_earnings_date || empty}{f?.next_earnings_date_end && ` 至 ${f.next_earnings_date_end}`}{f?.next_earnings_date && <small>{f.earnings_date_status === "confirmed" ? "公司已确认" : "预计日期"}</small>}</td>
          {renderAction && <td data-label="操作">{renderAction(stock)}</td>}
        </tr>{expanded === rowKey && <tr className="research-expanded-row"><td ref={detailsRef} colSpan={2 + (renderHolding ? 1 : 0) + (renderTarget ? 1 : 0) + Object.keys(columnLabels).filter(column => !hidden.includes(column)).length + (renderAction ? 1 : 0)}><div className="research-provenance"><h4 className="research-asset-name">{stock.name}</h4><p className="research-asset-introduction">{assetIntroduction(stock, f?.description)}</p>{!na && f?.message && <p className="watchlist-note" role="status">{f.message}</p>}{!na && <div>{Object.entries({eps:"EPS",pe:"PE",forward_pe:"远期 PE",guidance:"公司指引",next_earnings_date:"财报日期"}).map(([metric,label]) => <span key={metric}><b>{label}</b>{metricHint(f,metric) || "暂无来源数据"}</span>)}</div>}{!na && f?.financial_reports?.length ? <SECFinancialReports reports={f.financial_reports} /> : null}</div></td></tr>}</Fragment>;
      })}</tbody>
    </table>
    </div>
    {!stocks.length && !initialLoading && <div className="empty-state">{emptyMessage}</div>}
  </div>;
}

function SECFinancialReports({ reports }: { reports: NonNullable<Fundamentals["financial_reports"]> }) {
  const labels: Record<string, string> = { revenue: "营收", net_income: "净利润", assets: "总资产", liabilities: "总负债", equity: "股东权益", operating_cash_flow: "经营现金流" };
  return <section className="sec-financial-reports" aria-label="SEC 财报">
    <h5>SEC 财报与财务摘要</h5>
    <p className="watchlist-note">这里展示六项财务摘要；点击原始财报可查看完整正文。数值按各科目的披露期间列示，现金流可能为年初至今累计值。</p>
    {reports.map((report, index) => <details key={report.accession} open={index === 0}>
      <summary>{report.form} · 报告期末 {report.report_date} · 提交 {report.filed_date}</summary>
      <a href={report.source_url} target="_blank" rel="noreferrer">查看 SEC 原始财报</a>
      <dl>{Object.entries(report.metrics || {}).map(([key, metric]) => <div key={key}>
        <dt>{labels[key] || key}</dt><dd title={metric.tag}><strong>{numeric(metric.value)} {metric.unit}</strong><small>{metric.period_start ? `${metric.period_start} 至 ${metric.period_end}` : `截至 ${metric.period_end}`}</small></dd>
      </div>)}</dl>
    </details>)}
  </section>;
}
