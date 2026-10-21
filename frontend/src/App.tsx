import { FormEvent, PointerEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { WatchlistPanel } from "./WatchlistPanel";
import { AccountsPanel } from "./AccountsPanel";
import { ChatMarkdown } from "./ChatMarkdown";
import { RoundtableIcon, RoundtableView } from "./RoundtableView";
import { roundtableAgents, roundtableWelcome } from "./research";
import { StockDeepResearch, StockNews } from "./StockDeepResearch";
import { useDismissiblePopover } from "./useDismissiblePopover";
import { mergeQuoteRows, quoteCurrency, quoteTimeLabel, quoteStatusLabel } from "./quote-presentation";
import { createConversationDraft, hasUserMessage, readConversationHistory, serializeConversationHistory } from "./conversation-history";
import type { Message, SavedConversation } from "./conversation-history";
import type { LucideIcon } from "lucide-react";
import {
  Activity,
  BarChart3,
  Building2,
  ListPlus,
  Bell,
  Bitcoin,
  Bot,
  CheckCircle2,
  ChevronRight,
  CircleDollarSign,
  Command,
  Compass,
  Home,
  KeyRound,
  Layers3,
  Loader2,
  MessageCircle,
  Newspaper,
  Radio,
  Search,
  Send,
  Settings,
  ShieldCheck,
  Sparkles,
  Trash2,
  TrendingDown,
  TrendingUp,
  UserCircle,
  WalletCards,
  Zap
} from "lucide-react";

const DEFAULT_API_BASE = "http://119.91.58.30";

type Tab = "home" | "accounts" | "watchlist" | "chat" | "profile";
type View = Tab | "market" | "stock";
export type MarketId = "a_share" | "us_stock" | "hk_stock" | "crypto";
type KLinePeriod = "5m" | "15m" | "30m" | "1h" | "4h" | "1d" | "1w";

export type Market = {
  id: MarketId;
  displayName: string;
  title: string;
  subtitle: string;
  description: string;
  scope: string[];
  welcome: string;
  colors: [string, string];
  icon: LucideIcon;
  featured: string[];
};

type IndexQuote = {
  id: string;
  name: string;
  short_name?: string;
  value: number;
  change: number;
  change_percent: number;
  sparkline_data?: number[];
  source?: string;
  currency?: string;
  product_type?: string;
  quote_time?: string;
  quote_session?: string;
};

export type StockQuote = {
  id: string;
  symbol: string;
  name: string;
  market: string;
  current_price: number;
  change: number;
  change_percent: number;
  volume: number;
  high: number;
  low: number;
  open: number;
  previous_close: number;
  source?: string;
  provider_symbol?: string;
  currency?: string;
  product_type?: string;
  change_basis?: string;
  quote_available?: boolean;
  quote_stale?: boolean;
  quote_time?: string;
  quote_session?: string;
};

export type KLinePoint = {
  date: string;
  open: number;
  close: number;
  high: number;
  low: number;
  volume: number;
};

export type AnalysisResponse = {
  type: string;
  conclusion: "bullish" | "bearish" | "neutral" | string;
  summary: string;
  detail: string;
};

export type NewsItem = {
  id: string;
  title: string;
  source: string;
  time: string;
  summary: string;
  analysis?: string;
  sentiment: "positive" | "negative" | "neutral" | string;
  url: string;
};

const kLinePeriodLabels: Record<KLinePeriod, string> = {
  "5m": "5分",
  "15m": "15分",
  "30m": "30分",
  "1h": "1时",
  "4h": "4时",
  "1d": "日K",
  "1w": "周K"
};

const allKLinePeriods: KLinePeriod[] = ["5m", "15m", "30m", "1h", "4h", "1d", "1w"];

function availableKLinePeriods(marketId: MarketId): KLinePeriod[] {
  if (marketId === "us_stock" || marketId === "hk_stock") return allKLinePeriods;
  if (marketId === "a_share") return ["5m", "15m", "30m", "1h", "1d", "1w"];
  return allKLinePeriods;
}

function defaultKLineLimit(period: KLinePeriod) {
  if (period === "15m" || period === "1d") return period === "15m" ? 160 : 150;
  return 120;
}

const markets: Market[] = [
  {
    id: "a_share",
    displayName: "A 股",
    title: "A 股 Agent",
    subtitle: "沪深北交所",
    description: "政策、行业、资金与技术形态的综合研判。",
    scope: ["个股拆解", "行业轮动", "政策解读", "技术形态"],
    welcome:
      "你好！我是你的 A 股投资分析助手。\n\n我可以帮你分析个股、行业趋势、技术形态、基本面数据以及政策影响。\n\n有什么想聊的？",
    colors: ["#E95555", "#FF8A8A"],
    icon: BarChart3,
    featured: ["600519", "300750", "000858"]
  },
  {
    id: "us_stock",
    displayName: "美 股",
    title: "美股 Agent",
    subtitle: "NYSE · NASDAQ",
    description: "财报、成长性、利率周期与全球风险偏好的联动分析。",
    scope: ["财报解读", "宏观因子", "成长股", "估值比较"],
    welcome:
      "Hi！我是你的美股投资分析助手。\n\n我可以帮你研究美股个股、解读财报、分析宏观经济数据以及美联储政策影响。\n\n想聊哪只股票？",
    colors: ["#4A90E2", "#7BB7FF"],
    icon: CircleDollarSign,
    featured: ["AAPL", "NVDA", "TSLA"]
  },
  {
    id: "hk_stock",
    displayName: "港 股",
    title: "港股 Agent",
    subtitle: "香港市场 · HKD 报价",
    description: "跟踪港股行情，结合公司财报与估值研究。",
    scope: ["港股研究", "财报解读", "行情跟踪", "估值比较"],
    welcome: "你好！我是你的港股投资分析助手。可以帮你研究香港上市公司、公司财报与宏观风险。行情来自 Bitget，按接口返回的币种报价。",
    colors: ["#8B5CF6", "#C4B5FD"],
    icon: Building2,
    featured: ["00700", "01810", "00992"]
  },
  {
    id: "crypto",
    displayName: "币 圈",
    title: "Crypto Agent",
    subtitle: "BTC · ETH · DeFi",
    description: "现货、合约、链上数据和风险敞口的快速判断。",
    scope: ["趋势结构", "链上数据", "DeFi", "风险控制"],
    welcome:
      "你好！我是你的加密货币分析助手。\n\n我可以分析 BTC/ETH 走势、合约机会、链上数据、DeFi 协议以及加密市场结构。\n\n有什么想分析的？",
    colors: ["#F59E0B", "#F97316"],
    icon: Bitcoin,
    featured: ["BTC", "ETH", "SOL"]
  }
];

const promptDeck: Record<MarketId, string[]> = {
  a_share: ["帮我分析一下今天 A 股的主线和风险", "宁德时代现在更适合趋势还是波段？", "给我一个白酒板块的多空框架"],
  us_stock: ["复盘一下纳指走势和利率预期", "NVDA 当前估值是否透支？", "帮我比较 Apple 和 Microsoft 的防御性"],
  hk_stock: ["腾讯控股的估值和增长前景如何？", "小米集团最新财报有哪些关键变化？", "港股当前有哪些值得关注的行业？"],
  crypto: ["BTC 现在的关键支撑和压力在哪里？", "ETH 和 SOL 谁的短线弹性更高？", "帮我做一份合约风险提示"]
};

const storage = {
  apiBase: "wiseinvest.apiBase",
  token: "wiseinvest.token",
  conversations: "wiseinvest.conversations"
};

function uid() {
  return crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`;
}

function getSavedConversations(): SavedConversation[] {
  return readConversationHistory(localStorage.getItem(storage.conversations));
}

function saveConversations(conversations: SavedConversation[]) {
  localStorage.setItem(storage.conversations, serializeConversationHistory(conversations));
}

export async function requestJson<T>(apiBase: string, path: string, token: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(`${apiBase}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...init.headers
    }
  });

  if (!res.ok) {
    const text = await res.text();
    let message = text || `HTTP ${res.status}`;
    try {
      const body: { error?: unknown; message?: unknown } = JSON.parse(text);
      if (typeof body?.error === "string") message = body.error;
      else if (typeof body?.message === "string") message = body.message;
    } catch { /* Plain-text errors remain readable. */ }
    throw new Error(message);
  }

  return res.json() as Promise<T>;
}

async function healthCheck(apiBase: string): Promise<boolean> {
  try {
    const res = await fetch(`${apiBase}/health`, { cache: "no-store" });
    return res.ok;
  } catch {
    return false;
  }
}

async function fetchIndices(apiBase: string, marketId: MarketId) {
  return requestJson<IndexQuote[]>(apiBase, `/api/v1/stocks/indices?market=${marketId}`, "", {
    method: "GET"
  });
}

async function fetchWatchlist(apiBase: string, token: string, marketId: MarketId) {
  return requestJson<StockQuote[]>(apiBase, `/api/v1/stocks/watchlist?market=${marketId}`, token, {
    method: "GET"
  });
}

async function searchStocks(apiBase: string, marketId: MarketId, query = "") {
  return requestJson<StockQuote[]>(apiBase, `/api/v1/stocks/search?market=${marketId}&q=${encodeURIComponent(query)}`, "", {
    method: "GET"
  });
}

function productQuery(stock?: StockQuote) {
 return stock?.source === "bitget" && stock.provider_symbol ? `&provider_symbol=${encodeURIComponent(stock.provider_symbol)}&category=${stock.product_type === "stock_token" ? "SPOT" : "USDT-FUTURES"}` : "";
}

async function fetchQuote(apiBase: string, marketId: MarketId, code: string, product?: StockQuote) {
  return requestJson<StockQuote>(apiBase, `/api/v1/stocks/quote?market=${marketId}&code=${encodeURIComponent(code)}${productQuery(product)}`, "", {
    method: "GET"
  });
}

async function fetchKline(apiBase: string, marketId: MarketId, code: string, period: KLinePeriod = "1d", limit = defaultKLineLimit(period), product?: StockQuote) {
  return requestJson<KLinePoint[]>(
    apiBase,
    `/api/v1/stocks/kline?market=${marketId}&code=${encodeURIComponent(code)}&period=${period}&limit=${limit}${productQuery(product)}`,
    "",
    { method: "GET" }
  );
}

async function fetchAnalysis(apiBase: string, stock: StockQuote, marketId: MarketId, signal?: AbortSignal) {
  // The cache-only endpoint has no product identity; avoid reusing a stock's
  // analysis for a different Bitget trading product.
  if (!stock.provider_symbol || stock.source !== "bitget") {
    try {
      const cached = await requestJson<{ cached: boolean; analysis?: AnalysisResponse }>(apiBase, `/api/v1/stocks/analysis/cached?${new URLSearchParams({ code: stock.id, market: marketId })}`, "", { signal });
      if (cached.cached && cached.analysis?.detail) return cached.analysis;
    } catch (err) { if (signal?.aborted) throw err; }
  }
  return requestJson<AnalysisResponse>(apiBase, "/api/v1/stocks/analysis/enhance", "", {
    method: "POST",
    signal,
    body: JSON.stringify({
      code: stock.id,
      provider_symbol: stock.source === "bitget" ? stock.provider_symbol : undefined,
      category: stock.source === "bitget" && stock.provider_symbol ? stock.product_type === "stock_token" ? "SPOT" : "USDT-FUTURES" : undefined,
      market: marketId,
      name: stock.name,
      type: "comprehensive",
      price_summary: `${stock.name} ${stock.symbol} 当前价 ${stock.current_price}，涨跌幅 ${stock.change_percent}%`
    })
  });
}

