import type { StockQuote } from "./App";

// Short reference descriptions, checked 2026-10-17. Company profiles were
// verified with WeStock Data; fund and crypto sources are linked below.
const introductions: Record<string, string> = {
  "us_stock:NVDA": "英伟达开发 GPU、加速计算平台及 CUDA 软件生态，产品用于人工智能、数据中心、图形处理和游戏。",
  "us_stock:RLAY": "Relay Therapeutics 是一家精准医疗公司，结合计算与实验方法研究蛋白质运动，开发针对肿瘤和遗传病的药物。",
  "us_stock:MU": "美光科技专注于内存与存储芯片，主要产品包括 DRAM 和 NAND 闪存，服务于数据中心、手机、汽车及工业等领域。",
  // https://www.vaneck.com/us/en/investments/semiconductor-etf-smh/overview/
  "us_stock:SMH": "VanEck 半导体 ETF，跟踪美国上市半导体 25 指数，覆盖半导体制造及设备公司。",
  // https://www.bnbchain.org/en/what-is-bnb
  "crypto:BNB": "BNB 是 BNB Chain 生态的原生代币，用于支付链上交易手续费、质押及参与生态应用。",
  // https://bitcoin.org/en/
  "crypto:BTC": "比特币是一种开源、去中心化的数字货币，通过点对点网络记录和验证交易，无需中央机构参与。",
};

export function assetIntroduction(stock: StockQuote, description?: string) {
  const code = stock.market === "crypto"
    ? (stock.symbol || stock.id).split(/[\/_-]/)[0].toUpperCase()
    : stock.id.toUpperCase();
  const text = introductions[`${stock.market}:${code}`] || description?.replace(/\s+/g, " ").trim();
  if (!text) return "暂无标的简介";
  return text.length > 240 ? `${text.slice(0, 239).trimEnd()}…` : text;
}
