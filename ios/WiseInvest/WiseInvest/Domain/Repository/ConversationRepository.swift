import Foundation
import Combine

/// Protocol defining conversation repository operations
protocol ConversationRepository {
    /// Get or create conversation ID for an agent
    func getOrCreateConversation(agentType: AgentType) -> AnyPublisher<UInt, Error>
    
    /// Send a message and receive streaming response
    func sendMessage(
        conversationId: UInt,
        message: String
    ) -> AnyPublisher<String, Error>
    
    /// Get conversation history
    func getConversations() -> [Conversation]
    
    /// Save conversation
    func saveConversation(_ conversation: Conversation)
    
    /// Delete conversation
    func deleteConversation(id: String)
}
