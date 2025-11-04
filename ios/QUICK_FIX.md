# 快速修复指南 - iOS 部署目标错误

## 错误原因

您看到的错误:
```
'ObservableObject' is only available in macOS 10.15 or newer
'Published' is only available in macOS 10.15 or newer
'ViewBuilder' is only available in macOS 10.15 or newer
```

这是因为 **Xcode 项目尚未创建**,或者部署目标设置过低。

## 解决方案

### 方案 1: 在 Xcode 中创建项目(推荐)

这是最简单、最可靠的方法:

#### 1. 打开 Xcode

```bash
open -a Xcode
```

#### 2. 创建新项目

1. **File → New → Project...**
2. 选择 **iOS → App**
3. 填写信息:
   - Product Name: `WiseInvest`
   - Team: 选择您的团队
   - Organization Identifier: `com.wiseinvest`
   - Interface: **SwiftUI**
   - Language: **Swift**
4. 保存到: `/Users/songhanxu/WiseInvest/ios/`
5. **取消勾选** "Create Git repository"

#### 3. 设置部署目标

1. 点击项目导航器中的 **WiseInvest** (蓝色图标)
2. 选择 **WiseInvest** target
3. **General** 标签页
4. **Minimum Deployments** → 设置为 **iOS 15.0**

#### 4. 删除默认文件

删除 Xcode 自动创建的:
- `ContentView.swift`

#### 5. 添加现有代码

1. 右键点击 **WiseInvest** 文件夹
2. **Add Files to "WiseInvest"...**
3. 选择 `/Users/songhanxu/WiseInvest/ios/WiseInvest/` 下的所有文件夹:
   - `Core/`
   - `Data/`
   - `Domain/`
   - `Presentation/`
4. ✅ 勾选 "Copy items if needed"
5. ✅ 勾选 "Create groups"
6. 点击 **Add**

#### 6. 更新 WiseInvestApp.swift

打开 Xcode 创建的 `WiseInvestApp.swift`,替换为:

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

#### 7. 构建并运行

按 **⌘B** 构建,然后 **⌘R** 运行。

---

### 方案 2: 使用自动化脚本(如果项目已存在)

如果您已经创建了 Xcode 项目,只需修复部署目标:

```bash
cd /Users/songhanxu/WiseInvest/ios
./fix_deployment_target.sh
```

然后:
1. 关闭 Xcode
2. 重新打开项目: `open WiseInvest.xcodeproj`
3. Clean Build Folder: **⇧⌘K**
4. Build: **⌘B**

---

## 验证步骤

### 检查部署目标

1. 打开 Xcode 项目
2. 选择项目 → WiseInvest target
3. **General** 标签页
4. 确认 **Minimum Deployments** 显示 **iOS 15.0** 或更高

### 检查 Build Settings

1. 选择 **Build Settings** 标签页
2. 搜索 "iOS Deployment Target"
3. 确认所有配置(Debug/Release)都是 **15.0**

---

## 完整的项目结构

创建完成后应该是这样:

```
ios/
├── WiseInvest.xcodeproj/          ← Xcode 项目文件
│   └── project.pbxproj
├── WiseInvest/
│   ├── WiseInvestApp.swift        ← 应用入口
│   ├── Assets.xcassets/           ← 资源文件
│   ├── Core/
│   │   ├── Coordinator/
│   │   │   └── AppCoordinator.swift
│   │   └── Extensions/
│   │       └── Color+Extensions.swift
│   ├── Data/
│   │   ├── Network/
│   │   │   └── APIClient.swift
│   │   └── Repository/
│   │       └── ConversationRepositoryImpl.swift
│   ├── Domain/
│   │   ├── Models/
│   │   │   ├── AgentType.swift
│   │   │   ├── Message.swift
│   │   │   └── Conversation.swift
│   │   └── Repository/
│   │       └── ConversationRepository.swift
│   └── Presentation/
│       ├── Home/
│       │   ├── HomeView.swift
│       │   └── HomeViewModel.swift
│       ├── Conversation/
│       │   ├── ConversationView.swift
│       │   └── ConversationViewModel.swift
│       └── Components/
│           ├── MessageBubble.swift
│           └── AgentCard.swift
├── XCODE_SETUP_GUIDE.md           ← 详细设置指南
├── QUICK_FIX.md                   ← 本文件
└── fix_deployment_target.sh       ← 自动修复脚本
```

---

## 常见问题

### Q: 我应该使用哪个方案?

**A**: 
- 如果还没有创建 Xcode 项目 → 使用**方案 1**
- 如果已经有 `.xcodeproj` 文件 → 使用**方案 2**

### Q: 如何知道是否已经有 Xcode 项目?

**A**: 运行:
```bash
ls -la /Users/songhanxu/WiseInvest/ios/*.xcodeproj
```

如果显示 "No such file or directory" → 使用方案 1
如果显示项目文件 → 使用方案 2

### Q: 方案 2 运行后仍然有错误?

**A**: 可能需要:
1. 完全删除项目重新创建(方案 1)
2. 或者在 Xcode 中手动检查:
   - 所有 Swift 文件是否都在 "Compile Sources" 中
   - 部署目标是否真的是 15.0
   - 是否选择了正确的 SDK

### Q: 为什么需要 iOS 15.0?

**A**: 我们的代码使用了 SwiftUI 的现代特性:
- `@StateObject` (iOS 14.0+)
- `@Published` (iOS 13.0+)
- `@ViewBuilder` (iOS 13.0+)
- `.task` modifier (iOS 15.0+)
- Async/await (iOS 15.0+)

为了使用所有功能,最低需要 iOS 15.0。

---

## 下一步

项目创建成功后:

### 1. 配置 API 密钥

编辑 `WiseInvest/Data/Network/APIClient.swift`:

```swift
private let apiKey = "sk-your-actual-openai-api-key"
```

### 2. 启动后端

```bash
cd /Users/songhanxu/WiseInvest/backend
./start.sh
```

### 3. 运行 iOS 应用

在 Xcode 中:
1. 选择模拟器(如 iPhone 15 Pro)
2. 按 **⌘R** 运行

### 4. 测试功能

1. 应用启动后,点击 "Investment Advisor"
2. 输入问题,如 "What are the best investment strategies?"
3. 查看 AI 回复

---

## 需要更多帮助?

查看详细文档:
- `XCODE_SETUP_GUIDE.md` - 完整的 Xcode 设置指南
- `../TROUBLESHOOTING.md` - 故障排除指南
- `../README.md` - 项目总览

---

**最后更新**: 2024
**适用于**: Xcode 14.0+, iOS 15.0+
