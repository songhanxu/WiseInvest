import SwiftUI

/// Main app coordinator managing navigation flow
class AppCoordinator: ObservableObject {
    @Published var currentView: AppView = .home
    
    // Dependencies
    private let apiClient: APIClient
    private let conversationRepository: ConversationRepository
    
    init() {
        // Initialize dependencies
        self.apiClient = APIClient.shared
        self.conversationRepository = ConversationRepositoryImpl(apiClient: apiClient)
    }
    
    @ViewBuilder
    func start() -> some View {
        HomeView(coordinator: self)
    }
    
    func navigateToConversation(agentType: AgentType) {
        currentView = .conversation(agentType)
    }
    
    func navigateToHome() {
        currentView = .home
    }
}

/// App navigation views
enum AppView: Equatable {
    case home
    case conversation(AgentType)
}
