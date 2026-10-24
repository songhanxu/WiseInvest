import SwiftUI

struct MainTabView: View {
    @ObservedObject var coordinator: AppCoordinator
    @State private var selectedTab: Tab = .home

    enum Tab: Int {
        case home = 0
        case investment = 1
        case chat = 2
        case profile = 3
    }

    var body: some View {
        TabView(selection: $selectedTab) {
            HomeView(coordinator: coordinator)
                .tabItem {
                    Label("首页", systemImage: selectedTab == .home ? "house.fill" : "house")
                }
                .tag(Tab.home)

            InvestmentView()
                .tabItem {
                    Label("投资", systemImage: selectedTab == .investment ? "chart.bar.fill" : "chart.bar")
                }
                .tag(Tab.investment)

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
