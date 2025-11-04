import Foundation

/// Represents a conversation with an agent
struct Conversation: Identifiable, Codable {
    let id: String
    let agentType: AgentType
    var messages: [Message]
    let createdAt: Date
    var updatedAt: Date
    
    init(
        id: String = UUID().uuidString,
        agentType: AgentType,
        messages: [Message] = [],
        createdAt: Date = Date(),
        updatedAt: Date = Date()
    ) {
        self.id = id
        self.agentType = agentType
        self.messages = messages
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }
}
