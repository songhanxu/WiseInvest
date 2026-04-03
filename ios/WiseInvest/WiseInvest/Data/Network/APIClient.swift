import Foundation
import Combine

/// SSE delegate that parses group chat events (include agent_id field).
private class GroupStreamingDelegate: NSObject, URLSessionDataDelegate {
    let subject: PassthroughSubject<GroupStreamChunk, Error>
    var buffer = ""

    init(subject: PassthroughSubject<GroupStreamChunk, Error>) {
        self.subject = subject
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
        guard let chunk = String(data: data, encoding: .utf8) else { return }
        buffer += chunk

        let lines = buffer.components(separatedBy: "\n")
        buffer = lines.last ?? ""

        for line in lines.dropLast() {
            guard line.hasPrefix("data: ") else { continue }
            let jsonString = String(line.dropFirst(6))
            if jsonString == "[DONE]" {
                subject.send(completion: .finished)
                return
            }
            guard
                let jsonData = jsonString.data(using: .utf8),
                let json = try? JSONSerialization.jsonObject(with: jsonData) as? [String: Any]
            else { continue }

            if let errorText = json["error"] as? String {
                subject.send(completion: .failure(
                    NSError(domain: "SSE", code: -1, userInfo: [NSLocalizedDescriptionKey: errorText])
                ))
                return
            }

            let agentId  = (json["agent_id"] as? String) ?? ""
            let typeRaw  = (json["type"]     as? String) ?? GroupStreamChunkType.content.rawValue
            let content  = (json["content"]  as? String) ?? ""
            let chunkType = GroupStreamChunkType(rawValue: typeRaw) ?? .content
            subject.send(GroupStreamChunk(agentId: agentId, type: chunkType, content: content))
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        if let error = error {
            subject.send(completion: .failure(error))
        } else {
            subject.send(completion: .finished)
        }
    }
}

/// Streaming delegate for SSE
private class StreamingDelegate: NSObject, URLSessionDataDelegate {
    let subject: PassthroughSubject<StreamChunk, Error>
    var buffer = ""
    
    init(subject: PassthroughSubject<StreamChunk, Error>) {
        self.subject = subject
    }
    
    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
        guard let chunk = String(data: data, encoding: .utf8) else { return }
        buffer += chunk
        
        // Process complete lines
        let lines = buffer.components(separatedBy: "\n")
        buffer = lines.last ?? ""
        
        for line in lines.dropLast() {
            if line.hasPrefix("data: ") {
                let jsonString = String(line.dropFirst(6))
                if jsonString == "[DONE]" {
                    subject.send(completion: .finished)
                    return
                }
                
                if let data = jsonString.data(using: .utf8),
                   let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                    if let errorText = json["error"] as? String {
                        subject.send(completion: .failure(NSError(domain: "SSE", code: -1, userInfo: [NSLocalizedDescriptionKey: errorText])))
                        return
                    }

                    guard let content = json["content"] as? String else { continue }
                    let typeRaw = (json["type"] as? String) ?? StreamChunkType.content.rawValue
                    let type = StreamChunkType(rawValue: typeRaw) ?? .content
                    subject.send(StreamChunk(type: type, content: content))
                }
            }
        }
    }
    
    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        if let error = error {
            subject.send(completion: .failure(error))
        } else {
            subject.send(completion: .finished)
        }
    }
}

/// API client for backend communication
class APIClient {
    static let shared = APIClient()
    
    private let baseURL: String
    private let session: URLSession
    
    private init() {
        self.baseURL = APIConfig.baseURL

        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 60
        config.timeoutIntervalForResource = 300
        self.session = URLSession(configuration: config)
    }
    
    /// Adds the JWT Bearer token to a request if the user is authenticated
    private func addAuthHeader(to request: inout URLRequest) {
        if let token = AuthState.shared.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        // Skip ngrok browser warning page for free-tier tunnels
        request.setValue("true", forHTTPHeaderField: "ngrok-skip-browser-warning")
    }

    /// Get or create a conversation for the given agent type.
    /// For group_chat, reuses the most recent existing conversation to avoid
    /// creating a new one every time the sheet is opened.
    func getOrCreateConversation(agentType: AgentType) -> AnyPublisher<UInt, Error> {
        let subject = PassthroughSubject<UInt, Error>()

        // For group_chat, try to reuse an existing conversation first.
        if agentType == .groupChat {
            guard let listURL = URL(string: "\(baseURL)/api/v1/conversations/user/0") else {
                subject.send(completion: .failure(APIError.invalidURL))
                return subject.eraseToAnyPublisher()
            }
            var listRequest = URLRequest(url: listURL)
            listRequest.httpMethod = "GET"
            addAuthHeader(to: &listRequest)

            let task = session.dataTask(with: listRequest) { [weak self] data, response, error in
                guard let self else { return }
                if let data = data,
                   let list = try? JSONSerialization.jsonObject(with: data) as? [[String: Any]],
                   let existing = list.first(where: { ($0["agent_type"] as? String) == AgentType.groupChat.rawValue }),
                   let idNumber = existing["id"] as? NSNumber {
                    subject.send(idNumber.uintValue)
                    subject.send(completion: .finished)
                } else {
                    // No existing conversation — create one.
                    self.createConversation(agentType: agentType, subject: subject)
                }
            }
            task.resume()
        } else {
            createConversation(agentType: agentType, subject: subject)
        }

        return subject.eraseToAnyPublisher()
    }

