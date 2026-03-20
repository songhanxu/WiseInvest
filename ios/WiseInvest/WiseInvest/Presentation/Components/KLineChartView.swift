import SwiftUI

/// Mini sparkline chart for market index cards
struct SparklineView: View {
    let data: [Double]
    let isUp: Bool

    var body: some View {
        GeometryReader { geometry in
            let width = geometry.size.width
            let height = geometry.size.height
            guard data.count > 1 else { return AnyView(EmptyView()) }
            let minVal = data.min() ?? 0
            let maxVal = data.max() ?? 1
            let range = maxVal - minVal
            let step = width / CGFloat(data.count - 1)

            return AnyView(
                Path { path in
                    for (index, value) in data.enumerated() {
                        let x = step * CGFloat(index)
                        let y = height - (range > 0 ? CGFloat((value - minVal) / range) * height : height / 2)
                        if index == 0 {
                            path.move(to: CGPoint(x: x, y: y))
                        } else {
                            path.addLine(to: CGPoint(x: x, y: y))
                        }
                    }
                }
                .stroke(isUp ? Color.accentGreen : Color(hex: "E53935"), lineWidth: 1.5)
            )
        }
    }
}

/// K-Line (candlestick) chart view
struct KLineChartView: View {
    let data: [KLinePoint]
    let accentColor: Color

    @State private var selectedPoint: KLinePoint?
    @State private var selectedIndex: Int?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Selected point info or summary
            if let point = selectedPoint {
                selectedPointInfo(point)
            } else {
                chartSummary
            }

            // Chart
            GeometryReader { geometry in
                let width = geometry.size.width
                let height = geometry.size.height
                let candleWidth = max(2, (width - CGFloat(data.count)) / CGFloat(data.count))
                let minPrice = data.map { $0.low }.min() ?? 0
                let maxPrice = data.map { $0.high }.max() ?? 1
                let priceRange = maxPrice - minPrice

                ZStack(alignment: .bottom) {
                    // Grid lines
                    VStack(spacing: 0) {
                        ForEach(0..<4, id: \.self) { i in
                            Spacer()
                            Rectangle()
                                .fill(Color.white.opacity(0.05))
                                .frame(height: 0.5)
                        }
                        Spacer()
                    }

                    // Candlesticks
                    HStack(alignment: .bottom, spacing: 1) {
                        ForEach(Array(data.enumerated()), id: \.element.id) { index, point in
                            CandleView(
                                point: point,
                                width: candleWidth,
                                height: height,
                                minPrice: minPrice,
                                priceRange: priceRange,
                                isSelected: selectedIndex == index
                            )
                            .onTapGesture {
                                withAnimation(.easeInOut(duration: 0.15)) {
                                    if selectedIndex == index {
                                        selectedIndex = nil
                                        selectedPoint = nil
                                    } else {
                                        selectedIndex = index
                                        selectedPoint = point
                                    }
                                }
                            }
                        }
                    }
                }
                .gesture(
                    DragGesture(minimumDistance: 0)
                        .onChanged { value in
                            let step = width / CGFloat(data.count)
                            let idx = Int(value.location.x / step)
                            if idx >= 0 && idx < data.count {
                                selectedIndex = idx
                                selectedPoint = data[idx]
                            }
                        }
                        .onEnded { _ in
                            // keep selection
                        }
                )
            }
            .frame(height: 180)
        }
        .padding(16)
        .background(Color.secondaryBackground)
        .cornerRadius(16)
    }

    private func selectedPointInfo(_ point: KLinePoint) -> some View {
        let formatter = DateFormatter()
        formatter.dateFormat = "M月d日"
        return HStack(spacing: 16) {
            VStack(alignment: .leading, spacing: 2) {
                Text(formatter.string(from: point.date))
                    .font(.system(size: 12))
                    .foregroundColor(.textSecondary)
                HStack(spacing: 12) {
                    infoLabel("开", value: String(format: "%.2f", point.open))
                    infoLabel("收", value: String(format: "%.2f", point.close))
                    infoLabel("高", value: String(format: "%.2f", point.high))
                    infoLabel("低", value: String(format: "%.2f", point.low))
                }
            }
            Spacer()
        }
    }

    private func infoLabel(_ label: String, value: String) -> some View {
        HStack(spacing: 2) {
            Text(label)
                .font(.system(size: 10))
                .foregroundColor(.textTertiary)
            Text(value)
                .font(.system(size: 11, weight: .medium, design: .monospaced))
                .foregroundColor(.textPrimary)
        }
    }

    private var chartSummary: some View {
        HStack {
            Text("K线走势")
                .font(.system(size: 14, weight: .semibold))
                .foregroundColor(.textPrimary)
            Spacer()
            Text("近\(data.count)日")
                .font(.system(size: 12))
                .foregroundColor(.textTertiary)
        }
    }
}

// MARK: - Candle View

private struct CandleView: View {
    let point: KLinePoint
    let width: CGFloat
    let height: CGFloat
    let minPrice: Double
    let priceRange: Double
    let isSelected: Bool

    var body: some View {
        let color = point.isUp ? Color.accentGreen : Color(hex: "E53935")
        let bodyTop = max(point.open, point.close)
        let bodyBottom = min(point.open, point.close)

        let topY = priceRange > 0 ? CGFloat((point.high - minPrice) / priceRange) * height : 0
        let bodyTopY = priceRange > 0 ? CGFloat((bodyTop - minPrice) / priceRange) * height : 0
        let bodyBottomY = priceRange > 0 ? CGFloat((bodyBottom - minPrice) / priceRange) * height : 0
        let bottomY = priceRange > 0 ? CGFloat((point.low - minPrice) / priceRange) * height : 0

        let bodyHeight = max(1, bodyTopY - bodyBottomY)

        ZStack {
            // Wick (shadow line)
            Rectangle()
                .fill(color)
                .frame(width: 1, height: topY - bottomY)
                .offset(y: -(bottomY + (topY - bottomY) / 2 - height / 2))

            // Body
            Rectangle()
                .fill(color)
                .frame(width: max(2, width - 1), height: bodyHeight)
                .offset(y: -(bodyBottomY + bodyHeight / 2 - height / 2))
        }
        .frame(width: width, height: height)
        .opacity(isSelected ? 1.0 : 0.85)
    }
}
