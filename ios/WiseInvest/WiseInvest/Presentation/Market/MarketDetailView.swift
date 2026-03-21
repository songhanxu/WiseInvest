import SwiftUI

/// Market detail page showing indices, watchlist, and stock search
struct MarketDetailView: View {
    let market: Market
    @ObservedObject var coordinator: AppCoordinator
    @StateObject private var viewModel: MarketDetailViewModel
    @Environment(\.dismiss) private var dismiss
    @State private var showSearch = false
    @State private var selectedStock: Stock?
    @State private var selectedIndex: MarketIndex?

    init(market: Market, coordinator: AppCoordinator) {
        self.market = market
        self.coordinator = coordinator
        _viewModel = StateObject(wrappedValue: MarketDetailViewModel(market: market))
    }

    var body: some View {
        NavigationView {
            ZStack {
                Color.primaryBackground.ignoresSafeArea()

                VStack(spacing: 0) {
                    headerSection
                    
                    ScrollView {
                        VStack(alignment: .leading, spacing: 20) {
                            indicesSection
                            watchlistSection
                        }
                        .padding(.bottom, 32)
                    }
                }
            }
            .navigationBarHidden(true)
        }
        .sheet(isPresented: $showSearch) {
            StockSearchView(market: market, viewModel: viewModel)
        }
        .sheet(item: $selectedStock, onDismiss: {
            // Refresh watchlist when stock detail dismisses (user may have toggled star)
            viewModel.loadWatchlist()
        }) { stock in
            StockDetailView(
                stock: stock,
                market: market,
                coordinator: coordinator,
                watchlistIDs: viewModel.watchlistIDs
            )
        }
        .sheet(item: $selectedIndex, onDismiss: {
            // Refresh watchlist when index detail dismisses (user may have toggled star)
            viewModel.loadWatchlist()
        }) { index in
            // Convert MarketIndex to Stock for the detail view
            let indexStock = Stock(
                id: index.id,
                symbol: index.id.uppercased(),
                name: index.name,
                market: market.rawValue,
                currentPrice: index.value,
                change: index.change,
                changePercent: index.changePercent,
                volume: 0,
                high: index.value,
                low: index.value,
                open: index.value,
                previousClose: index.value - index.change,
                isIndex: true
            )
            StockDetailView(
                stock: indexStock,
                market: market,
                coordinator: coordinator,
                watchlistIDs: viewModel.watchlistIDs
            )
        }
        .onChange(of: showSearch) { isShowing in
            if !isShowing {
                // Refresh watchlist when search sheet dismisses
                viewModel.loadWatchlist()
            }
        }
        .onAppear {
            viewModel.onViewAppear()
        }
        .onDisappear {
            viewModel.stopAutoRefresh()
        }
    }

    // MARK: - Header

    private var tradingStatusColor: Color {
        switch viewModel.tradingStatus {
        case .open:
            return .accentGreen
        case .preMarket, .afterHours:
            return Color(hex: "FF9800")
        case .lunchBreak:
            return Color(hex: "FFD700")
        case .closed:
            return .textTertiary
        }
    }

    private var headerSection: some View {
        HStack(spacing: 12) {
            Button(action: { dismiss() }) {
                Image(systemName: "chevron.left")
                    .font(.system(size: 20, weight: .semibold))
                    .foregroundColor(.textPrimary)
            }

            ZStack {
                LinearGradient(
                    colors: market.gradientColors,
                    startPoint: .topLeading,
                    endPoint: .bottomTrailing
                )
                .frame(width: 38, height: 38)
                .cornerRadius(10)

                Image(systemName: market.icon)
                    .font(.system(size: 18))
                    .foregroundColor(.white)
            }

            VStack(alignment: .leading, spacing: 1) {
                Text(market.displayName)
                    .font(.system(size: 17, weight: .semibold))
                    .foregroundColor(.textPrimary)
                HStack(spacing: 4) {
                    Circle()
                        .fill(tradingStatusColor)
                        .frame(width: 6, height: 6)
                    Text(viewModel.tradingStatus.label)
                        .font(.system(size: 11))
                        .foregroundColor(.textSecondary)
                }
            }

            Spacer()

            Button(action: { showSearch = true }) {
                Image(systemName: "magnifyingglass")
                    .font(.system(size: 20))
                    .foregroundColor(.textPrimary)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(Color.secondaryBackground)
    }

    // MARK: - Indices Section

    private var indicesSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("大盘走势")
                .font(.system(size: 16, weight: .semibold))
                .foregroundColor(.textPrimary)
                .padding(.horizontal, 20)

            if viewModel.isLoadingIndices && viewModel.indices.isEmpty {
                // Skeleton placeholders (3 cards) with shimmer animation
                HStack(spacing: 8) {
                    ForEach(0..<3, id: \.self) { _ in
                        IndexCardSkeleton()
                    }
                }
                .padding(.horizontal, 16)
            } else {
                HStack(spacing: 8) {
                    ForEach(viewModel.indices) { index in
                        IndexCard(index: index, market: market) {
                            selectedIndex = index
                        }
                    }
                }
                .padding(.horizontal, 16)
            }
        }
        .padding(.top, 16)
    }

