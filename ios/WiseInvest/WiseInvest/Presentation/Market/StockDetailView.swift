import SwiftUI

/// Stock detail page: K-line chart + AI analysis + news + "Ask WiseInvest" button
struct StockDetailView: View {
    let stock: Stock
    let market: Market
    @ObservedObject var coordinator: AppCoordinator
    let watchlistIDs: Set<String>
    @Environment(\.dismiss) private var dismiss
    @State private var klineData: [KLinePoint] = []
    @State private var newsItems: [NewsItem] = []
    @State private var analysisItems: [AIAnalysisItem] = []
    @State private var showConversation = false
    @State private var isInWatchlist = false
    @State private var isLoadingKline = true
    @State private var isLoadingMore = false
    @State private var isLoadingNews = true
    @State private var liveStock: Stock?
    @State private var tradingStatus: MarketTradingHours.TradingStatus = .closed("已收盘")
    @State private var refreshTimer: Timer?
    @State private var selectedPeriod: StockDataService.KLinePeriod = .d1
    /// Monotonically increasing counter — each new quote request gets the next value.
    @State private var quoteCounter: UInt64 = 0
    /// High-water mark: the highest seq whose response has been accepted.
    @State private var quoteHighWaterMark: UInt64 = 0
    /// Alert state for timeout/error messages
    @State private var showTimeoutAlert = false
    @State private var timeoutAlertMessage = ""
    /// Timer for detecting K-line data load timeout
    @State private var klineTimeoutTimer: Timer?
    /// Timer for detecting news data load timeout
    @State private var newsTimeoutTimer: Timer?
    /// Selected news item for detail view
    @State private var selectedNewsItem: NewsItem?
    /// Client-side K-line retry state (with exponential backoff)
    @State private var klineRetryCount: Int = 0
    @State private var klineRetryTimer: Timer?
    private let klineMaxClientRetries = 3

    private let stockService = StockDataService.shared
    private let preloadManager = PreloadManager.shared

    /// Use liveStock if available (refreshed from API), fallback to initial stock
    private var displayStock: Stock { liveStock ?? stock }

    var body: some View {
        ZStack {
            Color.primaryBackground.ignoresSafeArea()

            GeometryReader { geometry in
                VStack(spacing: 0) {
                    headerSection

                    ScrollView(.vertical, showsIndicators: false) {
                        VStack(alignment: .leading, spacing: 20) {
                            // Stock price overview
                            priceSection

                            // K-Line chart with period selector
                            VStack(spacing: 0) {
                                // Period selector
                                periodSelector
                                    .padding(.horizontal, 16)
                                    .padding(.bottom, 8)

                                if isLoadingKline {
                                    KLineChartSkeleton()
                                        .padding(.horizontal, 16)
                                } else if !klineData.isEmpty {
                                    KLineChartView(
                                        data: klineData,
                                        accentColor: market.accentColor,
                                        onLoadMore: { loadMoreKLineData() },
                                        period: selectedPeriod.rawValue
                                    )
                                    .padding(.horizontal, 16)
                                } else {
                                    // Empty state — data failed to load, user can tap to retry
                                    VStack(spacing: 12) {
                                        Image(systemName: "chart.xyaxis.line")
                                            .font(.system(size: 32))
                                            .foregroundColor(.textTertiary)
                                        Text("暂无K线数据")
                                            .font(.system(size: 14))
                                            .foregroundColor(.textSecondary)
                                        Button(action: {
                                            klineRetryCount = 0
                                            loadKLineData()
                                        }) {
                                            HStack(spacing: 4) {
                                                Image(systemName: "arrow.clockwise")
                                                    .font(.system(size: 12))
                                                Text("点击重试")
                                                    .font(.system(size: 13, weight: .medium))
                                            }
                                            .foregroundColor(market.accentColor)
                                        }
                                    }
                                    .frame(height: 200)
                                    .frame(maxWidth: .infinity)
                                    .padding(.horizontal, 16)
                                }
                            }

                            // AI Analysis
                            aiAnalysisSection

                            // News
                            newsSection

                            // Bottom spacer for the floating button
                            Spacer().frame(height: 80)
                        }
                        .frame(width: geometry.size.width)
                    }

                    Spacer(minLength: 0)
                }
            }

            // Floating "问问慧投" button
            VStack {
                Spacer()
                askButton
            }
        }
        .onAppear {
            loadData()
            startAutoRefresh()
        }
        .onDisappear {
            stopAutoRefresh()
            klineTimeoutTimer?.invalidate()
            klineTimeoutTimer = nil
            klineRetryTimer?.invalidate()
            klineRetryTimer = nil
            newsTimeoutTimer?.invalidate()
            newsTimeoutTimer = nil
        }
        .onReceive(NotificationCenter.default.publisher(for: .newsEnhancementDidComplete)) { _ in
            // Background AI enhancement finished — refresh newsItems from cache
            // so that when user taps a NewsCard, the passed NewsItem already has analysis.
            if let cached = preloadManager.getCachedNews(stockId: stock.id, market: market), !cached.isEmpty {
                self.newsItems = cached
            }
        }
        .alert("数据加载失败", isPresented: $showTimeoutAlert) {
            Button("重试") {
                if isLoadingKline {
                    // Reset retry count so user-initiated retry gets fresh attempts
                    klineRetryCount = 0
                    performKLineLoad()
                }
                if isLoadingNews {
                    loadNewsData()
                }
            }
            Button("取消", role: .cancel) {
                // Dismiss skeleton — show empty state instead of infinite loading
                if isLoadingKline {
                    isLoadingKline = false
                }
                if isLoadingNews {
                    isLoadingNews = false
                }
            }
        } message: {
            Text(timeoutAlertMessage)
        }
        .sheet(isPresented: $showConversation) {
            StockConversationView(
                stock: displayStock,
                market: market,
                klineData: klineData,
                coordinator: coordinator
            )
        }
        .sheet(item: $selectedNewsItem) { news in
            NewsDetailView(news: news, stockCode: stock.id, stockName: stock.name, stockMarket: market.rawValue, accentColor: market.accentColor)
        }
    }

