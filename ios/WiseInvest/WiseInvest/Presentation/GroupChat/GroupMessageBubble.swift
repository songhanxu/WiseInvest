import SwiftUI

struct GroupMessageBubble: View {
    let message: GroupChatMessage
    @State private var animationPhase = 0

    var body: some View {
        if message.isFromUser {
            userBubble
        } else {
            agentBubble
        }
    }

    // MARK: - User Bubble (right-aligned)

    private var userBubble: some View {
        HStack(alignment: .bottom, spacing: 0) {
            Spacer(minLength: 64)
            VStack(alignment: .trailing, spacing: 4) {
                Text(message.content)
                    .font(.system(size: 15))
                    .foregroundColor(.white)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                    .background(Color.accentBlue)
                    .clipShape(BubbleShape(isFromUser: true))

                Text(formattedTime)
                    .font(.system(size: 11))
                    .foregroundColor(.textTertiary)
                    .padding(.trailing, 4)
            }
        }
        .padding(.horizontal, 16)
    }

    // MARK: - Agent Bubble (left-aligned with avatar)

    private var agentBubble: some View {
        guard let agent = message.agent else { return AnyView(EmptyView()) }

        return AnyView(
            HStack(alignment: .top, spacing: 10) {
                // Agent avatar
                ZStack {
                    Circle()
                        .fill(agent.color.opacity(0.85))
                        .frame(width: 36, height: 36)
                    Image(systemName: agent.icon)
                        .font(.system(size: 15))
                        .foregroundColor(.white)
                }

                VStack(alignment: .leading, spacing: 4) {
                    // Agent name
                    Text(agent.name)
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(agent.color)

                    // Bubble content
                    Group {
                        if message.isStreaming && message.content.isEmpty {
                            streamingDots
                        } else {
                            messageContent(for: message.content)
                        }
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                    .background(Color(hex: "1E2540"))
                    .clipShape(BubbleShape(isFromUser: false))

                    if !message.isStreaming {
                        Text(formattedTime)
                            .font(.system(size: 11))
                            .foregroundColor(.textTertiary)
                            .padding(.leading, 4)
                    }
                }

                Spacer(minLength: 40)
            }
            .padding(.horizontal, 16)
        )
    }

    // MARK: - Message Content

    @ViewBuilder
    private func messageContent(for content: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            InlineMarkdownText(text: content)
        }
    }

    // MARK: - Streaming Dots

    private var streamingDots: some View {
        HStack(spacing: 4) {
            ForEach(0..<3, id: \.self) { index in
                Circle()
                    .fill(Color.textSecondary)
                    .frame(width: 6, height: 6)
                    .opacity(dotOpacity(for: index))
            }
        }
        .onAppear { startAnimation() }
        .onDisappear { animationPhase = 0 }
    }

    private func dotOpacity(for index: Int) -> Double {
        let phase = (animationPhase + index) % 3
        return phase == 0 ? 1.0 : 0.3
    }

    private func startAnimation() {
        Timer.scheduledTimer(withTimeInterval: 0.4, repeats: true) { timer in
            if !message.isStreaming {
                timer.invalidate()
                return
            }
            animationPhase = (animationPhase + 1) % 3
        }
    }

    private var formattedTime: String {
        let formatter = DateFormatter()
        formatter.timeStyle = .short
        return formatter.string(from: message.timestamp)
    }
}

// MARK: - BubbleShape

private struct BubbleShape: Shape {
    let isFromUser: Bool
    let radius: CGFloat = 16
    let tailRadius: CGFloat = 4

    func path(in rect: CGRect) -> Path {
        var path = Path()
        if isFromUser {
            // Round all corners, slightly flatten bottom-right
            path.addRoundedRect(in: rect, cornerSize: CGSize(width: radius, height: radius))
        } else {
            path.addRoundedRect(in: rect, cornerSize: CGSize(width: radius, height: radius))
        }
        return path
    }
}
