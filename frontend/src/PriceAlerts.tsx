import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { requestJson } from "./App";
import { Bell, Check, Loader2, RotateCcw, X } from "lucide-react";
import { useDismissiblePopover } from "./useDismissiblePopover";

import { findPriceAlert, priceAlertState } from "./price-alert-state";
import type { AlertData, PriceAlert } from "./price-alert-state";
import { watchlistCode } from "./watchlist-state";
export type { PriceAlert } from "./price-alert-state";
type AlertHolding = { holding_id: string; account_id: string; name: string; id: string; symbol: string; market: string; currency?: string; quote_available?: boolean; current_price: number; stale?: boolean };

export function usePriceAlerts(apiBase: string, token: string, isActive = true) {
  const state = useMemo(() => priceAlertState(apiBase, token), [apiBase, token]);
  const { data, error } = useSyncExternalStore(state.subscribe, state.read);
  const activeRef = useRef(isActive); activeRef.current = isActive;
  const refresh = useCallback(() => token ? state.refresh(() => requestJson<AlertData>(apiBase, "/api/v1/price-alerts", token)) : Promise.resolve(), [apiBase, token, state]);
  useEffect(() => {
    const timer = window.setInterval(() => { if (activeRef.current && !document.hidden) void refresh(); }, 60000);
    return () => window.clearInterval(timer);
  }, [refresh]);
  useEffect(() => { if (isActive && token && (!state.read().data || Date.now() - state.lastAttemptAt >= 60000)) void refresh(); }, [isActive, token, refresh, state]);
  async function saveEmail(email: string) {
    await requestJson(apiBase, "/api/v1/price-alerts/settings", token, { method: "PUT", body: JSON.stringify({ email }) });
    state.emailSaved(email);
    await refresh();
  }
  async function save(stock: AlertHolding, target: number) {
    const row = await requestJson<PriceAlert>(apiBase, "/api/v1/price-alerts", token, { method: "PUT", body: JSON.stringify({ account_id: stock.account_id, holding_id: stock.holding_id, target_price: target, direction: "auto" }) });
    state.saved(row);
  }
  async function remove(holding: string) {
    await requestJson(apiBase, `/api/v1/price-alerts?holding_id=${encodeURIComponent(holding)}`, token, { method: "DELETE" });
    state.removed(holding);
  }
  return { data, error, refresh, saveEmail, save, remove };
}
export type PriceAlertsController = ReturnType<typeof usePriceAlerts>;

export function PriceAlertSettings({ alerts }: { alerts: PriceAlertsController }) {
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [failed, setFailed] = useState(false);
  const { rootRef, isOpen: editing, setIsOpen: setEditing } = useDismissiblePopover<HTMLElement>(!busy);
  const savedEmail = alerts.data?.email || "";
  useEffect(() => { if (!editing) setEmail(savedEmail); }, [savedEmail, editing]);

  return <section className="surface-card price-alert-settings" ref={rootRef}>
    <div className="price-alert-summary"><Bell size={17} /><div><h3>到价邮件提醒</h3><p>自动判断涨跌，每次设置只提醒一次。后端每分钟检查，需保持运行。</p></div></div>
    {!savedEmail || editing ? <form onSubmit={async event => {
      event.preventDefault();
      const next = email.trim();
      if (next === savedEmail) { setEditing(false); return; }
      setBusy(true); setMessage("");
      try {
        await alerts.saveEmail(next);
        setEditing(false); setMessage(next ? "邮箱已保存" : "邮件提醒已暂停"); setFailed(false);
      } catch (err) { setMessage(err instanceof Error ? err.message : "邮箱保存失败"); setFailed(true); }
      finally { setBusy(false); }
    }}>
      <input aria-label="收件邮箱" type="email" autoComplete="email" placeholder="填写接收提醒的邮箱" value={email} onChange={event => setEmail(event.target.value)} disabled={busy || !alerts.data} autoFocus={editing} />
      <button className="alert-confirm" type="submit" aria-label="保存收件邮箱" title="保存收件邮箱" disabled={busy || !alerts.data || email.trim() === savedEmail}>{busy ? <Loader2 size={15} className="spin" /> : <Check size={15} />}</button>
    </form> : <div className="price-alert-recipient"><span title={savedEmail}>{savedEmail}</span><button type="button" className="alert-text-button" aria-label="修改收件邮箱" onClick={() => { setEmail(savedEmail); setMessage(""); setEditing(true); }}>修改</button></div>}
    {alerts.error && <p className="form-note danger alert-setting-note" role="alert">{alerts.error}</p>}
    {message && <p className={`form-note alert-setting-note ${failed ? "danger" : "success"}`} role="status">{message}</p>}
    {alerts.data && !alerts.data.mailer_configured && <p className="form-note danger alert-setting-note" role="status">邮件服务尚未启用，暂不发送提醒。</p>}
  </section>;
}