    // MARK: - Header

    private var headerSection: some View {
        HStack(spacing: 12) {
            Button(action: { dismiss() }) {
                Image(systemName: "chevron.left")
                    .font(.system(size: 20, weight: .semibold))
                    .foregroundColor(.textPrimary)
            }

            VStack(alignment: .leading, spacing: 1) {
                Text(displayStock.name)
                    .font(.system(size: 17, weight: .semibold))
                    .foregroundColor(.textPrimary)
                HStack(spacing: 4) {
                    Text(displayStock.symbol)
                        .font(.system(size: 12, design: .monospaced))
                        .foregroundColor(.textSecondary)
                    Circle()
                        .fill(tradingStatusColor)
                        .frame(width: 5, height: 5)
                    Text(tradingStatus.label)
                        .font(.system(size: 10))
                        .foregroundColor(.textTertiary)
                }
            }

            Spacer()

            // Only show watchlist star for regular stocks, not indices
            if !stock.isIndex {
                Button(action: {
                    if isInWatchlist {
                        stockService.removeFromWatchlist(stock.id, for: market)
                    } else {
                        stockService.addToWatchlist(stock, for: market)
                    }
                    isInWatchlist.toggle()
                }) {
                    Image(systemName: isInWatchlist ? "star.fill" : "star")
                        .font(.system(size: 20))
                        .foregroundColor(isInWatchlist ? Color(hex: "FFD700") : .textSecondary)
                }
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(Color.secondaryBackground)
    }

    // MARK: - Price Section

    private var priceSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .lastTextBaseline, spacing: 8) {
                RollingNumberText(
                    text: displayStock.priceText,
                    font: .system(size: 36, weight: .bold, design: .monospaced),
                    color: displayStock.isUp ? .accentGreen : Color(hex: "E53935"),
                    minimumScaleFactor: 0.6
                )

                VStack(alignment: .leading, spacing: 2) {
                    RollingNumberText(
                        text: displayStock.changeText,
                        font: .system(size: 14, weight: .medium, design: .monospaced),
                        color: displayStock.isUp ? .accentGreen : Color(hex: "E53935")
                    )
                    RollingNumberText(
                        text: displayStock.changePercentText,
                        font: .system(size: 14, weight: .semibold, design: .monospaced),
                        color: displayStock.isUp ? .accentGreen : Color(hex: "E53935")
                    )
                }
            }

            HStack(spacing: 24) {
                rollingPriceTag("高", value: String(format: "%.2f", displayStock.high), color: .accentGreen)
                rollingPriceTag("低", value: String(format: "%.2f", displayStock.low), color: Color(hex: "E53935"))
                rollingPriceTag("量", value: String(format: "%.1f亿", displayStock.volume), color: .textSecondary)
            }
        }
        .padding(.horizontal, 20)
        .padding(.top, 12)
    }

    private func rollingPriceTag(_ label: String, value: String, color: Color) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label)
                .font(.system(size: 11))
                .foregroundColor(.textTertiary)
            RollingNumberText(
                text: value,
                font: .system(size: 13, weight: .medium, design: .monospaced),
                color: color
            )
        }
    }

    // MARK: - AI Analysis Section

    private var aiAnalysisSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Image(systemName: "brain.head.profile")
                    .font(.system(size: 14))
                    .foregroundColor(.accentBlue)
                Text("AI 智能分析")
                    .font(.system(size: 16, weight: .semibold))
                    .foregroundColor(.textPrimary)
            }
            .padding(.horizontal, 20)

            if isLoadingKline && analysisItems.isEmpty {
                // Skeleton placeholders for AI analysis cards
                VStack(spacing: 10) {
                    ForEach(0..<3, id: \.self) { _ in
                        AnalysisCardSkeleton()
                    }
                }
                .padding(.horizontal, 16)
            } else if !analysisItems.isEmpty {
                VStack(spacing: 10) {
                    ForEach(analysisItems) { item in
                        AnalysisCard(item: item)
                    }
                }
                .padding(.horizontal, 16)
            }
        }
    }

    // MARK: - News Section

    private var newsSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Image(systemName: "newspaper")
                    .font(.system(size: 14))
                    .foregroundColor(.accentBlue)
                Text("相关资讯")
                    .font(.system(size: 16, weight: .semibold))
                    .foregroundColor(.textPrimary)
                Spacer()
                if !newsItems.isEmpty {
                    Text("AI 摘要")
                        .font(.system(size: 11, weight: .medium))
                        .foregroundColor(.accentBlue.opacity(0.8))
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(Color.accentBlue.opacity(0.12))
                        .cornerRadius(4)
                }
            }
            .padding(.horizontal, 20)

            if isLoadingNews {
                VStack(spacing: 8) {
                    ForEach(0..<3, id: \.self) { _ in
                        NewsCardSkeleton()
                    }
                }
                .padding(.horizontal, 16)
            } else if newsItems.isEmpty {
                VStack(spacing: 8) {
                    Image(systemName: "newspaper")
                        .font(.system(size: 28))
                        .foregroundColor(.textTertiary.opacity(0.5))
                    Text("暂无相关资讯")
                        .font(.system(size: 13))
                        .foregroundColor(.textTertiary)
                    Button(action: { loadNewsData() }) {
                        HStack(spacing: 4) {
                            Image(systemName: "arrow.clockwise")
                                .font(.system(size: 11))
                            Text("重新加载")
                                .font(.system(size: 12, weight: .medium))
                        }
                        .foregroundColor(.accentBlue)
                    }
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 20)
            } else {
                VStack(spacing: 8) {
                    ForEach(newsItems) { news in
                        NewsCard(news: news, accentColor: market.accentColor)
                            .onTapGesture {
                                selectedNewsItem = news
                            }
                    }
                }
                .padding(.horizontal, 16)
            }
        }
    }

    // MARK: - Ask Button

    private var askButton: some View {
        Button(action: { showConversation = true }) {
            HStack(spacing: 8) {
                Image(systemName: "brain.head.profile")
                    .font(.system(size: 18))
                Text("问问慧投")
                    .font(.system(size: 16, weight: .semibold))
            }
            .foregroundColor(.white)
            .padding(.horizontal, 28)
            .padding(.vertical, 14)
            .background(
                LinearGradient(
                    colors: market.gradientColors,
                    startPoint: .leading,
                    endPoint: .trailing
                )
            )
            .cornerRadius(25)
            .shadow(color: market.accentColor.opacity(0.4), radius: 12, x: 0, y: 6)
        }
        .padding(.bottom, 20)
    }

    // MARK: - Data Loading

    private func loadData() {
        // Try cached quote first for instant display
        if let cachedQuote = preloadManager.getCachedQuote(stockId: stock.id, market: market) {
            self.liveStock = cachedQuote
        }

        // Refresh real-time stock quote (with high-water-mark protection)
        quoteCounter &+= 1
        let capturedSeq = quoteCounter
        stockService.getStockQuote(code: stock.id, market: market) { updated in
            guard capturedSeq > self.quoteHighWaterMark else { return }
            self.quoteHighWaterMark = capturedSeq
            if let updated = updated {
                self.liveStock = updated
                self.preloadManager.updateQuoteCache(stock: updated, market: self.market)
            }
        }

        // Load K-line with selected period
        loadKLineData()

        // Load news (try cache first)
        loadNewsData()

        // Check watchlist status from the parent view's watchlist data
        isInWatchlist = watchlistIDs.contains(stock.id)
    }

    /// Load news data with smart preload-aware strategy:
    /// 1. If cached news exists → instant display, no skeleton
    /// 2. If a batch preload is in-flight → wait for it (no extra request)
    /// 3. Otherwise → fetch from network as fallback
    private func loadNewsData() {
        // Use getOrWaitForNews which handles cache + in-flight preload detection
        let isWaiting = preloadManager.getOrWaitForNews(stockId: stock.id, market: market) { [self] result in
            if let news = result, !news.isEmpty {
                self.newsItems = news
                self.isLoadingNews = false
                self.newsTimeoutTimer?.invalidate()
                self.newsTimeoutTimer = nil
                return
            }
            // Preload finished but returned empty / no preload was in-flight — fetch directly
            self.fetchNewsFromNetwork()
        }

        if isWaiting {
            // A preload is in-flight — show skeleton and wait
            isLoadingNews = true
            // Safety timeout in case preload takes too long
            newsTimeoutTimer?.invalidate()
            newsTimeoutTimer = Timer.scheduledTimer(withTimeInterval: 20.0, repeats: false) { _ in
                if self.isLoadingNews && self.newsItems.isEmpty {
                    // Preload is taking too long — fall back to direct fetch
                    self.fetchNewsFromNetwork()
                }
            }
        }
        // If not waiting, getOrWaitForNews already called the callback synchronously
        // (either with cache or nil). If cache hit, isLoadingNews was set to false.
        // If nil, fetchNewsFromNetwork was called.
    }

    /// Fetch news directly from network (fallback when no cache and no in-flight preload)
    private func fetchNewsFromNetwork() {
        isLoadingNews = true
        newsTimeoutTimer?.invalidate()
        newsTimeoutTimer = Timer.scheduledTimer(withTimeInterval: 16.0, repeats: false) { _ in
            if self.isLoadingNews {
                self.isLoadingNews = false
            }
        }

        stockService.getNews(for: stock) { news in
            self.newsTimeoutTimer?.invalidate()
            self.newsTimeoutTimer = nil
            self.newsItems = news
            self.isLoadingNews = false
            if !news.isEmpty {
                self.preloadManager.updateNewsCache(stockId: self.stock.id, market: self.market, data: news)
            }
        }
    }

    /// Load K-line data for the current period. Called on initial load and period switch.
    /// After initial data arrives, immediately kicks off a background preload for more history.
    /// The skeleton screen is only dismissed when valid data arrives; on empty/error, it stays.
    ///
    /// **Retry strategy**: If the backend returns an error (e.g. "获取数据超时") or empty data,
    /// the client retries up to `klineMaxClientRetries` times with exponential backoff
    /// (2s, 4s, 8s). If all retries fail, a user-facing alert is shown with the backend's
    /// error message (or a generic timeout message).
    private func loadKLineData() {
        isLoadingKline = true
        isLoadingMore = false
        // Reset retry state on fresh load (e.g. period switch)
        klineRetryCount = 0
        klineRetryTimer?.invalidate()
        klineRetryTimer = nil

        performKLineLoad()
    }

    /// Internal method that performs a single K-line fetch attempt and handles retries.
    private func performKLineLoad() {
        // Cancel any pending timeout timer from a previous attempt
        klineTimeoutTimer?.invalidate()

        // Start timeout timer — if data hasn't arrived after 10 seconds, trigger retry or alert
        klineTimeoutTimer = Timer.scheduledTimer(withTimeInterval: 10.0, repeats: false) { _ in
            if self.isLoadingKline {
                self.handleKLineLoadFailure(errorMessage: "网络请求超时，请检查网络连接")
            }
        }

        stockService.getKLineData(for: stock, period: selectedPeriod) { result in
            // Cancel the timeout timer since we got a response
            self.klineTimeoutTimer?.invalidate()
            self.klineTimeoutTimer = nil

            if let errorMsg = result.errorMessage {
                // Backend returned a specific error — try to retry or show alert
                self.handleKLineLoadFailure(errorMessage: errorMsg)
                return
            }

            if result.points.isEmpty {
                // Empty data with no error — still retry
                self.handleKLineLoadFailure(errorMessage: "未能获取K线数据")
                return
            }

            // Success — reset retry state and display data
            self.klineRetryCount = 0
            self.klineRetryTimer?.invalidate()
            self.klineRetryTimer = nil
            self.klineData = result.points
            self.isLoadingKline = false

            // Generate AI analysis only for daily data
            if self.selectedPeriod == .d1 {
                self.analysisItems = Self.generateAnalysis(stock: self.displayStock, klineData: result.points)
            }
            // Background preload: immediately request more history data
            if result.points.count > 0 {
                self.backgroundPreloadKLineData()
            }
        }
    }

    /// Handle a K-line load failure: either schedule a retry or show the error to the user.
    private func handleKLineLoadFailure(errorMessage: String) {
        if klineRetryCount < klineMaxClientRetries {
            // Schedule a retry with exponential backoff: 2s, 4s, 8s
            klineRetryCount += 1
            let delay = pow(2.0, Double(klineRetryCount)) // 2, 4, 8
            klineRetryTimer?.invalidate()
            klineRetryTimer = Timer.scheduledTimer(withTimeInterval: delay, repeats: false) { _ in
                // Only retry if we're still loading and view hasn't disappeared
                if self.isLoadingKline {
                    self.performKLineLoad()
                }
            }
        } else {
            // All retries exhausted — show error alert to user
            // Don't set isLoadingKline = false so skeleton stays visible (user can retry via alert)
            self.timeoutAlertMessage = errorMessage
            self.showTimeoutAlert = true
        }
    }

    /// Background preload more historical K-line data. Called once after initial load.
    /// Does NOT show loading indicator — runs silently in the background.
    private func backgroundPreloadKLineData() {
        let currentCount = klineData.count
        // Request a large batch (current + 500 more) to build up a comfortable scroll buffer
        stockService.loadMoreKLineData(for: stock, period: selectedPeriod, currentCount: currentCount) { result in
            if result.points.count > self.klineData.count {
                self.klineData = result.points
            }
        }
    }

    /// Load more historical K-line data (pagination). Merges with existing data.
    /// Runs silently without loading indicator for seamless infinite scroll.
    private func loadMoreKLineData() {
        guard !isLoadingMore else { return }
        isLoadingMore = true
        stockService.loadMoreKLineData(for: stock, period: selectedPeriod, currentCount: klineData.count) { result in
            if result.points.count > self.klineData.count {
                self.klineData = result.points
            }
            // Reset flag after a short delay to allow next request but prevent hammering
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) {
                self.isLoadingMore = false
            }
        }
    }

    // MARK: - Period Selector

    private var periodSelector: some View {
        let availablePeriods = StockDataService.KLinePeriod.availablePeriods(for: stock.market)
        
        return HStack(spacing: 0) {
            ForEach(availablePeriods, id: \.rawValue) { period in
                Button(action: {
                    guard selectedPeriod != period else { return }
                    withAnimation(.easeInOut(duration: 0.2)) {
                        selectedPeriod = period
                    }
                    loadKLineData()
                }) {
                    Text(period.label)
                        .font(.system(size: 12, weight: selectedPeriod == period ? .semibold : .regular))
                        .foregroundColor(selectedPeriod == period ? .white : .textSecondary)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 7)
                        .background(
                            selectedPeriod == period
                                ? AnyView(
                                    RoundedRectangle(cornerRadius: 8)
                                        .fill(market.accentColor)
                                  )
                                : AnyView(Color.clear)
                        )
                }
                .buttonStyle(PlainButtonStyle())
            }
        }
        .padding(3)
        .background(Color.primaryBackground)
        .cornerRadius(10)
    }

    // MARK: - Auto Refresh

    private func startAutoRefresh() {
        tradingStatus = MarketTradingHours.tradingStatus(market: market)

        guard let interval = MarketTradingHours.refreshInterval(market: market) else {
            return
        }

        refreshTimer = Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { _ in
            refreshQuote()
        }
    }

    private func stopAutoRefresh() {
        refreshTimer?.invalidate()
        refreshTimer = nil
    }

    /// Lightweight refresh: update real-time stock quote AND sync the last K-line candle.
    /// Uses high-water-mark to discard stale responses — any response with seq > current
    /// high-water mark is accepted, advancing the mark. Multiple requests can be in-flight;
    /// whichever arrives with a higher seq wins.
    private func refreshQuote() {
        tradingStatus = MarketTradingHours.tradingStatus(market: market)

        // If market just closed, stop the timer
        if !tradingStatus.isActive && market != .crypto {
            stopAutoRefresh()
            return
        }

        quoteCounter &+= 1
        let capturedSeq = quoteCounter

        stockService.getStockQuote(code: stock.id, market: market) { updated in
            // Discard stale response — a newer response has already been accepted
            guard capturedSeq > self.quoteHighWaterMark else { return }
            self.quoteHighWaterMark = capturedSeq
            if let updated = updated {
                self.liveStock = updated
                // Sync K-line last candle with real-time price data
                self.syncKLineWithRealtimePrice(updated)
            }
        }
    }

    /// Update the last K-line candle with real-time price data so the chart
    /// stays in sync with the live quote.
    ///
    /// **CRITICAL**: Only the `close` price is updated from the real-time quote.
    /// The `high` and `low` values are NEVER overwritten from `stock.high`/`stock.low`,
    /// because those represent the **full-day** high/low from the quote API, which is
    /// a different data source than the K-line API. Using them would cause the latest
    /// candle to "jump" — the backend K-line data returns one set of high/low values,
    /// and the quote API returns a different set, leading to visual inconsistency.
    ///
    /// Instead, high/low are only expanded **naturally**: if the new close price
    /// exceeds the candle's existing high or falls below its existing low, only then
    /// are they adjusted. This ensures the K-line chart is always consistent with
    /// the backend-returned K-line data, with no forced corrections.
    ///
    /// Volume is also kept as-is from the K-line data — the quote API's volume
    /// represents the full-day cumulative volume, not per-candle volume.
    private func syncKLineWithRealtimePrice(_ stock: Stock) {
        guard !klineData.isEmpty, stock.currentPrice > 0 else { return }

        let lastIdx = klineData.count - 1
        let lastPoint = klineData[lastIdx]

        let newClose = stock.currentPrice

        // High/low: only expand if the current price naturally exceeds the candle's range.
        // NEVER use stock.high/stock.low — they are full-day values from the quote API,
        // not from the K-line data source. Applying them would cause candle jumps.
        let newHigh = max(lastPoint.high, newClose)
        let newLow = min(lastPoint.low, newClose)

        // Volume: keep the K-line data's volume — the quote API volume is full-day
        // cumulative, not per-candle. Using it would misrepresent intraday candles.
        let newVolume = lastPoint.volume

        let updatedPoint = KLinePoint(
            date: lastPoint.date,
            open: lastPoint.open,
            close: newClose,
            high: newHigh,
            low: newLow,
            volume: newVolume
        )

        klineData[lastIdx] = updatedPoint
    }

    private var tradingStatusColor: Color {
        switch tradingStatus {
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

    /// Generate AI analysis items from real stock and K-line data
    static func generateAnalysis(stock: Stock, klineData: [KLinePoint]) -> [AIAnalysisItem] {
        var items: [AIAnalysisItem] = []

        // Technical analysis from K-line
        if klineData.count >= 5 {
            let recent5 = klineData.suffix(5)
            let recent20 = klineData.suffix(min(20, klineData.count))
            let ma5 = recent5.reduce(0.0) { $0 + $1.close } / Double(recent5.count)
            let ma20 = recent20.reduce(0.0) { $0 + $1.close } / Double(recent20.count)
            let trend = stock.currentPrice > ma5 ? "短期均线上方" : "短期均线下方"
            let maTrend = ma5 > ma20 ? "多头排列" : "空头排列"

            let support = klineData.suffix(10).map { $0.low }.min() ?? stock.low
            let resistance = klineData.suffix(10).map { $0.high }.max() ?? stock.high

            items.append(AIAnalysisItem(
                id: "tech",
                title: "技术面分析",
                icon: "chart.xyaxis.line",
                content: "\(stock.name)当前价\(stock.priceText)，处于\(trend)，均线\(maTrend)。近期支撑位\(String(format: "%.2f", support))，阻力位\(String(format: "%.2f", resistance))。"
            ))
        }

        // Price trend
        if let first = klineData.first, let last = klineData.last, klineData.count > 1 {
            let periodChange = ((last.close - first.open) / first.open) * 100
            let direction = periodChange >= 0 ? "上涨" : "下跌"
            items.append(AIAnalysisItem(
                id: "trend",
                title: "走势概览",
                icon: "arrow.up.right.circle",
                content: "近\(klineData.count)个交易日累计\(direction)\(String(format: "%.2f", abs(periodChange)))%。当日涨跌\(stock.changeText)（\(stock.changePercentText)），成交量\(String(format: "%.1f", stock.volume))亿。"
            ))
        }

        // Volume analysis
        if klineData.count >= 5 {
            let avgVol = klineData.suffix(5).reduce(0.0) { $0 + $1.volume } / 5.0
            let currentVol = klineData.last?.volume ?? 0
            let volRatio = avgVol > 0 ? currentVol / avgVol : 1.0
            let volDesc = volRatio > 1.5 ? "明显放量" : (volRatio < 0.7 ? "缩量" : "正常水平")
            items.append(AIAnalysisItem(
                id: "volume",
                title: "成交量分析",
                icon: "chart.bar",
                content: "当前成交量处于近5日\(volDesc)状态（量比\(String(format: "%.2f", volRatio))）。\(volRatio > 1.5 ? "放量可能预示趋势加速或反转。" : volRatio < 0.7 ? "缩量表明市场观望情绪浓厚。" : "成交量平稳，无明显异动。")"
            ))
        }

        return items
    }
}

// MARK: - Analysis Card

private struct AnalysisCard: View {
    let item: AIAnalysisItem

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: item.icon)
                    .font(.system(size: 14))
                    .foregroundColor(.accentBlue)
                Text(item.title)
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.textPrimary)
            }

            Text(item.content)
                .font(.system(size: 13))
                .foregroundColor(.textSecondary)
                .lineSpacing(4)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.secondaryBackground)
        .cornerRadius(14)
    }
}

