import Foundation
import Combine
import SwiftUI

/// Real stock data service that fetches data from the backend API.
/// All watchlist operations are bound to the authenticated user's ID on the backend.
class StockDataService: ObservableObject {
    static let shared = StockDataService()

    private let baseURL: String
    private let session: URLSession
    /// Longer-timeout session for slow endpoints (news with LLM enhancement, etc.)
    private let longSession: URLSession

    private init() {
        self.baseURL = APIConfig.baseURL
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 3
        config.timeoutIntervalForResource = 6
        self.session = URLSession(configuration: config)

        let longConfig = URLSessionConfiguration.default
        longConfig.timeoutIntervalForRequest = 65
        longConfig.timeoutIntervalForResource = 120
        self.longSession = URLSession(configuration: longConfig)
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
        
        /// Returns available periods for a specific market
        /// - US stocks only support daily and weekly data due to API limitations
        /// - A-share: no 4h period (A-share trading hours = 4h/day, so 4h = 1d)
        /// - Crypto supports all periods including 4h
        static func availablePeriods(for market: String) -> [KLinePeriod] {
            switch market.lowercased() {
            case "us_stock":
                // US stocks: Sina API only provides daily data for individual stocks
                // US indices (via Tencent API) support daily and weekly
                return [.d1, .w1]
            case "a_share":
                // A-share: Sina API supports intraday periods but not 4h
                // (4h K-line = daily K-line due to 4h trading day)
                return [.m5, .m15, .m30, .h1, .d1, .w1]
            case "crypto":
                // Crypto (Binance) supports all periods including 4h
                return allCases
            default:
                return allCases
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

        longSession.dataTask(with: request) { data, response, error in
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
                        analysis: item.analysis ?? "",
                        sentiment: NewsSentiment(rawValue: item.sentiment) ?? .neutral,
                        url: item.url ?? ""
                    )
                }
                DispatchQueue.main.async { completion(news) }
            } catch {
                DispatchQueue.main.async { completion([]) }
            }
        }.resume()
    }

    // MARK: - News AI Enhancement (on-demand, called when user opens detail page)

    /// Fetches AI-enhanced summary + analysis for a single news item.
    /// Called when user taps into NewsDetailView.
    func enhanceNewsItem(newsID: String, code: String, market: String, name: String,
                         title: String, summary: String, source: String,
                         completion: @escaping (String, String, String) -> Void) {
        var components = URLComponents(string: "\(baseURL)/api/v1/stocks/news/enhance")
        components?.queryItems = [
            URLQueryItem(name: "news_id", value: newsID),
            URLQueryItem(name: "code", value: code),
            URLQueryItem(name: "market", value: market),
            URLQueryItem(name: "name", value: name),
            URLQueryItem(name: "title", value: title),
            URLQueryItem(name: "summary", value: summary),
            URLQueryItem(name: "source", value: source),
        ]
        guard let url = components?.url else {
            completion(summary, "", "neutral")
            return
        }

        var request = URLRequest(url: url)
        addAuthHeader(to: &request)

        longSession.dataTask(with: request) { data, response, error in
            guard let data = data, error == nil else {
                DispatchQueue.main.async { completion(summary, "", "neutral") }
                return
            }

            do {
                if let json = try JSONSerialization.jsonObject(with: data) as? [String: Any] {
                    let enhancedSummary = json["summary"] as? String ?? summary
                    let analysis = json["analysis"] as? String ?? ""
                    let sentiment = json["sentiment"] as? String ?? "neutral"
                    DispatchQueue.main.async { completion(enhancedSummary, analysis, sentiment) }
                } else {
                    DispatchQueue.main.async { completion(summary, "", "neutral") }
                }
            } catch {
                DispatchQueue.main.async { completion(summary, "", "neutral") }
            }
        }.resume()
    }

    // MARK: - Batch News AI Enhancement (triggered when entering a market section)

    /// Triggers batch AI summary+analysis enhancement for all watchlist stocks in a market.
    /// Called after news list returns when user enters a market section (e.g. A股).
    /// This is fire-and-forget — results are cached server-side and available on next fetch.
    func batchEnhanceNews(stocks: [(code: String, name: String, market: String)],
                          completion: (() -> Void)? = nil) {
        let urlString = "\(baseURL)/api/v1/stocks/news/enhance/batch"
        guard let url = URL(string: urlString) else {
            completion?()
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        addAuthHeader(to: &request)

        let body: [String: Any] = [
            "stocks": stocks.map { ["code": $0.code, "name": $0.name, "market": $0.market] }
        ]
        request.httpBody = try? JSONSerialization.data(withJSONObject: body)

        longSession.dataTask(with: request) { _, _, _ in
            DispatchQueue.main.async { completion?() }
        }.resume()
    }

    // MARK: - Batch News (parallel fetch for multiple stocks in one request)

    /// Fetches news for multiple stocks in a single batch request.
    /// The backend processes them in parallel and returns results for each stock.
    /// Completion is called with a dictionary keyed by "\(market)_\(code)".
    func getBatchNews(stocks: [(code: String, name: String, market: String)],
                      completion: @escaping ([String: [NewsItem]]) -> Void) {
        let urlString = "\(baseURL)/api/v1/stocks/news/batch"
        guard let url = URL(string: urlString) else {
            completion([:])
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        addAuthHeader(to: &request)

        let body: [String: Any] = [
            "stocks": stocks.map { ["code": $0.code, "name": $0.name, "market": $0.market] }
        ]
        request.httpBody = try? JSONSerialization.data(withJSONObject: body)

        longSession.dataTask(with: request) { data, response, error in
            guard let data = data, error == nil else {
                DispatchQueue.main.async { completion([:]) }
                return
            }

            do {
                let items = try JSONDecoder().decode([BatchNewsAPIResponse].self, from: data)
                var result: [String: [NewsItem]] = [:]
                for item in items {
                    let key = "\(item.market)_\(item.code)"
                    let news = item.news.map { n in
                        NewsItem(
                            id: n.id,
                            title: n.title,
                            source: n.source,
                            time: n.time,
                            summary: n.summary,
                            analysis: n.analysis ?? "",
                            sentiment: NewsSentiment(rawValue: n.sentiment) ?? .neutral,
                            url: n.url ?? ""
                        )
                    }
                    result[key] = news
                }
                DispatchQueue.main.async { completion(result) }
            } catch {
                DispatchQueue.main.async { completion([:]) }
            }
        }.resume()
    }

    // MARK: - AI Analysis Enhancement

    /// Fetches detailed AI analysis for a specific analysis type (tech/trend/volume).
    /// Uses POST to send kline/price summary in request body (avoids URL length limits).
    /// Triggers comprehensive AI analysis for a stock.
    /// The backend fetches multi-timeframe K-line data internally and generates a unified analysis
    /// combining technical, fundamental, and multi-timeframe trading advice.
    func enhanceAnalysis(code: String, market: String, name: String,
                         analysisType: String = "comprehensive",
                         klineSummary: String = "", priceSummary: String = "",
                         completion: @escaping (String, String, String) -> Void) {
        let urlString = "\(baseURL)/api/v1/stocks/analysis/enhance"
        guard let url = URL(string: urlString) else {
            print("[EnhanceAnalysis] Invalid URL: \(urlString)")
            completion("neutral", "", "")
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        addAuthHeader(to: &request)

        var body: [String: Any] = [
            "code": code,
            "market": market,
            "name": name,
        ]
        if !klineSummary.isEmpty { body["kline_summary"] = klineSummary }
        if !priceSummary.isEmpty { body["price_summary"] = priceSummary }
        request.httpBody = try? JSONSerialization.data(withJSONObject: body)

        print("[EnhanceAnalysis] POST \(urlString) code=\(code)")

        longSession.dataTask(with: request) { data, response, error in
            if let error = error {
                print("[EnhanceAnalysis] Network error: \(error.localizedDescription)")
                DispatchQueue.main.async { completion("neutral", "", "") }
                return
            }

            let httpStatus = (response as? HTTPURLResponse)?.statusCode ?? 0
            print("[EnhanceAnalysis] HTTP \(httpStatus)")

            guard let data = data else {
                print("[EnhanceAnalysis] No data received")
                DispatchQueue.main.async { completion("neutral", "", "") }
                return
            }

            // Debug: log raw response for troubleshooting
            if let rawStr = String(data: data, encoding: .utf8) {
                let preview = rawStr.count > 200 ? String(rawStr.prefix(200)) + "..." : rawStr
                print("[EnhanceAnalysis] Response: \(preview)")
            }

            do {
                if let json = try JSONSerialization.jsonObject(with: data) as? [String: Any] {
                    let conclusion = json["conclusion"] as? String ?? "neutral"
                    let summary = json["summary"] as? String ?? ""
                    let detail = json["detail"] as? String ?? ""
                    if detail.isEmpty {
                        print("[EnhanceAnalysis] WARNING: detail is empty, summary=\(summary)")
                    }
                    DispatchQueue.main.async { completion(conclusion, summary, detail) }
                } else {
                    print("[EnhanceAnalysis] Failed to parse JSON as [String: Any]")
                    DispatchQueue.main.async { completion("neutral", "", "") }
                }
            } catch {
                print("[EnhanceAnalysis] JSON parse error: \(error)")
                DispatchQueue.main.async { completion("neutral", "", "") }
            }
        }.resume()
    }

    /// Fetches overall bullish/bearish conclusion for a stock.
    func getAnalysisConclusion(code: String, market: String, name: String,
                               klineSummary: String, priceSummary: String,
                               completion: @escaping (String, String) -> Void) {
        var components = URLComponents(string: "\(baseURL)/api/v1/stocks/analysis/conclusion")
        components?.queryItems = [
            URLQueryItem(name: "code", value: code),
            URLQueryItem(name: "market", value: market),
            URLQueryItem(name: "name", value: name),
            URLQueryItem(name: "kline_summary", value: klineSummary),
            URLQueryItem(name: "price_summary", value: priceSummary),
        ]
        guard let url = components?.url else {
            completion("neutral", "")
            return
        }

        var request = URLRequest(url: url)
        addAuthHeader(to: &request)

        longSession.dataTask(with: request) { data, response, error in
            guard let data = data, error == nil else {
                DispatchQueue.main.async { completion("neutral", "") }
                return
            }

            do {
                if let json = try JSONSerialization.jsonObject(with: data) as? [String: Any] {
                    let conclusion = json["conclusion"] as? String ?? "neutral"
                    let summary = json["summary"] as? String ?? ""
                    DispatchQueue.main.async { completion(conclusion, summary) }
                } else {
                    DispatchQueue.main.async { completion("neutral", "") }
                }
            } catch {
                DispatchQueue.main.async { completion("neutral", "") }
            }
        }.resume()
    }

    // MARK: - Batch Analysis Conclusion (preload when entering a market section)

    struct AnalysisConclusionResult {
        let code: String
        let market: String
        let conclusion: String
        let summary: String
    }

    /// Fetches AI analysis conclusions for multiple stocks in one batch request.
    /// Called from PreloadManager when user enters a market section.
    func batchAnalysisConclusion(
        stocks: [(code: String, name: String, market: String, klineSummary: String, priceSummary: String)],
        completion: @escaping ([AnalysisConclusionResult]) -> Void
    ) {
        let urlString = "\(baseURL)/api/v1/stocks/analysis/batch-conclusion"
        guard let url = URL(string: urlString) else {
            completion([])
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        addAuthHeader(to: &request)

        let body: [String: Any] = [
            "stocks": stocks.map { [
                "code": $0.code,
                "name": $0.name,
                "market": $0.market,
                "kline_summary": $0.klineSummary,
                "price_summary": $0.priceSummary
            ] }
        ]
        request.httpBody = try? JSONSerialization.data(withJSONObject: body)

        longSession.dataTask(with: request) { data, response, error in
            guard let data = data, error == nil else {
                DispatchQueue.main.async { completion([]) }
                return
            }

            do {
                if let arr = try JSONSerialization.jsonObject(with: data) as? [[String: Any]] {
                    let results = arr.compactMap { item -> AnalysisConclusionResult? in
                        guard let code = item["code"] as? String,
                              let market = item["market"] as? String else { return nil }
                        return AnalysisConclusionResult(
                            code: code,
                            market: market,
                            conclusion: item["conclusion"] as? String ?? "neutral",
                            summary: item["summary"] as? String ?? ""
                        )
                    }
                    DispatchQueue.main.async { completion(results) }
                } else {
                    DispatchQueue.main.async { completion([]) }
                }
            } catch {
                DispatchQueue.main.async { completion([]) }
            }
        }.resume()
    }

    // MARK: - Batch Enhance Analysis (fire-and-forget preload)

    /// Triggers batch comprehensive AI analysis for multiple stocks.
    /// The backend fetches multi-timeframe K-line data internally and generates
    /// a single unified analysis per stock. Returns immediately with 202 status.
    func batchEnhanceAnalysis(
        stocks: [(code: String, name: String, market: String, klineSummary: String, priceSummary: String)],
        completion: (() -> Void)? = nil
    ) {
        let urlString = "\(baseURL)/api/v1/stocks/analysis/batch-enhance"
        guard let url = URL(string: urlString) else {
            completion?()
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        addAuthHeader(to: &request)

        let body: [String: Any] = [
            "stocks": stocks.map { [
                "code": $0.code,
                "name": $0.name,
                "market": $0.market,
                "kline_summary": $0.klineSummary,
                "price_summary": $0.priceSummary
            ] }
        ]
        request.httpBody = try? JSONSerialization.data(withJSONObject: body)

        print("[BatchEnhanceAnalysis] POST \(urlString) count=\(stocks.count)")

        // Fire-and-forget: we don't need the response content
        longSession.dataTask(with: request) { _, response, error in
            let httpStatus = (response as? HTTPURLResponse)?.statusCode ?? 0
            if let error = error {
                print("[BatchEnhanceAnalysis] Error: \(error.localizedDescription)")
            } else {
                print("[BatchEnhanceAnalysis] HTTP \(httpStatus)")
            }
            DispatchQueue.main.async { completion?() }
        }.resume()
    }

    /// Fetches the cached comprehensive analysis for a given stock from the backend.
    /// Returns a single (conclusion, summary, detail) result, or nil if not cached.
    func getCachedAnalysis(
        code: String, market: String,
        completion: @escaping ((conclusion: String, summary: String, detail: String)?) -> Void
    ) {
        var components = URLComponents(string: "\(baseURL)/api/v1/stocks/analysis/cached")
        components?.queryItems = [
            URLQueryItem(name: "code", value: code),
            URLQueryItem(name: "market", value: market),
        ]
        guard let url = components?.url else {
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
                if let json = try JSONSerialization.jsonObject(with: data) as? [String: Any],
                   let isCached = json["cached"] as? Bool, isCached,
                   let analysis = json["analysis"] as? [String: Any] {
                    let result = (
                        conclusion: analysis["conclusion"] as? String ?? "neutral",
                        summary: analysis["summary"] as? String ?? "",
                        detail: analysis["detail"] as? String ?? ""
                    )
                    DispatchQueue.main.async { completion(result) }
                } else {
                    DispatchQueue.main.async { completion(nil) }
                }
            } catch {
                DispatchQueue.main.async { completion(nil) }
            }
        }.resume()
    }
}

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
    let analysis: String?
    let sentiment: String
    let url: String?
}

private struct BatchNewsAPIResponse: Decodable {
    let code: String
    let market: String
    let news: [NewsAPIResponse]
}

// MARK: - AI Analysis Item (generated client-side from stock data — no separate API needed)

struct AIAnalysisItem: Identifiable, Hashable {
    let id: String
    let title: String
    let icon: String
    let content: String
    /// Detailed AI analysis (fetched from backend)
    var detail: String = ""
    /// Conclusion: bullish, bearish, neutral
    var conclusion: String = "neutral"
    /// Summary from AI (longer than content)
    var aiSummary: String = ""

    func hash(into hasher: inout Hasher) { hasher.combine(id) }
    static func == (lhs: AIAnalysisItem, rhs: AIAnalysisItem) -> Bool { lhs.id == rhs.id }
}

/// Overall conclusion aggregated from all analysis items
enum AnalysisConclusion: String {
    case bullish = "bullish"
    case bearish = "bearish"
    case neutral = "neutral"

    var label: String {
        switch self {
        case .bullish: return "看多"
        case .bearish: return "看空"
        case .neutral: return "中性"
        }
    }

    var icon: String {
        switch self {
        case .bullish: return "arrow.up.right.circle.fill"
        case .bearish: return "arrow.down.right.circle.fill"
        case .neutral: return "minus.circle.fill"
        }
    }
}
