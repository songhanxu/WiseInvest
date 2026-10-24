import { useCallback, useEffect, useRef, useState } from "react";
import { Building2, KeyRound, RefreshCw, ShieldCheck, WalletCards } from "lucide-react";
import { requestJson } from "./App";
import type { Market, MarketId, StockQuote } from "./App";
import { PriceAlertSettings, TargetPriceCell, usePriceAlerts } from "./PriceAlerts";
import { StockResearchTable, stockKey, useStockFundamentals } from "./StockResearchTable";
import { FilingInsightDialog, ReportDetailDialog } from "./FilingDialogs";
import type { FilingInsight } from "./FilingDialogs";

import { accountValuation, holdingNumber, holdingQuantity, loadExchangeRates } from "./account-valuation";
import type { AccountEquity, ExchangeRates } from "./account-valuation";

type Account = { id: string; provider: string; name: string; account_type: string; status: "ready" | "not_configured" | "unbound"; message?: string };
type Holding = StockQuote & { holding_id: string; account_id: string; account_name: string; quantity_unit?: string; stale?: boolean; holding_type: "spot" | "futures" | "balance"; side: string; quantity: string; margin_mode?: string; entry_price?: string; unrealized_pnl?: string; margin_currency?: string; market_value?: string };
type Snapshot = { account_id: string; holdings: Holding[]; account_equities?: AccountEquity[]; as_of: string; complete: boolean; stale?: boolean; last_success?: string; warnings: string[]; accounts?: { id: string; name: string; status: string; message?: string; as_of?: string }[] };
const initialAccounts: Account[] = [{id:"bitget",name:"Bitget"},{id:"binance",name:"币安"},{id:"gate",name:"Gate"}].map(item => ({...item,provider:item.id,account_type:"",status:"not_configured"}));

