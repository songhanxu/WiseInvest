# 聊天模型与思考强度

普通市场聊天和个股聊天可逐条选择 `DeepSeek Flash` 或 `DeepSeek V4 Pro`，以及 `关闭`、`低`、`高`、`Max` 四档思考强度。Web 在每个本地会话中保存选择；iOS 和微信小程序在设备上记住最近一次选择。默认值为 Flash + 高。

服务端需要配置官方 DeepSeek 地址、`OPENAI_API_KEY` 和 `OPENAI_MODEL_PRO=deepseek-v4-pro`，才能使用 Pro 选项。请求发送到 `POST /api/v1/messages` 或 `POST /api/v1/messages/stream` 时，可带 `model` 和 `reasoning_effort` 字段。后端只接受 `deepseek-flash`、已配置的 `deepseek-v4-pro`，以及 `none`、`low`、`high`、`max`。旧客户端不传这两个字段时，继续使用后端的默认模型设置。

`none` 发送 `thinking: {"type":"disabled"}`；其他档位发送 `thinking: {"type":"enabled"}` 和相应的 `reasoning_effort`。开启思考的市场聊天先获取相关行情和搜索资料，再让模型分析。高强度可能增加响应时间和费用。

财报解读仍按 [财报解读配置](filing-insights.md) 独立使用 Pro + Max，不受聊天页面的选择影响。
