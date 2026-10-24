# DeepSeek 相关上游修复移植（2026-10-21）

已将三组独立修复定向移植到当前工作区，未整合上游主分支的其他改动。对照的上游版本为 `206e0fa5c600127a314a97f88592c067f784f4a6`。此前的思考正文补齐、工具结果补齐及 DeepSeek schema 兼容转换保留。

## 移植内容

1. [ba0e478e0](https://github.com/QuantumNous/new-api/commit/ba0e478e061ee0c824922d89c05620de878103cf)：Claude → Chat Completions 转换中，同一个 assistant 消息同时包含正文与工具调用时保留两者。此前只保留工具调用，会丢失正文。只有工具调用的消息仍不生成额外正文。
2. [783735607](https://github.com/QuantumNous/new-api/commit/783735607d863bc182a00b939e498cc22e92f2f2)：Chat → Claude 转换合并相邻 assistant 消息时，按原顺序保留双方工具调用，避免后续 tool_result 找不到 tool_use。沿用上游对自动占位 `...` 和真实省略号的区分，普通文本合并行为保持原样。
3. [62f8db775](https://github.com/QuantumNous/new-api/commit/62f8db775bcdaa5da144d9c4a36f5ead82de3ac5) 与 [fc8f7d6fe](https://github.com/QuantumNous/new-api/commit/fc8f7d6fef519938d31e17f81035001890272330)：DeepSeek 渠道余额统一为美元。优先采用正数美元余额；美元为零或不存在时，按系统 `USDExchangeRate` 换算人民币钱包。拒绝无效汇率、缺失币种、负数及非有限余额，避免将异常响应当作零余额。

余额持久化额外沿用本地 New API / Sub2API 渠道已有的保护方式：先对渠道副本写库，成功后再更新当前对象；写库失败返回错误并保留原余额及时间。现有批量更新器遇到错误会跳过，因此不会因为失败查询或失败写库而将该渠道误判为余额耗尽。

两个转换修复属于共享协议转换逻辑，也适用于其他使用同一转换器的渠道。它们不会执行或补造工具结果，不改原生 DeepSeek Responses 请求的工具定义，不是 `reasoning_text` 或 JSON Schema 拒绝的通用解决方案。

## 兼容性与部署影响

- 不改数据库模型、迁移、数据库驱动、请求计费和扣费规则。
- 不增加请求链路上的网络或数据库调用；余额查询仍只请求原有官方 `/user/balance`。
- 经典前端的 `renderQuotaWithAmount` 和默认前端的 `formatCurrencyFromUSD` 都以美元基础值显示，已核对与新余额逻辑一致，无需新增 UI 设置。
- 已存储的旧余额不做猜测性批量换算；下一次成功查询该渠道余额时保存正确美元值。历史消费额度不变。
- 沿用上游“正数 USD 优先”的规则，不将 USD、CNY 两个钱包相加。
- 没有替换原有模型列表，也没有引入上游其他协议重构、参数默认值或计费调整。

## 验证

移植前用上游回归用例复现了 assistant 正文丢失和合并后工具调用丢失；移植后这些用例通过。保留上游的多次合并、纯工具消息、纯文本消息和真实省略号用例。

独立模块验证，工作目录 `relaykit/`：

```sh
GOWORK=off go test -json ./... -count=1
GOWORK=off go build ./...
```

10 个有测试的包、405 项测试/子测试通过，独立构建通过。

根模块回归与构建：

```sh
GOWORK=off CGO_ENABLED=0 go test -json ./controller ./relay/... -count=1
GOWORK=off CGO_ENABLED=0 go build -o /tmp/newapi-deepseek-upstream-check .
```

21 个有测试的包、1810 项测试/子测试通过，无失败，根模块构建通过。其中包含原有 Responses 思考、工具补齐、schema 转换及流式计费回归。外部数据库的用例由下述独立三库矩阵完整执行。

数据库矩阵在专用本地数据库 `newapi_ds_backport_20261021` 上运行，凭据只从测试容器读取并注入环境，不写入报告。实际版本：

| 数据库 | 版本 | 结果 |
| --- | --- | --- |
| SQLite | 3.50.4 | 通过 |
| MySQL | 8.4.11 | 通过 |
| PostgreSQL | 16.15 | 通过 |

使用现有余额测试 fixture 创建隔离测试表。测试覆盖美元优先、人民币换算、零美元回退、真实零值写入、无效汇率、无效响应、HTTP 错误及写库失败，核对数据库与内存余额一致，并保留渠道名称、密钥、状态及历史消费额度。同时回归 New API / Sub2API 余额查询，合计 209 项测试/子测试通过，无跳过。

```sh
# 在 Go 测试容器中执行，两个 TEST_*_DSN 指向专用测试库。
GOWORK=off CGO_ENABLED=0 go test -json ./controller \
  -run 'Test(GetDeepSeekBalanceUSD|DeepSeekBalanceDatabaseMatrix|NewAPIBalanceDatabaseMatrix|Sub2APIBalanceDatabaseMatrix)$' \
  -count=1
```

执行矩阵的本地脚本与完整日志保存在 `.local-tests/deepseek-upstream-20261021/`。没有使用生产数据库或官方生产 API Key。

已构建、验证并发布 `thefoolishfox1/new-api:build-20261021-215750-deepseek-compat` 与 `latest`，详见[发布记录](../releases/20261021-deepseek-compat.md)。没有执行 Git 提交或推送。
