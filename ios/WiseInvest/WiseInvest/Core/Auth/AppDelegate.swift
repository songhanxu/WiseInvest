import UIKit

/// AppDelegate is required to intercept URL-scheme and Universal Link callbacks from WeChat.
class AppDelegate: NSObject, UIApplicationDelegate {

    // MARK: - App Lifecycle

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        // Register WeChat SDK on launch
        WeChatManager.shared.registerApp()
        return true
    }

    // MARK: - URL Scheme callback (WeChat calls back via wx{AppID}://...)

    func application(
        _ app: UIApplication,
        open url: URL,
        options: [UIApplication.OpenURLOptionsKey: Any] = [:]
    ) -> Bool {
        return WeChatManager.shared.handleOpen(url: url)
    }

    // MARK: - Universal Link callback (required for WechatOpenSDK 1.8.6+)

    func application(
        _ application: UIApplication,
        continue userActivity: NSUserActivity,
        restorationHandler: @escaping ([UIUserActivityRestoring]?) -> Void
    ) -> Bool {
        return WeChatManager.shared.handleUserActivity(userActivity)
    }
}
