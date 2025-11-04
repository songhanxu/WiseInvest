# WiseInvest iOS 项目设置说明

## ✅ 代码文件已就绪

所有必要的 Swift 代码文件已经创建完成!现在需要在 Xcode 中进行一些配置。

## 📋 在 Xcode 中的配置步骤

### 1. 打开项目

```bash
cd /Users/songhanxu/WiseInvest/ios/WiseInvest
open WiseInvest.xcodeproj
```

### 2. 添加文件到项目

由于文件是在 Xcode 外部创建的,需要将它们添加到项目中:

1. **在 Xcode 项目导航器中**,右键点击 `WiseInvest` 文件夹
2. 选择 **"Add Files to WiseInvest"...**
3. 导航到 `/Users/songhanxu/WiseInvest/ios/WiseInvest/WiseInvest/`
4. 选择以下文件夹(按住 Command 键多选):
   - `Core/`
   - `Data/`
   - `Domain/`
   - `Presentation/`
5. **重要设置**:
   - ✅ 勾选 **"Copy items if needed"**
   - ✅ 选择 **"Create groups"**
   - ✅ 确保 **"Add to targets"** 中 `WiseInvest` 被勾选
6. 点击 **"Add"**

### 3. 验证部署目标

1. 在项目导航器中,点击最顶部的 **WiseInvest** 项目(蓝色图标)
2. 选择 **WiseInvest** target
3. 在 **General** 标签页中:
   - **Minimum Deployments**: 确保设置为 **iOS 15.0** 或更高

### 4. 配置 Info.plist (可选)

如果需要自定义 Info.plist:

1. 在项目设置中,选择 **Info** 标签页
2. 添加以下配置(如果还没有):
   - `App Transport Security Settings`
     - `Allow Arbitrary Loads`: YES
     - `Allow Local Networking`: YES

这允许应用连接到本地后端服务器。

### 5. 删除不需要的文件

如果项目中还有这些文件,请删除它们:
- `ContentView.swift`
- `Persistence.swift`
- `WiseInvest.xcdatamodeld`

### 6. 清理并构建

1. **Clean Build Folder**: `Product → Clean Build Folder` (⇧⌘K)
2. **Build**: `Product → Build` (⌘B)

## 🎯 项目结构

完成后,您的项目结构应该是:

```
WiseInvest/
├── WiseInvestApp.swift          # ✅ 已更新
├── Assets.xcassets/             # ✅ 保留
├── Info.plist                   # ✅ 已创建
├── Core/
│   ├── Coordinator/
│   │   └── AppCoordinator.swift
│   └── Extensions/
│       └── Color+Extensions.swift
├── Data/
│   ├── Network/
│   │   └── APIClient.swift
│   └── Repository/
│       └── ConversationRepositoryImpl.swift
├── Domain/
│   ├── Models/
│   │   ├── AgentType.swift
│   │   ├── Message.swift
│   │   └── Conversation.swift
│   └── Repository/
│       └── ConversationRepository.swift
└── Presentation/
    ├── Home/
    │   ├── HomeView.swift
    │   └── HomeViewModel.swift
    ├── Conversation/
    │   ├── ConversationView.swift
    │   └── ConversationViewModel.swift
    └── Components/
        ├── AgentCard.swift
        └── MessageBubble.swift
```

## 🚀 运行项目

### 1. 启动后端服务

在新的终端窗口中:

```bash
cd /Users/songhanxu/WiseInvest/backend
./start.sh
```

等待看到:
```
✅ Backend server started successfully
🌐 Server running at: http://localhost:8080
```

### 2. 运行 iOS 应用

1. 在 Xcode 中选择一个模拟器(推荐 iPhone 15 Pro)
2. 按 **⌘R** 运行
3. 应用应该启动并显示主页

### 3. 测试功能

1. 点击 **"Investment Advisor"** 卡片
2. 输入问题,例如: "What are the best investment strategies for beginners?"
3. 查看 AI 的流式回复

## ⚙️ 配置选项

### 修改后端 URL

如果后端运行在不同的地址,编辑 `Data/Network/APIClient.swift`:

```swift
private init() {
    // 修改这里的 URL
    self.baseURL = "http://localhost:8080"
    // ...
}
```

### 自定义主题颜色

编辑 `Core/Extensions/Color+Extensions.swift`:

```swift
static let primaryBackground = Color(hex: "0A0E27")  // 主背景色
static let accentBlue = Color(hex: "4A90E2")         // 强调色
// ...
```

## 🐛 常见问题

### Q: 编译错误 "Cannot find 'AppCoordinator' in scope"

**A**: 确保所有文件都已添加到项目:
1. 检查项目导航器中是否有完整的文件夹结构
2. 选择项目 → Build Phases → Compile Sources
3. 确认所有 .swift 文件都在列表中

### Q: 运行时错误 "Failed to connect to backend"

**A**: 
1. 确认后端服务正在运行: `curl http://localhost:8080/health`
2. 检查 Info.plist 中的网络权限设置
3. 如果使用真机,将 `localhost` 改为 Mac 的 IP 地址

### Q: 部署目标错误

**A**: 
1. 项目设置 → General → Minimum Deployments → iOS 15.0
2. Build Settings → iOS Deployment Target → 15.0

## 📱 功能特性

### ✅ 已实现

- 🎨 精美的深色主题 UI
- 💬 实时流式对话
- 🤖 双 Agent 支持(投资顾问 + 交易代理)
- 💾 对话历史保存
- 🔄 清除对话功能
- ⚡ 响应式设计
- 🎯 Clean Architecture + MVVM

### 🚧 待实现

- 币安 API 集成(Trading Agent)
- 用户认证
- 多语言支持
- 语音输入
- 图表展示

## 📚 架构说明

### Clean Architecture 分层

- **Presentation**: UI 层(Views + ViewModels)
- **Domain**: 业务逻辑层(Models + Repository Protocols)
- **Data**: 数据层(API Client + Repository Implementations)
- **Core**: 核心功能(Coordinator + Extensions)

### 设计模式

- **MVVM**: Presentation 层
- **Repository Pattern**: 数据访问抽象
- **Coordinator Pattern**: 导航管理
- **Dependency Injection**: 依赖注入

## 🎓 代码质量

- ✅ 类型安全
- ✅ 协议导向
- ✅ 可测试性
- ✅ 可维护性
- ✅ 可扩展性

## 需要帮助?

查看其他文档:
- `../TROUBLESHOOTING.md` - 故障排除
- `../README.md` - 项目总览
- `../../backend/README.md` - 后端文档

---

**最后更新**: 2024
**iOS 版本**: 15.0+
**Xcode 版本**: 14.0+
