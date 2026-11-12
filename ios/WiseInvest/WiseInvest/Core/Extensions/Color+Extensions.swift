import SwiftUI
import UIKit

extension Color {
    // Shared neutral surfaces and system-blue actions across all clients.
    static let primaryBackground = themed(light: "F2F2F7", dark: "000000")
    static let secondaryBackground = themed(light: "FFFFFF", dark: "1C1C1E")
    static let subtleBackground = themed(light: "E5E5EA", dark: "2C2C2E")
    static let marketIconBackground = themed(light: "F2F2F7", dark: "2C2C2E")
    static let accentBlue = themed(light: "007AFF", dark: "0A84FF")
    static let accentGreen = themed(light: "328269", dark: "70C99C")
    static let accentPurple = themed(light: "636366", dark: "AEAEB2")
    static let textPrimary = themed(light: "1C1C1E", dark: "F2F2F7")
    static let textSecondary = themed(light: "636366", dark: "AEAEB2")
    static let textTertiary = themed(light: "8E8E93", dark: "98989D")
    static let separator = themed(light: "E5E5EA", dark: "38383A")
    static let skeleton = textPrimary.opacity(0.10)
    static let userMessageBg = themed(light: "E5E5EA", dark: "2C2C2E")
    static let assistantMessageBg = themed(light: "FFFFFF", dark: "1C1C1E")

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
