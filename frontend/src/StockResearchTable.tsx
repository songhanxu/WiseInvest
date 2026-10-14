import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { requestJson } from "./App";
import type { Market, StockQuote } from "./App";

export type Fundamentals = {
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
export const numeric = (value?: number | null) => value == null || !Number.isFinite(value) ? "—" : new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 2 }).format(value);

export function useStockFundamentals(apiBase: string, stocks: StockQuote[], scope: string) {
  const [fundamentals, setFundamentals] = useState<Record<string, Fundamentals>>({});
  const [financialLoading, setFinancialLoading] = useState(false);
  const financialKeys = stocks.map(stockKey).sort().join("|");
  useEffect(() => {
    let cancelled = false;
    setFinancialLoading(Boolean(financialKeys));
    setFundamentals({});
    const rows = [...new Map(stocks.map(stock => [stockKey(stock), stock])).values()];
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
  }, [apiBase, scope, financialKeys]);

  return { fundamentals, financialLoading };
}

export function StockResearchTable<T extends StockQuote>({ stocks, markets, fundamentals, onOpenStock, getRowKey, renderHolding, renderAction, emptyMessage }: {
  stocks: T[];
  markets: Market[];
  fundamentals: Record<string, Fundamentals>;
  onOpenStock: (market: Market, stock: StockQuote) => void;
  getRowKey?: (stock: T) => string;
  renderHolding?: (stock: T) => ReactNode;
  renderAction?: (stock: T) => ReactNode;
  emptyMessage: string;
}) {
  return <div className="surface-card watchlist-table-container">
    <table className="unified-watchlist-table">
      <thead><tr><th>名称</th><th>代码</th><th>市场</th><th>价格</th><th>EPS <small>TTM</small></th><th>PE <small>TTM</small></th><th>远期 PE</th><th>最新财报预期 EPS <small>公司指引</small></th><th>下份财报时间</th>{renderAction && <th><span className="sr-only">操作</span></th>}</tr></thead>
      <tbody>{stocks.map(stock => {
        const market = markets.find(item => item.id === stock.market); const key = stockKey(stock); const f = fundamentals[key];
        const na = stock.market === "crypto" || f?.status === "not_applicable" || /\bETF\b|基金/i.test(stock.name);
        const empty = na ? "不适用" : "—";
        const guidance = f?.guidance_eps != null ? numeric(f.guidance_eps) : f?.guidance_eps_low != null && f.guidance_eps_high != null ? `${numeric(f.guidance_eps_low)}–${numeric(f.guidance_eps_high)}` : empty;
        const financialHint = [f?.message, f?.source, f?.currency ? `EPS 币种：${f.currency}` : "", f?.as_of ? `数据时间：${f.as_of}` : ""].filter(Boolean).join(" · ");
        const product = stock.product_type === "quanto_perpetual" ? "Quanto 合约" : stock.product_type === "stock_perpetual" ? "股票永续" : "股票代币";
        return <tr key={getRowKey ? getRowKey(stock) : key}>
          <td><button className="watchlist-name" disabled={!market} onClick={() => market && onOpenStock(market, stock)}>{stock.name}</button><small>{stock.source === "bitget" ? `Bitget · ${product}` : stock.source === "tencent" ? "腾讯 · 兜底行情" : ""}</small>{renderHolding?.(stock)}</td>
          <td className="mono" title={stock.provider_symbol}>{stock.symbol || stock.id}</td>
          <td><span className="watchlist-market-tag">{market?.displayName ?? (stock.market === "jp_stock" ? "日股" : stock.market)}</span></td>
          <td className="mono">{stock.quote_available === false || stock.current_price <= 0 ? "—" : numeric(stock.current_price)}<small>{stock.currency || (stock.market === "a_share" ? "CNY" : stock.market === "hk_stock" ? "HKD" : stock.market === "crypto" ? "USDT" : "USD")}{stock.change_basis === "24h" ? " · 24h" : ""}</small></td>
          <td className="mono" title={financialHint}>{f?.eps != null ? numeric(f.eps) : empty}{f?.eps != null && <small>{f.currency}</small>}</td>
          <td className="mono" title={financialHint}>{f?.pe != null && f.pe > 0 ? numeric(f.pe) : f?.eps != null && f.eps < 0 ? "亏损" : empty}</td>
          <td className="mono" title={financialHint}>{f?.forward_pe != null ? numeric(f.forward_pe) : empty}{f?.forward_period && <small>年度期末 {f.forward_period}</small>}</td>
          <td className="mono" title={f?.guidance_report_date ? `财报发布 ${f.guidance_report_date}` : financialHint}>{f?.guidance_source_url && /^https?:\/\//.test(f.guidance_source_url) ? <a href={f.guidance_source_url} target="_blank" rel="noreferrer">{guidance}</a> : guidance}<small>{f?.guidance_period ? `${f.guidance_period} · ${f.guidance_basis || ""}` : na ? "" : "未取得公司指引"}</small></td>
          <td className="mono">{f?.next_earnings_date || empty}{f?.next_earnings_date && <small>{f.earnings_date_status === "confirmed" ? "公司已确认" : "预计日期"}</small>}</td>
          {renderAction && <td>{renderAction(stock)}</td>}
        </tr>;
      })}</tbody>
    </table>
    {!stocks.length && <div className="empty-state">{emptyMessage}</div>}
  </div>;
}
