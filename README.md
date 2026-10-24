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
│   │       └── search/      # DeepSeek 搜索与备用搜索源
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
| `SEC_USER_AGENT` | SEC 财报访问标识（项目名和联系邮箱） | — |
| `SEARCH_PROVIDER` / `SEARCH_API_KEY` | Serper 或 Brave 备用搜索源；官方 DeepSeek 地址自动优先使用其 Search API | — |
| `FILING_INSIGHT_*` | 财报解读的输出预算、超时和思考模式 | 见 `.env.example` |
| `SERVER_PORT` | 后端端口 | `8080` |
| `DB_*` | PostgreSQL 连接配置 | `localhost:5432` |
| `REDIS_*` | Redis 连接配置 | `localhost:6379` |
| `WECHAT_APP_ID` | 微信开放平台 AppID | — |
| `WECHAT_APP_SECRET` | 微信开放平台 AppSecret | — |
| `BINANCE_API_KEY` | 币安 API（交易功能，可选） | — |

自选列表的最新 SEC 财报和每日 AI 解读配置、数据范围见 [财报解读说明](docs/filing-insights.md)。
联网搜索的优先级和配置见 [搜索说明](docs/web-search.md)。

## 网页账户登录

网页端注册时设置账号名和密码，并通过邮箱验证码或短信验证码确认联系方式（`/api/v1/auth/register`）。登录页用同一个输入框接收账号名、邮箱或手机号，三者使用同一密码（`/api/v1/auth/login`）；已绑定的邮箱和手机号还能分别用验证码登录（`/api/v1/auth/email/login`、`/api/v1/auth/phone/login`）。注册时未填写的另一种联系方式可在「我的」页面验证后绑定到同一账户（`/api/v1/auth/email/bind`、`/api/v1/auth/phone/bind`）。账号名为 3–32 位字母、数字、下划线、点或短横线。密码以 bcrypt 哈希保存；网页登录后使用现有 JWT 访问自选、账户和对话接口。旧的微信模拟账号若未设置密码、未绑定联系方式，不能仅凭账号名登录，需先完成安全的凭证绑定或迁移。正式构建默认请求当前网站同源的 `/api/`；若 API 部署在另一域名，构建时设置 `VITE_API_BASE_URL=https://api.example.com`。登录页只允许通过 HTTPS（本机调试地址除外）提交凭证。上线前还需接入密码找回；多实例部署时需使用共享的登录限流存储。

邮箱验证码需要配置 `SMTP_HOST`、`SMTP_PORT`、`SMTP_USERNAME`、`SMTP_PASSWORD` 和 `SMTP_FROM`；未配置时返回 503。短信验证码沿用腾讯云短信配置。仅本机调试且使用现有测试账号时，可在忽略的 `frontend/.env.local` 中设置 `VITE_ALLOW_HTTP_TEST_LOGIN=true`，允许向目前的 HTTP 测试服务器提交**测试密码**；注册与验证码仍须等待 HTTPS。

手机号验证码登录使用 `POST /api/v1/auth/phone/send-code` 和 `POST /api/v1/auth/phone/login`，已登录用户仍可调用 `POST /api/v1/auth/phone/bind` 绑定手机号。验证码保存 5 分钟，并限制发送与验证次数。开发环境只有同时设置 `SERVER_MODE=debug` 和 `ALLOW_MOCK_AUTH=true` 才会在后端日志输出验证码；正式环境必须配置腾讯云短信的密钥、SdkAppID、审核通过的签名及单变量验证码模板，否则发送接口返回 503。平台配置见 `backend/.env.example`。

小程序微信登录使用 `/api/v1/auth/wechat/miniprogram/login`，后端通过 `code2Session` 校验 `wx.login` 的 code；配置 `WECHAT_MINI_APP_ID` 和 `WECHAT_MINI_APP_SECRET`。iOS 使用原有 `/api/v1/auth/wechat/login` OAuth 接口，配置独立的 `WECHAT_APP_ID` 和 `WECHAT_APP_SECRET`。iOS 还需将官方 WechatOpenSDK 加入 Xcode 工程、填写 `Info.plist` 中的 AppID、配置 Universal Link 和 Associated Domains；未完成前微信按钮不会发起模拟的正式登录。两端可在同一微信开放平台 UnionID 可用时合并为同一账户。

## 接入真实微信登录

> 未配置微信 AppID 时，微信登录不可用。仅本地调试可同时设置 `SERVER_MODE=debug` 与 `ALLOW_MOCK_AUTH=true` 开启 Mock 登录；公开服务器不要启用。

### 第一步：注册微信开放平台

1. 前往 [open.weixin.qq.com](https://open.weixin.qq.com) 注册并创建**移动应用**
2. 填写 iOS Bundle ID（与 Xcode 项目一致）
3. 等待审核通过后，获取 **AppID** 和 **AppSecret**

### 第二步：后端配置

在后端运行环境中填入（小程序另需 `WECHAT_MINI_APP_ID` 和 `WECHAT_MINI_APP_SECRET`）：

```bash
WECHAT_APP_ID=wx1234567890abcdef
WECHAT_APP_SECRET=your_32_char_secret
```

### 第三步：iOS 集成 WechatOpenSDK

**1. 下载 SDK**

前往 [微信 iOS SDK 下载页](https://developers.weixin.qq.com/doc/oplatform/Downloads/iOS.html)，下载最新版 SDK，将 `WechatOpenSDK.xcframework` 拖入 Xcode 项目，设置为 **Embed & Sign**。

**2. 修改 `Info.plist`**

将 `wx_YOUR_APP_ID` 替换为真实 AppID：

```xml
<string>wx1234567890abcdef</string>
```

**3. 配置 AppID 和 Universal Link**

在 `Info.plist` 中设置 `WeChatAppID`、`WeChatUniversalLink`，并将 `CFBundleURLSchemes` 中的占位值替换为同一个 AppID。`WeChatManager.swift` 会在 SDK 可导入且 AppID 已配置时启用微信授权；无 SDK 或配置时不会使用假登录。

**4. 配置 Universal Link（必须）**

- 在服务器根目录部署 `.well-known/apple-app-site-association` 文件
- 在 Xcode → Signing & Capabilities → Associated Domains 中添加 `applinks:hanxusong.asia`，并确保开放平台登记的 Universal Link 与 `Info.plist` 一致

## 免责声明

本项目所有输出内容仅供参考，不构成投资建议。股市和加密货币市场存在较高风险，请根据自身情况独立判断，并在做出投资决策前咨询专业顾问。
