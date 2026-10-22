type ProductQuote = { source?: string; provider_symbol?: string; product_type?: string };
type ProductIdentity = { provider_symbol?: string; category?: "SPOT" | "USDT-FUTURES" };

// Crypto spot uses the market endpoint, which supports crypto discovery and
// fallback. The product endpoint currently only discovers stock instruments.
export function quoteProductIdentity(quote?: ProductQuote): ProductIdentity {
  if (quote?.source !== "bitget" || !quote.provider_symbol) return {};
  if (quote.product_type === "stock_token") return { provider_symbol: quote.provider_symbol, category: "SPOT" };
  if (["stock_perpetual", "quanto_perpetual", "crypto_perpetual", "other_perpetual"].includes(quote.product_type || "")) {
    return { provider_symbol: quote.provider_symbol, category: "USDT-FUTURES" };
  }
  return {};
}

export function quoteProductQuery(quote?: ProductQuote) {
  const product = quoteProductIdentity(quote);
  return product.provider_symbol && product.category ? `&${new URLSearchParams({ provider_symbol: product.provider_symbol, category: product.category })}` : "";
}