    private func createConversation(agentType: AgentType, subject: PassthroughSubject<UInt, Error>) {
        guard let url = URL(string: "\(baseURL)/api/v1/conversations") else {
            subject.send(completion: .failure(APIError.invalidURL))
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        addAuthHeader(to: &request)

        let requestBody: [String: Any] = [
            "agent_type": agentType.rawValue,
            "title": "\(agentType.displayName) Conversation"
        ]

        guard let body = try? JSONSerialization.data(withJSONObject: requestBody) else {
            subject.send(completion: .failure(APIError.decodingError))
            return
        }
        request.httpBody = body

        let task = session.dataTask(with: request) { data, response, error in
            if let error = error {
                subject.send(completion: .failure(error))
                return
            }

            if let httpResponse = response as? HTTPURLResponse,
               !(200...299).contains(httpResponse.statusCode) {
                let raw = data.flatMap { String(data: $0, encoding: .utf8) } ?? "nil"
                print("[APIClient] getOrCreateConversation HTTP \(httpResponse.statusCode), raw: \(raw)")
                if let data = data,
                   let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                   let errorMsg = json["error"] as? String {
                    subject.send(completion: .failure(APIError.httpError(httpResponse.statusCode, errorMsg)))
                } else {
                    subject.send(completion: .failure(APIError.httpError(httpResponse.statusCode, nil)))
                }
                return
            }

            guard let data = data,
                  let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let idNumber = json["id"] as? NSNumber else {
                let raw = data.flatMap { String(data: $0, encoding: .utf8) } ?? "nil"
                print("[APIClient] getOrCreateConversation decode failed, raw: \(raw)")
                subject.send(completion: .failure(APIError.decodingError))
                return
            }

            subject.send(idNumber.uintValue)
            subject.send(completion: .finished)
        }
        task.resume()
    }
    
    /// Send chat message with streaming response
    func sendChatMessage(
        conversationId: UInt,
        message: String
    ) -> AnyPublisher<StreamChunk, Error> {
        let subject = PassthroughSubject<StreamChunk, Error>()
        
        // Prepare request
        guard let url = URL(string: "\(baseURL)/api/v1/messages/stream") else {
            subject.send(completion: .failure(APIError.invalidURL))
            return subject.eraseToAnyPublisher()
        }
        
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
        request.timeoutInterval = 300
        addAuthHeader(to: &request)
        
        // Prepare request body
        let requestBody: [String: Any] = [
            "conversation_id": conversationId,
            "content": message
        ]
        
        do {
            request.httpBody = try JSONSerialization.data(withJSONObject: requestBody)
        } catch {
            subject.send(completion: .failure(error))
            return subject.eraseToAnyPublisher()
        }
        
        // Create streaming session with delegate
        let delegate = StreamingDelegate(subject: subject)
        let streamingSession = URLSession(configuration: .default, delegate: delegate, delegateQueue: nil)
        
        let task = streamingSession.dataTask(with: request)
        task.resume()
        
        return subject.eraseToAnyPublisher()
    }

    // MARK: - Group Chat (慧投圆桌)

    /// Streams a multi-agent roundtable response.
    /// - Parameters:
    ///   - conversationId: The `group_chat` conversation ID.
    ///   - message: The user's message.
    ///   - participants: Agent IDs to include. Pass `[]` for all four in default order.
    func sendGroupChatMessage(
        conversationId: UInt,
        message: String,
        participants: [String] = []
    ) -> AnyPublisher<GroupStreamChunk, Error> {
        let subject = PassthroughSubject<GroupStreamChunk, Error>()

        guard let url = URL(string: "\(baseURL)/api/v1/messages/group-chat/stream") else {
            subject.send(completion: .failure(APIError.invalidURL))
            return subject.eraseToAnyPublisher()
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
        request.timeoutInterval = 600
        addAuthHeader(to: &request)

        let body: [String: Any] = [
            "conversation_id": conversationId,
            "content": message,
            "participants": participants
        ]
        do {
            request.httpBody = try JSONSerialization.data(withJSONObject: body)
        } catch {
            subject.send(completion: .failure(error))
            return subject.eraseToAnyPublisher()
        }

        let delegate = GroupStreamingDelegate(subject: subject)
        let streamingSession = URLSession(configuration: .default, delegate: delegate, delegateQueue: nil)
        streamingSession.dataTask(with: request).resume()

        return subject.eraseToAnyPublisher()
    }

    // MARK: - Push Notifications

    /// Registers (or updates) the device's APNs push token with the backend.
    /// Requires the user to be authenticated; silently skips if no JWT is available.
    func registerDeviceToken(_ token: String) async throws {
        guard AuthState.shared.token != nil else { return }

        guard let url = URL(string: "\(baseURL)/api/v1/devices/token") else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        addAuthHeader(to: &request)

        let body: [String: String] = ["token": token, "platform": "ios"]
        request.httpBody = try JSONSerialization.data(withJSONObject: body)

        let (_, response) = try await URLSession.shared.data(for: request)
        if let http = response as? HTTPURLResponse, http.statusCode != 200 {
            throw APIError.httpError(http.statusCode, nil)
        }
    }
}

/// API errors
enum APIError: LocalizedError {
    case invalidURL
    case invalidResponse
    case noData
    case httpError(Int, String?)
    case decodingError
    
    var errorDescription: String? {
        switch self {
        case .invalidURL:
            return "Invalid URL"
        case .invalidResponse:
            return "Invalid response from server"
        case .noData:
            return "No data received"
        case .httpError(let code, let message):
            if let message = message {
                return "HTTP \(code): \(message)"
            }
            return "HTTP error: \(code)"
        case .decodingError:
            return "Failed to decode response"
        }
    }
}
