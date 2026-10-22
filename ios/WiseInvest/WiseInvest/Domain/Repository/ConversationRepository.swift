import Foundation
import Combine

/// Streaming chunk type from backend SSE
struct StreamChunk {
    let type: StreamChunkType
    let content: String
}

enum StreamChunkType: String {
    case content
    case thought
}

// MARK: - Group Chat SSE types

/// A streaming event from the multi-agent roundtable endpoint.
struct GroupStreamChunk {
    let agentId: String
    let type: GroupStreamChunkType
    let content: String
}

enum GroupStreamChunkType: String {
    case agentStart = "agent_start"
    case agentEnd   = "agent_end"
    case content
    case thought
}

/// Protocol defining conversation repository operations
protocol ConversationRepository {
    /// Get or create conversation ID for an agent
    func getOrCreateConversation(agentType: AgentType) -> AnyPublisher<UInt, Error>
    
    /// Send a message and receive streaming response
    func sendMessage(
        conversationId: UInt,
        message: String
    ) -> AnyPublisher<StreamChunk, Error>
    
    /// Get conversation history
    func getConversations() -> [Conversation]
    
    /// Save conversation
    func saveConversation(_ conversation: Conversation)
    
    /// Delete conversation
    func deleteConversation(id: String)
}
