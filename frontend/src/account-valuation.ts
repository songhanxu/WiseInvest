export type ExchangeRates = { rates: Record<string, number>; asOf: number };
type ValuedHolding = { account_id?: string; holding_type: string; market_value?: string; currency?: string };
export type AccountEquity = { account_id: string; amount: string; currency: string };

export const holdingNumber = (value?: string | number | null) => {
  if (value == null || String(value).trim() === "" || !Number.isFinite(Number(value))) return "—";
  return new Intl.NumberFormat("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(Number(value));
};
export const holdingQuantity = (value: string) => {
  const amount = Number(value);
  if (Number.isFinite(amount) && amount !== 0 && Math.abs(amount) < 0.01) return amount > 0 ? "< 0.01" : "> -0.01";
  return holdingNumber(value);
};

// Sum equity/balances, never contract notional. USDT/USD is a nominal 1:1
// estimate; other fiat currencies require an observed daily exchange rate.
export function accountValuation(holdings: ValuedHolding[], fx: ExchangeRates | null, equities: AccountEquity[] = [], complete = true) {
  let usdt = 0, valued = 0, missing = 0, futures = 0;
  const covered = new Set<string>();
  for (const equity of equities) {
    if (covered.has(equity.account_id) || !equity.amount.trim() || !Number.isFinite(Number(equity.amount)) || !["USDT", "USD"].includes(equity.currency)) continue;
    usdt += Number(equity.amount); covered.add(equity.account_id); valued++;
  }
  for (const row of holdings) {
    if (row.holding_type === "futures") { futures++; continue; }
    if (row.account_id && covered.has(row.account_id)) continue;
    const amount = row.market_value?.trim();
    const currency = row.currency?.toUpperCase();
    const rate = currency === "USDT" || currency === "USD" ? 1 : currency ? fx?.rates[currency] : undefined;
    if (!amount || !Number.isFinite(Number(amount)) || !rate || !Number.isFinite(rate) || rate <= 0) { missing++; continue; }
    usdt += Number(amount) / rate; valued++;
  }
  const available = valued > 0 || holdings.length === 0 && complete;
  const cnyRate = fx?.rates.CNY;
  return { usdt: available ? usdt : null, cny: available && cnyRate && Number.isFinite(cnyRate) && cnyRate > 0 ? usdt * cnyRate : null, missing, futures };
}

let cachedRates: ExchangeRates | null = null;
let fetchedAt = 0;
let pendingRates: Promise<ExchangeRates> | null = null;
export function loadExchangeRates(): Promise<ExchangeRates> {
  if (cachedRates && Date.now() - fetchedAt < 24 * 60 * 60 * 1000) return Promise.resolve(cachedRates);
  if (pendingRates) return pendingRates;
  // Public daily FX only: no account data, token or holdings leave this app.
  pendingRates = fetch("https://open.er-api.com/v6/latest/USD", { signal: AbortSignal.timeout(8000), credentials: "omit", referrerPolicy: "no-referrer" })
    .then(async response => {
      if (!response.ok) throw new Error("汇率暂不可用");
      const data = await response.json();
      const asOf = Number(data.time_last_update_unix) * 1000;
      if (data.result !== "success" || data.base_code !== "USD" || !Number.isFinite(asOf) || asOf <= 0 || asOf > Date.now() + 60000 || Date.now() - asOf > 7 * 86400000 || !Number.isFinite(data.rates?.CNY) || data.rates.CNY <= 0) throw new Error("汇率数据无效");
      cachedRates = { rates: data.rates, asOf }; fetchedAt = Date.now();
      return cachedRates;
    }).finally(() => { pendingRates = null; });
  return pendingRates;
}
