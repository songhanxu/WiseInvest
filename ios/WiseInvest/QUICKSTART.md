# 🚀 WiseInvest iOS 快速启动

## ✅ 当前状态

所有代码文件已创建完成!现在只需要在 Xcode 中添加文件即可。

## 📦 已创建的文件

### 核心文件 (15 个 Swift 文件)

```
✅ WiseInvestApp.swift              # 应用入口(已更新)
✅ Core/Coordinator/AppCoordinator.swift
✅ Core/Extensions/Color+Extensions.swift
✅ Domain/Models/AgentType.swift
✅ Domain/Models/Message.swift
✅ Domain/Models/Conversation.swift
✅ Domain/Repository/ConversationRepository.swift
✅ Data/Network/APIClient.swift
✅ Data/Repository/ConversationRepositoryImpl.swift
✅ Presentation/Home/HomeView.swift
✅ Presentation/Home/HomeViewModel.swift
✅ Presentation/Conversation/ConversationView.swift
✅ Presentation/Conversation/ConversationViewModel.swift
✅ Presentation/Components/AgentCard.swift
✅ Presentation/Components/MessageBubble.swift
```

## 🎯 三步完成设置

### 步骤 1: 运行添加文件助手

```bash
cd /Users/songhanxu/WiseInvest/ios/WiseInvest
./add_files_to_xcode.sh
```

这个脚本会:
- ✅ 检查项目状态
- ✅ 列出所有需要添加的文件
- ✅ 提供详细的添加步骤
- ✅ 可选择自动打开 Xcode

### 步骤 2: 在 Xcode 中添加文件

1. **右键点击** 项目导航器中的 `WiseInvest` 文件夹
2. 选择 **"Add Files to WiseInvest..."**
3. 选择这些文件夹:
   - `Core/`
   - `Data/`
   - `Domain/`
   - `Presentation/`
4. 配置:
   - ✅ Copy items if needed
   - ✅ Create groups
   - ✅ Add to targets: WiseInvest
5. 点击 **Add**

### 步骤 3: 构建并运行

```bash
# 在 Xcode 中:
# 1. Clean Build Folder: ⇧⌘K
# 2. Build: ⌘B
# 3. Run: ⌘R
```

## 🎨 功能预览

### 主页面
- 🎯 两个 Agent 卡片(投资顾问 + 交易代理)
- 📜 最近对话历史
- 🌙 深色主题设计

### 对话页面
- 💬 实时流式对话
- 🎨 精美的消息气泡
- ⚡ 流畅的动画效果
- 🗑️ 清除对话功能

## 🔧 配置检查

### 部署目标
确保设置为 **iOS 15.0+**:
- 项目设置 → General → Minimum Deployments

### 网络权限
Info.plist 已配置:
- ✅ Allow Arbitrary Loads
- ✅ Allow Local Networking

### 后端连接
默认连接到 `http://localhost:8080`

如需修改,编辑 `Data/Network/APIClient.swift`:
```swift
self.baseURL = "http://your-backend-url:8080"
```

## 🚀 完整运行流程

### 1. 启动后端

```bash
# 新终端窗口
cd /Users/songhanxu/WiseInvest/backend
./start.sh
```

等待看到:
```
✅ Backend server started successfully
🌐 Server running at: http://localhost:8080
```

### 2. 运行 iOS 应用

在 Xcode 中:
1. 选择模拟器(iPhone 15 Pro)
2. 按 **⌘R**

### 3. 测试对话

1. 点击 **"Investment Advisor"**
2. 输入: "What are the best investment strategies?"
3. 查看 AI 流式回复

## 📊 项目架构

```
┌─────────────────────────────────────┐
│         Presentation Layer          │
│  (Views + ViewModels + Coordinator) │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│          Domain Layer               │
│    (Models + Repository Protocol)   │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│           Data Layer                │
│  (API Client + Repository Impl)     │
└─────────────────────────────────────┘
```

### 设计模式
- ✅ **MVVM**: 视图与逻辑分离
- ✅ **Repository Pattern**: 数据访问抽象
- ✅ **Coordinator Pattern**: 导航管理
- ✅ **Dependency Injection**: 松耦合

## 🎯 代码特点

### 类型安全
```swift
enum AgentType: String, Codable {
    case investmentAdvisor = "investment_advisor"
    case tradingAgent = "trading_agent"
}
```

### 响应式编程
```swift
@Published var messages: [Message] = []
@Published var isLoading: Bool = false
```

### 流式处理
```swift
func sendMessage() -> AnyPublisher<String, Error>
```

## 📱 UI 设计

### 颜色主题
- 🌑 主背景: `#0A0E27`
- 🔷 强调蓝: `#4A90E2`
- 🟢 强调绿: `#50C878`

### 组件
- `AgentCard`: 可点击的 Agent 卡片
- `MessageBubble`: 聊天消息气泡
- `ConversationView`: 对话界面
- `HomeView`: 主页面

## 🐛 故障排除

### 编译错误

**问题**: Cannot find 'AppCoordinator' in scope

**解决**:
1. 确认所有文件都已添加到项目
2. Build Phases → Compile Sources 中有所有 .swift 文件
3. Clean Build Folder (⇧⌘K)

### 运行时错误

**问题**: Failed to connect to backend

**解决**:
1. 确认后端正在运行: `curl http://localhost:8080/health`
2. 检查 Info.plist 网络权限
3. 查看 Xcode Console 的详细错误

### 部署目标错误

**问题**: 'ObservableObject' is only available in iOS 15.0+

**解决**:
1. 项目设置 → General → Minimum Deployments → iOS 15.0
2. Build Settings → iOS Deployment Target → 15.0

## 📚 相关文档

- `SETUP_INSTRUCTIONS.md` - 详细设置说明
- `add_files_to_xcode.sh` - 文件添加助手
- `../TROUBLESHOOTING.md` - 故障排除指南
- `../../README.md` - 项目总览

## 💡 下一步

完成基础设置后,您可以:

1. **自定义 UI**: 修改 `Color+Extensions.swift` 中的颜色
2. **添加功能**: 实现币安 API 集成
3. **优化体验**: 添加语音输入、图表展示等
4. **测试**: 编写单元测试和 UI 测试

## 🎓 学习资源

- SwiftUI 官方文档
- Combine 框架指南
- Clean Architecture 最佳实践
- MVVM 设计模式

---

**准备好了吗?** 运行 `./add_files_to_xcode.sh` 开始吧! 🚀
