import Foundation
import Combine

// MARK: - WebSocket Message Protocol

/// Envelope for all WebSocket messages (mirrors backend WSMessage).
struct WSMessage: Codable {
    let type: String
    var channel: String?
    var market: String?
    var code: String?
    var period: String?
    var params: [String: String]?
    var data: AnyCodable?  // Generic data payload
}

/// Type-erased Codable wrapper for heterogeneous JSON payloads.
struct AnyCodable: Codable {
    let value: Any

    init(_ value: Any) { self.value = value }

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if let arr = try? container.decode([IndexAPIWSResponse].self) {
            value = arr; return
        }
        if let obj = try? container.decode(StockAPIWSResponse.self) {
            value = obj; return
        }
        if let str = try? container.decode(String.self) {
            value = str; return
        }
        value = NSNull()
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        if let v = value as? [IndexAPIWSResponse] { try container.encode(v) }
        else if let v = value as? StockAPIWSResponse { try container.encode(v) }
        else if let v = value as? String { try container.encode(v) }
        else { try container.encodeNil() }
    }
}

// MARK: - WebSocket Response Models (match backend JSON)

struct IndexAPIWSResponse: Codable {
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

    func toMarketIndex() -> MarketIndex {
        MarketIndex(
            id: id, name: name, shortName: shortName,
            value: value, change: change, changePercent: changePercent,
            sparklineData: sparklineData
        )
    }
}

struct StockAPIWSResponse: Codable {
    let id: String
    let symbol: String
    let name: String
    let market: String
    let currentPrice: Double
    let change: Double
    let changePercent: Double
    let volume: Double?
    let high: Double?
    let low: Double?
    let open: Double?
    let previousClose: Double?

    enum CodingKeys: String, CodingKey {
        case id, symbol, name, market, change, volume, high, low, open
        case currentPrice = "current_price"
        case changePercent = "change_percent"
        case previousClose = "previous_close"
    }

    func toStock() -> Stock {
        Stock(
            id: id, symbol: symbol, name: name, market: market,
            currentPrice: currentPrice, change: change,
            changePercent: changePercent, volume: volume ?? 0,
            high: high ?? currentPrice, low: low ?? currentPrice,
            open: open ?? currentPrice,
            previousClose: previousClose ?? (currentPrice - change)
        )
    }
}

// MARK: - Raw WebSocket Payload (for manual JSON parsing)

/// Lightweight struct to decode the outer envelope only;
/// `data` stays as raw JSON for type-specific parsing.
private struct RawWSPayload: Decodable {
    let type: String
    let market: String?
    let code: String?
    let period: String?
    let data: Data?  // raw JSON bytes

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        type = try container.decode(String.self, forKey: .type)
        market = try container.decodeIfPresent(String.self, forKey: .market)
        code = try container.decodeIfPresent(String.self, forKey: .code)
        period = try container.decodeIfPresent(String.self, forKey: .period)
        // Grab raw JSON for `data` field
        if container.contains(.data) {
            let rawData = try container.decode(RawJSON.self, forKey: .data)
            data = rawData.rawData
        } else {
            data = nil
        }
    }

    enum CodingKeys: String, CodingKey {
        case type, market, code, period, data
    }
}

/// Helper to capture raw JSON bytes for a field without parsing.
private struct RawJSON: Decodable {
    let rawData: Data

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        // Decode as generic JSON structure then re-encode to Data
        let jsonValue = try container.decode(JSONValue.self)
        rawData = try JSONEncoder().encode(jsonValue)
    }
}

/// A generic JSON value for roundtrip encoding.
private enum JSONValue: Codable {
    case string(String)
    case number(Double)
    case bool(Bool)
    case array([JSONValue])
    case object([String: JSONValue])
    case null

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if let v = try? container.decode(Bool.self) { self = .bool(v); return }
        if let v = try? container.decode(Double.self) { self = .number(v); return }
        if let v = try? container.decode(String.self) { self = .string(v); return }
        if let v = try? container.decode([JSONValue].self) { self = .array(v); return }
        if let v = try? container.decode([String: JSONValue].self) { self = .object(v); return }
        if container.decodeNil() { self = .null; return }
        throw DecodingError.dataCorruptedError(in: container, debugDescription: "Cannot decode JSON value")
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .string(let v): try container.encode(v)
        case .number(let v): try container.encode(v)
        case .bool(let v):   try container.encode(v)
        case .array(let v):  try container.encode(v)
        case .object(let v): try container.encode(v)
        case .null:          try container.encodeNil()
        }
    }
}

// MARK: - WebSocket Event

/// Events published by WebSocketClient for subscribers.
enum WSEvent {
    /// Indices updated for a market
    case indices(market: String, data: [MarketIndex])
    /// Individual stock quote updated
    case quote(market: String, code: String, data: Stock)
    /// Connection status changed
    case connected
    case disconnected
}

// MARK: - WebSocketClient

