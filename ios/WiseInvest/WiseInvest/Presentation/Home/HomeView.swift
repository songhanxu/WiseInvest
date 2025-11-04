import SwiftUI

/// Home screen showing available agents and recent conversations
struct HomeView: View {
    @ObservedObject var coordinator: AppCoordinator
    @StateObject private var viewModel: HomeViewModel
    
    init(coordinator: AppCoordinator) {
        self.coordinator = coordinator
        _viewModel = StateObject(wrappedValue: HomeViewModel(
            conversationRepository: ConversationRepositoryImpl(apiClient: .shared)
        ))
    }
    
    var body: some View {
        NavigationView {
            ZStack {
                Color.primaryBackground.ignoresSafeArea()
                
                ScrollView {
                    VStack(spacing: 24) {
                        // Header
                        headerSection
                        
                        // Agent Cards
                        agentCardsSection
                        
                        // Recent Conversations
                        if !viewModel.recentConversations.isEmpty {
                            recentConversationsSection
                        }
                    }
                    .padding()
                }
            }
            .navigationBarHidden(true)
        }
        .sheet(item: $selectedAgent) { agent in
            ConversationView(
                agentType: agent,
                coordinator: coordinator
            )
        }
    }
    
    @State private var selectedAgent: AgentType?
    
    private var headerSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("WiseInvest")
                .font(.system(size: 36, weight: .bold))
                .foregroundColor(.textPrimary)
            
            Text("Your AI-Powered Investment Assistant")
                .font(.system(size: 16))
                .foregroundColor(.textSecondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
    
    private var agentCardsSection: some View {
        VStack(spacing: 16) {
            Text("Choose Your Agent")
                .font(.system(size: 20, weight: .semibold))
                .foregroundColor(.textPrimary)
                .frame(maxWidth: .infinity, alignment: .leading)
            
            ForEach(viewModel.availableAgents, id: \.self) { agent in
                AgentCard(agentType: agent) {
                    selectedAgent = agent
                }
            }
        }
    }
    
    private var recentConversationsSection: some View {
        VStack(spacing: 16) {
            HStack {
                Text("Recent Conversations")
                    .font(.system(size: 20, weight: .semibold))
                    .foregroundColor(.textPrimary)
                
                Spacer()
            }
            
            ForEach(viewModel.recentConversations) { conversation in
                ConversationRow(conversation: conversation) {
                    selectedAgent = conversation.agentType
                }
            }
        }
    }
}

/// Row displaying a conversation summary
struct ConversationRow: View {
    let conversation: Conversation
    let onTap: () -> Void
    
    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 12) {
                Image(systemName: conversation.agentType.icon)
                    .font(.system(size: 24))
                    .foregroundColor(.accentBlue)
                    .frame(width: 40, height: 40)
                    .background(Color.secondaryBackground)
                    .clipShape(Circle())
                
                VStack(alignment: .leading, spacing: 4) {
                    Text(conversation.agentType.displayName)
                        .font(.system(size: 16, weight: .semibold))
                        .foregroundColor(.textPrimary)
                    
                    if let lastMessage = conversation.messages.last {
                        Text(lastMessage.content)
                            .font(.system(size: 14))
                            .foregroundColor(.textSecondary)
                            .lineLimit(1)
                    }
                }
                
                Spacer()
                
                Image(systemName: "chevron.right")
                    .font(.system(size: 14))
                    .foregroundColor(.textTertiary)
            }
            .padding()
            .background(Color.secondaryBackground)
            .cornerRadius(12)
        }
    }
}
