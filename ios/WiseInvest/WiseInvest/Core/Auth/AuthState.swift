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
    @Published private(set) var currentUser: User?
    @Published private(set) var token: String?

    static let shared = AuthState()

    private init() {
        restoreSession()
    }

    // MARK: - Public API

    /// Called after successful WeChat login / phone binding
    func signIn(token: String, user: User) {
        self.token = token
        self.currentUser = user
        self.isAuthenticated = true
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
        clearSession()
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
        self.token = token
        self.currentUser = user
        self.isAuthenticated = true
    }

    private func clearSession() {
        UserDefaults.standard.removeObject(forKey: StorageKey.token)
        UserDefaults.standard.removeObject(forKey: StorageKey.user)
    }
}