async function fetchNews(apiBase: string, stock: StockQuote, marketId: MarketId, signal?: AbortSignal) {
  return requestJson<NewsItem[]>(
    apiBase,
    `/api/v1/stocks/news?market=${marketId}&code=${encodeURIComponent(stock.id)}&name=${encodeURIComponent(stock.name)}`,
    "",
    { method: "GET", signal }
  );
}

async function createConversation(apiBase: string, token: string, market: Market) {
  return requestJson<{ id: number }>(apiBase, "/api/v1/conversations", token, {
    method: "POST",
    body: JSON.stringify({
      agent_type: market.id,
      title: `${market.title} Conversation`
    })
  });
}

async function devLogin(apiBase: string) {
  return requestJson<{ token: string; user: { display_name?: string; username?: string } }>(
    apiBase,
    "/api/v1/auth/wechat/login",
    "",
    {
      method: "POST",
      body: JSON.stringify({ code: "MOCK_WEB_LOGIN" })
    }
  );
}

async function streamMessage(
  apiBase: string,
  token: string,
  conversationId: number,
  content: string,
  onChunk: (chunk: { type?: string; content?: string; error?: string }) => void
) {
  const res = await fetch(`${apiBase}/api/v1/messages/stream`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
      Authorization: `Bearer ${token}`
    },
    body: JSON.stringify({ conversation_id: conversationId, content })
  });

  if (!res.ok || !res.body) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      if (!line.startsWith("data: ")) continue;
      const payload = line.slice(6).trim();
      if (!payload || payload === "[DONE]") continue;
      onChunk(JSON.parse(payload));
    }
  }
}

function useMarketPulse(apiBase: string, isActive: boolean) {
  const emptyIndices = () => ({ a_share: [], us_stock: [], hk_stock: [], crypto: [] }) as Record<MarketId, IndexQuote[]>;
  const [indices, setIndices] = useState(emptyIndices);
  const [loadingByMarket, setLoadingByMarket] = useState<Record<MarketId, boolean>>({ a_share: true, us_stock: true, hk_stock: true, crypto: true });
  const [errors, setErrors] = useState<Partial<Record<MarketId, string>>>({});
  const activeRef = useRef(isActive); activeRef.current = isActive;
  const requests = useRef(new Map<MarketId, AbortController>());
  const scopeVersion = useRef(0);

  const refresh = useCallback(() => {
    const version = scopeVersion.current;
    for (const market of markets) {
      if (requests.current.has(market.id)) continue;
      const controller = new AbortController();
      requests.current.set(market.id, controller);
      const timeout = window.setTimeout(() => controller.abort(), 12000);
      setLoadingByMarket(previous => ({ ...previous, [market.id]: true }));
      void requestJson<IndexQuote[]>(apiBase, `/api/v1/stocks/indices?market=${market.id}`, "", { signal: controller.signal })
        .then(rows => {
          if (version !== scopeVersion.current) return;
          if (!Array.isArray(rows) || !rows.length) throw new Error("该市场行情源暂不可用");
          setIndices(previous => ({ ...previous, [market.id]: rows }));
          setErrors(previous => ({ ...previous, [market.id]: "" }));
        })
        .catch(() => { if (version === scopeVersion.current) setErrors(previous => ({ ...previous, [market.id]: "行情暂不可用" })); })
        .finally(() => {
          window.clearTimeout(timeout);
          if (version !== scopeVersion.current) return;
          requests.current.delete(market.id);
          setLoadingByMarket(previous => ({ ...previous, [market.id]: false }));
        });
    }
  }, [apiBase]);

  useEffect(() => {
    setIndices(emptyIndices()); setErrors({});
    setLoadingByMarket({ a_share: true, us_stock: true, hk_stock: true, crypto: true });
    void refresh();
    const timer = window.setInterval(() => { if (activeRef.current && !document.hidden) refresh(); }, 30000);
    return () => {
      scopeVersion.current++;
      for (const controller of requests.current.values()) controller.abort();
      requests.current.clear();
      window.clearInterval(timer);
    };
  }, [refresh]);
  useEffect(() => { if (isActive) refresh(); }, [isActive, refresh]);
  const loading = Object.values(loadingByMarket).some(Boolean);
  const error = Object.values(errors).some(Boolean) ? "部分市场行情暂不可用" : null;
  return { indices, loading, loadingByMarket, errors, error };
}

