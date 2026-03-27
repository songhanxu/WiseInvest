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
    /// Overall AI conclusion (bullish/bearish/neutral)
    @State private var overallConclusion: AnalysisConclusion = .neutral
    @State private var conclusionSummary: String = ""
    @State private var isLoadingConclusion = false
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
    /// State for the comprehensive analysis detail view (triggered by the CTA button)
    @State private var showComprehensiveAnalysis = false
    /// Cached comprehensive analysis item (from preload)
    @State private var comprehensiveItem: AIAnalysisItem?

    private let stockService = StockDataService.shared
    private let preloadManager = PreloadManager.shared
    private let wsClient = WebSocketClient.shared

    /// Use liveStock if available (refreshed from API), fallback to initial stock
    private var displayStock: Stock { liveStock ?? stock }

    var body: some View {
        ZStack {
            Color.primaryBackground.ignoresSafeArea()

            VStack(spacing: 0) {
                headerSection
                priceSection
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.bottom, 8)
                    .background(Color.primaryBackground)

                ScrollView(.vertical, showsIndicators: false) {
                    LazyVStack(alignment: .leading, spacing: 0, pinnedViews: [.sectionHeaders]) {
                        // K-Line section with sticky header
                        Section(header: klineSectionHeader) {
                            klineSectionContent
                        }

                        // AI Analysis section with sticky header
                        Section(header: aiSectionHeader) {
                            aiSectionContent
                        }

                        // News section with sticky header
                        Section(header: newsSectionHeader) {
                            newsSectionContent
                        }

                        // Bottom spacer for the floating button
                        Spacer().frame(height: 80)
                    }
                }

                Spacer(minLength: 0)
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
        .onReceive(wsClient.eventPublisher.receive(on: DispatchQueue.main)) { event in
            handleWSQuoteEvent(event)
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
        .sheet(isPresented: $showComprehensiveAnalysis) {
            ComprehensiveAnalysisView(
                stock: displayStock,
                market: market,
                klineData: klineData,
                accentColor: market.accentColor,
                preloadedItem: comprehensiveItem
            )
        }
        .onReceive(NotificationCenter.default.publisher(for: .analysisEnhancementDidComplete)) { _ in
            if let cached = preloadManager.getCachedAnalysis(stockId: stock.id, market: market),
               let comp = cached.first(where: { $0.id == "comprehensive" && !$0.detail.isEmpty }) {
                self.comprehensiveItem = comp
                // Also update conclusion from the comprehensive item
                if !comp.conclusion.isEmpty {
                    self.overallConclusion = AnalysisConclusion(rawValue: comp.conclusion) ?? .neutral
                }
                if !comp.aiSummary.isEmpty {
                    self.conclusionSummary = comp.aiSummary
                    self.isLoadingConclusion = false
                }
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: .analysisConclusionBatchDidComplete)) { _ in
            // Batch analysis conclusions arrived — update if we don't have one yet
            if let cached = preloadManager.getCachedAnalysisConclusion(stockId: stock.id, market: market) {
                if self.overallConclusion == .neutral && self.conclusionSummary.isEmpty {
                    self.overallConclusion = AnalysisConclusion(rawValue: cached.conclusion) ?? .neutral
                    self.conclusionSummary = cached.summary
                    self.isLoadingConclusion = false
                }
            }
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

    // MARK: - K-Line Section (split for sticky header)

    /// Sticky header for K-line section
    private var klineSectionHeader: some View {
        HStack {
            Image(systemName: "chart.xyaxis.line")
                .font(.system(size: 14))
                .foregroundColor(market.accentColor)
            Text("K 线走势")
                .font(.system(size: 16, weight: .semibold))
                .foregroundColor(.textPrimary)
            Spacer()
            // Period selector inline in header
            periodSelectorCompact
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 10)
        .background(Color.primaryBackground)
    }

    /// K-line content below sticky header
    private var klineSectionContent: some View {
        Group {
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
        .padding(.bottom, 8)
    }

    // MARK: - AI Analysis Section (split for sticky header)

    /// Sticky header for AI analysis section
    private var aiSectionHeader: some View {
        HStack {
            Image(systemName: "brain.head.profile")
                .font(.system(size: 14))
                .foregroundColor(.accentBlue)
            Text("AI 智能分析")
                .font(.system(size: 16, weight: .semibold))
                .foregroundColor(.textPrimary)
            Spacer()
            // Overall conclusion badge
            if !analysisItems.isEmpty {
                conclusionBadge
            }
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 10)
        .background(Color.primaryBackground)
    }

    /// Conclusion badge showing bullish/bearish/neutral
    private var conclusionBadge: some View {
        HStack(spacing: 4) {
            if isLoadingConclusion {
                ProgressView()
                    .scaleEffect(0.6)
                    .frame(width: 12, height: 12)
            } else {
                Image(systemName: overallConclusion.icon)
                    .font(.system(size: 11))
            }
            Text(overallConclusion.label)
                .font(.system(size: 11, weight: .semibold))
        }
        .foregroundColor(conclusionColor)
        .padding(.horizontal, 10)
        .padding(.vertical, 4)
        .background(conclusionColor.opacity(0.12))
        .cornerRadius(6)
    }

    private var conclusionColor: Color {
        switch overallConclusion {
        case .bullish: return .accentGreen
        case .bearish: return Color(hex: "E53935")
        case .neutral: return .textSecondary
        }
    }

    /// AI analysis content below sticky header
    private var aiSectionContent: some View {
        Group {
            if isLoadingKline && analysisItems.isEmpty {
                VStack(spacing: 10) {
                    AnalysisCardSkeleton()
                    AnalysisCardSkeleton()
                    AnalysisCardSkeleton()
                }
                .padding(.horizontal, 16)
            } else if !analysisItems.isEmpty {
                // Conclusion summary banner
                if !conclusionSummary.isEmpty {
                    HStack(spacing: 8) {
                        Image(systemName: overallConclusion.icon)
                            .font(.system(size: 16))
                            .foregroundColor(conclusionColor)
                        Text(conclusionSummary)
                            .font(.system(size: 13))
                            .foregroundColor(.textSecondary)
                            .lineLimit(2)
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(conclusionColor.opacity(0.06))
                    .cornerRadius(10)
                    .padding(.horizontal, 16)
                    .padding(.bottom, 6)
                }

                // Three quick analysis cards (tech / trend / volume)
                VStack(spacing: 8) {
                    ForEach(analysisItems) { item in
                        QuickAnalysisCard(item: item)
                    }
                }
                .padding(.horizontal, 16)

                // "View AI Comprehensive Analysis" CTA button
                Button(action: { showComprehensiveAnalysis = true }) {
                    HStack(spacing: 8) {
                        Image(systemName: "brain.head.profile")
                            .font(.system(size: 15))
                            .foregroundColor(.white)
                        Text("点击查看 AI 综合分析")
                            .font(.system(size: 15, weight: .semibold))
                            .foregroundColor(.white)
                        Spacer()
                        Image(systemName: "chevron.right")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.white.opacity(0.7))
                    }
                    .padding(.horizontal, 16)
                    .padding(.vertical, 14)
                    .background(
                        LinearGradient(
                            colors: [market.accentColor, market.accentColor.opacity(0.7)],
                            startPoint: .leading,
                            endPoint: .trailing
                        )
                    )
                    .cornerRadius(12)
                }
                .buttonStyle(PlainButtonStyle())
                .padding(.horizontal, 16)
                .padding(.top, 8)
            }
        }
        .padding(.bottom, 8)
    }

    // MARK: - News Section (split for sticky header)

    /// Sticky header for the news section — pins to top when scrolled
    private var newsSectionHeader: some View {
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
        .padding(.vertical, 10)
        .background(Color.primaryBackground)
    }

    /// News section content (below the sticky header)
    private var newsSectionContent: some View {
        Group {
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

        // Load preloaded analysis conclusion immediately (before K-line finishes)
        if let cached = preloadManager.getCachedAnalysisConclusion(stockId: stock.id, market: market) {
            self.overallConclusion = AnalysisConclusion(rawValue: cached.conclusion) ?? .neutral
            self.conclusionSummary = cached.summary
            self.isLoadingConclusion = false
        }

        // Load preloaded comprehensive analysis from cache
        if let cachedItems = preloadManager.getCachedAnalysis(stockId: stock.id, market: market),
           let comp = cachedItems.first(where: { $0.id == "comprehensive" && !$0.detail.isEmpty }) {
            self.comprehensiveItem = comp
        }
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
                let items = Self.generateAnalysis(stock: self.displayStock, klineData: result.points)
                self.analysisItems = items
                // Cache analysis items
                self.preloadManager.updateAnalysisCache(stockId: self.stock.id, market: self.market, data: items)
                // Fetch overall conclusion from AI
                self.loadAnalysisConclusion(klineData: result.points)
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

    // MARK: - Period Selector (compact, in sticky header)

    private var periodSelectorCompact: some View {
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
                        .font(.system(size: 11, weight: selectedPeriod == period ? .semibold : .regular))
                        .foregroundColor(selectedPeriod == period ? .white : .textSecondary)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 5)
                        .background(
                            selectedPeriod == period
                                ? AnyView(
                                    RoundedRectangle(cornerRadius: 6)
                                        .fill(market.accentColor)
                                  )
                                : AnyView(Color.clear)
                        )
                }
                .buttonStyle(PlainButtonStyle())
            }
        }
        .padding(2)
        .background(Color.secondaryBackground.opacity(0.5))
        .cornerRadius(8)
    }

    // MARK: - Auto Refresh (WebSocket-based + HTTP fallback)

    private func startAutoRefresh() {
        tradingStatus = MarketTradingHours.tradingStatus(market: market)

        // Subscribe to real-time quote via WebSocket
        wsClient.connect()
        wsClient.subscribeQuote(market: market.rawValue, code: stock.id)

        // Fallback: if WebSocket is down, use timer-based HTTP polling
        if !wsClient.isConnected {
            guard let interval = MarketTradingHours.refreshInterval(market: market) else {
                return
            }
            refreshTimer = Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { _ in
                if wsClient.isConnected {
                    // WebSocket reconnected — stop HTTP fallback
                    refreshTimer?.invalidate()
                    refreshTimer = nil
                    return
                }
                refreshQuoteHTTP()
            }
        }
    }

    private func stopAutoRefresh() {
        refreshTimer?.invalidate()
        refreshTimer = nil
        wsClient.unsubscribeQuote(market: market.rawValue, code: stock.id)
    }

    /// Handle incoming WebSocket quote event — update live stock and sync K-line
    private func handleWSQuoteEvent(_ event: WSEvent) {
        switch event {
        case .quote(let wsMarket, let code, let updatedStock):
            guard wsMarket == market.rawValue, code == stock.id else { return }
            // Update trading status
            tradingStatus = MarketTradingHours.tradingStatus(market: market)
            // Apply update
            self.liveStock = updatedStock
            self.syncKLineWithRealtimePrice(updatedStock)

        default:
            break
        }
    }

    /// HTTP fallback: poll quote via HTTP when WebSocket is disconnected
    private func refreshQuoteHTTP() {
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

    /// Generate 3 quick analysis cards (tech/trend/volume) from real stock and K-line data.
    /// These are client-side summaries for instant display; the deep AI analysis comes from the backend.
    static func generateAnalysis(stock: Stock, klineData: [KLinePoint]) -> [AIAnalysisItem] {
        guard klineData.count >= 5 else { return [] }

        let recent5 = klineData.suffix(5)
        let recent20 = klineData.suffix(min(20, klineData.count))
        let ma5 = recent5.reduce(0.0) { $0 + $1.close } / Double(recent5.count)
        let ma20 = recent20.reduce(0.0) { $0 + $1.close } / Double(recent20.count)
        let trend = stock.currentPrice > ma5 ? "短期均线上方" : "短期均线下方"
        let maTrend = ma5 > ma20 ? "多头排列" : "空头排列"

        let support = klineData.suffix(10).map { $0.low }.min() ?? stock.low
        let resistance = klineData.suffix(10).map { $0.high }.max() ?? stock.high

        let periodChange = klineData.count > 1 ? ((klineData.last!.close - klineData.first!.open) / klineData.first!.open) * 100 : 0
        let direction = periodChange >= 0 ? "上涨" : "下跌"

        let avgVol = recent5.reduce(0.0) { $0 + $1.volume } / 5.0
        let currentVol = klineData.last?.volume ?? 0
        let volRatio = avgVol > 0 ? currentVol / avgVol : 1.0
        let volDesc = volRatio > 1.5 ? "放量" : (volRatio < 0.7 ? "缩量" : "量能平稳")

        // 1. 技术面分析卡片
        let techContent = "\(stock.name)当前价\(stock.priceText)，处于\(trend)，均线\(maTrend)。" +
            "MA5=\(String(format: "%.2f", ma5))，MA20=\(String(format: "%.2f", ma20))。" +
            "支撑位\(String(format: "%.2f", support))，阻力位\(String(format: "%.2f", resistance))。"

        // 2. 趋势分析卡片
        let trendContent = "近\(klineData.count)日累计\(direction)\(String(format: "%.2f", abs(periodChange)))%。" +
            "当前价格处于\(trend)，均线呈\(maTrend)，\(ma5 > ma20 ? "多头趋势延续" : "空头压力较大")。"

        // 3. 量能分析卡片
        let volContent = "当前\(volDesc)，量比\(String(format: "%.2f", volRatio))。" +
            "5日平均成交量\(String(format: "%.1f", avgVol))，" +
            "最新成交量\(String(format: "%.1f", currentVol))，" +
            "\(volRatio > 1.5 ? "资金活跃度较高" : (volRatio < 0.7 ? "市场观望情绪浓厚" : "资金参与度适中"))。"

        return [
            AIAnalysisItem(id: "tech", title: "技术面", icon: "chart.xyaxis.line", content: techContent),
            AIAnalysisItem(id: "trend", title: "趋势研判", icon: "arrow.up.right.circle", content: trendContent),
            AIAnalysisItem(id: "volume", title: "量能分析", icon: "chart.bar", content: volContent),
        ]
    }

    // MARK: - AI Analysis Conclusion

    /// Build a compact summary of K-line data for the AI backend
    static func buildKLineSummary(klineData: [KLinePoint]) -> String {
        guard !klineData.isEmpty else { return "无K线数据" }
        let count = klineData.count
        let last = klineData.last!
        let first = klineData.first!
        let recent5 = klineData.suffix(min(5, count))
        let recent20 = klineData.suffix(min(20, count))
        let ma5 = recent5.reduce(0.0) { $0 + $1.close } / Double(recent5.count)
        let ma20 = recent20.reduce(0.0) { $0 + $1.close } / Double(recent20.count)
        let periodHigh = klineData.map { $0.high }.max() ?? 0
        let periodLow = klineData.map { $0.low }.min() ?? 0
        let periodChange = count > 1 ? ((last.close - first.open) / first.open) * 100 : 0
        let avgVol = recent5.reduce(0.0) { $0 + $1.volume } / Double(recent5.count)
        let lastVol = last.volume
        let volRatio = avgVol > 0 ? lastVol / avgVol : 1.0

        return "共\(count)根K线，最新收盘\(String(format: "%.2f", last.close))，区间涨跌\(String(format: "%.2f", periodChange))%，MA5=\(String(format: "%.2f", ma5))，MA20=\(String(format: "%.2f", ma20))，区间最高\(String(format: "%.2f", periodHigh))，区间最低\(String(format: "%.2f", periodLow))，量比\(String(format: "%.2f", volRatio))"
    }

    /// Build a compact summary of current price data
    static func buildPriceSummary(stock: Stock) -> String {
        return "当前价\(stock.priceText)，涨跌\(stock.changeText)（\(stock.changePercentText)），高\(String(format: "%.2f", stock.high))，低\(String(format: "%.2f", stock.low))，量\(String(format: "%.1f亿", stock.volume))"
    }

    /// Load the overall bullish/bearish conclusion
    private func loadAnalysisConclusion(klineData: [KLinePoint]) {
        // Check preloaded cache first (from batch preload when entering market section)
        if let cached = preloadManager.getCachedAnalysisConclusion(stockId: stock.id, market: market) {
            self.overallConclusion = AnalysisConclusion(rawValue: cached.conclusion) ?? .neutral
            self.conclusionSummary = cached.summary
            self.isLoadingConclusion = false
            return
        }

        isLoadingConclusion = true
        let klineSummary = Self.buildKLineSummary(klineData: klineData)
        let priceSummary = Self.buildPriceSummary(stock: displayStock)

        stockService.getAnalysisConclusion(
            code: stock.id, market: market.rawValue, name: stock.name,
            klineSummary: klineSummary, priceSummary: priceSummary
        ) { conclusion, summary in
            self.isLoadingConclusion = false
            self.overallConclusion = AnalysisConclusion(rawValue: conclusion) ?? .neutral
            self.conclusionSummary = summary
            // Cache for future use
            self.preloadManager.updateAnalysisConclusionCache(
                stockId: self.stock.id, market: self.market,
                conclusion: conclusion, summary: summary
            )
        }
    }
}

// MARK: - Quick Analysis Card (compact, non-tappable)

private struct QuickAnalysisCard: View {
    let item: AIAnalysisItem

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 6) {
                Image(systemName: item.icon)
                    .font(.system(size: 13))
                    .foregroundColor(.accentBlue)
                Text(item.title)
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.textPrimary)
            }

            Text(item.content)
                .font(.system(size: 12))
                .foregroundColor(.textSecondary)
                .lineSpacing(3)
                .lineLimit(3)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.secondaryBackground)
        .cornerRadius(10)
    }
}

