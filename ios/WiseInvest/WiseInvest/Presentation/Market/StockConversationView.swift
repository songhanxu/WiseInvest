import SwiftUI

/// Conversation view with stock context - AI answers based on stock info and K-line data
struct StockConversationView: View {
    let stock: Stock
    let market: Market
    let klineData: [KLinePoint]
    @ObservedObject var coordinator: AppCoordinator
    @StateObject private var viewModel: ConversationViewModel
    @Environment(\.dismiss) private var dismiss
    @FocusState private var isInputFocused: Bool
    @State private var isAtBottom = true

    init(
        stock: Stock,
        market: Market,
        klineData: [KLinePoint],
        coordinator: AppCoordinator
    ) {
        self.stock = stock
        self.market = market
        self.klineData = klineData
        self.coordinator = coordinator

        // Build stock context for AI
        let context = Self.buildStockContext(stock: stock, klineData: klineData)

        _viewModel = StateObject(wrappedValue: ConversationViewModel(
            agentType: AgentType(market: market),
            market: market,
            conversationRepository: ConversationRepositoryImpl(apiClient: .shared),
            existingConversation: nil,
            stockContext: context
        ))
    }

    var body: some View {
        ZStack {
            Color.primaryBackground.ignoresSafeArea()

            VStack(spacing: 0) {
                headerSection
                stockInfoBar
                messagesSection
                if let error = viewModel.errorMessage {
                    errorBanner(error)
                }
                inputSection
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

            ZStack {
                LinearGradient(
                    colors: market.gradientColors,
                    startPoint: .topLeading,
                    endPoint: .bottomTrailing
                )
                .frame(width: 34, height: 34)
                .cornerRadius(8)

                Image(systemName: "brain.head.profile")
                    .font(.system(size: 16))
                    .foregroundColor(.white)
            }

            VStack(alignment: .leading, spacing: 1) {
                Text("慧投 · \(stock.name)")
                    .font(.system(size: 16, weight: .semibold))
                    .foregroundColor(.textPrimary)
                Text("基于实时数据的AI分析")
                    .font(.system(size: 11))
                    .foregroundColor(.textSecondary)
            }

            Spacer()

            Menu {
                Button(action: viewModel.clearConversation) {
                    Label("清空对话", systemImage: "trash")
                }
            } label: {
                Image(systemName: "ellipsis")
                    .font(.system(size: 20))
                    .foregroundColor(.textPrimary)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(Color.secondaryBackground)
    }

    // MARK: - Stock Info Bar

    private var stockInfoBar: some View {
        HStack(spacing: 16) {
            Text(stock.name)
                .font(.system(size: 14, weight: .medium))
                .foregroundColor(.textPrimary)

            Text(stock.priceText)
                .font(.system(size: 14, weight: .semibold, design: .monospaced))
                .foregroundColor(stock.isUp ? .accentGreen : Color(hex: "E53935"))

            Text(stock.changePercentText)
                .font(.system(size: 12, weight: .medium, design: .monospaced))
                .foregroundColor(stock.isUp ? .accentGreen : Color(hex: "E53935"))

            Spacer()

            Text(stock.symbol)
                .font(.system(size: 11, design: .monospaced))
                .foregroundColor(.textTertiary)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 8)
        .background(Color.secondaryBackground.opacity(0.5))
    }

    // MARK: - Messages

    private var messagesSection: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(spacing: 16) {
                    ForEach(viewModel.messages) { message in
                        MessageBubble(
                            message: message,
                            onRegenerate: isLastAssistantMessage(message)
                                ? { viewModel.regenerateLastMessage() }
                                : nil
                        )
                        .id(message.id)
                    }
                    Color.clear.frame(height: 1).id("__bottom__")
                        .onAppear  { isAtBottom = true  }
                        .onDisappear { isAtBottom = false }
                }
                .padding()
            }
            .scrollDismissesKeyboard(.interactively)
            .onTapGesture { isInputFocused = false }
            .onChange(of: viewModel.messages.count) { _ in
                isAtBottom = true
                withAnimation(.easeOut(duration: 0.2)) {
                    proxy.scrollTo("__bottom__", anchor: .bottom)
                }
            }
            .onChange(of: viewModel.messages.last?.content) { _ in
                guard viewModel.isLoading && isAtBottom else { return }
                proxy.scrollTo("__bottom__", anchor: .bottom)
            }
        }
    }

