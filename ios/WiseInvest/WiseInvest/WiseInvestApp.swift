import SwiftUI

@main
struct WiseInvestApp: App {
    @StateObject private var appCoordinator = AppCoordinator()
    
    var body: some Scene {
        WindowGroup {
            appCoordinator.start()
                .preferredColorScheme(.dark)
        }
    }
}