// MARK: - Comprehensive Analysis View (Full Screen with Markdown)

struct ComprehensiveAnalysisView: View {
    let stock: Stock
    let market: Market
    let klineData: [KLinePoint]
    var accentColor: Color = .accentBlue
    /// Preloaded comprehensive item (may have detail already)
    var preloadedItem: AIAnalysisItem? = nil
    @Environment(\.dismiss) private var dismiss
    @State private var isLoading = false
    @State private var detailText: String = ""
    @State private var summaryText: String = ""
    @State private var conclusion: AnalysisConclusion = .neutral
    @State private var errorMessage: String = ""
    @State private var retryCount: Int = 0

    var body: some View {
        ZStack {
            Color.primaryBackground.ignoresSafeArea()

            VStack(spacing: 0) {
                // Header
                HStack(spacing: 12) {
                    Button(action: { dismiss() }) {
                        Image(systemName: "chevron.left")
                            .font(.system(size: 20, weight: .semibold))
                            .foregroundColor(.textPrimary)
                    }

                    Image(systemName: "brain.head.profile")
                        .font(.system(size: 16))
                        .foregroundColor(accentColor)
                    Text("AI 综合分析")
                        .font(.system(size: 17, weight: .semibold))
                        .foregroundColor(.textPrimary)

                    Spacer()

                    // Conclusion badge
                    if !isLoading && !detailText.isEmpty {
                        conclusionBadge
                    }
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 12)
                .background(Color.secondaryBackground)

                ScrollView {
                    VStack(alignment: .leading, spacing: 20) {
                        // Stock info
                        HStack(spacing: 12) {
                            Text(stock.name)
                                .font(.system(size: 16, weight: .semibold))
                                .foregroundColor(.textPrimary)
                            Text(stock.symbol)
                                .font(.system(size: 13, design: .monospaced))
                                .foregroundColor(.textTertiary)
                            Spacer()
                            Text(stock.priceText)
                                .font(.system(size: 16, weight: .bold, design: .monospaced))
                                .foregroundColor(stock.isUp ? .accentGreen : Color(hex: "E53935"))
                            Text(stock.changePercentText)
                                .font(.system(size: 13, weight: .semibold, design: .monospaced))
                                .foregroundColor(stock.isUp ? .accentGreen : Color(hex: "E53935"))
                        }

                        Rectangle()
                            .fill(Color.white.opacity(0.08))
                            .frame(height: 1)

                        // AI Detailed Analysis (Markdown rendered)
                        VStack(alignment: .leading, spacing: 12) {
                            HStack(spacing: 6) {
                                Image(systemName: "sparkles")
                                    .font(.system(size: 14))
                                    .foregroundColor(accentColor)
                                Text("AI 深度分析")
                                    .font(.system(size: 16, weight: .semibold))
                                    .foregroundColor(.textPrimary)
                            }

                            if isLoading {
                                VStack(alignment: .leading, spacing: 10) {
                                    ForEach(0..<10, id: \.self) { _ in
                                        RoundedRectangle(cornerRadius: 4)
                                            .fill(Color.white.opacity(0.06))
                                            .frame(height: 14)
                                            .shimmer()
                                    }
                                    RoundedRectangle(cornerRadius: 4)
                                        .fill(Color.white.opacity(0.06))
                                        .frame(width: 180, height: 14)
                                        .shimmer()
                                }
                            } else if !detailText.isEmpty {
                                // Render detail with Markdown support
                                AnalysisMarkdownView(content: detailText)
                            } else {
                                // Failed state
                                VStack(spacing: 8) {
                                    Text(errorMessage.isEmpty ? "分析加载失败" : errorMessage)
                                        .font(.system(size: 14))
                                        .foregroundColor(.textTertiary)
                                        .multilineTextAlignment(.center)
                                    if retryCount < 3 {
                                        Button(action: { loadDetailAnalysis() }) {
                                            HStack(spacing: 4) {
                                                Image(systemName: "arrow.clockwise")
                                                    .font(.system(size: 12))
                                                Text("重试")
                                                    .font(.system(size: 13, weight: .medium))
                                            }
                                            .foregroundColor(accentColor)
                                        }
                                    } else {
                                        Text("多次重试失败，请稍后再试")
                                            .font(.system(size: 12))
                                            .foregroundColor(.textTertiary)
                                    }
                                }
                                .frame(maxWidth: .infinity)
                                .padding(.vertical, 20)
                            }
                        }
                        .padding(16)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(accentColor.opacity(0.06))
                        .cornerRadius(14)

                        // Conclusion section
                        if !isLoading && !detailText.isEmpty {
                            VStack(alignment: .leading, spacing: 10) {
                                HStack(spacing: 6) {
                                    Image(systemName: "chart.bar.xaxis")
                                        .font(.system(size: 14))
                                        .foregroundColor(accentColor)
                                    Text("AI 综合研判")
                                        .font(.system(size: 16, weight: .semibold))
                                        .foregroundColor(.textPrimary)
                                }

                                HStack(spacing: 12) {
                                    conclusionIndicator
                                    VStack(alignment: .leading, spacing: 4) {
                                        Text("AI 分析综合判断")
                                            .font(.system(size: 13))
                                            .foregroundColor(.textSecondary)
                                        Text(conclusionDescription)
                                            .font(.system(size: 14, weight: .medium))
                                            .foregroundColor(conclusionDisplayColor)
                                    }
                                }
                            }
                            .padding(16)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(Color.secondaryBackground)
                            .cornerRadius(14)
                        }

                        // Disclaimer
                        if !isLoading && !detailText.isEmpty {
                            Text("⚠️ 以上内容由 AI 自动生成，仅供参考，不构成投资建议。投资有风险，决策需谨慎。")
                                .font(.system(size: 11))
                                .foregroundColor(.textTertiary)
                                .padding(.horizontal, 4)
                        }

                        Spacer().frame(height: 40)
                    }
                    .padding(.horizontal, 20)
                    .padding(.top, 12)
                }
            }
        }
        .onAppear {
            retryCount = 0
            // Check if preloaded item already has detail
            if let preloaded = preloadedItem, !preloaded.detail.isEmpty {
                detailText = preloaded.detail
                summaryText = preloaded.aiSummary
                conclusion = AnalysisConclusion(rawValue: preloaded.conclusion) ?? .neutral
                return
            }
            // Check PreloadManager cache
            if let cachedItems = PreloadManager.shared.getCachedAnalysis(stockId: stock.id, market: market),
               let cached = cachedItems.first(where: { $0.id == "comprehensive" && !$0.detail.isEmpty }) {
                detailText = cached.detail
                summaryText = cached.aiSummary
                conclusion = AnalysisConclusion(rawValue: cached.conclusion) ?? .neutral
                return
            }
            loadDetailAnalysis()
        }
    }

