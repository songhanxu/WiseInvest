import Foundation
import Combine
import SwiftUI

/// Real stock data service that fetches data from the backend API.
/// All watchlist operations are bound to the authenticated user's ID on the backend.
class StockDataService: ObservableObject {
    static let shared = StockDataService()

    private let baseURL: String
    private let session: URLSession

    private init() {
        self.baseURL = APIConfig.baseURL
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 3
        config.timeoutIntervalForResource = 6
        self.session = URLSession(configuration: config)
    }

    // MARK: - Auth Header

    private func addAuthHeader(to request: inout URLRequest) {
        if let token = AuthState.shared.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        // Skip ngrok browser warning page for free-tier tunnels
        request.setValue("true", forHTTPHeaderField: "ngrok-skip-browser-warning")
    }

    // MARK: - Market Indices

    func getIndices(for market: Market, completion: @escaping ([MarketIndex]) -> Void) {
        let urlString = "\(baseURL)/api/v1/stocks/indices?market=\(market.rawValue)"
        guard let url = URL(string: urlString) else {
            completion([])
            return
        }

        var request = URLRequest(url: url)
        addAuthHeader(to: &request)

        session.dataTask(with: request) { data, response, error in
            if error != nil {
                DispatchQueue.main.async { completion([]) }
                return
            }

            guard let data = data else {
                DispatchQueue.main.async { completion([]) }
                return
            }

            do {
                let items = try JSONDecoder().decode([IndexAPIResponse].self, from: data)
                let indices = items.map { item in
                    MarketIndex(
                        id: item.id,
                        name: item.name,
                        shortName: item.shortName,
                        value: item.value,
                        change: item.change,
                        changePercent: item.changePercent,
                        sparklineData: item.sparklineData
                    )
                }
                DispatchQueue.main.async { completion(indices) }
            } catch {
                DispatchQueue.main.async { completion([]) }
            }
        }.resume()
    }

    // MARK: - Watchlist (bound to UserID via backend)

    func getWatchlist(for market: Market, completion: @escaping ([Stock]) -> Void) {
        let urlString = "\(baseURL)/api/v1/stocks/watchlist?market=\(market.rawValue)"
        guard let url = URL(string: urlString) else {
            completion([])
            return
        }

        var request = URLRequest(url: url)
        addAuthHeader(to: &request)

        session.dataTask(with: request) { data, response, error in
            if error != nil {
                DispatchQueue.main.async { completion([]) }
                return
            }

            if let httpResponse = response as? HTTPURLResponse {
                if httpResponse.statusCode == 401 || httpResponse.statusCode != 200 {
                    DispatchQueue.main.async { completion([]) }
                    return
                }
            }

            guard let data = data else {
                DispatchQueue.main.async { completion([]) }
                return
            }

            do {
                let stocks = try JSONDecoder().decode([StockAPIResponse].self, from: data)
                let result = stocks.map { $0.toStock() }
                DispatchQueue.main.async { completion(result) }
            } catch {
                DispatchQueue.main.async { completion([]) }
            }
        }.resume()
    }