function useMarketBoard(apiBase: string, token: string, market: Market, isActive = true) {
  const [watchlist, setWatchlist] = useState<StockQuote[]>([]);
  const [popular, setPopular] = useState<StockQuote[]>([]);
  const [loading, setLoading] = useState(true);
  const [hasLoaded, setHasLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const activeRef = useRef(isActive); activeRef.current = isActive;
  const requestVersion = useRef(0);
  const inFlight = useRef(false);

  const refresh = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    const version = ++requestVersion.current;
    setLoading(true);
    setError(null);
    const [watchlistResult, popularResult] = await Promise.allSettled([
      token ? fetchWatchlist(apiBase, token, market.id) : Promise.resolve([] as StockQuote[]),
      searchStocks(apiBase, market.id, "")
    ]);
    if (version !== requestVersion.current) return;
    // Refresh each source independently. A failed request must not erase cached rows.
    if (watchlistResult.status === "fulfilled") setWatchlist(previous => mergeQuoteRows(previous, watchlistResult.value));
    else setWatchlist(previous => previous.map(row => ({ ...row, quote_stale: true })));
    if (popularResult.status === "fulfilled") setPopular(previous => mergeQuoteRows(previous, popularResult.value));
    else setPopular(previous => previous.map(row => ({ ...row, quote_stale: true })));
    if (watchlistResult.status === "fulfilled" || popularResult.status === "fulfilled") setHasLoaded(true);
    const failures = [watchlistResult, popularResult].flatMap(result => result.status === "rejected" ? [result.reason instanceof Error ? result.reason.message : "市场数据加载失败"] : result.value.some(row => row.quote_available === false || !(row.current_price > 0)) ? ["部分标的行情暂不可用"] : []);
    setError(failures.length ? [...new Set(failures)].join("；") : null);
    inFlight.current = false;
    setLoading(false);
  }, [apiBase, token, market.id]);

  useEffect(() => {
    setWatchlist([]); setPopular([]); setHasLoaded(false); setError(null);
    if (activeRef.current) void refresh();
    return () => {
      requestVersion.current++; inFlight.current = false;
    };
  }, [refresh]);

  useEffect(() => { if (isActive) void refresh(); }, [isActive, refresh]);
  useEffect(() => {
    const timer = window.setInterval(() => { if (activeRef.current && !document.hidden) void refresh(); }, 30000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  return { watchlist, popular, loading, hasLoaded, error };
}

function useStockDetail(apiBase: string, market: Market, stock: StockQuote | null, period: KLinePeriod) {
  const [quote, setQuote] = useState<StockQuote | null>(stock);
  const [kline, setKline] = useState<KLinePoint[]>([]);
  const [analysis, setAnalysis] = useState<AnalysisResponse | null>(null);
  const [news, setNews] = useState<NewsItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingMoreKline, setLoadingMoreKline] = useState(false);
  const [analysisLoading, setAnalysisLoading] = useState(false);
  const [analysisError, setAnalysisError] = useState("");
  const [analysisRevision, setAnalysisRevision] = useState(0);
  const [newsLoading, setNewsLoading] = useState(false);
  const [newsError, setNewsError] = useState("");
  const [newsRevision, setNewsRevision] = useState(0);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!stock) return;
    let cancelled = false;
    setQuote(stock);
    setKline([]);
    setLoading(true);
    setError(null);

    Promise.allSettled([
      fetchQuote(apiBase, market.id, stock.id, stock),
      fetchKline(apiBase, market.id, stock.id, period, defaultKLineLimit(period), stock)
    ]).then((results) => {
      if (cancelled) return;
      if (results[0].status === "fulfilled") setQuote(results[0].value);
      if (results[1].status === "fulfilled" && Array.isArray(results[1].value)) setKline(results[1].value);
      if (results.every((result) => result.status === "rejected")) setError("股票行情加载失败");
      setLoading(false);
    });

    return () => {
      cancelled = true;
    };
  }, [apiBase, market.id, period, stock?.id, stock?.provider_symbol, stock?.product_type]);

  useEffect(() => {
    if (!stock) return;
    let cancelled = false;
    const controller = new AbortController();
    setAnalysis(null);
    setAnalysisError("");
    setAnalysisLoading(true);

    fetchAnalysis(apiBase, stock, market.id, controller.signal)
      .then((result) => {
        if (!cancelled) setAnalysis(result);
      })
      .catch((err) => {
        if (!cancelled) {
          setAnalysisError(err instanceof Error ? err.message : "AI 分析暂不可用");
        }
      })
      .finally(() => {
        if (!cancelled) setAnalysisLoading(false);
      });

    return () => { cancelled = true; controller.abort(); };
  }, [apiBase, market.id, stock?.id, stock?.provider_symbol, stock?.product_type, analysisRevision]);

  useEffect(() => {
    if (!stock) return;
    let cancelled = false;
    const controller = new AbortController();
    setNews([]); setNewsLoading(true); setNewsError("");
    fetchNews(apiBase, stock, market.id, controller.signal)
      .then((result) => {
        if (!cancelled) setNews(result);
      })
      .catch((err) => {
        if (!cancelled) { setNews([]); setNewsError(err instanceof Error ? err.message : "资讯加载失败"); }
      })
      .finally(() => {
        if (!cancelled) setNewsLoading(false);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [apiBase, market.id, stock?.id, stock?.provider_symbol, stock?.product_type, newsRevision]);

  const loadMoreKline = useCallback(() => {
    if (!stock || loadingMoreKline) return;
    setLoadingMoreKline(true);
    fetchKline(apiBase, market.id, stock.id, period, Math.max(kline.length + 500, defaultKLineLimit(period) + 500), stock)
      .then((result) => {
        if (Array.isArray(result) && result.length > kline.length) setKline(result);
      })
      .catch(() => undefined)
      .finally(() => setLoadingMoreKline(false));
  }, [apiBase, kline.length, loadingMoreKline, market.id, period, stock]);

  return { quote, kline, analysis, analysisError, reloadAnalysis: () => setAnalysisRevision(value => value + 1), news, newsError, reloadNews: () => setNewsRevision(value => value + 1), loading, loadingMoreKline, analysisLoading, newsLoading, error, loadMoreKline };
}

function usePreventPageZoom() {
  useEffect(() => {
    const preventGesture = (event: Event) => {
      event.preventDefault();
    };

    const preventZoomWheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
    };

    const preventZoomKeys = (event: KeyboardEvent) => {
      if (!event.ctrlKey && !event.metaKey) return;
      if (["+", "-", "=", "_", "0"].includes(event.key)) {
        event.preventDefault();
      }
    };

    document.addEventListener("gesturestart", preventGesture);
    document.addEventListener("gesturechange", preventGesture);
    document.addEventListener("gestureend", preventGesture);
    window.addEventListener("wheel", preventZoomWheel, { passive: false });
    window.addEventListener("keydown", preventZoomKeys);

    return () => {
      document.removeEventListener("gesturestart", preventGesture);
      document.removeEventListener("gesturechange", preventGesture);
      document.removeEventListener("gestureend", preventGesture);
      window.removeEventListener("wheel", preventZoomWheel);
      window.removeEventListener("keydown", preventZoomKeys);
    };
  }, []);
}

export function App() {
  const [view, setView] = useState<View>("home");
  const [watchlistVisited, setWatchlistVisited] = useState(false);
  const [apiBase, setApiBase] = useState(() => localStorage.getItem(storage.apiBase) || DEFAULT_API_BASE);
  const [token, setToken] = useState(() => localStorage.getItem(storage.token) || "");
  const [conversations, setConversations] = useState<SavedConversation[]>(getSavedConversations);
  const lastSavedHistory = useRef(serializeConversationHistory(conversations));
  const [conversationDraft, setConversationDraft] = useState<SavedConversation | null>(null);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [activeMarketId, setActiveMarketId] = useState<MarketId>("a_share");
  const [activeStock, setActiveStock] = useState<StockQuote | null>(null);
  const [stockReturnView, setStockReturnView] = useState<"market" | "watchlist" | "accounts">("market");
  const [draftPrompt, setDraftPrompt] = useState("");
  const [isOnline, setIsOnline] = useState<boolean | null>(null);
  const marketPulse = useMarketPulse(apiBase, view === "home" || view === "market");

  const activeConversation = conversations.find((item) => item.id === activeId)
    ?? (conversationDraft?.id === activeId ? conversationDraft : undefined);
  const activeMarket = markets.find((market) => market.id === activeMarketId) ?? markets[0];
  const selectedMarket = activeConversation
    ? markets.find((market) => market.id === activeConversation.marketId) ?? markets[0]
    : activeMarket;

  usePreventPageZoom();

  useEffect(() => { window.scrollTo(0, 0); }, [view, activeStock?.id, activeStock?.provider_symbol]);
  useEffect(() => { if (view === "watchlist") setWatchlistVisited(true); }, [view]);

  useEffect(() => {
    localStorage.setItem(storage.apiBase, apiBase);
  }, [apiBase]);

  useEffect(() => {
    localStorage.setItem(storage.token, token);
  }, [token]);

  useEffect(() => {
    const serialized = serializeConversationHistory(conversations);
    if (serialized !== lastSavedHistory.current) {
      lastSavedHistory.current = serialized;
      saveConversations(conversations);
    }
  }, [conversations]);

  useEffect(() => {
    const syncHistory = (event: StorageEvent) => {
      if (event.key !== storage.conversations || event.storageArea !== localStorage) return;
      const next = readConversationHistory(event.newValue);
      lastSavedHistory.current = serializeConversationHistory(next);
      setConversations(next);
    };
    window.addEventListener("storage", syncHistory);
    return () => window.removeEventListener("storage", syncHistory);
  }, []);

  useEffect(() => {
    healthCheck(apiBase).then(setIsOnline);
  }, [apiBase]);

  const openMarketBoard = (market: Market) => {
    setActiveMarketId(market.id);
    setActiveStock(null);
    setView("market");
  };

  const openStock = (market: Market, stock: StockQuote, returnView: "market" | "watchlist" | "accounts" = "market") => {
    setStockReturnView(returnView);
    setActiveMarketId(market.id);
    setActiveStock(stock);
    setView("stock");
  };

  const openAgentChat = (market: Market, seed?: string) => {
    const existing = conversations.find((item) => !item.kind && item.marketId === market.id);
    const conversation = existing
      ?? (!conversationDraft?.kind && conversationDraft?.marketId === market.id ? conversationDraft : createConversationDraft(market, uid));
    if (!existing) setConversationDraft(conversation);
    if (seed !== undefined || activeId !== conversation.id) setDraftPrompt(seed ?? "");
    setActiveId(conversation.id);
    setActiveMarketId(market.id);
    setView("chat");
  };

  const openRoundtable = (seed?: string) => {
    const existing = conversations.find(item => item.kind === "roundtable");
    const conversation: SavedConversation = existing ?? (conversationDraft?.kind === "roundtable" ? conversationDraft : {
      id: uid(), marketId: "a_share", kind: "roundtable", title: "", updatedAt: new Date().toISOString(),
      messages: [{ id: uid(), role: "assistant", content: roundtableWelcome }]
    });
    if (!existing) setConversationDraft(conversation);
    if (seed !== undefined || activeId !== conversation.id) setDraftPrompt(seed ?? "");
    setActiveId(conversation.id); setView("chat");
  };

  const updateConversation = (next: SavedConversation) => {
    if (!hasUserMessage(next)) return;
    setConversationDraft((prev) => prev?.id === next.id ? null : prev);
    setConversations((prev) =>
      [next, ...prev.filter((item) => item.id !== next.id)].sort(
        (a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()
      )
    );
  };

  const deleteConversation = (id: string) => {
    setConversations((prev) => prev.filter((item) => item.id !== id));
    if (activeId === id) {
      setActiveId(null);
      setDraftPrompt("");
    }
  };

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">
            <BarChart3 size={22} />
          </div>
          <div>
            <h1>慧投</h1>
          <p>投资研究工作台</p>
          </div>
        </div>

        <p className="nav-caption">工作空间</p>
        <nav className="nav-list">
          <NavButton active={view === "home" || view === "market" || (view === "stock" && stockReturnView === "market")} icon={Home} label="总览" onClick={() => setView("home")} />
          <NavButton active={view === "accounts" || (view === "stock" && stockReturnView === "accounts")} icon={WalletCards} label="实体账户" onClick={() => setView("accounts")} />
          <NavButton active={view === "watchlist" || (view === "stock" && stockReturnView === "watchlist")} icon={ListPlus} label="自选列表" onClick={() => setView("watchlist")} />
          <NavButton active={view === "chat"} icon={MessageCircle} label="AI 分析" onClick={() => setView("chat")} />
          <NavButton active={view === "profile"} icon={UserCircle} label="设置" onClick={() => setView("profile")} />
        </nav>

        <div className="sidebar-panel">
          <div className="panel-title">
            <Radio size={15} />
            服务状态
          </div>
          <StatusLine label="数据连接" ok={isOnline === true} pending={isOnline === null} />
          <StatusLine label="账户登录" ok={Boolean(token)} />
          <StatusLine label="市场行情" ok={!marketPulse.error} pending={marketPulse.loading} />
        </div>

        <div className="sidebar-footer">
          <ShieldCheck size={15} />
          <span>专注研究，审慎决策</span>
        </div>
      </aside>

      <main className="main">
        <TopBar
          key={`${view}:${activeMarketId}:${activeStock?.id ?? ""}:${activeStock?.provider_symbol ?? ""}:${activeId ?? ""}:${apiBase}`}
          pageTitle={{home:"总览",accounts:"实体账户",watchlist:"自选列表",chat:"AI 分析",profile:"设置",market:activeMarket.displayName,stock:activeStock?.name ?? "个股详情"}[view]}
          apiBase={apiBase}
          isOnline={isOnline}
          token={token}
          onOpenSettings={() => setView("profile")}
          onOpenStock={openStock}
        />

        <div hidden={view !== "home"}>
          <HomePanel
            key={`${apiBase}:${token}`}
            isActive={view === "home"}
            apiBase={apiBase}
            token={token}
            conversations={conversations}
            marketPulse={marketPulse}
            onOpenMarket={openMarketBoard}
            onOpenStock={openStock}
            onOpenConversation={(id) => {
              setActiveId(id);
              setDraftPrompt("");
              setView("chat");
            }}
            onNeedToken={() => setView("profile")}
          />
        </div>

        {view === "accounts" && (
          <AccountsPanel apiBase={apiBase} token={token} markets={markets} onOpenStock={(market, stock) => openStock(market, stock, "accounts")} onNeedToken={() => setView("profile")} />
        )}

        {(watchlistVisited || view === "watchlist") && <div hidden={view !== "watchlist"}>
          <WatchlistPanel key={`${apiBase}:${token}`} isActive={view === "watchlist"} apiBase={apiBase} token={token} markets={markets} onOpenStock={(market, stock) => openStock(market, stock, "watchlist")} onNeedToken={() => setView("profile")} />
        </div>}

        {view === "market" && (
          <MarketPanel
            apiBase={apiBase}
            token={token}
            market={activeMarket}
            quotes={marketPulse.indices[activeMarket.id]}
            indicesLoading={marketPulse.loadingByMarket[activeMarket.id]}
            onBack={() => setView("home")}
            onOpenStock={(stock) => openStock(activeMarket, stock)}
            onOpenAgent={() => openAgentChat(activeMarket)}
            onNeedToken={() => setView("profile")}
          />
        )}

        {view === "stock" && activeStock && (
          <StockDetailPanel
            apiBase={apiBase}
            token={token}
            market={activeMarket}
            stock={activeStock}
            onBack={() => setView(stockReturnView)}
            onAskAI={(seed) => openAgentChat(activeMarket, seed)}
            onAskRoundtable={openRoundtable}
          />
        )}

        {view === "chat" && (
          <ChatPanel
            apiBase={apiBase}
            token={token}
            conversations={conversations}
            activeConversation={activeConversation}
            market={selectedMarket}
            marketPulse={marketPulse}
            draftPrompt={draftPrompt}
            onSelect={(id) => {
              setActiveId(id);
              setDraftPrompt("");
            }}
            onDraftPromptChange={setDraftPrompt}
            onOpenMarket={openAgentChat}
            onOpenRoundtable={openRoundtable}
            onDelete={deleteConversation}
            onUpdate={updateConversation}
            onNeedToken={() => setView("profile")}
          />
        )}

        {view === "profile" && (
          <ProfilePanel
            apiBase={apiBase}
            token={token}
            isOnline={isOnline}
            onApiBaseChange={setApiBase}
            onTokenChange={setToken}
            onRefreshHealth={() => healthCheck(apiBase).then(setIsOnline)}
          />
        )}
      </main>

      <nav className="mobile-tabs">
        <NavButton active={view === "home" || view === "market" || view === "stock"} icon={Home} label="首页" onClick={() => setView("home")} compact />
        <NavButton active={view === "accounts" || (view === "stock" && stockReturnView === "accounts")} icon={WalletCards} label="账户" onClick={() => setView("accounts")} compact />
        <NavButton active={view === "watchlist" || (view === "stock" && stockReturnView === "watchlist")} icon={ListPlus} label="自选" onClick={() => setView("watchlist")} compact />
        <NavButton active={view === "chat"} icon={MessageCircle} label="对话" onClick={() => setView("chat")} compact />
        <NavButton active={view === "profile"} icon={UserCircle} label="我的" onClick={() => setView("profile")} compact />
      </nav>
    </div>
  );
}

function TopBar({
  pageTitle,
  apiBase,
  isOnline,
  token,
  onOpenSettings,
  onOpenStock
}: {
  pageTitle: string;
  apiBase: string;
  isOnline: boolean | null;
  token: string;
  onOpenSettings: () => void;
  onOpenStock: (market: Market, stock: StockQuote) => void;
}) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Array<{ market: Market; stock: StockQuote }>>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);
  const { rootRef, isOpen, setIsOpen } = useDismissiblePopover<HTMLFormElement>();

  useEffect(() => {
    const trimmed = query.trim();
    if (!trimmed || !isOpen) {
      setResults([]);
      setSearchError(null);
      setIsSearching(false);
      return;
    }

    let cancelled = false;
    setResults([]);
    setIsSearching(true);
    setSearchError(null);

    const timer = window.setTimeout(() => {
      Promise.allSettled(markets.map((market) => searchStocks(apiBase, market.id, trimmed)))
        .then((settled) => {
          if (cancelled) return;
          const next = settled.flatMap((result, index) =>
            result.status === "fulfilled" && Array.isArray(result.value)
              ? result.value.slice(0, 6).map((stock) => ({ market: markets[index], stock }))
              : []
          );
          setResults(next.slice(0, 10));
          if (next.length === 0 && settled.every((item) => item.status === "rejected")) {
            setSearchError("搜索服务暂不可用");
          }
        })
        .finally(() => {
          if (!cancelled) setIsSearching(false);
        });
    }, 280);

    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [apiBase, query, isOpen]);

  const submitSearch = (event: FormEvent) => {
    event.preventDefault();
    if (isOpen && results[0]) {
      onOpenStock(results[0].market, results[0].stock);
      setQuery("");
      setResults([]);
      setIsOpen(false);
    }
  };

  return (
    <header className="topbar">
      <div className="topbar-location"><span>工作台</span><ChevronRight size={13} /><strong>{pageTitle}</strong></div>
      <form className="search-shell" ref={rootRef} onSubmit={submitSearch}>
        <div className="search-box">
          <Search size={17} />
          <input
            value={query}
            onChange={(event) => { setQuery(event.target.value); setIsOpen(true); }}
            onFocus={() => setIsOpen(true)}
            onClick={() => setIsOpen(true)}
            placeholder="搜索股票、币种、指数代码"
          />
          {isSearching && <Loader2 className="spin" size={16} />}
        </div>
        {isOpen && (query.trim() || searchError) && (
          <div className="search-results" role="region" aria-label="顶部搜索结果">
            {searchError ? (
              <div className="search-empty">{searchError}</div>
            ) : results.length > 0 ? (
              results.map(({ market, stock }) => (
                <button
                  type="button"
                  key={`${market.id}-${stock.id}`}
                  onClick={() => {
                    onOpenStock(market, stock);
                    setQuery("");
                    setResults([]);
                    setIsOpen(false);
                  }}
                >
                  <MarketIcon market={market} mini />
                  <span>
                    <strong>{stock.name}</strong>
                    <small>{stock.symbol} · {market.displayName}</small>
                  </span>
                  <ChangePill value={stock.change_percent} small />
                </button>
              ))
            ) : (
              <div className="search-empty">{isSearching ? "搜索中..." : "暂无匹配结果"}</div>
            )}
          </div>
        )}
      </form>
      <div className="topbar-actions">
        <span className={`connection-indicator ${isOnline ? "online" : ""}`}><i />{isOnline === null ? "连接中" : isOnline ? "数据已连接" : "连接待恢复"}</span>
        <button className="topbar-account" aria-label={token ? "我的账户" : "登录账户"} onClick={onOpenSettings}><UserCircle size={18}/><span>{token ? "我的账户" : "登录账户"}</span></button>
        <button className="icon-button" onClick={onOpenSettings} aria-label="打开设置">
          <Settings size={18} />
        </button>
      </div>
    </header>
  );
}

