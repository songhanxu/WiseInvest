import Foundation

/// Represents the authenticated user profile
struct User: Codable, Equatable {
    let id: UInt
    let displayName: String
    let avatar: String
    let wechatNickname: String?
    let wechatAvatar: String?
    let phone: String?

    enum CodingKeys: String, CodingKey {
        case id
        case displayName = "display_name"
        case avatar
        case wechatNickname = "wechat_nickname"
        case wechatAvatar = "wechat_avatar"
        case phone
    }

    /// Returns the best available avatar URL
    var effectiveAvatar: String {
        if !avatar.isEmpty { return avatar }
        return wechatAvatar ?? ""
    }

    /// Returns the best available display name
    var effectiveName: String {
        if !displayName.isEmpty { return displayName }
        return wechatNickname ?? "用户"
    }
}
