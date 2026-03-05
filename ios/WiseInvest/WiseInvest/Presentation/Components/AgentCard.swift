import SwiftUI

// MARK: - MarketCard

/// Large card for displaying a market module on the home screen
struct MarketCard: View {
    let market: Market
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            ZStack(alignment: .bottomLeading) {
                // Background gradient
                LinearGradient(
                    colors: market.gradientColors,
                    startPoint: .topLeading,
                    endPoint: .bottomTrailing
                )

                // Decorative large icon (top right)
                Image(systemName: market.icon)
                    .font(.system(size: 80, weight: .ultraLight))
                    .foregroundColor(.white.opacity(0.12))
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topTrailing)
                    .padding(.top, 16)
                    .padding(.trailing, 20)

                // Content (bottom left)
                VStack(alignment: .leading, spacing: 6) {
                    Text(market.displayName)
                        .font(.system(size: 28, weight: .bold))
                        .foregroundColor(.white)

                    Text(market.subtitle)
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.white.opacity(0.75))

                    Spacer().frame(height: 4)

                    Text(market.description)
                        .font(.system(size: 13))
                        .foregroundColor(.white.opacity(0.85))
                        .lineLimit(1)
                }
                .padding(20)
            }
            .frame(height: 140)
            .cornerRadius(20)
            .shadow(color: market.gradientColors[0].opacity(0.4), radius: 12, x: 0, y: 6)
        }
        .buttonStyle(ScaleButtonStyle())
    }
}

// MARK: - AgentCard (legacy, kept for backward compatibility)

struct AgentCard: View {
    let agentType: AgentType
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 16) {
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

// MARK: - ScaleButtonStyle

struct ScaleButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? 0.96 : 1.0)
            .animation(.easeInOut(duration: 0.15), value: configuration.isPressed)
    }
}
