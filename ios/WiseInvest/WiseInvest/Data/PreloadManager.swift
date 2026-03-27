import Foundation
import Combine

/// Centralized preload & cache manager.
/// Fetches market data in the background so that subsequent screens can display cached data instantly,
/// then silently refresh in the background.
final class PreloadManager {
    static let shared = PreloadManager()

    private let stockService = StockDataService.shared
    private let queue = DispatchQueue(label: "com.wiseinvest.preload", qos: .utility)

    // MARK: - Cache Storage

    /// Cached indices per market
    private var indicesCache: [String: CachedData<[MarketIndex]>] = [:]
    /// Cached watchlist per market
    private var watchlistCache: [String: CachedData<[Stock]>] = [:]
    /// Cached stock quotes (key = "\(market)_\(stockId)")
    private var quoteCache: [String: CachedData<Stock>] = [:]
    /// Cached news per stock (key = "\(market)_\(stockId)")
    private var newsCache: [String: CachedData<[NewsItem]>] = [:]
    /// Cached AI analysis per stock (key = "\(market)_\(stockId)")
    private var analysisCache: [String: CachedData<[AIAnalysisItem]>] = [:]
    /// Cached AI analysis conclusion per stock (key = "\(market)_\(stockId)")
    private var analysisConclusionCache: [String: CachedData<(conclusion: String, summary: String)>] = [:]

    /// Thread-safe access
    private let lock = NSLock()

    /// Track in-flight preloads to avoid duplicate requests
    private var inFlightIndices: Set<String> = []
    private var inFlightWatchlist: Set<String> = []
    private var inFlightQuotes: Set<String> = []
    private var inFlightNews: Set<String> = []

    /// Callbacks waiting for news to become available (key = "\(market)_\(stockId)")
    private var newsWaiters: [String: [([NewsItem]) -> Void]] = [:]

    /// Track in-flight AI enhancement per news item (key = newsItemId)
    private var inFlightEnhance: Set<String> = []
    /// Track in-flight batch analysis conclusion per market
    private var inFlightBatchAnalysis: Set<String> = []
    /// Track in-flight batch analysis enhance per market
    private var inFlightBatchEnhance: Set<String> = []

    private init() {}

    // MARK: - Cache Wrapper

    struct CachedData<T> {
        let data: T
        let timestamp: Date

        var age: TimeInterval { Date().timeIntervalSince(timestamp) }

        /// Data is considered fresh within this window (seconds)
        func isFresh(maxAge: TimeInterval) -> Bool {
            age < maxAge
        }
    }

    // MARK: - Cache TTL Constants

    /// Indices cache: fresh for 30 seconds (real-time data updates frequently)
    private let indicesTTL: TimeInterval = 30
    /// Watchlist cache: fresh for 60 seconds
    private let watchlistTTL: TimeInterval = 60
    /// Quote cache: fresh for 15 seconds
    private let quoteTTL: TimeInterval = 15
    /// News cache: fresh for 5 minutes (news changes infrequently)
    private let newsTTL: TimeInterval = 300

    // MARK: - Preload All Markets (called from HomeView)

    /// Preload indices and watchlist for all markets in the background.
    /// Called when HomeView appears — user will likely tap a market card soon.
    /// After watchlists arrive, immediately kicks off batch news preloading for all stocks.
    func preloadAllMarkets() {
        for market in Market.allCases {
            preloadIndices(for: market)
            preloadWatchlistAndNews(for: market)
        }
    }

    /// Preload indices for a specific market
    func preloadIndices(for market: Market) {
        let key = market.rawValue
        lock.lock()
        // Skip if already in-flight or cache is fresh
        if inFlightIndices.contains(key) {
            lock.unlock()
            return
        }
        if let cached = indicesCache[key], cached.isFresh(maxAge: indicesTTL) {
            lock.unlock()
            return
        }
        inFlightIndices.insert(key)
        lock.unlock()

        stockService.getIndices(for: market) { [weak self] indices in
            guard let self = self else { return }
            self.lock.lock()
            if !indices.isEmpty {
                self.indicesCache[key] = CachedData(data: indices, timestamp: Date())
            }
            self.inFlightIndices.remove(key)
            self.lock.unlock()
        }
    }

