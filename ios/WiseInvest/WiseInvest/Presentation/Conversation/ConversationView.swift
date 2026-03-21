import SwiftUI

/// Conversation screen for chatting with a market agent
struct ConversationView: View {
    let agentType: AgentType
    let market: Market?
    @ObservedObject var coordinator: AppCoordinator
    @StateObject private var viewModel: ConversationViewModel
    @Environment(\.dismiss) private var dismiss
    @FocusState private var isInputFocused: Bool
    /// Tracks whether the list bottom anchor is visible.
    /// Auto-scroll during streaming is skipped when the user has scrolled up.
    @State private var isAtBottom = true

    init(
        agentType: AgentType,
        market: Market? = nil,
        coordinator: AppCoordinator,
        existingConversation: Conversation? = nil
    ) {
        self.agentType = agentType
        self.market = market
        self.coordinator = coordinator
        _viewModel = StateObject(wrappedValue: ConversationViewModel(
            agentType: agentType,
            market: market,
            conversationRepository: ConversationRepositoryImpl(apiClient: .shared),
            existingConversation: existingConversation
        ))
    }
    
    var body: some View {
        ZStack {
            Color.primaryBackground.ignoresSafeArea()
            
            VStack(spacing: 0) {
                // Header
                headerSection
                
                // Messages
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
                            // Invisible anchor for scroll-to-bottom.
                            // onAppear/onDisappear tells us whether the user has scrolled
                            // the view so that the bottom is no longer visible.
                            Color.clear.frame(height: 1).id("__bottom__")
                                .onAppear  { isAtBottom = true  }
                                .onDisappear { isAtBottom = false }
                        }
                        .padding()
                    }
                    // 下拉时键盘随手指移动收起
                    .scrollDismissesKeyboard(.interactively)
                    // 点击消息区域收起键盘
                    .onTapGesture {
                        isInputFocused = false
                    }
                    // Scroll when a new message is added (user sent something — always follow)
                    .onChange(of: viewModel.messages.count) { _ in
                        isAtBottom = true
                        withAnimation(.easeOut(duration: 0.2)) {
                            proxy.scrollTo("__bottom__", anchor: .bottom)
                        }
                    }
                    // Scroll while streaming content grows — only if user hasn't scrolled up
                    .onChange(of: viewModel.messages.last?.content) { _ in
                        guard viewModel.isLoading && isAtBottom else { return }
                        proxy.scrollTo("__bottom__", anchor: .bottom)
                    }
                }
                
                // Error message
                if let error = viewModel.errorMessage {
                    errorBanner(error)
                }
                
                // Input area
                inputSection
            }
        }
    }
    
    private var headerSection: some View {
        HStack(spacing: 12) {
            Button(action: { dismiss() }) {
                Image(systemName: "chevron.left")
                    .font(.system(size: 20, weight: .semibold))
                    .foregroundColor(.textPrimary)
            }

            // Market icon with gradient background
            ZStack {
                LinearGradient(
                    colors: market?.gradientColors ?? [.accentBlue, .accentBlue],
                    startPoint: .topLeading,
                    endPoint: .bottomTrailing
                )
                .frame(width: 38, height: 38)
                .cornerRadius(10)

                Image(systemName: market?.icon ?? agentType.icon)
                    .font(.system(size: 18))
                    .foregroundColor(.white)
            }

            VStack(alignment: .leading, spacing: 1) {
                Text(market?.displayName ?? agentType.displayName)
                    .font(.system(size: 17, weight: .semibold))
                    .foregroundColor(.textPrimary)
                Text(market?.subtitle ?? agentType.description)
                    .font(.system(size: 12))
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
        .padding(.vertical, 12)
        .background(Color.secondaryBackground)
    }
    
    private var inputSection: some View {
        HStack(spacing: 12) {
            TextField("Type your message...", text: $viewModel.inputText, axis: .vertical)
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
                // 发送后收起键盘
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
    
    /// 判断是否是最后一条完整的 assistant 消息（用于显示重新生成按钮）
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
}