    private func loadDetailAnalysis() {
        isLoading = true
        errorMessage = ""
        retryCount += 1

        // First try fetching from backend cache
        StockDataService.shared.getCachedAnalysis(code: stock.id, market: market.rawValue) { result in
            if let cached = result, !cached.detail.isEmpty {
                withAnimation(.easeInOut(duration: 0.3)) {
                    self.conclusion = AnalysisConclusion(rawValue: cached.conclusion) ?? .neutral
                    self.summaryText = cached.summary
                    self.detailText = cached.detail
                    self.isLoading = false
                    self.errorMessage = ""
                }
                PreloadManager.shared.updateSingleAnalysis(
                    stockId: stock.id, market: market, analysisId: "comprehensive",
                    conclusion: cached.conclusion, summary: cached.summary, detail: cached.detail
                )
                return
            }
            // Cache miss — call the enhance API
            self.callEnhanceAPI()
        }
    }

    private func callEnhanceAPI() {
        StockDataService.shared.enhanceAnalysis(
            code: stock.id, market: market.rawValue, name: stock.name
        ) { conclusionStr, summary, detail in
            if !detail.isEmpty {
                withAnimation(.easeInOut(duration: 0.3)) {
                    self.conclusion = AnalysisConclusion(rawValue: conclusionStr) ?? .neutral
                    self.summaryText = summary
                    self.detailText = detail
                    self.isLoading = false
                    self.errorMessage = ""
                }
                PreloadManager.shared.updateSingleAnalysis(
                    stockId: stock.id, market: market, analysisId: "comprehensive",
                    conclusion: conclusionStr, summary: summary, detail: detail
                )
            } else if self.retryCount <= 1 {
                // First failure: auto-retry after 5s
                DispatchQueue.main.asyncAfter(deadline: .now() + 5) {
                    self.retryCount += 1
                    StockDataService.shared.getCachedAnalysis(code: self.stock.id, market: self.market.rawValue) { result in
                        if let cached = result, !cached.detail.isEmpty {
                            withAnimation(.easeInOut(duration: 0.3)) {
                                self.conclusion = AnalysisConclusion(rawValue: cached.conclusion) ?? .neutral
                                self.summaryText = cached.summary
                                self.detailText = cached.detail
                                self.isLoading = false
                                self.errorMessage = ""
                            }
                            PreloadManager.shared.updateSingleAnalysis(
                                stockId: self.stock.id, market: self.market, analysisId: "comprehensive",
                                conclusion: cached.conclusion, summary: cached.summary, detail: cached.detail
                            )
                        } else {
                            self.callEnhanceAPI()
                        }
                    }
                }
            } else {
                withAnimation(.easeInOut(duration: 0.3)) {
                    self.conclusion = AnalysisConclusion(rawValue: conclusionStr) ?? .neutral
                    self.summaryText = summary
                    self.isLoading = false
                    self.detailText = ""
                    if !summary.isEmpty {
                        self.errorMessage = summary
                    } else {
                        self.errorMessage = "AI 分析生成失败，请稍后重试"
                    }
                }
            }
        }
    }