function HomePanel({
  apiBase,
  token,
  isActive,
  conversations,
  marketPulse,
  onOpenMarket,
  onOpenStock,
  onOpenConversation,
  onNeedToken
}: {
  apiBase: string;
  token: string;
  isActive: boolean;
  conversations: SavedConversation[];
  marketPulse: ReturnType<typeof useMarketPulse>;
  onOpenMarket: (market: Market) => void;
  onOpenStock: (market: Market, stock: StockQuote) => void;
  onOpenConversation: (id: string) => void;
  onNeedToken: () => void;
}) {
  const leadingQuote = markets
    .flatMap((market) => marketPulse.indices[market.id].map((quote) => ({ market, quote })))
    .sort((a, b) => Math.abs(b.quote.change_percent) - Math.abs(a.quote.change_percent))[0];

  return (
    <section className="workspace home-workspace">
      <div className="hero-panel">
        <div>
          <p className="eyebrow">市场 · 研究 · 决策</p>
          <h2>看清市场，从容决策。</h2>
          <p className="hero-copy">
            关注你的市场、自选与持仓，让每一次研究更有依据。
          </p>
        </div>
        <div className="hero-metrics">
          <Metric label="覆盖市场" value="4" sub="A 股 / 美股 / 港股 / Crypto" />
          <Metric label="分析会话" value={String(conversations.length)} sub="本地工作记录" />
          <Metric
            label="焦点波动"
            value={leadingQuote ? `${formatPercent(leadingQuote.quote.change_percent)}` : "--"}
            sub={leadingQuote ? `${leadingQuote.market.displayName} · ${leadingQuote.quote.short_name ?? leadingQuote.quote.name}` : "等待行情"}
            negative={leadingQuote ? leadingQuote.quote.change_percent < 0 : false}
          />
        </div>
      </div>

      <div className="dashboard-grid">
        <div className="main-column">
          <SectionHeader title="全球市场" subtitle="各市场代表标的与最新行情" />
          <div className="market-matrix">
            {markets.map((market) => (
              <MarketDeskCard
                key={market.id}
                market={market}
                quotes={marketPulse.indices[market.id]}
                loading={marketPulse.loadingByMarket[market.id]}
                error={marketPulse.errors[market.id]}
                onClick={() => onOpenMarket(market)}
              />
            ))}
          </div>

          <SectionHeader title="自选列表" subtitle="点击个股直接进入个股分析页面" />
          {!token && (
            <div className="auth-callout">
              <KeyRound size={18} />
              <span>登录后这里会展示你的真实自选股；当前先显示各市场热门观察标的。</span>
              <button onClick={onNeedToken}>去登录</button>
            </div>
          )}
          <div className="watch-grid">
            {markets.map((market) => (
              <WatchlistPreview
                key={market.id}
                isActive={isActive}
                apiBase={apiBase}
                token={token}
                market={market}
                onOpenMarket={onOpenMarket}
                onOpenStock={(stock) => onOpenStock(market, stock)}
              />
            ))}
          </div>
        </div>

        <aside className="insight-column">
          <DecisionPanel leadingQuote={leadingQuote} conversations={conversations.length} />

          <section className="surface-card">
            <SectionHeader title="快速研究" subtitle="选择一个市场，开始分析" compact />
            <div className="prompt-stack">
              {markets.map((market) => (
                <button key={market.id} onClick={() => onOpenMarket(market)}>
                  <MarketIcon market={market} small />
                  <span>{market.displayName} 市场研究</span>
                </button>
              ))}
            </div>
          </section>

          <section className="surface-card">
            <SectionHeader title="最近对话" subtitle="继续未完成的分析" compact />
            <RecentList conversations={conversations} onOpenConversation={onOpenConversation} />
          </section>
        </aside>
      </div>
    </section>
  );
}

function MarketPanel({
  apiBase,
  token,
  market,
  quotes,
  indicesLoading,
  onBack,
  onOpenStock,
  onOpenAgent,
  onNeedToken
}: {
  apiBase: string;
  token: string;
  market: Market;
  quotes: IndexQuote[];
  indicesLoading: boolean;
  onBack: () => void;
  onOpenStock: (stock: StockQuote) => void;
  onOpenAgent: () => void;
  onNeedToken: () => void;
}) {
  const board = useMarketBoard(apiBase, token, market);

  const openIndex = (index: IndexQuote) => {
    onOpenStock({
      id: index.id,
      symbol: index.id.toUpperCase(),
      name: index.name,
      market: market.id,
      current_price: index.value,
      change: index.change,
      change_percent: index.change_percent,
      volume: 0,
      high: index.value,
      low: index.value,
      open: index.value - index.change,
      previous_close: index.value - index.change,
      source: index.source, currency: index.currency, product_type: index.product_type
    });
  };

  return (
    <section className="workspace market-workspace">
      <div className="market-hero">
        <button className="ghost-button" onClick={onBack}>返回首页</button>
        <div className="market-hero-main">
          <MarketIcon market={market} />
          <div>
            <p className="eyebrow">Market Board</p>
            <h2>{market.displayName} 市场板块</h2>
            <p>{market.subtitle} · 自选股、指数走势和股票分析入口</p>
          </div>
        </div>
        <button className="primary-action" onClick={onOpenAgent}>
          <MessageCircle size={17} />
          问问 {market.displayName} Agent
        </button>
      </div>

      <div className="market-board-grid">
        <div className="main-column">
          <SectionHeader title={market.id === "a_share" ? "大盘走势" : "市场行情"} subtitle={market.id === "a_share" ? "点击指数查看走势与 AI 分析" : "Bitget · 点击标的查看走势与 AI 分析"} />
          <div className="index-grid">
            {indicesLoading ? (
              [0, 1, 2].map((item) => (
                <div className="index-card loading" key={item}>
                  <div className="skeleton-lines"><span /><span /><span /></div>
                </div>
              ))
            ) : quotes.length > 0 ? (
              quotes.map((quote) => (
                <button className="index-card" key={quote.id} onClick={() => openIndex(quote)}>
                  <div>
                    <span>{quote.short_name ?? quote.name}</span>
                    <strong>{formatNumber(quote.value)}</strong>
                  </div>
                  <ChangePill value={quote.change_percent} />
                  <Sparkline values={quote.sparkline_data ?? []} positive={quote.change_percent >= 0} />
                </button>
              ))
            ) : (
              <div className="empty-state">暂无指数数据</div>
            )}
          </div>

          <SectionHeader title="自选股" subtitle={token ? "来自你的后端账户自选列表" : "登录后展示你的真实自选股"} />
          {!token && (
            <div className="auth-callout">
              <KeyRound size={18} />
              <span>需要 Token 才能读取自选股。你可以先去“账户与设置”使用开发登录。</span>
              <button onClick={onNeedToken}>去设置</button>
            </div>
          )}
          <StockTable
            title={token ? "我的自选" : "自选股待登录"}
            stocks={board.watchlist}
            loading={board.loading && !board.hasLoaded && Boolean(token)}
            emptyText={token ? "当前市场暂无自选股，可以先从热门观察进入分析。" : "登录后这里会显示你的自选股。"}
            onOpenStock={onOpenStock}
          />

          <SectionHeader title="热门观察" subtitle="无自选时也可以直接查看股票分析" />
          <StockTable
            title="热门标的"
            stocks={board.popular}
            loading={board.loading && !board.hasLoaded}
            emptyText="热门标的暂不可用"
            onOpenStock={onOpenStock}
          />
        </div>

        <aside className="insight-column">
          <section className="surface-card">
            <SectionHeader title="板块能力" subtitle={market.subtitle} compact />
            <div className="capability-list">
              {market.scope.map((item) => <span key={item}>{item}</span>)}
            </div>
          </section>
          <section className="surface-card">
            <SectionHeader title="分析路径" subtitle="参考 iOS 股票详情" compact />
            <div className="workflow-list">
              <WorkflowItem icon={Activity} title="先看价格与K线" text="确认走势、波动和关键位置。" />
              <WorkflowItem icon={WalletCards} title="再看自选组合" text="从你的关注列表进入具体标的。" />
              <WorkflowItem icon={Sparkles} title="最后问问慧投" text="让 Agent 基于标的上下文继续追问。" />
            </div>
          </section>
          {board.error && <div className="error-banner">{board.error}</div>}
        </aside>
      </div>
    </section>
  );
}