    // MARK: - Input

    private var inputSection: some View {
        HStack(spacing: 12) {
            TextField("向慧投提问关于\(stock.name)的问题...", text: $viewModel.inputText, axis: .vertical)
                .textFieldStyle(.plain)
                .padding(12)
                .background(Color.secondaryBackground)
                .cornerRadius(20)
                .foregroundColor(.textPrimary)
                .lineLimit(1...5)
                .disabled(viewModel.isLoading)
                .focused($isInputFocused)

            Button(action: {
                viewModel.sendMessage()
                isInputFocused = false
            }) {
                if viewModel.isLoading {
                    ProgressView()
                        .progressViewStyle(CircularProgressViewStyle(tint: .white))
                        .frame(width: 44, height: 44)
                } else {
                    Image(systemName: "arrow.up.circle.fill")
                        .font(.system(size: 44))
                        .foregroundColor(viewModel.inputText.isEmpty ? .textTertiary : .accentBlue)
                }
            }
            .disabled(viewModel.inputText.isEmpty || viewModel.isLoading)
        }
        .padding()
        .background(Color.primaryBackground)
    }

    // MARK: - Helpers

    private func isLastAssistantMessage(_ message: Message) -> Bool {
        guard message.role == .assistant, !message.isStreaming else { return false }
        return viewModel.messages.last(where: { $0.role == .assistant && !$0.isStreaming })?.id == message.id
    }

    private func errorBanner(_ message: String) -> some View {
        HStack {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundColor(.red)
            Text(message)
                .font(.system(size: 14))
                .foregroundColor(.textPrimary)
            Spacer()
            Button(action: { viewModel.errorMessage = nil }) {
                Image(systemName: "xmark")
                    .foregroundColor(.textSecondary)
            }
        }
        .padding()
        .background(Color.red.opacity(0.2))
        .cornerRadius(8)
        .padding(.horizontal)
    }

    // MARK: - Stock Context Builder

    static func buildStockContext(stock: Stock, klineData: [KLinePoint]) -> String {
        var ctx = """
        【当前分析标的】\(stock.name) (\(stock.symbol))
        当前价格: \(stock.priceText)
        涨跌: \(stock.changeText) (\(stock.changePercentText))
        今日高/低: \(String(format: "%.2f", stock.high)) / \(String(format: "%.2f", stock.low))
        开盘价: \(String(format: "%.2f", stock.open))
        昨收价: \(String(format: "%.2f", stock.previousClose))
        成交量: \(String(format: "%.1f", stock.volume))亿
        """

        if !klineData.isEmpty {
            let recent = klineData.suffix(10)
            ctx += "\n\n【近期K线数据（最近\(recent.count)个交易日）】\n"
            let formatter = DateFormatter()
            formatter.dateFormat = "MM-dd"
            for point in recent {
                ctx += "\(formatter.string(from: point.date)): 开\(String(format: "%.2f", point.open)) 收\(String(format: "%.2f", point.close)) 高\(String(format: "%.2f", point.high)) 低\(String(format: "%.2f", point.low))\n"
            }

            // Simple trend analysis
            if let first = recent.first, let last = recent.last {
                let trendPercent = ((last.close - first.open) / first.open) * 100
                let trend = trendPercent >= 0 ? "上涨" : "下跌"
                ctx += "\n近期趋势: \(trend) \(String(format: "%.2f", abs(trendPercent)))%"
            }
        }

        return ctx
    }
}
