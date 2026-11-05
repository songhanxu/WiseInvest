import Foundation
import Combine

/// Key names for persistent storage
private enum StorageKey {
    static let token = "auth_token"
    static let user = "auth_user"
}

/// Global auth state — injected as an EnvironmentObject throughout the app
final class AuthState: ObservableObject {
    @Published private(set) var isAuthenticated: Bool = false
    @Published private(set) var isGuest: Bool = false
    @Published private(set) var currentUser: User?
    @Published private(set) var token: String?

    static let shared = AuthState()
    private var lastRefreshAt: Date?
    private var refreshing = false

    private init() {
        restoreSession()
    }

    // MARK: - Public API

    /// Called after successful WeChat login / phone binding
    func signIn(token: String, user: User) {
        self.token = token
        self.currentUser = user
        self.isAuthenticated = true
        self.isGuest = false
        persistSession(token: token, user: user)
    }

    /// Update user profile and refresh token after phone binding
    func updateSession(token: String, user: User) {
        signIn(token: token, user: user)
    }

    /// Sign out the current user
    func signOut() {
        self.token = nil
        self.currentUser = nil
        self.isAuthenticated = false
        self.isGuest = false
        lastRefreshAt = nil
        clearSession()
    }

    func startTrial() { isGuest = true }
    func showLogin() { isGuest = false }

    func refreshSession() {
        guard isAuthenticated, let token else { return }
        guard Self.tokenIsActive(token) else { signOut(); return }
        guard !refreshing, lastRefreshAt.map({ Date().timeIntervalSince($0) > 86400 }) ?? true else { return }
        guard let url = URL(string: "\(APIConfig.baseURL)/api/v1/auth/refresh") else { return }
        refreshing = true
        lastRefreshAt = Date()
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        URLSession.shared.dataTask(with: request) { [weak self] data, response, _ in
            DispatchQueue.main.async {
                guard let self else { return }
                self.refreshing = false
                guard self.token == token else { return }
                if (response as? HTTPURLResponse)?.statusCode == 401 { self.signOut(); return }
                guard let data, let result = try? JSONDecoder().decode(SessionRefresh.self, from: data) else {
                    self.lastRefreshAt = nil
                    return
                }
                self.signIn(token: result.token, user: result.user)
            }
        }.resume()
    }

    // MARK: - Persistence (UserDefaults — use Keychain in production)

    private func persistSession(token: String, user: User) {
        UserDefaults.standard.set(token, forKey: StorageKey.token)
        if let data = try? JSONEncoder().encode(user) {
            UserDefaults.standard.set(data, forKey: StorageKey.user)
        }
    }

    private func restoreSession() {
        guard let token = UserDefaults.standard.string(forKey: StorageKey.token),
              let data = UserDefaults.standard.data(forKey: StorageKey.user),
              let user = try? JSONDecoder().decode(User.self, from: data) else {
            return
        }
        guard Self.tokenIsActive(token) else { clearSession(); return }
        self.token = token
        self.currentUser = user
        self.isAuthenticated = true
    }

    private func clearSession() {
        UserDefaults.standard.removeObject(forKey: StorageKey.token)
        UserDefaults.standard.removeObject(forKey: StorageKey.user)
    }

    private struct SessionRefresh: Decodable { let token: String; let user: User }

    private static func tokenIsActive(_ token: String) -> Bool {
        let parts = token.split(separator: ".")
        guard parts.count == 3 else { return false }
        var payload = String(parts[1]).replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        payload += String(repeating: "=", count: (4 - payload.count % 4) % 4)
        guard let data = Data(base64Encoded: payload),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let expiration = json["exp"] as? TimeInterval else { return false }
        return expiration > Date().timeIntervalSince1970
    }
}
