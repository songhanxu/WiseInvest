import Foundation
import Combine

/// ViewModel for Home screen
class HomeViewModel: ObservableObject {
    @Published var availableAgents: [AgentType] = AgentType.allCases
    @Published var recentConversations: [Conversation] = []
    
    private let conversationRepository: ConversationRepository
    private var cancellables = Set<AnyCancellable>()
    
    init(conversationRepository: ConversationRepository) {
        self.conversationRepository = conversationRepository
        loadRecentConversations()
    }
    
    func loadRecentConversations() {
        recentConversations = conversationRepository.getConversations().prefix(5).map { $0 }
    }
    
    func deleteConversation(id: String) {
        conversationRepository.deleteConversation(id: id)
        loadRecentConversations()
    }
}
