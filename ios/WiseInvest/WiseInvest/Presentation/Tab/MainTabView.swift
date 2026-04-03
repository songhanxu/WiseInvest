import SwiftUI

struct MainTabView: View {
    @ObservedObject var coordinator: AppCoordinator
    @State private var selectedTab: Tab = .home

    enum Tab: Int {
        case home = 0
        case chat = 1
        case profile = 2
    }

    var body: some View {
        TabView(selection: $selectedTab) {
            HomeView(coordinator: coordinator)
                .tabItem {
                    Label("首页", systemImage: selectedTab == .home ? "house.fill" : "house")
                }
                .tag(Tab.home)

            ChatTabView(coordinator: coordinator)
                .tabItem {
                    Label("对话", systemImage: selectedTab == .chat
                          ? "bubble.left.and.bubble.right.fill"
                          : "bubble.left.and.bubble.right")
                }
                .tag(Tab.chat)

            ProfileView()
                .tabItem {
                    Label("我的", systemImage: selectedTab == .profile ? "person.circle.fill" : "person.circle")
                }
                .tag(Tab.profile)
        }
        .tint(.accentBlue)
        .onAppear { configureTabBar() }
    }

    private func configureTabBar() {
        let appearance = UITabBarAppearance()
        appearance.configureWithOpaqueBackground()
        appearance.backgroundColor = UIColor(Color.secondaryBackground)

        appearance.stackedLayoutAppearance.normal.iconColor = UIColor(Color.textTertiary)
        appearance.stackedLayoutAppearance.normal.titleTextAttributes = [
            .foregroundColor: UIColor(Color.textTertiary)
        ]
        appearance.stackedLayoutAppearance.selected.iconColor = UIColor(Color.accentBlue)
        appearance.stackedLayoutAppearance.selected.titleTextAttributes = [
            .foregroundColor: UIColor(Color.accentBlue)
        ]

        appearance.shadowColor = UIColor(white: 1, alpha: 0.08)

        UITabBar.appearance().standardAppearance = appearance
        UITabBar.appearance().scrollEdgeAppearance = appearance
    }
}
