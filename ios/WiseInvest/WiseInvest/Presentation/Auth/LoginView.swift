import SwiftUI
import Combine

struct LoginView: View {
    @EnvironmentObject private var authState: AuthState
    @AppStorage("appearance") private var appearance = "dark"
    @StateObject private var viewModel = LoginViewModel()
    @State private var showsPassword = false
    @FocusState private var focusedField: Field?

    private enum Field {
        case identifier
        case password
    }

    var body: some View {
        GeometryReader { geometry in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    header
                    Spacer(minLength: 64)
                    Text("WISEINVEST ACCOUNT")
                        .font(.system(size: 11, weight: .semibold, design: .rounded))
                        .tracking(2.2)
                        .foregroundStyle(Color.accentBlue)
                    Text("欢迎回来")
                        .font(.system(size: 36, weight: .bold))
                        .foregroundStyle(Color.textPrimary)
                        .padding(.top, 12)
                    Text("把市场信息，整理成自己的判断。")
                        .font(.system(size: 15))
                        .foregroundStyle(Color.textSecondary)
                        .padding(.top, 8)
                    loginCard.padding(.top, 32)
                    Text("账户用于同步你的自选与研究记录")
                        .font(.system(size: 12))
                        .foregroundStyle(Color.textTertiary)
                        .frame(maxWidth: .infinity)
                        .padding(.top, 24)
                    Spacer(minLength: 48)
                }
                .frame(maxWidth: 430)
                .frame(minHeight: geometry.size.height, alignment: .top)
                .padding(.horizontal, 24)
                .frame(maxWidth: .infinity)
            }
            .scrollDismissesKeyboard(.interactively)
        }
        .background(Color.primaryBackground.ignoresSafeArea())
    }

    private var header: some View {
        HStack(spacing: 11) {
            Image(systemName: "chart.bar.xaxis")
                .font(.system(size: 18, weight: .semibold))
                .foregroundStyle(.white)
                .frame(width: 38, height: 38)
                .background(Color.accentBlue, in: RoundedRectangle(cornerRadius: 11))
            Text("慧投")
                .font(.system(size: 19, weight: .semibold))
                .foregroundStyle(Color.textPrimary)
            Text("WiseInvest")
                .font(.system(size: 12, weight: .medium))
                .foregroundStyle(Color.textTertiary)
            Spacer()
            Button {
                appearance = appearance == "dark" ? "light" : "dark"
            } label: {
                Image(systemName: appearance == "dark" ? "sun.max" : "moon")
                    .font(.system(size: 17, weight: .medium))
                    .foregroundStyle(Color.textSecondary)
                    .frame(width: 40, height: 40)
                    .background(Color.secondaryBackground, in: Circle())
            }
            .accessibilityLabel(appearance == "dark" ? "切换到浅色模式" : "切换到深色模式")
        }
        .padding(.top, 20)
    }

    private var loginCard: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text("登录账户")
                .font(.system(size: 21, weight: .semibold))
                .foregroundStyle(Color.textPrimary)
            Text("使用与网页端相同的账号和密码")
                .font(.system(size: 13))
                .foregroundStyle(Color.textSecondary)
                .padding(.top, 6)

            Text("账号 / 邮箱 / 手机号")
                .font(.system(size: 13, weight: .medium))
                .foregroundStyle(Color.textPrimary)
                .padding(.top, 28)
            HStack(spacing: 12) {
                Image(systemName: "person")
                    .frame(width: 18)
                    .foregroundStyle(Color.textTertiary)
                TextField("输入账号名、邮箱或手机号", text: $viewModel.identifier)
                    .textContentType(.username)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .keyboardType(.emailAddress)
                    .submitLabel(.next)
                    .focused($focusedField, equals: .identifier)
                    .onSubmit { focusedField = .password }
                    .foregroundStyle(Color.textPrimary)
            }
            .modifier(LoginFieldStyle())
            .padding(.top, 10)

            Text("密码")
                .font(.system(size: 13, weight: .medium))
                .foregroundStyle(Color.textPrimary)
                .padding(.top, 20)
            HStack(spacing: 12) {
                Image(systemName: "lock")
                    .frame(width: 18)
                    .foregroundStyle(Color.textTertiary)
                Group {
                    if showsPassword {
                        TextField("输入密码", text: $viewModel.password)
                    } else {
                        SecureField("输入密码", text: $viewModel.password)
                    }
                }
                .textContentType(.password)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .submitLabel(.go)
                .focused($focusedField, equals: .password)
                .onSubmit(submit)
                .foregroundStyle(Color.textPrimary)
                Button {
                    showsPassword.toggle()
                } label: {
                    Image(systemName: showsPassword ? "eye.slash" : "eye")
                        .font(.system(size: 16))
                        .foregroundStyle(Color.textSecondary)
                }
                .accessibilityLabel(showsPassword ? "隐藏密码" : "显示密码")
            }
            .modifier(LoginFieldStyle())
            .padding(.top, 10)

            if !viewModel.errorMessage.isEmpty {
                Text(viewModel.errorMessage)
                    .font(.system(size: 13))
                    .foregroundStyle(.red)
                    .padding(.top, 14)
            }
            Button(action: submit) {
                HStack(spacing: 8) {
                    Spacer()
                    if viewModel.isLoading { ProgressView().tint(.white) }
                    Text(viewModel.isLoading ? "正在登录" : "登录账户")
                        .font(.system(size: 16, weight: .semibold))
                    if !viewModel.isLoading {
                        Image(systemName: "arrow.right")
                            .font(.system(size: 15, weight: .semibold))
                    }
                    Spacer()
                }
                .foregroundStyle(.white)
                .frame(height: 52)
                .background(Color.accentBlue, in: RoundedRectangle(cornerRadius: 13))
            }
            .disabled(!viewModel.canSubmit)
            .opacity(viewModel.canSubmit ? 1 : 0.65)
            .padding(.top, 26)
        }
        .padding(24)
        .background(Color.secondaryBackground, in: RoundedRectangle(cornerRadius: 22))
        .overlay {
            RoundedRectangle(cornerRadius: 22)
                .stroke(Color.textTertiary.opacity(0.13), lineWidth: 1)
        }
    }

    private func submit() {
        focusedField = nil
        viewModel.login(authState: authState)
    }
}

private struct LoginFieldStyle: ViewModifier {
    func body(content: Content) -> some View {
        content
            .font(.system(size: 15))
            .frame(height: 52)
            .padding(.horizontal, 15)
            .background(Color.subtleBackground, in: RoundedRectangle(cornerRadius: 12))
            .overlay {
                RoundedRectangle(cornerRadius: 12)
                    .stroke(Color.textTertiary.opacity(0.16), lineWidth: 1)
            }
    }
}

final class LoginViewModel: ObservableObject {
    @Published var identifier = ""
    @Published var password = ""
    @Published var isLoading = false
    @Published var errorMessage = ""

    private var cancellables = Set<AnyCancellable>()

    var canSubmit: Bool {
        !isLoading && !identifier.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !password.isEmpty
    }

    func login(authState: AuthState) {
        guard canSubmit else { return }
        isLoading = true
        errorMessage = ""
        let account = identifier.trimmingCharacters(in: .whitespacesAndNewlines)
        AuthService.shared.accountLogin(identifier: account, password: password)
            .sink(receiveCompletion: { [weak self] completion in
                self?.isLoading = false
                if case .failure(let error) = completion {
                    self?.errorMessage = error.localizedDescription
                }
            }, receiveValue: { [weak self] response in
                self?.password = ""
                authState.signIn(token: response.token, user: response.user)
            })
            .store(in: &cancellables)
    }
}
