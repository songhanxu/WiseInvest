# 股票行情与统一自选列表

美股、港股与加密货币行情优先使用 Bitget，A 股保持原数据源。美股与港股报价、搜索在 Bitget 无结果或失败时使用腾讯兜底；加密货币使用 Binance 公开行情兜底。按缺失标的逐个补齐，不覆盖已经取得的 Bitget 报价。页面保留已取得的数据，刷新失败时标注缓存。公司财报数据源保持独立。

周 K 由日 K 按 UTC 周一聚合，30 分钟 K 由 15 分钟 K 聚合。每条报价保留 `source`、`provider_symbol`、`currency`、`product_type`、`quote_time`、`quote_session` 和 `change_basis`，价格、币种及滚动 24 小时涨跌按 Bitget 原值展示。页面以普通标的列表展示股票，产品身份保留在内部以正确查询行情、K 线和分析。

主页美股概览展示 SPY、QQQ、DIA 的实际价格及其代码；港股展示 00700、01810、00992；加密货币展示 BTC、ETH。迷你走势图读取 Bitget 小时 K 线，无数据时显示缺失状态。腾讯报价时间读取上游字段并按交易所时区转换；美股常规时段收盘报价标注实际收盘日期（美东），不使用接口抓取时间。Bitget 仅在时间有效且一分钟内时显示实时；旧报价显示报价时间或缓存，时间缺失明确标为未提供。

美股日/周 K 可使用原股票日 K 源兜底，港股日/周 K 使用腾讯兜底；不把日 K 冒充分钟 K。加密货币 K 线使用 Binance 兜底。

主页和自选列表先显示已有数据，后台更新；仅首次无数据时显示与最终内容结构相同的骨架屏。

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

Bitget 官方行情及股票资料接口提供交易行情、公司名称和交易时段，不提供这些公司财务指标。基本面使用独立的数据源链路：美股已披露 EPS 和财务摘要以 SEC EDGAR 为主，Yahoo Finance 补充估值、分析师预期和未覆盖的字段 / 市场，美股未来财报日期可通过 `FINNHUB_API_KEY` 启用 Finnhub 财报日历补充。所有来源都没有数据时返回 JSON `null`。

EPS 和 PE 属于标的股票，不是代币或合约指标；远期估值使用供应商提供的分析师预期。FMP 接入已移除，后端不再读取其密钥或调用其接口。

SEC 默认启用（`SEC_FUNDAMENTALS_ENABLED=true`），通过后端 `SEC_USER_AGENT` 配置项目名和真实联系邮箱，不需要 API Key。官方 ticker → CIK 映射每日刷新；Company Facts 与 Submissions 按公司查询，客户端统一限制为每秒最多四次请求，403 / 429 后跨标的退避一分钟。联系邮箱只保存在后端环境中，不写进前端或提交到仓库。

SEC EPS 采用实际披露的基本 / 稀释 EPS，保留 `eps_basis`、`eps_period_start`、`eps_period_end` 和 `eps_filed_date`。优先使用最新期间的稀释值，同一期间采用最新已提交的修订数据；不通过四季度相加或累计值相减生成 TTM。页面 EPS 列明确标示单季、年度或累计口径，Yahoo 补充值则标明供应商 TTM。PE TTM 仍由 Yahoo 提供并标明来源，不能拿单季度 EPS 计算或解释其 TTM PE。

`financial_reports` 返回最近八份 10-K / 10-Q / 20-F / 40-F（含修订）的原始链接与财务摘要：营收、净利润、资产、负债、权益和经营现金流；每个科目分别保留币种、XBRL 标签和实际起止日期，不把现金流年初至今累计数当成单季度数。摘要只使用同一申报编号、同一报告期末的标准标签，不等于完整三张财务报表。缺失科目不补零。SEC 提交日期只用于历史申报展示，不写入“下份财报时间”。外籍 / ADR 主体的每普通股与每 ADS 换算未核验前，EPS 由 Yahoo 补充，仍可展示其 SEC 报表与标准科目。

Yahoo Finance 参考 Stocksera 的 Yahoo 数据获取方式，后端直接处理 cookie / crumb 会话，不新增 Python 运行时。设置 `YAHOO_FUNDAMENTALS_ENABLED=false` 可关闭。Yahoo 的财务币种与股票报价币种分别处理，直接使用供应商估值，不用报价币种的价格除以另一币种 EPS。供应商未提供明确年度期末时，页面标注预期口径；预计财报日期支持区间，并按交易所时区转换。Finnhub 仅查询美股未来四个月日期，其季度 EPS 共识不会被当成年度 EPS 或公司指引。

