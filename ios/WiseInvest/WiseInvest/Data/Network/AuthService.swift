import Foundation
import Combine

/// Handles all authentication-related API calls
class AuthService {
    static let shared = AuthService()

    private let baseURL: String

    private init() {
        self.baseURL = APIConfig.baseURL
    }

    // MARK: - Account Password Login

    struct AccountLoginResponse: Codable {
        let token: String
        let user: User
    }

    private struct AuthErrorResponse: Decodable {
        let error: String?
    }

    private enum AccountLoginError: LocalizedError {
        case message(String)

        var errorDescription: String? {
            switch self {
            case .message(let text): return text
            }
        }
    }

    func accountLogin(identifier: String, password: String) -> AnyPublisher<AccountLoginResponse, Error> {
        guard let url = URL(string: "\(baseURL)/api/v1/auth/login") else {
            return Fail(error: APIError.invalidURL).eraseToAnyPublisher()
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        // Match the web payload and keep username compatible with older servers.
        request.httpBody = try? JSONSerialization.data(withJSONObject: [
            "identifier": identifier,
            "username": identifier,
            "password": password
        ])

        return URLSession.shared.dataTaskPublisher(for: request)
            .tryMap { data, response in
                guard let http = response as? HTTPURLResponse else {
                    throw AccountLoginError.message("无法连接服务，请检查网络后重试")
                }
                guard http.statusCode == 200 else {
                    let serverMessage = try? JSONDecoder().decode(AuthErrorResponse.self, from: data).error
                    throw AccountLoginError.message(serverMessage ?? "登录失败，请稍后重试")
                }
                return data
            }
            .decode(type: AccountLoginResponse.self, decoder: JSONDecoder())
            .mapError { error in
                if error is URLError {
                    return AccountLoginError.message("无法连接服务，请检查网络后重试") as Error
                }
                if error is DecodingError {
                    return AccountLoginError.message("服务返回了无效的登录结果") as Error
                }
                return error
            }
            .receive(on: DispatchQueue.main)
            .eraseToAnyPublisher()
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

    struct PhoneLoginResponse: Codable {
        let token: String
        let user: User
    }

    func phoneLogin(phone: String, code: String) -> AnyPublisher<PhoneLoginResponse, Error> {
        guard let url = URL(string: "\(baseURL)/api/v1/auth/phone/login") else {
            return Fail(error: APIError.invalidURL).eraseToAnyPublisher()
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try? JSONSerialization.data(withJSONObject: ["phone": phone, "code": code])
        return URLSession.shared.dataTaskPublisher(for: request)
            .tryMap { data, response in
                guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
                    throw APIError.invalidResponse
                }
                return data
            }
            .decode(type: PhoneLoginResponse.self, decoder: JSONDecoder())
            .receive(on: DispatchQueue.main)
            .eraseToAnyPublisher()
    }

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
