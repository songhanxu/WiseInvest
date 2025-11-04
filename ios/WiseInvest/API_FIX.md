# ✅ API 路由问题已修复

## 问题说明

后端返回 404 错误:
```
{"status":404,"path":"/api/v1/chat"}
```

**原因**: iOS 客户端的 API 路径与后端不匹配。

## 🔧 后端 API 结构

后端实际的 API 端点:

```
POST /api/v1/conversations
- 创建新对话
- Body: {"user_id": uint, "agent_type": string, "title": string}
- Response: {"id": uint, ...}

POST /api/v1/messages/stream
- 发送消息并获取流式响应
- Body: {"conversation_id": uint, "content": string}
- Response: SSE 流式数据
```

## ✅ 已执行的修复

### 1. 更新 APIClient.swift

**修改前**:
```swift
// 错误的端点
func sendChatMessage(
    agentType: AgentType,
    message: String,
    conversationHistory: [Message]
) -> AnyPublisher<String, Error>

// 请求: POST /api/v1/chat
```

**修改后**:
```swift
// 新增: 获取或创建对话
func getOrCreateConversation(agentType: AgentType) -> AnyPublisher<UInt, Error>
// 请求: POST /api/v1/conversations

// 更新: 发送消息
func sendChatMessage(
    conversationId: UInt,
    message: String
) -> AnyPublisher<String, Error>
// 请求: POST /api/v1/messages/stream
```

### 2. 更新 ConversationRepository.swift

添加了获取对话 ID 的方法:
```swift
protocol ConversationRepository {
    func getOrCreateConversation(agentType: AgentType) -> AnyPublisher<UInt, Error>
    func sendMessage(conversationId: UInt, message: String) -> AnyPublisher<String, Error>
    // ...
}
```

### 3. 更新 ConversationRepositoryImpl.swift

实现了对话 ID 缓存:
```swift
class ConversationRepositoryImpl: ConversationRepository {
    private var conversationIdCache: [AgentType: UInt] = [:]
    
    func getOrCreateConversation(agentType: AgentType) -> AnyPublisher<UInt, Error> {
        // 先检查缓存
        if let cachedId = conversationIdCache[agentType] {
            return Just(cachedId).setFailureType(to: Error.self).eraseToAnyPublisher()
        }
        // 创建新对话并缓存 ID
        return apiClient.getOrCreateConversation(agentType: agentType)
            .handleEvents(receiveOutput: { [weak self] id in
                self?.conversationIdCache[agentType] = id
            })
            .eraseToAnyPublisher()
    }
}
```

### 4. 更新 ConversationViewModel.swift

在初始化时获取对话 ID:
```swift
class ConversationViewModel: ObservableObject {
    private var conversationId: UInt?
    
    init(agentType: AgentType, conversationRepository: ConversationRepository) {
        // ...
        
        // 获取或创建对话
        conversationRepository.getOrCreateConversation(agentType: agentType)
            .receive(on: DispatchQueue.main)
            .sink(
                receiveCompletion: { [weak self] completion in
                    if case .failure(let error) = completion {
                        self?.errorMessage = "Failed to initialize conversation"
                    }
                },
                receiveValue: { [weak self] id in
                    self?.conversationId = id
                }
            )
            .store(in: &cancellables)
    }
    
    func sendMessage() {
        guard let conversationId = conversationId else {
            errorMessage = "Conversation not initialized"
            return
        }
        
        // 使用 conversationId 发送消息
        conversationRepository.sendMessage(
            conversationId: conversationId,
            message: messageText
        )
        // ...
    }
}
```

## 🔄 新的工作流程

1. **用户打开对话页面**
   - ViewModel 初始化
   - 调用 `getOrCreateConversation(agentType:)`
   - 后端创建新对话,返回 conversation_id
   - conversation_id 被缓存

2. **用户发送消息**
   - 使用缓存的 conversation_id
   - 调用 `sendMessage(conversationId:message:)`
   - 后端返回流式响应
   - UI 实时显示 AI 回复

## 📊 API 请求示例

### 创建对话
```http
POST http://localhost:8080/api/v1/conversations
Content-Type: application/json

{
  "user_id": 1,
  "agent_type": "investment_advisor",
  "title": "Investment Advisor Conversation"
}

Response:
{
  "id": 123,
  "user_id": 1,
  "agent_type": "investment_advisor",
  "title": "Investment Advisor Conversation",
  "created_at": "2024-11-04T11:00:00Z"
}
```

### 发送消息
```http
POST http://localhost:8080/api/v1/messages/stream
Content-Type: application/json
Accept: text/event-stream

{
  "conversation_id": 123,
  "content": "What are the best investment strategies?"
}

Response (SSE):
data: {"content": "Based"}

data: {"content": " on"}

data: {"content": " your"}

data: [DONE]
```

## ✅ 验证修复

### 1. 重新构建

```bash
# 在 Xcode 中:
# Clean: ⇧⌘K
# Build: ⌘B
```

### 2. 确保后端运行

```bash
cd /Users/songhanxu/WiseInvest/backend
./start.sh
```

### 3. 运行 iOS 应用

```bash
# 在 Xcode 中:
# Run: ⌘R
```

### 4. 测试对话

1. 点击 "Investment Advisor"
2. 等待对话初始化(应该很快)
3. 输入消息: "What are the best investment strategies?"
4. 查看流式响应

## 🎯 预期结果

- ✅ 不再有 404 错误
- ✅ 对话成功创建
- ✅ 消息成功发送
- ✅ AI 回复流式显示

## 🐛 如果仍有问题

### 检查后端日志

```bash
# 查看后端日志
cd /Users/songhanxu/WiseInvest/backend
tail -f logs/app.log
```

### 检查网络请求

在 Xcode Console 中查看:
- 对话创建请求
- 消息发送请求
- 响应状态码

### 常见问题

**Q: 仍然看到 404 错误**

A: 
1. 确认后端正在运行
2. 检查 baseURL 是否正确 (http://localhost:8080)
3. 查看后端日志确认路由注册

**Q: 对话创建失败**

A:
1. 检查数据库是否正常运行
2. 查看后端日志中的错误信息
3. 确认 user_id 和 agent_type 格式正确

**Q: 消息发送失败**

A:
1. 确认 conversation_id 已正确获取
2. 检查消息内容是否为空
3. 查看后端日志中的 LLM 调用情况

## 📚 相关文件

修改的文件:
- `Data/Network/APIClient.swift` - API 客户端
- `Domain/Repository/ConversationRepository.swift` - 仓储协议
- `Data/Repository/ConversationRepositoryImpl.swift` - 仓储实现
- `Presentation/Conversation/ConversationViewModel.swift` - 视图模型

## 🎉 总结

**问题**: API 路径不匹配导致 404 错误

**解决**: 
1. 更新 API 客户端以匹配后端路由
2. 实现对话创建和 ID 缓存
3. 更新 ViewModel 使用新的 API 流程

**结果**: iOS 应用现在可以正确与后端通信! 🚀

---

**最后更新**: 2024-11-04
**状态**: ✅ 已修复