    // MARK: - Watchlist Section

    private var watchlistSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("自选股")
                    .font(.system(size: 16, weight: .semibold))
                    .foregroundColor(.textPrimary)

                Text("\(viewModel.watchlist.count)")
                    .font(.system(size: 13))
                    .foregroundColor(.textTertiary)

                Spacer()

                Button(action: { showSearch = true }) {
                    HStack(spacing: 4) {
                        Image(systemName: "plus")
                            .font(.system(size: 13, weight: .semibold))
                        Text("添加")
                            .font(.system(size: 13, weight: .medium))
                    }
                    .foregroundColor(.accentBlue)
                }
            }
            .padding(.horizontal, 20)

            if viewModel.isLoadingWatchlist && viewModel.watchlist.isEmpty {
                // Skeleton placeholders (3 rows) with shimmer animation
                VStack(spacing: 2) {
                    ForEach(0..<3, id: \.self) { _ in
                        StockRowSkeleton()
                            .padding(.horizontal, 20)
                    }
                }
            } else if viewModel.watchlist.isEmpty {
                emptyWatchlistView
            } else {
                VStack(spacing: 2) {
                    ForEach(viewModel.watchlist) { stock in
                        StockRow(stock: stock) {
                            selectedStock = stock
                        }
                        .padding(.horizontal, 20)
                    }
                }
            }
        }
    }

    private var emptyWatchlistView: some View {
        VStack(spacing: 12) {
            Image(systemName: "star")
                .font(.system(size: 36))
                .foregroundColor(.textTertiary)
            Text("暂无自选股")
                .font(.system(size: 15))
                .foregroundColor(.textSecondary)
            Button(action: { showSearch = true }) {
                Text("搜索并添加")
                    .font(.system(size: 14, weight: .medium))
                    .foregroundColor(.accentBlue)
                    .padding(.horizontal, 20)
                    .padding(.vertical, 8)
                    .background(Color.accentBlue.opacity(0.15))
                    .cornerRadius(20)
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 40)
    }
}

// MARK: - Index Card

private struct IndexCard: View {
    let index: MarketIndex
    let market: Market
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 2) {
                    Text(index.shortName)
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.textSecondary)
                        .lineLimit(1)
                    Spacer(minLength: 2)
                    RollingNumberText(
                        text: index.changePercentText,
                        font: .system(size: 11, weight: .semibold, design: .monospaced),
                        color: index.isUp ? .accentGreen : Color(hex: "E53935")
                    )
                    .lineLimit(1)
                }

                RollingNumberText(
                    text: index.valueText,
                    font: .system(size: 16, weight: .bold, design: .monospaced),
                    color: .textPrimary,
                    minimumScaleFactor: 0.7
                )
                .lineLimit(1)

                if !index.sparklineData.isEmpty {
                    SparklineView(data: index.sparklineData, isUp: index.isUp)
                        .frame(height: 28)
                } else {
                    Rectangle()
                        .fill(Color.clear)
                        .frame(height: 28)
                }
            }
            .padding(10)
            .frame(maxWidth: .infinity)
            .background(Color.secondaryBackground)
            .cornerRadius(12)
        }
        .buttonStyle(ScaleButtonStyle())
    }
}

// MARK: - Stock Row

struct StockRow: View {
    let stock: Stock
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 14) {
                VStack(alignment: .leading, spacing: 3) {
                    Text(stock.name)
                        .font(.system(size: 15, weight: .medium))
                        .foregroundColor(.textPrimary)
                    Text(stock.symbol)
                        .font(.system(size: 12, design: .monospaced))
                        .foregroundColor(.textTertiary)
                }

                Spacer()

                RollingNumberText(
                    text: stock.priceText,
                    font: .system(size: 16, weight: .semibold, design: .monospaced),
                    color: .textPrimary
                )

                RollingNumberText(
                    text: stock.changePercentText,
                    font: .system(size: 13, weight: .semibold, design: .monospaced),
                    color: .white
                )
                .padding(.horizontal, 10)
                .padding(.vertical, 5)
                .background(
                    RoundedRectangle(cornerRadius: 6)
                        .fill(stock.isUp ? Color.accentGreen : Color(hex: "E53935"))
                )
                .frame(width: 80)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .background(Color.secondaryBackground)
            .cornerRadius(12)
        }
        .buttonStyle(ScaleButtonStyle())
    }
}

