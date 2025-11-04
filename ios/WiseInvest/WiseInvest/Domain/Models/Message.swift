import Foundation

/// Represents a chat message
struct Message: Identifiable, Codable, Equatable {
    let id: String
    let role: MessageRole
    var content: String  // Changed to var to support streaming updates
    let timestamp: Date
    var isStreaming: Bool
    
    init(
        id: String = UUID().uuidString,
        role: MessageRole,
        content: String,
        timestamp: Date = Date(),
        isStreaming: Bool = false
    ) {
        self.id = id
        self.role = role
        self.content = content
        self.timestamp = timestamp
        self.isStreaming = isStreaming
    }
}

/// Message role in conversation
enum MessageRole: String, Codable {
    case user
    case assistant
    case system
}
