# WiseInvest iOS

慧投 iOS 客户端 - 智能加密货币投资助手

## 🚀 快速开始

### 首次使用 - 创建 Xcode 项目

由于 Xcode 项目文件（.xcodeproj）是复杂的二进制格式，需要通过 Xcode 创建。

**📖 推荐阅读**：[SIMPLE_SETUP.md](SIMPLE_SETUP.md) - 图文并茂的详细步骤（5-10 分钟）

**🔧 或使用脚本**：
```bash
cd /Users/songhanxu/WiseInvest/ios
./create_xcode_project.sh
```

**📚 详细文档**：[CREATE_XCODE_PROJECT.md](CREATE_XCODE_PROJECT.md)

### 项目已创建 - 直接运行

```bash
# 打开项目
open WiseInvest.xcodeproj

# 在 Xcode 中：
# 1. 选择模拟器（iPhone 14 Pro）
# 2. 点击 Run (⌘R)
```

---

## 技术栈

- **SwiftUI**: 声明式 UI 框架
- **Combine**: 响应式编程
- **MVVM**: 架构模式
- **Clean Architecture**: 分层架构
- **Coordinator Pattern**: 导航管理

## 架构设计

```
ios/WiseInvest/
├── WiseInvestApp.swift          # App 入口
├── Core/                        # 核心层
│   ├── Coordinator/            # 导航协调器
│   ├── Network/                # 网络层
│   └── Config/                 # 配置
├── Domain/                      # 领域层
│   ├── Model/                  # 领域模型
│   └── Repository/             # 仓库接口
├── Data/                        # 数据层
│   └── Repository/             # 仓库实现
└── Presentation/                # 表现层
    ├── Home/                   # 首页
    │   ├── HomeView.swift
    │   └── HomeViewModel.swift
    └── Conversation/           # 对话页
        ├── ConversationView.swift
        └── ConversationViewModel.swift
```

### 架构层次说明

1. **Presentation Layer (表现层)**
   - SwiftUI Views
   - ViewModels (MVVM)
   - UI 组件

2. **Domain Layer (领域层)**
   - 业务模型
   - Repository 接口
   - 业务规则

3. **Data Layer (数据层)**
   - Repository 实现
   - API 调用
   - 数据转换

4. **Core Layer (核心层)**
   - 网络客户端
   - 导航管理
   - 工具类

## 功能特性

### 已实现

- ✅ 双 Agent 系统（投资顾问 + 交易助手）
- ✅ 实时流式对话
- ✅ 对话历史管理
- ✅ 精美的 UI 设计
- ✅ 深色模式
- ✅ 响应式布局

### 计划中

- ⏳ 用户认证
- ⏳ 本地数据持久化
- ⏳ 推送通知
- ⏳ Siri 集成
- ⏳ Widget 小组件
- ⏳ Apple Watch 应用

## 快速开始

### 前置要求

- Xcode 14.0+
- iOS 15.0+
- Swift 5.7+

### 安装步骤

1. **克隆项目**
```bash
git clone https://github.com/songhanxu/WiseInvest
cd WiseInvest/ios
```

2. **打开项目**
```bash
open WiseInvest.xcodeproj
```

3. **配置后端地址**

编辑 `Core/Config/Configuration.swift`：

```swift
static var apiBaseURL: String {
    #if DEBUG
    return "http://localhost:8080/api/v1"  // 本地开发
    #else
    return "https://api.wiseinvest.com/api/v1"  // 生产环境
    #endif
}
```

4. **运行项目**

在 Xcode 中选择目标设备，点击 Run (⌘R)

## UI 设计

### 设计原则

- **简洁优雅**: 去除冗余元素，突出核心功能
- **深色主题**: 护眼且符合金融应用调性
- **流畅动画**: 提升用户体验
- **响应式**: 适配不同屏幕尺寸

### 颜色方案

```swift
// 主色调
Background: #1a1a2e, #16213e (渐变)
Investment Advisor: #4CAF50 (绿色)
Trading Agent: #2196F3 (蓝色)

// 辅助色
Text Primary: #FFFFFF
Text Secondary: #FFFFFF (70% opacity)
Card Background: #FFFFFF (10% opacity)
```

### 组件库

- `AgentCard`: Agent 选择卡片
- `MessageBubble`: 消息气泡
- `ConversationRow`: 对话列表项
- `ScaleButtonStyle`: 按压缩放动画

## 开发指南

### 添加新页面

1. 在 `Presentation/` 下创建新文件夹
2. 创建 `View` 和 `ViewModel`
3. 在 `AppCoordinator` 中添加导航逻辑

示例：

```swift
// MyFeatureView.swift
struct MyFeatureView: View {
    @StateObject private var viewModel: MyFeatureViewModel
    
    var body: some View {
        // UI 实现
    }
}

// MyFeatureViewModel.swift
class MyFeatureViewModel: ObservableObject {
    @Published var data: [Item] = []
    
    func loadData() {
        // 加载数据
    }
}
```

### 网络请求

使用 `APIClient` 进行网络请求：

```swift
// 普通请求
apiClient.request(endpoint: "/agents")
    .sink { completion in
        // 处理完成
    } receiveValue: { (agents: [AgentInfo]) in
        // 处理数据
    }
    .store(in: &cancellables)

// 流式请求
apiClient.streamRequest(
    endpoint: "/messages/stream",
    method: .post,
    body: request,
    onChunk: { chunk in
        // 处理每个数据块
    },
    onComplete: {
        // 完成
    },
    onError: { error in
        // 错误处理
    }
)
```

### 状态管理

使用 Combine 进行响应式状态管理：

```swift
class MyViewModel: ObservableObject {
    @Published var items: [Item] = []
    @Published var isLoading = false
    @Published var errorMessage: String?
    
    private var cancellables = Set<AnyCancellable>()
}
```

## 测试

### 单元测试

```bash
# 运行所有测试
⌘U in Xcode
```

### UI 测试

```swift
// WiseInvestUITests.swift
func testConversationFlow() {
    let app = XCUIApplication()
    app.launch()
    
    // 测试对话流程
    app.buttons["Investment Advisor"].tap()
    // ...
}
```

## 性能优化

- 使用 `LazyVStack` 优化长列表
- 图片缓存和压缩
- 网络请求去重
- 内存管理和泄漏检测

## 调试技巧

```swift
// 打印网络请求
#if DEBUG
print("API Request: \(endpoint)")
print("Response: \(data)")
#endif

// 使用 Instruments 分析性能
// Product -> Profile (⌘I)
```

## 发布

### App Store 发布清单

- [ ] 更新版本号
- [ ] 配置生产环境 API
- [ ] 添加 App Icon
- [ ] 准备截图和描述
- [ ] 隐私政策
- [ ] 测试所有功能
- [ ] Archive 并上传

## 贡献指南

1. Fork 项目
2. 创建特性分支
3. 提交更改
4. 创建 Pull Request

## 许可证

MIT License
