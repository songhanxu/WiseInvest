import { useCallback, useEffect, useRef, useState } from "react";
import { Building2, KeyRound, RefreshCw, ShieldCheck, WalletCards } from "lucide-react";
import { requestJson } from "./App";
import type { Market, MarketId, StockQuote } from "./App";
import { StockResearchTable, stockKey, useStockFundamentals } from "./StockResearchTable";

type Account = { id: string; provider: string; name: string; account_type: string; status: "ready" | "not_configured" | "unbound"; message?: string };
type Holding = StockQuote & { holding_id: string; holding_type: "spot" | "futures"; side: string; quantity: string; margin_mode?: string };
type Snapshot = { account_id: string; holdings: Holding[]; as_of: string; complete: boolean; warnings: string[] };
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
    <div className="watchlist-heading"><div><p className="eyebrow">Connected Accounts</p><h2>实体账户</h2><p>查看交易所实际持仓，结合公司财报和估值进行研究。</p></div><button className="ghost-button" onClick={() => void refresh()} disabled={loading || accountLoading || !token}><RefreshCw size={16} className={loading || accountLoading ? "spin" : ""} />刷新</button></div>
    <section className="surface-card account-selector-card">
      <div className="account-selector"><WalletCards size={24} /><div><label htmlFor="connected-account">选择账户</label><select id="connected-account" value={accountId} disabled={accountLoading} onChange={event => { setSnapshot(null); setHoldingsError(""); setAccountId(event.target.value); }}>{accounts.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></div><span className={`account-connection-status ${token && account?.status === "ready" ? "ready" : ""}`}>{accountLoading ? "检查连接…" : token && account?.status === "ready" ? "已配置" : "未连接"}</span></div>
      <div className="account-description"><ShieldCheck size={15} /><span>只读同步</span><span>{account?.account_type === "classic" ? "经典账户" : account?.account_type === "uta" ? "统一交易账户" : "Bitget"}</span><span>{snapshot ? `${stocks.length} 项股票持仓` : "持仓尚未同步"}</span>{snapshot?.as_of && <span>更新 {new Date(snapshot.as_of).toLocaleTimeString("zh-CN", { hour12: false })}</span>}</div>
    </section>
    {!token && <div className="auth-callout"><KeyRound size={18} /><span>请先登录慧投，读取与你绑定的实体账户。</span><button onClick={onNeedToken}>去登录</button></div>}
    {token && !accountLoading && !accountError && account?.status !== "ready" && <div className="account-connect-note"><Building2 size={20} /><div><strong>{account?.status === "unbound" ? "当前慧投用户尚未绑定 Bitget" : "Bitget 账户尚未连接"}</strong><p>完成账户连接后，这里会显示真实持仓和最新行情。</p></div></div>}
    {accountError && <p className="form-note danger" role="alert">{accountError}</p>}
    {holdingsError && <p className="form-note danger" role="alert">{snapshot ? "同步失败，当前显示上次取得的数据。" : "持仓同步失败。"}{holdingsError}</p>}
    {snapshot?.warnings.map(warning => <p className="form-note danger" role="status" key={warning}>{warning}</p>)}
    <div className="watchlist-toolbar"><div className="watchlist-filters">{[{ id: "all", displayName: "全部" }, ...markets.filter(market => market.id === "us_stock" || market.id === "hk_stock")].map(market => <button key={market.id} className={filter === market.id ? "active" : ""} onClick={() => setFilter(market.id as MarketId | "all")}>{market.displayName}<span>{snapshot ? market.id === "all" ? stocks.length : stocks.filter(stock => stock.market === market.id).length : "—"}</span></button>)}</div><span className="watchlist-note">{financialLoading ? "财报数据加载中…" : stocks.some(stock => fundamentals[stockKey(stock)]?.status === "unavailable") ? "部分财报指标暂未取得" : ""}</span></div>
    <StockResearchTable stocks={visible} markets={markets} fundamentals={fundamentals} onOpenStock={onOpenStock} getRowKey={stock => stock.holding_id} emptyMessage={emptyMessage}
      renderHolding={stock => <small className="account-position-detail">{stock.holding_type === "spot" ? "现货" : stock.side === "short" ? "合约 · 空头" : stock.side === "long" ? "合约 · 多头" : "合约"} · 数量 {stock.quantity}</small>}
    />
    <p className="watchlist-data-note">这里只显示实际持有的股票代币及股票合约。现货与合约、合约多头与空头分别列示；价格按对应 Bitget 产品报价。公司财报指标与自选列表使用相同的数据来源和口径。</p>
  </section>;
}
