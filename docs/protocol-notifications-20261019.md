# 协议转换与分类通知修复（2026-10-19）

在 `feat/upstream-relay-policy-20261019` 中选择性移植上游修复，保留本地计费、请求策略、
缓存脱敏及始终记录请求 IP 的规则。对应上游参考：`ff3684658`、`ca2cae676`、
`4b13d9388`、`c10780e05`、`e0483122c`、`6461f084f`、`55bb6b903`。
只引入 `0a03088e0` 中供转换器传递诊断使用的 collector。

## 协议行为

- Chat → Responses 保留图片 `low/high/auto/original` detail；未指定时使用 `auto`。
  兼容本地 token 估算提前解析图片的路径，不修改其估算默认值。Claude 图片和工具结果图片
  转换为 Responses 时补齐 `auto`。
- Responses reasoning 输出项始终带 `summary`，没有摘要时输出 `[]`，包括流式首个输出项。
- Gemini v1beta/Vertex AI 遇到传统 Schema 无法表示的工具或结构化输出约束时，使用
  `parametersJsonSchema` / `responseJsonSchema`；Gemini v1 沿用传统字段，清理不兼容的
  枚举等内容，并返回转换诊断。旧协议降级会放宽部分约束，不能等价保留全部 JSON Schema。
- Responses → Claude/Gemini：用户输入或工具结果后，移除末尾已完成且不含工具调用的
  assistant 输出，避免重试时被解释成不支持的 prefill。保留待执行工具调用、正常对话历史、
  Chat/Claude 的 prefill 语义；不修改用于重试的原请求。

## 通知设置与兼容规则

经典前端和默认前端均支持：

- 个人设置：余额不足提醒、渠道通知（管理员可见）、已有的上游模型更新通知，各自独立。
- 运营设置 → 监控：全站余额不足提醒开关。关闭后个人提醒开关不可操作，但保留个人选择；
  渠道通知和上游模型更新通知仍可配置。验证码和账号安全邮件继续走原有发送路径。
- 个人阈值 `0` 表示使用站点阈值；继续使用现有额度单位。拒绝负阈值。

旧 JSON 没有新字段时，余额提醒默认开启、渠道通知默认仅 root 接收；其他管理员主动开启。
禁用账号和普通用户不接收管理员渠道通知，普通用户不能通过接口设置管理员通知开关。
保存通知配置保留语言、侧栏及扣费偏好；旧客户端省略新字段时保留其已有值。切换通知方式时
仍按原规则清空未选方式的凭据。认证缓存仍移除 Webhook/Bark/Gotify 凭据，发送时从数据库
读取完整配置。

钱包和订阅提醒仅在余额从阈值以上（含等于）降至阈值以下时触发；显示最终扣费后的余额。
余额持续偏低不再按请求反复尝试发信，充值回到阈值以上后可再次触发。该判断沿用请求的
额度快照，并非账户并发更新下的全局 exactly-once 投递保证；最终还受通知限流约束。
通知开关关闭时不消耗限流次数。

Webhook 正文改为按既有 `{{value}}` 占位符替换，修复 printf 错用产生的 `%!(EXTRA...)`
乱码；保留原有 JSON 字段、values 列表和签名规则。

## 限流

保留原有“用户 + 通知类型”隔离规则（渠道状态通知的类型仍包含渠道 ID 和状态）。
窗口从首次通知尝试开始计算，不再按钟表小时分桶。Redis 使用 `TxPipeline` 将
`SET NX`（带 TTL）与 `INCR` 放入一个事务，避免首次创建竞争及 TTL 遗失；后续尝试不刷新 TTL。
内存模式保护读、检查、写入全过程，并避免清理线程删除刚更新的条目。

`NOTIFICATION_LIMIT_DURATION_MINUTE` 默认 10 分钟，非正数或时长溢出使用默认值；
`NOTIFY_LIMIT_COUNT <= 0` 不允许发送。Redis 错误返回失败，不绕过限流。
升级后使用不带小时后缀的新键，首次通知建立新窗口，旧键按原 TTL 过期。

## 验证

环境：Go **1.26.3**、Bun **1.4.0**；Redis **8.10.2**；SQLite **3.50.4**、
MySQL **8.4.11**、PostgreSQL **16.15**。上游及通知收件端均为本地测试 fixture。

三数据库矩阵执行了全部 SQLite/MySQL/PostgreSQL 子测试，没有跳过：旧配置更新、新开关
true/false/省略值、个人阈值 0、负数拒绝、管理员权限、原有偏好保留、全站选项持久化、
root 默认与管理员主动订阅的收件规则。使用 `notification_settings_test_` 表前缀，测试后清理。
没有变更表结构、迁移、驱动或日志数据库路径；新增偏好写入现有 setting JSON 文本，
全站开关复用 options 表。

Go 容器为 `newapi-sync-go-20261019`，挂载 `/workspace`，`GOWORK=off`。
MySQL/PostgreSQL 通过 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN` 环境变量注入，连接隔离实例
`newapi-sync-mysql-20261019:3306/newapi_sync` 和
`newapi-sync-postgres-20261019:5432/newapi_sync`。命令如下：

```sh
# 注入上述两项 DSN 后执行
go test ./controller -run TestNotificationSettingsDatabaseMatrix -count=1 -v

# 真实 Redis 并发限制；固定时间推进另用 miniredis，避免 sleep/timing 断言
TEST_REDIS_ADDR=newapi-redis-verify-20261019:6379 \
  go test ./service -run 'TestNotificationLimit|TestQuotaReminder|TestDisabledQuota' -count=1 -v

go test -p 2 ./common ./controller ./model ./service ./middleware ./relay/... -count=1
go test ./controller -count=1 -timeout 8m -v
go build -o /tmp/newapi-compat-notify .
cd relaykit
GOWORK=off go test ./... -count=1
GOWORK=off go build ./...
```

相关 common/model/service/middleware/relay 包完整回归通过，包括正常/异常流式、用量、
渠道自动禁用及退款回归。首次组合测试中的 controller 执行被人工中止以检查耗时；
第一次单独执行遇到一次本地 WebSocket 读取超时，同期 SQLite 写入日志显示明显变慢。
未修改生产逻辑、测试断言或超时时间，再次独立执行 controller 整包测试全部通过
（100.924 秒），包括此前超时的 `TestResponsesStreamOutcomesPreserveAccounting`。

前端新增 4 个测试文件、7 个用例，覆盖两套界面的个人开关、管理员可见性、站点关闭时
保留个人选择、阈值 0、全站开关保存及状态刷新。默认前端类型检查、涉及文件 lint、
两套前端构建通过。新增 8 个文案键通过 `add-missing-keys.mjs` 写入两套前端全部语言，
并分别执行 `bun run i18n:sync`，保留原有翻译。

```sh
cd web
bun run typecheck
bun run test src/features/profile/components/tabs/__tests__/notifications.test.tsx \
  src/features/system-settings/integrations/__tests__/notification-settings.test.tsx
bun run build
cd classic
bun ../node_modules/vitest/vitest.mjs run \
  src/components/settings/personal/cards/__tests__/notifications.test.jsx \
  src/pages/Setting/Operation/__tests__/notification-settings.test.jsx
bun run build
```

未进行真实厂商账户联调或生产通知投递，未部署运行中的服务。
