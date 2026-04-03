import Foundation
import Combine

class GroupChatViewModel: ObservableObject {
    @Published var messages: [GroupChatMessage] = []
    @Published var inputText: String = ""
    @Published var isProcessing: Bool = false
    @Published var errorMessage: String?

    private var conversationId: UInt?
    private var cancellables = Set<AnyCancellable>()

    // Tracks the message currently being streamed for each agent persona ID
    private var streamingMessageIds: [String: UUID] = [:]

    init() {
        appendWelcome()
        initConversation()
    }

    // MARK: - Public

    func sendMessage() {
        let text = inputText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, !isProcessing else { return }

        guard let conversationId else {
            errorMessage = "对话初始化中，请稍候再试"
            return
        }

        let userMsg = GroupChatMessage(agentId: nil, content: text)
        messages.append(userMsg)
        inputText = ""
        isProcessing = true
        errorMessage = nil
        streamingMessageIds = [:]

        let participants = resolvedParticipants(for: text)

        APIClient.shared
            .sendGroupChatMessage(conversationId: conversationId, message: text, participants: participants)
            .receive(on: DispatchQueue.main)
            .sink(
                receiveCompletion: { [weak self] completion in
                    guard let self else { return }
                    self.isProcessing = false
                    self.streamingMessageIds.removeAll()
                    if case .failure(let error) = completion {
                        if case APIError.httpError(401, _) = error {
                            AuthState.shared.signOut()
                            return
                        }
                        self.errorMessage = error.localizedDescription
                    }
                },
                receiveValue: { [weak self] chunk in
                    self?.handle(chunk: chunk)
                }
            )
            .store(in: &cancellables)
    }

    // MARK: - Private: SSE event handling

    private func handle(chunk: GroupStreamChunk) {
        switch chunk.type {

        case .agentStart:
            // Create a placeholder streaming message for this agent
            let msg = GroupChatMessage(agentId: chunk.agentId, content: "", isStreaming: true)
            streamingMessageIds[chunk.agentId] = msg.id
            messages.append(msg)

        case .content:
            guard
                let msgId = streamingMessageIds[chunk.agentId],
                let idx = messages.firstIndex(where: { $0.id == msgId })
            else { return }
            messages[idx].content += chunk.content

        case .thought:
            // Thoughts are transient; we could show them in the streaming indicator
            // but for group chat we simply discard them to keep the UI clean.
            break

        case .agentEnd:
            guard
                let msgId = streamingMessageIds[chunk.agentId],
                let idx = messages.firstIndex(where: { $0.id == msgId })
            else { return }
            messages[idx].isStreaming = false
            streamingMessageIds.removeValue(forKey: chunk.agentId)
        }
    }

    // MARK: - Private: @mention routing

    /// Returns the ordered participant list based on @mentions in the user text.
    /// Defaults to all four agents when no explicit mention is found.
    private func resolvedParticipants(for text: String) -> [String] {
        let mentionMap: [(String, String)] = [
            ("@主持人", "orchestrator"),
            ("@价值派", "value"),
            ("@趋势派", "trend"),
            ("@量化派", "quant"),
        ]

        // @主持人 or @all → all agents
        if text.contains("@主持人") || text.contains("@all") || text.contains("@所有人") {
            return []   // empty = all, handled by backend
        }

        // Check for specific individual mentions
        var mentioned: [String] = []
        for (mention, id) in mentionMap where mention != "@主持人" {
            if text.contains(mention) { mentioned.append(id) }
        }
        return mentioned.isEmpty ? [] : mentioned
    }

    // MARK: - Private: initialisation

    private func appendWelcome() {
        let welcome = GroupChatMessage(
            agentId: GroupChatAgent.orchestrator.id,
            content: "欢迎来到慧投圆桌！这里汇聚了**价值派**、**趋势派**、**量化派**三位分析师。\n\n发送消息即可开始圆桌讨论，@主持人 可让所有人参与，也可直接 @价值派、@趋势派、@量化派 单独咨询。",
            isStreaming: false
        )
        messages.append(welcome)
    }

    func retryInit() {
        errorMessage = nil
        initConversation()
    }

    private func initConversation() {
        APIClient.shared
            .getOrCreateConversation(agentType: .groupChat)
            .receive(on: DispatchQueue.main)
            .sink(
                receiveCompletion: { [weak self] completion in
                    if case .failure(let error) = completion {
                        if case APIError.httpError(401, _) = error {
                            AuthState.shared.signOut()
                            return
                        }
                        self?.errorMessage = "初始化失败：\(error.localizedDescription)"
                    }
                },
                receiveValue: { [weak self] id in
                    self?.conversationId = id
                }
            )
            .store(in: &cancellables)
    }
}
