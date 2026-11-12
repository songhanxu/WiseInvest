import SwiftUI

struct ProfileView: View {
    @EnvironmentObject var authState: AuthState
    @AppStorage("appearance") private var appearance = "dark"
    @State private var showSignOutConfirmation = false

    var body: some View {
        NavigationView {
            ZStack {
                Color.primaryBackground.ignoresSafeArea()

                ScrollView {
                    VStack(spacing: 20) {
                        if authState.isAuthenticated {
                            avatarSection
                        } else {
                            SignInGate(feature: "个人账户")
                                .padding(.horizontal, 20)
                                .padding(.top, 24)
                        }
                        settingsSection
                        if authState.isAuthenticated { signOutButton }
                    }
                    .padding(.bottom, 32)
                }
            }
            .navigationTitle("个人中心")
            .navigationBarTitleDisplayMode(.large)
        }
    }

    // MARK: - Avatar Section

    private var avatarSection: some View {
        VStack(spacing: 14) {
            // Avatar
            Group {
                if let user = authState.currentUser, !user.effectiveAvatar.isEmpty,
                   let url = URL(string: user.effectiveAvatar) {
                    AsyncImage(url: url) { phase in
                        switch phase {
                        case .success(let image):
                            image.resizable().scaledToFill()
                        default:
                            defaultAvatar
                        }
                    }
                } else {
                    defaultAvatar
                }
            }
            .frame(width: 80, height: 80)
            .clipShape(Circle())

            VStack(spacing: 4) {
                Text(authState.currentUser?.effectiveName ?? "投资者")
                    .font(.system(size: 20, weight: .bold))
                    .foregroundColor(.textPrimary)

                if let phone = authState.currentUser?.phone {
                    Text(phone)
                        .font(.system(size: 14))
                        .foregroundColor(.textSecondary)
                }
            }
        }
        .padding(.top, 24)
    }

    private var defaultAvatar: some View {
        Circle()
            .fill(Color.subtleBackground)
            .overlay(
                Image(systemName: "person.fill")
                    .font(.system(size: 34))
                    .foregroundColor(.textSecondary)
            )
    }

    // MARK: - Settings Section

    private var settingsSection: some View {
        VStack(spacing: 0) {
            Toggle(isOn: Binding(
                get: { appearance == "dark" },
                set: { appearance = $0 ? "dark" : "light" }
            )) {
                Label("深色模式", systemImage: "moon.stars")
                    .font(.system(size: 15))
                    .foregroundColor(.textPrimary)
            }
            .tint(.accentBlue)
            .padding(16)
            .background(Color.secondaryBackground)
            .cornerRadius(16)

            Spacer().frame(height: 16)

            settingsGroup(items: [
                SettingsItem(icon: "person.crop.circle", iconColor: .accentBlue, title: "账号信息"),
                SettingsItem(icon: "bell", iconColor: .textSecondary, title: "消息通知"),
                SettingsItem(icon: "shield.checkerboard", iconColor: .textSecondary, title: "隐私与安全"),
            ])

            Spacer().frame(height: 16)

            settingsGroup(items: [
                SettingsItem(icon: "questionmark.circle", iconColor: .textSecondary, title: "帮助与反馈"),
                SettingsItem(icon: "info.circle", iconColor: .textSecondary, title: "关于慧投"),
            ])
        }
        .padding(.horizontal, 20)
    }

    private func settingsGroup(items: [SettingsItem]) -> some View {
        VStack(spacing: 0) {
            ForEach(Array(items.enumerated()), id: \.element.title) { index, item in
                settingsRow(item: item)

                if index < items.count - 1 {
                    Divider()
                        .background(Color.textTertiary.opacity(0.15))
                        .padding(.leading, 54)
                }
            }
        }
        .background(Color.secondaryBackground)
        .cornerRadius(16)
    }

    private func settingsRow(item: SettingsItem) -> some View {
        Button(action: {}) {
            HStack(spacing: 14) {
                ZStack {
                    RoundedRectangle(cornerRadius: 8)
                        .fill(item.iconColor.opacity(0.15))
                        .frame(width: 34, height: 34)
                    Image(systemName: item.icon)
                        .font(.system(size: 16))
                        .foregroundColor(item.iconColor)
                }

                Text(item.title)
                    .font(.system(size: 15))
                    .foregroundColor(.textPrimary)

                Spacer()

                Image(systemName: "chevron.right")
                    .font(.system(size: 13))
                    .foregroundColor(.textTertiary)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 13)
        }
        .buttonStyle(PlainButtonStyle())
    }

    // MARK: - Sign Out

    private var signOutButton: some View {
        Button(action: { showSignOutConfirmation = true }) {
            Text("退出登录")
                .font(.system(size: 16, weight: .medium))
                .foregroundColor(.red)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 14)
                .background(Color.secondaryBackground)
                .cornerRadius(16)
        }
        .padding(.horizontal, 20)
        .confirmationDialog("确认退出登录？", isPresented: $showSignOutConfirmation) {
            Button("退出登录", role: .destructive) {
                authState.signOut()
            }
            Button("取消", role: .cancel) {}
        }
    }
}

// MARK: - Supporting Views

private struct StatCard: View {
    let title: String
    let value: String
    let icon: String
    let color: Color

    var body: some View {
        VStack(spacing: 8) {
            Image(systemName: icon)
                .font(.system(size: 20))
                .foregroundColor(color)

            Text(value)
                .font(.system(size: 20, weight: .bold))
                .foregroundColor(.textPrimary)

            Text(title)
                .font(.system(size: 11))
                .foregroundColor(.textSecondary)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 16)
        .background(Color.secondaryBackground)
        .cornerRadius(14)
    }
}

private struct SettingsItem {
    let icon: String
    let iconColor: Color
    let title: String
}
