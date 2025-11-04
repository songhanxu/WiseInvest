import Foundation
import Combine

/// Streaming delegate for SSE
private class StreamingDelegate: NSObject, URLSessionDataDelegate {
    let subject: PassthroughSubject<String, Error>
    var buffer = ""
    
    init(subject: PassthroughSubject<String, Error>) {
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
                   let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                   let content = json["content"] as? String {
                    subject.send(content)
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
        // Use localhost for simulator, or your Mac's IP for physical device
        self.baseURL = "http://localhost:8080"
        
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 60
        config.timeoutIntervalForResource = 300
        self.session = URLSession(configuration: config)
    }
    
    /// Create or get conversation ID
    func getOrCreateConversation(agentType: AgentType) -> AnyPublisher<UInt, Error> {
        let subject = PassthroughSubject<UInt, Error>()
        
        // For now, use a fixed user ID (in production, this should come from auth)
        let userId: UInt = 1
        
        guard let url = URL(string: "\(baseURL)/api/v1/conversations") else {
            subject.send(completion: .failure(APIError.invalidURL))
            return subject.eraseToAnyPublisher()
        }
        
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        
        let requestBody: [String: Any] = [
            "user_id": userId,
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
            
            guard let data = data,
                  let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let id = json["id"] as? UInt else {
                subject.send(completion: .failure(APIError.decodingError))
                return
            }
            
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
    ) -> AnyPublisher<String, Error> {
        let subject = PassthroughSubject<String, Error>()
        
        // Prepare request
        guard let url = URL(string: "\(baseURL)/api/v1/messages/stream") else {
            subject.send(completion: .failure(APIError.invalidURL))
            return subject.eraseToAnyPublisher()
        }
        
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
        request.timeoutInterval = 300 // 5 minutes for streaming
        
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
}

/// API errors
enum APIError: LocalizedError {
    case invalidURL
    case invalidResponse
    case noData
    case httpError(Int)
    case decodingError
    
    var errorDescription: String? {
        switch self {
        case .invalidURL:
            return "Invalid URL"
        case .invalidResponse:
            return "Invalid response from server"
        case .noData:
            return "No data received"
        case .httpError(let code):
            return "HTTP error: \(code)"
        case .decodingError:
            return "Failed to decode response"
        }
    }
}
