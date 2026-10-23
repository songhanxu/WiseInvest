# Stocksera 数据获取与 WiseInvest 覆盖对比

检查日期：2026-10-22。参考目录：`For reference only/Stocksera`；当前项目：`WiseInvest`。结论依据两个工作副本中的代码，未把 README 的宣传功能或 AI 能搜索到的内容当成已经接入的结构化数据。参考项目保持只读。

## EPS 和财报问题

原来的 `backend/internal/adapter/api/handler/stock_fundamentals.go` 只有 FMP 在线数据源。只有配置 `FMP_API_KEY` 才调用上游；TTM EPS、PE、分析师预期、财报日期分别调用不同的 FMP 接口，任意一个接口无订阅权限都会导致对应字段缺失。行情接口使用 Bitget / 腾讯，不会因此自动取得财务数据。`STOCK_FUNDAMENTALS_FILE` 是人工核验后的补充文件，不是自动财报抓取程序。

当前列表中的“下份财报时间”是发布日期，不是完整财务报表。原项目没有可读取完整资产负债表、利润表、现金流量表的接口。

Stocksera 把这些数据分给不同来源：

| 内容 | 参考项目获取方法 | 缓存 / 存储 | 代码证据 |
| --- | --- | --- | --- |
| 个股基本面、EPS 等统计 | `yfinance.Ticker(symbol).info`；页面读取信息字段 | `database/yf_cached_api.json`，约 10 分钟 | `helpers.py::check_market_hours`、`static/javascript/stock/ticker_price.js` |
| 季度资产负债表 | `ticker.quarterly_balance_sheet`，提取报表日期、科目、数值 | `database/financials.json`，记录一周后的下次更新日期；另有批处理脚本 | `helpers.py::check_financial_data`、`scheduled_tasks/stocks/get_financial.py` |
| 未来财报日历 | Finnhub `earnings_calendar`，分三段查询未来三周；读取发布日期、盘前盘后、季度 EPS 实际/预期、营收实际/预期 | MySQL `earnings`；开发指南建议每日更新 | `scheduled_tasks/discover/get_earnings.py` |
| 财报日历的市值补充 | FMP `api/v3/quote` | 与财报事件合并 | `helpers.py::get_ticker_list_stats` |
| SEC 原始文件链接 | Finnhub `filings`，查询约三年、最多 100 条 | MySQL `sec_fillings` | `helpers.py::get_sec_fillings` |

参考实现存在应避免继承的行为：缺失资产负债表值被替换为零；财报日历只保留长度不超过四位的股票代码；任务先清空日历表再请求数据。其 `get_financial.py` 使用旧的 `requests_cache.CachedSession`，当前 yfinance 源码已经拒绝这种缓存 session。因此应参考来源和字段口径，不能直接复制旧依赖用法。

## 本分支完成的调整

- 移除 FMP 的基本面请求、未使用的行情辅助代码与密钥读取；美股实际披露 EPS 和财务摘要使用 SEC EDGAR 为主，Yahoo Finance 补充估值、预期、缺失字段和其他市场。
- Yahoo 使用后端独立 cookie / crumb 会话，跨股票共享会话；认证拒绝后最多刷新一次，会话初始化失败退避。支持美股、港股、沪深及北交所代码映射，实际覆盖取决于供应商。
- 配置 `FINNHUB_API_KEY` 后并行补充美股未来四个月财报日历，只使用匹配代码、合法且未过期的日期。供应商日期均标记预计；Yahoo 日期区间保持为区间。
- 每个有效指标分别记录实际来源、查询时间及适用币种；缓存结果在人工文件覆盖前复制，避免并发请求互相污染。
- 零 EPS 保留为零，亏损 EPS 保留负数，缺失保持 `null`。不将季度 EPS 预期、年度分析师预期填入公司管理层指引。
- 页面展示财报日期区间及远期估值口径。Yahoo 部分字段缺失时保留其余成功字段，美股日期可由 Finnhub 补充。

本分支已新增 SEC 最近八份定期申报链接与六类财务科目的摘要。完整三张报表、自定义标签、历史业绩实际 / 预期对比仍未接入，具体差异如下。

## 数据覆盖差异

“缺少”指缺少专门的结构化获取、存储或查询能力；聊天、网页搜索、新闻摘要提到同一主题不算数据集接入。来源列记录参考代码中的来源，除本分支涉及的接口外，没有逐一验证这些旧服务目前是否仍可用。

