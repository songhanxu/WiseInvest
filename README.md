# 慧投 (WiseInvest)

> 基于 tRPC-Agent-Go 的智能加密货币投资助手，提供专业投资分析与自动化交易能力

[![iOS](https://img.shields.io/badge/iOS-15.0+-blue.svg)](https://developer.apple.com/ios/)
[![Swift](https://img.shields.io/badge/Swift-5.7+-orange.svg)](https://swift.org/)
[![Go](https://img.shields.io/badge/Go-1.19+-00ADD8.svg)](https://golang.org/)
[![tRPC](https://img.shields.io/badge/tRPC-Agent-00D9FF.svg)](https://github.com/trpc-group/trpc-go)

## 📚 快速导航

- **📦 [安装指南](INSTALL.md)** - 详细的安装步骤
- **🚀 [快速启动](QUICKSTART.md)** - 5 分钟内启动项目
- **📱 [iOS 项目创建](iOS_PROJECT_GUIDE.md)** - 创建 Xcode 项目指南 ⭐
- **🔧 [故障排除](TROUBLESHOOTING.md)** - 常见问题解决方案
- **🏗️ [架构设计](ARCHITECTURE.md)** - 深入了解技术架构
- **📁 [项目结构](PROJECT_STRUCTURE.md)** - 详细的目录说明
- **📝 [项目总结](PROJECT_SUMMARY.md)** - 完整的交付文档
- **🔧 [后端文档](backend/README.md)** - Go 后端开发指南
- **📱 [iOS 文档](ios/README.md)** - iOS 客户端开发指南

## 🎯 产品愿景

慧投是一款基于 tRPC-Agent-Go 框架构建的智能加密货币投资助手，通过双 Agent 架构提供专业的投资分析和自动化交易能力。让加密货币投资更智能、更安全、更高效。

## ✨ 核心特性

### 🤖 双 Agent 智能系统

#### 1️⃣ 投资对话 Agent (Investment Advisor Agent)
- **专业投资分析**：基于 AI 的市场分析和投资建议
- **自然语言交互**：用日常语言询问投资问题，获得专业解答
- **实时市场洞察**：分析加密货币市场趋势、技术指标和新闻事件
- **风险评估**：计算波动率、夏普比率、最大回撤等关键指标
- **投资组合优化**：提供个性化的资产配置建议
- **情绪识别**：识别投资者情绪状态，提供理性建议

#### 2️⃣ 币安交易 Agent (Binance Trading Agent)
- **币安 API 集成**：深度集成币安交易所 API
- **自动化交易**：支持市价单、限价单、止损单等多种订单类型
- **实时行情监控**：WebSocket 实时推送价格变动
- **账户管理**：查询余额、持仓、交易历史
- **风险控制**：设置止损止盈、仓位管理、交易限额
- **交易执行**：快速、安全地执行交易指令

### 🏗️ 基于 tRPC-Agent-Go 框架
- **高性能 RPC**：基于 tRPC 的高性能微服务架构
- **Agent 编排**：灵活的 Agent 协作与任务编排
- **插件化设计**：可扩展的工具和能力插件系统
- **流式响应**：支持 AI 对话的流式输出
- **可观测性**：完善的日志、监控和追踪能力

### 🔒 安全与合规
- **API 密钥加密**：本地 AES-256 加密存储交易所 API 密钥
- **生物识别**：Face ID/Touch ID 安全访问
- **权限隔离**：只读 API 与交易 API 分离管理
- **交易确认**：重要交易操作需要二次确认
- **审计日志**：完整的操作日志记录

## 🏗️ 技术架构

```
┌─────────────────────────────────────────────────────────────┐
│                      iOS Client (SwiftUI)                    │
│  ┌──────────────────────┐      ┌──────────────────────┐    │
│  │  投资对话界面         │      │  交易控制界面         │    │
│  └──────────────────────┘      └──────────────────────┘    │
└────────────────────┬──────────────────────┬─────────────────┘
                     │                      │
                     ▼                      ▼
         ┌───────────────────────────────────────────┐
         │      tRPC-Agent-Go 服务层                  │
         │  ┌─────────────────────────────────────┐  │
         │  │        Agent 编排引擎                │  │
         │  └─────────────────────────────────────┘  │
         │           │                    │           │
         │           ▼                    ▼           │
         │  ┌──────────────────┐  ┌─────────────────┐│
         │  │ Investment Agent │  │ Trading Agent   ││
         │  │  - AI 对话       │  │  - 币安 API     ││
         │  │  - 市场分析      │  │  - 订单管理     ││
         │  │  - 风险评估      │  │  - 实时行情     ││
         │  └──────────────────┘  └─────────────────┘│
         └───────────┬──────────────────┬─────────────┘
                     │                  │
                     ▼                  ▼
         ┌──────────────────┐  ┌──────────────────┐
         │   AI 服务         │  │  币安交易所       │
         │  - LLM API       │  │  - REST API      │
         │  - 向量数据库     │  │  - WebSocket     │
         └──────────────────┘  └──────────────────┘
```

### iOS 客户端技术栈
- **SwiftUI + Combine**：现代化响应式 UI 框架
- **Core Data**：本地数据持久化
- **Charts**：数据可视化
- **CryptoKit**：加密货币相关加密操作
- **WebSocket**：实时行情推送

### tRPC-Agent-Go 后端技术栈
- **tRPC-Go**：腾讯开源的高性能 RPC 框架
- **Agent Framework**：智能 Agent 编排和管理
- **Gin**：HTTP API 网关
- **WebSocket**：实时双向通信
- **Redis**：高速缓存和消息队列
- **PostgreSQL**：用户数据和交易记录存储

### 外部服务集成
- **币安 API**：加密货币交易和行情数据
- **AI 服务**：大语言模型（OpenAI/Claude/本地模型）
- **市场数据**：CoinGecko、CoinMarketCap 等

## 🚀 快速开始

### 环境要求
- iOS 15.0+
- Xcode 14.0+
- Go 1.19+
- Redis 6.0+
- PostgreSQL 13.0+

### 后端部署

```bash
# 克隆项目
git clone https://github.com/songhanxu/WiseInvest
cd WiseInvest

# 安装依赖（macOS）
brew install postgresql@15 redis

# 启动服务
brew services start postgresql@15
brew services start redis

# 进入后端目录
cd backend

# 安装 Go 依赖
go mod download

# 初始化数据库
make db-init

# 配置环境变量
cp .env.example .env
# 编辑 .env 文件，填入必要的配置

# 启动后端服务
go run cmd/server/main.go
```

### 配置币安 API

```bash
# 在 .env 文件中配置币安 API 密钥
BINANCE_API_KEY=your_binance_api_key
BINANCE_SECRET_KEY=your_binance_secret_key
BINANCE_TESTNET=true  # 开发环境使用测试网

# 配置 AI 服务
OPENAI_API_KEY=your_openai_api_key
# 或使用其他 AI 服务
CLAUDE_API_KEY=your_claude_api_key
```

### iOS 应用构建

```bash
# 进入 iOS 项目目录（如果分离）
cd WiseInvest_ios

# 安装依赖
xcodebuild -resolvePackageDependencies

# 创建 Xcode 项目
# 查看 ios/CREATE_XCODE_PROJECT.md 获取详细步骤

# 或运行自动化脚本
cd ios
./create_xcode_project.sh
```

## 📋 开发路线图

### Phase 1: 双 Agent 核心功能 (Week 1-8)
- [x] tRPC-Agent-Go 框架集成
- [ ] 投资对话 Agent 开发
  - [ ] AI 对话引擎
  - [ ] 市场数据分析
  - [ ] 投资建议生成
- [ ] 币安交易 Agent 开发
  - [ ] 币安 API 集成
  - [ ] 订单管理系统
  - [ ] 实时行情推送
- [ ] iOS 基础界面

### Phase 2: 高级交易功能 (Week 9-12)
- [ ] 自动化交易策略
- [ ] 风险控制系统
- [ ] 回测引擎
- [ ] 交易信号推送

### Phase 3: 智能化增强 (未来版本)
- [ ] 多 Agent 协同决策
- [ ] 量化策略市场
- [ ] 社区信号分享
- [ ] 专业投研报告

## 🔧 配置说明

### API 密钥配置

```swift
// Config.swift
struct APIConfig {
    static let binanceAPIKey = "YOUR_BINANCE_API_KEY"
    static let binanceSecretKey = "YOUR_BINANCE_SECRET_KEY"
    static let openAIKey = "YOUR_OPENAI_KEY"
    static let baseURL = "https://api.wiseinvest.com"
}
```

### 环境变量

```bash
# .env
BINANCE_API_KEY=your_binance_api_key
BINANCE_SECRET_KEY=your_binance_secret_key
BINANCE_TESTNET=true
OPENAI_API_KEY=your_openai_api_key
REDIS_URL=redis://localhost:6379
DATABASE_URL=postgresql://user:pass@localhost/wiseinvest
```

## 📊 功能演示

### 投资对话 Agent 示例
```
用户: "帮我分析一下 BTC 和 ETH 的投资风险"

AI: "根据当前市场数据分析：
    
    BTC (比特币):
    - 当前价格: $43,250
    - 24h 波动率: 2.3%
    - 市值占比: 52%
    - 风险等级: 中等
    
    ETH (以太坊):
    - 当前价格: $2,280
    - 24h 波动率: 3.1%
    - 市值占比: 18%
    - 风险等级: 中高
    
    建议: BTC 和 ETH 相关性较高(0.78)，建议配置一些低相关性资产
    如稳定币或其他 Layer1 公链来分散风险..."
```

### 币安交易 Agent 示例
```
用户: "帮我在 BTC 价格跌到 42000 时买入 0.1 个"

AI: "收到指令，为您创建限价买单：
    
    交易对: BTC/USDT
    类型: 限价买单
    价格: $42,000
    数量: 0.1 BTC
    预计成本: $4,200 USDT
    
    当前账户余额: $15,000 USDT
    
    是否确认执行？[确认/取消]"

用户: "确认"

AI: "订单已提交成功！
    订单ID: 123456789
    状态: 等待成交
    
    我会持续监控订单状态，成交后立即通知您。"
```

## 🛡️ 安全与隐私

- **数据本地化**：敏感投资数据仅存储在用户设备
- **端到端加密**：所有网络传输使用 TLS 1.3 加密
- **API 密钥保护**：交易所 API 密钥本地加密存储，永不上传
- **最小权限原则**：仅请求必要的系统权限
- **交易二次确认**：所有交易操作需要用户明确确认
- **审计日志**：完整记录所有操作，可随时查看

## 📄 合规声明

⚠️ **重要提示**：
1. 本应用提供的所有信息和建议仅供参考，不构成投资建议
2. 加密货币投资具有极高风险，可能导致本金全部损失
3. 用户应根据自身风险承受能力做出独立的投资决策
4. 自动化交易功能需谨慎使用，建议先在测试网环境熟悉
5. 请遵守所在地区的法律法规，部分地区可能禁止加密货币交易

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
- [tRPC-Go](https://github.com/trpc-group/trpc-go) - 腾讯开源的高性能 RPC 框架
- [Binance API](https://binance-docs.github.io/apidocs/) - 币安交易所 API
- [OpenAI](https://openai.com/) - AI 语言模型
- [Charts](https://github.com/danielgindi/Charts) - iOS 图表库

---

**让加密货币投资更智能，让交易决策更理性** 🚀
