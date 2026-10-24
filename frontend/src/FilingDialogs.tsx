import { useEffect } from "react";
import { ExternalLink, Loader2, RefreshCw, X } from "lucide-react";
import type { StockQuote } from "./App";
import type { Fundamentals, SECReport } from "./StockResearchTable";
import { numeric } from "./StockResearchTable";
import { ChatMarkdown } from "./ChatMarkdown";

export type FilingInsight = {
  code: string;
  day: string;
  status: "pending" | "ready" | "failed" | "unavailable";
  accession?: string;
  report_date?: string;
  filed_date?: string;
  generated_at?: string;
  content?: string;
  message?: string;
  sources?: { label: string; url: string; date?: string }[];
};

const labels: Record<string, string> = {
  revenue: "营业收入", net_income: "净利润", assets: "总资产", liabilities: "总负债",
  equity: "股东权益", operating_cash_flow: "经营现金流",
};

function DialogShell({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [onClose]);
  return <div className="filing-dialog-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) onClose(); }}>
    <section className="filing-dialog" role="dialog" aria-modal="true" aria-label={title}>
      <div className="filing-dialog-header"><h2>{title}</h2><button type="button" aria-label="关闭" onClick={onClose}><X size={18} /></button></div>
      <div className="filing-dialog-body">{children}</div>
    </section>
  </div>;
}

export function ReportDetailDialog({ stock, fundamentals, onClose }: { stock: StockQuote; fundamentals: Fundamentals; onClose: () => void }) {
  const reports = fundamentals.financial_reports || [];
  const report = reports[0];
  if (!report) return null;
  return <DialogShell title={`${stock.name} · 当前最新财报`} onClose={onClose}>
    <div className="filing-dialog-meta"><span>{report.form}</span><span>报告期末 {report.report_date}</span><span>提交 SEC {report.filed_date}</span></div>
    <p className="watchlist-note">以下数值来自该份 SEC 申报的标准化财务科目。每项保留原始计量单位和披露期间；现金流等项目可能是年初至今累计值。</p>
    <div className="filing-metric-grid">{Object.entries(report.metrics || {}).map(([key, metric]) => <div className="filing-metric" key={key}>
      <span>{labels[key] || key}</span><strong>{numeric(metric.value)} {metric.unit}</strong>
      <small>{metric.period_start ? `${metric.period_start} 至 ${metric.period_end}` : `截至 ${metric.period_end}`}</small>
    </div>)}</div>
    {!Object.keys(report.metrics || {}).length && <p className="watchlist-note">本份申报没有可匹配的标准科目摘要，请查看 SEC 原始文件。</p>}
    <a className="filing-original-link" href={report.source_url} target="_blank" rel="noopener noreferrer">查看完整 SEC 财报正文 <ExternalLink size={14} /></a>
    {reports.length > 1 && <section className="filing-history"><h3>近期申报</h3>{reports.slice(1).map((item: SECReport) => <a key={item.accession} href={item.source_url} target="_blank" rel="noopener noreferrer">{item.form} · 期末 {item.report_date} · 提交 {item.filed_date}<ExternalLink size={12} /></a>)}</section>}
  </DialogShell>;
}

export function FilingInsightDialog({ stock, report, insight, busy, onRetry, onClose }: {
  stock: StockQuote; report: SECReport; insight?: FilingInsight; busy: boolean; onRetry: () => void; onClose: () => void;
}) {
  const current = insight?.status === "ready" && insight.accession === report.accession;
  return <DialogShell title={`${stock.name} · AI 财报解读`} onClose={onClose}>
    <div className="filing-dialog-meta"><span>{report.form}</span><span>报告期末 {report.report_date}</span><span>提交 {report.filed_date}</span></div>
    {current ? <>
      <p className="watchlist-note">解读生成于 {insight.generated_at ? new Date(insight.generated_at).toLocaleString("zh-CN") : insight.day}。股价情景与买卖倾向依据生成时可取得的数据，之后的新公告和行情可能改变结论。</p>
      <ChatMarkdown content={insight.content || ""} disableExternalContent />
      {!!insight.sources?.length && <div className="filing-insight-sources"><h3>核对来源</h3>{insight.sources.map((source, index) => <a key={`${source.url}-${index}`} href={source.url} target="_blank" rel="noopener noreferrer">{source.label}{source.date ? ` · ${source.date}` : ""}<ExternalLink size={12} /></a>)}</div>}
    </> : <div className="filing-insight-state" role="status">
      {insight?.status === "pending" || busy ? <><Loader2 className="spin" size={22} /><p>正在核对财报、历史股价并生成解读…</p></> : <><p>{insight?.status === "failed" ? insight.message || "解读生成失败" : insight?.status === "ready" ? "这份解读对应旧财报，请核对并更新。" : "今天的解读尚未生成。"}</p><button type="button" className="ghost-button" onClick={onRetry}><RefreshCw size={14} />{insight?.status === "failed" || insight?.status === "ready" ? "重新生成" : "开始生成"}</button></>}
    </div>}
  </DialogShell>;
}
