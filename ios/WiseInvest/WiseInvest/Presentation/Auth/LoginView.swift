import SwiftUI
import Combine

struct LoginView: View {
    @EnvironmentObject var authState: AuthState
    @StateObject private var viewModel = LoginViewModel()

    var body: some View {
        ZStack {
            // 全屏背景图（底部用渐变遮住 AI 水印）
            GeometryReader { geo in
                Image("Images/Login/Background")
                    .resizable()
                    .scaledToFill()
                    .frame(width: geo.size.width, height: geo.size.height)
                    .clipped()
                    // 底部渐变遮罩，既隐藏水印又让登录卡片区域更易读
                    .overlay(
                        LinearGradient(
                            stops: [
                                .init(color: .clear,                       location: 0.0),
                                .init(color: .clear,                       location: 0.55),
                                .init(color: Color(hex: "#0A0E1A").opacity(0.85), location: 0.78),
                                .init(color: Color(hex: "#0A0E1A"),        location: 1.0),
                            ],
                            startPoint: .top,
                            endPoint: .bottom
                        )
                    )
            }
            .ignoresSafeArea()

            VStack(spacing: 0) {
                Spacer()

                // Logo & tagline
                logoSection

                Spacer()

                // Login card
                loginCard

                Spacer().frame(height: 48)
            }
            .padding(.horizontal, 24)
        }
        .sheet(isPresented: $viewModel.showPhoneBinding) {
            PhoneBindingView(onComplete: {
                viewModel.showPhoneBinding = false
            })
            .environmentObject(authState)
        }
        .alert("登录失败", isPresented: $viewModel.showError) {
            Button("确定", role: .cancel) {}
        } message: {
            Text(viewModel.errorMessage)
        }
    }

    // MARK: - Logo Section

    private var logoSection: some View {
        VStack(spacing: 20) {
            // 品牌图标
            Image("Images/Login/LoginBackground")
                .resizable()
                .scaledToFill()
                .frame(width: 120, height: 120)
                // 用 UnevenRoundedRectangle 做上圆下直裁剪，或直接用 clipped + offset
                .clipShape(
                    CropBottomShape(cropFraction: 0.12)
                )
                .clipShape(Circle())
                .shadow(color: Color(hex: "#3B82F6").opacity(0.35), radius: 24, x: 0, y: 8)

            // 书法 slogan
            Image("Images/Login/slogan")
                .resizable()
                .scaledToFit()
                .frame(maxWidth: 280)
                .frame(height: 72)
                .clipped()
                // 向上偏移让内容居中，底部超出的部分被 clipped 截掉
                .clipShape(CropBottomShape(cropFraction: 0.15))
        }
    }

    // MARK: - Login Card