    func addToWatchlist(_ stock: Stock, for market: Market, completion: ((Bool) -> Void)? = nil) {
        let urlString = "\(baseURL)/api/v1/stocks/watchlist"
        guard let url = URL(string: urlString) else {
            completion?(false)
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        addAuthHeader(to: &request)

        let body: [String: Any] = [
            "stock_code": stock.id,
            "symbol": stock.symbol,
            "name": stock.name,
            "market": market.rawValue
        ]
        request.httpBody = try? JSONSerialization.data(withJSONObject: body)

        session.dataTask(with: request) { data, response, error in
            if error != nil {
                DispatchQueue.main.async { completion?(false) }
                return
            }

            let statusCode = (response as? HTTPURLResponse)?.statusCode ?? 0
            DispatchQueue.main.async { completion?(statusCode == 200) }
        }.resume()
    }

    func removeFromWatchlist(_ stockId: String, for market: Market, completion: ((Bool) -> Void)? = nil) {
        let urlString = "\(baseURL)/api/v1/stocks/watchlist"
        guard let url = URL(string: urlString) else {
            completion?(false)
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "DELETE"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        addAuthHeader(to: &request)

        let body: [String: Any] = [
            "stock_code": stockId,
            "market": market.rawValue
        ]
        request.httpBody = try? JSONSerialization.data(withJSONObject: body)

        session.dataTask(with: request) { data, response, error in
            if error != nil {
                DispatchQueue.main.async { completion?(false) }
                return
            }

            let statusCode = (response as? HTTPURLResponse)?.statusCode ?? 0
            DispatchQueue.main.async { completion?(statusCode == 200) }
        }.resume()
    }

    // MARK: - Search

    func searchStocks(query: String, market: Market, completion: @escaping ([Stock]) -> Void) {
        var urlString = "\(baseURL)/api/v1/stocks/search?market=\(market.rawValue)"
        if !query.isEmpty {
            urlString += "&q=\(query.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? query)"
        }
        guard let url = URL(string: urlString) else {
            completion([])
            return
        }

        var request = URLRequest(url: url)
        addAuthHeader(to: &request)

        session.dataTask(with: request) { data, response, error in
            guard let data = data, error == nil else {
                DispatchQueue.main.async { completion([]) }
                return
            }

            do {
                let stocks = try JSONDecoder().decode([StockAPIResponse].self, from: data)
                let result = stocks.map { $0.toStock() }
                DispatchQueue.main.async { completion(result) }
            } catch {
                DispatchQueue.main.async { completion([]) }
            }
        }.resume()
    }

    // MARK: - Stock Quote (single stock real-time data refresh)

    func getStockQuote(code: String, market: Market, completion: @escaping (Stock?) -> Void) {
        let urlString = "\(baseURL)/api/v1/stocks/quote?code=\(code)&market=\(market.rawValue)"
        guard let url = URL(string: urlString) else {
            completion(nil)
            return
        }

        var request = URLRequest(url: url)
        addAuthHeader(to: &request)

        session.dataTask(with: request) { data, response, error in
            guard let data = data, error == nil else {
                DispatchQueue.main.async { completion(nil) }
                return
            }

            do {
                let stock = try JSONDecoder().decode(StockAPIResponse.self, from: data)
                DispatchQueue.main.async { completion(stock.toStock()) }
            } catch {
                DispatchQueue.main.async { completion(nil) }
            }
        }.resume()
    }

    // MARK: - K-Line Data

    /// Supported K-line periods
    enum KLinePeriod: String, CaseIterable {
        case m5 = "5m"
        case m15 = "15m"
        case m30 = "30m"
        case h1 = "1h"
        case h4 = "4h"
        case d1 = "1d"
        case w1 = "1w"

        var label: String {
            switch self {
            case .m5: return "5分"
            case .m15: return "15分"
            case .m30: return "30分"
            case .h1: return "1时"
            case .h4: return "4时"
            case .d1: return "日K"
            case .w1: return "周K"
            }
        }
    }

    /// Default initial candle count per period
    static func defaultLimit(for period: KLinePeriod) -> Int {
        switch period {
        case .m5:  return 120
        case .m15: return 160
        case .m30: return 120
        case .h1:  return 120
        case .h4:  return 120
        case .d1:  return 150
        case .w1:  return 120
        }
    }

    /// Parse K-line API response items into KLinePoint array
    private func parseKLineItems(_ items: [KLineAPIResponse]) -> [KLinePoint] {
        let dayFormatter = DateFormatter()
        dayFormatter.dateFormat = "yyyy-MM-dd"
        let minuteFormatter = DateFormatter()
        minuteFormatter.dateFormat = "yyyy-MM-dd HH:mm"
        let sinaMinuteFormatter = DateFormatter()
        sinaMinuteFormatter.dateFormat = "yyyy-MM-dd HH:mm:ss"

        return items.compactMap { item -> KLinePoint? in
            let date = minuteFormatter.date(from: item.date)
                ?? sinaMinuteFormatter.date(from: item.date)
                ?? dayFormatter.date(from: item.date)
            guard let parsedDate = date else { return nil }
            return KLinePoint(
                date: parsedDate,
                open: item.open,
                close: item.close,
                high: item.high,
                low: item.low,
                volume: item.volume
            )
        }
    }

    /// Result type for K-line data fetch — includes optional error message from backend
    struct KLineResult {
        let points: [KLinePoint]
        let errorMessage: String?

        var isEmpty: Bool { points.isEmpty }
        var hasError: Bool { errorMessage != nil }
    }

    func getKLineData(for stock: Stock, period: KLinePeriod = .d1, limit: Int? = nil, completion: @escaping (KLineResult) -> Void) {
        let market = stock.market
        let effectiveLimit = limit ?? Self.defaultLimit(for: period)
        let urlString = "\(baseURL)/api/v1/stocks/kline?code=\(stock.id)&market=\(market)&period=\(period.rawValue)&limit=\(effectiveLimit)"
        guard let url = URL(string: urlString) else {
            completion(KLineResult(points: [], errorMessage: "无效的请求地址"))
            return
        }

        var request = URLRequest(url: url)
        addAuthHeader(to: &request)

        session.dataTask(with: request) { [weak self] data, response, error in
            guard let self = self else {
                DispatchQueue.main.async { completion(KLineResult(points: [], errorMessage: "服务不可用")) }
                return
            }

            if let error = error {
                let msg = (error as? URLError)?.code == .timedOut ? "网络请求超时" : "网络连接失败"
                DispatchQueue.main.async { completion(KLineResult(points: [], errorMessage: msg)) }
                return
            }

            guard let data = data else {
                DispatchQueue.main.async { completion(KLineResult(points: [], errorMessage: "未收到数据")) }
                return
            }

            // First try to decode as structured error response from backend
            // Format: {"error": "获取数据超时", "data": [], "retries": 3}
            if let errorResp = try? JSONDecoder().decode(KLineErrorResponse.self, from: data),
               errorResp.error != nil {
                DispatchQueue.main.async {
                    completion(KLineResult(points: [], errorMessage: errorResp.error))
                }
                return
            }

            // Normal response: array of K-line items
            do {
                let items = try JSONDecoder().decode([KLineAPIResponse].self, from: data)
                let points = self.parseKLineItems(items)
                DispatchQueue.main.async { completion(KLineResult(points: points, errorMessage: nil)) }
            } catch {
                DispatchQueue.main.async { completion(KLineResult(points: [], errorMessage: "数据解析失败")) }
            }
        }.resume()
    }

    /// Load more historical K-line data (larger limit). Returns ALL data from API.
    /// Caller is responsible for merging with existing data and deduplicating.
    func loadMoreKLineData(for stock: Stock, period: KLinePeriod, currentCount: Int, completion: @escaping (KLineResult) -> Void) {
        let market = stock.market
        // Request significantly more data than currently loaded (500 more for a large scroll buffer)
        let newLimit = currentCount + 500
        let urlString = "\(baseURL)/api/v1/stocks/kline?code=\(stock.id)&market=\(market)&period=\(period.rawValue)&limit=\(newLimit)"
        guard let url = URL(string: urlString) else {
            completion(KLineResult(points: [], errorMessage: "无效的请求地址"))
            return
        }

        var request = URLRequest(url: url)
        addAuthHeader(to: &request)

        session.dataTask(with: request) { [weak self] data, response, error in
            guard let self = self else {
                DispatchQueue.main.async { completion(KLineResult(points: [], errorMessage: nil)) }
                return
            }

            if error != nil {
                DispatchQueue.main.async { completion(KLineResult(points: [], errorMessage: nil)) }
                return
            }

            guard let data = data else {
                DispatchQueue.main.async { completion(KLineResult(points: [], errorMessage: nil)) }
                return
            }

            // Check for backend error response
            if let errorResp = try? JSONDecoder().decode(KLineErrorResponse.self, from: data),
               errorResp.error != nil {
                DispatchQueue.main.async { completion(KLineResult(points: [], errorMessage: errorResp.error)) }
                return
            }

            do {
                let items = try JSONDecoder().decode([KLineAPIResponse].self, from: data)
                let points = self.parseKLineItems(items)
                DispatchQueue.main.async { completion(KLineResult(points: points, errorMessage: nil)) }
            } catch {
                DispatchQueue.main.async { completion(KLineResult(points: [], errorMessage: nil)) }
            }
        }.resume()
    }

    // MARK: - News

    func getNews(for stock: Stock, completion: @escaping ([NewsItem]) -> Void) {
        let market = stock.market
        let name = stock.name.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? stock.name
        let urlString = "\(baseURL)/api/v1/stocks/news?code=\(stock.id)&market=\(market)&name=\(name)"
        guard let url = URL(string: urlString) else {
            completion([])
            return
        }

        var request = URLRequest(url: url)
        addAuthHeader(to: &request)

        session.dataTask(with: request) { data, response, error in
            guard let data = data, error == nil else {
                DispatchQueue.main.async { completion([]) }
                return
            }

            do {
                let items = try JSONDecoder().decode([NewsAPIResponse].self, from: data)
                let news = items.map { item in
                    NewsItem(
                        id: item.id,
                        title: item.title,
                        source: item.source,
                        time: item.time,
                        summary: item.summary,
                        sentiment: NewsSentiment(rawValue: item.sentiment) ?? .neutral
                    )
                }
                DispatchQueue.main.async { completion(news) }
            } catch {
                DispatchQueue.main.async { completion([]) }
            }
        }.resume()
    }
}

// MARK: - API Response Models

private struct IndexAPIResponse: Decodable {
    let id: String
    let name: String
    let shortName: String
    let value: Double
    let change: Double
    let changePercent: Double
    let sparklineData: [Double]

    enum CodingKeys: String, CodingKey {
        case id, name, value, change
        case shortName = "short_name"
        case changePercent = "change_percent"
        case sparklineData = "sparkline_data"
    }
}

private struct StockAPIResponse: Decodable {
    let id: String
    let symbol: String
    let name: String
    let market: String
    let currentPrice: Double
    let change: Double
    let changePercent: Double
    let volume: Double
    let high: Double
    let low: Double
    let open: Double
    let previousClose: Double

    enum CodingKeys: String, CodingKey {
        case id, symbol, name, market, change, volume, high, low, open
        case currentPrice = "current_price"
        case changePercent = "change_percent"
        case previousClose = "previous_close"
    }

    func toStock() -> Stock {
        Stock(
            id: id,
            symbol: symbol,
            name: name,
            market: market,
            currentPrice: currentPrice,
            change: change,
            changePercent: changePercent,
            volume: volume,
            high: high,
            low: low,
            open: open,
            previousClose: previousClose
        )
    }
}

private struct KLineAPIResponse: Decodable {
    let date: String
    let open: Double
    let close: Double
    let high: Double
    let low: Double
    let volume: Double
}

/// Backend error response for K-line data
/// Format: {"error": "获取数据超时", "data": [], "retries": 3}
private struct KLineErrorResponse: Decodable {
    let error: String?
    let retries: Int?
}

private struct NewsAPIResponse: Decodable {
    let id: String
    let title: String
    let source: String
    let time: String
    let summary: String
    let sentiment: String
}

// MARK: - AI Analysis Item (generated client-side from stock data — no separate API needed)

struct AIAnalysisItem: Identifiable {
    let id: String
    let title: String
    let icon: String
    let content: String
}
