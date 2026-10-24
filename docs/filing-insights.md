# 自选列表：最新财报与每日 AI 解读

自选列表中，SEC 能识别的美股定期申报会显示在“当前最新财报”列。日期分为报告期末和 SEC 提交日。点击申报条目可查看六项标准化财务摘要、各科目的实际期间与单位，并可打开 SEC 原始全文。星形图标打开该份报告的 AI 解读。

每天首次打开自选列表时，前端为当前账户的美股自选请求解读。后端按用户、上海时区日期和股票代码去重，最多同时处理两只股票；部署环境用 Redis 跨后端实例共享结果。新的一天会重新生成。如果当天出现新财报，点开其 AI 图标会核对 SEC 最新申报，并替换对应旧版解读。失败结果十分钟后可重新提交，弹窗也提供立即重试。

后端生产环境需配置：

```dotenv
SEC_FUNDAMENTALS_ENABLED=true
SEC_USER_AGENT=WiseInvest your-contact@example.com
OPENAI_API_KEY=your-model-key
OPENAI_BASE_URL=https://your-openai-compatible-endpoint/v1
OPENAI_MODEL=your-model-name
FILING_INSIGHT_MAX_TOKENS=8192
FILING_INSIGHT_RETRY_MAX_TOKENS=16384
FILING_INSIGHT_TIMEOUT=180s
FILING_INSIGHT_THINKING=auto
```

`SEC_USER_AGENT` 必须填写实际项目名和联系邮箱。模型密钥只放在后端 `backend/.env.production` 或服务器环境变量里，绝不放进前端。`docker-compose.yml` 已通过 `env_file` 读取该文件。使用官方 DeepSeek API 地址和有效模型密钥时，新闻搜索优先调用 DeepSeek Search；可继续配置 `SEARCH_PROVIDER=serper` / `brave` 和 `SEARCH_API_KEY` 作为失败或无结果时的备选。其他模型服务商仍使用原来的搜索配置。

财报解读只在模型返回非空正文且 `finish_reason=stop` 时标记成功。若首次返回 `length`，会用同一份资料和更紧凑的输出要求重试一次；第二次仍截断则记为失败，绝不保存截断内容。上述令牌数是请求上限，应按实际模型支持范围调整；`FILING_INSIGHT_RETRY_MAX_TOKENS` 不得小于首次上限。`FILING_INSIGHT_TIMEOUT` 是两次尝试共享的总时限。`FILING_INSIGHT_THINKING=auto` 对官方 DeepSeek V4/Flash 的这类财报解读请求关闭思考模式，对其他模型不增加 DeepSeek 专用参数；普通聊天不受此设置影响。

生成时先从 SEC Submissions 确认最新 10-K、10-Q、20-F 或 40-F，再取该申报的 Company Facts、尝试读取 SEC 原文，并对照腾讯美股与标普 500 日线。SEC 原文无法读取时，解读会标注正文缺失，避免把结构化摘要写成完整财报。历史行情若不覆盖财报提交前一日，不能计算财报后累计涨跌；新闻搜索结果只作为可能原因的线索。AI 被要求区分事实、推断和不确定性，比较同口径期间，给出未来情景及有条件的买入、持有、减仓或卖出倾向。解读是研究意见，生成后行情与公告会变化。

接口均需登录：

- `GET /api/v1/stocks/watchlist/filing-insights`：读取当前用户今日自选解读状态和内容。
- `POST /api/v1/stocks/watchlist/filing-insights/refresh`：空 JSON `{}` 提交今日全部美股自选；`{"code":"AAPL"}` 仅提交一只；`{"code":"AAPL","retry":true}` 重试失败结果。新财报时带 `retry:true` 和已展示财报的 `accession`，后端核对 SEC 后才替换旧解读。

当前覆盖范围是 SEC 可识别的美股上市公司，其他市场在该列显示「—」。财报详情页内的标准化摘要是六项；完整正文通过 SEC 官方原文链接查看。若 SEC、模型或行情数据源暂时不可达，解读会显示失败原因并允许重试，不会伪造数值或走势。

若弹窗提示模型失败，先在服务器检查后端日志中的 `SEC filing insight model request failed`。新版会在弹窗区分鉴权失败（401）、模型或地址不存在（404）、限流或额度（429）、请求不兼容（400）、超时和空回复；日志保留服务商返回的具体错误。可使用 `docker compose -f docker-compose.yml logs --tail=200 backend` 查看，不要将 `OPENAI_API_KEY` 的值贴到聊天或日志里。已有失败解读会短暂缓存；部署修复后点击“重新生成”可立即重试。
