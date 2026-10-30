import SwiftUI

// MARK: - MarketCard

/// Large card for displaying a market module on the home screen
struct MarketCard: View {
    let market: Market
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            HStack(alignment: .center, spacing: 16) {
                Image(systemName: market.icon)
                    .font(.system(size: 22, weight: .regular))
                    .foregroundColor(.accentBlue)
                    .frame(width: 48, height: 48)
                    .background(Color.marketIconBackground, in: RoundedRectangle(cornerRadius: 12))

                VStack(alignment: .leading, spacing: 6) {
                    Text(market.displayName)
                        .font(.system(size: 19, weight: .semibold))
                        .foregroundColor(.textPrimary)

                    Text(market.subtitle)
                        .font(.system(size: 12))
                        .foregroundColor(.textSecondary)

                    Text(market.description)
                        .font(.system(size: 12))
                        .foregroundColor(.textTertiary)
                        .lineLimit(1)
                }
                Spacer(minLength: 0)
                Image(systemName: "chevron.right")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(.textTertiary)
            }
            .padding(18)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color.secondaryBackground, in: RoundedRectangle(cornerRadius: 16))
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
