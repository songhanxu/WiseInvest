export type PriceAlert = {
  id: number; holding_id: string; account_id: string; asset_key?: string; code?: string;
  target_price: number; currency: string; direction: "above" | "below";
  trigger_mode: "auto" | "above" | "below"; revision: number;
  status: "armed" | "sending" | "sent" | "error"; last_error?: string;
  sent_at?: string; last_checked_at?: string | null;
};
export type AlertData = { alerts: PriceAlert[]; email: string; mailer_configured: boolean; check_interval_seconds: number };

export function savedAlertKey(row: PriceAlert) {
  return row.asset_key || (row.holding_id.startsWith("watchlist:") ? row.holding_id.slice(10) : undefined);
}
export function findPriceAlert(rows: PriceAlert[], holdingId: string, assetKey?: string) {
  return rows.find(row => assetKey && savedAlertKey(row) === assetKey) || rows.find(row => row.holding_id === holdingId);
}

// Shared by retained account/watchlist panels. Mutations invalidate older reads,
// so switching pages cannot restore a removed alert or an obsolete target.
export class PriceAlertState {
  private snapshot: { data: AlertData | null; error: string } = { data: null, error: "" };
  private listeners = new Set<() => void>();
  private version = 0;
  private pending: Promise<void> | null = null;
  lastAttemptAt = 0;
  read = () => this.snapshot;
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
  private publish(data: AlertData | null, error = "") { this.snapshot = { data, error }; this.listeners.forEach(fn => fn()); }
  refresh(loader: () => Promise<AlertData>) {
    if (this.pending) return this.pending;
    const version = this.version;
    this.lastAttemptAt = Date.now();
    const request = loader().then(data => {
      if (!Array.isArray(data.alerts)) throw new Error("提醒数据格式无效");
      if (version === this.version) this.publish(data);
    }).catch(error => {
      if (version === this.version) this.publish(this.snapshot.data, error instanceof Error ? error.message : "提醒读取失败");
    }).finally(() => { if (this.pending === request) this.pending = null; });
    this.pending = request;
    return request;
  }
  saved(row: PriceAlert) {
    this.version++; this.pending = null;
    const data = this.snapshot.data;
    if (data) this.publish({ ...data, alerts: [...data.alerts.filter(item => item.id !== row.id && item.holding_id !== row.holding_id && !(savedAlertKey(row) && savedAlertKey(item) === savedAlertKey(row))), row] });
  }
  removed(holdingId: string) {
    this.version++; this.pending = null;
    const data = this.snapshot.data;
    if (data) this.publish({ ...data, alerts: data.alerts.filter(row => row.holding_id !== holdingId) });
  }
  emailSaved(email: string) {
    this.version++; this.pending = null;
    if (this.snapshot.data) this.publish({ ...this.snapshot.data, email });
  }
}
const scopes = new Map<string, PriceAlertState>();
export function priceAlertState(apiBase: string, token: string) {
  const key = JSON.stringify([apiBase, token]);
  const state = scopes.get(key) || new PriceAlertState();
  scopes.delete(key); scopes.set(key, state);
  if (scopes.size > 8) scopes.delete(scopes.keys().next().value!);
  return state;
}
