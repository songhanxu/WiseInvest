import SwiftUI
import Combine

struct PhoneBindingView: View {
    @EnvironmentObject var authState: AuthState
    let onComplete: () -> Void

    @StateObject private var viewModel = PhoneBindingViewModel()

    var body: some View {
        NavigationView {
            ZStack {
                Color(hex: "#0A0E1A").ignoresSafeArea()

                ScrollView {
                    VStack(spacing: 32) {
                        // Header
                        VStack(spacing: 12) {
                            ZStack {
                                Circle()
                                    .fill(Color(hex: "#1E3A5F"))
                                    .frame(width: 72, height: 72)
                                Image(systemName: "phone.fill")
                                    .font(.system(size: 32))
                                    .foregroundColor(Color(hex: "#3B82F6"))
                            }
                            Text("绑定手机号")
                                .font(.system(size: 24, weight: .bold))
                                .foregroundColor(.white)
                            Text("绑定手机号后可用于账号安全验证")
                                .font(.system(size: 14))
                                .foregroundColor(Color(hex: "#64748B"))
                                .multilineTextAlignment(.center)
                        }
                        .padding(.top, 32)

                        // Form
                        VStack(spacing: 16) {
                            // Phone input
                            VStack(alignment: .leading, spacing: 8) {
                                Text("手机号")
                                    .font(.system(size: 13, weight: .medium))
                                    .foregroundColor(Color(hex: "#94A3B8"))

                                HStack(spacing: 12) {
                                    Text("+86")
                                        .font(.system(size: 16, weight: .medium))
                                        .foregroundColor(Color(hex: "#94A3B8"))
                                        .padding(.horizontal, 12)
                                        .padding(.vertical, 14)
                                        .background(Color(hex: "#1E293B"))
                                        .cornerRadius(12)

                                    TextField("请输入手机号", text: $viewModel.phone)
                                        .keyboardType(.phonePad)
                                        .font(.system(size: 16))
                                        .foregroundColor(.white)
                                        .padding(.horizontal, 16)
                                        .padding(.vertical, 14)
                                        .background(Color(hex: "#1E293B"))
                                        .cornerRadius(12)
                                }
                            }

                            // Code input
                            if viewModel.codeSent {
                                VStack(alignment: .leading, spacing: 8) {
                                    Text("验证码")
                                        .font(.system(size: 13, weight: .medium))
                                        .foregroundColor(Color(hex: "#94A3B8"))

                                    HStack(spacing: 12) {
                                        TextField("请输入验证码", text: $viewModel.code)
                                            .keyboardType(.numberPad)
                                            .font(.system(size: 16))
                                            .foregroundColor(.white)
                                            .padding(.horizontal, 16)
                                            .padding(.vertical, 14)
                                            .background(Color(hex: "#1E293B"))
                                            .cornerRadius(12)

                                        Button(action: { viewModel.sendCode() }) {
                                            Text(viewModel.countdown > 0 ? "\(viewModel.countdown)s" : "重新发送")
                                                .font(.system(size: 14, weight: .medium))
                                                .foregroundColor(viewModel.countdown > 0 ? Color(hex: "#475569") : Color(hex: "#3B82F6"))
                                                .frame(width: 88)
                                                .padding(.vertical, 14)
                                                .background(Color(hex: "#1E293B"))
                                                .cornerRadius(12)
                                        }
                                        .disabled(viewModel.countdown > 0)
                                    }
                                }
                                .transition(.move(edge: .bottom).combined(with: .opacity))
                            }

                            // Send code / bind button
                            Button(action: {
                                if viewModel.codeSent {
                                    viewModel.bindPhone(authState: authState, onComplete: onComplete)
                                } else {
                                    viewModel.sendCode()
                                }
                            }) {
                                HStack {
                                    if viewModel.isLoading {
                                        ProgressView()
                                            .progressViewStyle(CircularProgressViewStyle(tint: .white))
                                    } else {
                                        Text(viewModel.codeSent ? "确认绑定" : "发送验证码")
                                            .font(.system(size: 17, weight: .semibold))
                                            .foregroundColor(.white)
                                    }
                                }
                                .frame(maxWidth: .infinity)
                                .frame(height: 54)
                                .background(
                                    LinearGradient(
                                        colors: [Color(hex: "#3B82F6"), Color(hex: "#1D4ED8")],
                                        startPoint: .leading,
                                        endPoint: .trailing
                                    )
                                )
                                .cornerRadius(16)
                            }
                            .disabled(viewModel.isLoading || viewModel.phone.count < 11 || (viewModel.codeSent && viewModel.code.count < 6))
                            .opacity((viewModel.phone.count < 11 || (viewModel.codeSent && viewModel.code.count < 6)) ? 0.6 : 1)
                            .buttonStyle(ScaleButtonStyle())
                        }
                        .padding(.horizontal, 24)

                        // Skip button
                        Button(action: onComplete) {
                            Text("暂时跳过")
                                .font(.system(size: 14))
                                .foregroundColor(Color(hex: "#475569"))
                        }
                    }
                }
            }
            .navigationBarHidden(true)
        }
        .alert("提示", isPresented: $viewModel.showError) {
            Button("确定", role: .cancel) {}
        } message: {
            Text(viewModel.errorMessage)
        }
        .animation(.easeInOut, value: viewModel.codeSent)
    }
}

// MARK: - PhoneBindingViewModel

final class PhoneBindingViewModel: ObservableObject {
    @Published var phone = ""
    @Published var code = ""
    @Published var codeSent = false
    @Published var countdown = 0
    @Published var isLoading = false
    @Published var showError = false
    @Published var errorMessage = ""

    private var cancellables = Set<AnyCancellable>()
    private var countdownTimer: AnyCancellable?

    func sendCode() {
        guard phone.count >= 11 else { return }
        isLoading = true

        AuthService.shared.sendPhoneCode(phone: phone)
            .sink(receiveCompletion: { [weak self] completion in
                DispatchQueue.main.async {
                    self?.isLoading = false
                    if case .failure(let error) = completion {
                        self?.errorMessage = error.localizedDescription
                        self?.showError = true
                    }
                }
            }, receiveValue: { [weak self] in
                DispatchQueue.main.async {
                    self?.codeSent = true
                    self?.startCountdown()
                }
            })
            .store(in: &cancellables)
    }

    func bindPhone(authState: AuthState, onComplete: @escaping () -> Void) {
        guard code.count >= 6 else { return }
        isLoading = true

        AuthService.shared.bindPhone(phone: phone, code: code)
            .sink(receiveCompletion: { [weak self] completion in
                DispatchQueue.main.async {
                    self?.isLoading = false
                    if case .failure(let error) = completion {
                        self?.errorMessage = error.localizedDescription
                        self?.showError = true
                    }
                }
            }, receiveValue: { response in
                DispatchQueue.main.async {
                    authState.updateSession(token: response.token, user: response.user)
                    onComplete()
                }
            })
            .store(in: &cancellables)
    }

    private func startCountdown() {
        countdown = 60
        countdownTimer?.cancel()
        countdownTimer = Timer.publish(every: 1, on: .main, in: .common)
            .autoconnect()
            .sink { [weak self] _ in
                guard let self = self else { return }
                if self.countdown > 0 {
                    self.countdown -= 1
                } else {
                    self.countdownTimer?.cancel()
                }
            }
    }
}
