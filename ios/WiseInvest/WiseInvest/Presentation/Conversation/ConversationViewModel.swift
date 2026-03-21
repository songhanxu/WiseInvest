import Foundation
import Combine

/// ViewModel for Conversation screen
class ConversationViewModel: ObservableObject {
    @Published var messages: [Message] = []
    @Published var inputText: String = ""
    @Published var isLoading: Bool = false
    @Published var errorMessage: String?

    private let agentType: AgentType
    private let market: Market?
    private let conversationRepository: ConversationRepository
    private var conversation: Conversation
    private var cancellables = Set<AnyCancellable>()
    private var streamingMessageId: String?
    private var conversationId: UInt?

    /// Optional stock context to prepend to each user message for stock-specific conversations
    private var stockContext: String?

    init(
        agentType: AgentType,
        market: Market? = nil,
        conversationRepository: ConversationRepository,
        existingConversation: Conversation? = nil,
        stockContext: String? = nil
    ) {
        self.agentType = agentType
        self.market = market
        self.conversationRepository = conversationRepository
        self.stockContext = stockContext

        if let existing = existingConversation, !existing.messages.isEmpty {
            // Restore saved conversation messages
            self.conversation = existing
            self.messages = existing.messages
        } else {
            // New conversation: show welcome message
            self.conversation = existingConversation ?? Conversation(agentType: agentType)

            let welcomeContent: String
            if stockContext != nil {
                // Stock-specific welcome message
                welcomeContent = "你好！我已获取到当前标的的实时数据和K线走势，可以为你进行深度分析。\n\n你可以问我关于技术面、基本面、买卖时机等任何问题。"
            } else {
                welcomeContent = market?.welcomeMessage ?? Self.welcomeMessage(for: agentType)
            }

            let welcomeMessage = Message(
                role: .assistant,
                content: welcomeContent
            )
            messages.append(welcomeMessage)
            conversation.messages.append(welcomeMessage)
        }

        // Get or create backend conversation
        conversationRepository.getOrCreateConversation(agentType: agentType)
            .receive(on: DispatchQueue.main)
            .sink(
                receiveCompletion: { [weak self] completion in
                    if case .failure(let error) = completion {
                        // Auto sign-out on 401 (expired/invalid token)
                        if case APIError.httpError(401, _) = error {
                            AuthState.shared.signOut()
                            return
                        }
                        self?.errorMessage = "Failed to initialize conversation: \(error.localizedDescription)"
                    }
                },
                receiveValue: { [weak self] id in
                    self?.conversationId = id
                }
            )
            .store(in: &cancellables)
    }
    
    func sendMessage() {
        guard !inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return
        }
        
        guard let conversationId = conversationId else {
            errorMessage = "Conversation not initialized"
            return
        }
        
        let userMessage = Message(role: .user, content: inputText)
        messages.append(userMessage)
        conversation.messages.append(userMessage)
        
        // Prepend stock context to user message for backend (not displayed in UI)
        let messageText: String
        if let ctx = stockContext {
            messageText = "\(ctx)\n\n用户提问: \(inputText)"
        } else {
            messageText = inputText
        }
        inputText = ""
        isLoading = true
        errorMessage = nil
        
        // Create streaming assistant message
        let assistantMessageId = UUID().uuidString
        streamingMessageId = assistantMessageId
        let streamingMessage = Message(
            id: assistantMessageId,
            role: .assistant,
            content: "",
            isStreaming: true
        )
        messages.append(streamingMessage)
        
