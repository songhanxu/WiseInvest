import SwiftUI

// MARK: - Shimmer Modifier

/// A shimmer animation modifier that creates a glowing sweep effect across the content
struct ShimmerModifier: ViewModifier {
    @State private var phase: CGFloat = -1

    func body(content: Content) -> some View {
        content
            .overlay(
                GeometryReader { geometry in
                    LinearGradient(
                        gradient: Gradient(stops: [
                            .init(color: Color.white.opacity(0), location: 0),
                            .init(color: Color.white.opacity(0.15), location: 0.3),
                            .init(color: Color.white.opacity(0.35), location: 0.5),
                            .init(color: Color.white.opacity(0.15), location: 0.7),
                            .init(color: Color.white.opacity(0), location: 1.0),
                        ]),
                        startPoint: .leading,
                        endPoint: .trailing
                    )
                    .frame(width: geometry.size.width * 1.5)
                    .offset(x: geometry.size.width * phase)
                }
            )
            .clipped()
            .onAppear {
                withAnimation(
                    Animation.easeInOut(duration: 1.5)
                        .repeatForever(autoreverses: false)
                ) {
                    phase = 1.5
                }
            }
    }
}

/// A pulse animation modifier that makes the content breathe in and out
struct PulseModifier: ViewModifier {
    @State private var opacity: Double = 0.4

    func body(content: Content) -> some View {
        content
            .opacity(opacity)
            .onAppear {
                withAnimation(
                    Animation.easeInOut(duration: 1.0)
                        .repeatForever(autoreverses: true)
                ) {
                    opacity = 1.0
                }
            }
    }
}

extension View {
    func shimmer() -> some View {
        modifier(ShimmerModifier())
    }

    func pulse() -> some View {
        modifier(PulseModifier())
    }
}

// MARK: - Skeleton Shapes

/// A rounded rectangle skeleton placeholder with shimmer + pulse animation
struct SkeletonBox: View {
    var width: CGFloat? = nil
    var height: CGFloat = 16
    var cornerRadius: CGFloat = 6

    var body: some View {
        RoundedRectangle(cornerRadius: cornerRadius)
            .fill(Color.white.opacity(0.12))
            .frame(width: width, height: height)
            .shimmer()
            .pulse()
    }
}

// MARK: - Index Card Skeleton

/// Skeleton placeholder for an IndexCard while loading
struct IndexCardSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            // Title row: name + change percent
            HStack(spacing: 2) {
                SkeletonBox(width: 36, height: 12, cornerRadius: 4)
                Spacer(minLength: 2)
                SkeletonBox(width: 40, height: 12, cornerRadius: 4)
            }

            // Price
            SkeletonBox(width: 70, height: 18, cornerRadius: 4)

            // Sparkline placeholder
            SkeletonBox(height: 28, cornerRadius: 4)
        }
        .padding(10)
        .frame(maxWidth: .infinity)
        .background(Color.secondaryBackground)
        .cornerRadius(12)
    }
}

// MARK: - Stock Row Skeleton

/// Skeleton placeholder for a StockRow while loading
struct StockRowSkeleton: View {
    var body: some View {
        HStack(spacing: 14) {
            VStack(alignment: .leading, spacing: 6) {
                SkeletonBox(width: 80, height: 14, cornerRadius: 4)
                SkeletonBox(width: 50, height: 11, cornerRadius: 4)
            }

            Spacer()

            SkeletonBox(width: 60, height: 16, cornerRadius: 4)

            SkeletonBox(width: 80, height: 28, cornerRadius: 6)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(Color.secondaryBackground)
        .cornerRadius(12)
    }
}

// MARK: - K-Line Chart Skeleton

/// Skeleton placeholder for KLineChartView while loading
struct KLineChartSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Header row: "K线走势" + "近X日"
            HStack {
                SkeletonBox(width: 70, height: 14, cornerRadius: 4)
                Spacer()
                SkeletonBox(width: 40, height: 12, cornerRadius: 4)
            }

            // Fake candlestick area
            HStack(alignment: .bottom, spacing: 3) {
                ForEach(0..<20, id: \.self) { i in
                    // Varying heights to mimic candlesticks
                    let heights: [CGFloat] = [90, 110, 75, 130, 95, 120, 85, 140, 100, 70,
                                              115, 80, 125, 105, 65, 135, 88, 110, 98, 72]
                    RoundedRectangle(cornerRadius: 1.5)
                        .fill(Color.white.opacity(0.12))
                        .frame(height: heights[i % heights.count])
                        .shimmer()
                        .pulse()
                }
            }
            .frame(height: 150)
        }
        .padding(16)
        .background(Color.secondaryBackground)
        .cornerRadius(16)
    }
}

// MARK: - AI Analysis Card Skeleton

/// Skeleton placeholder for an AnalysisCard while loading
struct AnalysisCardSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Title row: icon + title
            HStack(spacing: 8) {
                SkeletonBox(width: 14, height: 14, cornerRadius: 3)
                SkeletonBox(width: 80, height: 14, cornerRadius: 4)
            }

            // Content lines
            SkeletonBox(height: 12, cornerRadius: 4)
            SkeletonBox(width: UIScreen.main.bounds.width * 0.7, height: 12, cornerRadius: 4)
            SkeletonBox(width: UIScreen.main.bounds.width * 0.5, height: 12, cornerRadius: 4)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.secondaryBackground)
        .cornerRadius(14)
    }
}

// MARK: - News Card Skeleton

/// Skeleton placeholder for a NewsCard (AI summary style) while loading
struct NewsCardSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            // Top row: sentiment badge + source + time
            HStack {
                SkeletonBox(width: 36, height: 18, cornerRadius: 4)
                SkeletonBox(width: 50, height: 11, cornerRadius: 4)
                Spacer()
                SkeletonBox(width: 60, height: 11, cornerRadius: 4)
            }

            // Title lines
            SkeletonBox(height: 14, cornerRadius: 4)
            SkeletonBox(width: UIScreen.main.bounds.width * 0.7, height: 14, cornerRadius: 4)

            // AI Summary block
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    SkeletonBox(width: 10, height: 10, cornerRadius: 3)
                    SkeletonBox(height: 12, cornerRadius: 4)
                }
                SkeletonBox(width: UIScreen.main.bounds.width * 0.65, height: 12, cornerRadius: 4)
                SkeletonBox(width: UIScreen.main.bounds.width * 0.5, height: 12, cornerRadius: 4)
            }
            .padding(10)
            .background(Color.white.opacity(0.04))
            .cornerRadius(8)

            // Bottom hint
            HStack {
                Spacer()
                SkeletonBox(width: 50, height: 11, cornerRadius: 4)
            }
        }
        .padding(14)
        .background(Color.secondaryBackground)
        .cornerRadius(14)
    }
}
