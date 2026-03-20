import Foundation
import Combine

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

    /// Create or get conversation ID
    func getOrCreateConversation(agentType: AgentType) -> AnyPublisher<UInt, Error> {
        let subject = PassthroughSubject<UInt, Error>()

        guard let url = URL(string: "\(baseURL)/api/v1/conversations") else {
            subject.send(completion: .failure(APIError.invalidURL))
            return subject.eraseToAnyPublisher()
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        addAuthHeader(to: &request)

        let requestBody: [String: Any] = [
            "agent_type": agentType.rawValue,
            "title": "\(agentType.displayName) Conversation"
        ]
        
        do {
            request.httpBody = try JSONSerialization.data(withJSONObject: requestBody)
        } catch {
            subject.send(completion: .failure(error))
            return subject.eraseToAnyPublisher()
        }
        
        let task = session.dataTask(with: request) { data, response, error in
            if let error = error {
                subject.send(completion: .failure(error))
                return
            }
            
            // Check HTTP status code first
            if let httpResponse = response as? HTTPURLResponse,
               !(200...299).contains(httpResponse.statusCode) {
                let raw = data.flatMap { String(data: $0, encoding: .utf8) } ?? "nil"
                print("[APIClient] getOrCreateConversation HTTP \(httpResponse.statusCode), raw: \(raw)")
                // Try to extract server error message
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
                  let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
                subject.send(completion: .failure(APIError.decodingError))
                return
            }

            // JSONSerialization 返回 NSNumber，兼容 Int / UInt 两种情况
            guard let idNumber = json["id"] as? NSNumber else {
                // 打印原始响应方便调试
                let raw = String(data: data, encoding: .utf8) ?? "nil"
                print("[APIClient] getOrCreateConversation decode failed, raw: \(raw)")
                subject.send(completion: .failure(APIError.decodingError))
                return
            }
            let id = idNumber.uintValue
            
            subject.send(id)
            subject.send(completion: .finished)
        }
        
        task.resume()
        return subject.eraseToAnyPublisher()
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