    /// Preload watchlist for a specific market, then batch-preload news for all stocks.
    /// Also triggers batch comprehensive analysis after watchlist arrives.
    private func preloadWatchlistAndNews(for market: Market) {
        let key = market.rawValue
        lock.lock()
        let hasFreshWatchlist = watchlistCache[key]?.isFresh(maxAge: watchlistTTL) ?? false
        let hasFreshNews = hasAllNewsFresh(for: market)
        let cachedStocks = watchlistCache[key]?.data ?? []
        lock.unlock()

        // Always trigger analysis for cached watchlist (even if news are fresh)
        if hasFreshWatchlist && !cachedStocks.isEmpty {
            batchPreloadAnalysisEnhance(stocks: cachedStocks, market: market)
        }

        // If watchlist is fresh and all news are fresh, skip news preload
        if hasFreshWatchlist && hasFreshNews {
            return
        }

        // If watchlist is fresh but news need refreshing, use cached watchlist
        if hasFreshWatchlist {
            if !cachedStocks.isEmpty {
                batchPreloadNews(stocks: cachedStocks, market: market)
            }
            return
        }

        // Fetch watchlist, then batch news + analysis
        lock.lock()
        if inFlightWatchlist.contains(key) {
            lock.unlock()
            return
        }
        inFlightWatchlist.insert(key)
        lock.unlock()

        stockService.getWatchlist(for: market) { [weak self] stocks in
            guard let self = self else { return }
            self.lock.lock()
            self.watchlistCache[key] = CachedData(data: stocks, timestamp: Date())
            self.inFlightWatchlist.remove(key)
            self.lock.unlock()

            // Immediately preload news for ALL watchlist stocks via batch API
            if !stocks.isEmpty {
                self.batchPreloadNews(stocks: stocks, market: market)
                // Trigger comprehensive AI analysis for all stocks
                self.batchPreloadAnalysisEnhance(stocks: stocks, market: market)
            }
        }
    }

    /// Preload watchlist only (without news), used for cache warming
    func preloadWatchlist(for market: Market) {
        let key = market.rawValue
        lock.lock()
        if inFlightWatchlist.contains(key) {
            lock.unlock()
            return
        }
        if let cached = watchlistCache[key], cached.isFresh(maxAge: watchlistTTL) {
            lock.unlock()
            return
        }
        inFlightWatchlist.insert(key)
        lock.unlock()

        stockService.getWatchlist(for: market) { [weak self] stocks in
            guard let self = self else { return }
            self.lock.lock()
            self.watchlistCache[key] = CachedData(data: stocks, timestamp: Date())
            self.inFlightWatchlist.remove(key)
            self.lock.unlock()
        }
    }

    // MARK: - Preload Stock Detail Data (called from MarketDetailView)

    /// Preload quote data for stocks in the watchlist.
    /// Called when watchlist data arrives so the user gets instant data when tapping a stock.
    func preloadWatchlistQuotes(stocks: [Stock], market: Market) {
        for stock in stocks {
            preloadQuote(for: stock, market: market)
        }
    }

    /// Preload a single stock's quote
    func preloadQuote(for stock: Stock, market: Market) {
        let key = "\(market.rawValue)_\(stock.id)"
        lock.lock()
        if inFlightQuotes.contains(key) {
            lock.unlock()
            return
        }
        if let cached = quoteCache[key], cached.isFresh(maxAge: quoteTTL) {
            lock.unlock()
            return
        }
        inFlightQuotes.insert(key)
        lock.unlock()

        stockService.getStockQuote(code: stock.id, market: market) { [weak self] updated in
            guard let self = self else { return }
            self.lock.lock()
            if let updated = updated {
                self.quoteCache[key] = CachedData(data: updated, timestamp: Date())
            }
            self.inFlightQuotes.remove(key)
            self.lock.unlock()
        }
    }