export function TargetPriceCell({ stock, alerts }: { stock: AlertHolding; alerts: PriceAlertsController }) {
  const assetKey = ["a_share", "us_stock", "hk_stock", "crypto"].includes(stock.market) ? `${stock.market}:${watchlistCode(stock)}` : undefined;
  const row = findPriceAlert(alerts.data?.alerts || [], stock.holding_id, assetKey);
  const [draft, setDraft] = useState(row ? String(row.target_price) : "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { setDraft(row ? String(row.target_price) : ""); setError(""); }, [row?.target_price, row?.revision, stock.holding_id]);
  const canSave = Boolean(alerts.data) && !stock.stale && stock.quote_available !== false && stock.current_price > 0;
  const value = Number(draft);
  const hasDraft = Boolean(draft.trim());
  const valid = hasDraft && Number.isFinite(value) && value > 0 && value <= 1e15;
  const amountChanged = Boolean(row) && (!hasDraft || value !== row?.target_price);
  const currencyChanged = Boolean(row?.currency && stock.currency && row.currency !== stock.currency);
  const changed = amountChanged || currencyChanged;
  const inputCurrency = row && !amountChanged ? row.currency : stock.currency;
  const rearm = row?.status === "sent" || row?.status === "error";
  const sending = row?.status === "sending";
  const showSave = !row || changed || rearm;
  const pending = Boolean(row) && (!alerts.data?.email || !alerts.data?.mailer_configured || Boolean(row?.last_error) || !canSave || currencyChanged);
  const status = !row ? "" : row.status === "sent" ? "已提醒" : sending ? "发送中" : row.status === "error" ? "发送失败 / 结果待确认" : !alerts.data?.email ? "等待设置收件邮箱" : !alerts.data?.mailer_configured ? "等待启用邮件服务" : currencyChanged ? "报价币种已变更" : row.last_error || !canSave ? "等待有效行情" : "监控中";
  const detail = row ? [`${status}：${row.direction === "above" ? "涨至" : "跌至"} ${row.target_price} ${row.currency}`, row.last_error, row.sent_at ? `提醒时间 ${new Date(row.sent_at).toLocaleString("zh-CN", {hour12:false})}` : row.last_checked_at ? `上次检查 ${new Date(row.last_checked_at).toLocaleString("zh-CN", {hour12:false})}` : ""].filter(Boolean).join("；") : "";
  const saveLabel = `${currencyChanged ? `按${stock.currency}更新` : rearm ? "重新启用" : row ? "保存" : "开启"}${stock.name}提醒`;
  const saveTitle = !canSave ? "等待有效行情后可保存" : !valid ? "请输入大于 0 的价格" : currencyChanged ? `原提醒 ${row?.target_price} ${row?.currency}；点击后按当前 ${stock.currency} 行情重新设置` : rearm ? `${detail}；点击重新启用` : row ? "保存新的期望价格" : "开启到价提醒";
  async function remove() {
    if (busy || sending) return;
    setBusy(true); setError("");
    try { await alerts.remove(row?.holding_id || stock.holding_id); }
    catch (err) { setError(err instanceof Error ? err.message : "关闭失败"); }
    finally { setBusy(false); }
  }

  return <div className="price-alert-cell">
    <form className="price-alert-form" onSubmit={async event => {
      event.preventDefault();
      if (!valid) { setError("请输入有效的期望价格"); return; }
      if (!showSave || sending || !canSave || busy) return;
      setBusy(true); setError("");
      try { await alerts.save(stock, value); }
      catch (err) { setError(err instanceof Error ? err.message : "保存失败"); }
      finally { setBusy(false); }
    }}>
      <label className="target-price-input"><input type="number" step="any" min="0" max="1000000000000000" required aria-label={`${stock.name}期望价格`} value={draft} onChange={event => { setDraft(event.target.value); setError(""); }} onKeyDown={event => { if (event.key === "Escape") { setDraft(row ? String(row.target_price) : ""); setError(""); } }} disabled={busy || !alerts.data || sending} /><span>{inputCurrency}</span></label>
      <span className="target-price-action-slot">
        {busy || sending ? <span className="target-price-progress" role="status" aria-label={sending ? "提醒发送中" : "保存中"}><Loader2 size={13} className="spin" /></span>
          : row && !hasDraft ? <button className="target-price-action clear" type="button" aria-label={`清除${stock.name}提醒`} title="清除提醒" onClick={() => void remove()}><X size={13} /></button>
          : hasDraft && (!row || changed) ? <button className="target-price-action confirm" type="submit" aria-label={saveLabel} title={saveTitle} disabled={!valid || !canSave}><Check size={14} /></button>
          : rearm ? <button className="target-price-action" type="submit" aria-label={saveLabel} title={saveTitle} disabled={!valid || !canSave}><RotateCcw size={13} /></button>
          : row ? <button className={`target-price-action active ${pending ? "pending" : ""}`} type="button" aria-label={`关闭${stock.name}提醒`} title={`${detail}；点击关闭提醒`} onClick={() => void remove()}><Bell size={13} /></button>
          : null}
      </span>
    </form>
    {currencyChanged && <small className="target-currency-note">当前报价 {stock.currency}，提醒待确认</small>}
    {error && <small className="danger" role="alert">{error}</small>}
  </div>;
}