        // Send message to backend
        conversationRepository.sendMessage(
            conversationId: conversationId,
            message: messageText
        )
        .receive(on: DispatchQueue.main)
        .sink(
            receiveCompletion: { [weak self] completion in
                guard let self = self else { return }
                self.isLoading = false
                
                if case .failure(let error) = completion {
                    self.errorMessage = error.localizedDescription
                    // Remove streaming message on error
                    self.messages.removeAll { $0.id == assistantMessageId }
                } else {
                    // Finalize streaming message
                    if let index = self.messages.firstIndex(where: { $0.id == assistantMessageId }) {
                        var finalMessage = self.messages[index]
                        finalMessage.isStreaming = false
                        self.messages[index] = finalMessage
                        self.conversation.messages.append(finalMessage)

                        // Generate title from first user message (only once)
                        if self.conversation.title.isEmpty {
                            let firstUserMsg = self.messages.first(where: { $0.role == .user })?.content ?? ""
                            if !firstUserMsg.isEmpty {
                                self.conversation.title = Self.generateTitle(from: firstUserMsg)
                            }
                        }

                        self.conversation.updatedAt = Date()
                        self.conversationRepository.saveConversation(self.conversation)
                    }
                }
                self.streamingMessageId = nil
            },
            receiveValue: { [weak self] chunk in
                guard let self = self,
                      let index = self.messages.firstIndex(where: { $0.id == assistantMessageId }) else {
                    return
                }

                var updatedMessage = self.messages[index]
                switch chunk.type {
                case .content:
                    updatedMessage.content += chunk.content
                case .thought:
                    let incomingLines = chunk.content
                        .components(separatedBy: .newlines)
                        .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
                        .filter { !$0.isEmpty }

                    for line in incomingLines where updatedMessage.thinkingLines.count < 4 {
                        if updatedMessage.thinkingLines.last != line {
                            updatedMessage.thinkingLines.append(line)
                        }
                    }
                }
                self.messages[index] = updatedMessage
            }
        )
        .store(in: &cancellables)
    }
    
    func regenerateLastMessage() {
        guard !isLoading else { return }
        guard let conversationId = conversationId else { return }

        guard let lastUserIndex = messages.lastIndex(where: { $0.role == .user }) else { return }
        let lastUserContent: String
        if let ctx = stockContext {
            lastUserContent = "\(ctx)\n\n用户提问: \(messages[lastUserIndex].content)"
        } else {
            lastUserContent = messages[lastUserIndex].content
        }

        if let lastAssistantIndex = messages.lastIndex(where: { $0.role == .assistant }),
           lastAssistantIndex > lastUserIndex {
            messages.remove(at: lastAssistantIndex)
        }

        isLoading = true
        errorMessage = nil

        let assistantMessageId = UUID().uuidString
        streamingMessageId = assistantMessageId
        let streamingMessage = Message(id: assistantMessageId, role: .assistant, content: "", isStreaming: true)
        messages.append(streamingMessage)

        conversationRepository.sendMessage(conversationId: conversationId, message: lastUserContent)
            .receive(on: DispatchQueue.main)
            .sink(
                receiveCompletion: { [weak self] completion in
                    guard let self = self else { return }
                    self.isLoading = false
                    if case .failure(let error) = completion {
                        self.errorMessage = error.localizedDescription
                        self.messages.removeAll { $0.id == assistantMessageId }
                    } else {
                        if let index = self.messages.firstIndex(where: { $0.id == assistantMessageId }) {
                            var finalMessage = self.messages[index]
                            finalMessage.isStreaming = false
                            self.messages[index] = finalMessage
                        }
                        self.conversation.updatedAt = Date()
                        self.conversationRepository.saveConversation(self.conversation)
                    }
                    self.streamingMessageId = nil
                },
                receiveValue: { [weak self] chunk in
                    guard let self = self,
                          let index = self.messages.firstIndex(where: { $0.id == assistantMessageId }) else { return }
                    var updatedMessage = self.messages[index]
                    switch chunk.type {
                    case .content:
                        updatedMessage.content += chunk.content
                    case .thought:
                        let incomingLines = chunk.content
                            .components(separatedBy: .newlines)
                            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
                            .filter { !$0.isEmpty }
                        for line in incomingLines where updatedMessage.thinkingLines.count < 4 {
                            if updatedMessage.thinkingLines.last != line {
                                updatedMessage.thinkingLines.append(line)
                            }
                        }
                    }
                    self.messages[index] = updatedMessage
                }
            )
            .store(in: &cancellables)
    }

    func clearConversation() {
        messages.removeAll()
        conversation.messages.removeAll()
        conversation.title = ""
        
        let welcomeMessage = Message(
            role: .assistant,
            content: market?.welcomeMessage ?? Self.welcomeMessage(for: agentType)
        )
        messages.append(welcomeMessage)
        conversation.messages.append(welcomeMessage)
        conversationRepository.saveConversation(conversation)
    }

    // MARK: - Title Generation

    /// Generates a concise "动词+名词" title from the user's first message.
    static func generateTitle(from firstUserMessage: String) -> String {
        var msg = firstUserMessage

        // Strip common filler phrases
        let fillers = ["请帮我", "帮我", "请你帮我", "请你", "请问", "你能帮我", "能帮我", "我想要了解", "我想了解", "我想知道", "我需要", "我要"]
        for filler in fillers {
            msg = msg.replacingOccurrences(of: filler, with: "")
        }
        msg = msg.replacingOccurrences(of: "一下", with: "")
        msg = msg.replacingOccurrences(of: " ", with: "")
        msg = msg.trimmingCharacters(in: .whitespacesAndNewlines)

        // Known action verbs to preserve
        let actionVerbs = ["分析", "查询", "查看", "对比", "预测", "解读", "介绍", "解析", "研究", "评估", "总结"]
        let hasVerb = actionVerbs.contains { msg.hasPrefix($0) }

        if !hasVerb {
            // Turn question form into "分析X" by stripping trailing question words
            let questionEndings = ["怎么样", "如何", "怎么了", "怎么看", "好不好", "吗", "？", "?"]
            for ending in questionEndings {
                if msg.hasSuffix(ending) {
                    msg = String(msg.dropLast(ending.count))
                    break
                }
            }
            msg = "分析" + msg.trimmingCharacters(in: .whitespacesAndNewlines)
        }

        // Truncate to at most 13 characters (enough for "查询恒瑞医药的股票走势" = 11 chars)
        if msg.count > 13 {
            msg = String(msg.prefix(13))
        }

        // Strip trailing punctuation
        let punctuation = CharacterSet(charactersIn: "，。！？,.!?、")
        msg = msg.trimmingCharacters(in: punctuation)

        return msg.isEmpty ? "新对话" : msg
    }

    // MARK: - Private Helpers

    private static func welcomeMessage(for agentType: AgentType) -> String {
        switch agentType {
        case .aShare:
            return Market.aShare.welcomeMessage
        case .usStock:
            return Market.usStock.welcomeMessage
        case .crypto:
            return Market.crypto.welcomeMessage
        case .investmentAdvisor, .conversation:
            return "你好！我是你的投资分析助手，可以帮你分析市场走势、解读财务数据、制定投资策略。有什么想聊的？"
        case .tradingAgent:
            return "你好！我是交易执行助手，可以帮你管理币安账户、分析合约机会、执行智能交易策略。需要什么帮助？"
        case .orchestrator:
            return "你好！我是智能调度助手，可以协调分析和交易功能，为你提供一体化的投资体验。有什么需要？"
        }
    }
}
