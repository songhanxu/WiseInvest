# WiseInvest Backend

基于 Go 的高性能 AI 投资助手后端服务。

## 技术栈

- **Go 1.21+**: 主要编程语言
- **Gin**: HTTP Web 框架
- **GORM**: ORM 框架
- **PostgreSQL**: 主数据库
- **Redis**: 缓存和会话存储
- **OpenAI API**: AI 对话能力

## 架构设计

项目采用 **Clean Architecture** + **DDD (Domain-Driven Design)** 架构：

```
backend/
├── cmd/                    # 应用入口
│   └── server/
│       └── main.go
├── internal/               # 内部代码
│   ├── adapter/           # 适配器层
│   │   ├── api/          # HTTP API
│   │   │   ├── handler/  # 请求处理器
│   │   │   ├── middleware/ # 中间件
│   │   │   └── router.go
│   │   └── repository/   # 数据仓库实现
│   ├── application/       # 应用层
│   │   └── service/      # 业务服务
│   ├── domain/            # 领域层
│   │   ├── agent/        # Agent 领域
│   │   └── model/        # 领域模型
│   └── infrastructure/    # 基础设施层
│       ├── cache/        # 缓存
│       ├── config/       # 配置
│       ├── database/     # 数据库
│       ├── llm/          # LLM 客户端
│       └── logger/       # 日志
├── go.mod
├── go.sum
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

### 架构层次说明

1. **Domain Layer (领域层)**
   - 核心业务逻辑
   - Agent 接口和实现
   - 领域模型定义
   - 不依赖任何外部框架

2. **Application Layer (应用层)**
   - 业务用例编排
   - 服务协调
   - 事务管理

3. **Adapter Layer (适配器层)**
   - HTTP API 处理
   - 数据库访问
   - 外部服务集成

4. **Infrastructure Layer (基础设施层)**
   - 技术实现细节
   - 第三方库封装
   - 配置管理

## 快速开始

### 前置要求

- Go 1.21+
- PostgreSQL 13+
- Redis 6+
- OpenAI API Key

### 本地开发

1. **克隆项目**
```bash
git clone https://github.com/songhanxu/WiseInvest
cd WiseInvest/backend
```

2. **安装依赖**
```bash
make deps
```

3. **配置环境变量**
```bash
cp .env.example .env
# 编辑 .env 文件，填入你的配置
```

4. **启动数据库（使用 Docker）**
```bash
docker-compose up -d postgres redis
```

5. **运行服务**
```bash
make run
```

服务将在 `http://localhost:8080` 启动。

### 使用 Docker

```bash
# 构建并启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f backend

# 停止服务
docker-compose down
```

## API 文档

### 健康检查

```bash
GET /health
```

### Agent 相关

```bash
# 获取可用的 Agent 列表
GET /api/v1/agents

# 创建对话
POST /api/v1/conversations
{
  "user_id": 1,
  "agent_type": "investment_advisor",
  "title": "My Conversation"
}

# 获取对话详情
GET /api/v1/conversations/:id

# 获取用户的所有对话
GET /api/v1/conversations/user/:userId

# 发送消息
POST /api/v1/messages
{
  "conversation_id": 1,
  "content": "帮我分析一下 BTC 的投资风险"
}

# 发送消息（流式响应）
POST /api/v1/messages/stream
{
  "conversation_id": 1,
  "content": "帮我分析一下 BTC 的投资风险"
}
```

## 开发指南

### 添加新的 Agent

1. 在 `internal/domain/agent/` 创建新的 Agent 实现
2. 实现 `Agent` 接口
3. 在 `factory.go` 中注册新的 Agent
4. 更新 `GetAvailableAgents()` 方法

示例：

```go
// internal/domain/agent/my_agent.go
type MyAgent struct {
    llmClient *llm.OpenAIClient
    logger    *logger.Logger
}

func (a *MyAgent) GetType() string {
    return "my_agent"
}

func (a *MyAgent) GetSystemPrompt() string {
    return "你是一个..."
}

func (a *MyAgent) Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
    // 实现处理逻辑
}
```

### 代码规范

- 遵循 Go 官方代码规范
- 使用 `golangci-lint` 进行代码检查
- 所有公开函数必须有注释
- 单元测试覆盖率 > 80%

### 测试

```bash
# 运行所有测试
make test

# 运行测试并生成覆盖率报告
make test-coverage

# 运行 linter
make lint
```

## 部署

### 环境变量配置

生产环境必须配置以下环境变量：

- `OPENAI_API_KEY`: OpenAI API 密钥
- `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`: 数据库配置
- `REDIS_HOST`, `REDIS_PORT`: Redis 配置
- `JWT_SECRET`: JWT 密钥（生产环境必须修改）

### 性能优化

- 使用 Redis 缓存频繁访问的数据
- 数据库连接池配置
- 启用 GZIP 压缩
- 使用 CDN 加速静态资源

## 监控和日志

- 日志格式：JSON
- 日志级别：debug, info, warn, error
- 监控指标：请求延迟、错误率、吞吐量

## 贡献指南

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

## 许可证

MIT License
