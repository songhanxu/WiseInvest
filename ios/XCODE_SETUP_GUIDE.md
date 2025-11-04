# WiseInvest iOS 项目 Xcode 设置指南

## 问题说明

您遇到的错误是因为 SwiftUI 代码需要 **iOS 15.0+** 作为最低部署目标,但 Xcode 项目尚未创建或配置。

## 解决方案:在 Xcode 中创建项目

### 步骤 1: 打开 Xcode

1. 启动 **Xcode** 应用程序
2. 如果看到欢迎界面,点击 "Create a new Xcode project"
3. 如果没有欢迎界面,选择菜单 **File → New → Project...**

### 步骤 2: 选择项目模板

1. 在模板选择界面:
   - 选择 **iOS** 标签页
   - 选择 **App** 模板
   - 点击 **Next**

### 步骤 3: 配置项目

填写以下信息:

- **Product Name**: `WiseInvest`
- **Team**: 选择您的开发团队(如果没有,选择 "None")
- **Organization Identifier**: `com.wiseinvest`
- **Bundle Identifier**: 自动生成为 `com.wiseinvest.WiseInvest`
- **Interface**: 选择 **SwiftUI**
- **Language**: 选择 **Swift**
- **Storage**: 选择 **None**
- **Include Tests**: 可以取消勾选

点击 **Next**

### 步骤 4: 选择保存位置

1. 导航到: `/Users/songhanxu/WiseInvest/ios/`
2. **重要**: 确保 "Create Git repository" 取消勾选(我们已经有 Git 仓库)
3. 点击 **Create**

### 步骤 5: 删除默认文件

Xcode 会创建一些默认文件,我们需要删除它们:

1. 在左侧项目导航器中,找到并删除以下文件(右键 → Delete → Move to Trash):
   - `ContentView.swift` (如果存在)
   - 任何其他自动生成的 Swift 文件(除了 `WiseInvestApp.swift`)

### 步骤 6: 添加现有的 Swift 文件

1. 在项目导航器中,右键点击 **WiseInvest** 文件夹
2. 选择 **Add Files to "WiseInvest"...**
3. 导航到 `/Users/songhanxu/WiseInvest/ios/WiseInvest/`
4. 选择所有文件夹:
   - `Core/`
   - `Data/`
   - `Domain/`
   - `Presentation/`
5. **重要设置**:
   - ✅ 勾选 "Copy items if needed"
   - ✅ 勾选 "Create groups"
   - ✅ 确保 "Add to targets" 中 WiseInvest 被勾选
6. 点击 **Add**

### 步骤 7: 配置部署目标 (关键!)

1. 在项目导航器中,点击最顶部的 **WiseInvest** 项目(蓝色图标)
2. 在中间面板,确保选中 **WiseInvest** target
3. 选择 **General** 标签页
4. 在 **Deployment Info** 部分:
   - **Minimum Deployments**: 设置为 **iOS 15.0** 或更高
   - **iPhone Orientation**: 勾选 Portrait
   - **iPad Orientation**: 可以全部勾选

### 步骤 8: 替换 WiseInvestApp.swift

1. 在项目导航器中,找到 Xcode 自动生成的 `WiseInvestApp.swift`
2. 打开它,将内容替换为:

```swift
import SwiftUI

@main
struct WiseInvestApp: App {
    @StateObject private var appCoordinator = AppCoordinator()
    
    var body: some Scene {
        WindowGroup {
            appCoordinator.start()
                .preferredColorScheme(.dark)
        }
    }
}
```

### 步骤 9: 添加 Assets

1. 在项目导航器中,找到 `Assets.xcassets`
2. 确保它存在(Xcode 应该已经创建了)
3. 如果需要,可以添加 App Icon 和其他资源

### 步骤 10: 配置 API 密钥

1. 打开 `WiseInvest/Data/Network/APIClient.swift`
2. 找到这一行:
```swift
private let apiKey = "YOUR_OPENAI_API_KEY"
```
3. 替换为您的实际 OpenAI API 密钥

### 步骤 11: 构建项目

1. 选择一个模拟器或真机作为目标设备(顶部工具栏)
2. 按 **⌘B** 或点击 **Product → Build**
3. 确保没有编译错误

### 步骤 12: 运行项目

1. 按 **⌘R** 或点击运行按钮 ▶️
2. 应用应该在模拟器或设备上启动

## 验证清单

构建前请确认:

- ✅ 部署目标设置为 iOS 15.0 或更高
- ✅ 所有 Swift 文件都已添加到项目
- ✅ WiseInvestApp.swift 内容正确
- ✅ Assets.xcassets 存在
- ✅ 选择了有效的开发团队(用于真机测试)
- ✅ OpenAI API 密钥已配置

## 常见问题

### Q: 仍然看到 "only available in macOS 10.15" 错误

**A**: 检查部署目标设置:
1. 项目导航器 → WiseInvest 项目 → WiseInvest target
2. Build Settings 标签页
3. 搜索 "iOS Deployment Target"
4. 确保设置为 15.0 或更高

### Q: 找不到某些 Swift 文件

**A**: 确保在添加文件时:
- 选择了正确的文件夹
- 勾选了 "Create groups"
- 勾选了 "Add to targets: WiseInvest"

### Q: 编译错误 "Cannot find 'AppCoordinator' in scope"

**A**: 确保所有文件都已正确添加:
1. 检查项目导航器中是否有完整的文件夹结构
2. 检查 Build Phases → Compile Sources 中是否包含所有 .swift 文件

### Q: 需要配置开发团队

**A**: 
1. 项目设置 → Signing & Capabilities
2. 选择您的 Apple ID 团队
3. 如果没有,点击 "Add Account" 添加您的 Apple ID

## 项目结构

创建完成后,您的项目结构应该是:

```
WiseInvest/
├── WiseInvest.xcodeproj/
├── WiseInvest/
│   ├── WiseInvestApp.swift
│   ├── Assets.xcassets/
│   ├── Core/
│   │   ├── Coordinator/
│   │   └── Extensions/
│   ├── Data/
│   │   ├── Network/
│   │   └── Repository/
│   ├── Domain/
│   │   ├── Models/
│   │   └── Repository/
│   └── Presentation/
│       ├── Home/
│       ├── Conversation/
│       └── Components/
```

## 下一步

项目创建成功后:

1. **启动后端服务**:
   ```bash
   cd /Users/songhanxu/WiseInvest/backend
   ./start.sh
   ```

2. **运行 iOS 应用**:
   - 在 Xcode 中按 ⌘R

3. **测试对话功能**:
   - 点击 "Investment Advisor" 卡片
   - 输入投资相关问题
   - 查看 AI 回复

## 需要帮助?

如果遇到问题:

1. 检查 Xcode 版本(需要 Xcode 14.0+)
2. 检查 macOS 版本(需要 macOS 12.0+)
3. 查看 TROUBLESHOOTING.md 文档
4. 检查后端服务是否正常运行

---

**提示**: 首次构建可能需要几分钟来下载和编译依赖项。