    private var conclusionBadge: some View {
        HStack(spacing: 4) {
            Circle()
                .fill(conclusionDisplayColor)
                .frame(width: 6, height: 6)
            Text(conclusion.label)
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(conclusionDisplayColor)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 6)
        .background(conclusionDisplayColor.opacity(0.12))
        .cornerRadius(8)
    }

    private var conclusionIndicator: some View {
        ZStack {
            Circle()
                .stroke(conclusionDisplayColor.opacity(0.3), lineWidth: 3)
                .frame(width: 48, height: 48)
            Circle()
                .trim(from: 0, to: conclusionProgress)
                .stroke(conclusionDisplayColor, style: StrokeStyle(lineWidth: 3, lineCap: .round))
                .frame(width: 48, height: 48)
                .rotationEffect(.degrees(-90))
            Image(systemName: conclusion.icon)
                .font(.system(size: 18))
                .foregroundColor(conclusionDisplayColor)
        }
    }

    private var conclusionDisplayColor: Color {
        switch conclusion {
        case .bullish: return .accentGreen
        case .bearish: return Color(hex: "E53935")
        case .neutral: return .textSecondary
        }
    }

    private var conclusionProgress: CGFloat {
        switch conclusion {
        case .bullish: return 0.85
        case .bearish: return 0.75
        case .neutral: return 0.5
        }
    }

