# 多交易所实体账户

实体账户页默认选择「全部账户（合并）」。服务按当前慧投用户的绑定读取 Bitget、币安和 Gate；也可以切换单个平台。合并保留每个账户的独立行、数量单位、报价币种、现货/合约及多空方向，不跨平台净额抵消，也不把币种权益与合约名义价值相加。

## 读取范围

| 平台 | 本次接入的只读数据 |
| --- | --- |
| Bitget | UTA / 经典账户余额；USDT 合约持仓；股票和加密资产均保留 |
| 币安 | 主网现货 `GET /api/v3/account`；USDⓈ-M `GET /fapi/v3/positionRisk`；可选股票代币元数据识别 |
| Gate | 经典现货 `GET /spot/accounts` 或统一账户 `GET /unified/accounts`；`GET /stock/positions`；`GET /futures/usdt/positions` |

「全部账户」表示合并这些已接入且已绑定的账户范围，不包含理财、资金账户、期权、币本位合约、币安组合保证金账户、独立股票经纪子账户等尚未接入的钱包或产品。

Gate 统一账户使用各币种 `equity`，标注「币种权益」，保留负值；不使用可能包含借贷额度的 `available` 充当实际持仓。Gate 合约数量是「张」，保留原始合约数量，不将其当成币数或股票股数。官方元数据不足以确认标的映射的产品保留原代码和数量，归入「其他 / 待识别」，不猜测 EPS 对应的公司。

## 环境配置

本机运行使用 `backend/.env`；Docker 使用 `backend/.env.production`。密钥仅保存在后端，创建 **HMAC、只读** 密钥，并按需启用现货、合约及股票的查询权限。`OWNER_USER_ID` 是当前后端数据库中的慧投用户 ID，不是交易所 UID；未填写或为 0 时禁用该平台的私有请求。

```dotenv
# 币安复用现有的密钥配置，账户请求始终使用官方主网。
BINANCE_API_KEY=
BINANCE_API_SECRET=
BINANCE_OWNER_USER_ID=0
BINANCE_ACCOUNT_NAME=币安
BINANCE_ACCOUNT_TYPE=spot
BINANCE_INCLUDE_FUTURES=true
BINANCE_INCLUDE_STOCKS=true

GATE_API_KEY=
GATE_API_SECRET=
GATE_OWNER_USER_ID=0
GATE_ACCOUNT_NAME=Gate
# spot 或 unified，按实际账户模式选择
GATE_ACCOUNT_TYPE=spot
GATE_INCLUDE_FUTURES=true
GATE_INCLUDE_STOCKS=true
```

账户接入独立于旧行情配置 `BINANCE_BASE_URL` / `BINANCE_TESTNET`，不将私有密钥发往公共行情镜像、任意配置地址或测试网。若现有币安密钥用于测试网，应另外创建并填写主网只读密钥。

没有开通股票或合约查询的账号，可以将对应 `INCLUDE_STOCKS` / `INCLUDE_FUTURES` 设为 `false`；关闭的产品不属于该次同步范围。已启用的任一数据源失败时会明确显示「同步不完整」，成功取得的余额和持仓仍保留。绑定后端实际公网出口 IP；VPN 改节点可能改变出口。

修改环境配置后重启本地后端。在页面「设置」选择 `http://localhost:8080` 并使用本地慧投账号登录，然后进入实体账户。

## 接口与错误处理

- `GET /api/v1/accounts`：三家平台的配置、绑定状态。其他用户看不到私人账户标签。
- `GET /api/v1/accounts/all/holdings`：合并当前用户已连接账户，包含账户级同步状态及每行来源。
- `GET /api/v1/accounts/{bitget|binance|gate}/holdings`：读取单个平台。

以上接口均需 JWT。认证和归属校验在网络调用、缓存读取前执行。请求仅允许固定官方主机的 GET 白名单路径，拒绝重定向；平台原始错误、请求 URL、密钥与签名不会返回前端。币安使用 HMAC-SHA256，Gate 使用 APIv4 HMAC-SHA512。单账户完整快照缓存 10 秒，失败退避 15 秒；上次完整快照可用于失败时回退，并标明旧快照。合并结果中的数据时间是成功快照里最早的时间，各账户另附自己的时间。

股票基本面继续来自 FMP / 已核验的公司资料。加密资产和未确认公司映射的资产不请求 FMP 公司财报；订阅不足时保留空值并显示原因。

## 官方依据

- [币安现货账户](https://developers.binance.com/en/docs/catalog/core-trading-spot-trading/api/rest-api/account)
- [币安 USDⓈ-M 账户及持仓](https://developers.binance.com/en/docs/catalog/core-trading-derivatives-trading-usd-s-m-futures/api/rest-api/account)
- [币安股票代币元数据](https://developers.binance.com/en/docs/catalog/advanced-trading-stocks-trading/api/rest-api/market-data)
- [Gate 现货](https://www.gate.com/docs/developers/apiv4/en/spot/)、[统一账户](https://www.gate.com/docs/developers/apiv4/en/unified/)、[股票](https://www.gate.com/docs/developers/apiv4/en/stock/)、[合约](https://www.gate.com/docs/developers/apiv4/en/futures/)、[认证签名](https://www.gate.com/docs/developers/apiv4/en/)

目前已用模拟平台响应验证签名、空列表、数量精度、负权益、多空方向、错误脱敏、账户归属和部分失败。Bitget 可在本机使用已配置凭证验证；币安及 Gate 还需填写对应真实密钥后验证账号权限与实际返回。
