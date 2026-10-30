import SwiftUI
import UIKit

extension Color {
    // The light palette follows the web workspace; the dark palette keeps the iOS navy surfaces.
    static let primaryBackground = themed(light: "F7F8F6", dark: "0A0E27")
    static let secondaryBackground = themed(light: "FFFFFF", dark: "1A1F3A")
    static let subtleBackground = themed(light: "F3F5F2", dark: "121831")
    static let marketIconBackground = themed(light: "EDF2E9", dark: "283552")
    static let accentBlue = themed(light: "35664B", dark: "4A90E2")
    static let accentGreen = themed(light: "2C7951", dark: "50C878")
    static let accentPurple = themed(light: "736887", dark: "9B59B6")

    static let textPrimary = themed(light: "202522", dark: "FFFFFF")
    static let textSecondary = themed(light: "727B72", dark: "A0A0A0")
    static let textTertiary = themed(light: "879187", dark: "888FA8")
    static let skeleton = textPrimary.opacity(0.10)

    static let userMessageBg = themed(light: "35664B", dark: "4A90E2")
    static let assistantMessageBg = themed(light: "EDF1EC", dark: "2C3E50")

    static func themed(light: String, dark: String) -> Color {
        Color(uiColor: UIColor { traits in
            UIColor(Color(hex: traits.userInterfaceStyle == .dark ? dark : light))
        })
    }
    
    // Helper to create color from hex
    init(hex: String) {
        let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        var int: UInt64 = 0
        Scanner(string: hex).scanHexInt64(&int)
        let a, r, g, b: UInt64
        switch hex.count {
        case 3: // RGB (12-bit)
            (a, r, g, b) = (255, (int >> 8) * 17, (int >> 4 & 0xF) * 17, (int & 0xF) * 17)
        case 6: // RGB (24-bit)
            (a, r, g, b) = (255, int >> 16, int >> 8 & 0xFF, int & 0xFF)
        case 8: // ARGB (32-bit)
            (a, r, g, b) = (int >> 24, int >> 16 & 0xFF, int >> 8 & 0xFF, int & 0xFF)
        default:
            (a, r, g, b) = (255, 0, 0, 0)
        }
        
        self.init(
            .sRGB,
            red: Double(r) / 255,
            green: Double(g) / 255,
            blue:  Double(b) / 255,
            opacity: Double(a) / 255
        )
    }
}
