# 联网搜索

当 `OPENAI_BASE_URL` 指向官方 `https://api.deepseek.com`（可带 `/v1`）且配置 `OPENAI_API_KEY` 时，后端自动优先使用 [DeepSeek Search API](https://api-docs.deepseek.com/guides/search/)。它是独立的 `POST /v1/tools/search` 接口，不要求把聊天切换到 Responses API，也不要求将模型换成 V4 Pro。现有 `web_search` 工具、行情 Agent、圆桌、财报新闻和自选新闻都共用这个搜索入口。

可选的 `SEARCH_PROVIDER=serper` 或 `brave` 与 `SEARCH_API_KEY` 作为备选：DeepSeek 请求失败或没有结果时才使用；如果模型服务商不是官方 DeepSeek，这组配置就是主搜索源。没有任何可用搜索源时，依照现有逻辑只使用 SEC、行情等已获取的资料。搜索结果仍只是新闻线索，财报和价格数据应优先以原始来源为准。

DeepSeek Search 和模型调用使用同一把后端 API Key，但分别计费和限流；费用及限额请以[官方文档](https://api-docs.deepseek.com/guides/search/)为准。后端重启或部署新版本后此优先级才生效。前端无需改动。