// MARK: - News Card (AI Summary Style)

private struct NewsCard: View {
    let news: NewsItem
    var accentColor: Color = .accentBlue

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            // Top row: sentiment badge + source + time
            HStack(spacing: 6) {
                sentimentBadge
                Text(news.source)
                    .font(.system(size: 11, weight: .medium))
                    .foregroundColor(.textSecondary)
                Spacer()
                Text(news.time)
                    .font(.system(size: 11))
                    .foregroundColor(.textTertiary)
            }

            // Title
            Text(news.title)
                .font(.system(size: 14, weight: .semibold))
                .foregroundColor(.textPrimary)
                .lineLimit(2)

            // AI Summary
            if !news.summary.isEmpty {
                HStack(alignment: .top, spacing: 6) {
                    Image(systemName: "sparkles")
                        .font(.system(size: 10))
                        .foregroundColor(accentColor.opacity(0.7))
                        .padding(.top, 2)
                    Text(news.summary)
                        .font(.system(size: 12))
                        .foregroundColor(.textSecondary)
                        .lineSpacing(3)
                        .lineLimit(3)
                }
                .padding(10)
                .background(accentColor.opacity(0.06))
                .cornerRadius(8)
            }

            // Bottom: tap hint
            HStack {
                Spacer()
                HStack(spacing: 3) {
                    Text("查看详情")
                        .font(.system(size: 11))
                    Image(systemName: "chevron.right")
                        .font(.system(size: 9, weight: .semibold))
                }
                .foregroundColor(.textTertiary)
            }
        }
        .padding(14)
        .background(Color.secondaryBackground)
        .cornerRadius(14)
        .contentShape(Rectangle())
    }

    private var sentimentBadge: some View {
        Text(news.sentiment.label)
            .font(.system(size: 10, weight: .semibold))
            .foregroundColor(sentimentColor)
            .padding(.horizontal, 8)
            .padding(.vertical, 3)
            .background(sentimentColor.opacity(0.15))
            .cornerRadius(4)
    }

    private var sentimentColor: Color {
        switch news.sentiment {
        case .positive: return .accentGreen
        case .negative: return Color(hex: "E53935")
        case .neutral:  return .textSecondary
        }
    }
}

