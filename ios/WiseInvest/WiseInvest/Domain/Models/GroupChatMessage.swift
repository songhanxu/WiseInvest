import Foundation

/// A message in the investment roundtable group chat
struct GroupChatMessage: Identifiable {
    let id: UUID
    /// nil means the message is from the current user
    let agentId: String?
    var content: String
    let timestamp: Date
    var isStreaming: Bool

    init(agentId: String? = nil, content: String, timestamp: Date = Date(), isStreaming: Bool = false) {
        self.id = UUID()
        self.agentId = agentId
        self.content = content
        self.timestamp = timestamp
        self.isStreaming = isStreaming
    }

    var isFromUser: Bool { agentId == nil }

    var agent: GroupChatAgent? {
        guard let agentId else { return nil }
        return GroupChatAgent.agent(for: agentId)
    }
}