    /// Preload news for a stock (the slowest endpoint ~9s).
    /// Called when user is on MarketDetailView — by the time they tap a stock,
    /// news may already be cached.
    func preloadNews(for stock: Stock, market: Market) {
        let key = "\(market.rawValue)_\(stock.id)"
        lock.lock()
        if inFlightNews.contains(key) {
            lock.unlock()
            return
        }
        if let cached = newsCache[key], cached.isFresh(maxAge: newsTTL) {
            lock.unlock()
            return
        }
        inFlightNews.insert(key)
        lock.unlock()

        stockService.getNews(for: stock) { [weak self] news in
            guard let self = self else { return }
            self.lock.lock()
            if !news.isEmpty {
                self.newsCache[key] = CachedData(data: news, timestamp: Date())
            }
            self.inFlightNews.remove(key)
            let waiters = self.newsWaiters.removeValue(forKey: key) ?? []
            self.lock.unlock()
            // Fire callbacks outside lock
            for callback in waiters {
                DispatchQueue.main.async { callback(news) }
            }
        }
    }

    /// Batch preload news for ALL stocks via the batch API endpoint.
    /// After news arrive, immediately kick off parallel AI enhancement for each news item.
    func batchPreloadNews(stocks: [Stock], market: Market) {
        // Filter out stocks that already have fresh news cache
        lock.lock()
        let uncachedStocks = stocks.filter { stock in
            let key = "\(market.rawValue)_\(stock.id)"
            if inFlightNews.contains(key) { return false }
            if let cached = newsCache[key], cached.isFresh(maxAge: newsTTL) { return false }
            return true
        }
        // Mark all as in-flight
        for stock in uncachedStocks {
            inFlightNews.insert("\(market.rawValue)_\(stock.id)")
        }
        lock.unlock()

        // If all stocks have fresh cache, just trigger enhancement for those missing analysis
        if uncachedStocks.isEmpty {
            enhanceAllCachedNews(stocks: stocks, market: market)
            return
        }

        let batchInput = uncachedStocks.map { (code: $0.id, name: $0.name, market: market.rawValue) }

        stockService.getBatchNews(stocks: batchInput) { [weak self] results in
            guard let self = self else { return }
            self.lock.lock()
            var pendingCallbacks: [(key: String, news: [NewsItem], callbacks: [([NewsItem]) -> Void])] = []
            for stock in uncachedStocks {
                let key = "\(market.rawValue)_\(stock.id)"
                let news = results[key] ?? []
                if !news.isEmpty {
                    self.newsCache[key] = CachedData(data: news, timestamp: Date())
                }
                self.inFlightNews.remove(key)
                // Collect waiters
                if let waiters = self.newsWaiters.removeValue(forKey: key), !waiters.isEmpty {
                    pendingCallbacks.append((key: key, news: news, callbacks: waiters))
                }
            }
            self.lock.unlock()
            // Fire callbacks outside lock
            for item in pendingCallbacks {
                for callback in item.callbacks {
                    DispatchQueue.main.async { callback(item.news) }
                }
            }

            // Now kick off parallel AI enhancement for ALL stocks (including previously cached)
            self.enhanceAllCachedNews(stocks: stocks, market: market)
        }
    }

    // MARK: - Parallel Per-Item AI Enhancement