    private var conclusionDescription: String {
        switch conclusion {
        case .bullish: return "看多 — 建议关注做多机会"
        case .bearish: return "看空 — 建议谨慎或关注做空"
        case .neutral: return "中性 — 观望为主"
        }
    }
}

// MARK: - Analysis Markdown View (renders AI analysis with markdown)

/// Custom markdown renderer for analysis content — uses the same approach as MarkdownContentView
/// but with styles adjusted for the analysis context (lighter background, smaller fonts)
struct AnalysisMarkdownView: View {
    let content: String

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            ForEach(Array(content.components(separatedBy: "\n").enumerated()), id: \.offset) { _, line in
                AnalysisMarkdownLine(line: line)
            }
        }
    }
}

/// Single line markdown renderer for analysis content
private struct AnalysisMarkdownLine: View {
    let line: String

    private var t: String { line.trimmingCharacters(in: .whitespaces) }

    var body: some View {
        Group {
            if t.isEmpty {
                Color.clear.frame(height: 8)
            } else if t.hasPrefix("#### ") {
                Text(LocalizedStringKey(String(t.dropFirst(5))))
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.textPrimary)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 8)
                    .padding(.bottom, 2)
            } else if t.hasPrefix("### ") {
                Text(LocalizedStringKey(String(t.dropFirst(4))))
                    .font(.system(size: 15, weight: .bold))
                    .foregroundColor(.textPrimary)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 10)
                    .padding(.bottom, 2)
            } else if t.hasPrefix("## ") {
                Text(LocalizedStringKey(String(t.dropFirst(3))))
                    .font(.system(size: 16, weight: .bold))
                    .foregroundColor(.textPrimary)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 12)
                    .padding(.bottom, 2)
            } else if t.hasPrefix("# ") {
                Text(LocalizedStringKey(String(t.dropFirst(2))))
                    .font(.system(size: 18, weight: .bold))
                    .foregroundColor(.textPrimary)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 12)
                    .padding(.bottom, 4)
            } else if t.hasPrefix("- ") || t.hasPrefix("* ") {
                HStack(alignment: .top, spacing: 6) {
                    Text("•")
                        .font(.system(size: 13))
                        .foregroundColor(.textTertiary)
                        .frame(width: 10, alignment: .center)
                        .padding(.top, 1)
                    Text(LocalizedStringKey(String(t.dropFirst(2))))
                        .font(.system(size: 14))
                        .foregroundColor(.textSecondary)
                        .fixedSize(horizontal: false, vertical: true)
                        .lineSpacing(4)
                }
                .padding(.vertical, 1)
            } else if t.range(of: #"^\d+\. "#, options: .regularExpression) != nil,
                      let dotIdx = t.firstIndex(of: ".") {
                let num = String(t[t.startIndex..<dotIdx])
                let content = String(t[t.index(dotIdx, offsetBy: 2)...])
                HStack(alignment: .top, spacing: 6) {
                    Text("\(num).")
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.textTertiary)
                        .frame(width: 18, alignment: .trailing)
                    Text(LocalizedStringKey(content))
                        .font(.system(size: 14))
                        .foregroundColor(.textSecondary)
                        .fixedSize(horizontal: false, vertical: true)
                        .lineSpacing(4)
                }
                .padding(.vertical, 1)
            } else if t.hasPrefix("> ") {
                HStack(alignment: .top, spacing: 8) {
                    Rectangle()
                        .fill(Color.accentBlue)
                        .frame(width: 3)
                        .cornerRadius(1.5)
                    Text(LocalizedStringKey(String(t.dropFirst(2))))
                        .font(.system(size: 13))
                        .foregroundColor(.textSecondary)
                        .fixedSize(horizontal: false, vertical: true)
                        .lineSpacing(4)
                }
                .padding(.vertical, 2)
            } else if t.hasPrefix("---") || t.hasPrefix("***") {
                Rectangle()
                    .fill(Color.white.opacity(0.1))
                    .frame(height: 1)
                    .padding(.vertical, 6)
            } else {
                // Regular paragraph with inline markdown parsing
                Text(LocalizedStringKey(t))
                    .font(.system(size: 14))
                    .foregroundColor(.textSecondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .lineSpacing(5)
                    .padding(.vertical, 1)
            }
        }
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
