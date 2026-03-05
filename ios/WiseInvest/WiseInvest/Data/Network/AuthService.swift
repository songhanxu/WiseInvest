import Foundation
import Combine

/// Handles all authentication-related API calls
class AuthService {
    static let shared = AuthService()

    private let baseURL: String

    private init() {
        self.baseURL = APIConfig.baseURL
    }

    // MARK: - WeChat Login

    struct WeChatLoginResponse: Codable {
        let token: String
        let needsPhoneBinding: Bool
        let user: User

        enum CodingKeys: String, CodingKey {
            case token
            case needsPhoneBinding = "needs_phone_binding"
            case user
        }
    }

    func wechatLogin(code: String) -> AnyPublisher<WeChatLoginResponse, Error> {
        guard let url = URL(string: "\(baseURL)/api/v1/auth/wechat/login") else {
            return Fail(error: APIError.invalidURL).eraseToAnyPublisher()
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try? JSONSerialization.data(withJSONObject: ["code": code])

        return URLSession.shared.dataTaskPublisher(for: request)
            .tryMap { data, response in
                guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
                    throw APIError.invalidResponse
                }
                return data
            }
            .decode(type: WeChatLoginResponse.self, decoder: JSONDecoder())
            .receive(on: DispatchQueue.main)
            .eraseToAnyPublisher()
    }

    // MARK: - SMS Code

    func sendPhoneCode(phone: String) -> AnyPublisher<Void, Error> {
        guard let url = URL(string: "\(baseURL)/api/v1/auth/phone/send-code") else {
            return Fail(error: APIError.invalidURL).eraseToAnyPublisher()
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try? JSONSerialization.data(withJSONObject: ["phone": phone])
        if let token = AuthState.shared.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        return URLSession.shared.dataTaskPublisher(for: request)
            .tryMap { _, response in
                guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
                    throw APIError.invalidResponse
                }
            }
            .receive(on: DispatchQueue.main)
            .eraseToAnyPublisher()
    }

    // MARK: - Bind Phone

    struct BindPhoneResponse: Codable {
        let token: String
        let user: User
    }

    func bindPhone(phone: String, code: String) -> AnyPublisher<BindPhoneResponse, Error> {
        guard let url = URL(string: "\(baseURL)/api/v1/auth/phone/bind") else {
            return Fail(error: APIError.invalidURL).eraseToAnyPublisher()
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try? JSONSerialization.data(withJSONObject: ["phone": phone, "code": code])
        if let token = AuthState.shared.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        return URLSession.shared.dataTaskPublisher(for: request)
            .tryMap { data, response in
                guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
                    throw APIError.invalidResponse
                }
                return data
            }
            .decode(type: BindPhoneResponse.self, decoder: JSONDecoder())
            .receive(on: DispatchQueue.main)
            .eraseToAnyPublisher()
    }

    // MARK: - Get Profile

    func getMe() -> AnyPublisher<User, Error> {
        guard let url = URL(string: "\(baseURL)/api/v1/auth/me") else {
            return Fail(error: APIError.invalidURL).eraseToAnyPublisher()
        }
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        if let token = AuthState.shared.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        return URLSession.shared.dataTaskPublisher(for: request)
            .tryMap { data, response in
                guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
                    throw APIError.invalidResponse
                }
                return data
            }
            .decode(type: User.self, decoder: JSONDecoder())
            .receive(on: DispatchQueue.main)
            .eraseToAnyPublisher()
    }
}
