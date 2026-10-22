import SwiftUI
import Combine

/// ViewModel for the Market Detail screen — uses WebSocket for real-time data push,
/// with HTTP API fallback for initial load and watchlist operations.
class MarketDetailViewModel: ObservableObject {
    let market: Market
    private let stockService = StockDataService.shared
    private let wsClient = WebSocketClient.shared

    @Published var indices: [MarketIndex] = []
    @Published var watchlist: [Stock] = []
    @Published var searchResults: [Stock] = []
    @Published var searchText: String = ""
    @Published var isSearching: Bool = false
    @Published var isLoadingIndices: Bool = false
    @Published var isLoadingWatchlist: Bool = false

    /// Current trading status for UI display
    @Published var tradingStatus: MarketTradingHours.TradingStatus = .closed("已收盘")

    /// Track which stock IDs are in the user's watchlist (for fast lookup)
    @Published var watchlistIDs: Set<String> = []

    private var cancellables = Set<AnyCancellable>()
    /// Unique token per ViewModel instance — callbacks check this to avoid stale updates
    private let instanceID = UUID()
    private var isInvalidated = false

    /// Counter for in-flight watchlist mutations (add/remove).
    /// loadWatchlist waits for pending mutations to finish before overwriting.
    private var pendingMutations = 0

    /// Timer to periodically check if trading hours changed (every 60s)
    private var tradingCheckTimer: Timer?

    /// Fallback HTTP refresh timer — used only when WebSocket is disconnected
    private var httpFallbackTimer: Timer?

    /// Monotonically increasing counter — each new request gets the next value.
    private var indicesCounter: UInt64 = 0
    /// High-water mark: the highest sequence number whose response has been accepted.
    /// Only responses with seq > highWaterMark are applied; others are stale and discarded.
    private var indicesHighWaterMark: UInt64 = 0

    /// Same pair for watchlist requests.
    private var watchlistCounter: UInt64 = 0
    private var watchlistHighWaterMark: UInt64 = 0

    /// Whether the initial data has been loaded (set to true after first `onAppear`).
    private var hasAppeared = false

    private let preloadManager = PreloadManager.shared

    init(market: Market) {
        self.market = market

        // Instantly populate from preload cache (no network wait)
        if let cachedIndices = preloadManager.getCachedIndices(for: market) {
            self.indices = cachedIndices
        }
        if let cachedWatchlist = preloadManager.getCachedWatchlist(for: market) {
            self.watchlist = cachedWatchlist
            self.watchlistIDs = Set(cachedWatchlist.map { $0.id })
        }

        // Debounced search
        $searchText
            .debounce(for: .milliseconds(300), scheduler: DispatchQueue.main)
            .removeDuplicates()
            .sink { [weak self] query in
                self?.performSearch(query: query)
            }
            .store(in: &cancellables)

        // Subscribe to WebSocket events
        setupWebSocketSubscription()
    }

    deinit {
        // Mark as invalidated so any in-flight callbacks are discarded
        isInvalidated = true
        stopAutoRefresh()
        cancellables.removeAll()
    }

    // MARK: - WebSocket Subscription

    private func setupWebSocketSubscription() {
        wsClient.eventPublisher
            .receive(on: DispatchQueue.main)
            .sink { [weak self] event in
                guard let self = self, !self.isInvalidated else { return }
                self.handleWSEvent(event)
            }
            .store(in: &cancellables)
    }

    private func handleWSEvent(_ event: WSEvent) {
        switch event {
        case .indices(let wsMarket, let data):
            guard wsMarket == market.rawValue, !data.isEmpty else { return }
            self.indices = data
            self.isLoadingIndices = false
            self.preloadManager.updateIndicesCache(for: self.market, data: data)

        case .quote(let wsMarket, let code, let stock):
            guard wsMarket == market.rawValue else { return }
            // Update the matching watchlist stock in-place
            if let idx = watchlist.firstIndex(where: { $0.id == code }) {
                watchlist[idx].currentPrice = stock.currentPrice
                watchlist[idx].change = stock.change
                watchlist[idx].changePercent = stock.changePercent
                if stock.volume > 0 { watchlist[idx].volume = stock.volume }
                if stock.high > 0 { watchlist[idx].high = stock.high }
                if stock.low > 0 { watchlist[idx].low = stock.low }
            }

        case .connected:
            // WebSocket connected — subscribe to channels and stop fallback timer
            subscribeToWSChannels()
            httpFallbackTimer?.invalidate()
            httpFallbackTimer = nil

        case .disconnected:
            // WebSocket lost — start fallback HTTP polling
            startHTTPFallback()
        }
    }

    /// Subscribe to WebSocket channels for this market
    private func subscribeToWSChannels() {
        // Always subscribe to indices
        wsClient.subscribeIndices(market: market.rawValue)

        // Subscribe to individual quote channels for each watchlist stock
        for stock in watchlist {
            wsClient.subscribeQuote(market: market.rawValue, code: stock.id)
        }
    }

