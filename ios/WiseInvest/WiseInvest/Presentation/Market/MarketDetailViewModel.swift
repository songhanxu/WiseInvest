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

    func loadData() {
        loadIndices()
        loadWatchlist()
    }

    private func loadIndices() {
        isLoadingIndices = true
        print("[MarketDetailVM] Loading indices for \(market.rawValue), baseURL: \(APIConfig.baseURL)")
        stockService.getIndices(for: market) { [weak self] indices in
            print("[MarketDetailVM] Indices loaded: \(indices.count) items")
            self?.indices = indices
            self?.isLoadingIndices = false
        }
    }

    func loadWatchlist() {
        isLoadingWatchlist = true
        print("[MarketDetailVM] Loading watchlist for \(market.rawValue)")
        stockService.getWatchlist(for: market) { [weak self] stocks in
            print("[MarketDetailVM] Watchlist loaded: \(stocks.count) items")
            self?.watchlist = stocks
            self?.watchlistIDs = Set(stocks.map { $0.id })
            self?.isLoadingWatchlist = false
        }
    }

    func addToWatchlist(_ stock: Stock) {
        // Optimistic update
        if !watchlistIDs.contains(stock.id) {
            watchlist.append(stock)
            watchlistIDs.insert(stock.id)
        }
        stockService.addToWatchlist(stock, for: market) { [weak self] success in
            if !success {
                // Revert on failure
                self?.watchlist.removeAll { $0.id == stock.id }
                self?.watchlistIDs.remove(stock.id)
            }
        }
    }

    func removeFromWatchlist(_ stockId: String) {
        // Optimistic update
        watchlist.removeAll { $0.id == stockId }
        watchlistIDs.remove(stockId)
        stockService.removeFromWatchlist(stockId, for: market) { [weak self] success in
            if !success {
                // Reload on failure
                self?.loadWatchlist()
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
            self?.searchResults = results
            self?.isSearching = false
        }
    }
}
