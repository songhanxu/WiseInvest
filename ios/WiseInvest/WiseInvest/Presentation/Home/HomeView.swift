import SwiftUI
import Combine

struct HomeView: View {
    @ObservedObject var coordinator: AppCoordinator
    @EnvironmentObject var authState: AuthState
    @StateObject private var viewModel = HomeViewModel()
    @State private var sheetItem: ConversationSheetItem?
    @State private var showAccountMenu = false

    var body: some View {
        NavigationView {
            ZStack {
                Color.primaryBackground.ignoresSafeArea()

                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        headerSection
                        marketSection
                        if !viewModel.recentConversations.isEmpty {
                            recentSection
                        }
                    }
                }
            }
            .navigationBarHidden(true)
        }
        .sheet(item: $sheetItem, onDismiss: {
            viewModel.loadRecentConversations()
        }) { item in
            ConversationView(
                agentType: AgentType(market: item.market),
                market: item.market,
                coordinator: coordinator,
                existingConversation: item.existingConversation
            )
        }
    }

    // MARK: - Header

    private var headerSection: some View {
        HStack(alignment: .center) {
            VStack(alignment: .leading, spacing: 4) {
                Text("慧投")
                    .font(.system(size: 32, weight: .bold))
                    .foregroundColor(.white)
                Text("WiseInvest")
                    .font(.system(size: 14, weight: .medium))
                    .foregroundColor(.textSecondary)
            }
            Spacer()
            avatarButton
        }
        .padding(.horizontal, 20)
        .padding(.top, 20)
        .padding(.bottom, 28)
    }

    // MARK: - Avatar Button

    @ViewBuilder
    private var avatarButton: some View {
        Button(action: { showAccountMenu.toggle() }) {
            Group {
                if let user = authState.currentUser, !user.effectiveAvatar.isEmpty,
                   let url = URL(string: user.effectiveAvatar) {
                    AsyncImage(url: url) { phase in
                        switch phase {
                        case .success(let image):
                            image.resizable().scaledToFill()
                        default:
                            defaultAvatarView
                        }
                    }
                } else {
                    defaultAvatarView
                }
            }
            .frame(width: 44, height: 44)
            .clipShape(Circle())
            .overlay(Circle().stroke(Color.accentBlue.opacity(0.5), lineWidth: 1.5))
        }
        .confirmationDialog(
            accountMenuTitle,
            isPresented: $showAccountMenu,
            titleVisibility: .visible
        ) {
            Button("切换账号") {
                authState.signOut()
            }
            Button("退出登录", role: .destructive) {
                authState.signOut()
            }
            Button("取消", role: .cancel) {}
        }
    }

    private var defaultAvatarView: some View {
        Circle()
            .fill(Color.accentBlue.opacity(0.2))
            .overlay(
                Image(systemName: "person.fill")
                    .font(.system(size: 20))
                    .foregroundColor(.accentBlue)
            )
    }

    private var accountMenuTitle: String {
        if let user = authState.currentUser {
            let name = user.effectiveName
            let phone = user.phone.map { "手机：\($0)" } ?? "未绑定手机"
            return "\(name)\n\(phone)"
        }
        return "账号管理"
    }

    // MARK: - Market Cards

    private var marketSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionHeader(title: "选择市场", subtitle: "进入对话，开始分析")
                .padding(.horizontal, 20)

            VStack(spacing: 12) {
                ForEach(Market.allCases) { market in
                    MarketCard(market: market) {
                        sheetItem = ConversationSheetItem(market: market)
                    }
                    .padding(.horizontal, 20)
                }
            }
            .padding(.bottom, 8)
        }
    }

    // MARK: - Recent Conversations

    private var recentSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionHeader(title: "最近对话", subtitle: nil)
                .padding(.horizontal, 20)
                .padding(.top, 8)

            VStack(spacing: 8) {
                ForEach(viewModel.recentConversations) { conversation in
                    RecentConversationRow(conversation: conversation) {
                        if let market = Market(rawValue: conversation.agentType.rawValue) {
                            sheetItem = ConversationSheetItem(market: market, existingConversation: conversation)
                        }
                    }
                    .padding(.horizontal, 20)
                    .swipeActions(edge: .trailing) {
                        Button(role: .destructive) {
                            viewModel.deleteConversation(id: conversation.id)
                        } label: {
                            Label("删除", systemImage: "trash")
                        }
                    }
                }
            }
            .padding(.bottom, 32)
        }
    }
}

// MARK: - ConversationSheetItem

private struct ConversationSheetItem: Identifiable {
    let market: Market
    let existingConversation: Conversation?

    var id: String {
        existingConversation?.id ?? market.rawValue
    }

    init(market: Market, existingConversation: Conversation? = nil) {
        self.market = market
        self.existingConversation = existingConversation
    }
}

// MARK: - SectionHeader

private struct SectionHeader: View {
    let title: String
    let subtitle: String?

    var body: some View {
        HStack(alignment: .lastTextBaseline, spacing: 8) {
            Text(title)
                .font(.system(size: 18, weight: .semibold))
                .foregroundColor(.white)
            if let subtitle = subtitle {
                Text(subtitle)
                    .font(.system(size: 13))
                    .foregroundColor(.textTertiary)
            }
        }
    }
}

// MARK: - RecentConversationRow

private struct RecentConversationRow: View {
    let conversation: Conversation
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 14) {
                // Market icon
                ZStack {
                    market.accentColor.opacity(0.15)
                        .frame(width: 44, height: 44)
                        .cornerRadius(12)
                    Image(systemName: market.icon)
                        .font(.system(size: 18))
                        .foregroundColor(market.accentColor)
                }

                VStack(alignment: .leading, spacing: 3) {
                    Text(displayTitle)
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
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .background(Color.secondaryBackground)
            .cornerRadius(14)
        }
        .buttonStyle(ScaleButtonStyle())
    }

    private var market: Market {
        Market(rawValue: conversation.agentType.rawValue) ?? .aShare
    }

    /// Show AI-generated title when available, otherwise fall back to market name
    private var displayTitle: String {
        if !conversation.title.isEmpty {
            return conversation.title
        }
        return market.displayName + " · 新对话"
    }

    private func timeAgo(from date: Date) -> String {
        let diff = Date().timeIntervalSince(date)
        if diff < 3600 { return "\(Int(diff / 60)) 分钟前" }
        if diff < 86400 { return "\(Int(diff / 3600)) 小时前" }
        let formatter = DateFormatter()
        formatter.dateFormat = "M月d日"
        return formatter.string(from: date)
    }
}