    /// Unsubscribe from all WebSocket channels for this market
    private func unsubscribeFromWSChannels() {
        wsClient.unsubscribeIndices(market: market.rawValue)
        for stock in watchlist {
            wsClient.unsubscribeQuote(market: market.rawValue, code: stock.id)
        }
    }

    // MARK: - Lifecycle (called by View)

    /// Called from the View's `.onAppear`. Performs first load + starts WebSocket subscription.
    func onViewAppear() {
        tradingStatus = MarketTradingHours.tradingStatus(market: market)

        if !hasAppeared {
            hasAppeared = true
            loadData()
        }
        startAutoRefresh()
    }

    // MARK: - Auto Refresh (WebSocket-based + HTTP fallback)

    /// Start WebSocket subscription + trading status check timer.
    func startAutoRefresh() {
        stopAutoRefresh()

        // Update trading status immediately
        tradingStatus = MarketTradingHours.tradingStatus(market: market)

        // Connect WebSocket (idempotent if already connected)
        wsClient.connect()

        // Subscribe to market data channels
        subscribeToWSChannels()

        // Check trading status every 60 seconds
        tradingCheckTimer = Timer.scheduledTimer(withTimeInterval: 60, repeats: true) { [weak self] _ in
            guard let self = self, !self.isInvalidated else { return }
            let newStatus = MarketTradingHours.tradingStatus(market: self.market)
            self.tradingStatus = newStatus
        }

        // If WebSocket is not connected, start HTTP fallback immediately
        if !wsClient.isConnected {
            startHTTPFallback()
        }
    }

    func stopAutoRefresh() {
        tradingCheckTimer?.invalidate()
        tradingCheckTimer = nil
        httpFallbackTimer?.invalidate()
        httpFallbackTimer = nil
        // Note: we don't unsubscribe from WebSocket here because other ViewModels
        // for different markets may still need the connection. The WebSocket client
        // manages its own lifecycle.
        unsubscribeFromWSChannels()
    }

    /// HTTP fallback polling — only used when WebSocket is disconnected.
    private func startHTTPFallback() {
        httpFallbackTimer?.invalidate()
        guard let interval = MarketTradingHours.refreshInterval(market: market) else { return }
        httpFallbackTimer = Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { [weak self] _ in
            guard let self = self, !self.isInvalidated else { return }
            // Stop fallback if WebSocket reconnected
            if self.wsClient.isConnected {
                self.httpFallbackTimer?.invalidate()
                self.httpFallbackTimer = nil
                return
            }
            self.refreshLiveDataHTTP()
        }
    }

    /// Lightweight HTTP refresh (fallback when WS is down): only update indices + watchlist quotes.
    private func refreshLiveDataHTTP() {
        // --- Indices ---
        indicesCounter &+= 1
        let idxSeq = indicesCounter
        stockService.getIndices(for: market) { [weak self] indices in
            guard let self = self, !self.isInvalidated else { return }
            guard idxSeq > self.indicesHighWaterMark else { return }
            self.indicesHighWaterMark = idxSeq
            if !indices.isEmpty {
                self.indices = indices
                self.preloadManager.updateIndicesCache(for: self.market, data: indices)
            }
        }

        // --- Watchlist ---
        if !watchlist.isEmpty && pendingMutations == 0 {
            watchlistCounter &+= 1
            let wlSeq = watchlistCounter
            stockService.getWatchlist(for: market) { [weak self] stocks in
                guard let self = self, !self.isInvalidated else { return }
                guard wlSeq > self.watchlistHighWaterMark else { return }
                self.watchlistHighWaterMark = wlSeq
                if !stocks.isEmpty || self.watchlist.isEmpty {
                    self.watchlist = stocks
                    self.watchlistIDs = Set(stocks.map { $0.id })
                    self.preloadManager.updateWatchlistCache(for: self.market, data: stocks)
                }
            }
        }
    }

    func loadData() {
        loadIndices()
        loadWatchlist()
    }

    private func loadIndices() {
        // If we already have cached data, skip showing skeleton
        isLoadingIndices = indices.isEmpty
        indicesCounter &+= 1
        let capturedSeq = indicesCounter
        stockService.getIndices(for: market) { [weak self] indices in
            guard let self = self, !self.isInvalidated else { return }
            guard capturedSeq > self.indicesHighWaterMark else { return }
            self.indicesHighWaterMark = capturedSeq
            self.indices = indices
            self.isLoadingIndices = false
            // Update preload cache
            if !indices.isEmpty {
                self.preloadManager.updateIndicesCache(for: self.market, data: indices)
                // Convert indices to Stock-like objects and preload AI analysis conclusions
                let indexStocks = indices.map { index in
                    Stock(id: index.id, symbol: index.id.uppercased(), name: index.name,
                          market: self.market.rawValue, currentPrice: index.value,
                          change: index.change, changePercent: index.changePercent,
                          volume: 0, high: index.value, low: index.value,
                          open: index.value, previousClose: index.value - index.change, isIndex: true)
                }
                self.preloadManager.batchPreloadAnalysisConclusions(stocks: indexStocks, market: self.market)
                // Also trigger batch deep analysis (tech/trend/volume) for all indices
                self.preloadManager.batchPreloadAnalysisEnhance(stocks: indexStocks, market: self.market)
            }
        }
    }

