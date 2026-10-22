import SwiftUI

/// Represents an AI agent participating in the investment roundtable group chat
struct GroupChatAgent: Identifiable {
    let id: String
    let name: String
    let icon: String
    let colorHex: String
    let role: String
    let tradingStyle: TradingStyle

    enum TradingStyle {
        case orchestrator   // 主持人：综合协调
        case value          // 价值派：基本面
        case trend          // 趋势派：技术面
        case quant          // 量化派：数据模型

        var description: String {
            switch self {
            case .orchestrator: return "综合协调 · 主持讨论"
            case .value:        return "价值投资 · 基本面分析"
            case .trend:        return "趋势交易 · 技术分析"
            case .quant:        return "量化模型 · 数据驱动"
            }
        }
    }

    var color: Color { Color(hex: colorHex) }

    static let allAgents: [GroupChatAgent] = [
        GroupChatAgent(
            id: "orchestrator",
            name: "主持人",
            icon: "brain.head.profile",
            colorHex: "8B5CF6",
            role: "圆桌主持",
            tradingStyle: .orchestrator
        ),
        GroupChatAgent(
            id: "value",
            name: "价值派",
            icon: "scalemass",
            colorHex: "10B981",
            role: "价值投资者",
            tradingStyle: .value
        ),
        GroupChatAgent(
            id: "trend",
            name: "趋势派",
            icon: "chart.line.uptrend.xyaxis",
            colorHex: "3B82F6",
            role: "趋势交易者",
            tradingStyle: .trend
        ),
        GroupChatAgent(
            id: "quant",
            name: "量化派",
            icon: "function",
            colorHex: "F59E0B",
            role: "量化分析师",
            tradingStyle: .quant
        ),
    ]

    static func agent(for id: String) -> GroupChatAgent? {
        allAgents.first { $0.id == id }
    }

    static var orchestrator: GroupChatAgent { allAgents[0] }
}
