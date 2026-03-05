import SwiftUI

@main
struct WiseInvestApp: App {
    // Bridges UIApplicationDelegate for WeChat URL/UniversalLink callbacks
    @UIApplicationDelegateAdaptor(AppDelegate.self) var appDelegate

    @StateObject private var appCoordinator = AppCoordinator()

    var body: some Scene {
        WindowGroup {
            appCoordinator.start()
                .preferredColorScheme(.dark)
                // Also handle URL open via SwiftUI (supplements AppDelegate)
                .onOpenURL { url in
                    WeChatManager.shared.handleOpen(url: url)
                }
        }
    }
}
