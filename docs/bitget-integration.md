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

私有接口需要 API Key、Secret 和 Passphrase，并按官方规则签名。实体账户功能已接入上述交易账户资产及持仓查询；经典合约使用 `GET /api/v2/mix/position/all-position?productType=USDT-FUTURES`。当前不读取资金账户余额、订单历史或 Bitget App 自选，也不下单或转账。

官方依据：[公开行情](https://www.bitget.com/docs/catalog/market/market-data)、[股票参考资料](https://www.bitget.com/docs/catalog/reality/market-data)、[账户资产](https://www.bitget.com/docs/catalog/account/assets-balance)、[合约持仓](https://www.bitget.com/docs/catalog/trading/position-management)、[认证与签名](https://www.bitget.com/docs/classic/rest-api)。

## 连接自己的 Bitget 实体账户

前端在「总览」和「自选列表」之间增加「实体账户」。账户选择器目前只有 Bitget；接口返回账户数组，后续可扩展其他账户。列表与自选共用组件，包含名称、代码、市场、价格、EPS、PE、远期 PE、公司最新财报 EPS 指引和下份财报时间。名称下补充持仓数量、现货/合约以及方向。默认每分钟刷新，支持手动刷新。

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

股票根据官方 `symbolType=stock` 资料识别，不用币名首字母猜测。未出现在当前官方股票资料中的资产不会被归为股票。现货冻结/锁定数量计入持仓，合约多空分别展示；港股 Quanto 行情仍以 HKD 计价。列表不是完整账户权益报表。未配置 FMP/公司财报数据时财务列显示缺失，不从 Bitget 行情推算财报。

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

回归测试覆盖混合行情兜底、HKD/USDT 单位、股票身份、分钟/周 K 聚合、离线行情下保留自选、公司指引可追溯性、基本面币种不混算及缺失值；账户测试覆盖固定签名、拒绝交易与重定向、UTA/经典返回结构、数量精度、用户/缓存隔离、股票与加密货币过滤、多空分别展示及部分失败。

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