接口按指标返回 `metrics` 来源、币种与查询时间；失败原因使用受控提示，不返回上游原文或凭证。请求合并和缓存同时覆盖各数据源：核心字段齐全时缓存六小时，无数据或缺少 EPS、远期 PE、财报日期时缓存一分钟；Yahoo 会话初始化失败跨标的退避一分钟。公共 Yahoo 接口仍可能受网络、HTTP 429 或覆盖范围影响，不能视作有可用性保证的数据服务。完整数据缺口见 [Stocksera 数据覆盖对比](stocksera-data-comparison.md)。

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

私有接口需要 API Key、Secret 和 Passphrase，并按官方规则签名。实体账户功能已接入上述交易账户资产及持仓查询；经典合约使用 `GET /api/v2/mix/position/all-position?productType=USDT-FUTURES`。当前不读取资金账户余额、订单历史或 Bitget App 自选，也不下单或转账。

官方依据：[公开行情](https://www.bitget.com/docs/catalog/market/market-data)、[股票参考资料](https://www.bitget.com/docs/catalog/reality/market-data)、[账户资产](https://www.bitget.com/docs/catalog/account/assets-balance)、[合约持仓](https://www.bitget.com/docs/catalog/trading/position-management)、[认证与签名](https://www.bitget.com/docs/classic/rest-api)。

## 连接自己的 Bitget 实体账户

前端在「总览」和「自选列表」之间增加「实体账户」。账户选择器默认展示全部账户合并结果，支持切换 Bitget、币安和 Gate，配置说明见 [多交易所实体账户](exchange-accounts.md)。列表与自选共用组件，包含名称、代码、市场、价格、期望价格、EPS、当前最新财报、PE、公司指引 EPS 和下份财报时间；实体账户额外显示持仓详情，包括数量、现货/合约和方向。当前最新财报可查看 SEC 摘要、原文和 AI 解读。默认每分钟刷新，支持手动刷新。

1. 在 Bitget API 管理中创建 **HMAC 只读** API Key，保存 Key、Secret 和创建时填写的 Passphrase。UTA 需要「UTA mgt. (read)」查询资产；包含合约时还需「UTA trade (read)」。经典账户选择对应现货、合约查询权限。绑定后端实际出口公网 IP；云服务器运行时填写云服务器出口 IP。
2. 在慧投登录将使用这个账户的用户。带该用户的 Bearer token 调用 `GET /api/v1/auth/me`，取响应中的 `id`，这是慧投用户 ID，**不是 Bitget UID**。仅该用户可读取这套配置的持仓。
3. 在运行后端的环境中填写以下变量。本地已在 `backend/.env` 中预留空项；部署时填写服务器环境。不要把密钥填入前端环境、页面 JWT 输入框或 Git。

```dotenv
BITGET_API_KEY=
BITGET_API_SECRET=
BITGET_PASSPHRASE=
BITGET_OWNER_USER_ID=0
BITGET_ACCOUNT_NAME=Bitget
BITGET_ACCOUNT_TYPE=uta
BITGET_INCLUDE_FUTURES=true
```

将 `BITGET_OWNER_USER_ID` 的 `0` 替换成自己的慧投用户 ID；`0` 保持禁用。统一交易账户选 `uta`，经典账户选 `classic`，不会自动迁移账户。仅查现货时将 `BITGET_INCLUDE_FUTURES=false`。

4. 重启后端，确保前端「设置」指向该后端，再进入「实体账户」。已配置表示凭证和用户绑定完整；实际连通性以持仓请求是否成功为准。权限、IP 或账户类型不匹配会显示同步错误，而不是伪装成空账户。

新增 `GET /api/v1/accounts` 与 `GET /api/v1/accounts/bitget/holdings`，均需要 JWT。账号未绑定、凭证未填完整时不会访问 Bitget 私有接口。私有请求固定访问官方主机，拒绝重定向，不把凭证写到 URL、返回值或上游错误消息；不允许调用交易接口。

### 给外部 AI 的只读持仓接口

`GET /api/v1/automation/bitget/holdings` 使用单独的长期机器密钥，不需要慧投登录 JWT。它只返回 `BITGET_OWNER_USER_ID` 所绑定的这一个 Bitget 账户持仓快照；不能读取其他交易所、其他慧投用户，也没有交易或修改操作。未设置密钥时返回 404；密钥错误返回 401；Bitget 账户未绑定返回 503。返回体与 `GET /api/v1/accounts/bitget/holdings` 相同，包含 `holdings`、`account_equities`、`as_of`、`stale`、`complete` 和 `warnings`。外部 AI 应检查 `stale`、`complete` 与 `warnings`，避免把旧快照当作最新持仓。

在**运行后端的环境**中新增以下配置，然后重建后端容器。密钥须为 `openssl rand -hex 32` 产生的 64 位十六进制字符串，与 Bitget API Key / Secret / Passphrase 不同。更换此值并重建容器即可撤销旧密钥。

```dotenv
BITGET_HOLDINGS_ACCESS_KEY=生成的64位十六进制字符串
```

调用时只在请求头中传入密钥，切勿放在 URL 查询参数、AI 提示词、聊天记录或 Git 仓库。若 AI 平台支持 Secret / 环境变量，把它存在那里，并让工具调用时填入请求头：

```bash
curl --fail-with-body --silent --show-error \
  -H "Authorization: Bearer $BITGET_HOLDINGS_ACCESS_KEY" \
  'https://你的域名/api/v1/automation/bitget/holdings'
```

公网访问必须走 HTTPS。仓库内默认 Nginx 配置目前只监听 80 端口，必须先配置 HTTPS 终止并让可信反向代理把实际协议以 `X-Forwarded-Proto: https` 转发给后端。当前 HTTP IP 直连地址不能用于这个接口；不要尝试在 HTTP URL 上发送机器密钥。后端仅信任本机或 Docker 内网代理的协议标头，外部请求自行伪造该标头不会绕过检查。`BITGET_OWNER_USER_ID` 应继续填写实际绑定这套 Bitget 凭证的慧投用户 ID；如管理员账户 ID 为 2，就设为 `2`。

如果另一个 AI 在你本地电脑上运行，且服务器暂时没有 HTTPS，可以走 SSH 隧道。先在本地终端保持下面的隧道运行，再让 AI 的工具访问本机地址；HTTP 只在本机环回和服务器内部传输，公网段走 SSH 加密：

```bash
ssh -N -L 127.0.0.1:18080:127.0.0.1:8080 root@119.91.58.30
```

```bash
curl --fail-with-body --silent --show-error \
  -H "Authorization: Bearer $BITGET_HOLDINGS_ACCESS_KEY" \
  'http://127.0.0.1:18080/api/v1/automation/bitget/holdings'
```

本地隧道仅适用于 AI 能访问这台电脑的情况；托管在外部云端的 AI 仍需要公开 HTTPS 入口。上述 `18080` 端口只绑定本机，不要把它映射到公网。

股票根据官方 `symbolType=stock` 资料识别，不用币名首字母猜测。加密资产也会展示；无法识别的资产保留原数量并归入其他分类。未出现在当前官方股票资料中的资产不会被归为股票。现货冻结/锁定数量计入持仓，合约多空分别展示；港股 Quanto 行情仍以 HKD 计价。列表不是完整账户权益报表。公司财报数据源不可用时财务列显示缺失，不从 Bitget 行情推算财报。

服务端完整持仓快照短暂缓存 10 秒，读取缓存前仍检查用户身份；部分失败不缓存。行情失败时保留持仓数量并显示价格缺失。凭证只在服务端读取；浏览器仅持有慧投登录 token。

### 持仓同步失败的诊断

页面的「已配置」仅表示凭证字段和慧投用户绑定完整，并不代表已通过 Bitget 验证。失败时，后端分别标注资产/合约查询、HTTP 状态和数字错误码，转换为固定提示；不转发可能包含账号或密钥的上游 `msg`。私有 GET 请求同样带上官方要求的 `Content-Type: application/json`。前端读取 JSON 中的错误文字，避免直接显示整段 JSON。

在腾讯云项目根目录可运行：

```bash
sudo python3 scripts/check-bitget.py
```

脚本在内存中读取正在运行的 `wiseinvest-api` 容器环境，按配置的 UTA/经典账户发送固定官方只读请求；拒绝重定向，不下单、不变更配置。输出只包含账户类型、凭证是否齐全、HTTP 状态和数字错误码，不输出密钥、上游原始消息或持仓。它从服务器主机发起请求，可辅助检查出口连接和凭证；若容器使用了代理或不同网络，结果不能代替容器本身的连通性验证。修改环境文件后，必须重建后端容器，诊断脚本才会读到新配置。

常见错误码参见 [Bitget 官方错误码](https://www.bitget.com/zh-CN/api-doc/affiliate/error-code/restapi)：`40014` 查询权限不足，`40036` Passphrase 错误，`40038` IP 未在白名单，`40008` 时间戳过期，`40009` 签名错误，`40007` Content-Type 无效。`00000` 且 HTTP 200 表示该只读请求成功。请提供脚本输出即可排查，无需提供凭证。

## 本地验证与上线

在 `backend` 中运行 `go run ./cmd/server`，在 `frontend` 中运行 `npm run dev`。前端「设置」可选择本地后端 `http://127.0.0.1:8080`；默认仍使用腾讯云实例。仅更新前端不会让尚未部署的远端后端获得港股和 Bitget 功能，需要同时部署后端代码。

回归测试覆盖混合行情兜底、HKD/USDT 单位、股票身份、分钟/周 K 聚合、离线行情下保留自选、公司指引可追溯性、基本面币种不混算及缺失值；账户测试覆盖固定签名、拒绝交易与重定向、UTA/经典返回结构、数量精度、用户/缓存隔离、股票与加密货币分类、多空分别展示及部分失败。

## 腾讯云已部署后端的更新

日常更新可使用根目录的 `deploy-update.sh`。在服务器执行：

```bash
cd /root/WiseInvest
git pull --ff-only origin 'feature/整体功能优化'
bash deploy-update.sh
```

`deploy.sh` 也包含相同兼容处理：检测到 PostgreSQL、Redis 和 Nginx 均在运行且已有生产配置时，直接转入 `deploy-update.sh`，不再执行系统安装步骤。首次部署或基础服务未全部运行时继续初始化流程；其 V1 分支也先构建再清理旧后端，并保留已有基础服务容器。两个入口均需通过后端和 Nginx 代理健康检查才报告成功，HTTP 404/502 不再被当成成功。

脚本自动定位项目目录，兼容 Compose V2/V1，按需使用 sudo。读取现有 `backend/.env.production`，确认 PostgreSQL、Redis、Nginx 已在运行后构建镜像并重建后端容器，等待健康检查、刷新 Nginx、验证代理。失败时退出并显示后端日志。默认每次健康检查最多等待 120 秒，可用 `DEPLOY_HEALTH_TIMEOUT=180 bash deploy-update.sh` 调整。脚本不拉取 Git、不安装或升级系统、不改写环境文件，也不重建数据库和 Redis。它只更新后端 API；首次部署仍参考 `deploy.sh`。

如果服务器只有旧版 `docker-compose 1.29.2`，重建可能报 `KeyError: 'ContainerConfig'`。[Docker 官方说明](https://docs.docker.com/engine/deprecated/#container-and-containerconfig-fields-in-image-inspect)解释了该镜像字段的移除，旧 Compose 已停止维护。脚本优先使用现代 `docker compose`；回退到 V1 时，在镜像构建成功后先运行 `rm -s -f backend`，再创建后端，避开旧版的 recreate 路径。该命令按当前 Compose 项目和 backend 服务定位容器，也能清理失败后被改名的旧容器；不使用 `-v`，不删除数据库、Redis 或生产配置。本仓库后端容器没有持久化数据卷，所有持久化业务数据存储在独立 PostgreSQL/Redis 服务中。

已经遇到上述错误时，拉取修复后重新运行 `bash deploy-update.sh` 即可。手动恢复也可在项目根目录执行：

```bash
sudo docker-compose rm -s -f backend
sudo docker-compose up -d --no-deps backend
curl -fsS --retry 20 --retry-connrefused --retry-delay 3 http://127.0.0.1:8080/health
sudo docker-compose exec -T nginx nginx -t
sudo docker-compose exec -T nginx nginx -s reload
curl -fsS http://127.0.0.1/health
```

手动恢复使用已经构建成功的镜像；长期建议迁移到仍受支持的 Docker Compose。不要通过 `docker-compose down -v` 清理此错误，那会删除数据卷。

以下适用于本仓库 `docker-compose.yml` 的现有 Docker 部署，数据库、Redis 和 Nginx 已在运行。通过腾讯云控制台或 SSH 登录服务器，在 **服务器原有 WiseInvest 仓库目录** 执行：

```bash
git fetch origin
git switch 'feature/整体功能优化'
git pull --ff-only origin 'feature/整体功能优化'
nano backend/.env.production
```

Compose 读取 `backend/.env.production`，不是本地开发用的 `backend/.env`。保留生产环境现有数据库、JWT、微信和 LLM 配置，补充上面的七个 `BITGET_*` 配置即可。只有公共行情时无需配置私有凭证。绑定用户 ID 必须从这个 **云端后端** 的 `/api/v1/auth/me` 取得，本地数据库用户 ID 不通用。

```bash
sudo docker compose up -d --build --no-deps backend
curl -fsS --retry 20 --retry-connrefused --retry-delay 3 http://127.0.0.1:8080/health
sudo docker compose exec nginx nginx -s reload
curl -fsS http://127.0.0.1/health
sudo docker compose ps backend
sudo docker compose logs --tail=80 backend
```

后端重建后 reload Nginx，使其重新解析后端容器地址；健康检查应返回 `status: ok`。上面手动命令适用于现代 Compose；V1 请使用已修复的部署脚本或前面的手动恢复命令，不能直接把命令名换成 `docker-compose` 后重复重建。现有服务可通过任一部署脚本进入后端更新流程，无需删除数据库或卷。生产环境文件已被 Git 忽略，并从 Docker 构建上下文排除，凭证通过运行时环境注入。

登录前端后进入「实体账户」验证真实持仓。没有凭证时接口返回未连接；`/health` 正常仅代表后端运行，不代表 Bitget 凭证验证成功。当前 Compose 只部署 API，不构建或托管 Web 前端；本地前端「设置」切换到云端 API 地址即可使用新接口。


## 持仓、财报与自选的一致性

- 从具体 Bitget 产品进入详情时，网页传递 `provider_symbol` 和 `category`（`SPOT` / `USDT-FUTURES`）；行情、K 线及综合分析的缓存和请求均按产品区分。已指定的产品不可用时返回错误，不切换到同名产品。省略产品参数的旧客户端保持原有按市场和股票代码查询的行为。
- 持仓读取合并并发请求，成功快照缓存 10 秒，失败或不完整结果退避 15 秒。已有完整快照时返回它并设置 `stale=true`、`complete=false`、`last_success` 和警告；`as_of` 保留该快照时间。没有完整快照时允许返回部分结果，完全失败仍返回 502。取消单个请求不影响其他等待者。
- 现货估算市值为数量乘以该代币最新报价；合约开仓均价和未实现盈亏直接读取账户接口，盈亏使用保证金币种。现货账户资产接口不提供成本，因此不推测现货成本或盈亏；也不把合约数量乘报价误当成账户净资产。字段参见 [UTA 持仓接口](https://www.bitget.com/legacy-docs/uta/trade/Get-Position) 和 [经典账户持仓接口](https://www.bitget.com/docs/catalog/classic-contract-position/classic-contract-position)。
- 财报按标的合并并发查询，最多缓存 512 个标的；正常结果缓存 6 小时，失败结果缓存 1 分钟。请求结束或切换账户后，网页取消不再需要的财报请求。
- 财报响应的 `metrics` 为 EPS、PE、远期 PE、指引和财报日期分别记录来源、币种及数据时间。本地 EPS 与线上币种冲突时不覆盖线上 EPS；指引使用自己的币种。指引必须包含 HTTPS 来源、有效发布日期、期间和会计口径，发布日期超过 180 天或在未来时隐藏指引数值和元数据。此规则只表示当前页面的有效期策略，不保证报告发布后未被公司修订。
- 后端首次升级时，在事务内规范化旧自选代码，合并同一用户、市场和代码的重复记录，保留最早记录，并创建唯一索引 `idx_watchlist_identity`。后续新增使用冲突忽略，删除兼容旧加密货币代码。迁移在后端启动时执行，未直接修改线上数据库。

验证：`go test ./...`；并发检查：`go test -race ./internal/adapter/api/handler ./internal/infrastructure/bitget`。设置 `WISEINVEST_TEST_POSTGRES_DSN` 可执行 `go test ./internal/infrastructure/database -run TestWatchlistMigrationAndConcurrentMembership`；测试只在新建的临时 schema 中工作，结束后移除该 schema。
