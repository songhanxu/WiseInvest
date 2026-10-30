import Foundation

struct InvestmentStock: Codable, Identifiable, Hashable {
    let id: String
    let symbol: String
    let name: String
    let market: String
    let currentPrice: Double?
    let changePercent: Double?
    let currency: String?
    let source: String?
    let quoteTime: String?
    let quoteAvailable: Bool?

    var key: String { "\(market):\(id)" }
    enum CodingKeys: String, CodingKey {
        case id, symbol, name, market, currency, source
        case currentPrice = "current_price", changePercent = "change_percent"
        case quoteTime = "quote_time", quoteAvailable = "quote_available"
    }
}

struct InvestmentMetric: Codable {
    let value: Double
    let unit: String
    let periodStart: String?
    let periodEnd: String?
    enum CodingKeys: String, CodingKey { case value, unit, periodStart = "period_start", periodEnd = "period_end" }
}

struct InvestmentReport: Codable, Identifiable {
    let form: String
    let kind: String?
    let filedDate: String
    let reportDate: String
    let accession: String
    let sourceURL: String
    let metrics: [String: InvestmentMetric]?
    var id: String { accession }
    var displayForm: String { kind == "earnings_release" ? "8-K 业绩公告" : form }
    enum CodingKeys: String, CodingKey {
        case form, kind, accession, metrics
        case filedDate = "filed_date", reportDate = "report_date", sourceURL = "source_url"
    }
}

struct InvestmentFundamentals: Codable {
    let eps: Double?
    let pe: Double?
    let forwardPE: Double?
    let nextEarningsDate: String?
    let status: String
    let message: String?
    let financialReports: [InvestmentReport]?
    enum CodingKeys: String, CodingKey {
        case eps, pe, status, message
        case forwardPE = "forward_pe", nextEarningsDate = "next_earnings_date"
        case financialReports = "financial_reports"
    }
}

struct InvestmentInsightSource: Codable, Identifiable {
    let label: String
    let url: String
    let date: String?
    var id: String { url }
}

struct InvestmentInsight: Codable {
    let code: String
    let status: String
    let accession: String?
    let reportDate: String?
    let generatedAt: String?
    let content: String?
    let message: String?
    let sources: [InvestmentInsightSource]?
    enum CodingKeys: String, CodingKey {
        case code, status, accession, content, message, sources
        case reportDate = "report_date", generatedAt = "generated_at"
    }
}

struct InvestmentInsightResponse: Codable { let insights: [String: InvestmentInsight] }
private struct InvestmentIgnoredResponse: Decodable {}

struct InvestmentAccount: Codable, Identifiable {
    let id: String
    let name: String
    let status: String
    let message: String?
}

struct InvestmentEquity: Codable, Identifiable {
    let accountID: String
    let amount: String
    let currency: String
    var id: String { accountID + currency }
    enum CodingKeys: String, CodingKey { case accountID = "account_id", amount, currency }
}

struct InvestmentHolding: Codable, Identifiable {
    let holdingID: String
    let accountID: String
    let accountName: String?
    let symbol: String?
    let name: String?
    let quantity: String
    let quantityUnit: String?
    let marketValue: String?
    let unrealizedPnL: String?
    let currency: String?
    let side: String?
    var id: String { holdingID }
    enum CodingKeys: String, CodingKey {
        case symbol, name, quantity, currency, side
        case holdingID = "holding_id", accountID = "account_id", accountName = "account_name"
        case quantityUnit = "quantity_unit", marketValue = "market_value"
        case unrealizedPnL = "unrealized_pnl"
    }
}

struct InvestmentSnapshot: Codable {
    let holdings: [InvestmentHolding]
    let accountEquities: [InvestmentEquity]?
    let warnings: [String]?
    let stale: Bool
    enum CodingKeys: String, CodingKey {
        case holdings, warnings, stale
        case accountEquities = "account_equities"
    }
}

struct InvestmentAlert: Codable, Identifiable {
    let holdingID: String
    let targetPrice: Double
    var id: String { holdingID }
    enum CodingKeys: String, CodingKey { case holdingID = "holding_id", targetPrice = "target_price" }
}

struct InvestmentAlertResponse: Codable {
    let alerts: [InvestmentAlert]
    let email: String
    let mailerConfigured: Bool
    enum CodingKeys: String, CodingKey { case alerts, email, mailerConfigured = "mailer_configured" }
}

enum InvestmentRequestError: LocalizedError {
    case unauthenticated
    case invalidResponse
    case server(String)
    var errorDescription: String? {
        switch self {
        case .unauthenticated: return "请先登录后查看投资数据"
        case .invalidResponse: return "服务器返回的数据无法读取"
        case .server(let message): return message
        }
    }
}

final class InvestmentDataService {
    static let shared = InvestmentDataService()
    private let session: URLSession
    private let decoder = JSONDecoder()