    /// For each stock's cached news, fire off individual enhance requests with concurrency limit.
    /// Each news item that lacks analysis gets its own request, but at most 3 run concurrently
    /// to avoid overwhelming the DeepSeek LLM backend.
    private func enhanceAllCachedNews(stocks: [Stock], market: Market) {
        lock.lock()
        // Build a flat list of (stockId, stockName, newsItem) that need enhancement
        var itemsToEnhance: [(stockId: String, stockName: String, news: NewsItem)] = []
        for stock in stocks {
            let key = "\(market.rawValue)_\(stock.id)"
            guard let cached = newsCache[key], !cached.data.isEmpty else { continue }
            for item in cached.data {
                if item.analysis.isEmpty && !inFlightEnhance.contains(item.id) {
                    inFlightEnhance.insert(item.id)
                    itemsToEnhance.append((stockId: stock.id, stockName: stock.name, news: item))
                }
            }
        }
        lock.unlock()

        guard !itemsToEnhance.isEmpty else { return }

        // Limit concurrency to 3 to avoid DeepSeek overload
        let newsSemaphore = DispatchSemaphore(value: 3)
        let newsEnhanceQueue = DispatchQueue(label: "com.wiseinvest.news-enhance", qos: .utility)

        for entry in itemsToEnhance {
            newsEnhanceQueue.async {
                newsSemaphore.wait()
                let group = DispatchGroup()
                group.enter()

                self.stockService.enhanceNewsItem(
                    newsID: entry.news.id,
                    code: entry.stockId,
                    market: market.rawValue,
                    name: entry.stockName,
                    title: entry.news.title,
                    summary: entry.news.summary,
                    source: entry.news.source
                ) { [weak self] summary, analysis, sentimentStr in
                    defer {
                        group.leave()
                        newsSemaphore.signal()
                    }
                    guard let self = self else { return }
                    let sentiment = NewsSentiment(rawValue: sentimentStr) ?? entry.news.sentiment

                    self.lock.lock()
                    self.inFlightEnhance.remove(entry.news.id)

                    // Update the news item in cache
                    let cacheKey = "\(market.rawValue)_\(entry.stockId)"
                    if var cached = self.newsCache[cacheKey] {
                        var updatedNews = cached.data
                        if let idx = updatedNews.firstIndex(where: { $0.id == entry.news.id }) {
                            let old = updatedNews[idx]
                            updatedNews[idx] = NewsItem(
                                id: old.id,
                                title: old.title,
                                source: old.source,
                                time: old.time,
                                summary: summary.isEmpty ? old.summary : summary,
                                analysis: analysis,
                                sentiment: sentiment,
                                url: old.url
                            )
                        }
                        self.newsCache[cacheKey] = CachedData(data: updatedNews, timestamp: cached.timestamp)
                    }
                    self.lock.unlock()

                    // Notify observers (StockDetailView) that enhanced data is available
                    if !analysis.isEmpty {
                        DispatchQueue.main.async {
                            NotificationCenter.default.post(name: .newsEnhancementDidComplete, object: nil)
                        }
                    }
                }

                group.wait()
            }
        }
    }

    /// Check if all watchlist stocks for a market have fresh news cache
    private func hasAllNewsFresh(for market: Market) -> Bool {
        guard let watchlist = watchlistCache[market.rawValue]?.data, !watchlist.isEmpty else {
            return false
        }
        for stock in watchlist {
            let key = "\(market.rawValue)_\(stock.id)"
            guard let cached = newsCache[key], cached.isFresh(maxAge: newsTTL) else {
                return false
            }
        }
        return true
    }

    // MARK: - Cache Retrieval

    /// Get cached indices. Returns nil if no cache or cache expired.
    func getCachedIndices(for market: Market) -> [MarketIndex]? {
        lock.lock()
        defer { lock.unlock() }
        guard let cached = indicesCache[market.rawValue] else { return nil }
        // Return even slightly stale data — caller will refresh in background
        return cached.data.isEmpty ? nil : cached.data
    }

    /// Get cached watchlist.
    func getCachedWatchlist(for market: Market) -> [Stock]? {
        lock.lock()
        defer { lock.unlock() }
        guard let cached = watchlistCache[market.rawValue] else { return nil }
        return cached.data
    }

    /// Get cached stock quote.
    func getCachedQuote(stockId: String, market: Market) -> Stock? {
        lock.lock()
        defer { lock.unlock() }
        let key = "\(market.rawValue)_\(stockId)"
        return quoteCache[key]?.data
    }

    /// Get cached news.
    func getCachedNews(stockId: String, market: Market) -> [NewsItem]? {
        lock.lock()
        defer { lock.unlock() }
        let key = "\(market.rawValue)_\(stockId)"
        guard let cached = newsCache[key] else { return nil }
        return cached.data.isEmpty ? nil : cached.data
    }

