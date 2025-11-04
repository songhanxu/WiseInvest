import SwiftUI

/// Card component for displaying an agent option
struct AgentCard: View {
    let agentType: AgentType
    let onTap: () -> Void
    
    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 16) {
                // Icon
                ZStack {
                    LinearGradient(
                        colors: agentType.gradientColors.map { Color(hex: $0) },
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                    .frame(width: 60, height: 60)
                    .cornerRadius(16)
                    
                    Image(systemName: agentType.icon)
                        .font(.system(size: 28))
                        .foregroundColor(.white)
                }
                
                // Content
                VStack(alignment: .leading, spacing: 6) {
                    Text(agentType.displayName)
                        .font(.system(size: 20, weight: .semibold))
                        .foregroundColor(.textPrimary)
                    
                    Text(agentType.description)
                        .font(.system(size: 14))
                        .foregroundColor(.textSecondary)
                        .lineLimit(2)
                }
                
                Spacer()
                
                // Arrow
                Image(systemName: "chevron.right")
                    .font(.system(size: 16, weight: .semibold))
                    .foregroundColor(.textTertiary)
            }
            .padding(20)
            .background(Color.secondaryBackground)
            .cornerRadius(16)
            .shadow(color: Color.black.opacity(0.2), radius: 10, x: 0, y: 4)
        }
        .buttonStyle(ScaleButtonStyle())
    }
}

/// Button style with scale animation
struct ScaleButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? 0.96 : 1.0)
            .animation(.easeInOut(duration: 0.2), value: configuration.isPressed)
    }
}
