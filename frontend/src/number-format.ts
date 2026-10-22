export function formatMarketNumber(value: number) {
  if (!Number.isFinite(value)) return "—";
  const amount = Math.abs(value);
  const digits = amount > 0 && amount < 1 ? Math.min(12, Math.max(4, 2 - Math.floor(Math.log10(amount)))) : 2;
  return new Intl.NumberFormat("zh-CN", { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(value);
}