function DecisionPanel({
  leadingQuote,
  conversations
}: {
  leadingQuote?: { market: Market; quote: IndexQuote };
  conversations: number;
}) {
  const tone = leadingQuote && leadingQuote.quote.change_percent < 0 ? "red" : "green";
  const direction = leadingQuote
    ? leadingQuote.quote.change_percent >= 0
      ? "强势市场优先跟踪延续性"
      : "波动放大，先控制风险暴露"
    : "等待市场数据同步";

  return (
    <section className="decision-card">
      <SectionHeader title="AI 决策摘要" subtitle="首页只保留可行动结论" compact />
      <div className="decision-verdict">
        <Badge tone={tone}>{leadingQuote ? leadingQuote.market.displayName : "待同步"}</Badge>
        <h3>{direction}</h3>
        <p>
          {leadingQuote
            ? `${leadingQuote.quote.short_name ?? leadingQuote.quote.name} 当前波动 ${formatPercent(leadingQuote.quote.change_percent)}，建议先查看对应市场自选列表，再进入个股确认支撑、压力和新闻驱动。`
            : "市场指数同步后，将在这里生成今日主线和风险提示。"}
        </p>
      </div>
      <div className="decision-grid">
        <div>
          <span>风险边界</span>
          <strong>{leadingQuote && leadingQuote.quote.change_percent < 0 ? "降低追涨" : "控制仓位"}</strong>
        </div>
        <div>
          <span>分析记录</span>
          <strong>{conversations}</strong>
        </div>
      </div>
      <div className="decision-steps">
        <span><b>1</b> 观察市场雷达的相对强弱</span>
        <span><b>2</b> 点击自选股进入个股分析</span>
        <span><b>3</b> 用 AI 验证关键价位和风险</span>
      </div>
    </section>
  );
}

function StockDetailPanel({
  apiBase,
  token,
  market,
  stock,
  onBack,
  onAskAI,
  onAskRoundtable
}: {
  apiBase: string;
  token: string;
  market: Market;
  stock: StockQuote;
  onBack: () => void;
  onAskAI: (seed: string) => void;
  onAskRoundtable: (seed?: string) => void;
}) {
  const availablePeriods = useMemo(() => availableKLinePeriods(market.id), [market.id]);
  const [selectedPeriod, setSelectedPeriod] = useState<KLinePeriod>("1d");
  useEffect(() => {
    if (!availablePeriods.includes(selectedPeriod)) setSelectedPeriod("1d");
  }, [availablePeriods, selectedPeriod]);

  const detail = useStockDetail(apiBase, market, stock, selectedPeriod);
  const display = detail.quote ?? stock;

  return (
    <section className="workspace stock-workspace">
      <div className="stock-header">
        <button className="ghost-button" onClick={onBack}>返回 {market.displayName}</button>
        <div>
          <p className="eyebrow">Stock Analysis</p>
          <h2>{display.name}</h2>
          <p>{display.symbol} · {market.displayName} · {display.currency || (market.id === "a_share" ? "CNY" : "USD")} · {quoteStatusLabel(display)}{display.change_basis === "24h" ? " · 24h 涨跌" : ""}</p>
        </div>
        <button
          className="primary-action"
          onClick={() => onAskAI(`请结合当前行情、K线和风险边界，分析 ${display.name}（${display.symbol}）的机会与风险。`)}
        >
          <MessageCircle size={17} />
          问问慧投
        </button>
      </div>

      <div className="stock-detail-grid">
        <div className="main-column">
          <section className="price-panel">
            <div>
              <span>最新价</span>
              <strong className={display.change_percent >= 0 ? "up" : "down"}>{display.quote_available === false || display.current_price <= 0 ? "—" : formatNumber(display.current_price)}</strong>
            </div>
            <ChangePill value={display.change_percent} />
            <div className="price-stats">
              <PriceStat label="涨跌额" value={formatSigned(display.change)} positive={display.change >= 0} />
              <PriceStat label="今高" value={formatNumber(display.high)} />
              <PriceStat label="今低" value={formatNumber(display.low)} />
              <PriceStat label="成交量" value={formatNumber(display.volume)} />
            </div>
          </section>

          <section className="surface-card">
            <div className="kline-section-header">
              <SectionHeader title="K 线走势" subtitle={kLinePeriodLabels[selectedPeriod]} compact />
              <div className="period-selector" aria-label="K 线周期">
                {availablePeriods.map((period) => (
                  <button
                    key={period}
                    className={selectedPeriod === period ? "active" : ""}
                    onClick={() => setSelectedPeriod(period)}
                    type="button"
                  >
                    {kLinePeriodLabels[period]}
                  </button>
                ))}
              </div>
            </div>
            {detail.error && <p className="form-note danger" role="alert">{detail.error}</p>}
            {detail.loading ? (
              <div className="chart-skeleton"><div className="skeleton-lines"><span /><span /><span /></div></div>
            ) : detail.kline.length > 0 ? (
              <KLineChart
                data={detail.kline}
                period={selectedPeriod}
                loadingMore={detail.loadingMoreKline}
                onLoadMore={detail.loadMoreKline}
              />
            ) : (
              <div className="empty-state">暂无 K 线数据</div>
            )}
          </section>

          <StockDeepResearch key={`${apiBase}:${market.id}:${stock.id}:${stock.provider_symbol || ""}`} stock={display} period={kLinePeriodLabels[selectedPeriod]} candles={detail.kline} loading={detail.loading} analysis={detail.analysis} analysisLoading={detail.analysisLoading} error={detail.analysisError} onReload={detail.reloadAnalysis} onAskAI={onAskAI} onAskRoundtable={onAskRoundtable} />
          <StockNews key={`${apiBase}:${market.id}:${stock.id}`} apiBase={apiBase} stock={display} market={market.id} news={detail.news} loading={detail.newsLoading} error={detail.newsError} onReload={detail.reloadNews} onAskAI={onAskAI} />
        </div>

        <aside className="insight-column">
          <section className="surface-card">
            <SectionHeader title="标的档案" subtitle="行情上下文" compact />
            <div className="profile-list">
              <span><b>代码</b>{display.symbol}</span>
              <span><b>市场</b>{market.subtitle}</span>
              <span><b>前收</b>{formatNumber(display.previous_close)}</span>
              <span><b>开盘</b>{formatNumber(display.open)}</span>
            </div>
          </section>
          <section className="surface-card">
            <SectionHeader title="下一步" subtitle="让 AI 继续追问" compact />
            <div className="prompt-stack">
              {[
                `给我 ${display.name} 的支撑位和压力位`,
                `分析 ${display.name} 当前适合左侧还是右侧交易`,
                `总结 ${display.name} 的三条主要风险`
              ].map((prompt) => (
                <button key={prompt} onClick={() => onAskAI(prompt)}>
                  <Zap size={15} />
                  <span>{prompt}</span>
                </button>
              ))}
            </div>
          </section>
          {detail.error && <div className="error-banner">{detail.error}</div>}
          {!token && <div className="auth-callout compact"><KeyRound size={18} /><span>登录后可把股票加入真实自选并保存对话。</span></div>}
        </aside>
      </div>
    </section>
  );
}

function StockTable({
  title,
  stocks,
  loading,
  emptyText,
  onOpenStock
}: {
  title: string;
  stocks: StockQuote[];
  loading: boolean;
  emptyText: string;
  onOpenStock: (stock: StockQuote) => void;
}) {
  return (
    <section className="stock-table surface-card">
      <div className="table-title">{title}</div>
      {loading ? (
        <div className="skeleton-lines"><span /><span /><span /></div>
      ) : stocks.length === 0 ? (
        <div className="empty-state compact">{emptyText}</div>
      ) : (
        <div className="stock-rows">
          {stocks.map((stock) => (
            <button key={`${stock.market}-${stock.id}`} className="stock-row" onClick={() => onOpenStock(stock)}>
              <div>
                <strong>{stock.name}</strong>
                <span>{stock.symbol}</span>
              </div>
              <span className="mono">{formatNumber(stock.current_price)}</span>
              <ChangePill value={stock.change_percent} small />
              <ChevronRight size={16} />
            </button>
          ))}
        </div>
      )}
    </section>
  );
}

function ChatPanel({
  apiBase,
  token,
  conversations,
  activeConversation,
  market,
  marketPulse,
  draftPrompt,
  onSelect,
  onDraftPromptChange,
  onOpenMarket,
  onOpenRoundtable,
  onDelete,
  onUpdate,
  onNeedToken
}: {
  apiBase: string;
  token: string;
  conversations: SavedConversation[];
  activeConversation?: SavedConversation;
  market: Market;
  marketPulse: ReturnType<typeof useMarketPulse>;
  draftPrompt: string;
  onSelect: (id: string) => void;
  onDraftPromptChange: (value: string) => void;
  onOpenMarket: (market: Market, seed?: string) => void;
  onOpenRoundtable: (seed?: string) => void;
  onDelete: (id: string) => void;
  onUpdate: (conversation: SavedConversation) => void;
  onNeedToken: () => void;
}) {
  return (
    <section className="chat-workspace">
      <aside className="conversation-rail">
        <div className="rail-section">
          <div className="rail-heading">Agent</div>
          <button className={`roundtable ${activeConversation?.kind === "roundtable" ? "selected" : ""}`} onClick={() => onOpenRoundtable()}>
            <div className="roundtable-grid">
              <Bot size={14} />
              <BarChart3 size={14} />
              <CircleDollarSign size={14} />
              <Bitcoin size={14} />
            </div>
            <div>
              <strong>慧投圆桌</strong>
              <span>多 Agent 投资圆桌讨论</span>
            </div>
          </button>
          {markets.map((item) => (
            <button
              className={`agent-row ${!activeConversation?.kind && market.id === item.id ? "selected" : ""}`}
              key={item.id}
              onClick={() => onOpenMarket(item)}
            >
              <MarketIcon market={item} small />
              <div>
                <strong>{item.title}</strong>
                <span>{item.description}</span>
              </div>
              <ChevronRight size={17} />
            </button>
          ))}
        </div>

        <div className="rail-section">
          <div className="rail-heading">历史对话</div>
          {conversations.length === 0 ? (
            <div className="rail-empty">还没有保存的会话</div>
          ) : (
            conversations.map((conversation) => {
              const itemMarket = markets.find((item) => item.id === conversation.marketId) ?? markets[0];
              return (
                <button
                  className={`history-row ${activeConversation?.id === conversation.id ? "active" : ""}`}
                  key={conversation.id}
                  onClick={() => onSelect(conversation.id)}
                >
                  {conversation.kind === "roundtable" ? <RoundtableIcon mini /> : <MarketIcon market={itemMarket} mini />}
                  <div>
                    <strong>{conversation.title}</strong>
                    <span>{formatTime(conversation.updatedAt)}</span>
                  </div>
                  <Trash2
                    size={16}
                    onClick={(event) => {
                      event.stopPropagation();
                      onDelete(conversation.id);
                    }}
                  />
                </button>
              );
            })
          )}
        </div>
      </aside>

      <div className="chat-main">
        {activeConversation ? (
          activeConversation.kind === "roundtable" ? <RoundtableView key={activeConversation.id} apiBase={apiBase} token={token} conversation={activeConversation} draftPrompt={draftPrompt} onDraftPromptChange={onDraftPromptChange} onUpdate={onUpdate} onNeedToken={onNeedToken} /> : <ConversationView
            key={activeConversation.id}
            apiBase={apiBase}
            token={token}
            market={market}
            conversation={activeConversation}
            draftPrompt={draftPrompt}
            onDraftPromptChange={onDraftPromptChange}
            onUpdate={onUpdate}
            onNeedToken={onNeedToken}
          />
        ) : (
          <div className="chat-empty">
            <Sparkles size={34} />
            <h2>选择一个 Agent 开始分析</h2>
            <p>正式工作台会把历史会话、上下文建议和流式回复保存在同一视图里。</p>
          </div>
        )}
      </div>

      <aside className="context-panel">
        {activeConversation?.kind === "roundtable" ? <>
          <section className="surface-card roundtable-role-list"><SectionHeader title="圆桌成员" subtitle="不同视角，交叉验证" compact />{roundtableAgents.map(agent => <div key={agent.id}><span>{agent.mark}</span><div><strong>{agent.name}</strong><p>{agent.role}</p></div></div>)}</section>
          <section className="surface-card"><SectionHeader title="开始讨论" subtitle="点击填入问题" compact /><div className="prompt-stack">{["从价值、趋势与风险三个角度，讨论我关注的公司", "@价值派 如何判断公司的估值是否合理？", "@主持人 投资观点出现分歧时，应该重点核验什么？"].map(prompt => <button key={prompt} onClick={() => onOpenRoundtable(prompt)}><Zap size={15} /><span>{prompt}</span></button>)}</div></section>
        </> : <><MarketContext market={market} quotes={marketPulse.indices[market.id]} loading={marketPulse.loadingByMarket[market.id]} />
        <section className="surface-card">
          <SectionHeader title="推荐问题" subtitle="直接填入输入框" compact />
          <div className="prompt-stack">
            {promptDeck[market.id].map((prompt) => (
              <button key={prompt} onClick={() => onOpenMarket(market, prompt)}>
                <Zap size={15} />
                <span>{prompt}</span>
              </button>
            ))}
          </div>
        </section>
        <section className="surface-card">
          <SectionHeader title="Agent 能力" subtitle={market.subtitle} compact />
          <div className="capability-list">
            {market.scope.map((item) => (
              <span key={item}>{item}</span>
            ))}
          </div>
        </section>
        </>}
      </aside>
    </section>
  );
}