// MARK: - Stock Search View

struct StockSearchView: View {
    let market: Market
    @ObservedObject var viewModel: MarketDetailViewModel
    @Environment(\.dismiss) private var dismiss
    @State private var hotStocks: [Stock] = []
    @State private var isLoadingHot = true

    var body: some View {
        ZStack {
            Color.primaryBackground.ignoresSafeArea()

            VStack(spacing: 0) {
                // Search header
                HStack(spacing: 12) {
                    HStack(spacing: 8) {
                        Image(systemName: "magnifyingglass")
                            .font(.system(size: 16))
                            .foregroundColor(.textTertiary)

                        TextField("搜索股票代码或名称", text: $viewModel.searchText)
                            .font(.system(size: 15))
                            .foregroundColor(.textPrimary)
                            .autocapitalization(.allCharacters)
                            .disableAutocorrection(true)
                    }
                    .padding(10)
                    .background(Color.secondaryBackground)
                    .cornerRadius(12)

                    Button("取消") {
                        viewModel.searchText = ""
                        dismiss()
                    }
                    .font(.system(size: 15))
                    .foregroundColor(.accentBlue)
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 12)

                // Results
                if viewModel.searchText.isEmpty {
                    // Hot stocks
                    VStack(alignment: .leading, spacing: 12) {
                        Text("热门\(market.displayName)标的")
                            .font(.system(size: 14, weight: .medium))
                            .foregroundColor(.textSecondary)
                            .padding(.horizontal, 20)

                        if isLoadingHot {
                            ProgressView()
                                .frame(maxWidth: .infinity)
                                .padding(.vertical, 30)
                        } else {
                            ScrollView {
                                VStack(spacing: 2) {
                                    ForEach(hotStocks) { stock in
                                        SearchResultRow(
                                            stock: stock,
                                            isInWatchlist: viewModel.isInWatchlist(stock.id),
                                            onAdd: { viewModel.addToWatchlist(stock) },
                                            onRemove: { viewModel.removeFromWatchlist(stock.id) }
                                        )
                                        .padding(.horizontal, 16)
                                    }
                                }
                            }
                        }
                    }
                    .padding(.top, 8)
                } else if viewModel.isSearching {
                    ProgressView()
                        .frame(maxWidth: .infinity)
                        .padding(.top, 60)
                } else if viewModel.searchResults.isEmpty {
                    VStack(spacing: 12) {
                        Image(systemName: "magnifyingglass")
                            .font(.system(size: 36))
                            .foregroundColor(.textTertiary)
                        Text("未找到匹配结果")
                            .font(.system(size: 15))
                            .foregroundColor(.textSecondary)
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.top, 60)
                } else {
                    ScrollView {
                        VStack(spacing: 2) {
                            ForEach(viewModel.searchResults) { stock in
                                SearchResultRow(
                                    stock: stock,
                                    isInWatchlist: viewModel.isInWatchlist(stock.id),
                                    onAdd: { viewModel.addToWatchlist(stock) },
                                    onRemove: { viewModel.removeFromWatchlist(stock.id) }
                                )
                                .padding(.horizontal, 16)
                            }
                        }
                        .padding(.top, 4)
                    }
                }

                Spacer()
            }
        }
        .onAppear {
            loadHotStocks()
        }
    }

    private func loadHotStocks() {
        isLoadingHot = true
        StockDataService.shared.searchStocks(query: "", market: market) { stocks in
            self.hotStocks = stocks
            self.isLoadingHot = false
        }
    }
}

// MARK: - Search Result Row

private struct SearchResultRow: View {
    let stock: Stock
    let isInWatchlist: Bool
    let onAdd: () -> Void
    let onRemove: () -> Void

    var body: some View {
        HStack(spacing: 14) {
            VStack(alignment: .leading, spacing: 3) {
                Text(stock.name)
                    .font(.system(size: 15, weight: .medium))
                    .foregroundColor(.textPrimary)
                Text(stock.symbol)
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundColor(.textTertiary)
            }

            Spacer()

            VStack(alignment: .trailing, spacing: 3) {
                Text(stock.priceText)
                    .font(.system(size: 14, weight: .medium, design: .monospaced))
                    .foregroundColor(.textPrimary)
                Text(stock.changePercentText)
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundColor(stock.isUp ? .accentGreen : Color(hex: "E53935"))
            }

            Button(action: {
                if isInWatchlist {
                    onRemove()
                } else {
                    onAdd()
                }
            }) {
                Image(systemName: isInWatchlist ? "star.fill" : "star")
                    .font(.system(size: 18))
                    .foregroundColor(isInWatchlist ? Color(hex: "FFD700") : .textTertiary)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(Color.secondaryBackground)
        .cornerRadius(12)
    }
}
