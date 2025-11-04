import Foundation
import Combine

/// Implementation of ConversationRepository
class ConversationRepositoryImpl: ConversationRepository {
    private let apiClient: APIClient
    private var conversations: [Conversation] = []
    private var conversationIdCache: [AgentType: UInt] = [:]
    
    init(apiClient: APIClient) {
        self.apiClient = apiClient
        loadConversations()
    }
    
    func getOrCreateConversation(agentType: AgentType) -> AnyPublisher<UInt, Error> {
        // Check cache first
        if let cachedId = conversationIdCache[agentType] {
            return Just(cachedId)
                .setFailureType(to: Error.self)
                .eraseToAnyPublisher()
        }
        
        // Create new conversation
        return apiClient.getOrCreateConversation(agentType: agentType)
            .handleEvents(receiveOutput: { [weak self] id in
                self?.conversationIdCache[agentType] = id
            })
            .eraseToAnyPublisher()
    }
    
    func sendMessage(
        conversationId: UInt,
        message: String
    ) -> AnyPublisher<String, Error> {
        return apiClient.sendChatMessage(
            conversationId: conversationId,
            message: message
        )
    }
    
    func getConversations() -> [Conversation] {
        return conversations.sorted { $0.updatedAt > $1.updatedAt }
    }
    
    func saveConversation(_ conversation: Conversation) {
        if let index = conversations.firstIndex(where: { $0.id == conversation.id }) {
            conversations[index] = conversation
        } else {
            conversations.append(conversation)
        }
        persistConversations()
    }
    
    func deleteConversation(id: String) {
        conversations.removeAll { $0.id == id }
        persistConversations()
    }
    
    // MARK: - Private Methods
    
    private func loadConversations() {
        guard let data = UserDefaults.standard.data(forKey: "conversations"),
              let decoded = try? JSONDecoder().decode([Conversation].self, from: data) else {
            return
        }
        conversations = decoded
    }
    
    private func persistConversations() {
        guard let encoded = try? JSONEncoder().encode(conversations) else {
            return
        }
        UserDefaults.standard.set(encoded, forKey: "conversations")
    }
}
