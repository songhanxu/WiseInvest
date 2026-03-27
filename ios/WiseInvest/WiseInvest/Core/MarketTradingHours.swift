import Foundation

/// Determines whether a given market is currently in a trading session.
/// Used to control auto-refresh behavior: refresh frequently during trading hours,
/// pause during market close to save bandwidth and battery.
struct MarketTradingHours {

    // MARK: - Public API

    /// Returns `true` when the market is open and data should auto-refresh.
    static func isTradingNow(market: Market) -> Bool {
        switch market {
        case .aShare:  return isAShareTrading()
        case .usStock: return isUSStockTrading()
        case .crypto:  return true  // 24/7
        }
    }

    /// Suggested auto-refresh interval (in seconds) for each market.
    /// Short intervals during trading hours for near-real-time updates;
    /// nil during off-hours to stop refreshing entirely.
    static func refreshInterval(market: Market) -> TimeInterval? {
        switch market {
        case .aShare:
            return isAShareTrading() ? 1 : nil
        case .usStock:
            return isUSStockTrading() ? 2 : nil
        case .crypto:
            return 1  // always on (matches Binance WebSocket stream ~1s push interval)
        }
    }

    /// Returns a human-readable trading status string for display.
    static func tradingStatus(market: Market) -> TradingStatus {
        switch market {
        case .aShare:
            if isAShareTrading() {
                return .open("交易中")
            } else if isAShareLunchBreak() {
                return .lunchBreak("午间休市")
            } else {
                return .closed(nextOpenDescription(market: market))
            }
        case .usStock:
            if isUSStockTrading() {
                return .open("交易中")
            } else if isUSPreMarket() {
                return .preMarket("盘前交易")
            } else if isUSAfterHours() {
                return .afterHours("盘后交易")
            } else {
                return .closed(nextOpenDescription(market: market))
            }
        case .crypto:
            return .open("全天交易")
        }
    }

    enum TradingStatus {
        case open(String)
        case lunchBreak(String)
        case preMarket(String)
        case afterHours(String)
        case closed(String)

        var label: String {
            switch self {
            case .open(let s), .lunchBreak(let s), .preMarket(let s),
                 .afterHours(let s), .closed(let s):
                return s
            }
        }

        var isActive: Bool {
            switch self {
            case .open, .preMarket, .afterHours: return true
            case .lunchBreak, .closed: return false
            }
        }
    }

    // MARK: - A-Share (Shanghai / Shenzhen)
    // Trading hours: Mon–Fri 09:30–11:30, 13:00–15:00 CST (UTC+8)
    // No trading on weekends and Chinese public holidays (holidays not modeled here)

    private static func isAShareTrading() -> Bool {
        let cal = Calendar.current
        let now = Date()
        guard let cst = TimeZone(identifier: "Asia/Shanghai") else { return false }
        var cstCal = cal
        cstCal.timeZone = cst
        let comps = cstCal.dateComponents([.weekday, .hour, .minute], from: now)
        guard let wd = comps.weekday, let h = comps.hour, let m = comps.minute else { return false }

        // Mon(2) – Fri(6) only
        guard wd >= 2 && wd <= 6 else { return false }

        let minuteOfDay = h * 60 + m
        // Morning: 09:30 – 11:30
        let morningOpen = 9 * 60 + 30
        let morningClose = 11 * 60 + 30
        // Afternoon: 13:00 – 15:00
        let afternoonOpen = 13 * 60
        let afternoonClose = 15 * 60

        return (minuteOfDay >= morningOpen && minuteOfDay < morningClose)
            || (minuteOfDay >= afternoonOpen && minuteOfDay < afternoonClose)
    }

    private static func isAShareLunchBreak() -> Bool {
        let cal = Calendar.current
        let now = Date()
        guard let cst = TimeZone(identifier: "Asia/Shanghai") else { return false }
        var cstCal = cal
        cstCal.timeZone = cst
        let comps = cstCal.dateComponents([.weekday, .hour, .minute], from: now)
        guard let wd = comps.weekday, let h = comps.hour, let m = comps.minute else { return false }
        guard wd >= 2 && wd <= 6 else { return false }

        let minuteOfDay = h * 60 + m
        // Lunch break: 11:30 – 13:00
        return minuteOfDay >= 11 * 60 + 30 && minuteOfDay < 13 * 60
    }

    // MARK: - US Stock (NYSE / NASDAQ)
    // Regular trading: Mon–Fri 09:30–16:00 ET (Eastern Time)
    // Pre-market: 04:00–09:30 ET
    // After-hours: 16:00–20:00 ET

    private static func isUSStockTrading() -> Bool {
        let cal = Calendar.current
        let now = Date()
        guard let et = TimeZone(identifier: "America/New_York") else { return false }
        var etCal = cal
        etCal.timeZone = et
        let comps = etCal.dateComponents([.weekday, .hour, .minute], from: now)
        guard let wd = comps.weekday, let h = comps.hour, let m = comps.minute else { return false }
        guard wd >= 2 && wd <= 6 else { return false }

        let minuteOfDay = h * 60 + m
        // Regular: 09:30 – 16:00
        return minuteOfDay >= 9 * 60 + 30 && minuteOfDay < 16 * 60
    }

    private static func isUSPreMarket() -> Bool {
        let cal = Calendar.current
        let now = Date()
        guard let et = TimeZone(identifier: "America/New_York") else { return false }
        var etCal = cal
        etCal.timeZone = et
        let comps = etCal.dateComponents([.weekday, .hour, .minute], from: now)
        guard let wd = comps.weekday, let h = comps.hour, let m = comps.minute else { return false }
        guard wd >= 2 && wd <= 6 else { return false }

        let minuteOfDay = h * 60 + m
        // Pre-market: 04:00 – 09:30
        return minuteOfDay >= 4 * 60 && minuteOfDay < 9 * 60 + 30
    }

    private static func isUSAfterHours() -> Bool {
        let cal = Calendar.current
        let now = Date()
        guard let et = TimeZone(identifier: "America/New_York") else { return false }
        var etCal = cal
        etCal.timeZone = et
        let comps = etCal.dateComponents([.weekday, .hour, .minute], from: now)
        guard let wd = comps.weekday, let h = comps.hour, let m = comps.minute else { return false }
        guard wd >= 2 && wd <= 6 else { return false }

        let minuteOfDay = h * 60 + m
        // After-hours: 16:00 – 20:00
        return minuteOfDay >= 16 * 60 && minuteOfDay < 20 * 60
    }

    // MARK: - Helpers

    private static func nextOpenDescription(market: Market) -> String {
        switch market {
        case .aShare:
            return "已收盘"
        case .usStock:
            return "已收盘"
        case .crypto:
            return "全天交易"
        }
    }
}
