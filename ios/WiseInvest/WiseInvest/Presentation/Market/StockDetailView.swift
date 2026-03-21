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
    @State private var isLoadingNews = true
    @State private var liveStock: Stock?

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

                        // K-Line chart
                        if isLoadingKline {
                            KLineChartSkeleton()
                                .padding(.horizontal, 16)
                        } else if !klineData.isEmpty {
                            KLineChartView(data: klineData, accentColor: market.accentColor)
                                .padding(.horizontal, 16)
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
                Text(displayStock.symbol)
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundColor(.textSecondary)
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
                Text(displayStock.priceText)
                    .font(.system(size: 36, weight: .bold, design: .monospaced))
                    .foregroundColor(displayStock.isUp ? .accentGreen : Color(hex: "E53935"))

                VStack(alignment: .leading, spacing: 2) {
                    Text(displayStock.changeText)
                        .font(.system(size: 14, weight: .medium, design: .monospaced))
                        .foregroundColor(displayStock.isUp ? .accentGreen : Color(hex: "E53935"))
                    Text(displayStock.changePercentText)
                        .font(.system(size: 14, weight: .semibold, design: .monospaced))
                        .foregroundColor(displayStock.isUp ? .accentGreen : Color(hex: "E53935"))
                }
            }

            HStack(spacing: 24) {
                priceTag("高", value: String(format: "%.2f", displayStock.high), color: .accentGreen)
                priceTag("低", value: String(format: "%.2f", displayStock.low), color: Color(hex: "E53935"))
                priceTag("量", value: String(format: "%.1f亿", displayStock.volume), color: .textSecondary)
            }
        }
        .padding(.horizontal, 20)
        .padding(.top, 12)
    }

    private func priceTag(_ label: String, value: String, color: Color) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label)
                .font(.system(size: 11))
                .foregroundColor(.textTertiary)
            Text(value)
                .font(.system(size: 13, weight: .medium, design: .monospaced))
                .foregroundColor(color)
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
        // Refresh real-time stock quote
        stockService.getStockQuote(code: stock.id, market: market) { updated in
            if let updated = updated {
                self.liveStock = updated
            }
        }

        // Load K-line
        isLoadingKline = true
        stockService.getKLineData(for: stock) { points in
            self.klineData = points
            self.isLoadingKline = false
            // Generate AI analysis from real K-line data
            self.analysisItems = Self.generateAnalysis(stock: self.displayStock, klineData: points)
        }

        // Load news
        isLoadingNews = true
        stockService.getNews(for: stock) { news in
            self.newsItems = news
            self.isLoadingNews = false
        }

        // Check watchlist status from the parent view's watchlist data
        isInWatchlist = watchlistIDs.contains(stock.id)
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