    private var loginCard: some View {
        VStack(spacing: 20) {
            // WeChat login button
            Button(action: { viewModel.loginWithWeChat(authState: authState) }) {
                HStack(spacing: 12) {
                    if viewModel.isLoading {
                        ProgressView()
                            .progressViewStyle(CircularProgressViewStyle(tint: .white))
                            .frame(width: 24, height: 24)
                    } else {
                        // WeChat logo (green chat bubble icon)
                        ZStack {
                            RoundedRectangle(cornerRadius: 6)
                                .fill(Color.white.opacity(0.15))
                                .frame(width: 28, height: 28)
                            Image(systemName: "ellipsis.bubble.fill")
                                .font(.system(size: 18))
                                .foregroundColor(.white)
                        }
                    }
                    Text(viewModel.isLoading ? "正在打开微信..." : "微信登录")
                        .font(.system(size: 17, weight: .semibold))
                        .foregroundColor(.white)
                }
                .frame(maxWidth: .infinity)
                .frame(height: 54)
                .background(
                    LinearGradient(
                        colors: viewModel.isLoading
                            ? [Color(hex: "#4CAF50").opacity(0.6), Color(hex: "#2E7D32").opacity(0.6)]
                            : [Color(hex: "#4CAF50"), Color(hex: "#2E7D32")],
                        startPoint: .leading,
                        endPoint: .trailing
                    )
                )
                .cornerRadius(16)
                .shadow(color: Color(hex: "#4CAF50").opacity(0.3), radius: 12, x: 0, y: 4)
            }
            .disabled(viewModel.isLoading)
            .buttonStyle(ScaleButtonStyle())

            // Show a notice if WeChat is not installed
            if !WeChatManager.shared.isWeChatInstalled {
                HStack(spacing: 6) {
                    Image(systemName: "exclamationmark.circle")
                        .font(.system(size: 12))
                    Text("未检测到微信，将使用测试模式登录")
                        .font(.system(size: 12))
                }
                .foregroundColor(Color(hex: "#94A3B8"))
            }

            // Divider
            HStack {
                Rectangle().fill(Color(hex: "#1E293B")).frame(height: 1)
                Text("安全登录")
                    .font(.system(size: 12))
                    .foregroundColor(Color(hex: "#475569"))
                    .padding(.horizontal, 12)
                Rectangle().fill(Color(hex: "#1E293B")).frame(height: 1)
            }

            // Feature hints
            HStack(spacing: 24) {
                featureHint(icon: "lock.shield.fill", text: "数据安全")
                featureHint(icon: "chart.bar.fill", text: "实时行情")
                featureHint(icon: "brain.filled.head.profile", text: "AI 分析")
            }
        }
        .padding(24)
        .background(Color(hex: "#111827").opacity(0.8))
        .cornerRadius(24)
        .overlay(
            RoundedRectangle(cornerRadius: 24)
                .stroke(Color(hex: "#1E293B"), lineWidth: 1)
        )
    }

    private func featureHint(icon: String, text: String) -> some View {
        VStack(spacing: 6) {
            Image(systemName: icon)
                .font(.system(size: 20))
                .foregroundColor(Color(hex: "#3B82F6"))
            Text(text)
                .font(.system(size: 11))
                .foregroundColor(Color(hex: "#64748B"))
        }
        .frame(maxWidth: .infinity)
    }
}

// MARK: - LoginViewModel

final class LoginViewModel: ObservableObject {
    @Published var isLoading = false
    @Published var showPhoneBinding = false
    @Published var showError = false
    @Published var errorMessage = ""

    private var cancellables = Set<AnyCancellable>()

    /// Triggers the WeChat OAuth flow via WeChatManager.
    /// When the real SDK is integrated, this will jump to the WeChat app for authorization.
    /// Until then, a mock code is used automatically.
    func loginWithWeChat(authState: AuthState) {
        isLoading = true

        // Set up callbacks before calling sendAuthRequest
        WeChatManager.shared.onLoginCode = { [weak self] code in
            self?.performLogin(code: code, authState: authState)
        }
        WeChatManager.shared.onLoginError = { [weak self] message in
            DispatchQueue.main.async {
                self?.isLoading = false
                self?.errorMessage = message
                self?.showError = true
            }
        }

        WeChatManager.shared.sendAuthRequest()
    }

    func performLogin(code: String, authState: AuthState) {
        AuthService.shared.wechatLogin(code: code)
            .sink(receiveCompletion: { [weak self] completion in
                DispatchQueue.main.async {
                    self?.isLoading = false
                    if case .failure(let error) = completion {
                        self?.errorMessage = error.localizedDescription
                        self?.showError = true
                    }
                }
            }, receiveValue: { [weak self] response in
                DispatchQueue.main.async {
                    authState.signIn(token: response.token, user: response.user)
                    if response.needsPhoneBinding {
                        self?.showPhoneBinding = true
                    }
                }
            })
            .store(in: &cancellables)
    }
}

// MARK: - CropBottomShape
// 裁掉图片底部一定比例，用于去除 AI 生成水印

private struct CropBottomShape: Shape {
    /// 从底部裁掉的比例，0.12 = 裁掉 12%
    let cropFraction: CGFloat

    func path(in rect: CGRect) -> Path {
        var path = Path()
        let croppedHeight = rect.height * (1 - cropFraction)
        path.addRect(CGRect(x: rect.minX, y: rect.minY,
                            width: rect.width, height: croppedHeight))
        return path
    }
}

