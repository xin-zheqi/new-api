# 上游功能同步与验证记录（2026-10-19）

分支：`feat/upstream-relay-policy-20261019`。

基于本地 main `529a85a4ad2983b61b09556438fdd9af7e19fa77`，先通过
`ac66294aaac73155b3cf05d73a845a371d8d586b` 保存原有工作区定制，再选择性移植上游功能。
对照的上游快照为 `058c8231b68c51ee02abee15befcd720fee9f4a2`，参考发布版本为
`v1.0.0-rc.45`。没有整仓覆盖或合并 main。

## 范围与来源

| 功能 | 参考上游提交 | 本地处理 |
| --- | --- | --- |
| Claude 零用量、缓存用量与流式 usage | `1b63c28dc` | 缓存输入和工具调用仍可计费，保留本地空响应识别 |
| 流式、图片与协议转换 | `13c8e90b8`、`5ac756284`、`767a82b87` | 修复图片 URL 包装、同帧思考与正文、多值 beta 头等兼容问题 |
| 分组限流隔离 | `45fbb149f` | 保留本地用户覆盖配置和自定义限流时间窗口 |
| Docker 过载判断 | `09109ec31` | 使用容器资源限制，避免直接按宿主机资源判断 |
| 联网搜索 | `acb347a19`、`d10f11fff` | 各厂商工具转换、实际调用计数、Gemini grounding 按模型计价 |
| DeepSeek | `6cf0d90b5` | Search/Fetch、思考控制、模型别名路由、峰谷计费与工具价格 |
| 请求策略与决策记录 | `73a471f3a` | 统一配置接口、会话模式、重试策略、请求检查、渠道健康与管理日志 |
| 渠道优先级排序 | `5e9595190`、`5c31c1fa9` | 预览、事务排序、版本冲突检查、撤销、拖拽和键盘操作 |

## 本地兼容处理

- 默认前端和经典前端共用 `/api/option/request_policy` 及渠道优先级接口。
  经典前端保留原有设置入口，并增加独立的请求策略中心。
- 策略保存复用本地 `UpdateOptionsBulk`：先验证完整结果，再提交数据库事务，
  最后在同一个选项锁内发布配置。无效字段或保存失败不会部分生效。
- 保留自动测试时间段、跨午夜时段及历史“仅测试自动禁用渠道”配置；新界面同步维护
  历史标志与显式测试模式。未显式迁移的会话规则继续使用历史行为。
- 可视化编辑会话规则保留 JSON 中未知的扩展字段和参数覆盖模板。
- 重试仍受客户端取消、响应已开始、指定渠道、内容审计拒绝及原有状态码规则约束。
  已开始发送的 SSE 响应不会被拼接上另一条重试响应。
- 路由决策写入 `other.admin_info.request_policy`，仅管理端展示，不包含会话原始密钥。
- 排序只调整渠道与能力表中的优先级。预览不写数据库；提交和撤销都校验 revision。
  经典前端显示完整顺序，避免分页、筛选或标签分组导致错误的局部重排。
- 保留已有 DeepSeek 模型。新增 `@thinking:on/off/adaptive/数值` 与 `@effort:...`
  的处理范围限定在 DeepSeek；其它厂商沿用本地既有处理方式。
- DeepSeek Search/Fetch 有有效结果才按请求收费，空结果走退款路径。
  显式配置的价格（包括零价格）和自定义表达式优先于新增默认值。
- 经典前端补齐搜索工具默认价格、结构化调用明细，以及高级自定义渠道的两个
  Responses 转换模板。模板只追加草稿，保留原有路由、认证和扩展字段。
- 新增文案覆盖两套界面的七种语言，并核对插值变量。

## 验证环境与命令

Go：测试容器 `golang:1.26.3-bookworm`，`GOWORK=off`；前端使用 Bun 安装锁定依赖。

后端验证命令（仓库根目录）：

```sh
go test ./model ./controller ./router -count=1 -timeout 5m
go test ./common ./middleware ./relay/... ./service ./setting/... -count=1 -timeout 5m
go test ./controller -run TestDeepSeekModifierRoutingDatabaseMatrix -count=1 -v
go build -o /tmp/newapi-sync .
```

独立模块验证（`relaykit/`）：

```sh
GOWORK=off go build ./...
GOWORK=off go test ./... -count=1
```

数据库验证均使用真实实例，未使用 mock 替代：

| 数据库 | 实际版本 | 测试连接 |
| --- | --- | --- |
| SQLite | 3.50.4 | 测试内独立 SQLite 数据库 |
| MySQL | 8.4.11 | `newapi-sync-mysql-20261019:3306/newapi_sync` |
| PostgreSQL | 16.15 | `newapi-sync-postgres-20261019:5432/newapi_sync` |

通过 `TEST_MYSQL_DSN` 和 `TEST_POSTGRES_DSN` 向 Go 测试容器传入测试连接。
PostgreSQL 使用 `postgres://...` URI，使本地 `InitDB` 正确识别数据库类型。
矩阵覆盖策略事务失败回滚、持久化后重复加载、旧规则 JSON 保留、严格/优先会话模式、
内存缓存开关、排序预览/提交/撤销/冲突及 DeepSeek 修饰模型的 SQL 与缓存路由。
本次没有新增数据库表、字段或迁移，也没有升级 GORM 或数据库驱动。

前端验证：

- 默认前端：`bun run typecheck`、变更文件 oxlint、保留版权头的格式检查、`bun run build`。
- 默认前端 Vitest：请求策略表单与路由、工具价格、优先级拖拽及引导、排序预览、
  消费日志和工具调用明细。
- 经典前端：变更文件 ESLint、Prettier 检查、`bun run build`，以及经典前端 Vitest 配置中的
  交互测试。已有 `node:test` 测试继续由 Node 执行，不交给 Vitest。
- 两套界面分别执行 `bun run i18n:sync`；新键和插值在全部支持语言中一致。

## 最终结果

- 后端上述测试集全部通过；SQLite 3.50.4、MySQL 8.4.11、PostgreSQL 16.15 的
  数据库矩阵全部通过。
- 默认前端 9 个测试文件、108 项测试通过；经典前端 9 个测试文件、15 项测试通过，
  另有 3 项既有 Node 测试通过。
- 默认前端类型检查、两套前端的变更文件 lint/格式检查与生产构建均通过。
- 独立 `relaykit` 构建和测试通过；主程序构建通过。
- 默认前端新增 162 个翻译键，经典前端新增 186 个翻译键，均覆盖七种支持语言；
  原有翻译未删除或改写，新增文案的插值变量检查通过。
- `git diff --check HEAD` 通过；没有遗留合并冲突，也没有纳入依赖目录或构建产物。

空响应与正常流式的现有回归测试也随 relay/helper 测试集运行，包含等待输入 usage 之后的
输出内容、正常 SSE、空 Responses 返回等情况。测试使用可控上游响应；没有使用生产密钥
访问真实厂商接口，也没有对线上服务实施负载测试。

构建产物只在本地生成；没有部署、推送镜像或修改 main。