/// Singleton WebSocket client for real-time market data.
/// Uses `URLSessionWebSocketTask` (iOS 13+) with automatic reconnection and heartbeat.
class WebSocketClient: ObservableObject {
    static let shared = WebSocketClient()

    /// Publisher for all WebSocket events
    let eventPublisher = PassthroughSubject<WSEvent, Never>()

    @Published private(set) var isConnected = false

    /// Thread-safe internal flag (read/write from any queue)
    private var _connected: Bool = false
    private let _connectedLock = NSLock()
    private var connectedFlag: Bool {
        get { _connectedLock.lock(); defer { _connectedLock.unlock() }; return _connected }
        set {
            _connectedLock.lock(); _connected = newValue; _connectedLock.unlock()
            DispatchQueue.main.async { [weak self] in self?.isConnected = newValue }
        }
    }

    private var webSocketTask: URLSessionWebSocketTask?
    private var session: URLSession!
    private var pingTimer: Timer?
    private var reconnectTimer: Timer?
    private var reconnectDelay: TimeInterval = 1.0
    private let maxReconnectDelay: TimeInterval = 30.0

    /// Monotonically increasing ID so that callbacks from stale connections are ignored.
    private var connectionID: UInt64 = 0

    /// Subscriptions to restore after reconnect
    private var activeSubscriptions: Set<String> = [] // Serialized JSON subscribe messages

    private let decoder = JSONDecoder()
    private let queue = DispatchQueue(label: "com.wiseinvest.websocket", qos: .userInitiated)

    private init() {
        let config = URLSessionConfiguration.default
        config.waitsForConnectivity = true
        session = URLSession(configuration: config)
    }

    // MARK: - Connect / Disconnect

    func connect() {
        queue.async { [weak self] in
            self?._connect()
        }
    }

    private func _connect() {
        // Close existing connection if any
        webSocketTask?.cancel(with: .goingAway, reason: nil)
        webSocketTask = nil
        connectedFlag = false

        // Bump connection ID so stale callbacks are ignored
        connectionID &+= 1
        let thisConnectionID = connectionID

        let wsURL = Self.webSocketURL()
        guard let url = URL(string: wsURL) else {
            print("[WS] Invalid WebSocket URL: \(wsURL)")
            return
        }

        print("[WS] Connecting to \(wsURL)")

        var request = URLRequest(url: url)
        request.setValue("true", forHTTPHeaderField: "ngrok-skip-browser-warning")

        let task = session.webSocketTask(with: request)
        webSocketTask = task
        task.resume()

        // Send an initial ping to verify the connection is actually open.
        // Only after the pong comes back do we start receiveMessage and mark connected.
        task.sendPing { [weak self] error in
            guard let self = self else { return }
            self.queue.async {
                // Guard against stale callback from a previous connection attempt
                guard self.connectionID == thisConnectionID else { return }

                if let error = error {
                    print("[WS] Initial ping failed: \(error.localizedDescription)")
                    self.handleConnectionFailure()
                    return
                }
                // Connection confirmed — now safe to read, heartbeat, & restore subs
                print("[WS] Connected successfully")
                self.connectedFlag = true
                self.reconnectDelay = 1.0
                DispatchQueue.main.async {
                    self.startPingTimer()
                }
                self.eventPublisher.send(.connected)

                // Start reading messages ONLY after connection is confirmed
                self.receiveMessage(expectedConnectionID: thisConnectionID)

                // Restore active subscriptions
                for subMsg in self.activeSubscriptions {
                    self.sendRaw(subMsg)
                }
            }
        }
    }

    /// Handle a connection failure (initial ping failed or immediate disconnect).
    private func handleConnectionFailure() {
        webSocketTask?.cancel(with: .goingAway, reason: nil)
        webSocketTask = nil
        connectedFlag = false
        eventPublisher.send(.disconnected)
        scheduleReconnect()
    }

    func disconnect() {
        queue.async { [weak self] in
            guard let self = self else { return }
            // Bump connectionID to invalidate any in-flight callbacks
            self.connectionID &+= 1
            DispatchQueue.main.async { self.stopPingTimer() }
            self.reconnectTimer?.invalidate()
            self.reconnectTimer = nil
            self.webSocketTask?.cancel(with: .goingAway, reason: nil)
            self.webSocketTask = nil
            self.connectedFlag = false
            self.eventPublisher.send(.disconnected)
        }
    }

    // MARK: - Subscribe / Unsubscribe

    /// Subscribe to indices updates for a market (e.g. "a_share", "crypto")
    func subscribeIndices(market: String) {
        let msg = """
        {"type":"subscribe","channel":"indices","params":{"market":"\(market)"}}
        """
        activeSubscriptions.insert(msg)
        sendRaw(msg)
    }

    /// Unsubscribe from indices updates
    func unsubscribeIndices(market: String) {
        let subMsg = """
        {"type":"subscribe","channel":"indices","params":{"market":"\(market)"}}
        """
        let unsubMsg = """
        {"type":"unsubscribe","channel":"indices","params":{"market":"\(market)"}}
        """
        activeSubscriptions.remove(subMsg)
        sendRaw(unsubMsg)
    }