    /// Flag: set when add/remove mutation succeeds, cleared after successful remote refresh
    private var hasPendingChanges = false

    func loadWatchlist() {
        // If there are in-flight add/remove mutations, delay the refresh so the
        // backend has time to persist the changes before we overwrite the
        // optimistic state with the server response.
        if pendingMutations > 0 {
            DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) { [weak self] in
                self?.forceLoadWatchlist()
            }
            return
        }
        forceLoadWatchlist()
    }

    private func forceLoadWatchlist() {
        // If we already have cached data, skip showing skeleton
        isLoadingWatchlist = watchlist.isEmpty
        watchlistCounter &+= 1
        let capturedSeq = watchlistCounter
        stockService.getWatchlist(for: market) { [weak self] stocks in
            guard let self = self, !self.isInvalidated else { return }
            guard capturedSeq > self.watchlistHighWaterMark else { return }
            self.watchlistHighWaterMark = capturedSeq
            // If we had pending changes and the server returned empty but we have local data,
            // it likely means the server hasn't caught up yet — keep local state and retry.
            if stocks.isEmpty && !self.watchlist.isEmpty && self.hasPendingChanges {
                self.isLoadingWatchlist = false
                DispatchQueue.main.asyncAfter(deadline: .now() + 2.0) { [weak self] in
                    guard let self = self, !self.isInvalidated else { return }
                    self.hasPendingChanges = false // avoid infinite retry
                    self.forceLoadWatchlist()
                }
                return
            }
            self.hasPendingChanges = false
            self.watchlist = stocks
            self.watchlistIDs = Set(stocks.map { $0.id })
            self.isLoadingWatchlist = false
            // Update preload cache and preload stock details
            self.preloadManager.updateWatchlistCache(for: self.market, data: stocks)
            self.preloadManager.preloadWatchlistQuotes(stocks: stocks, market: self.market)
            // Batch preload news for ALL watchlist stocks (backend processes in parallel)
            self.preloadManager.batchPreloadNews(stocks: stocks, market: self.market)
            // Batch preload AI analysis conclusions for all watchlist stocks
            self.preloadManager.batchPreloadAnalysisConclusions(stocks: stocks, market: self.market)
            // Also trigger batch deep analysis (tech/trend/volume) for all watchlist stocks
            self.preloadManager.batchPreloadAnalysisEnhance(stocks: stocks, market: self.market)

            // Subscribe to quote channels for watchlist stocks via WebSocket
            for stock in stocks {
                self.wsClient.subscribeQuote(market: self.market.rawValue, code: stock.id)
            }
        }
    }

    func addToWatchlist(_ stock: Stock) {
        // Optimistic update
        if !watchlistIDs.contains(stock.id) {
            watchlist.append(stock)
            watchlistIDs.insert(stock.id)
        }
        pendingMutations += 1
        hasPendingChanges = true
        preloadManager.invalidateWatchlistCache(for: market)
        // Subscribe to WebSocket quote for the new stock
        wsClient.subscribeQuote(market: market.rawValue, code: stock.id)
        stockService.addToWatchlist(stock, for: market) { [weak self] success in
            guard let self = self else { return }
            self.pendingMutations = max(0, self.pendingMutations - 1)
            if !success {
                // Revert on failure
                self.watchlist.removeAll { $0.id == stock.id }
                self.watchlistIDs.remove(stock.id)
                self.wsClient.unsubscribeQuote(market: self.market.rawValue, code: stock.id)
            }
        }
    }

    func removeFromWatchlist(_ stockId: String) {
        // Optimistic update
        watchlist.removeAll { $0.id == stockId }
        watchlistIDs.remove(stockId)
        pendingMutations += 1
        hasPendingChanges = true
        preloadManager.invalidateWatchlistCache(for: market)
        // Unsubscribe from WebSocket quote
        wsClient.unsubscribeQuote(market: market.rawValue, code: stockId)
        stockService.removeFromWatchlist(stockId, for: market) { [weak self] success in
            guard let self = self else { return }
            self.pendingMutations = max(0, self.pendingMutations - 1)
            if !success {
                // Reload on failure
                self.loadWatchlist()
            }
        }
    }

    func isInWatchlist(_ stockId: String) -> Bool {
        watchlistIDs.contains(stockId)
    }

    private func performSearch(query: String) {
        if query.isEmpty {
            searchResults = []
            return
        }
        isSearching = true
        stockService.searchStocks(query: query, market: market) { [weak self] results in
            guard let self = self, !self.isInvalidated else { return }
            self.searchResults = results
            self.isSearching = false
        }
    }
}