    /// Wait for news to become available if a preload is in-flight,
    /// or return cached news immediately. If no preload is in progress and no cache,
    /// returns nil (caller should fetch directly).
    ///
    /// - Returns: `true` if we registered a waiter (callback will fire later),
    ///            `false` if we have cache (already set in the callback) or no preload in progress.
    func getOrWaitForNews(stockId: String, market: Market, callback: @escaping ([NewsItem]?) -> Void) -> Bool {
        let key = "\(market.rawValue)_\(stockId)"
        lock.lock()
        // Already cached — return immediately
        if let cached = newsCache[key], !cached.data.isEmpty {
            let data = cached.data
            lock.unlock()
            DispatchQueue.main.async { callback(data) }
            return false
        }
        // Preload is in-flight — register as a waiter
        if inFlightNews.contains(key) {
            if newsWaiters[key] == nil {
                newsWaiters[key] = []
            }
            newsWaiters[key]!.append { news in
                callback(news.isEmpty ? nil : news)
            }
            lock.unlock()
            return true
        }
        // No cache, no in-flight preload
        lock.unlock()
        callback(nil)
        return false
    }

    /// Check if cached data is still fresh
    func isIndicesFresh(for market: Market) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return indicesCache[market.rawValue]?.isFresh(maxAge: indicesTTL) ?? false
    }

    func isWatchlistFresh(for market: Market) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return watchlistCache[market.rawValue]?.isFresh(maxAge: watchlistTTL) ?? false
    }

    func isNewsFresh(stockId: String, market: Market) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        let key = "\(market.rawValue)_\(stockId)"
        return newsCache[key]?.isFresh(maxAge: newsTTL) ?? false
    }

    // MARK: - Cache Update (called after successful network requests from ViewModels)

    /// Update indices cache (called by MarketDetailViewModel after successful load)
    func updateIndicesCache(for market: Market, data: [MarketIndex]) {
        lock.lock()
        indicesCache[market.rawValue] = CachedData(data: data, timestamp: Date())
        lock.unlock()
    }

    /// Update watchlist cache
    func updateWatchlistCache(for market: Market, data: [Stock]) {
        lock.lock()
        watchlistCache[market.rawValue] = CachedData(data: data, timestamp: Date())
        lock.unlock()
    }

    /// Update quote cache
    func updateQuoteCache(stock: Stock, market: Market) {
        lock.lock()
        let key = "\(market.rawValue)_\(stock.id)"
        quoteCache[key] = CachedData(data: stock, timestamp: Date())
        lock.unlock()
    }

    /// Update news cache
    func updateNewsCache(stockId: String, market: Market, data: [NewsItem]) {
        lock.lock()
        let key = "\(market.rawValue)_\(stockId)"
        newsCache[key] = CachedData(data: data, timestamp: Date())
        lock.unlock()
    }

    // MARK: - Invalidation

    /// Invalidate watchlist cache for a market (called after add/remove operations)
    func invalidateWatchlistCache(for market: Market) {
        lock.lock()
        watchlistCache.removeValue(forKey: market.rawValue)
        lock.unlock()
    }

    // MARK: - AI Enhancement Check

    /// Check if AI enhancement is in-flight for a specific news item
    func isEnhancementInFlight(newsId: String) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return inFlightEnhance.contains(newsId)
    }

    /// Clear all caches (e.g. on logout)
    func clearAll() {
        lock.lock()
        indicesCache.removeAll()
        watchlistCache.removeAll()
        quoteCache.removeAll()
        newsCache.removeAll()
        analysisCache.removeAll()
        analysisConclusionCache.removeAll()
        inFlightEnhance.removeAll()
        inFlightBatchAnalysis.removeAll()
        inFlightBatchEnhance.removeAll()
        lock.unlock()
    }

    // MARK: - AI Analysis Cache

    /// TTL for analysis cache: 15 minutes
    private let analysisTTL: TimeInterval = 900

    /// Get cached analysis items for a stock
    func getCachedAnalysis(stockId: String, market: Market) -> [AIAnalysisItem]? {
        lock.lock()
        defer { lock.unlock() }
        let key = "\(market.rawValue)_\(stockId)"
        guard let cached = analysisCache[key], cached.isFresh(maxAge: analysisTTL) else { return nil }
        return cached.data.isEmpty ? nil : cached.data
    }

    /// Update analysis cache
    func updateAnalysisCache(stockId: String, market: Market, data: [AIAnalysisItem]) {
        lock.lock()
        let key = "\(market.rawValue)_\(stockId)"
        analysisCache[key] = CachedData(data: data, timestamp: Date())
        lock.unlock()
    }

    /// Update a single analysis item in the cache (after AI enhancement completes)
    func updateSingleAnalysis(stockId: String, market: Market, analysisId: String,
                               conclusion: String, summary: String, detail: String) {
        lock.lock()
        let key = "\(market.rawValue)_\(stockId)"
        guard var cached = analysisCache[key] else {
            lock.unlock()
            return
        }
        var items = cached.data
        if let idx = items.firstIndex(where: { $0.id == analysisId }) {
            items[idx].conclusion = conclusion
            items[idx].aiSummary = summary
            items[idx].detail = detail
        }
        analysisCache[key] = CachedData(data: items, timestamp: cached.timestamp)
        lock.unlock()

        // Notify observers
        DispatchQueue.main.async {
            NotificationCenter.default.post(name: .analysisEnhancementDidComplete, object: nil)
        }
    }

    // MARK: - Analysis Conclusion Cache

    /// Get cached analysis conclusion for a stock
    func getCachedAnalysisConclusion(stockId: String, market: Market) -> (conclusion: String, summary: String)? {
        lock.lock()
        defer { lock.unlock() }
        let key = "\(market.rawValue)_\(stockId)"
        guard let cached = analysisConclusionCache[key], cached.isFresh(maxAge: analysisTTL) else { return nil }
        return cached.data
    }

    /// Update analysis conclusion cache for a stock
    func updateAnalysisConclusionCache(stockId: String, market: Market, conclusion: String, summary: String) {
        lock.lock()
        let key = "\(market.rawValue)_\(stockId)"
        analysisConclusionCache[key] = CachedData(data: (conclusion: conclusion, summary: summary), timestamp: Date())
        lock.unlock()
    }

    // MARK: - Batch Analysis Conclusion Preload

    /// Preload AI analysis conclusions for all stocks (indices + watchlist) when entering a market section.
    /// All stocks are processed in a single batch request to the backend, which handles them in parallel.
    func batchPreloadAnalysisConclusions(stocks: [Stock], market: Market) {
        let marketKey = market.rawValue
        lock.lock()
        if inFlightBatchAnalysis.contains(marketKey) {
            lock.unlock()
            return
        }
        // Filter out stocks that already have fresh conclusion cache
        let uncachedStocks = stocks.filter { stock in
            let key = "\(market.rawValue)_\(stock.id)"
            guard let cached = analysisConclusionCache[key], cached.isFresh(maxAge: analysisTTL) else { return true }
            return false
        }
        if uncachedStocks.isEmpty {
            lock.unlock()
            return
        }
        inFlightBatchAnalysis.insert(marketKey)
        lock.unlock()

        // Build batch input — use basic price info since we don't have full K-line data at this stage
        let batchInput = uncachedStocks.map { stock -> (code: String, name: String, market: String, klineSummary: String, priceSummary: String) in
            let priceSummary = "当前价\(stock.priceText)，涨跌\(stock.changeText)（\(stock.changePercentText)），高\(String(format: "%.2f", stock.high))，低\(String(format: "%.2f", stock.low))，量\(String(format: "%.1f亿", stock.volume))"
            return (code: stock.id, name: stock.name, market: market.rawValue, klineSummary: "暂无K线数据", priceSummary: priceSummary)
        }

        stockService.batchAnalysisConclusion(stocks: batchInput) { [weak self] results in
            guard let self = self else { return }
            self.lock.lock()
            self.inFlightBatchAnalysis.remove(marketKey)
            for result in results {
                let key = "\(result.market)_\(result.code)"
                self.analysisConclusionCache[key] = CachedData(
                    data: (conclusion: result.conclusion, summary: result.summary),
                    timestamp: Date()
                )
            }
            self.lock.unlock()

            // Notify observers
            DispatchQueue.main.async {
                NotificationCenter.default.post(name: .analysisConclusionBatchDidComplete, object: nil)
            }
        }
    }

    // MARK: - Batch Enhance Analysis Preload (comprehensive analysis for all stocks)

    /// Trigger comprehensive AI analysis for all stocks with concurrency limit.
    /// Each stock gets its own independent enhanceAnalysis request, but at most 2 run
    /// concurrently to avoid overwhelming the DeepSeek LLM backend (which would cause
    /// all requests to time out).
    func batchPreloadAnalysisEnhance(stocks: [Stock], market: Market) {
        lock.lock()

        // Filter stocks that don't have a comprehensive analysis cached
        let uncachedStocks = stocks.filter { stock in
            let key = "\(market.rawValue)_\(stock.id)"
            guard let cached = analysisCache[key], cached.isFresh(maxAge: analysisTTL) else { return true }
            let items = cached.data
            return !items.contains(where: { $0.id == "comprehensive" && !$0.detail.isEmpty })
        }

        // Also filter out stocks already in-flight
        let toProcess = uncachedStocks.filter { stock in
            let flightKey = "\(market.rawValue)_\(stock.id)_enhance"
            if inFlightBatchEnhance.contains(flightKey) { return false }
            inFlightBatchEnhance.insert(flightKey)
            return true
        }

        lock.unlock()

        guard !toProcess.isEmpty else { return }

        // Use a serial DispatchQueue + semaphore to limit concurrency to 2
        let enhanceSemaphore = DispatchSemaphore(value: 2)
        let enhanceQueue = DispatchQueue(label: "com.wiseinvest.enhance-preload", qos: .utility)

        for stock in toProcess {
            enhanceQueue.async {
                enhanceSemaphore.wait()
                let group = DispatchGroup()
                group.enter()

                self.stockService.enhanceAnalysis(
                    code: stock.id, market: market.rawValue, name: stock.name
                ) { [weak self] conclusionStr, summary, detail in
                    defer {
                        group.leave()
                        enhanceSemaphore.signal()
                    }
                    guard let self = self else { return }

                    let flightKey = "\(market.rawValue)_\(stock.id)_enhance"
                    self.lock.lock()
                    self.inFlightBatchEnhance.remove(flightKey)

                    if !detail.isEmpty {
                        let key = "\(market.rawValue)_\(stock.id)"
                        var items = self.analysisCache[key]?.data ?? []

                        if let idx = items.firstIndex(where: { $0.id == "comprehensive" }) {
                            items[idx].conclusion = conclusionStr
                            items[idx].aiSummary = summary
                            items[idx].detail = detail
                        } else {
                            var item = AIAnalysisItem(
                                id: "comprehensive",
                                title: "AI 综合分析",
                                icon: "brain.head.profile",
                                content: summary
                            )
                            item.conclusion = conclusionStr
                            item.aiSummary = summary
                            item.detail = detail
                            items.append(item)
                        }

                        self.analysisCache[key] = CachedData(data: items, timestamp: Date())
                        // Also update conclusion cache
                        self.analysisConclusionCache[key] = CachedData(
                            data: (conclusion: conclusionStr, summary: summary),
                            timestamp: Date()
                        )
                    }
                    self.lock.unlock()

                    // Notify observers immediately when each stock completes
                    if !detail.isEmpty {
                        DispatchQueue.main.async {
                            NotificationCenter.default.post(name: .analysisEnhancementDidComplete, object: nil)
                            NotificationCenter.default.post(name: .analysisConclusionBatchDidComplete, object: nil)
                        }
                    }
                }

                group.wait()
            }
        }
    }
}

// MARK: - Notification for news enhancement completion

extension Notification.Name {
    /// Posted when an AI enhancement finishes and newsCache is updated with analysis.
    /// Observers (e.g. StockDetailView) should refresh their newsItems from cache.
    static let newsEnhancementDidComplete = Notification.Name("newsEnhancementDidComplete")
    /// Posted when an AI analysis enhancement finishes for a stock.
    static let analysisEnhancementDidComplete = Notification.Name("analysisEnhancementDidComplete")
    /// Posted when batch analysis conclusions are preloaded for a market.
    static let analysisConclusionBatchDidComplete = Notification.Name("analysisConclusionBatchDidComplete")
}
