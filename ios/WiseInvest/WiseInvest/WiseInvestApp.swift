import SwiftUI

@main
struct WiseInvestApp: App {
    // Bridges UIApplicationDelegate for WeChat URL/UniversalLink callbacks
    @UIApplicationDelegateAdaptor(AppDelegate.self) var appDelegate

    @StateObject private var appCoordinator = AppCoordinator()
    @AppStorage("appearance") private var appearance = "dark"
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            appCoordinator.start()
                .preferredColorScheme(appearance == "light" ? .light : .dark)
                .tint(.accentBlue)
                .onChange(of: scenePhase) { _, phase in
                    if phase == .active { AuthState.shared.refreshSession() }
                }
                .onAppear { AuthState.shared.refreshSession() }
                // Also handle URL open via SwiftUI (supplements AppDelegate)
                .onOpenURL { url in
                    WeChatManager.shared.handleOpen(url: url)
                }
        }
    }
}
