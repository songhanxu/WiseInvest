import { useCallback, useEffect, useRef, useState } from "react";
import { requestJson } from "./App";

export type PriceAlert = {
  id: number; holding_id: string; account_id: string; target_price: number;
  currency: string; direction: "above" | "below"; trigger_mode: "auto" | "above" | "below"; revision: number;
  status: "armed" | "sending" | "sent" | "error"; last_error?: string;
  sent_at?: string; last_checked_at?: string | null;
};
type AlertData = { alerts: PriceAlert[]; email: string; mailer_configured: boolean; check_interval_seconds: number };
type AlertHolding = { holding_id: string; account_id: string; name: string; currency?: string; quote_available?: boolean; current_price: number; stale?: boolean };

export function usePriceAlerts(apiBase: string, token: string) {
  const [data, setData] = useState<AlertData | null>(null);
  const [error, setError] = useState("");
  const version = useRef(0);
  const scope = `${apiBase}:${token}`;
  const scopeRef = useRef(scope); scopeRef.current = scope;
  const refresh = useCallback(async () => {
    const request = ++version.current;
    if (!token) { setData(null); setError(""); return; }
    try {
      const next = await requestJson<AlertData>(apiBase, "/api/v1/price-alerts", token);
      if (!Array.isArray(next.alerts)) throw new Error("提醒数据格式无效");
      if (request === version.current) { setData(next); setError(""); }
    } catch (err) { if (request === version.current) setError(err instanceof Error ? err.message : "提醒读取失败"); }
  }, [apiBase, token]);
  useEffect(() => {
    setData(null); setError(""); void refresh();
    const timer = window.setInterval(() => { if (!document.hidden) void refresh(); }, 60000);
    return () => { version.current++; window.clearInterval(timer); };
  }, [refresh]);
  async function saveEmail(email: string) {
    await requestJson(apiBase, "/api/v1/price-alerts/settings", token, { method: "PUT", body: JSON.stringify({ email }) });
    if (scopeRef.current === scope) await refresh();
  }
  async function save(stock: AlertHolding, target: number) {
    const row = await requestJson<PriceAlert>(apiBase, "/api/v1/price-alerts", token, { method: "PUT", body: JSON.stringify({ account_id: stock.account_id, holding_id: stock.holding_id, target_price: target, direction: "auto" }) });
    if (scopeRef.current === scope) { version.current++; setData(previous => previous ? { ...previous, alerts: [...previous.alerts.filter(item => item.holding_id !== row.holding_id), row] } : previous); }
  }
  async function remove(holding: string) {
    await requestJson(apiBase, `/api/v1/price-alerts?holding_id=${encodeURIComponent(holding)}`, token, { method: "DELETE" });
    if (scopeRef.current === scope) { version.current++; setData(previous => previous ? { ...previous, alerts: previous.alerts.filter(item => item.holding_id !== holding) } : previous); }
  }
  return { data, error, refresh, saveEmail, save, remove };
}
export type PriceAlertsController = ReturnType<typeof usePriceAlerts>;

