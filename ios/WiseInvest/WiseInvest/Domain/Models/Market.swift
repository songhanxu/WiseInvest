import SwiftUI

/// The market modules of WiseInvest.
enum Market: String, Codable, CaseIterable, Identifiable {
    case aShare  = "a_share"
    case usStock = "us_stock"
    case hkStock = "hk_stock"
    case crypto  = "crypto"

    var id: String { rawValue }

    var displayName: String {
        switch self {
        case .aShare:  return "A 股"
        case .usStock: return "美 股"
        case .hkStock: return "港 股"
        case .crypto:  return "币 圈"
        }
    }

    var subtitle: String {
        switch self {
        case .aShare:  return "沪深北交所"
        case .usStock: return "NYSE · NASDAQ"
        case .hkStock: return "Bitget Quanto · HKD 报价"
        case .crypto:  return "加密货币"
        }
    }

    var description: String {
        switch self {
        case .aShare:  return "个股分析 · 行业研究 · 政策解读"
        case .usStock: return "财报分析 · 成长股 · 宏观经济"
        case .hkStock: return "港股研究 · 财报分析 · Quanto 合约"
        case .crypto:  return "现货合约 · 链上数据 · DeFi"
        }
    }

    var icon: String {
        switch self {
        case .aShare:  return "chart.bar.xaxis"
        case .usStock: return "dollarsign.circle"
        case .hkStock: return "building.2"
        case .crypto:  return "bitcoinsign.circle"
        }
    }

    var gradientColors: [Color] {
        switch self {
        case .aShare:  return [Color(hex: "B71C1C"), Color(hex: "E53935")]
        case .usStock: return [Color(hex: "0D47A1"), Color(hex: "1976D2")]
        case .hkStock: return [Color(hex: "7C3AED"), Color(hex: "A78BFA")]
        case .crypto:  return [Color(hex: "E65100"), Color(hex: "FF9800")]
        }
    }

    var accentColor: Color { gradientColors[0] }

    /// Maps to the backend agent_type string
    var agentType: String { rawValue }

    /// Welcome message shown when opening a new conversation
    var welcomeMessage: String {
        switch self {
        case .aShare:
            return "你好！我是你的A股投资分析助手。\n\n我可以帮你分析个股、行业趋势、技术形态、基本面数据以及政策影响。\n\n有什么想聊的？"
        case .usStock:
            return "Hi！我是你的美股投资分析助手。\n\n我可以帮你研究美股个股、解读财报、分析宏观经济数据以及美联储政策影响。\n\n想聊哪只股票？"
        case .hkStock:
            return "你好！我是你的港股研究助手。行情来自 Bitget Quanto 合约，按 HKD 报价。可以帮你研究港股公司财报、估值和宏观风险。"
        case .crypto:
            return "你好！我是你的加密货币分析助手。\n\n我可以分析BTC/ETH走势、合约机会、链上数据、DeFi协议以及加密市场结构。\n\n有什么想分析的？"
        }
    }
}
