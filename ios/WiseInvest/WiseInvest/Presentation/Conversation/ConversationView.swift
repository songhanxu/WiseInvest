import SwiftUI

/// Conversation screen for chatting with an agent
struct ConversationView: View {
    let agentType: AgentType
    @ObservedObject var coordinator: AppCoordinator
    @StateObject private var viewModel: ConversationViewModel
    @Environment(\.dismiss) private var dismiss
    
    init(agentType: AgentType, coordinator: AppCoordinator) {
        self.agentType = agentType
        self.coordinator = coordinator
        _viewModel = StateObject(wrappedValue: ConversationViewModel(
            agentType: agentType,
            conversationRepository: ConversationRepositoryImpl(apiClient: .shared)
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
                                MessageBubble(message: message)
                                    .id(message.id)
                            }
                        }
                        .padding()
                    }
                    .onChange(of: viewModel.messages.count) { _ in
                        if let lastMessage = viewModel.messages.last {
                            withAnimation {
                                proxy.scrollTo(lastMessage.id, anchor: .bottom)
                            }
                        }
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
        HStack {
            Button(action: { dismiss() }) {
                Image(systemName: "chevron.left")
                    .font(.system(size: 20, weight: .semibold))
                    .foregroundColor(.textPrimary)
            }
            
            Image(systemName: agentType.icon)
                .font(.system(size: 24))
                .foregroundColor(.accentBlue)
            
            VStack(alignment: .leading, spacing: 2) {
                Text(agentType.displayName)
                    .font(.system(size: 18, weight: .semibold))
                    .foregroundColor(.textPrimary)
                
                Text(agentType.description)
                    .font(.system(size: 12))
                    .foregroundColor(.textSecondary)
            }
            
            Spacer()
            
            Menu {
                Button(action: viewModel.clearConversation) {
                    Label("Clear Conversation", systemImage: "trash")
                }
            } label: {
                Image(systemName: "ellipsis")
                    .font(.system(size: 20))
                    .foregroundColor(.textPrimary)
            }
        }
        .padding()
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
            
            Button(action: viewModel.sendMessage) {
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