export function AccountsPanel({ apiBase, token, markets, isActive = true, onOpenStock, onNeedToken }: {
  apiBase: string; token: string; markets: Market[]; isActive?: boolean;
  onOpenStock: (market: Market, stock: StockQuote) => void;
  onNeedToken: () => void;
}) {
  const priceAlerts = usePriceAlerts(apiBase, token, isActive);
  const activeRef = useRef(isActive); activeRef.current = isActive;
  const [fx, setFx] = useState<ExchangeRates | null>(null);
  useEffect(() => {
    if (!isActive) return;
    let cancelled = false;
    void loadExchangeRates().then(rates => { if (!cancelled) setFx(rates); }).catch(() => {});
    return () => { cancelled = true; };
  }, [isActive]);
  const [accounts, setAccounts] = useState<Account[]>(initialAccounts);
  const [accountId, setAccountId] = useState("all");
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [accountLoading, setAccountLoading] = useState(false);
  const [loading, setLoading] = useState(false);
  const [accountError, setAccountError] = useState("");
  const [holdingsError, setHoldingsError] = useState("");
  const [showConnectionHelp, setShowConnectionHelp] = useState(false);
  const [insights, setInsights] = useState<Record<string, FilingInsight>>({});
  const [insightError, setInsightError] = useState("");
  const [insightBusy, setInsightBusy] = useState("");
  const [filingView, setFilingView] = useState<{ code: string; type: "report" | "insight" } | null>(null);
  const lastInsightRefresh = useRef("");
  const insightScopeRef = useRef("");
  const [filter, setFilter] = useState<MarketId | "all" | "other">("all");
  const version = useRef(0);
  const listVersion = useRef(0);
  const account: Account | undefined = accountId === "all" ? { id:"all", provider:"all", name:"全部账户", account_type:"all", status:accounts.some(item=>item.status==="ready") ? "ready" : "not_configured" } : accounts.find(item => item.id === accountId);
  const stocks = snapshot?.holdings ?? [];
  const { fundamentals, financialLoading, refreshFundamentals } = useStockFundamentals(apiBase, stocks, `${token}:${accountId}`);
  const insightCodes = [...new Set(stocks.filter(stock => stock.market === "us_stock").map(stock => stock.id.toUpperCase()))].sort().join(",");
  const insightScope = `${apiBase}|${token}|${accountId}|${insightCodes}`;
  insightScopeRef.current = insightScope;
  const readInsights = useCallback(async () => {
    if (!token || !insightCodes || insightScopeRef.current !== insightScope) return;
    const response = await requestJson<{ insights: Record<string, FilingInsight> }>(apiBase, `/api/v1/stocks/watchlist/filing-insights?codes=${encodeURIComponent(insightCodes)}`, token);
    if (insightScopeRef.current === insightScope) setInsights(response.insights || {});
  }, [apiBase, token, insightCodes, insightScope]);
  const refreshInsight = useCallback(async (code: string, retry = false, accession = "") => {
    if (!token) return;
    const scope = insightScope;
    setInsightBusy(code); setInsightError("");
    try {
      await requestJson(apiBase, "/api/v1/stocks/watchlist/filing-insights/refresh", token, { method: "POST", body: JSON.stringify({ code, retry, accession }) });
      await readInsights();
    } catch (err) { if (insightScopeRef.current === scope) setInsightError(err instanceof Error ? err.message : "AI 解读暂不可用"); }
    finally { if (insightScopeRef.current === scope) setInsightBusy(""); }
  }, [apiBase, token, readInsights, insightScope]);
  useEffect(() => {
    setInsights({}); setInsightError(""); setFilingView(null); lastInsightRefresh.current = "";
  }, [apiBase, token, accountId]);
  useEffect(() => {
    if (!isActive || !token || !insightCodes) return;
    const scope = insightScope;
    void readInsights().catch(err => { if (insightScopeRef.current === scope) setInsightError(err instanceof Error ? err.message : "AI 解读暂不可用"); });
    const day = new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date());
    const key = `${apiBase}|${token}|${accountId}|${day}|${insightCodes}`;
    if (lastInsightRefresh.current === key) return;
    lastInsightRefresh.current = key;
    void requestJson(apiBase, "/api/v1/stocks/watchlist/filing-insights/refresh", token, { method: "POST", body: JSON.stringify({ codes: insightCodes.split(",") }) })
      .then(() => readInsights())
      .catch(err => { if (lastInsightRefresh.current === key) { lastInsightRefresh.current = ""; setInsightError(err instanceof Error ? err.message : "AI 解读暂不可用"); } });
  }, [apiBase, token, accountId, isActive, insightCodes, insightScope, readInsights]);
  useEffect(() => {
    if (!isActive || !token || !Object.values(insights).some(row => row.status === "pending")) return;
    const timer = window.setInterval(() => { void readInsights().catch(() => {}); }, 5000);
    return () => window.clearInterval(timer);
  }, [isActive, token, insights, readInsights]);

  const refreshAccounts = useCallback(async () => {
    const request = ++listVersion.current;
    if (!token) { setAccounts(initialAccounts); setAccountError(""); setAccountLoading(false); return; }
    setAccountLoading(true); setAccountError("");
    try {
      const data = await requestJson<Account[]>(apiBase, "/api/v1/accounts", token);
      if (!Array.isArray(data)) throw new Error("账户列表格式不正确");
      if (request !== listVersion.current) return;
      setAccounts(data);
      setAccountId(previous => previous === "all" || data.some(item => item.id === previous) ? previous : "all");
    } catch (err) {
      if (request === listVersion.current) setAccountError(err instanceof Error ? err.message : "账户列表暂不可用");
    } finally { if (request === listVersion.current) setAccountLoading(false); }
  }, [apiBase, token]);

  useEffect(() => {
    setAccounts(initialAccounts); setAccountId("all"); setSnapshot(null); setFilter("all");
    return () => { listVersion.current++; version.current++; };
  }, [refreshAccounts]);
  useEffect(() => { if (isActive) void refreshAccounts(); }, [isActive, refreshAccounts]);

  const refreshHoldings = useCallback(async () => {
    const request = ++version.current;
    if (!token || !accountId || account?.status !== "ready") { setSnapshot(null); setLoading(false); return; }
    setLoading(true); setHoldingsError("");
    try {
      const data = await requestJson<Snapshot>(apiBase, `/api/v1/accounts/${encodeURIComponent(accountId)}/holdings`, token);
      if (!Array.isArray(data.holdings) || data.account_id !== accountId) throw new Error("持仓数据格式不正确");
      if (request === version.current) setSnapshot(data);
    } catch (err) {
      if (request === version.current) setHoldingsError(err instanceof Error ? err.message : "持仓读取失败");
    } finally { if (request === version.current) setLoading(false); }
  }, [apiBase, token, accountId, account?.status]);

  useEffect(() => {
    setSnapshot(null); setHoldingsError(""); setFilter("all");
    const timer = window.setInterval(() => { if (activeRef.current && !document.hidden) void refreshHoldings(); }, 60000);
    return () => { window.clearInterval(timer); version.current++; };
  }, [refreshHoldings]);
  useEffect(() => { if (isActive) void refreshHoldings(); }, [isActive, refreshHoldings]);

  async function refresh() { void loadExchangeRates().then(setFx).catch(() => {}); await refreshAccounts(); await Promise.all([refreshHoldings(), priceAlerts.refresh()]); }
  const valuation = snapshot ? accountValuation(stocks, fx, snapshot.account_equities, snapshot.complete) : null;
  const partialTotal = Boolean(holdingsError || snapshot?.complete === false || valuation?.missing);
  const visible = stocks.filter(stock => filter === "all" || stock.market === filter);
  const emptyMessage = loading ? "正在读取实际持仓…" : !token ? "登录并连接账户后展示实际持仓。" : accountError ? "账户列表暂不可用。" : account?.status !== "ready" ? "账户尚未连接，连接后将在这里展示实际持仓。" : holdingsError || (snapshot && !snapshot.complete) ? "持仓数据尚未完整取得，请稍后刷新。" : snapshot ? "该账户当前没有持仓。" : "等待持仓同步…";

  return <section className="workspace watchlist-workspace accounts-workspace">
    <div className="watchlist-heading"><div><p className="eyebrow">我的投资</p><h2>实体账户</h2><p>持仓、行情与公司基本面，在这里一起看清。</p></div><button className="ghost-button" onClick={() => { refreshFundamentals(); void refresh(); }} disabled={loading || accountLoading || financialLoading || !token}><RefreshCw size={16} className={loading || accountLoading || financialLoading ? "spin" : ""} />刷新</button></div>
    <section className="surface-card account-selector-card">
      <div className="account-selector"><WalletCards size={24} /><div><label htmlFor="connected-account">选择账户</label><select id="connected-account" value={accountId} disabled={accountLoading} onChange={event => { setSnapshot(null); setHoldingsError(""); setAccountId(event.target.value); }}><option value="all">全部账户（合并）</option>{accounts.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></div><span className={`account-connection-status ${token && account?.status === "ready" ? holdingsError || snapshot?.stale || snapshot?.complete === false ? "failed" : "ready" : ""}`}>{accountLoading ? "检查连接…" : token && account?.status === "ready" ? holdingsError ? "同步失败" : snapshot?.stale ? "数据已过期" : snapshot?.complete === false ? "同步不完整" : snapshot ? "同步正常" : "已配置" : "未连接"}</span></div>
      <div className="account-description"><ShieldCheck size={15} /><span>只读同步</span><span>{accountId === "all" ? "各平台持仓合并展示" : account?.account_type === "classic" ? "经典账户" : account?.account_type === "uta" || account?.account_type === "unified" ? "统一交易账户" : "现货账户"}</span>{snapshot?.as_of && <span>数据时间 {new Date(snapshot.as_of).toLocaleString("zh-CN", { hour12: false })}</span>}{snapshot?.stale && snapshot.last_success && <span>上次完整同步 {new Date(snapshot.last_success).toLocaleString("zh-CN", { hour12: false })}</span>}</div>
    </section>
    <section className="account-valuation" aria-label="账户资产估值">
      <div className="account-total-primary"><span>{partialTotal ? "已获取资产小计" : "资产总额"}<small>估算{snapshot?.stale || holdingsError ? " · 上次快照" : ""}</small></span><strong>{holdingNumber(valuation?.usdt)}<small>U</small></strong></div>
      <div className="account-total-cny"><span>人民币估值</span><strong>{valuation?.cny != null ? `≈ ¥ ${holdingNumber(valuation.cny)}` : "—"}</strong></div>
      <div className="account-valuation-note"><span>已接入账户余额及现货市值 · USD / USDT 按 1:1 估算</span>{fx ? <span>1 U ≈ ¥ {holdingNumber(fx.rates.CNY)} · {new Date(fx.asOf).toLocaleDateString("zh-CN")} 日汇率 · <a href="https://www.exchangerate-api.com" target="_blank" rel="noreferrer">ExchangeRate-API</a></span> : <span>人民币折算汇率暂未取得</span>}{partialTotal && <span>{valuation?.missing ? `${valuation.missing} 项资产暂缺估值；` : ""}部分数据未取得，小计可能不完整。</span>}{Boolean(valuation?.futures) && <span>合约名义价值不计入；统一账户权益包含的盈亏不重复相加，独立合约账户保证金未接入。</span>}</div>
    </section>
    {!token && <div className="auth-callout"><KeyRound size={18} /><span>请先登录慧投，读取与你绑定的实体账户。</span><button onClick={onNeedToken}>去登录</button></div>}
    {token && !accountLoading && !accountError && account?.status !== "ready" && <div className="account-connect-note"><Building2 size={20} /><div><strong>{account?.status === "unbound" ? "当前慧投用户尚未绑定此账户" : "账户尚未连接"}</strong><p>请在对应交易所创建只读 API 凭证，再联系此慧投服务的管理员完成账户绑定。</p><button className="ghost-button" onClick={() => setShowConnectionHelp(previous => !previous)}>{showConnectionHelp ? "收起连接说明" : "查看连接说明"}</button></div></div>}
    {showConnectionHelp && <section className="surface-card connection-help"><h3>连接交易所账户</h3><ol><li>在对应交易所的 API 管理中创建仅有读取权限的凭证。</li><li>由服务管理员在部署环境中设置凭证、账户类型和你的慧投用户绑定，完成后重启后端。</li><li>回到这里点击刷新，成功读取后会显示同步时间。</li></ol><p>现货成本和盈亏需要成交记录；当前只展示可从账户接口确认的数据。</p></section>}
    {accountError && <p className="form-note danger" role="alert">{accountError}</p>}
    {holdingsError && <p className="form-note danger" role="alert">{snapshot ? "同步失败，当前显示上次取得的数据。" : "持仓同步失败。"}{holdingsError}</p>}
    {insightError && <p className="form-note danger" role="status">AI 财报解读：{insightError}</p>}
    {snapshot?.warnings.map(warning => <p className="form-note danger" role="status" key={warning}>{warning}</p>)}
    <div className="watchlist-toolbar"><div className="watchlist-filters">{[{ id: "all", displayName: "全部" }, ...markets, { id:"other", displayName:"其他 / 待识别" }].map(market => <button key={market.id} className={filter === market.id ? "active" : ""} onClick={() => setFilter(market.id as MarketId | "all" | "other")}>{market.displayName}<span>{snapshot ? market.id === "all" ? stocks.length : stocks.filter(stock => stock.market === market.id).length : "—"}</span></button>)}</div><span className="watchlist-note">{financialLoading ? "财报数据加载中…" : stocks.some(stock => fundamentals[stockKey(stock)]?.status === "unavailable") ? "部分财报指标暂未取得" : ""}</span></div>
    {accountId === "all" && <div className="account-provider-statuses">{accounts.map(item => { const sync=snapshot?.accounts?.find(state=>state.id===item.id); const labels:Record<string,string>={ready:"等待同步",synced:"同步正常",partial:"同步不完整",stale:"上次快照",failed:"同步失败",not_configured:"未配置",unbound:"未绑定"};return <button className="ghost-button" key={item.id} onClick={()=>setAccountId(item.id)} title={sync?.message || item.message}>{item.name} · {labels[sync?.status || (holdingsError && item.status === "ready" ? "failed" : item.status)] || "等待同步"}</button>;})}</div>}
    {account?.account_type === "unified" && <p className="watchlist-note">统一账户余额按币种权益展示，包含负债与盈亏影响，不能与合约名义价值直接相加。</p>}
    {token && <PriceAlertSettings alerts={priceAlerts} />}
    <StockResearchTable stocks={visible} markets={markets} fundamentals={fundamentals} onOpenStock={onOpenStock} getRowKey={stock => stock.holding_id} emptyMessage={emptyMessage} loading={loading && !snapshot}
      onOpenReport={stock => setFilingView({ code: stock.id.toUpperCase(), type: "report" })}
      onOpenInsight={stock => { const code = stock.id.toUpperCase(); const report = fundamentals[stockKey(stock)]?.financial_reports?.[0]; const stale = insights[code]?.status === "ready" && !!report && insights[code].accession !== report.accession; setFilingView({ code, type: "insight" }); if (!insights[code]?.status || insights[code]?.status === "unavailable" || stale) void refreshInsight(code, stale, report?.accession); }}
      insightStatus={Object.fromEntries(Object.entries(insights).map(([code, insight]) => [code, insight.status]))}
      renderTarget={stock => <TargetPriceCell stock={stock} alerts={priceAlerts} />}
      getHoldingValue={stock => stock.market_value != null && stock.market_value.trim() !== "" && Number.isFinite(Number(stock.market_value)) && stock.currency ? { amount: Number(stock.market_value), currency: stock.currency } : null}
      renderHolding={stock => <div className="holding-cell"><small>{stock.account_name || stock.source}{stock.stale ? " · 上次快照" : ""}</small><strong><span className="holding-quantity" title={`原始数量 ${stock.quantity} ${stock.quantity_unit || ""}`}>{holdingQuantity(stock.quantity)} {stock.quantity_unit}</span><span>{stock.holding_type === "balance" ? "币种权益" : stock.holding_type === "spot" ? "现货" : stock.side === "short" ? "空头" : "多头"}</span></strong>{stock.holding_type !== "futures" && <small>{stock.market_value ? `${stock.holding_type === "balance" ? "权益估值" : "市值"} ${holdingNumber(stock.market_value)} ${stock.currency}` : "估值暂不可用"}</small>}{stock.entry_price && <small>均价 {holdingNumber(stock.entry_price)}</small>}{stock.unrealized_pnl && <small className={Number(stock.unrealized_pnl)>0 ? "up" : Number(stock.unrealized_pnl)<0 ? "down" : ""}>浮盈亏 {holdingNumber(stock.unrealized_pnl)} {stock.margin_currency}</small>}</div>}

    />
    {filingView && (() => {
      const stock = stocks.find(item => item.market === "us_stock" && item.id.toUpperCase() === filingView.code);
      const data = stock && fundamentals[stockKey(stock)];
      const report = data?.financial_reports?.[0];
      if (!stock || !data || !report) return null;
      return filingView.type === "report" ? <ReportDetailDialog stock={stock} fundamentals={data} onClose={() => setFilingView(null)} />
        : <FilingInsightDialog stock={stock} report={report} insight={insights[filingView.code]} busy={insightBusy === filingView.code} onRetry={() => void refreshInsight(filingView.code, true, report.accession)} onClose={() => setFilingView(null)} />;
    })()}
    <p className="watchlist-data-note">展示已连接账户的股票、加密资产余额及已启用的合约持仓；不同平台、产品和多空方向分别列示。行情与公司财报沿用自选列表的数据来源和口径；当前最新财报与 AI 解读覆盖 SEC 收录的美股定期申报。全部账户不包含未配置、未绑定的平台或未接入的理财、资金账户及期权。价格保留各平台币种与数量单位，无法识别的资产仍保留数量。</p>
  </section>;
}
