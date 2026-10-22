import SwiftUI

struct ChatTabView: View {
    @ObservedObject var coordinator: AppCoordinator
    @StateObject private var viewModel = ChatTabViewModel()
    @State private var showGroupChat = false
    @State private var sheetItem: ChatSheetItem?

    var body: some View {
        NavigationView {
            ZStack {
                Color.primaryBackground.ignoresSafeArea()

                ScrollView {
                    VStack(spacing: 0) {
                        groupChatRow

                        Divider()
                            .background(Color.textTertiary.opacity(0.1))
                            .padding(.leading, 84)

                        agentConversationList
                    }
                }
            }
            .navigationTitle("对话")
            .navigationBarTitleDisplayMode(.large)
        }
        .sheet(isPresented: $showGroupChat) {
            GroupChatView()
        }
        .sheet(item: $sheetItem) { item in
            switch item {
            case .market(let market):
                ConversationView(
                    agentType: AgentType(market: market),
                    market: market,
                    coordinator: coordinator
                )
            case .existingConversation(let market, let conversation):
                ConversationView(
                    agentType: AgentType(market: market),
                    market: market,
                    coordinator: coordinator,
                    existingConversation: conversation
                )
            }
        }
        .onAppear { viewModel.refresh() }
    }

    // MARK: - Group Chat Row (first in list)

    private var groupChatRow: some View {
        Button(action: { showGroupChat = true }) {
            HStack(spacing: 14) {
                groupAvatarView

                VStack(alignment: .leading, spacing: 4) {
                    Text("慧投圆桌")
                        .font(.system(size: 15, weight: .semibold))
                        .foregroundColor(.textPrimary)
                    Text("多 Agent 投资圆桌讨论")
                        .font(.system(size: 12))
                        .foregroundColor(.textSecondary)
                        .lineLimit(1)
                }

                Spacer()

                Image(systemName: "chevron.right")
                    .font(.system(size: 13))
                    .foregroundColor(.textTertiary)
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 14)
        }
        .buttonStyle(PlainButtonStyle())
    }

    private var groupAvatarView: some View {
        ZStack {
            RoundedRectangle(cornerRadius: 14)
                .fill(Color(hex: "1A1F3A"))

            LazyVGrid(
                columns: [GridItem(.fixed(21)), GridItem(.fixed(21))],
                spacing: 4
            ) {
                ForEach(GroupChatAgent.allAgents.prefix(4)) { agent in
                    ZStack {
                        Circle()
                            .fill(agent.color.opacity(0.85))
                        Image(systemName: agent.icon)
                            .font(.system(size: 9))
                            .foregroundColor(.white)
                    }
                    .frame(width: 21, height: 21)
                }
            }
            .padding(5)
        }
        .frame(width: 52, height: 52)
    }

    // MARK: - Section Divider

    private func sectionDivider(title: String) -> some View {
        HStack(spacing: 10) {
            Rectangle()
                .fill(Color.textTertiary.opacity(0.25))
                .frame(height: 1)
            Text(title)
                .font(.system(size: 12))
                .foregroundColor(.textTertiary)
                .fixedSize()
            Rectangle()
                .fill(Color.textTertiary.opacity(0.25))
                .frame(height: 1)
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 14)
    }

    // MARK: - Agent Conversation List

    private var agentConversationList: some View {
        VStack(spacing: 0) {
            ForEach(Market.allCases) { market in
                agentRow(for: market)
                Divider()
                    .background(Color.textTertiary.opacity(0.1))
                    .padding(.leading, 84)
            }

            if !viewModel.recentConversations.isEmpty {
                sectionDivider(title: "历史对话")

                ForEach(viewModel.recentConversations) { conversation in
                    recentConversationRow(conversation)
                    Divider()
                        .background(Color.textTertiary.opacity(0.1))
                        .padding(.leading, 84)
                }
            }
        }
    }

    private func agentRow(for market: Market) -> some View {
        Button(action: { sheetItem = .market(market) }) {
            HStack(spacing: 14) {
                // Market icon
                ZStack {
                    LinearGradient(
                        colors: market.gradientColors,
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                    .frame(width: 52, height: 52)
                    .cornerRadius(14)

                    Image(systemName: market.icon)
                        .font(.system(size: 22))
                        .foregroundColor(.white)
                }

                VStack(alignment: .leading, spacing: 4) {
                    Text(market.displayName + " Agent")
                        .font(.system(size: 15, weight: .semibold))
                        .foregroundColor(.textPrimary)
                    Text(market.description)
                        .font(.system(size: 12))
                        .foregroundColor(.textSecondary)
                        .lineLimit(1)
                }

                Spacer()

                Image(systemName: "chevron.right")
                    .font(.system(size: 13))
                    .foregroundColor(.textTertiary)
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 14)
        }
        .buttonStyle(PlainButtonStyle())
    }

    private func recentConversationRow(_ conversation: Conversation) -> some View {
        let market = Market(rawValue: conversation.agentType.rawValue) ?? .aShare

        return Button(action: {
            sheetItem = .existingConversation(market: market, conversation: conversation)
        }) {
            HStack(spacing: 14) {
                ZStack {
                    market.accentColor.opacity(0.15)
                        .frame(width: 52, height: 52)
                        .cornerRadius(14)
                    Image(systemName: market.icon)
                        .font(.system(size: 22))
                        .foregroundColor(market.accentColor)
                }

                VStack(alignment: .leading, spacing: 4) {
                    Text(conversation.title.isEmpty ? market.displayName + " · 对话" : conversation.title)
                        .font(.system(size: 15, weight: .medium))
                        .foregroundColor(.textPrimary)
                        .lineLimit(1)
                    Text(timeAgo(from: conversation.updatedAt))
                        .font(.system(size: 12))
                        .foregroundColor(.textTertiary)
                }

                Spacer()

                Image(systemName: "chevron.right")
                    .font(.system(size: 13))
                    .foregroundColor(.textTertiary)
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 12)
        }
        .buttonStyle(PlainButtonStyle())
    }

    private func timeAgo(from date: Date) -> String {
        let diff = Date().timeIntervalSince(date)
        if diff < 3600 { return "\(max(1, Int(diff / 60))) 分钟前" }
        if diff < 86400 { return "\(Int(diff / 3600)) 小时前" }
        let formatter = DateFormatter()
        formatter.dateFormat = "M月d日"
        return formatter.string(from: date)
    }
}

// MARK: - ChatSheetItem

private enum ChatSheetItem: Identifiable {
    case market(Market)
    case existingConversation(market: Market, conversation: Conversation)

    var id: String {
        switch self {
        case .market(let m): return "market_\(m.rawValue)"
        case .existingConversation(_, let c): return "conv_\(c.id)"
        }
    }
}

// MARK: - ChatTabViewModel

class ChatTabViewModel: ObservableObject {
    @Published var recentConversations: [Conversation] = []

    private let repository = ConversationRepositoryImpl(apiClient: .shared)

    init() { refresh() }

    func refresh() {
        recentConversations = repository.getConversations().prefix(5).map { $0 }
    }
}
