import SwiftUI
import Combine

/// Main app coordinator managing navigation flow
class AppCoordinator: ObservableObject {
    @Published var currentView: AppView = .home

    private let apiClient: APIClient
    private let conversationRepository: ConversationRepository
    private let authState: AuthState
    private var cancellables = Set<AnyCancellable>()

    init() {
        self.apiClient = APIClient.shared
        self.conversationRepository = ConversationRepositoryImpl(apiClient: apiClient)
        self.authState = AuthState.shared
    }

    @ViewBuilder
    func start() -> some View {
        RootView(coordinator: self)
            .environmentObject(AuthState.shared)
    }

    func navigateToConversation(agentType: AgentType) {
        currentView = .conversation(agentType)
    }

    func navigateToHome() {
        currentView = .home
    }
}

/// Switches between LoginView and HomeView based on auth state
private struct RootView: View {
    @ObservedObject var coordinator: AppCoordinator
    @EnvironmentObject var authState: AuthState

    var body: some View {
        Group {
            if authState.isAuthenticated {
                HomeView(coordinator: coordinator)
            } else {
                LoginView()
            }
        }
        .animation(.easeInOut(duration: 0.3), value: authState.isAuthenticated)
    }
}

/// App navigation views
enum AppView: Equatable {
    case home
    case conversation(AgentType)
}
