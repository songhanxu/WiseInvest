import SwiftUI
import Combine

struct LoginView: View {
    @EnvironmentObject private var authState: AuthState
    @AppStorage("appearance") private var appearance = "dark"
    @StateObject private var viewModel = LoginViewModel()
    @State private var showsPassword = false
    @FocusState private var focusedField: Field?

    private enum Field { case identifier, password }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                HStack {
                    Text("慧投")
                        .font(.title3.weight(.semibold))
                        .foregroundStyle(Color.textPrimary)
                    Spacer()
                    Button {
                        appearance = appearance == "dark" ? "light" : "dark"
                    } label: {
                        Image(systemName: appearance == "dark" ? "sun.max" : "moon")
                            .font(.system(size: 19))
                            .foregroundStyle(Color.textSecondary)
                            .frame(width: 44, height: 44)
                    }
                    .accessibilityLabel(appearance == "dark" ? "切换到浅色模式" : "切换到深色模式")
                }

                VStack(alignment: .leading, spacing: 10) {
                    Text("登录")
                        .font(.largeTitle.bold())
                        .foregroundStyle(Color.textPrimary)
                    Text("同步你的自选、持仓与研究记录。")
                        .font(.subheadline)
                        .foregroundStyle(Color.textSecondary)
                }
                .padding(.top, 44)
                .padding(.bottom, 32)

                VStack(spacing: 0) {
                    TextField("账号、邮箱或手机号", text: $viewModel.identifier)
                        .textContentType(.username)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.emailAddress)
                        .submitLabel(.next)
                        .focused($focusedField, equals: .identifier)
                        .onSubmit { focusedField = .password }
                        .padding(.horizontal, 18)
                        .frame(minHeight: 58)
                        .accessibilityIdentifier("login.identifier")
                    Divider().padding(.leading, 18)
                    HStack(spacing: 0) {
                        Group {
                            if showsPassword {
                                TextField("密码", text: $viewModel.password)
                            } else {
                                SecureField("密码", text: $viewModel.password)
                            }
                        }
                        .textContentType(.password)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .submitLabel(.go)
                        .focused($focusedField, equals: .password)
                        .onSubmit(submit)
                        .padding(.leading, 18)
                        .frame(minHeight: 58)
                        .accessibilityIdentifier("login.password")
                        Button {
                            showsPassword.toggle()
                            focusedField = .password
                        } label: {
                            Image(systemName: showsPassword ? "eye.slash" : "eye")
                                .font(.system(size: 16))
                                .foregroundStyle(Color.textTertiary)
                                .frame(width: 52, height: 58)
                        }
                        .accessibilityLabel(showsPassword ? "隐藏密码" : "显示密码")
                    }
                }
                .font(.body)
                .foregroundStyle(Color.textPrimary)
                .background(Color.secondaryBackground, in: RoundedRectangle(cornerRadius: 14))
                .disabled(viewModel.isLoading)

                if !viewModel.errorMessage.isEmpty {
                    Text(viewModel.errorMessage)
                        .font(.footnote)
                        .foregroundStyle(.red)
                        .padding(.top, 12)
                        .accessibilityIdentifier("login.error")
                }

                Button(action: submit) {
                    HStack(spacing: 10) {
                        if viewModel.isLoading { ProgressView().tint(.white) }
                        Text(viewModel.isLoading ? "正在登录" : "登录")
                            .font(.body.weight(.semibold))
                    }
                    .frame(maxWidth: .infinity, minHeight: 52)
                    .foregroundStyle(viewModel.canSubmit || viewModel.isLoading ? Color.white : Color.textTertiary)
                    .background(viewModel.canSubmit || viewModel.isLoading ? Color.accentBlue : Color.subtleBackground, in: RoundedRectangle(cornerRadius: 14))
                }
                .disabled(!viewModel.canSubmit)
                .padding(.top, 24)
                .accessibilityIdentifier("login.submit")

                Button("先试用") { authState.startTrial() }
                    .font(.body)
                    .foregroundStyle(Color.accentBlue)
                    .frame(maxWidth: .infinity, minHeight: 48)
                    .padding(.top, 12)
                    .accessibilityIdentifier("login.trial")
            }
            .frame(maxWidth: 420)
            .padding(.horizontal, 28)
            .padding(.top, 12)
            .padding(.bottom, 32)
            .frame(maxWidth: .infinity)
        }
        .scrollDismissesKeyboard(.interactively)
        .background(Color.primaryBackground.ignoresSafeArea())
    }

    private func submit() {
        guard viewModel.canSubmit else { return }
        focusedField = nil
        viewModel.login(authState: authState)
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
