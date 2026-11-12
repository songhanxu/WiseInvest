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
                    .foregroundColor(.textSecondary)
                    .frame(width: 36, height: 36)
                    .background(Color.marketIconBackground, in: RoundedRectangle(cornerRadius: 12))

                VStack(alignment: .leading, spacing: 6) {
                    Text(market.displayName)
                        .font(.system(size: 17, weight: .medium))
                        .foregroundColor(.textPrimary)

                    Text(market.subtitle)
                        .font(.system(size: 12))
                        .foregroundColor(.textSecondary)


                }
                Spacer(minLength: 0)
                Image(systemName: "chevron.right")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(.textTertiary)
            }
            .padding(18)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color.secondaryBackground)
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
                    RoundedRectangle(cornerRadius: 16)
                        .fill(Color.marketIconBackground)
                    .frame(width: 60, height: 60)
                    .cornerRadius(16)

                    Image(systemName: agentType.icon)
                        .font(.system(size: 28))
                        .foregroundColor(.textSecondary)
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
