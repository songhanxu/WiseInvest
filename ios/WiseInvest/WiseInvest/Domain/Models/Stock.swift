import Foundation

// MARK: - Stock Model

struct Stock: Identifiable, Codable, Hashable {
    let id: String           // 股票代码，如 "600519", "AAPL", "BTC"
    let symbol: String       // 交易所代码，如 "SH600519"
    let name: String         // 名称
    let market: String       // 所属市场 raw value
    var currentPrice: Double
    var change: Double       // 涨跌额
    var changePercent: Double // 涨跌幅 %
    var volume: Double       // 成交量（亿）
    var high: Double
    var low: Double
    var open: Double
    var previousClose: Double
    var isIndex: Bool        // 是否为大盘指数（指数不可加入自选）

    enum CodingKeys: String, CodingKey {
        case id, symbol, name, market, currentPrice, change, changePercent
        case volume, high, low, open, previousClose, isIndex
    }

    /// Default initializer with isIndex defaulting to false
    init(id: String, symbol: String, name: String, market: String,
         currentPrice: Double, change: Double, changePercent: Double,
         volume: Double, high: Double, low: Double, open: Double,
         previousClose: Double, isIndex: Bool = false) {
        self.id = id
        self.symbol = symbol
        self.name = name
        self.market = market
        self.currentPrice = currentPrice
        self.change = change
        self.changePercent = changePercent
        self.volume = volume
        self.high = high
        self.low = low
        self.open = open
        self.previousClose = previousClose
        self.isIndex = isIndex
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        symbol = try c.decode(String.self, forKey: .symbol)
        name = try c.decode(String.self, forKey: .name)
        market = try c.decode(String.self, forKey: .market)
        currentPrice = try c.decode(Double.self, forKey: .currentPrice)
        change = try c.decode(Double.self, forKey: .change)
        changePercent = try c.decode(Double.self, forKey: .changePercent)
        volume = try c.decode(Double.self, forKey: .volume)
        high = try c.decode(Double.self, forKey: .high)
        low = try c.decode(Double.self, forKey: .low)
        open = try c.decode(Double.self, forKey: .open)
        previousClose = try c.decode(Double.self, forKey: .previousClose)
        isIndex = try c.decodeIfPresent(Bool.self, forKey: .isIndex) ?? false
    }

    var isUp: Bool { change >= 0 }

    var changePercentText: String {
        let sign = changePercent >= 0 ? "+" : ""
        return "\(sign)\(String(format: "%.2f", changePercent))%"
    }

    var changeText: String {
        let sign = change >= 0 ? "+" : ""
        return "\(sign)\(String(format: "%.2f", change))"
    }

    var priceText: String {
        if currentPrice >= 1000 {
            return String(format: "%.0f", currentPrice)
        } else if currentPrice >= 1 {
            return String(format: "%.2f", currentPrice)
        } else {
            return String(format: "%.4f", currentPrice)
        }
    }
}

// MARK: - Market Index

struct MarketIndex: Identifiable, Hashable {
    let id: String
    let name: String
    let shortName: String
    var value: Double
    var change: Double
    var changePercent: Double
    var sparklineData: [Double] // 最近若干点的走势数据

    var isUp: Bool { change >= 0 }

    var valueText: String {
        String(format: "%.2f", value)
    }

    var changePercentText: String {
        let sign = changePercent >= 0 ? "+" : ""
        return "\(sign)\(String(format: "%.2f", changePercent))%"
    }
}

// MARK: - K-Line Data Point

struct KLinePoint: Identifiable {
    let id = UUID()
    let date: Date
    let open: Double
    let close: Double
    let high: Double
    let low: Double
    let volume: Double

    var isUp: Bool { close >= open }
}

// MARK: - News Item

struct NewsItem: Identifiable {
    let id: String
    let title: String
    let source: String
    let time: String
    let summary: String
    let sentiment: NewsSentiment
}

enum NewsSentiment: String {
    case positive = "positive"
    case negative = "negative"
    case neutral  = "neutral"

    var label: String {
        switch self {
        case .positive: return "利好"
        case .negative: return "利空"
        case .neutral:  return "中性"
        }
    }
}