    /// Subscribe to a single stock quote (e.g. market="crypto", code="BTC")
    func subscribeQuote(market: String, code: String) {
        let msg = """
        {"type":"subscribe","channel":"quote","params":{"market":"\(market)","code":"\(code)"}}
        """
        activeSubscriptions.insert(msg)
        sendRaw(msg)
    }

    /// Unsubscribe from a stock quote
    func unsubscribeQuote(market: String, code: String) {
        let subMsg = """
        {"type":"subscribe","channel":"quote","params":{"market":"\(market)","code":"\(code)"}}
        """
        let unsubMsg = """
        {"type":"unsubscribe","channel":"quote","params":{"market":"\(market)","code":"\(code)"}}
        """
        activeSubscriptions.remove(subMsg)
        sendRaw(unsubMsg)
    }

    /// Unsubscribe from all channels (e.g. when leaving a view)
    func unsubscribeAll() {
        for sub in activeSubscriptions {
            // Convert subscribe to unsubscribe
            let unsub = sub.replacingOccurrences(of: "\"subscribe\"", with: "\"unsubscribe\"")
            sendRaw(unsub)
        }
        activeSubscriptions.removeAll()
    }

    // MARK: - WebSocket URL

    private static func webSocketURL() -> String {
        let base = APIConfig.baseURL
        // Convert http:// → ws://, https:// → wss://
        let wsBase: String
        if base.hasPrefix("https://") {
            wsBase = "wss://" + base.dropFirst("https://".count)
        } else if base.hasPrefix("http://") {
            wsBase = "ws://" + base.dropFirst("http://".count)
        } else {
            wsBase = "ws://" + base
        }
        return wsBase + "/api/v1/ws"
    }

    // MARK: - Send

    private func sendRaw(_ text: String) {
        guard let task = webSocketTask, connectedFlag else { return }
        task.send(.string(text)) { error in
            if let error = error {
                print("[WS] Send error: \(error.localizedDescription)")
            }
        }
    }

    // MARK: - Receive

    private func receiveMessage(expectedConnectionID: UInt64) {
        webSocketTask?.receive { [weak self] result in
            guard let self = self else { return }
            self.queue.async {
                // Ignore callbacks from stale connections
                guard self.connectionID == expectedConnectionID else { return }

                switch result {
                case .success(let message):
                    switch message {
                    case .string(let text):
                        self.handleTextMessage(text)
                    case .data(let data):
                        if let text = String(data: data, encoding: .utf8) {
                            self.handleTextMessage(text)
                        }
                    @unknown default:
                        break
                    }
                    // Continue reading (same connectionID)
                    self.receiveMessage(expectedConnectionID: expectedConnectionID)

                case .failure(let error):
                    print("[WS] Receive error: \(error.localizedDescription)")
                    self.connectedFlag = false
                    self.eventPublisher.send(.disconnected)
                    self.scheduleReconnect()
                }
            }
        }
    }

    private func handleTextMessage(_ text: String) {
        guard let data = text.data(using: .utf8) else { return }

        // Parse outer envelope
        guard let payload = try? decoder.decode(RawWSPayload.self, from: data) else { return }

        switch payload.type {
        case "pong":
            // Heartbeat response — no action needed
            break

        case "indices":
            guard let rawData = payload.data,
                  let market = payload.market,
                  let items = try? decoder.decode([IndexAPIWSResponse].self, from: rawData) else { return }
            let indices = items.map { $0.toMarketIndex() }
            eventPublisher.send(.indices(market: market, data: indices))

        case "quote":
            guard let rawData = payload.data,
                  let market = payload.market,
                  let code = payload.code,
                  let item = try? decoder.decode(StockAPIWSResponse.self, from: rawData) else { return }
            eventPublisher.send(.quote(market: market, code: code, data: item.toStock()))

        default:
            break
        }
    }

    // MARK: - Heartbeat (Ping/Pong)

    private func startPingTimer() {
        stopPingTimer()
        pingTimer = Timer.scheduledTimer(withTimeInterval: 25, repeats: true) { [weak self] _ in
            self?.sendRaw("{\"type\":\"ping\"}")
        }
    }

    private func stopPingTimer() {
        pingTimer?.invalidate()
        pingTimer = nil
    }

    // MARK: - Auto Reconnect

    private func scheduleReconnect() {
        DispatchQueue.main.async { [weak self] in
            guard let self = self else { return }
            self.stopPingTimer()
            self.reconnectTimer?.invalidate()

            let delay = self.reconnectDelay
            print("[WS] Reconnecting in \(delay)s ...")
            self.reconnectTimer = Timer.scheduledTimer(withTimeInterval: delay, repeats: false) { [weak self] _ in
                self?.connect()
            }

            // Exponential backoff: 1s → 2s → 4s → 8s → ... → 30s max
            self.reconnectDelay = min(self.reconnectDelay * 2, self.maxReconnectDelay)
        }
    }
}
