import Foundation
import Combine

class HomeViewModel: ObservableObject {
    @Published var recentConversations: [Conversation] = []

    private let conversationRepository: ConversationRepositoryImpl

    init() {
        self.conversationRepository = ConversationRepositoryImpl(apiClient: .shared)
        loadRecentConversations()
    }

    func loadRecentConversations() {
        recentConversations = conversationRepository.getConversations()
            .prefix(5)
            .map { $0 }
    }

    func deleteConversation(id: String) {
        conversationRepository.deleteConversation(id: id)
        loadRecentConversations()
    }
}
