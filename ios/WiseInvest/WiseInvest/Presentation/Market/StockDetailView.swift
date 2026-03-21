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
    /// Client-side K-line retry state (with exponential backoff)
    @State private var klineRetryCount: Int = 0
    @State private var klineRetryTimer: Timer?
    private let klineMaxClientRetries = 3

    private let stockService = StockDataService.shared

    /// Use liveStock if available (refreshed from API), fallback to initial stock
    private var displayStock: Stock { liveStock ?? stock }

    var body: some View {
        ZStack {
            Color.primaryBackground.ignoresSafeArea()

            VStack(spacing: 0) {
                headerSection

                ScrollView {
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
                    color: displayStock.isUp ? .accentGreen : Color(hex: "E53935")
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
            }
            .padding(.horizontal, 20)

            if isLoadingNews {
                // Skeleton placeholders for news cards
                VStack(spacing: 8) {
                    ForEach(0..<3, id: \.self) { _ in
                        NewsCardSkeleton()
                    }
                }
                .padding(.horizontal, 16)
            } else if newsItems.isEmpty {
                Text("暂无相关资讯")
                    .font(.system(size: 13))
                    .foregroundColor(.textTertiary)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 20)
            } else {
                VStack(spacing: 8) {
                    ForEach(newsItems) { news in
                        NewsCard(news: news)
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
        // Refresh real-time stock quote (with high-water-mark protection)
        quoteCounter &+= 1
        let capturedSeq = quoteCounter
        stockService.getStockQuote(code: stock.id, market: market) { updated in
            guard capturedSeq > self.quoteHighWaterMark else { return }
            self.quoteHighWaterMark = capturedSeq
            if let updated = updated {
                self.liveStock = updated
            }
        }

        // Load K-line with selected period
        loadKLineData()

        // Load news
        loadNewsData()

        // Check watchlist status from the parent view's watchlist data
        isInWatchlist = watchlistIDs.contains(stock.id)
    }

    /// Load news data with timeout protection. The skeleton screen only dismisses
    /// when valid data arrives; on empty/error, it stays.
    private func loadNewsData() {
        isLoadingNews = true

        // Start timeout timer
        newsTimeoutTimer?.invalidate()
        newsTimeoutTimer = Timer.scheduledTimer(withTimeInterval: 8.0, repeats: false) { _ in
            if self.isLoadingNews {
                // On timeout, dismiss the news skeleton and show empty state
                // (news is less critical, just show "暂无资讯" rather than blocking)
                self.isLoadingNews = false
            }
        }

        stockService.getNews(for: stock) { news in
            self.newsTimeoutTimer?.invalidate()
            self.newsTimeoutTimer = nil
            self.newsItems = news
            self.isLoadingNews = false
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
        HStack(spacing: 0) {
            ForEach(StockDataService.KLinePeriod.allCases, id: \.rawValue) { period in
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

// MARK: - News Card

private struct NewsCard: View {
    let news: NewsItem

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                sentimentBadge
                Spacer()
                Text(news.time)
                    .font(.system(size: 11))
                    .foregroundColor(.textTertiary)
            }

            Text(news.title)
                .font(.system(size: 14, weight: .medium))
                .foregroundColor(.textPrimary)
                .lineLimit(2)

            HStack {
                Text(news.source)
                    .font(.system(size: 11))
                    .foregroundColor(.textTertiary)
                Spacer()
            }
        }
        .padding(14)
        .background(Color.secondaryBackground)
        .cornerRadius(14)
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
