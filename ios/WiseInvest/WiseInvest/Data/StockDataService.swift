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
        config.timeoutIntervalForRequest = 15
        config.timeoutIntervalForResource = 30
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
        print("[StockDataService] getIndices URL: \(urlString)")
        guard let url = URL(string: urlString) else {
            print("[StockDataService] ERROR: Invalid URL")
            completion([])
            return
        }

        var request = URLRequest(url: url)
        addAuthHeader(to: &request)

        session.dataTask(with: request) { data, response, error in
            if let error = error {
                print("[StockDataService] getIndices NETWORK ERROR: \(error.localizedDescription)")
                DispatchQueue.main.async { completion([]) }
                return
            }
            
            if let httpResponse = response as? HTTPURLResponse {
                print("[StockDataService] getIndices HTTP status: \(httpResponse.statusCode)")
            }
            
            guard let data = data else {
                print("[StockDataService] getIndices ERROR: No data")
                DispatchQueue.main.async { completion([]) }
                return
            }
            
            // Print raw response for debugging
            let rawString = String(data: data, encoding: .utf8) ?? "nil"
            print("[StockDataService] getIndices raw response (\(data.count) bytes): \(String(rawString.prefix(500)))")

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
                print("[StockDataService] getIndices decoded \(indices.count) indices OK")
                DispatchQueue.main.async { completion(indices) }
            } catch {
                print("[StockDataService] Failed to decode indices: \(error)")
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
            guard let data = data, error == nil else {
                DispatchQueue.main.async { completion([]) }
                return
            }

            do {
                let stocks = try JSONDecoder().decode([StockAPIResponse].self, from: data)
                let result = stocks.map { $0.toStock() }
                DispatchQueue.main.async { completion(result) }
            } catch {
                print("[StockDataService] Failed to decode watchlist: \(error)")
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

        session.dataTask(with: request) { _, response, error in
            let success = error == nil
            DispatchQueue.main.async { completion?(success) }
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

        session.dataTask(with: request) { _, response, error in
            let success = error == nil
            DispatchQueue.main.async { completion?(success) }
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
                print("[StockDataService] Failed to decode search results: \(error)")
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

    func getKLineData(for stock: Stock, days: Int = 60, completion: @escaping ([KLinePoint]) -> Void) {
        let market = stock.market
        let urlString = "\(baseURL)/api/v1/stocks/kline?code=\(stock.id)&market=\(market)&days=\(days)"
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
                let items = try JSONDecoder().decode([KLineAPIResponse].self, from: data)
                let formatter = DateFormatter()
                formatter.dateFormat = "yyyy-MM-dd"

                let points = items.compactMap { item -> KLinePoint? in
                    guard let date = formatter.date(from: item.date) else { return nil }
                    return KLinePoint(
                        date: date,
                        open: item.open,
                        close: item.close,
                        high: item.high,
                        low: item.low,
                        volume: item.volume
                    )
                }
                DispatchQueue.main.async { completion(points) }
            } catch {
                print("[StockDataService] Failed to decode K-line data: \(error)")
                DispatchQueue.main.async { completion([]) }
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
                print("[StockDataService] Failed to decode news: \(error)")
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