    private init() {
        let configuration = URLSessionConfiguration.default
        configuration.timeoutIntervalForRequest = 25
        configuration.timeoutIntervalForResource = 40
        session = URLSession(configuration: configuration)
    }

    func get<T: Decodable>(_ path: String, authenticated: Bool = true) async throws -> T {
        try await send(path, method: "GET", authenticated: authenticated)
    }

    func send<T: Decodable>(_ path: String, method: String, body: [String: Any]? = nil, authenticated: Bool = true) async throws -> T {
        guard let url = URL(string: APIConfig.baseURL + path) else { throw InvestmentRequestError.invalidResponse }
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let requestToken = authenticated ? AuthState.shared.token : nil
        if authenticated {
            guard let token = requestToken else { throw InvestmentRequestError.unauthenticated }
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        if let body { request.httpBody = try JSONSerialization.data(withJSONObject: body) }
        let (data, response) = try await session.data(for: request)
        guard let response = response as? HTTPURLResponse else { throw InvestmentRequestError.invalidResponse }
        if authenticated && response.statusCode == 401 {
            // A request started before a new login must not sign out the new session.
            await MainActor.run {
                if AuthState.shared.token == requestToken {
                    AuthState.shared.signOut()
                }
            }
            throw InvestmentRequestError.unauthenticated
        }
        guard (200..<300).contains(response.statusCode) else {
            let message = (try? JSONSerialization.jsonObject(with: data) as? [String: String])?["error"]
            throw InvestmentRequestError.server(message ?? "请求失败 (\(response.statusCode))")
        }
        do { return try decoder.decode(T.self, from: data) }
        catch { throw InvestmentRequestError.invalidResponse }
    }

    func watchlist() async throws -> [InvestmentStock] { try await get("/api/v1/stocks/watchlist/all") }
    func search(_ query: String, market: String) async throws -> [InvestmentStock] {
        if market == "all" {
            return await withTaskGroup(of: [InvestmentStock].self) { group in
                for market in ["a_share", "us_stock", "hk_stock", "crypto"] {
                    group.addTask { (try? await self.search(query, market: market)) ?? [] }
                }
                var rows: [InvestmentStock] = []
                for await result in group { rows.append(contentsOf: result) }
                return rows
            }
        }
        let q = query.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? query
        return try await get("/api/v1/stocks/search?q=\(q)&market=\(market)", authenticated: false)
    }
    func add(_ stock: InvestmentStock) async throws {
        let _: [String: String] = try await send("/api/v1/stocks/watchlist", method: "POST", body: [
            "stock_code": stock.id, "symbol": stock.symbol, "name": stock.name, "market": stock.market
        ])
    }
    func remove(_ stock: InvestmentStock) async throws {
        let _: [String: String] = try await send("/api/v1/stocks/watchlist", method: "DELETE", body: ["stock_code": stock.id, "market": stock.market])
    }
    func fundamentals(_ stock: InvestmentStock) async throws -> InvestmentFundamentals {
        let code = stock.id.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? stock.id
        return try await get("/api/v1/stocks/fundamentals?market=\(stock.market)&code=\(code)", authenticated: false)
    }
    func insights() async throws -> [String: InvestmentInsight] {
        let response: InvestmentInsightResponse = try await get("/api/v1/stocks/watchlist/filing-insights")
        return response.insights
    }
    func refreshInsight(code: String, retry: Bool = false, accession: String? = nil) async throws {
        var body: [String: Any] = ["code": code, "retry": retry]
        if let accession { body["accession"] = accession }
        let _: InvestmentIgnoredResponse = try await send("/api/v1/stocks/watchlist/filing-insights/refresh", method: "POST", body: body)
    }
    func refreshAllInsights() async throws {
        let _: InvestmentIgnoredResponse = try await send("/api/v1/stocks/watchlist/filing-insights/refresh", method: "POST", body: [:])
    }
    func accounts() async throws -> [InvestmentAccount] { try await get("/api/v1/accounts") }
    func holdings() async throws -> InvestmentSnapshot { try await get("/api/v1/accounts/all/holdings") }
    func alerts() async throws -> InvestmentAlertResponse { try await get("/api/v1/price-alerts") }
    func saveEmail(_ email: String) async throws {
        let _: [String: Bool] = try await send("/api/v1/price-alerts/settings", method: "PUT", body: ["email": email])
    }
    func saveAlert(holdingID: String, targetPrice: Double, accountID: String = "watchlist") async throws {
        let _: InvestmentAlert = try await send("/api/v1/price-alerts", method: "PUT", body: [
            "account_id": accountID, "holding_id": holdingID, "target_price": targetPrice, "direction": "auto"
        ])
    }
    func removeAlert(holdingID: String) async throws {
        let encoded = holdingID.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? holdingID
        let _: [String: Bool] = try await send("/api/v1/price-alerts?holding_id=\(encoded)", method: "DELETE")
    }
}
