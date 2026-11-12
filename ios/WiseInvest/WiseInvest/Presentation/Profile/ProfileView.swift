import SwiftUI

struct ProfileView: View {
    @EnvironmentObject var authState: AuthState
    @AppStorage("appearance") private var appearance = "dark"
    @State private var showSignOutConfirmation = false
    @State private var presentedInfo: ProfileInfo?

    private enum ProfileInfo: String, Identifiable {
        case account, data, about
        var id: String { rawValue }
        var title: String {
            switch self {
            case .account: return "账号信息"
            case .data: return "数据与同步"
            case .about: return "关于慧投"
            }
        }
    }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Button {
                        if authState.isAuthenticated { presentedInfo = .account }
                        else { authState.showLogin() }
                    } label: {
                        HStack(spacing: 14) {
                            accountAvatar
                            VStack(alignment: .leading, spacing: 5) {
                                Text(authState.isAuthenticated ? authState.currentUser?.effectiveName ?? "我的账户" : "登录慧投")
                                    .font(.body.weight(.semibold))
                                    .foregroundStyle(Color.textPrimary)
                                Text(authState.isAuthenticated ? "查看账号信息" : "同步自选与研究记录")
                                    .font(.subheadline)
                                    .foregroundStyle(Color.textSecondary)
                            }
                            Spacer(minLength: 8)
                            Image(systemName: "chevron.right")
                                .font(.caption.weight(.semibold))
                                .foregroundStyle(Color.textTertiary)
                        }
                        .padding(.vertical, 8)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("profile.account")
                }
                .listRowBackground(Color.secondaryBackground)

                Section("外观") {
                    Toggle(isOn: Binding(
                        get: { appearance == "dark" },
                        set: { appearance = $0 ? "dark" : "light" }
                    )) {
                        settingsLabel("深色模式", icon: "moon")
                    }
                    .tint(.accentBlue)
                    .accessibilityIdentifier("profile.appearance")
                }
                .listRowBackground(Color.secondaryBackground)

                Section("通用") {
                    Button { presentedInfo = .data } label: {
                        settingsRow("数据与同步", icon: "arrow.triangle.2.circlepath")
                    }
                    Button { presentedInfo = .about } label: {
                        settingsRow("关于慧投", icon: "info.circle")
                    }
                }
                .listRowBackground(Color.secondaryBackground)

                if authState.isAuthenticated {
                    Section {
                        Button("退出登录", role: .destructive) { showSignOutConfirmation = true }
                            .frame(maxWidth: .infinity, minHeight: 30)
                    }
                    .listRowBackground(Color.secondaryBackground)
                }
            }
            .listStyle(.insetGrouped)
            .listSectionSpacing(24)
            .scrollContentBackground(.hidden)
            .background(Color.primaryBackground)
            .navigationTitle("我的")
            .navigationBarTitleDisplayMode(.large)
        }
        .confirmationDialog("确认退出登录？", isPresented: $showSignOutConfirmation) {
            Button("退出登录", role: .destructive) { authState.signOut() }
            Button("取消", role: .cancel) {}
        }
        .sheet(item: $presentedInfo) { info in
            NavigationStack {
                List {
                    switch info {
                    case .account:
                        if let user = authState.currentUser {
                            Section {
                                LabeledContent("昵称", value: user.effectiveName)
                                LabeledContent("用户 ID", value: String(user.id))
                                if let phone = user.phone, !phone.isEmpty {
                                    LabeledContent("手机号", value: phone)
                                }
                            }
                        }
                    case .data:
                        Section("账户数据") {
                            Text("登录后可访问你的自选、账户持仓和价格提醒。")
                        }
                        Section("当前设备") {
                            Text("对话记录保存在当前设备。登录状态和外观选择会在下次打开时恢复。")
                        }
                    case .about:
                        Section {
                            LabeledContent("应用", value: "慧投 WiseInvest")
                            LabeledContent("版本", value: Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "1.0")
                        }
                        Section {
                            Text("关注市场，理解公司，记录你的研究。")
                        }
                    }
                }
                .navigationTitle(info.title)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar { ToolbarItem(placement: .confirmationAction) { Button("完成") { presentedInfo = nil } } }
            }
            .tint(.accentBlue)
        }
    }

    private var accountAvatar: some View {
        Group {
            if authState.isAuthenticated, let value = authState.currentUser?.effectiveAvatar,
               !value.isEmpty, let url = URL(string: value) {
                AsyncImage(url: url) { image in image.resizable().scaledToFill() }
                    placeholder: { avatarPlaceholder }
            } else { avatarPlaceholder }
        }
        .frame(width: 48, height: 48)
        .clipShape(Circle())
    }

    private var avatarPlaceholder: some View {
        Image(systemName: "person.crop.circle.fill")
            .resizable().scaledToFit()
            .foregroundStyle(Color.textTertiary.opacity(0.6))
    }

    private func settingsLabel(_ title: String, icon: String) -> some View {
        HStack(spacing: 12) {
            Image(systemName: icon)
                .font(.system(size: 18, weight: .regular))
                .foregroundStyle(Color.textSecondary)
                .frame(width: 24)
            Text(title).foregroundStyle(Color.textPrimary)
        }
        .padding(.vertical, 3)
    }

    private func settingsRow(_ title: String, icon: String) -> some View {
        HStack {
            settingsLabel(title, icon: icon)
            Spacer()
            Image(systemName: "chevron.right")
                .font(.caption.weight(.semibold))
                .foregroundStyle(Color.textTertiary)
        }
        .contentShape(Rectangle())
    }
}
