import Foundation

/// Types of AI agents available in the app
enum AgentType: String, Codable, CaseIterable, Identifiable {
    case investmentAdvisor = "investment_advisor"
    case tradingAgent = "trading_agent"
    
    // Identifiable conformance
    var id: String { self.rawValue }
    
    var displayName: String {
        switch self {
        case .investmentAdvisor:
            return "Investment Advisor"
        case .tradingAgent:
            return "Trading Agent"
        }
    }
    
    var description: String {
        switch self {
        case .investmentAdvisor:
            return "Get professional investment advice and market insights"
        case .tradingAgent:
            return "Execute trades and manage your Binance portfolio"
        }
    }
    
    var icon: String {
        switch self {
        case .investmentAdvisor:
            return "chart.line.uptrend.xyaxis"
        case .tradingAgent:
            return "bitcoinsign.circle.fill"
        }
    }
    
    var gradientColors: [String] {
        switch self {
        case .investmentAdvisor:
            return ["4A90E2", "5B9FE3"]
        case .tradingAgent:
            return ["50C878", "60D888"]
        }
    }
}