function ConversationView({
  apiBase,
  token,
  market,
  conversation,
  draftPrompt,
  onDraftPromptChange,
  onUpdate,
  onNeedToken
}: {
  apiBase: string;
  token: string;
  market: Market;
  conversation: SavedConversation;
  draftPrompt: string;
  onDraftPromptChange: (value: string) => void;
  onUpdate: (conversation: SavedConversation) => void;
  onNeedToken: () => void;
}) {
  const [input, setInput] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const bottomRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [conversation.messages, isLoading]);

  useEffect(() => {
    setInput(draftPrompt);
  }, [conversation.id, draftPrompt]);

  const send = async (event: FormEvent) => {
    event.preventDefault();
    const text = input.trim();
    if (!text || isLoading) return;
    if (!token) {
      setError("需要 JWT Token 才能调用受保护的对话接口。");
      onNeedToken();
      return;
    }

    setInput("");
    onDraftPromptChange("");
    setIsLoading(true);
    setError(null);

    const userMessage: Message = { id: uid(), role: "user", content: text };
    const assistantId = uid();
    let next: SavedConversation = {
      ...conversation,
      title: conversation.title || text.slice(0, 18),
      updatedAt: new Date().toISOString(),
      messages: [...conversation.messages, userMessage, { id: assistantId, role: "assistant", content: "", thinking: [] }]
    };
    onUpdate(next);

    try {
      const backendId = next.backendId ?? (await createConversation(apiBase, token, market)).id;
      next = { ...next, backendId };
      onUpdate(next);

      await streamMessage(apiBase, token, backendId, text, (chunk) => {
        if (chunk.error) throw new Error(chunk.error);
        next = {
          ...next,
          updatedAt: new Date().toISOString(),
          messages: next.messages.map((message) => {
            if (message.id !== assistantId) return message;
            if (chunk.type === "thought") {
              const thought = chunk.content?.trim();
              return thought ? { ...message, thinking: [...(message.thinking ?? []), thought].slice(-4) } : message;
            }
            return { ...message, content: message.content + (chunk.content ?? "") };
          })
        };
        onUpdate(next);
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "发送失败");
      next = { ...next, messages: next.messages.filter((message) => message.id !== assistantId) };
      onUpdate(next);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="conversation">
      <header className="conversation-header">
        <MarketIcon market={market} />
        <div>
          <h2>{market.title}</h2>
          <p>{market.subtitle} · {conversation.backendId ? `Session #${conversation.backendId}` : "Local draft"}</p>
        </div>
        <Badge tone={token ? "green" : "muted"}>{token ? "Authenticated" : "Token required"}</Badge>
      </header>

      <div className="messages">
        {conversation.messages.map((message) => (
          <div className={`message ${message.role}`} key={message.id}>
            {message.thinking && message.thinking.length > 0 && (
              <div className="thinking">
                {message.thinking.map((line, index) => (
                  <span key={`${line}-${index}`}>{line}</span>
                ))}
              </div>
            )}
            {message.role === "assistant" ? (
              <ChatMarkdown content={message.content || (isLoading ? "正在思考..." : "")} />
            ) : (
              <p>{message.content}</p>
            )}
          </div>
        ))}
        <div ref={bottomRef} />
      </div>

      {error && <div className="error-banner">{error}</div>}

      <form className="composer" onSubmit={send}>
        <textarea
          value={input}
          onChange={(event) => {
            setInput(event.target.value);
            onDraftPromptChange(event.target.value);
          }}
          placeholder={`向 ${market.title} 提问，例如：${promptDeck[market.id][0]}`}
          rows={1}
          disabled={isLoading}
        />
        <button disabled={!input.trim() || isLoading}>
          {isLoading ? <Loader2 className="spin" size={22} /> : <Send size={22} />}
        </button>
      </form>
    </div>
  );
}

function ProfilePanel({
  apiBase,
  token,
  isOnline,
  onApiBaseChange,
  onTokenChange,
  onRefreshHealth
}: {
  apiBase: string;
  token: string;
  isOnline: boolean | null;
  onApiBaseChange: (value: string) => void;
  onTokenChange: (value: string) => void;
  onRefreshHealth: () => void;
}) {
  const [loginState, setLoginState] = useState<"idle" | "loading" | "done" | "error">("idle");
  const maskedToken = useMemo(() => (token ? `${token.slice(0, 14)}...${token.slice(-8)}` : "未设置"), [token]);

  const handleDevLogin = async () => {
    setLoginState("loading");
    try {
      const result = await devLogin(apiBase);
      onTokenChange(result.token);
      setLoginState("done");
    } catch {
      setLoginState("error");
    }
  };

  return (
    <section className="workspace profile-workspace">
      <div className="profile-header">
        <div>
          <p className="eyebrow">偏好与连接</p>
          <h2>账户与设置</h2>
          <p>管理账户登录与服务连接。</p>
        </div>
        <div className="profile-status">
          <CheckCircle2 size={22} />
          <span>{isOnline ? "后端在线" : "等待连接"}</span>
        </div>
      </div>

      <div className="settings-grid">
        <section className="settings-card primary">
          <Settings size={22} />
          <div>
            <h3>服务连接</h3>
            <p>设置慧投服务地址，检查当前连接状态。</p>
          </div>
          <input value={apiBase} onChange={(event) => onApiBaseChange(event.target.value)} />
          <button onClick={onRefreshHealth}>检测连接</button>
        </section>

        <section className="settings-card">
          <KeyRound size={22} />
          <div>
            <h3>登录凭证</h3>
            <p>{maskedToken}</p>
          </div>
          <textarea
            value={token}
            onChange={(event) => onTokenChange(event.target.value.trim())}
            placeholder="粘贴你的慧投登录凭证"
          />
          <button onClick={handleDevLogin} disabled={loginState === "loading"}>
            {loginState === "loading" ? "登录中..." : "开发登录"}
          </button>
          {loginState === "error" && <span className="form-note danger">此服务未开放测试登录，请使用已有的登录凭证。</span>}
          {loginState === "done" && <span className="form-note success">登录凭证已保存到当前设备。</span>}
        </section>

        <section className="settings-card">
          <ShieldCheck size={22} />
          <div>
            <h3>你的研究空间</h3>
            <p>登录后同步自选列表与已绑定的实体账户。登录凭证保存在当前设备。</p>
          </div>
          <div className="check-list">
            <span>多市场自选</span>
            <span>只读持仓同步</span>
            <span>行情与基本面研究</span>
          </div>
        </section>
      </div>
    </section>
  );
}

function MarketDeskCard({
  market,
  quotes,
  loading,
  error,
  onClick
}: {
  market: Market;
  quotes: IndexQuote[];
  loading: boolean;
  error?: string;
  onClick: () => void;
}) {
  const main = quotes[0];

  return (
    <button className="market-desk-card" onClick={onClick}>
      <div className="market-card-head">
        <MarketIcon market={market} />
        <div>
          <h3>{market.displayName}</h3>
          <p>{market.description}</p>
        </div>
        <ChevronRight size={19} />
      </div>

      {loading && !main ? (
        <div className="market-quote-skeleton" role="status" aria-label="正在加载市场行情">
          <div className="quote-strip" aria-hidden="true"><div><span className="skeleton-block market-name-skeleton" /><strong><span className="skeleton-block market-value-skeleton" /></strong></div><span className="skeleton-block market-change-skeleton" /></div>
          <div className="skeleton-block market-chart-skeleton" aria-hidden="true" />
        </div>
      ) : main ? (
        <>
          <div className="quote-strip">
            <div>
              <span>{main.short_name ?? main.name}</span>
              <strong>{formatNumber(main.value)}</strong>
            </div>
            <ChangePill value={main.change_percent} />
          </div>
          <Sparkline values={main.sparkline_data ?? []} positive={main.change_percent >= 0} />
          {(main.source || error) && <small className="market-quote-note">{quoteStatusLabel({ ...main, market: market.id })}{error ? " · 更新暂不可用，保留上次行情" : ""}</small>}
        </>
      ) : (
        <div className="quote-empty">行情暂不可用，可继续查看市场。</div>
      )}

      <div className="capability-list">
        {market.scope.slice(0, 3).map((item) => (
          <span key={item}>{item}</span>
        ))}
      </div>
    </button>
  );
}

function WatchlistPreview({
  apiBase,
  token,
  isActive,
  market,
  onOpenMarket,
  onOpenStock
}: {
  apiBase: string;
  token: string;
  isActive: boolean;
  market: Market;
  onOpenMarket: (market: Market) => void;
  onOpenStock: (stock: StockQuote) => void;
}) {
  const board = useMarketBoard(apiBase, token, market, isActive);
  const initialLoading = board.loading && !board.hasLoaded;
  const rows = board.watchlist.length > 0 ? board.watchlist : board.popular.slice(0, 4);
  const isRealWatchlist = board.watchlist.length > 0;

  return (
    <section className="watch-panel" aria-label={`${market.displayName}自选预览`} aria-busy={board.loading}>
      <div className="watch-title">
        <MarketIcon market={market} mini />
        <div>
          <strong>{market.displayName}</strong>
          <span>{initialLoading ? <span className="skeleton-block watch-subtitle-skeleton" aria-hidden="true" /> : isRealWatchlist ? "我的自选" : "热门观察"}</span>
        </div>
        {board.loading && !initialLoading && <Loader2 className="spin watch-refresh-indicator" size={12} aria-label="正在更新行情" />}
      </div>
      {initialLoading ? (
        <div role="status" aria-label="正在加载自选行情">
          {Array.from({ length: 4 }, (_, index) => <div className="watch-preview-row watch-preview-skeleton" aria-hidden="true" key={index}><span className="skeleton-block watch-symbol-skeleton" /><span className="watch-price"><span className="skeleton-block watch-price-skeleton" /></span><span className="skeleton-block watch-change-skeleton" /><span className="watch-quote-meta"><span className="skeleton-block watch-meta-skeleton" /></span></div>)}
        </div>
      ) : rows.length > 0 ? (
        rows.map((stock) => (
          <button className="watch-preview-row" key={`${market.id}-${stock.id}`} onClick={() => onOpenStock(stock)}>
            <span className="watch-symbol">{watchDisplayName(market, stock)}</span>
            <span className="watch-price"><strong>{stock.quote_available === false || stock.current_price <= 0 ? "—" : formatNumber(stock.current_price)}</strong><small>{quoteCurrency(stock)}</small></span>
            {stock.quote_available === false || stock.current_price <= 0 ? <span className="watch-change-empty">—</span> : <ChangePill value={stock.change_percent} small />}
            <span className="watch-quote-meta" title={quoteTimeLabel(stock)}>{quoteStatusLabel(stock)}</span>
          </button>
        ))
      ) : (
        <div className="empty-state compact">暂无自选</div>
      )}
      {board.error && <p className="watch-preview-error" role="status" title={board.error}>{rows.length ? "更新暂不可用，保留上次行情" : "行情暂不可用，请稍后重试"}</p>}
      <button className="watch-more" onClick={() => onOpenMarket(market)}>
        <span>查看全部</span>
        <small>进入板块</small>
      </button>
    </section>
  );
}

function watchDisplayName(market: Market, stock: StockQuote) {
  if (market.id === "a_share" || market.id === "hk_stock") return stock.name;
  return stock.symbol.replace(/\/USDT$/i, "").replace(/USDT$/i, "");
}

function WatchPanel({ market, onOpenMarket }: { market: Market; onOpenMarket: (market: Market) => void }) {
  return (
    <section className="watch-panel">
      <div className="watch-title">
        <MarketIcon market={market} mini />
        <strong>{market.displayName}</strong>
      </div>
      {market.featured.map((symbol) => (
        <button key={symbol} onClick={() => onOpenMarket(market)}>
          <span>{symbol}</span>
          <small>进板块</small>
        </button>
      ))}
    </section>
  );
}

function MarketContext({ market, quotes, loading }: { market: Market; quotes: IndexQuote[]; loading: boolean }) {
  return (
    <section className="surface-card market-context">
      <SectionHeader title="市场上下文" subtitle={market.displayName} compact />
      {loading ? (
        <div className="skeleton-lines">
          <span />
          <span />
          <span />
        </div>
      ) : quotes.length > 0 ? (
        <div className="context-quotes">
          {quotes.slice(0, 4).map((quote) => (
            <div key={quote.id}>
              <span>{quote.short_name ?? quote.name}</span>
              <strong>{formatNumber(quote.value)}</strong>
              <ChangePill value={quote.change_percent} small />
            </div>
          ))}
        </div>
      ) : (
        <p className="muted-text">实时市场上下文暂不可用。</p>
      )}
    </section>
  );
}

function RecentList({
  conversations,
  onOpenConversation
}: {
  conversations: SavedConversation[];
  onOpenConversation: (id: string) => void;
}) {
  if (conversations.length === 0) {
    return <div className="empty-state compact">暂无历史会话</div>;
  }

  return (
    <div className="recent-list">
      {conversations.slice(0, 4).map((conversation) => {
        const market = markets.find((item) => item.id === conversation.marketId) ?? markets[0];
        return (
          <button key={conversation.id} onClick={() => onOpenConversation(conversation.id)}>
            {conversation.kind === "roundtable" ? <RoundtableIcon mini /> : <MarketIcon market={market} mini />}
            <div>
              <strong>{conversation.title}</strong>
              <span>{formatTime(conversation.updatedAt)}</span>
            </div>
          </button>
        );
      })}
    </div>
  );
}

function SectionHeader({ title, subtitle, compact = false }: { title: string; subtitle?: string; compact?: boolean }) {
  return (
    <div className={`section-header ${compact ? "compact" : ""}`}>
      <div>
        <h3>{title}</h3>
        {subtitle && <p>{subtitle}</p>}
      </div>
    </div>
  );
}

function WorkflowItem({ icon: Icon, title, text }: { icon: LucideIcon; title: string; text: string }) {
  return (
    <div className="workflow-item">
      <Icon size={18} />
      <div>
        <strong>{title}</strong>
        <span>{text}</span>
      </div>
    </div>
  );
}

function Metric({
  label,
  value,
  sub,
  negative = false
}: {
  label: string;
  value: string;
  sub: string;
  negative?: boolean;
}) {
  return (
    <div className={`metric ${negative ? "negative" : ""}`}>
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{sub}</small>
    </div>
  );
}

function NavButton({
  active,
  icon: Icon,
  label,
  onClick,
  compact = false
}: {
  active: boolean;
  icon: LucideIcon;
  label: string;
  onClick: () => void;
  compact?: boolean;
}) {
  return (
    <button className={active ? "active" : ""} onClick={onClick}>
      <Icon size={compact ? 20 : 19} />
      <span>{label}</span>
    </button>
  );
}

function MarketIcon({ market, small = false, mini = false }: { market: Market; small?: boolean; mini?: boolean }) {
  const Icon = market.icon;
  return (
    <div
      className={`market-icon ${small ? "small" : ""} ${mini ? "mini" : ""}`}
      style={{
        "--c1": market.colors[0],
        "--c2": market.colors[1]
      } as React.CSSProperties}
    >
      <Icon size={mini ? 14 : small ? 18 : 22} />
    </div>
  );
}

function ChangePill({ value, small = false }: { value: number; small?: boolean }) {
  const positive = value >= 0;
  const Icon = positive ? TrendingUp : TrendingDown;
  return (
    <span className={`change-pill ${positive ? "positive" : "negative"} ${small ? "small" : ""}`}>
      <Icon size={small ? 12 : 14} />
      {formatPercent(value)}
    </span>
  );
}

function Badge({ children, tone = "muted" }: { children: string; tone?: "green" | "red" | "blue" | "muted" }) {
  return <span className={`badge ${tone}`}>{children}</span>;
}

function StatusLine({ label, ok, pending = false }: { label: string; ok: boolean; pending?: boolean }) {
  return (
    <div className="status-line">
      <span className={pending ? "pending" : ok ? "ok" : "bad"} />
      <small>{label}</small>
    </div>
  );
}

function Sparkline({ values, positive }: { values: number[]; positive: boolean }) {
  const points = useMemo(() => {
    const data = values.filter(Number.isFinite);
    if (data.length < 2) return "";
    const min = Math.min(...data);
    const max = Math.max(...data);
    const spread = max - min || 1;
    return data
      .map((value, index) => {
        const x = (index / Math.max(1, data.length - 1)) * 100;
        const y = 38 - ((value - min) / spread) * 34;
        return `${x},${y}`;
      })
      .join(" ");
  }, [values]);

  if (!points) return <div className="sparkline-empty">暂无走势数据</div>;
  return (
    <svg className={`sparkline ${positive ? "positive" : "negative"}`} viewBox="0 0 100 42" preserveAspectRatio="none">
      <polyline points={points} fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" />
    </svg>
  );
}

function KLineChart({
  data,
  period,
  onLoadMore,
  loadingMore = false
}: {
  data: KLinePoint[];
  period: KLinePeriod;
  onLoadMore?: () => void;
  loadingMore?: boolean;
}) {
  const chartRef = useRef<HTMLDivElement | null>(null);
  const dragRef = useRef<{ x: number; offset: number } | null>(null);
  const loadMoreAnchorRef = useRef<"latest" | "history" | null>(null);
  const previousLengthRef = useRef(data.length);
  const requestedLengthRef = useRef(0);
  const [visibleCount, setVisibleCount] = useState(40);
  const [scrollOffset, setScrollOffset] = useState(0);
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);
  const [isDragging, setIsDragging] = useState(false);

  const normalizedData = useMemo(
    () =>
      data
        .filter((item) => Number.isFinite(item.open) && Number.isFinite(item.close) && Number.isFinite(item.high) && Number.isFinite(item.low))
        .slice()
        .sort((a, b) => new Date(a.date).getTime() - new Date(b.date).getTime()),
    [data]
  );

  const maxVisibleCount = Math.max(15, Math.min(220, normalizedData.length || 40));
  const maxOffset = Math.max(0, normalizedData.length - visibleCount);

  useEffect(() => {
    setVisibleCount((count) => clamp(count, 15, maxVisibleCount));
  }, [maxVisibleCount]);

  useEffect(() => {
    setScrollOffset((offset) => clamp(offset, 0, maxOffset));
  }, [maxOffset]);

  useEffect(() => {
    setVisibleCount((count) => Math.min(count || 40, maxVisibleCount));
    setScrollOffset(0);
    setHoverIndex(null);
    previousLengthRef.current = normalizedData.length;
  }, [period]);

  useEffect(() => {
    const previousLength = previousLengthRef.current;
    const addedCount = normalizedData.length - previousLength;
    if (addedCount > 0 && previousLength > 0) {
      const anchor = loadMoreAnchorRef.current;
      setScrollOffset((offset) => {
        if (anchor === "history" && offset > 0) {
          return clamp(offset + addedCount, 0, Math.max(0, normalizedData.length - visibleCount));
        }
        return clamp(offset, 0, Math.max(0, normalizedData.length - visibleCount));
      });
      requestedLengthRef.current = 0;
    }
    loadMoreAnchorRef.current = null;
    previousLengthRef.current = normalizedData.length;
  }, [normalizedData.length, visibleCount]);

  useEffect(() => {
    if (!onLoadMore || loadingMore || requestedLengthRef.current === normalizedData.length) return;

    const nearHistoricalEdge = maxOffset > 8 && scrollOffset > 0 && scrollOffset >= maxOffset - 8;
    const zoomedToLoadedRange = scrollOffset === 0 && normalizedData.length > 0 && visibleCount >= normalizedData.length - 2;

    if (nearHistoricalEdge || zoomedToLoadedRange) {
      loadMoreAnchorRef.current = nearHistoricalEdge ? "history" : "latest";
      requestedLengthRef.current = normalizedData.length;
      onLoadMore();
    }
  }, [loadingMore, maxOffset, normalizedData.length, onLoadMore, scrollOffset, visibleCount]);

  useEffect(() => {
    const node = chartRef.current;
    if (!node) return;

    const handleNativeWheel = (event: WheelEvent) => {
      const lockedScrollX = window.scrollX;
      const lockedScrollY = window.scrollY;
      event.preventDefault();
      event.stopPropagation();
      window.requestAnimationFrame(() => window.scrollTo(lockedScrollX, lockedScrollY));
      if (normalizedData.length < 2) return;
      const direction = event.deltaY > 0 ? 1 : -1;
      setVisibleCount((count) => clamp(count + direction * 6, 15, maxVisibleCount));
    };

    node.addEventListener("wheel", handleNativeWheel, { passive: false });
    return () => node.removeEventListener("wheel", handleNativeWheel);
  }, [maxVisibleCount, normalizedData.length]);

  const chart = useMemo(() => {
    const viewWidth = 1000;
    const top = 16;
    const priceHeight = 236;
    const gap = 18;
    const volumeHeight = 54;
    const viewHeight = top + priceHeight + gap + volumeHeight + 18;
    const start = Math.max(0, normalizedData.length - visibleCount - scrollOffset);
    const visible = normalizedData.slice(start, start + visibleCount);
    const priceHigh = Math.max(...visible.map((item) => item.high));
    const priceLow = Math.min(...visible.map((item) => item.low));
    const rawRange = priceHigh - priceLow || Math.max(1, Math.abs(priceHigh) * 0.02);
    const priceMax = priceHigh + rawRange * 0.08;
    const priceMin = priceLow - rawRange * 0.08;
    const priceRange = priceMax - priceMin || 1;
    const maxVolume = Math.max(...visible.map((item) => item.volume), 1);
    const candleStep = visible.length ? viewWidth / visible.length : viewWidth;
    const candleWidth = clamp(candleStep * 0.56, 4, 16);
    const yForPrice = (price: number) => top + ((priceMax - price) / priceRange) * priceHeight;
    const priceTicks = Array.from({ length: 4 }, (_, index) => priceMax - (priceRange / 3) * index);
    const bars = visible.map((item, index) => {
      const x = (index + 0.5) * candleStep;
      const openY = yForPrice(item.open);
      const closeY = yForPrice(item.close);
      const highY = yForPrice(item.high);
      const lowY = yForPrice(item.low);
      const volumeTop = top + priceHeight + gap + (1 - item.volume / maxVolume) * volumeHeight;
      return {
        ...item,
        fullIndex: start + index,
        x,
        openY,
        closeY,
        highY,
        lowY,
        bodyTop: Math.min(openY, closeY),
        bodyHeight: Math.max(2, Math.abs(closeY - openY)),
        volumeTop,
        volumeHeight: Math.max(1, top + priceHeight + gap + volumeHeight - volumeTop),
        candleWidth,
        up: item.close >= item.open
      };
    });
    const timeTickIndexes = Array.from(
      new Set(
        [0, 0.25, 0.5, 0.75, 1]
          .map((ratio) => Math.round((bars.length - 1) * ratio))
          .filter((index) => index >= 0 && index < bars.length)
      )
    );
    const timeTicks = timeTickIndexes.map((index) => {
      const bar = bars[index];
      return {
        anchor: index === 0 ? "start" : index === bars.length - 1 ? "end" : "middle",
        date: bar.date,
        label: formatKLineAxisDate(bar.date, period),
        x: bar.x
      };
    });
    return { bars, candleStep, priceHeight, priceMax, priceMin, priceTicks, start, timeTicks, top, viewHeight, viewWidth, volumeHeight };
  }, [normalizedData, scrollOffset, visibleCount]);

  const hoverBar = hoverIndex === null ? null : chart.bars.find((bar) => bar.fullIndex === hoverIndex) ?? null;

  const updateHover = (clientX: number) => {
    const rect = chartRef.current?.getBoundingClientRect();
    if (!rect || chart.bars.length === 0) return;
    const localX = clamp(clientX - rect.left, 0, rect.width);
    const barIndex = clamp(Math.floor((localX / rect.width) * chart.bars.length), 0, chart.bars.length - 1);
    setHoverIndex(chart.start + barIndex);
  };

  const handlePointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    dragRef.current = { x: event.clientX, offset: scrollOffset };
    setIsDragging(true);
    event.currentTarget.setPointerCapture(event.pointerId);
    updateHover(event.clientX);
  };

  const handlePointerMove = (event: PointerEvent<HTMLDivElement>) => {
    updateHover(event.clientX);
    if (!dragRef.current) return;
    const width = chartRef.current?.getBoundingClientRect().width ?? 1;
    const candleWidth = width / Math.max(1, visibleCount);
    const movedCandles = Math.round((event.clientX - dragRef.current.x) / candleWidth);
    setScrollOffset(clamp(dragRef.current.offset + movedCandles, 0, maxOffset));
  };

  const stopDrag = (event: PointerEvent<HTMLDivElement>) => {
    dragRef.current = null;
    setIsDragging(false);
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  };

  const resetToLatest = () => {
    setVisibleCount(Math.min(40, maxVisibleCount));
    setScrollOffset(0);
    setHoverIndex(null);
  };

  if (chart.bars.length === 0) return <div className="empty-state">暂无 K 线数据</div>;

  return (
    <div className={`kline-chart ${isDragging ? "dragging" : ""}`}>
      <div
        ref={chartRef}
        className="kline-stage"
        onPointerDown={handlePointerDown}
        onPointerLeave={() => {
          if (!dragRef.current) setHoverIndex(null);
        }}
        onPointerMove={handlePointerMove}
        onPointerUp={stopDrag}
      >
        <svg className="kline-canvas" viewBox={`0 0 ${chart.viewWidth} ${chart.viewHeight}`} preserveAspectRatio="none">
          {chart.priceTicks.map((price) => {
            const y = chart.top + ((chart.priceMax - price) / (chart.priceMax - chart.priceMin || 1)) * chart.priceHeight;
            return <line key={price} className="grid-line" x1="0" x2={chart.viewWidth} y1={y} y2={y} vectorEffect="non-scaling-stroke" />;
          })}
          <line
            className="volume-separator"
            x1="0"
            x2={chart.viewWidth}
            y1={chart.top + chart.priceHeight + 8}
            y2={chart.top + chart.priceHeight + 8}
            vectorEffect="non-scaling-stroke"
          />
          {chart.bars.map((bar) => (
            <g className={bar.up ? "candle up" : "candle down"} key={`${bar.date}-${bar.fullIndex}`}>
              <rect
                className="volume"
                x={bar.x - bar.candleWidth / 2}
                y={bar.volumeTop}
                width={bar.candleWidth}
                height={bar.volumeHeight}
                rx="1.5"
              />
              <line x1={bar.x} x2={bar.x} y1={bar.highY} y2={bar.lowY} vectorEffect="non-scaling-stroke" />
              <rect x={bar.x - bar.candleWidth / 2} y={bar.bodyTop} width={bar.candleWidth} height={bar.bodyHeight} rx="2" />
            </g>
          ))}
          {hoverBar && (
            <g className="crosshair">
              <line x1={hoverBar.x} x2={hoverBar.x} y1="0" y2={chart.viewHeight} vectorEffect="non-scaling-stroke" />
              <line x1="0" x2={chart.viewWidth} y1={hoverBar.closeY} y2={hoverBar.closeY} vectorEffect="non-scaling-stroke" />
              <circle cx={hoverBar.x} cy={hoverBar.closeY} r="5" />
            </g>
          )}
        </svg>
        <div className="kline-axis">
          {chart.priceTicks.map((price) => (
            <span
              key={price}
              style={{ top: `${((chart.top + ((chart.priceMax - price) / (chart.priceMax - chart.priceMin || 1)) * chart.priceHeight) / chart.viewHeight) * 100}%` }}
            >
              {formatNumber(price)}
            </span>
          ))}
        </div>
        {hoverBar && (
          <div className={`kline-tooltip ${hoverBar.x > chart.viewWidth * 0.58 ? "left" : "right"}`}>
            <strong>{formatKLineDate(hoverBar.date, period)}</strong>
            <span>开 {formatNumber(hoverBar.open)}</span>
            <span>高 {formatNumber(hoverBar.high)}</span>
            <span>低 {formatNumber(hoverBar.low)}</span>
            <span>收 {formatNumber(hoverBar.close)}</span>
            <em className={hoverBar.close >= hoverBar.open ? "up" : "down"}>
              {formatPercent(((hoverBar.close - hoverBar.open) / Math.max(Math.abs(hoverBar.open), 0.0001)) * 100)}
            </em>
            <small>量 {formatCompactNumber(hoverBar.volume)}</small>
          </div>
        )}
        {(scrollOffset > 0 || loadingMore) && (
          <button
            className="kline-latest-button"
            disabled={loadingMore}
            onClick={resetToLatest}
            onPointerDown={(event) => event.stopPropagation()}
            type="button"
          >
            {loadingMore ? "更新中" : "最新"}
          </button>
        )}
      </div>
      <div className="kline-time-axis" aria-hidden="true">
        {chart.timeTicks.map((tick) => (
          <span
            className={`anchor-${tick.anchor}`}
            key={`${tick.date}-${tick.x}`}
            style={{ left: `${(tick.x / chart.viewWidth) * 100}%` }}
          >
            {tick.label}
          </span>
        ))}
      </div>
    </div>
  );
}

