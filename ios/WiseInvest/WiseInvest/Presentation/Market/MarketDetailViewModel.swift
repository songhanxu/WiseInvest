import SwiftUI
import Combine

/// ViewModel for the Market Detail screen — fetches real data from backend API
class MarketDetailViewModel: ObservableObject {
    let market: Market
    private let stockService = StockDataService.shared

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

    /// Timer for auto-refreshing during trading hours
    private var refreshTimer: Timer?
    /// Timer to periodically check if trading hours changed (every 60s)
    private var tradingCheckTimer: Timer?

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
    }

    deinit {
        // Mark as invalidated so any in-flight callbacks are discarded
        isInvalidated = true
        stopAutoRefresh()
        cancellables.removeAll()
    }

    // MARK: - Lifecycle (called by View)

    /// Called from the View's `.onAppear`. Performs first load + starts auto-refresh.
    func onViewAppear() {
        tradingStatus = MarketTradingHours.tradingStatus(market: market)

        if !hasAppeared {
            hasAppeared = true
            loadData()
        }
        startAutoRefresh()
    }

    // MARK: - Auto Refresh

    /// Start or restart the auto-refresh timer based on trading hours.
    func startAutoRefresh() {
        stopAutoRefresh()

        // Update trading status immediately
        tradingStatus = MarketTradingHours.tradingStatus(market: market)

        // Set up data refresh timer if market is open
        if let interval = MarketTradingHours.refreshInterval(market: market) {
            refreshTimer = Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { [weak self] _ in
                guard let self = self, !self.isInvalidated else { return }
                self.refreshLiveData()
            }
        }

        // Check trading status every 60 seconds to start/stop refresh timer
        tradingCheckTimer = Timer.scheduledTimer(withTimeInterval: 60, repeats: true) { [weak self] _ in
            guard let self = self, !self.isInvalidated else { return }
            let newStatus = MarketTradingHours.tradingStatus(market: self.market)
            let wasActive = self.tradingStatus.isActive
            let isActive = newStatus.isActive

            self.tradingStatus = newStatus

            // Trading session changed — restart or stop the refresh timer
            if wasActive != isActive {
                if isActive {
                    self.startAutoRefresh()
                } else {
                    self.refreshTimer?.invalidate()
                    self.refreshTimer = nil
                }
            }
        }
    }

    func stopAutoRefresh() {
        refreshTimer?.invalidate()
        refreshTimer = nil
        tradingCheckTimer?.invalidate()
        tradingCheckTimer = nil
    }

    /// Lightweight refresh: only update indices and watchlist quotes (no skeleton).
    /// Uses high-water-mark: any response whose seq > current high-water mark is accepted
    /// (it carries newer data), while responses with seq <= high-water mark are discarded.
    /// Multiple requests can be in-flight simultaneously — whichever arrives with a higher
    /// seq wins and advances the water mark.
    private func refreshLiveData() {
        // --- Indices ---
        indicesCounter &+= 1
        let idxSeq = indicesCounter
        stockService.getIndices(for: market) { [weak self] indices in
            guard let self = self, !self.isInvalidated else { return }
            // Accept only if this response is newer than the last accepted one
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
        stockService.addToWatchlist(stock, for: market) { [weak self] success in
            guard let self = self else { return }
            self.pendingMutations = max(0, self.pendingMutations - 1)
            if !success {
                // Revert on failure
                self.watchlist.removeAll { $0.id == stock.id }
                self.watchlistIDs.remove(stock.id)
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