| 数据类别 | Stocksera 的内容与来源 | WiseInvest 当前状态 |
| --- | --- | --- |
| EPS、PE、远期估值 | Yahoo 个股统计，FMP 行情补充 | 美股披露 EPS 以 SEC 为主，PE / 远期估值由 Yahoo 补充；EPS 明确标记期间，未将单季值当 TTM |
| 扩展基本面统计 | Yahoo Beta、总股本 / 流通股、PEG、企业价值 / 收入、净利润、空头比例、股息率等 | A 股已有部分 PE / PB / 市值等字段，缺少跨市场统一的上述统计返回和展示 |
| 下一次财报日期 | Finnhub 财报日历 | 现由 Yahoo 提供，以及可配置的 Finnhub 美股回退 |
| 季度 EPS / 营收、实际与预期 | Finnhub 日历的 `epsActual`、`epsEstimate`、`revenueActual`、`revenueEstimate`；参考任务只查询未来三周，不能据此认为有完整历史业绩库 | 缺少单次业绩的实际 / 预期字段与超预期计算，当前只提取未来发布日期；也没有历史业绩库 |
| 季度资产负债表 | Yahoo `quarterly_balance_sheet`：日期、科目、多期数值 | 现有 SEC 定期申报链接及资产、负债、权益等标准科目摘要；缺少完整报表。参考项目的这个函数也没有提供完整三张报表 |
| SEC 文件 | Finnhub filings 的表格类型、提交日期、原文与索引链接 | 现已提供最近八份 SEC 定期申报及原文链接，尚未提供 8-K 等全部文件类型和完整历史检索 |
| 主要 / 机构 / 基金股东 | Yahoo holders 数据（`app/views.py` 的 holders 系列函数） | 缺少持股主体、数量、占比与变化数据 |
| 内部人交易 | Finviz，包含人员、关系、交易日期、买卖、金额、持股、Form 4 链接 | 缺少专门交易记录；交易所账户持仓不是上市公司内部人交易 |
| 分析师评级与同业 | Finnhub `recommendation_trends`、`company_peers` | 缺少评级分布历史和同业列表；AI 建议不等于分析师评级数据 |
| 期权链 / Max Pain | SwaggyStocks 的到期日、行权价、看涨看跌数据与 max pain | 缺少股票期权数据；已有加密 / 股票永续合约不覆盖此项 |
| 做空交易量 | FINRA 日度文件，short volume、short percentage | 缺少日度卖空成交数据 |
| 可借券数量 / 借券费率 | IBKR FTP `usa.txt`，`FEERATE`、`AVAILABLE` | 缺少借券库存及费率历史 |
| FTD / Reg SHO | SEC 未交割数据、NYSE / Nasdaq threshold securities 名单 | 缺少未交割量和监管阈值名单 |
| 高做空 / 低流通股筛选 | HighShortInterest、LowFloat 网页表格 | 缺少对应全市场数据与筛选入口 |
| 分红、拆股、IPO | Polygon 分红及拆股、Finnhub IPO calendar；Yahoo 单股分红拆股 | 缺少专用公司行动日历和历史接口 |
| ETF 专属统计 | Yahoo 总资产、NAV、三 / 五年收益、收益率等；另有 ARK 外部页面 | 当前主要识别基金不适用公司 EPS，缺少 ETF 统计和持仓数据 |
| 宏观时间序列与发布日历 | 纽约联储逆回购、美国财政部现金余额、FRED 利率、通胀、零售、初请失业与发布日期 | 缺少专用时间序列及事件日历，已有资讯与 AI 解读 |
| 社交热度与情绪 | Reddit 提及 / 情绪、Stocktwits、Twitter 讨论及关注数、Google Trends、CNN Fear & Greed | 已有股票新闻及情绪解读；缺少这些独立数据集与历史序列 |
| 政府人员交易 | Senate / House Stock Watcher 历史 JSON | 缺少议员交易数据；参考 README 本身提示这部分有故障 |
| 市场发现与比较 | 大市值榜单、指数成份股汇总、相关系数 / Beta（Yahoo 历史价计算）、Cramer 追踪、停牌 | 已有市场看板、指数行情、K 线与技术分析；缺少对应专题数据集与计算入口 |

参考项目中的额外数据普遍面向美股，不能据此推断已经覆盖 A 股和港股。WiseInvest 的跨市场行情、交易所只读持仓、价格邮件提醒和 AI 对话不在这份“缺少数据”清单内。

## 配置与验证

```dotenv
SEC_FUNDAMENTALS_ENABLED=true
SEC_USER_AGENT=
YAHOO_FUNDAMENTALS_ENABLED=true
FINNHUB_API_KEY=
STOCK_FUNDAMENTALS_FILE=
```

SEC_USER_AGENT 填写项目名和真实联系邮箱；它与 Finnhub 密钥都留在后端环境变量中。默认 Yahoo 无需密钥，但部署网络必须可访问 Yahoo；Finnhub 仍受账户权限与额度限制。FMP 已不再作为当前项目的数据源。公司指引继续通过核验文件接入，必须有原文链接、发布日期、期间、会计口径。

接口示例：`GET /api/v1/stocks/fundamentals?market=us_stock&code=AAPL`；港股使用 `market=hk_stock&code=00700`。`next_earnings_date_end` 只在供应商返回多个日期时存在。`forward_basis` 用于说明未提供明确年度期末的分析师估值口径。`metrics.*.as_of` 是本次抓取时间，不代表财报期末。

本地公开网络探测：Yahoo 的会话和 crumb 请求返回 HTTP 429，因此没有获得真实股票数据，不能声称此次已验证线上恢复。自动化验证使用本地 HTTP 服务模拟正常数据、访问受限、会话刷新、限流、部分数据和日期/币种场景；SEC 官方 ticker 映射、Submissions 和 Company Facts 请求已实测成功，并使用下载的真实响应验证 EPS 与财报摘要解析。Yahoo 的实盘连通性仍需在实际部署网络验证。

## 建议的后续顺序

1. 接入季度利润表、资产负债表、现金流量表和历史 EPS / 营收，建立统一的财报期间、币种、原始来源模型。
2. 补充历史实际 / 预期对比、SEC / 投资者关系原文与公司行动日历。
3. 根据研究需求再增加内部人 / 股东、做空 / 期权、宏观与社交数据。

当前供应商验证依据：[SEC EDGAR API 文档](https://www.sec.gov/search-filings/edgar-application-programming-interfaces)、[SEC 自动访问要求](https://www.sec.gov/about/webmaster-frequently-asked-questions)、[yfinance Quote 源码](https://github.com/ranaroussi/yfinance/blob/main/yfinance/scrapers/quote.py)、[yfinance 会话源码](https://github.com/ranaroussi/yfinance/blob/main/yfinance/data.py)、[Finnhub 财报日历文档](https://finnhub.io/docs/api/earnings-calendar)。