function PriceStat({ label, value, positive }: { label: string; value: string; positive?: boolean }) {
  return (
    <div className="price-stat">
      <span>{label}</span>
      <strong className={positive === undefined ? "" : positive ? "up" : "down"}>{value}</strong>
    </div>
  );
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function formatPercent(value: number) {
  return `${value >= 0 ? "+" : ""}${value.toFixed(2)}%`;
}

function formatNumber(value: number) {
  const digits = Math.abs(value) > 0 && Math.abs(value) < 1 ? 4 : 2;
  return new Intl.NumberFormat("zh-CN", {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits
  }).format(value);
}

function formatSigned(value: number) {
  return `${value >= 0 ? "+" : ""}${formatNumber(value)}`;
}

function formatCompactNumber(value: number) {
  return new Intl.NumberFormat("zh-CN", {
    notation: "compact",
    maximumFractionDigits: 2
  }).format(value);
}

function formatKLineDate(value: string, period: KLinePeriod) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  if (period === "1d" || period === "1w") {
    return new Intl.DateTimeFormat("zh-CN", { month: "2-digit", day: "2-digit" }).format(date);
  }
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  }).format(date);
}

function formatKLineAxisDate(value: string, period: KLinePeriod) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  if (period === "1d" || period === "1w") {
    return new Intl.DateTimeFormat("zh-CN", { month: "2-digit", day: "2-digit" }).format(date);
  }
  return new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit" }).format(date);
}

function analysisLabel(value: string) {
  if (value === "bullish") return "偏多";
  if (value === "bearish") return "偏空";
  return "中性";
}

function formatTime(value: string) {
  const diff = Date.now() - new Date(value).getTime();
  if (diff < 60 * 60 * 1000) return `${Math.max(1, Math.floor(diff / 60000))} 分钟前`;
  if (diff < 24 * 60 * 60 * 1000) return `${Math.floor(diff / 3600000)} 小时前`;
  return new Intl.DateTimeFormat("zh-CN", { month: "numeric", day: "numeric" }).format(new Date(value));
}
