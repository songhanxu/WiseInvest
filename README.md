# 慧投 WiseInvest

> AI 驱动的多市场投资分析助手——覆盖 A 股、美股、加密货币三大市场，实时调用行情数据，结合 LLM 生成专业投资分析报告

[![iOS](https://img.shields.io/badge/iOS-16.0+-blue.svg)](https://developer.apple.com/ios/)
[![Swift](https://img.shields.io/badge/Swift-5.9+-orange.svg)](https://swift.org/)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8.svg)](https://golang.org/)

## 功能特性

### 三大市场 AI 分析 Agent

| 市场 | Agent | 数据来源 |
|------|-------|---------|
| A 股 | AShareAgent | 腾讯股票 API（实时行情）、东方财富（板块/基本面/名称搜索）|
| 美股 | USStockAgent | Yahoo Finance Chart API |
| 币圈 | CryptoAgent | CoinGecko Public API |

每个 Agent 支持两条执行路径：
- **Path A（Tool Calling）**：模型原生支持工具调用时，由 LLM 自主决定调用哪些 Skill、何时调用
- **Path B（并发预取）**：不支持 Tool Calling 的模型（如 DeepSeek-Reasoner），自动并发预取相关数据后注入 System Prompt

### Skill 系统

| Skill | 说明 |
|-------|------|
| `web_search` | Serper.dev 实时搜索，用于获取最新新闻和公告 |
| `get_ashare_price` | A 股实时行情（腾讯 API，GBK 解码） |
| `get_ashare_sectors` | A 股行业/概念板块涨跌排行（东方财富） |
| `get_ashare_fundamentals` | A 股个股基本面（PE、PB、市值、换手率、52周区间）|
| `lookup_ashare_code` | 通过股票名称搜索代码（东方财富搜索 API） |
| `get_us_stock_price` | 美股实时行情（Yahoo Finance） |
| `get_crypto_price` | 加密货币价格（CoinGecko） |

### 智能名称解析

输入"分析特变电工"无需提供代码——Agent 自动：
1. 提取股票名称
2. 调用 `lookup_ashare_code` 解析为 6 位代码（URL encoding 处理中文）
3. 并发拉取实时价格 + 基本面数据
4. 结合搜索结果交给 LLM 完成分析

### iOS 客户端

- **SwiftUI + Combine** 构建，支持 iOS 16+
- 流式 SSE 消息推送（边生成边展示）
- Markdown 渲染（h1–h6 标题、粗体、斜体、代码块、列表、引用块、分割线）
- 对话历史本地持久化（UserDefaults），AI 自动生成"动词+名词"对话标题
- 流式生成时自动滚动到底部

## 技术架构

```
┌──────────────────────────────────────────┐
│            iOS Client (SwiftUI)           │
│  HomeView → ConversationView             │
│  SSE streaming / Markdown rendering      │
└────────────────────┬─────────────────────┘
                     │ REST + SSE
                     ▼
┌──────────────────────────────────────────┐
│           Go Backend (Gin)               │
│                                          │
│  ┌──────────┐  ┌──────────┐  ┌────────┐ │
│  │ AShare   │  │ USStock  │  │ Crypto │ │
│  │  Agent   │  │  Agent   │  │  Agent │ │
│  └────┬─────┘  └────┬─────┘  └───┬────┘ │
│       │              │            │      │
│  ┌────▼──────────────▼────────────▼────┐ │
│  │            Skill Registry           │ │
│  │  web_search / price / fundamentals  │ │
│  └──────────────────────────────────── ┘ │
│                                          │
│  PostgreSQL · Redis · LLM (OpenAI-compat)│
└──────────────────────────────────────────┘
```

## 快速开始

### 依赖环境

- Go 1.21+
- PostgreSQL 15+
- Redis 7+
- Xcode 15+（iOS 开发）

### 1. 安装基础服务（macOS）

```bash
brew install postgresql@15 redis
brew services start postgresql@15
brew services start redis
```

### 2. 配置后端

```bash
cd backend
cp .env.example .env
```

编辑 `.env`，填入以下关键配置：

```bash
# LLM（兼容 OpenAI 接口，支持 DeepSeek / GPT / 其他）
OPENAI_API_KEY=your_api_key
OPENAI_BASE_URL=https://api.deepseek.com/v1   # 或其他兼容地址
OPENAI_MODEL=deepseek-chat

# 搜索（用于实时新闻，可在 serper.dev 免费注册）
SERPER_API_KEY=your_serper_api_key

# 数据库
DB_HOST=localhost
DB_USER=wiseinvest
DB_PASSWORD=wiseinvest
DB_NAME=wiseinvest
```

### 3. 启动服务

```bash
./start.sh
```

脚本会自动初始化数据库并编译启动后端服务（默认监听 `:8080`）。

### 4. 配置 iOS 客户端

打开 `ios/WiseInvest/WiseInvest.xcodeproj`，在 `APIConfig.swift` 中修改后端地址：

```swift
static let baseURL = "http://localhost:8080"   // 本机调试
// static let baseURL = "https://your-tunnel.ngrok.io"  // 真机调试
```

在 Xcode 中选择模拟器或连接的真机，`⌘R` 运行即可。

## 项目结构

```
WiseInvest/
├── backend/
│   ├── cmd/server/          # 入口
│   ├── internal/
│   │   ├── adapter/api/     # HTTP 路由与 Handler
│   │   ├── application/     # 业务服务层
│   │   ├── domain/agent/    # 各市场 Agent 实现
│   │   └── infrastructure/
│   │       ├── llm/         # LLM 客户端（Tool Calling / 流式）
│   │       ├── skill/       # Skill 实现
│   │       └── search/      # Serper 搜索封装
│   └── .env.example
├── ios/WiseInvest/          # SwiftUI iOS 客户端
├── start.sh                 # 一键启动脚本
└── README.md
```

## 环境变量说明

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `OPENAI_API_KEY` | LLM API Key（必填） | — |
| `OPENAI_BASE_URL` | LLM 接口地址 | `https://api.openai.com/v1` |
| `OPENAI_MODEL` | 使用的模型 | `gpt-4-turbo-preview` |
| `SERPER_API_KEY` | Serper 搜索 API Key | — |
| `SERVER_PORT` | 后端端口 | `8080` |
| `DB_*` | PostgreSQL 连接配置 | `localhost:5432` |
| `REDIS_*` | Redis 连接配置 | `localhost:6379` |
| `BINANCE_API_KEY` | 币安 API（交易功能，可选） | — |

## 免责声明

本项目所有输出内容仅供参考，不构成投资建议。股市和加密货币市场存在较高风险，请根据自身情况独立判断，并在做出投资决策前咨询专业顾问。
