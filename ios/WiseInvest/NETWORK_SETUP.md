# 🔧 网络权限配置指南

## ✅ Info.plist 冲突已解决

我已经删除了手动创建的 `Info.plist` 文件。现代 Xcode 项目会自动生成 Info.plist,不需要手动创建。

## 📋 配置网络权限

为了让应用能够连接到本地后端服务器 (http://localhost:8080),需要在 Xcode 中配置网络权限。

### 方法 1: 在 Xcode 中配置 (推荐)

1. **打开项目设置**
   - 在项目导航器中,点击最顶部的 **WiseInvest** (蓝色图标)
   - 选择 **WiseInvest** target
   - 选择 **Info** 标签页

2. **添加网络权限**
   - 点击任意一行,然后点击 **+** 按钮
   - 添加以下配置:

   ```
   Key: App Transport Security Settings
   Type: Dictionary
   
   展开后添加:
   ├─ Key: Allow Arbitrary Loads
   │  Type: Boolean
   │  Value: YES
   │
   └─ Key: Allow Local Networking  
      Type: Boolean
      Value: YES
   ```

3. **详细步骤**:
   
   a. 添加 `App Transport Security Settings`:
      - 点击 **+** 按钮
      - 输入: `App Transport Security Settings`
      - Type 选择: `Dictionary`
   
   b. 展开 `App Transport Security Settings`,添加子项:
      - 点击 `App Transport Security Settings` 左边的三角形展开
      - 点击 **+** 按钮添加第一个子项:
        - Key: `Allow Arbitrary Loads`
        - Type: `Boolean`
        - Value: 勾选 ✅ (YES)
      
      - 再点击 **+** 按钮添加第二个子项:
        - Key: `Allow Local Networking`
        - Type: `Boolean`
        - Value: 勾选 ✅ (YES)

### 方法 2: 直接编辑 Info.plist (备选)

如果您更喜欢直接编辑 plist 文件:

1. 在项目导航器中,找到 `Info.plist` 文件
2. 右键点击,选择 **Open As → Source Code**
3. 在 `<dict>` 标签内添加:

```xml
<key>NSAppTransportSecurity</key>
<dict>
    <key>NSAllowsArbitraryLoads</key>
    <true/>
    <key>NSAllowsLocalNetworking</key>
    <true/>
</dict>
```

完整示例:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>NSAppTransportSecurity</key>
    <dict>
        <key>NSAllowsArbitraryLoads</key>
        <true/>
        <key>NSAllowsLocalNetworking</key>
        <true/>
    </dict>
</dict>
</plist>
```

## 🔒 安全说明

### 开发环境配置

当前配置适用于**开发环境**:
- ✅ `Allow Arbitrary Loads`: 允许 HTTP 连接(开发用)
- ✅ `Allow Local Networking`: 允许连接本地服务器

### 生产环境建议

在发布到 App Store 前,应该:

1. **移除 Allow Arbitrary Loads**
2. **使用 HTTPS**
3. **配置特定域名白名单**:

```xml
<key>NSAppTransportSecurity</key>
<dict>
    <key>NSExceptionDomains</key>
    <dict>
        <key>your-api-domain.com</key>
        <dict>
            <key>NSIncludesSubdomains</key>
            <true/>
            <key>NSTemporaryExceptionAllowsInsecureHTTPLoads</key>
            <true/>
        </dict>
    </dict>
</dict>
```

## ✅ 验证配置

### 1. 构建项目

在 Xcode 中:
- Clean Build Folder: `⇧⌘K`
- Build: `⌘B`

应该不再有 Info.plist 冲突错误。

### 2. 测试网络连接

运行应用后:
1. 确保后端服务正在运行
2. 点击 "Investment Advisor"
3. 发送消息
4. 如果能收到回复,说明网络配置成功

### 3. 检查日志

如果仍然无法连接,在 Xcode Console 中查看错误信息:
- 打开 Console: `⌘⇧Y`
- 查找网络相关错误

## 🐛 常见问题

### Q: 仍然看到 Info.plist 冲突错误

**A**: 
1. 关闭 Xcode
2. 删除 DerivedData:
   ```bash
   rm -rf ~/Library/Developer/Xcode/DerivedData/WiseInvest-*
   ```
3. 重新打开项目
4. Clean Build Folder (`⇧⌘K`)
5. 重新构建 (`⌘B`)

### Q: 网络请求失败

**A**: 检查以下几点:
1. 后端服务是否运行: `curl http://localhost:8080/health`
2. Info.plist 中是否已添加网络权限
3. APIClient.swift 中的 baseURL 是否正确
4. 查看 Xcode Console 的详细错误信息

### Q: 真机测试无法连接

**A**: 真机无法使用 localhost,需要:
1. 获取 Mac 的 IP 地址:
   ```bash
   ifconfig | grep "inet " | grep -v 127.0.0.1
   ```
2. 修改 `Data/Network/APIClient.swift`:
   ```swift
   self.baseURL = "http://192.168.x.x:8080"  // 替换为您的 Mac IP
   ```
3. 确保 Mac 和 iPhone 在同一 WiFi 网络

## 📱 不同环境配置

### 模拟器 (推荐开发使用)

```swift
// Data/Network/APIClient.swift
#if targetEnvironment(simulator)
    self.baseURL = "http://localhost:8080"
#else
    self.baseURL = "http://192.168.1.100:8080"  // 真机使用 Mac IP
#endif
```

### 使用环境变量

```swift
private init() {
    #if DEBUG
        self.baseURL = "http://localhost:8080"
    #else
        self.baseURL = "https://api.wiseinvest.com"
    #endif
}
```

## 🎯 下一步

配置完成后:

1. **Clean Build**: `⇧⌘K`
2. **Build**: `⌘B`
3. **Run**: `⌘R`

应该可以正常运行了! 🚀

## 📚 相关文档

- [Apple - App Transport Security](https://developer.apple.com/documentation/security/preventing_insecure_network_connections)
- [Configuring App Transport Security](https://developer.apple.com/library/archive/documentation/General/Reference/InfoPlistKeyReference/Articles/CocoaKeys.html#//apple_ref/doc/uid/TP40009251-SW33)

---

**提示**: 如果遇到其他问题,请查看 `TROUBLESHOOTING.md` 或项目文档。
