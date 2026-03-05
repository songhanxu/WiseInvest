import Foundation
import UIKit

// ─────────────────────────────────────────────────────────────────────────────
// WeChatManager
//
// 集成步骤（首次接入）:
// 1. 前往 https://open.weixin.qq.com 注册移动应用，获取 AppID + AppSecret
// 2. 下载官方 SDK: https://developers.weixin.qq.com/doc/oplatform/Downloads/iOS.html
//    将 WechatOpenSDK.xcframework 拖入 Xcode 项目 → Embed & Sign
// 3. 将 Info.plist 里的 wx_YOUR_APP_ID 替换为真实 AppID
// 4. 将下方 wechatAppID 替换为真实 AppID，universalLink 替换为你的域名
// 5. 解注释所有 /* SDK */ 注释块中的代码
// 6. 在 Xcode → Signing & Capabilities 添加 Associated Domains:
//    applinks:your-domain.com  （Universal Link，SDK 1.8.6+ 必须）
// ─────────────────────────────────────────────────────────────────────────────

final class WeChatManager: NSObject {
    static let shared = WeChatManager()

    // ── 配置（填入真实值后解注释 SDK 代码）────────────────────────────────
    let wechatAppID    = "wx_YOUR_APP_ID"
    let universalLink  = "https://your-domain.com/wechat/"

    // 登录成功后回调，带微信授权 code
    var onLoginCode: ((String) -> Void)?
    // 登录失败回调
    var onLoginError: ((String) -> Void)?

    private override init() { super.init() }

    // MARK: - 注册 SDK（在 App 启动时调用）

    func registerApp() {
        /* SDK:
        WXApi.registerApp(wechatAppID, universalLink: universalLink)
        WXApi.startLog(by: .detail) { msg in
            print("[WeChat] \(msg)")
        }
        */
        print("[WeChatManager] registerApp called (mock mode — SDK not yet integrated)")
    }

    // MARK: - 发起授权登录

    func sendAuthRequest(from viewController: UIViewController? = nil) {
        /* SDK:
        let req = SendAuthReq()
        req.scope = "snsapi_userinfo"
        req.state = "wiseinvest_\(Int.random(in: 100000...999999))"
        WXApi.send(req) { [weak self] success in
            if !success {
                self?.onLoginError?("无法打开微信，请确保微信已安装")
            }
        }
        return
        */

        // ── Mock fallback（SDK 接入前使用）───────────────────────────────
        print("[WeChatManager] sendAuthRequest — using mock code (SDK not integrated)")
        let mockCode = "MOCK_WECHAT_CODE_\(Int.random(in: 10000...99999))"
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { [weak self] in
            self?.onLoginCode?(mockCode)
        }
    }

    // MARK: - 处理微信回调 URL（在 AppDelegate 中调用）

    @discardableResult
    func handleOpen(url: URL) -> Bool {
        /* SDK:
        return WXApi.handleOpen(url, delegate: self)
        */
        return false
    }

    // MARK: - 处理 Universal Link 回调（在 AppDelegate 中调用）

    @discardableResult
    func handleUserActivity(_ userActivity: NSUserActivity) -> Bool {
        /* SDK:
        return WXApi.handleOpenUniversalLink(userActivity, delegate: self)
        */
        return false
    }

    // MARK: - 检查微信是否安装

    var isWeChatInstalled: Bool {
        /* SDK:
        return WXApi.isWXAppInstalled()
        */
        return UIApplication.shared.canOpenURL(URL(string: "weixin://")!)
    }
}

// MARK: - WXApiDelegate
// 解注释此扩展并让 WeChatManager 遵循 WXApiDelegate 协议

/* SDK:
extension WeChatManager: WXApiDelegate {
    func onReq(_ req: BaseReq) {
        // 收到来自微信的请求（一般不需要处理）
    }

    func onResp(_ resp: BaseResp) {
        guard let authResp = resp as? SendAuthResp else { return }

        if authResp.errCode == 0, let code = authResp.code {
            print("[WeChatManager] auth success, code: \(code)")
            onLoginCode?(code)
        } else {
            let msg = authResp.errStr ?? "微信授权失败"
            print("[WeChatManager] auth failed: \(msg) (errCode: \(authResp.errCode))")
            onLoginError?(msg)
        }
    }
}
*/
