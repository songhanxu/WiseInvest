import Foundation

/// AgentType is kept for backward compatibility with the existing conversation infrastructure.
/// New code should use `Market` directly.
enum AgentType: String, Codable, CaseIterable, Identifiable {
    // Market-based types (primary)
    case aShare  = "a_share"
    case usStock = "us_stock"
    case crypto  = "crypto"

    // Legacy types (kept for backward compatibility)
    case orchestrator      = "orchestrator"
    case conversation      = "conversation"
    case investmentAdvisor = "investment_advisor"
    case tradingAgent      = "trading_agent"

    var id: String { rawValue }

    /// Creates an AgentType from a Market value
    init(market: Market) {
        switch market {
        case .aShare:  self = .aShare
        case .usStock: self = .usStock
        case .crypto:  self = .crypto
        }
    }

    var displayName: String {
        switch self {
        case .aShare:          return "A 股"
        case .usStock:         return "美 股"
        case .crypto:          return "币 圈"
        case .orchestrator:    return "智能助手"
        case .conversation,
             .investmentAdvisor: return "投资顾问"
        case .tradingAgent:    return "交易助手"
        }
    }

    var description: String {
        switch self {
        case .aShare:          return "沪深北交所 · A股投资分析"
        case .usStock:         return "NYSE/NASDAQ · 美股研究"
        case .crypto:          return "加密货币 · 合约现货分析"
        case .orchestrator:    return "主控 AI 助手"
        case .conversation,
             .investmentAdvisor: return "专业投资分析"
        case .tradingAgent:    return "交易执行助手"
        }
    }

    var icon: String {
        switch self {
        case .aShare:          return "chart.bar.xaxis"
        case .usStock:         return "dollarsign.circle"
        case .crypto:          return "bitcoinsign.circle"
        case .orchestrator:    return "brain.head.profile"
        case .conversation,
             .investmentAdvisor: return "chart.line.uptrend.xyaxis"
        case .tradingAgent:    return "arrow.left.arrow.right"
        }
    }

    var gradientColors: [String] {
        switch self {
        case .aShare:          return ["B71C1C", "E53935"]
        case .usStock:         return ["0D47A1", "1976D2"]
        case .crypto:          return ["E65100", "FF9800"]
        case .orchestrator:    return ["9C27B0", "AB47BC"]
        case .conversation,
             .investmentAdvisor: return ["4CAF50", "66BB6A"]
        case .tradingAgent:    return ["2196F3", "42A5F5"]
        }
    }
}
