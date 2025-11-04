import SwiftUI

/// Message bubble component for chat interface
struct MessageBubble: View {
    let message: Message
    @State private var animationPhase = 0
    
    var body: some View {
        HStack {
            if message.role == .user {
                Spacer(minLength: 60)
            }
            
            VStack(alignment: message.role == .user ? .trailing : .leading, spacing: 4) {
                Text(message.content)
                    .font(.system(size: 16))
                    .foregroundColor(.textPrimary)
                    .padding(12)
                    .background(backgroundColor)
                    .cornerRadius(16)
                
                if message.isStreaming {
                    StreamingIndicator(animationPhase: animationPhase)
                        .padding(.horizontal, 12)
                        .onAppear {
                            startAnimation()
                        }
                } else {
                    Text(formattedTime)
                        .font(.system(size: 12))
                        .foregroundColor(.textTertiary)
                        .padding(.horizontal, 12)
                }
            }
            
            if message.role == .assistant {
                Spacer(minLength: 60)
            }
        }
    }
    
    private var backgroundColor: Color {
        switch message.role {
        case .user:
            return Color.userMessageBg
        case .assistant:
            return Color.assistantMessageBg
        case .system:
            return Color.secondaryBackground
        }
    }
    
    private var formattedTime: String {
        let formatter = DateFormatter()
        formatter.timeStyle = .short
        return formatter.string(from: message.timestamp)
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
}

/// Streaming indicator with animated dots
struct StreamingIndicator: View {
    let animationPhase: Int
    
    var body: some View {
        HStack(spacing: 4) {
            ForEach(0..<3, id: \.self) { index in
                Circle()
                    .fill(Color.textSecondary)
                    .frame(width: 6, height: 6)
                    .opacity(streamingOpacity(for: index))
            }
        }
    }
    
    private func streamingOpacity(for index: Int) -> Double {
        let phase = (animationPhase + index) % 3
        return phase == 0 ? 1.0 : 0.3
    }
}