// MARK: - News Detail View (Full Screen)

struct NewsDetailView: View {
    let news: NewsItem
    let stockCode: String
    let stockName: String
    let stockMarket: String
    var accentColor: Color = .accentBlue
    @Environment(\.dismiss) private var dismiss
    @State private var showSafari = false
    @State private var isLoadingAI = false
    @State private var enhancedSummary: String
    @State private var enhancedAnalysis: String = ""
    @State private var enhancedSentiment: NewsSentiment

    init(news: NewsItem, stockCode: String, stockName: String, stockMarket: String, accentColor: Color = .accentBlue) {
        self.news = news
        self.stockCode = stockCode
        self.stockName = stockName
        self.stockMarket = stockMarket
        self.accentColor = accentColor
        // Initialize sentiment and summary directly from news data so
        // they are correct on the very first frame (no flash from neutral → actual)
        _enhancedSentiment = State(initialValue: news.sentiment)
        _enhancedSummary = State(initialValue: news.summary)
    }

    var body: some View {
        ZStack {
            Color.primaryBackground.ignoresSafeArea()

            VStack(spacing: 0) {
                // Header — consistent with StockDetailView / MarketDetailView
                HStack(spacing: 12) {
                    Button(action: { dismiss() }) {
                        Image(systemName: "chevron.left")
                            .font(.system(size: 20, weight: .semibold))
                            .foregroundColor(.textPrimary)
                    }

                    Text("资讯详情")
                        .font(.system(size: 17, weight: .semibold))
                        .foregroundColor(.textPrimary)

                    Spacer()

                    sentimentBadge
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 12)
                .background(Color.secondaryBackground)

                ScrollView {
                    VStack(alignment: .leading, spacing: 20) {
                        // Title
                        Text(news.title)
                            .font(.system(size: 20, weight: .bold))
                            .foregroundColor(.textPrimary)
                            .lineSpacing(4)

                        // Meta info
                        HStack(spacing: 12) {
                            HStack(spacing: 4) {
                                Image(systemName: "building.2")
                                    .font(.system(size: 11))
                                Text(news.source)
                                    .font(.system(size: 13, weight: .medium))
                            }
                            .foregroundColor(.textSecondary)

                            HStack(spacing: 4) {
                                Image(systemName: "clock")
                                    .font(.system(size: 11))
                                Text(news.time)
                                    .font(.system(size: 13))
                            }
                            .foregroundColor(.textTertiary)

                            Spacer()
                        }

                        // Divider
                        Rectangle()
                            .fill(Color.white.opacity(0.08))
                            .frame(height: 1)

                        // AI Summary section
                        VStack(alignment: .leading, spacing: 12) {
                            HStack(spacing: 6) {
                                Image(systemName: "sparkles")
                                    .font(.system(size: 14))
                                    .foregroundColor(accentColor)
                                Text("AI 摘要分析")
                                    .font(.system(size: 16, weight: .semibold))
                                    .foregroundColor(.textPrimary)
                            }

                            if isLoadingAI {
                                // Loading skeleton — matches the actual two-section layout
                                VStack(alignment: .leading, spacing: 12) {
                                    // Skeleton for AI Summary text (3 lines)
                                    ForEach(0..<3, id: \.self) { _ in
                                        RoundedRectangle(cornerRadius: 4)
                                            .fill(Color.white.opacity(0.06))
                                            .frame(height: 14)
                                            .shimmer()
                                    }
                                    RoundedRectangle(cornerRadius: 4)
                                        .fill(Color.white.opacity(0.06))
                                        .frame(width: 180, height: 14)
                                        .shimmer()

                                    // Divider skeleton
                                    Rectangle()
                                        .fill(accentColor.opacity(0.08))
                                        .frame(height: 1)
                                        .padding(.vertical, 4)

                                    // Skeleton for 深度分析 header
                                    HStack(spacing: 6) {
                                        RoundedRectangle(cornerRadius: 3)
                                            .fill(Color.white.opacity(0.06))
                                            .frame(width: 14, height: 14)
                                            .shimmer()
                                        RoundedRectangle(cornerRadius: 3)
                                            .fill(Color.white.opacity(0.06))
                                            .frame(width: 64, height: 14)
                                            .shimmer()
                                    }

                                    // Skeleton for analysis text (5 lines)
                                    ForEach(0..<5, id: \.self) { _ in
                                        RoundedRectangle(cornerRadius: 4)
                                            .fill(Color.white.opacity(0.06))
                                            .frame(height: 13)
                                            .shimmer()
                                    }
                                    RoundedRectangle(cornerRadius: 4)
                                        .fill(Color.white.opacity(0.06))
                                        .frame(width: 140, height: 13)
                                        .shimmer()
                                }
                            } else {
                                // Short summary
                                Text(enhancedSummary.isEmpty ? news.summary : enhancedSummary)
                                    .font(.system(size: 15, weight: .medium))
                                    .foregroundColor(.textPrimary)
                                    .lineSpacing(6)
                                    .fixedSize(horizontal: false, vertical: true)

                                // Detailed analysis (if available)
                                if !enhancedAnalysis.isEmpty {
                                    Rectangle()
                                        .fill(accentColor.opacity(0.15))
                                        .frame(height: 1)

                                    HStack(spacing: 6) {
                                        Image(systemName: "text.magnifyingglass")
                                            .font(.system(size: 13))
                                            .foregroundColor(accentColor.opacity(0.8))
                                        Text("深度分析")
                                            .font(.system(size: 14, weight: .semibold))
                                            .foregroundColor(.textPrimary.opacity(0.85))
                                    }

                                    Text(enhancedAnalysis)
                                        .font(.system(size: 14))
                                        .foregroundColor(.textSecondary)
                                        .lineSpacing(6)
                                        .fixedSize(horizontal: false, vertical: true)
                                }
                            }
                        }
                        .padding(16)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(accentColor.opacity(0.06))
                        .cornerRadius(14)

                        // Sentiment analysis — always visible since sentiment is set in the first round
                        VStack(alignment: .leading, spacing: 10) {
                            HStack(spacing: 6) {
                                Image(systemName: "chart.bar.xaxis")
                                    .font(.system(size: 14))
                                    .foregroundColor(accentColor)
                                Text("情绪研判")
                                    .font(.system(size: 16, weight: .semibold))
                                    .foregroundColor(.textPrimary)
                            }

                            HStack(spacing: 12) {
                                sentimentIndicator
                                VStack(alignment: .leading, spacing: 4) {
                                    Text("AI 判断该新闻对标的影响为")
                                        .font(.system(size: 13))
                                        .foregroundColor(.textSecondary)
                                    Text(sentimentDescription)
                                        .font(.system(size: 14, weight: .medium))
                                        .foregroundColor(sentimentColor)
                                }
                            }
                        }
                        .padding(16)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(Color.secondaryBackground)
                        .cornerRadius(14)

                        // Open source link button
                        if !news.url.isEmpty, let _ = URL(string: news.url) {
                            Button(action: { showSafari = true }) {
                                HStack {
                                    Image(systemName: "safari")
                                        .font(.system(size: 16))
                                    Text("阅读原文")
                                        .font(.system(size: 15, weight: .medium))
                                    Spacer()
                                    Image(systemName: "arrow.up.right.square")
                                        .font(.system(size: 14))
                                }
                                .foregroundColor(accentColor)
                                .padding(16)
                                .background(Color.secondaryBackground)
                                .cornerRadius(14)
                            }
                        }

                        Spacer().frame(height: 40)
                    }
                    .padding(.horizontal, 20)
                }
            }
        }
        .sheet(isPresented: $showSafari) {
            if let url = URL(string: news.url) {
                SafariView(url: url)
                    .ignoresSafeArea()
            }
        }
        .onAppear {
            // Sentiment and summary are initialized in init() — always correct from frame 1.
            // Only need to load AI analysis if not already available.
            if !news.analysis.isEmpty {
                enhancedAnalysis = news.analysis
                return
            }

            // Check if PreloadManager has updated news with analysis since view was created
            let market = Market(rawValue: stockMarket) ?? .aShare
            if let cachedNews = PreloadManager.shared.getCachedNews(stockId: stockCode, market: market) {
                if let updatedItem = cachedNews.first(where: { $0.id == news.id }), !updatedItem.analysis.isEmpty {
                    enhancedSummary = updatedItem.summary.isEmpty ? news.summary : updatedItem.summary
                    enhancedAnalysis = updatedItem.analysis
                    enhancedSentiment = updatedItem.sentiment
                    return
                }
            }

            // Not in cache yet — fetch individually (enhancement may still be in-flight in background)
            fetchEnhancementFromNetwork()
        }
    }

