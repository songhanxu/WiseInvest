# 慧投 (WiseInvest)

> 一款面向新一代投资者的AI金融顾问iOS应用，让投资决策更智能、更理性

[![iOS](https://img.shields.io/badge/iOS-15.0+-blue.svg)](https://developer.apple.com/ios/)
[![Swift](https://img.shields.io/badge/Swift-5.7+-orange.svg)](https://swift.org/)
[![Go](https://img.shields.io/badge/Go-1.19+-00ADD8.svg)](https://golang.org/)
[![Python](https://img.shields.io/badge/Python-3.9+-3776AB.svg)](https://www.python.org/)

## 🎯 产品愿景

慧投不是传统的冰冷投资软件，而是一个随时待命、知识渊博、理性冷静的私人投资顾问。它帮助用户在信息过载的金融市场中找到信号，在情绪波动时保持理性，让每个人都能享受专业级的投资分析服务。

## ✨ 核心特性

### 🤖 AI对话式投资分析
- **自然语言交互**：用日常语言询问投资问题
- **个性化解读**：将复杂的金融数据转化为易懂的建议
- **情绪识别**：识别投资者情绪状态，提供理性建议

### 📊 智能投资组合分析
- **风险评估**：计算波动率、夏普比率、最大回撤等关键指标
- **相关性分析**：识别投资组合中的风险集中度
- **可视化展示**：直观的图表和AR 3D展示

### 📱 iOS生态深度集成
- **Siri快捷指令**：语音查询投资组合状态
- **Widget小组件**：主屏幕实时显示收益情况
- **Apple Watch**：手腕上的投资助手
- **推送通知**：重要市场事件及时提醒

### 🔒 安全与合规
- **数据加密**：本地AES-256加密存储
- **生物识别**：Face ID/Touch ID安全访问
- **合规设计**：严格遵循金融产品监管要求

## 🏗️ 技术架构

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   iOS Client    │◄──►│   Go API Gateway │◄──►│ Python Workers  │
│   (SwiftUI)     │    │   (Gin/WebSocket)│    │ (NumPy/Pandas)  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                │                        │
                                ▼                        ▼
                       ┌─────────────────┐    ┌─────────────────┐
                       │   Redis Cache   │    │  Financial APIs │
                       │   (实时数据)     │    │ (Alpha Vantage) │
                       └─────────────────┘    └─────────────────┘
```

### 前端技术栈
- **SwiftUI + Combine**：现代化响应式UI框架
- **Core Data**：本地数据持久化
- **Charts**：数据可视化
- **ARKit**：增强现实投资组合展示

### 后端技术栈
- **Go**：高性能API网关，WebSocket实时通信
- **Python**：金融计算引擎，AI模型推理
- **Redis**：高速数据缓存
- **PostgreSQL**：用户数据存储

## 🚀 快速开始

### 环境要求
- iOS 15.0+
- Xcode 14.0+
- Go 1.19+
- Python 3.9+
- Redis 6.0+

### 后端部署

```bash
# 克隆项目
git clone https://github.com/songhanxu/WiseInvest_backend
cd WiseInvest_backend

# 启动Go API服务
cd backend/api
go mod tidy
go run main.go

# 启动Python计算服务
cd ../workers
pip install -r requirements.txt
python worker.py

# 启动Redis缓存
redis-server
```

### iOS应用构建

```bash
# 克隆项目
git clone https://github.com/songhanxu/WiseInvest_ios
# 进入iOS项目目录
cd WiseInvest_ios

# 安装依赖
xcodebuild -resolvePackageDependencies

# 构建项目
xcodebuild -scheme WiseInvest -configuration Debug
```

## 📋 开发路线图

### Phase 1: MVP核心功能 (Week 1-8)
- [x] 基础架构搭建
- [x] 投资组合分析引擎
- [x] AI对话功能
- [ ] iOS基础界面

### Phase 2: iOS特色功能 (Week 9-12)
- [ ] Siri集成
- [ ] Widget小组件
- [ ] Apple Watch应用
- [ ] AR可视化

### Phase 3: 高级功能 (未来版本)
- [ ] 多Agent协同分析
- [ ] 情景模拟与回测
- [ ] 社区功能
- [ ] 专业研报

## 🔧 配置说明

### API密钥配置

```swift
// Config.swift
struct APIConfig {
    static let alphaVantageKey = "YOUR_ALPHA_VANTAGE_KEY"
    static let openAIKey = "YOUR_OPENAI_KEY"
    static let baseURL = "https://api.wiseinvest.com"
}
```

### 环境变量

```bash
# .env
ALPHA_VANTAGE_API_KEY=your_key_here
OPENAI_API_KEY=your_key_here
REDIS_URL=redis://localhost:6379
DATABASE_URL=postgresql://user:pass@localhost/wiseinvest
```

## 📊 功能演示

### 对话式分析
```
用户: "帮我分析一下我持有的苹果和特斯拉股票的风险"

AI: "根据您的持仓分析，苹果(AAPL)和特斯拉(TSLA)的相关性为0.65，
    说明两只股票走势较为相似。您的组合波动率为28.5%，
    主要风险来自特斯拉的高波动性。建议考虑增加一些
    低相关性的资产来分散风险..."
```

### Siri集成
```
"嘿Siri，我的投资组合今天表现如何？"
"嘿Siri，特斯拉股价怎么样？"
"嘿Siri，帮我分析一下科技股的风险"
```

## 🛡️ 安全与隐私

- **数据本地化**：敏感投资数据仅存储在用户设备
- **端到端加密**：所有网络传输使用TLS 1.3加密
- **最小权限原则**：仅请求必要的系统权限
- **透明度报告**：定期发布数据使用透明度报告

## 📄 合规声明

⚠️ **重要提示**：本应用提供的所有信息和建议仅供参考，不构成投资建议。投资有风险，入市需谨慎。用户应根据自身情况做出独立的投资决策。

## 🤝 贡献指南

我们欢迎社区贡献！请查看 [CONTRIBUTING.md](CONTRIBUTING.md) 了解详细信息。

### 开发流程
1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

## 📞 联系我们

- **项目主页**：https://github.com/songhanxu/WiseInvest
- **问题反馈**：https://github.com/songhanxu/WiseInvest/issues
- **邮箱**：support@wiseinvest.com
- **微信群**：扫描二维码加入开发者社群

## 📜 开源协议

本项目采用 [MIT License](LICENSE) 开源协议。

## 🙏 致谢

感谢以下开源项目和服务提供商：
- [Alpha Vantage](https://www.alphavantage.co/) - 金融数据API
- [OpenAI](https://openai.com/) - AI语言模型
- [Charts](https://github.com/danielgindi/Charts) - iOS图表库
- [Gin](https://github.com/gin-gonic/gin) - Go Web框架

---

**让投资更智能，让决策更理性** 🚀
