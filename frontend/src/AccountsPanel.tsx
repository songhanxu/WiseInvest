import { useCallback, useEffect, useRef, useState } from "react";
import { Building2, KeyRound, RefreshCw, ShieldCheck, WalletCards } from "lucide-react";
import { requestJson } from "./App";
import type { Market, MarketId, StockQuote } from "./App";
import { StockResearchTable, numeric, stockKey, useStockFundamentals } from "./StockResearchTable";

type Account = { id: string; provider: string; name: string; account_type: string; status: "ready" | "not_configured" | "unbound"; message?: string };
type Holding = StockQuote & { holding_id: string; holding_type: "spot" | "futures"; side: string; quantity: string; margin_mode?: string; entry_price?: string; unrealized_pnl?: string; margin_currency?: string; market_value?: string };
type Snapshot = { account_id: string; holdings: Holding[]; as_of: string; complete: boolean; stale?: boolean; last_success?: string; warnings: string[] };
const bitgetAccount: Account = { id: "bitget", provider: "bitget", name: "Bitget", account_type: "", status: "not_configured" };

export function AccountsPanel({ apiBase, token, markets, onOpenStock, onNeedToken }: {
  apiBase: string; token: string; markets: Market[];
  onOpenStock: (market: Market, stock: StockQuote) => void;
  onNeedToken: () => void;
}) {
  const [accounts, setAccounts] = useState<Account[]>([bitgetAccount]);
  const [accountId, setAccountId] = useState("bitget");
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [accountLoading, setAccountLoading] = useState(false);
  const [loading, setLoading] = useState(false);
  const [accountError, setAccountError] = useState("");
  const [holdingsError, setHoldingsError] = useState("");
  const [showConnectionHelp, setShowConnectionHelp] = useState(false);
  const [filter, setFilter] = useState<MarketId | "all">("all");
  const version = useRef(0);
  const listVersion = useRef(0);
  const account = accounts.find(item => item.id === accountId);
  const stocks = snapshot?.holdings ?? [];
  const { fundamentals, financialLoading } = useStockFundamentals(apiBase, stocks, `${token}:${accountId}`);

  const refreshAccounts = useCallback(async () => {
    const request = ++listVersion.current;
    if (!token) { setAccounts([bitgetAccount]); setAccountError(""); setAccountLoading(false); return; }
    setAccountLoading(true); setAccountError("");
    try {
      const data = await requestJson<Account[]>(apiBase, "/api/v1/accounts", token);
      if (!Array.isArray(data)) throw new Error("账户列表格式不正确");
      if (request !== listVersion.current) return;
      setAccounts(data);
      setAccountId(previous => data.some(item => item.id === previous) ? previous : data[0]?.id ?? "");
    } catch (err) {
      if (request === listVersion.current) setAccountError(err instanceof Error ? err.message : "账户列表暂不可用");
    } finally { if (request === listVersion.current) setAccountLoading(false); }
  }, [apiBase, token]);

  useEffect(() => {
    setAccounts([bitgetAccount]); setAccountId("bitget"); setSnapshot(null); setFilter("all");
    void refreshAccounts();
    return () => { listVersion.current++; version.current++; };
  }, [refreshAccounts]);

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
    void refreshHoldings();
    const timer = window.setInterval(() => { if (!document.hidden) void refreshHoldings(); }, 60000);
    return () => { window.clearInterval(timer); version.current++; };
  }, [refreshHoldings]);

  async function refresh() { await refreshAccounts(); await refreshHoldings(); }
  const visible = stocks.filter(stock => filter === "all" || stock.market === filter);
  const emptyMessage = loading ? "正在读取实际股票持仓…" : !token ? "登录并连接账户后展示实际持仓。" : accountError ? "账户列表暂不可用。" : account?.status !== "ready" ? "账户尚未连接，连接后将在这里展示实际股票持仓。" : holdingsError || (snapshot && !snapshot.complete) ? "持仓数据尚未完整取得，请稍后刷新。" : snapshot ? "该账户当前没有可识别的股票持仓。" : "等待持仓同步…";

  return <section className="workspace watchlist-workspace accounts-workspace">
    <div className="watchlist-heading"><div><p className="eyebrow">我的投资</p><h2>实体账户</h2><p>持仓、行情与公司基本面，在这里一起看清。</p></div><button className="ghost-button" onClick={() => void refresh()} disabled={loading || accountLoading || !token}><RefreshCw size={16} className={loading || accountLoading ? "spin" : ""} />刷新</button></div>
    <section className="surface-card account-selector-card">
      <div className="account-selector"><WalletCards size={24} /><div><label htmlFor="connected-account">选择账户</label><select id="connected-account" value={accountId} disabled={accountLoading} onChange={event => { setSnapshot(null); setHoldingsError(""); setAccountId(event.target.value); }}>{accounts.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></div><span className={`account-connection-status ${token && account?.status === "ready" ? holdingsError || snapshot?.stale || snapshot?.complete === false ? "failed" : "ready" : ""}`}>{accountLoading ? "检查连接…" : token && account?.status === "ready" ? holdingsError ? "同步失败" : snapshot?.stale ? "数据已过期" : snapshot?.complete === false ? "同步不完整" : snapshot ? "同步正常" : "已配置" : "未连接"}</span></div>
      <div className="account-description"><ShieldCheck size={15} /><span>只读同步</span><span>{account?.account_type === "classic" ? "经典账户" : account?.account_type === "uta" ? "统一交易账户" : "Bitget"}</span>{snapshot?.as_of && <span>数据时间 {new Date(snapshot.as_of).toLocaleString("zh-CN", { hour12: false })}</span>}{snapshot?.stale && snapshot.last_success && <span>上次完整同步 {new Date(snapshot.last_success).toLocaleString("zh-CN", { hour12: false })}</span>}</div>
    </section>
    <div className="account-facts"><div><span>股票持仓</span><strong>{snapshot ? stocks.length : "—"}<small>项</small></strong></div><div><span>覆盖市场</span><strong>{snapshot ? new Set(stocks.map(stock => stock.market)).size : "—"}<small>个</small></strong></div><div><span>现货 / 合约</span><strong>{snapshot ? `${stocks.filter(stock => stock.holding_type === "spot").length} / ${stocks.filter(stock => stock.holding_type === "futures").length}` : "—"}</strong></div><div><span>最近同步</span><strong className="fact-status">{!token || account?.status !== "ready" ? "等待连接" : loading ? "正在更新" : holdingsError || snapshot?.stale ? "待更新" : snapshot ? new Date(snapshot.as_of).toLocaleTimeString("zh-CN", {hour12:false}) : "等待同步"}</strong></div></div>
    {!token && <div className="auth-callout"><KeyRound size={18} /><span>请先登录慧投，读取与你绑定的实体账户。</span><button onClick={onNeedToken}>去登录</button></div>}
    {token && !accountLoading && !accountError && account?.status !== "ready" && <div className="account-connect-note"><Building2 size={20} /><div><strong>{account?.status === "unbound" ? "当前慧投用户尚未绑定 Bitget" : "Bitget 账户尚未连接"}</strong><p>请在 Bitget 创建只读 API 凭证，再联系此慧投服务的管理员完成账户绑定。</p><button className="ghost-button" onClick={() => setShowConnectionHelp(previous => !previous)}>{showConnectionHelp ? "收起连接说明" : "查看连接说明"}</button></div></div>}
    {showConnectionHelp && <section className="surface-card connection-help"><h3>连接 Bitget</h3><ol><li>在 Bitget 的 API 管理中创建仅有读取权限的凭证。</li><li>由服务管理员在部署环境中设置凭证、账户类型和你的慧投用户绑定，完成后重启后端。</li><li>回到这里点击刷新，成功读取后会显示同步时间。</li></ol><p>现货成本和盈亏需要成交记录；当前只展示可从账户接口确认的数据。</p></section>}
    {accountError && <p className="form-note danger" role="alert">{accountError}</p>}
    {holdingsError && <p className="form-note danger" role="alert">{snapshot ? "同步失败，当前显示上次取得的数据。" : "持仓同步失败。"}{holdingsError}</p>}
    {snapshot?.warnings.map(warning => <p className="form-note danger" role="status" key={warning}>{warning}</p>)}
    <div className="watchlist-toolbar"><div className="watchlist-filters">{[{ id: "all", displayName: "全部" }, ...markets.filter(market => market.id === "us_stock" || market.id === "hk_stock")].map(market => <button key={market.id} className={filter === market.id ? "active" : ""} onClick={() => setFilter(market.id as MarketId | "all")}>{market.displayName}<span>{snapshot ? market.id === "all" ? stocks.length : stocks.filter(stock => stock.market === market.id).length : "—"}</span></button>)}</div><span className="watchlist-note">{financialLoading ? "财报数据加载中…" : stocks.some(stock => fundamentals[stockKey(stock)]?.status === "unavailable") ? "部分财报指标暂未取得" : ""}</span></div>
    <StockResearchTable stocks={visible} markets={markets} fundamentals={fundamentals} onOpenStock={onOpenStock} getRowKey={stock => stock.holding_id} emptyMessage={emptyMessage}
      renderHolding={stock => <div className="holding-cell"><strong>{stock.quantity}<span>{stock.holding_type === "spot" ? "现货" : stock.side === "short" ? "空头" : "多头"}</span></strong>{stock.holding_type === "spot" ? <small>{stock.market_value ? `市值 ${numeric(Number(stock.market_value))} ${stock.currency}` : "市值暂不可用"}</small> : <><small>均价 {numeric(stock.entry_price ? Number(stock.entry_price) : null)}</small><small className={Number(stock.unrealized_pnl) > 0 ? "up" : Number(stock.unrealized_pnl) < 0 ? "down" : ""}>浮盈亏 {numeric(stock.unrealized_pnl ? Number(stock.unrealized_pnl) : null)} {stock.margin_currency}</small></>}</div>}

    />
    <p className="watchlist-data-note">这里只显示实际持有的股票代币及股票合约。现货与合约、合约多头与空头分别列示；价格按对应 Bitget 产品报价。公司财报指标与自选列表使用相同的数据来源和口径。</p>
  </section>;
}
