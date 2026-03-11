import UIKit
import UserNotifications

/// AppDelegate is required to intercept URL-scheme and Universal Link callbacks from WeChat,
/// and to handle remote push notification registration.
class AppDelegate: NSObject, UIApplicationDelegate {

    // MARK: - App Lifecycle

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        // Register WeChat SDK on launch
        WeChatManager.shared.registerApp()

        // Request push notification permission and register with APNs
        registerForPushNotifications()

        return true
    }

    // MARK: - Push Notifications

    private func registerForPushNotifications() {
        UNUserNotificationCenter.current().requestAuthorization(
            options: [.alert, .badge, .sound]
        ) { granted, error in
            guard granted else { return }
            DispatchQueue.main.async {
                UIApplication.shared.registerForRemoteNotifications()
            }
        }
    }

    func application(
        _ application: UIApplication,
        didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data
    ) {
        let tokenStr = deviceToken.map { String(format: "%02.2hhx", $0) }.joined()
        Task {
            do {
                try await APIClient.shared.registerDeviceToken(tokenStr)
            } catch {
                // Non-fatal: push will still work next launch
                print("[AppDelegate] device token registration failed: \(error)")
            }
        }
    }

    func application(
        _ application: UIApplication,
        didFailToRegisterForRemoteNotificationsWithError error: Error
    ) {
        print("[AppDelegate] failed to register for remote notifications: \(error)")
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