    /// Fetch AI enhancement for this single news item from network (fallback)
    private func fetchEnhancementFromNetwork() {
        isLoadingAI = true
        StockDataService.shared.enhanceNewsItem(
            newsID: news.id,
            code: stockCode,
            market: stockMarket,
            name: stockName,
            title: news.title,
            summary: news.summary,
            source: news.source
        ) { summary, analysis, _ in
            withAnimation(.easeInOut(duration: 0.3)) {
                self.enhancedSummary = summary
                self.enhancedAnalysis = analysis
                self.isLoadingAI = false
            }
        }
    }

    private var sentimentBadge: some View {
        HStack(spacing: 4) {
            Circle()
                .fill(sentimentColor)
                .frame(width: 6, height: 6)
            Text(enhancedSentiment.label)
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(sentimentColor)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 6)
        .background(sentimentColor.opacity(0.12))
        .cornerRadius(8)
    }

    private var sentimentIndicator: some View {
        ZStack {
            Circle()
                .stroke(sentimentColor.opacity(0.3), lineWidth: 3)
                .frame(width: 48, height: 48)
            Circle()
                .trim(from: 0, to: sentimentProgress)
                .stroke(sentimentColor, style: StrokeStyle(lineWidth: 3, lineCap: .round))
                .frame(width: 48, height: 48)
                .rotationEffect(.degrees(-90))
            Image(systemName: sentimentIcon)
                .font(.system(size: 18))
                .foregroundColor(sentimentColor)
        }
    }

