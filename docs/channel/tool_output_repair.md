# 渠道工具结果补齐

此开关处理 Responses 请求历史中存在工具调用、却缺少对应工具结果的兼容问题，例如上游返回 `No tool output found for tool call ...`。它默认关闭，与“思考正文补齐”“思考内容转换”相互独立，也适用于没有开启思考的请求。

经典前端：渠道管理 → 编辑 → 额外设置 → **工具结果补齐**。默认前端：编辑渠道 → 高级设置 → 渠道额外设置 → **工具结果补齐**。API 对应现有渠道 `setting` JSON 内的 `tool_output_repair`，不新增数据库列。

## 行为和边界

- 在协议转换和参数覆盖之后、发送上游之前，检查最终 Responses `input` 中的 `function_call` / `custom_tool_call`。
- 保留真实结果，包括空字符串和包含图片的结果数组。只有能唯一对应同一调用的 item `id` 或旧 `tool_call_id`，才会修正结果的 `call_id`。
- 对完整本地历史中跨批次重复的 `call_id`（例如每轮都使用 `fc_0`），仅当每个调用均有可明确配对的真实结果、上一批次已全部结束时，成对更换后续调用与结果的编号。重复的调用 item `id` 同步更换；已有唯一 item `id`、思考内容、工具参数和真实结果保留。编号在当前请求内避开所有已有 ID，重复执行修复不会再次改写。此处理独立于思考开关，不增加上游重试。
- 确实没有结果时，补入 `tool_output_unavailable` 诊断，明确说明执行状态未知，不能当作成功，也不应自动重复有副作用的操作。不会生成虚假的成功结果或执行工具；只有上游明确返回同一缺失调用错误时，才会进行一次定向重试。
- 不修改原始请求及真实历史；当前渠道尝试的修改不会污染后续重试。思考正文和已有输出保持不变。
- `previous_response_id` / `conversation` 指向的远端历史无法凭空读取；但请求中明确携带的本地调用仍会检查。若上游返回明确的 `No tool output found for tool call <id>`，网关会在同一渠道内最多定向重试一次，并用该 ID 补入“执行状态未知”结果。没有可扩展的 `input` 数组、错误信息不明确或上游再次失败时不会继续重试。`item_reference`、调用未完成、参数截断、有歧义的 ID 冲突、未知/重复结果、跨用户轮次错位、需要改写签名结果身份等情况保持原样。存在远端历史标识时不执行重复编号修复，避免破坏远端保存的关联。
- 请求体透传、最终非 Responses 协议跳过。超过 4096 个输入项或 256 个调用/结果时，整段保持原样，不做部分修复。

它不能恢复已经丢失的真实工具输出，也不能保证模型不会自行再次调用工具。补入的是兼容诊断结果，执行状态仍未知。如果错误在 Codex/Sub2API 内部就产生、请求尚未到达 New API，或上游不返回明确的缺失调用 ID，此开关无法修复。尚未取得生产故障请求，因此本实现是有明确边界的兼容措施，不代表生产根因已经确认。

成功请求的管理员消费日志在 `other.admin_info.tool_output_repair` 下记录 `missing_outputs` / `relinked_outputs` 数量；发生重复编号修复时额外记录 `renamed_calls`。不在该标记中记录工具参数或结果。未发生修复时不增加标记。

## 2026-10-21 验证

### 后端与数据库

Go 1.26.3，在挂载当前源码的容器 `/workspace` 中执行：

```sh
GOWORK=off CGO_ENABLED=0 go test ./relay/... ./service \
  -run 'ToolOutputRepair|Reasoning|Responses|Retry|Upstream|Stream' -count=1 -timeout 5m
GOWORK=off CGO_ENABLED=0 go test ./relay/common ./relay ./service \
  -run 'ToolOutputRepair|ReasoningRepair' -count=1
GOWORK=off CGO_ENABLED=0 go build -o /tmp/newapi-tool-output-check .
cd relaykit && GOWORK=off go build ./...
```

以上通过。覆盖真实结果保留、并行调用部分缺失、两种调用格式、ID 别名、歧义跳过、幂等、上限、关闭/透传、参数覆盖、渠道重试及管理员日志。

分别连接实际 SQLite **3.50.4**、MySQL **8.4.11**、PostgreSQL **16.15**，执行：

```sh
# TEST_MYSQL_DSN / TEST_POSTGRES_DSN 指向专用测试库，凭据不写入报告。
GOWORK=off CGO_ENABLED=0 go test ./controller \
  -run '^TestReasoningRepairSettingsDatabaseMatrix$' -v -count=1
```

三库均通过：旧记录缺少开关时默认关闭，两个修复开关的独立组合能保存/重读，其他设置保留。没有结构迁移或数据库依赖变更。

性能测量命令：

```sh
go test ./relay/common -run '^$' \
  -bench '^BenchmarkToolOutputRepairRequest$' -benchmem -count=1
```

本机 i7-13700K、Linux amd64 容器中，关闭路径 **0 次分配**；开启后扫描约 128 KiB 无需修复的请求约 **0.127 ms**、1 次分配。此为局部基准，不是生产吞吐保证。修复不访问数据库、Redis 或网络，修改集中为一次 JSON 重建。

### 实际客户端和页面

当前源码编译的二进制在隔离 Docker 容器运行；真实 Codex CLI **0.163.0** → New API DeepSeek 渠道（type 43）→ 本地模拟上游。模拟上游检查工具调用与输出的 `call_id`，缺少时返回同一条错误。客户端和网关重试均关闭。

- 正常两轮工具调用及最终回答：开关开/关均通过。
- 故意从下游历史移除首个工具结果：关闭时第二次请求返回 400；开启后补入不可用说明，第三次请求完成最终回答，没有额外重试。
- 同时保留第二个真实图片工具结果，原有消息和思考历史完整保留。
- HTTP 对照检查：真实空结果不改写、别名结果可恢复、未知结果 ID 不猜测；关闭思考时仍能补齐。

默认前端新旧设置测试 **60 项**、经典前端新旧设置测试 **6 项**通过；类型检查、相关文件检查、两套 i18n 同步及生产构建通过。

Chromium 使用真实嵌入页面及 API，连接隔离 PostgreSQL 实例：经典/默认前端 × 深/浅主题 × 1440/390 像素共 **8 种组合**均通过。验证默认关闭、开关操作、保存重读、关闭、相邻设置保留和容器重启后的持久化；无页面脚本错误，截图检查无溢出或主题异常。仅测试实例提高登录/静态资源限额。

脚本、请求记录、报告和截图保存在仓库同级 `tool-output-repair-verification-20261021/`。这是本地模拟协议验证，未调用 DeepSeek 官方生产 API，也未验证生产 Sub2API 实例。后续已完成最终 Docker 镜像验证与上传，见 [发布记录](../releases/20261021-tool-output-repair.md)。
