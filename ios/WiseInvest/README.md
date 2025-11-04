# WiseInvest iOS 应用

> 基于 SwiftUI 的 AI 投资助手 iOS 客户端

## ✅ 项目状态

**所有代码文件已创建完成!** 现在只需在 Xcode 中添加文件即可运行。

## 📦 项目结构

```
WiseInvest/
├── WiseInvestApp.swift                    # ✅ 应用入口
├── Assets.xcassets/                       # ✅ 资源文件
├── Info.plist                             # ✅ 配置文件
│
├── Core/                                  # 核心功能
│   ├── Coordinator/
│   │   └── AppCoordinator.swift          # ✅ 导航协调器
│   └── Extensions/
│       └── Color+Extensions.swift        # ✅ 颜色扩展
│
├── Domain/                                # 领域层
│   ├── Models/
│   │   ├── AgentType.swift               # ✅ Agent 类型
│   │   ├── Message.swift                 # ✅ 消息模型
│   │   └── Conversation.swift            # ✅ 对话模型
│   └── Repository/
│       └── ConversationRepository.swift  # ✅ 仓储协议
│
├── Data/                                  # 数据层
│   ├── Network/
│   │   └── APIClient.swift               # ✅ API 客户端
│   └── Repository/
│       └── ConversationRepositoryImpl.swift # ✅ 仓储实现
│
└── Presentation/                          # 展示层
    ├── Home/
    │   ├── HomeView.swift                # ✅ 主页视图
    │   └── HomeViewModel.swift           # ✅ 主页视图模型
    ├── Conversation/
    │   ├── ConversationView.swift        # ✅ 对话视图
    │   └── ConversationViewModel.swift   # ✅ 对话视图模型
    └── Components/
        ├── AgentCard.swift               # ✅ Agent 卡片组件
        └── MessageBubble.swift           # ✅ 消息气泡组件
```

**统计**: 15 个 Swift 文件,约 1200 行代码

## 🚀 快速开始

### 方式 1: 使用自动化脚本(推荐)

```bash
cd /Users/songhanxu/WiseInvest/ios/WiseInvest
./add_files_to_xcode.sh
```

按照脚本提示操作即可。

### 方式 2: 手动添加

1. **打开项目**:
   ```bash
   open WiseInvest.xcodeproj
   ```

2. **添加文件**:
   - 右键点击 `WiseInvest` 文件夹
   - 选择 "Add Files to WiseInvest..."
   - 选择 `Core/`, `Data/`, `Domain/`, `Presentation/` 文件夹
   - ✅ Copy items if needed
   - ✅ Create groups
   - ✅ Add to targets: WiseInvest

3. **构建运行**:
   - Clean: ⇧⌘K
   - Build: ⌘B
   - Run: ⌘R

## 🎯 功能特性

### ✅ 已实现

- **双 Agent 系统**
  - 💼 Investment Advisor - 投资建议
  - 💰 Trading Agent - 交易执行

- **实时对话**
  - 💬 流式响应
  - ⚡ 即时反馈
  - 🎨 精美 UI

- **数据持久化**
  - 💾 对话历史保存
  - 🔄 自动同步

- **用户体验**
  - 🌙 深色主题
  - 📱 响应式设计
  - ✨ 流畅动画

### 🚧 待实现

- 币安 API 集成
- 用户认证
- 多语言支持
- 语音输入
- 图表展示

## 🏗️ 架构设计

### Clean Architecture

```
┌─────────────────────────────────────┐
│      Presentation Layer             │
│   Views + ViewModels + Coordinator  │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│         Domain Layer                │
│   Models + Repository Protocols     │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│          Data Layer                 │
│   API Client + Repository Impl      │
└─────────────────────────────────────┘
```

### 设计模式

- **MVVM**: 视图与业务逻辑分离
- **Repository Pattern**: 数据访问抽象
- **Coordinator Pattern**: 导航流程管理
- **Dependency Injection**: 依赖注入

### 技术栈

- **UI**: SwiftUI
- **响应式**: Combine
- **网络**: URLSession
- **存储**: UserDefaults
- **架构**: Clean Architecture + MVVM

## 🎨 UI 设计

### 颜色主题

```swift
// 背景色
primaryBackground   = #0A0E27  // 深蓝黑
secondaryBackground = #1A1F3A  // 次级背景

// 强调色
accentBlue   = #4A90E2  // 蓝色
accentGreen  = #50C878  // 绿色
accentPurple = #9B59B6  // 紫色

// 文字色
textPrimary   = #FFFFFF  // 主文字
textSecondary = #A0A0A0  // 次级文字
textTertiary  = #666666  // 三级文字
```

### 组件库

- `AgentCard` - Agent 选择卡片
- `MessageBubble` - 聊天消息气泡
- `ConversationRow` - 对话历史行
- `ScaleButtonStyle` - 缩放按钮样式

## ⚙️ 配置

### 部署目标

- **最低版本**: iOS 15.0
- **推荐版本**: iOS 16.0+
- **Xcode**: 14.0+

### 网络配置

默认后端地址: `http://localhost:8080`

修改方法:
```swift
// Data/Network/APIClient.swift
private init() {
    self.baseURL = "http://your-backend-url:8080"
}
```

### Info.plist

已配置网络权限:
- ✅ Allow Arbitrary Loads
- ✅ Allow Local Networking

## 🧪 测试

### 运行测试

```bash
# 启动后端
cd /Users/songhanxu/WiseInvest/backend
./start.sh

# 运行 iOS 应用
# 在 Xcode 中按 ⌘R
```

### 测试场景

1. **投资顾问对话**
   - 点击 "Investment Advisor"
   - 输入: "What are the best investment strategies?"
   - 验证流式响应

2. **交易代理对话**
   - 点击 "Trading Agent"
   - 输入: "Show my portfolio"
   - 验证响应内容

3. **对话历史**
   - 发送多条消息
   - 返回主页
   - 验证历史记录显示

## 📚 文档

- `QUICKSTART.md` - 快速启动指南
- `SETUP_INSTRUCTIONS.md` - 详细设置说明
- `add_files_to_xcode.sh` - 文件添加助手

## 🐛 故障排除

### 常见问题

**Q: 编译错误 "Cannot find 'AppCoordinator' in scope"**

A: 确保所有文件都已添加到项目:
- Build Phases → Compile Sources
- 应该包含所有 15 个 .swift 文件

**Q: 运行时错误 "Failed to connect to backend"**

A: 检查后端服务:
```bash
curl http://localhost:8080/health
```

**Q: 部署目标错误**

A: 设置最低版本为 iOS 15.0:
- 项目设置 → General → Minimum Deployments

更多问题请查看 `../TROUBLESHOOTING.md`

## 🔄 更新日志

### v1.0.0 (2024)

- ✅ 初始版本
- ✅ 双 Agent 系统
- ✅ 实时流式对话
- ✅ Clean Architecture
- ✅ 深色主题 UI

## 🤝 贡献

欢迎提交 Issue 和 Pull Request!

## 📄 许可证

MIT License

## 👥 作者

WiseInvest Team

---

**准备好了吗?** 运行 `./add_files_to_xcode.sh` 开始使用! 🚀