export function PriceAlertSettings({ alerts }: { alerts: PriceAlertsController }) {
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [failed, setFailed] = useState(false);
  useEffect(() => { setEmail(alerts.data?.email || ""); setMessage(""); }, [alerts.data?.email]);
  return <section className="surface-card price-alert-settings"><div><h3>到价邮件提醒</h3><p>填写期望价格并保存，系统按保存时的现价自动判断涨到或跌到，每次设置只提醒一次。</p></div>
    <form onSubmit={async event => { event.preventDefault(); setBusy(true); setMessage(""); try { await alerts.saveEmail(email.trim()); setMessage(email.trim() ? "提醒邮箱已保存" : "提醒邮箱已清除，邮件提醒暂停"); setFailed(false); } catch (err) { setMessage(err instanceof Error ? err.message : "邮箱保存失败"); setFailed(true); } finally { setBusy(false); } }}>
      <label htmlFor="price-alert-email">收件邮箱</label><input id="price-alert-email" type="email" autoComplete="email" placeholder="填写接收提醒的邮箱" value={email} onChange={event => setEmail(event.target.value)} disabled={busy || !alerts.data} /><button className="ghost-button" type="submit" disabled={busy || !alerts.data}>{busy ? "保存中…" : "保存邮箱"}</button>
    </form>
    {alerts.error && <p className="form-note danger" role="alert">{alerts.error}</p>}
    {message && <p className={`form-note ${failed ? "danger" : "success"}`} role="status">{message}</p>}
    {alerts.data && !alerts.data.mailer_configured && <p className="form-note danger" role="status">邮件发送服务尚未启用，目标价可以先保存；配置完成后才会发送提醒。</p>}
    {alerts.data && !alerts.data.email && <p className="watchlist-note">请先保存收件邮箱，才能接收到价提醒。</p>}
    <p className="watchlist-data-note">后端每分钟检查一次；关闭页面后继续监控，需后端保持运行且行情连接正常。提醒使用对应行的报价币种；移出自选或持仓、行情不可用时暂停检测。发送失败会显示状态，检查配置后重新保存目标价即可启用。</p>
  </section>;
}

export function TargetPriceCell({ stock, alerts }: { stock: AlertHolding; alerts: PriceAlertsController }) {
  const row = alerts.data?.alerts.find(item => item.holding_id === stock.holding_id);
  const [draft, setDraft] = useState(row ? String(row.target_price) : "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { setDraft(row ? String(row.target_price) : ""); setError(""); }, [row?.target_price, stock.holding_id]);
  const canSave = Boolean(alerts.data) && !stock.stale && stock.quote_available !== false && stock.current_price > 0;
  const status = !row ? "" : row.status === "sent" ? "已邮件提醒" : row.status === "sending" ? "发送中 / 结果待确认" : row.status === "error" ? "发送失败 / 待确认" : !alerts.data?.email ? "待设置收件邮箱" : !alerts.data?.mailer_configured ? "待启用邮件服务" : row.last_error ? "行情暂不可用" : "监控中";
  return <div className="target-price-cell"><form onSubmit={async event => { event.preventDefault(); const value = Number(draft); if (!draft.trim() || !Number.isFinite(value) || value <= 0) { setError("请输入大于 0 的价格"); return; } setBusy(true); setError(""); try { await alerts.save(stock, value); } catch (err) { setError(err instanceof Error ? err.message : "保存失败"); } finally { setBusy(false); } }}>
    <div className="target-price-input"><input type="number" step="any" min="0" max="1000000000000000" aria-label={`${stock.name}期望价格`} placeholder="设置期望价格" value={draft} onChange={event => setDraft(event.target.value)} disabled={busy || !alerts.data} /><span>{stock.currency}</span></div>
    <div className="target-price-actions"><button type="submit" disabled={busy || !canSave}>{busy ? "保存中…" : row ? row.status === "sent" || row.status === "error" ? "重新启用" : "保存" : "设置提醒"}</button>{row && <button type="button" disabled={busy} onClick={async () => { setBusy(true); setError(""); try { await alerts.remove(stock.holding_id); } catch (err) { setError(err instanceof Error ? err.message : "删除失败"); } finally { setBusy(false); } }}>删除</button>}</div>
  </form>{row && <small className={row.status === "error" ? "danger" : ""} title={row.last_error}>{row.direction === "above" ? "≥" : "≤"} {row.target_price} {row.currency} · {status}</small>}{row?.sent_at && <small>{new Date(row.sent_at).toLocaleString("zh-CN", { hour12: false })}</small>}{row?.last_checked_at && row.status === "armed" && <small>检查于 {new Date(row.last_checked_at).toLocaleTimeString("zh-CN", {hour12:false})}</small>}{error && <small className="danger" role="alert">{error}</small>}{!canSave && alerts.data && <small>等待有效行情后可保存</small>}</div>;
}
