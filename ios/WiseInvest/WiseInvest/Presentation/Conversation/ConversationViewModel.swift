import Foundation
import Combine

/// ViewModel for Conversation screen
class ConversationViewModel: ObservableObject {
    @Published var messages: [Message] = []
    @Published var inputText: String = ""
    @Published var isLoading: Bool = false
    @Published var errorMessage: String?
    
    private let agentType: AgentType
    private let conversationRepository: ConversationRepository
    private var conversation: Conversation
    private var cancellables = Set<AnyCancellable>()
    private var streamingMessageId: String?
    private var conversationId: UInt?
    
    init(agentType: AgentType, conversationRepository: ConversationRepository) {
        self.agentType = agentType
        self.conversationRepository = conversationRepository
        self.conversation = Conversation(agentType: agentType)
        
        // Add welcome message
        let welcomeMessage = Message(
            role: .assistant,
            content: getWelcomeMessage(for: agentType)
        )
        messages.append(welcomeMessage)
        conversation.messages.append(welcomeMessage)
        
        // Get or create conversation
        conversationRepository.getOrCreateConversation(agentType: agentType)
            .receive(on: DispatchQueue.main)
            .sink(
                receiveCompletion: { [weak self] completion in
                    if case .failure(let error) = completion {
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
        
        let messageText = inputText
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
                updatedMessage.content += chunk
                self.messages[index] = updatedMessage
            }
        )
        .store(in: &cancellables)
    }
    
    func clearConversation() {
        messages.removeAll()
        conversation.messages.removeAll()
        
        // Add welcome message
        let welcomeMessage = Message(
            role: .assistant,
            content: getWelcomeMessage(for: agentType)
        )
        messages.append(welcomeMessage)
        conversation.messages.append(welcomeMessage)
    }
    
    private func getWelcomeMessage(for agentType: AgentType) -> String {
        switch agentType {
        case .investmentAdvisor:
            return "Hello! I'm your Investment Advisor. I can help you with investment strategies, market analysis, portfolio optimization, and financial planning. How can I assist you today?"
        case .tradingAgent:
            return "Hello! I'm your Trading Agent. I can help you execute trades on Binance, monitor your portfolio, and manage your cryptocurrency investments. What would you like to do?"
        }
    }
}
