type Quote = { market?: string; source?: string; currency?: string; change_basis?: string; product_type?: string; quote_time?: string; quote_session?: string; quote_stale?: boolean };

type Row = Quote & { id: string; market: string; name: string; current_price: number; quote_available?: boolean; quote_stale?: boolean };

export function mergeQuoteRows<T extends Row>(previous: T[], incoming: T[]): T[] {
  const cached = new Map(previous.map(row => [`${row.market}:${row.id}`, row]));
  return incoming.map(row => {
    const old = cached.get(`${row.market}:${row.id}`);
    if ((row.quote_available === false || !(row.current_price > 0)) && old && old.current_price > 0) {
      return { ...old, name: row.name || old.name, quote_stale: true };
    }
    return { ...row, quote_stale: row.quote_stale === true };
  });
}

export function quoteCurrency(quote: Quote) {
  return quote.currency || ({ a_share: "CNY", hk_stock: "HKD", crypto: "USDT" } as Record<string, string>)[quote.market || ""] || "USD";
}

export function quoteSourceLabel(quote: Quote) {
  if (quote.source === "bitget") return `Bitget${quote.change_basis === "24h" || quote.product_type !== "equity" ? " · 24h" : ""}`;
  if (quote.source === "tencent") return "腾讯 · 股票";
  if (quote.source === "binance") return "Binance · 24h";
  return quote.change_basis === "24h" ? "24h 行情" : "";
}

export function quoteTimeLabel(quote: Quote) {
  if (!quote.quote_time) return "";
  const raw = quote.quote_time;
  const stamp = /^\d{10,13}$/.test(raw) ? Number(raw) * (raw.length === 10 ? 1000 : 1) : Date.parse(raw);
  if (!Number.isFinite(stamp) || stamp <= 0) return "";
  const usEquity = quote.market === "us_stock" && quote.source === "tencent";
  const parts = new Intl.DateTimeFormat("en-GB", { timeZone: usEquity ? "America/New_York" : "Asia/Shanghai", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).formatToParts(stamp);
  const part = (type: string) => parts.find(item => item.type === type)?.value;
  return `${usEquity ? "美东" : "北京"} ${part("month")}/${part("day")} ${part("hour")}:${part("minute")}`;
}

export function quoteStatusLabel(quote: Quote, now = Date.now()) {
  const source = ({ bitget: "Bitget", tencent: "腾讯", binance: "Binance" } as Record<string, string>)[quote.source || ""] || "行情";
  const time = quoteTimeLabel(quote);
  if (!time) return `${source} · ${quote.quote_stale ? "缓存，" : ""}报价时间未提供`;
  const stamp = /^\d{10,13}$/.test(quote.quote_time!) ? Number(quote.quote_time) * (quote.quote_time!.length === 10 ? 1000 : 1) : Date.parse(quote.quote_time!);
  const shortTime = time.replace(/^(美东|北京) /, "");
  const close = quote.quote_session === "close" || (quote.source === "tencent" && quote.market === "us_stock" && time.endsWith("16:00"));
  if (close) return `${source} · ${shortTime.slice(0, 5)} 收盘${quote.quote_stale ? " · 缓存" : ""}`;
  if (quote.quote_stale) return `${source} · ${shortTime} 缓存`;
  const recent = now - stamp <= 60000 && stamp <= now + 60000;
  if (recent && (quote.source === "bitget" || quote.source === "binance")) return `${source} · 实时 ${shortTime.slice(-5)}`;
  return `${source} · ${shortTime} 报价`;
}
