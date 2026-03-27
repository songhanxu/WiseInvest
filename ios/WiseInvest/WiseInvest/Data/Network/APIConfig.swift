import Foundation

/// Backend API configuration
enum APIConfig {

    // MARK: - 📝 在这里修改配置

    /// 本地局域网模式：Mac 的 IP 地址（真机同 Wi-Fi 时使用）
    /// 运行 `ipconfig getifaddr en0` 查看当前 IP
    static let localDeviceHost = "30.22.158.12"
    static let localPort = 8080

    /// 公网模式：Cloudflare Tunnel 地址（手机随处可用）
    /// 格式：https://<tunnel-id>.cfargotunnel.com
    /// 创建方式见 WiseInvest/start-tunnel.sh
    static let tunnelURL: String? = "http://119.91.123.144"
  // 腾讯云服务器公网地址

    // MARK: - 自动选择 URL（无需手动修改）

    static var baseURL: String {
        // 如果配置了 Tunnel URL，优先使用（真机随处可用）
        if let tunnel = tunnelURL, !tunnel.isEmpty {
            return tunnel
        }
        // 模拟器始终用 localhost
        #if targetEnvironment(simulator)
        return "http://localhost:\(localPort)"
        #else
        // 真机：同一 Wi-Fi 下用局域网 IP
        return "http://\(localDeviceHost):\(localPort)"
        #endif
    }

    /// WebSocket URL — auto-derived from baseURL (http→ws, https→wss)
    static var wsURL: String {
        let base = baseURL
        if base.hasPrefix("https://") {
            return "wss://" + base.dropFirst("https://".count) + "/api/v1/ws"
        } else if base.hasPrefix("http://") {
            return "ws://" + base.dropFirst("http://".count) + "/api/v1/ws"
        }
        return "ws://" + base + "/api/v1/ws"
    }
}