    private var sentimentColor: Color {
        switch enhancedSentiment {
        case .positive: return .accentGreen
        case .negative: return Color(hex: "E53935")
        case .neutral:  return .textSecondary
        }
    }

    private var sentimentProgress: CGFloat {
        switch enhancedSentiment {
        case .positive: return 0.85
        case .negative: return 0.75
        case .neutral:  return 0.5
        }
    }

    private var sentimentIcon: String {
        switch enhancedSentiment {
        case .positive: return "arrow.up.right"
        case .negative: return "arrow.down.right"
        case .neutral:  return "minus"
        }
    }

    private var sentimentDescription: String {
        switch enhancedSentiment {
        case .positive: return "利好 — 正面影响"
        case .negative: return "利空 — 负面影响"
        case .neutral:  return "中性 — 影响有限"
        }
    }
}

// MARK: - Safari View (for opening original article)

import SafariServices

struct SafariView: UIViewControllerRepresentable {
    let url: URL

    func makeUIViewController(context: Context) -> SFSafariViewController {
        let config = SFSafariViewController.Configuration()
        config.entersReaderIfAvailable = false
        let safari = SFSafariViewController(url: url, configuration: config)
        safari.preferredControlTintColor = UIColor(Color.accentBlue)
        return safari
    }

    func updateUIViewController(_ uiViewController: SFSafariViewController, context: Context) {}
}
