# 股票行情与统一自选列表

美股个股报价、搜索和 K 线优先使用 Bitget Reality 股票代币。保留腾讯股票报价、搜索和新浪日 K 作为兜底；不把日 K 冒充分钟 K。周 K 由日 K 按 UTC 周一聚合，30 分钟 K 由 15 分钟 K 聚合。每条报价返回 `source`、`provider_symbol`、`currency`、`product_type` 和 `change_basis`。Bitget 涨跌幅为滚动 24 小时，腾讯兜底为相对昨收。

真正的道指、标普和纳指指数继续使用原来的腾讯源。股票代币 ETF 的价格不是指数点位，不能互相替换。

新增 `hk_stock` 市场，使用 Bitget 的 HKD Quanto 永续合约，包括腾讯、小米、联想、MiniMax 和智谱。搜索读取实际已上架、非 offline 的合约。港股报价单位是 HKD，结算单位是 USDT；这些是合约行情，不是港交所现货。没有上架的港股不构造虚假交易对。

## API 与自选存储

- `GET /api/v1/stocks/search?market=hk_stock&q=腾讯`
- `GET /api/v1/stocks/quote?market=us_stock&code=AAPL`
- `GET /api/v1/stocks/kline?market=us_stock&code=AAPL&period=1w&limit=60`
- `GET /api/v1/stocks/watchlist/all`：需要 JWT，仅汇总当前用户已有的各市场自选记录。
- `POST /api/v1/stocks/watchlist` / `DELETE /api/v1/stocks/watchlist`：沿用现有存储，Web 与 iOS 使用同一后端账户时共享自选。
- `GET /api/v1/stocks/fundamentals?market=us_stock&code=AAPL`：独立读取基本面，慢接口不阻塞自选及报价。

上游报价失败不会把自选删除或隐藏。无报价记录返回 `quote_available: false`，页面显示「—」。统一列表兼容旧版股票代码，并按市场和代码去重。

## EPS、估值与财报数据

Bitget 官方行情及股票资料接口提供交易行情、公司名称和交易时段，不提供这些公司财务指标。没有配置数据源时返回 JSON `null`，不生成或估算财报数据。

可在后端环境中配置 `FMP_API_KEY`，使用 [FMP 官方 API](https://site.financialmodelingprep.com/developer/docs)：

- `income-statement-ttm`：稀释 EPS TTM 和报表币种。
- `ratios-ttm`：标的股票 PE TTM。
- `analyst-estimates?period=annual`：未来年度 EPS 共识，页面标明年度期末。
- `profile`：股票价格及其报价币种；只有与 EPS 币种一致时才计算远期 PE。
- `earnings`：未来财报公告日期，供应商日期显示为预计日期。

EPS 和 PE 属于标的股票，不是代币或合约指标；远期 PE 使用年度分析师预期。不同市场的数据覆盖与访问权限取决于 FMP 订阅。

「最新财报预期 EPS」专指公司管理层指引，不用分析师共识替代。公司指引可通过 `STOCK_FUNDAMENTALS_FILE` 引入经核验的结构化财报记录。将 `backend/stock-fundamentals.example.json` 复制到自己的数据文件，按 `market:code` 编写；例子全部为空，不包含模拟财务数据。

填写 `source`、`as_of`；公司指引还必须有 `guidance_source_url`（财报/投资者关系原文）、`guidance_report_date`、`guidance_period`、`guidance_basis`（GAAP/Non-GAAP 等）。EPS 指引支持单值或上下界。只有公司已确认的发布日期才设置 `earnings_date_status: confirmed`。文件在每次基本面请求中读取，方便补充或刷新；需定期更新以保持对应最新财报。基金、ETF 和加密货币不展示公司 EPS 指引。

## Bitget 自选与实际账户 API 查询结论

截至 2026-10-14，在官方公开文档中没有找到读取 Bitget App 星标自选列表的受支持接口。因此本项目继续维护自己的自选，不使用未公开的 App 内部接口。

实际账户数据有官方私有接口：

| 数据 | 接口 | 说明 |
| --- | --- | --- |
| UTA 总权益、余额、股票代币持仓、未实现盈亏 | `GET /api/v3/account/assets` | UTA management 读取权限；rToken 复用该接口 |
| UTA 资金账户 | `GET /api/v3/account/funding-assets` | UTA management 读取权限 |
| UTA 合约持仓 | `GET /api/v3/position/current-position?category=USDT-FUTURES` | UTA trade 读取权限；包括港股 Quanto |
| 经典现货账户余额 | `GET /api/v2/spot/account/assets` | 对应 Classic account |

私有接口需要 API Key、Secret 和 Passphrase，并按官方规则签名。未来接入应使用服务端保存的只读凭证并绑定具体 WiseInvest 用户，不能把一套全局交易所凭证暴露给所有登录用户，也不能在浏览器保存 Secret。本次仅实现公共行情，未接入用户的私有资产。

官方依据：[公开行情](https://www.bitget.com/docs/catalog/market/market-data)、[股票参考资料](https://www.bitget.com/docs/catalog/reality/market-data)、[账户资产](https://www.bitget.com/docs/catalog/account/assets-balance)、[合约持仓](https://www.bitget.com/docs/catalog/trading/position-management)、[认证与签名](https://www.bitget.com/docs/classic/rest-api)。

## 本地验证与上线

在 `backend` 中运行 `go run ./cmd/server`，在 `frontend` 中运行 `npm run dev`。前端「设置」可选择本地后端 `http://127.0.0.1:8080`；默认仍使用腾讯云实例。仅更新前端不会让尚未部署的远端后端获得港股和 Bitget 功能，需要同时部署后端代码。

回归测试覆盖混合行情兜底、HKD/USDT 单位、股票身份、分钟/周 K 聚合、离线行情下保留自选、公司指引可追溯性、基本面币种不混算及缺失值。
