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

    /// Track which stock IDs are in the user's watchlist (for fast lookup)
    @Published var watchlistIDs: Set<String> = []

    private var cancellables = Set<AnyCancellable>()
    /// Unique token per ViewModel instance — callbacks check this to avoid stale updates
    private let instanceID = UUID()
    private var isInvalidated = false

    /// Counter for in-flight watchlist mutations (add/remove).
    /// loadWatchlist waits for pending mutations to finish before overwriting.
    private var pendingMutations = 0

    init(market: Market) {
        self.market = market
        loadData()

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
        cancellables.removeAll()
    }

    func loadData() {
        loadIndices()
        loadWatchlist()
    }

    private func loadIndices() {
        isLoadingIndices = true
        stockService.getIndices(for: market) { [weak self] indices in
            guard let self = self, !self.isInvalidated else { return }
            self.indices = indices
            self.isLoadingIndices = false
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
        isLoadingWatchlist = true
        stockService.getWatchlist(for: market) { [weak self] stocks in
            guard let self = self, !self.isInvalidated else { return }
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
