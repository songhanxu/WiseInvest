import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronUp, ExternalLink, Loader2, RefreshCw, Users, X } from "lucide-react";
import { requestJson } from "./App";
import type { AnalysisResponse, KLinePoint, MarketId, NewsItem, StockQuote } from "./App";
import { ChatMarkdown } from "./ChatMarkdown";
import { technicalOverview } from "./research";

const number = (value: number | null) => value == null ? "—" : new Intl.NumberFormat("zh-CN", { maximumFractionDigits: Math.abs(value) < 1 ? 6 : 2 }).format(value);
const stance = (value?: string) => value === "bullish" ? "偏多" : value === "bearish" ? "偏空" : "中性";
const tone = (value?: string) => value === "bullish" || value === "positive" ? "green" : value === "bearish" || value === "negative" ? "red" : "muted";

export function StockDeepResearch({ stock, period, candles, loading, analysis, analysisLoading, error, onReload, onAskAI, onAskRoundtable }: {
  stock: StockQuote; period: string; candles: KLinePoint[]; loading: boolean; analysis: AnalysisResponse | null; analysisLoading: boolean; error: string;
  onReload: () => void; onAskAI: (prompt: string) => void; onAskRoundtable: (prompt?: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const overview = useMemo(() => technicalOverview(candles), [candles]);
  const context = `${stock.name}（${stock.symbol}，${stock.market}）${stock.provider_symbol ? `，交易产品 ${stock.provider_symbol}` : ""}，当前报价 ${stock.current_price} ${stock.currency || ""}`;
  const roundtablePrompt = `请主持圆桌讨论：${context}。请价值派、趋势派、量化派分别分析估值、趋势与风险，指出观点分歧和需要进一步核验的数据。`;
  return <section className="surface-card deep-research">
    <div className="research-section-heading"><div><h3>个股深入分析</h3><p>技术概览与多周期综合研究</p></div><button type="button" className="research-text-button" onClick={onReload} disabled={analysisLoading} aria-label="重新获取个股分析"><RefreshCw size={13} className={analysisLoading ? "spin" : ""} />重新获取</button></div>
    <div className="technical-overview" aria-label="技术概览">
      <div><span>均线位置 · {period}</span><strong>{loading ? "加载中" : overview.trend}</strong><small>MA5 {number(overview.ma5)} · MA20 {number(overview.ma20)}</small></div>
      <div><span>价格与趋势</span><strong>{loading ? "—" : overview.last != null && overview.ma20 != null ? `${number((overview.last / overview.ma20 - 1) * 100)}%` : "—"}</strong><small>当前周期收盘价相对 MA20</small></div>
      <div><span>成交量</span><strong>{loading || overview.volumeRatio == null ? "—" : `${number(overview.volumeRatio)}×`}</strong><small>最新一根 / 前 20 根平均量</small></div>
    </div>
    <p className="research-data-caption">基于当前{period} K 线计算，最新一根可能尚未收盘。{!loading && overview.ma20 == null ? "至少需要 20 根有效 K 线计算趋势。" : ""}</p>
    <div className="deep-analysis-content">
      {error && <p className="form-note danger" role="alert">{error}{analysis?.detail ? "，保留上次分析。" : ""}</p>}
      {analysisLoading && !analysis?.detail ? <div className="research-loading" role="status"><Loader2 size={16} className="spin" /><span>正在获取多周期综合分析…</span></div> : analysis?.detail ? <>
        <div className="analysis-summary"><span className={`badge ${tone(analysis.conclusion)}`}>{stance(analysis.conclusion)}</span><h4>{analysis.summary || "综合分析已就绪"}</h4></div>
        <button type="button" className="research-text-button analysis-expand" aria-expanded={expanded} aria-controls="stock-deep-report" onClick={() => setExpanded(value => !value)}>{expanded ? "收起深入分析" : "展开深入分析"}{expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}</button>
        {expanded && <article id="stock-deep-report" className="deep-analysis-report"><ChatMarkdown content={analysis.detail} /></article>}
      </> : <p className="research-empty" role="status">{analysis?.summary || "暂未取得有效分析，稍后可重新获取。"}</p>}
    </div>
    <div className="deep-research-actions"><button type="button" className="research-text-button" onClick={() => onAskAI(`结合最新可用数据，对${context}继续深入分析，重点解释支撑阻力、不同周期的判断及风险；缺失数据请明确说明。`)}>继续追问</button><button type="button" className="research-text-button" onClick={() => onAskRoundtable(roundtablePrompt)}><Users size={14} />交给圆桌讨论</button></div>
  </section>;
}

function articleURL(value: string) {
  try { const url = new URL(value); return ["http:", "https:"].includes(url.protocol) ? url.href : undefined; } catch { return undefined; }
}

export function StockNews({ apiBase, stock, market, news, loading, error, onReload, onAskAI }: {
  apiBase: string; stock: StockQuote; market: MarketId; news: NewsItem[]; loading: boolean; error?: string; onReload?: () => void; onAskAI: (prompt: string) => void;
}) {
  const [selected, setSelected] = useState<NewsItem | null>(null);
  return <section className="surface-card stock-news">
    <div className="research-section-heading"><div><h3>相关资讯</h3><p>点击查看事件摘要与 AI 深入解读</p></div>{onReload && <button type="button" className="research-text-button" disabled={loading} onClick={onReload} aria-label="更新个股资讯"><RefreshCw size={13} className={loading ? "spin" : ""} />更新资讯</button>}</div>
    {error && <p className="form-note danger" role="alert">{error}{news.length ? "，保留上次资讯。" : ""}</p>}
    {loading && !news.length ? <div className="research-loading" role="status"><Loader2 size={16} className="spin" />正在获取资讯…</div> : news.length ? <div className="news-list">{news.slice(0, 12).map(item => <button type="button" key={item.id} className="news-card news-detail-button" onClick={() => setSelected(item)}><div><span className={`badge ${tone(item.sentiment)}`}>{item.sentiment === "positive" ? "利好" : item.sentiment === "negative" ? "利空" : "中性"}</span><span>{item.source || "资讯"}</span></div><h3>{item.title}</h3><p>{item.summary || "查看事件详情与解读"}</p><small>{item.time}</small></button>)}</div> : <div className="empty-state">暂无相关资讯</div>}
    {selected && <NewsDetail key={selected.id} apiBase={apiBase} stock={stock} market={market} news={selected} onClose={() => setSelected(null)} onAskAI={onAskAI} />}
  </section>;
}

function NewsDetail({ apiBase, stock, market, news, onClose, onAskAI }: { apiBase: string; stock: StockQuote; market: MarketId; news: NewsItem; onClose: () => void; onAskAI: (prompt: string) => void }) {
  const modal = useRef<HTMLDialogElement>(null);
  const [enhanced, setEnhanced] = useState<{ summary?: string; analysis?: string; sentiment?: string }>(news);
  const [busy, setBusy] = useState(!news.analysis);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const element = modal.current;
    element?.showModal();
    return () => element?.close();
  }, []);
  useEffect(() => {
    if (news.analysis && revision === 0) return;
    const controller = new AbortController();
    setBusy(true); setError("");
    const query = new URLSearchParams({ news_id: news.id, code: stock.id, market, name: stock.name, title: news.title, summary: news.summary || "", source: news.source || "" });
    requestJson<{ summary?: string; analysis?: string; sentiment?: string }>(apiBase, `/api/v1/stocks/news/enhance?${query}`, "", { signal: controller.signal })
      .then(result => { if (!controller.signal.aborted) { setEnhanced({ ...news, ...result }); if (!result.analysis?.trim()) setError("后端暂未返回有效解读，可以稍后重试。"); } })
      .catch(err => { if (!controller.signal.aborted) setError(err instanceof Error ? err.message : "资讯解读暂不可用"); })
      .finally(() => { if (!controller.signal.aborted) setBusy(false); });
    return () => controller.abort();
  }, [apiBase, stock.id, stock.name, market, news.id, revision]);
  const url = articleURL(news.url);
  return <dialog ref={modal} className="research-news-dialog" aria-labelledby="news-detail-title" onCancel={onClose} onClick={event => { if (event.target === event.currentTarget) onClose(); }}>
    <div className="research-news-modal"><header><span>资讯解读 · {stock.symbol}</span><button type="button" className="research-icon-button" aria-label="关闭资讯详情" onClick={onClose}><X size={18} /></button></header>
      <h2 id="news-detail-title">{news.title}</h2><div className="research-news-meta"><span>{news.source || "来源未注明"}</span><time>{news.time}</time>{url && <a href={url} target="_blank" rel="noopener noreferrer">查看原文<ExternalLink size={12} /></a>}</div>
      <section><h3>事件摘要</h3><ChatMarkdown content={enhanced.summary || news.summary || "暂无摘要"} /></section>
      <section><div className="research-section-heading"><h3>AI 深入解读</h3><button type="button" className="research-text-button" disabled={busy} onClick={() => setRevision(value => value + 1)}><RefreshCw size={12} className={busy ? "spin" : ""} />重新获取</button></div>{busy ? <div className="research-loading" role="status"><Loader2 size={15} className="spin" />正在分析事件背景、影响与风险…</div> : enhanced.analysis ? <ChatMarkdown content={enhanced.analysis} /> : <p className="research-empty">暂未取得有效解读。</p>}{error && <p className="form-note danger" role="alert">{error}</p>}</section>
      <footer><button type="button" className="research-text-button" onClick={() => { onClose(); onAskAI(`请分析这则事件对${stock.name}（${stock.symbol}）的影响：${news.title}\n已有摘要：${enhanced.summary || news.summary || "暂无"}\n请区分已知事实、推测和待核验信息。`); }}>就此事件继续追问</button></footer>
    </div>
  </dialog>;
}
