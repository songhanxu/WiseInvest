import SwiftUI

struct GroupChatView: View {
    @StateObject private var viewModel = GroupChatViewModel()
    @Environment(\.dismiss) private var dismiss
    @FocusState private var isInputFocused: Bool
    @State private var isAtBottom = true
    @State private var showAgentPanel = false

    private let agents = GroupChatAgent.allAgents

    var body: some View {
        ZStack {
            Color.primaryBackground.ignoresSafeArea()

            VStack(spacing: 0) {
                headerBar

                if showAgentPanel {
                    agentPanel
                        .transition(.move(edge: .top).combined(with: .opacity))
                }

                Divider()
                    .background(Color.textTertiary.opacity(0.2))

                messagesArea

                if let error = viewModel.errorMessage {
                    errorBanner(error)
                }

                inputBar
            }
        }
        .animation(.spring(response: 0.35, dampingFraction: 0.8), value: showAgentPanel)
    }

    // MARK: - Header

    private var headerBar: some View {
        HStack(spacing: 12) {
            Button(action: { dismiss() }) {
                Image(systemName: "chevron.left")
                    .font(.system(size: 18, weight: .semibold))
                    .foregroundColor(.textPrimary)
                    .frame(width: 36, height: 36)
            }

            groupAvatarStack
                .onTapGesture { withAnimation { showAgentPanel.toggle() } }

            VStack(alignment: .leading, spacing: 1) {
                Text("慧投圆桌")
                    .font(.system(size: 17, weight: .semibold))
                    .foregroundColor(.textPrimary)
                Text("\(agents.count) 位分析师")
                    .font(.system(size: 12))
                    .foregroundColor(.textSecondary)
            }
            .onTapGesture { withAnimation { showAgentPanel.toggle() } }

            Spacer()

            Button(action: { withAnimation { showAgentPanel.toggle() } }) {
                Image(systemName: showAgentPanel ? "chevron.up" : "person.2")
                    .font(.system(size: 16))
                    .foregroundColor(.textSecondary)
                    .frame(width: 36, height: 36)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(Color.secondaryBackground)
    }

    // MARK: - Group Avatar Stack

    private var groupAvatarStack: some View {
        ZStack {
            RoundedRectangle(cornerRadius: 12)
                .fill(Color(hex: "0A0E27"))
                .frame(width: 44, height: 44)

            LazyVGrid(
                columns: [GridItem(.fixed(18)), GridItem(.fixed(18))],
                spacing: 2
            ) {
                ForEach(agents.prefix(4)) { agent in
                    ZStack {
                        Circle()
                            .fill(agent.color.opacity(0.85))
                        Image(systemName: agent.icon)
                            .font(.system(size: 7))
                            .foregroundColor(.white)
                    }
                    .frame(width: 18, height: 18)
                }
            }
        }
        .frame(width: 44, height: 44)
    }

    // MARK: - Agent Panel

    private var agentPanel: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 14) {
                ForEach(agents) { agent in
                    VStack(spacing: 6) {
                        ZStack {
                            Circle()
                                .fill(agent.color.opacity(0.2))
                                .frame(width: 48, height: 48)
                            Circle()
                                .stroke(agent.color.opacity(0.6), lineWidth: 1.5)
                                .frame(width: 48, height: 48)
                            Image(systemName: agent.icon)
                                .font(.system(size: 20))
                                .foregroundColor(agent.color)
                        }
                        Text(agent.name)
                            .font(.system(size: 12, weight: .medium))
                            .foregroundColor(.textPrimary)
                        Text(agent.role)
                            .font(.system(size: 10))
                            .foregroundColor(.textTertiary)
                    }
                    .frame(width: 72)
                }
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 12)
        }
        .background(Color(hex: "141830"))
    }

    // MARK: - Messages Area

    private var messagesArea: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(spacing: 14) {
                    dateSeparator(date: Date())
                        .padding(.top, 12)

                    ForEach(viewModel.messages) { message in
                        GroupMessageBubble(message: message)
                            .id(message.id)
                    }

                    Color.clear.frame(height: 1).id("__bottom__")
                        .onAppear  { isAtBottom = true  }
                        .onDisappear { isAtBottom = false }
                }
                .padding(.bottom, 8)
            }
            .scrollDismissesKeyboard(.interactively)
            .onTapGesture { isInputFocused = false }
            .onChange(of: viewModel.messages.count) { _ in
                isAtBottom = true
                withAnimation(.easeOut(duration: 0.25)) {
                    proxy.scrollTo("__bottom__", anchor: .bottom)
                }
            }
            .onChange(of: viewModel.messages.last?.content) { _ in
                guard isAtBottom else { return }
                proxy.scrollTo("__bottom__", anchor: .bottom)
            }
        }
    }

    private func dateSeparator(date: Date) -> some View {
        let formatter = DateFormatter()
        formatter.dateFormat = "M月d日 EEEE"
        formatter.locale = Locale(identifier: "zh_CN")

        return HStack(spacing: 8) {
            Rectangle().fill(Color.textTertiary.opacity(0.3)).frame(height: 1)
            Text(formatter.string(from: date))
                .font(.system(size: 11))
                .foregroundColor(.textTertiary)
                .fixedSize()
            Rectangle().fill(Color.textTertiary.opacity(0.3)).frame(height: 1)
        }
        .padding(.horizontal, 24)
    }

    // MARK: - Input Bar

    private var inputBar: some View {
        VStack(spacing: 0) {
            mentionHintStrip

            HStack(spacing: 12) {
                TextField("发送消息，@主持人 让所有人参与…", text: $viewModel.inputText, axis: .vertical)
                    .textFieldStyle(.plain)
                    .padding(12)
                    .background(Color.secondaryBackground)
                    .cornerRadius(20)
                    .foregroundColor(.textPrimary)
                    .lineLimit(1...5)
                    .focused($isInputFocused)
                    .disabled(viewModel.isProcessing)

                sendButton
            }
            .padding()
            .background(Color.primaryBackground)
        }
    }

    private var mentionHintStrip: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(["@主持人", "@价值派", "@趋势派", "@量化派"], id: \.self) { mention in
                    Button(action: {
                        viewModel.inputText += mention + " "
                        isInputFocused = true
                    }) {
                        Text(mention)
                            .font(.system(size: 12, weight: .medium))
                            .foregroundColor(.accentBlue)
                            .padding(.horizontal, 10)
                            .padding(.vertical, 5)
                            .background(Color.accentBlue.opacity(0.12))
                            .cornerRadius(10)
                    }
                }
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 8)
        }
        .background(Color.primaryBackground)
    }

    private func errorBanner(_ message: String) -> some View {
        HStack(spacing: 10) {
            Image(systemName: "exclamationmark.triangle.fill").foregroundColor(.red)
            Text(message)
                .font(.system(size: 13))
                .foregroundColor(.textPrimary)
                .lineLimit(2)
            Spacer()
            Button(action: { viewModel.retryInit() }) {
                Text("重试")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.accentBlue)
            }
            Button(action: { viewModel.errorMessage = nil }) {
                Image(systemName: "xmark")
                    .font(.system(size: 12))
                    .foregroundColor(.textSecondary)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(Color.red.opacity(0.18))
    }

    private var sendButton: some View {
        Button(action: {
            viewModel.sendMessage()
            isInputFocused = false
        }) {
            if viewModel.isProcessing {
                ProgressView()
                    .progressViewStyle(CircularProgressViewStyle(tint: .white))
                    .frame(width: 44, height: 44)
            } else {
                Image(systemName: "arrow.up.circle.fill")
                    .font(.system(size: 44))
                    .foregroundColor(viewModel.inputText.isEmpty ? .textTertiary : .accentBlue)
            }
        }
        .disabled(viewModel.inputText.isEmpty || viewModel.isProcessing)
    }
}
