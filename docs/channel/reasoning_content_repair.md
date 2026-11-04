# 渠道思考正文补齐

渠道设置 `reasoning_content_repair` 默认关闭。经典前端和默认前端均在渠道的额外设置中提供“思考正文补齐”开关，所有渠道均可保存该设置。

## 生效范围

- 正文恢复和缺失占位仅处理最终发往上游的 Chat Completions / Responses JSON，且最终参数启用了思考。判断发生在模型映射、协议转换和参数覆盖之后；显式关闭思考优先。
- 未显式指定思考时，只识别 `deepseek-reasoner`、`deepseek-flash` / V4 系列等已知思考模型的默认行为；这同样适用于 OpenAI 兼容渠道的对应模型名，不猜测其他模型。显式关闭思考始终优先。
- Chat 历史：同一条 assistant 消息缺少 `reasoning_content` 时，可从其已有的字符串 `reasoning` 补齐。
- Responses 历史：同一条 reasoning item 缺少 `content` 时，可从其已有的字符串 `reasoning_content` 或 `reasoning_text` 补为 `reasoning_text` 内容块。
- Responses 格式修复：开关开启后，历史 reasoning 条目同时缺少 `summary` 和 `content`（或两者为 null）时，补入 `summary: []`。这项结构修复也适用于其他模型或下一轮未开启思考的请求；不生成正文，保留已有摘要、正文和加密数据。
- DeepSeek 缺失历史占位：开启思考且请求中存在可识别的本地 assistant / 工具调用历史时，为缺少正文的历史轮次添加明确的缺失说明。当前请求可以没有 `tools` 声明（ZCode/Sub2API 转换后可能出现这种情况）；全新的 user-only 请求不会注入占位。适用于 DeepSeek 渠道，也适用于最终模型名为 `deepseek-*`、`deepseek/...` 或 `deepseek-ai/deepseek-*` 的 OpenAI 兼容渠道。后一类渠道仍须显式开启思考。
- Chat 补入缺失的 `reasoning_content`；Responses 优先补到已有空 reasoning 条目的 `content`，整条 reasoning 丢失时在对应 assistant / 并行工具调用组前插入合法条目。摘要不冒充原文；加密和签名条目保持原样，Responses 必要时单独插入明文说明。
- 原生 Responses HTTP 流和 WebSocket 流：按 `item_id`、`output_index`、`content_index` 收集已有正文，补齐 `response.output_item.done` 和成功的最终 response 中缺失的正文。保留已有正文及其他扩展字段。
- 请求体透传优先，跳过修复；Claude / Gemini 原生格式不修改。转换器已经丢弃的扩展字段无法在最终 JSON 阶段恢复。

## 边界与性能

不会从普通回答、用户消息、工具结果、摘要或其他轮次生成原始思考。真正丢失的原文无法恢复，此时只在符合上述 DeepSeek 条件的请求历史里填写：

> 历史思考内容缺失，Agent或请求链路中可能丢弃历史思考内容。

占位说明不注入下游响应、SSE delta 或最终回答，不改变上游 token 用量、错误事件和重试判定。上游可能将新增说明计入输入 token；本功能不保证所有第三方网关都接受占位正文，也不能补救损坏的签名或错误的工具调用参数。兼容网关的 `previous_response_id` / 远端 conversation、item reference 和超限历史不生成占位说明。DeepSeek 类型的原生接口不支持远端会话参数，因此即使客户端同时携带这些参数，仍可修复请求中明确提供的本地历史；不会查询或虚构远端历史。

空 `reasoning_text` 块或纯空白正文也按缺失处理，优先恢复同一条目内真实的别名正文，否则按上述条件补入缺失说明。保留已有块的扩展字段，不覆盖真实正文、签名或未知格式。历史回答 / 思考条目的 `incomplete` 状态会原样保留，缺失说明不会将其改成已完成。未完成或参数损坏的工具调用组保持原样，但不再阻止其他独立轮次补齐。流式正文恢复同样识别空文本块，且只恢复实际收到的完整正文，不向响应插入占位说明。

每次渠道尝试只修改独立的出站 JSON，原始历史仍可用于重试。正常完整历史保持原样；已有真实思考优先，即使它出现在同一工具调用组后方。重复经过修复不会继续累加占位条目，两个补齐开关可以独立使用。

修复不访问 Redis、数据库或额外网络接口。每阶段最多检查 4096 条历史，真实别名恢复最多补齐 256 项、2 MiB 正文，缺失占位最多检查 256 个 assistant 组；超出组数则放弃整批占位。每阶段实际需要变更时才合并 JSON，避免每条历史都复制整个请求。流缓存仅限当前响应，预算为 2 MiB、最多 256 个输出位置、每项 64 个正文块；遇到超限、身份冲突或可检测的序号缺口时，不注入不完整正文。

已有的“思考内容转换”是另一个设置：它将 Chat 响应的 `reasoning_content` 拼入普通 `content` 的 `<think>` 标签中，用于展示兼容，不负责历史补齐。本设置不会从 `<think>` 展示文本猜测原始思考。

## 2026-10-21 初版验证记录（仅真实正文恢复）

以下命令从仓库根目录运行，Go 验证环境为 `golang:1.26.3-bookworm`。容器内路径为 `/workspace`，设置 `CGO_ENABLED=0 GOWORK=off`。

```sh
go test ./relay/... ./controller ./service -count=1
go test ./relay/common -run '^$' -bench '^BenchmarkReasoningRepairRequest$' -benchmem -count=1
go build -o /tmp/newapi-ds-responses-check .
cd relaykit && GOWORK=off go build ./...
```

独立三库验证使用专用测试数据库；`TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN` 从测试容器环境注入，没有使用生产数据库：

```sh
go test ./controller -run '^TestReasoningRepairSettingsDatabaseMatrix$' -v -count=1
```

SQLite 3.50.4、MySQL 8.4.11、PostgreSQL 16.15 均通过旧记录默认关闭、开启/关闭保存和相邻设置保留测试。本次只增加现有 TEXT 设置中的 JSON 属性，没有 schema / migration 变更。

在 i7-13700K 的本地基准中，关闭开关为 0 B/op、0 allocs/op；开启后的 128 KiB 无需补齐请求约 0.28 ms/op。此数值是本地微基准，不代表生产吞吐保证。

```sh
cd web
bun run test src/features/channels/components/__tests__/reasoning-content-repair-setting.test.tsx
bun run typecheck
bun run i18n:sync
bun run build
cd classic
bunx vitest run src/components/table/channels/__tests__/reasoning-content-repair.test.jsx
bun run i18n:sync
bun run build
```

两套前端的设置加载、默认关闭、切换、保存测试通过（默认前端 30 项，经典前端 3 项）；默认前端还覆盖键盘切换和权限禁用。两套前端均完成构建及七种语言同步。

真实 Codex CLI 0.163.0 → 当前 New API DeepSeek 渠道 → 本地模拟上游：

- 完整正文：多轮工具调用及会话续接通过，原有正常响应事件保持不变。
- 人为移除最终 reasoning item 的 `content`、保留真实 reasoning delta：关闭开关时第二轮复现 400；开启后两次工具调用及最终回答通过。
- 完全没有可恢复正文：仍返回模拟上游的缺正文错误，不生成占位思考。

本地验证脚本和原始请求/响应位于仓库同级目录 `deepseek-repair-verification-20261021/`。这些结果验证协议链路和已知故障，不等于 DeepSeek 官方接口、Codex Desktop 或 Sub2API 全场景实测。

## 缺失历史占位与结构修复

DeepSeek 官方文档说明：思考请求携带工具时，需要回传之前各轮的思考正文；Responses 的 summary 和 encrypted_content 不能替代明文 content：

- https://api-docs.deepseek.com/guides/thinking_mode
- https://api-docs.deepseek.com/guides/responses_api

`Invalid 'reasoning': at least one of 'summary' or 'content' is required.` 属于条目结构错误，和 DeepSeek 要求回传原始正文的错误分开处理。当前没有生产原始载荷，不能断言是哪一层删除了字段。

本次只扩展现有开关的请求处理与说明，没有新增数据库字段、依赖或存储操作。验证记录见仓库同级目录 `reasoning-fallback-verification-20261021/`。

最终验证结果：

- `go test ./relay/... ./controller ./service -count=1`：22 个有测试的包通过，JSON 报告记录 2176 个通过项（包含子测试）。首轮扩大测试曾失败，控制器独立复跑和最后整套复跑均通过；保留了最终完整 JSON 报告，没有据此修改无关控制器逻辑。
- 根模块构建及 `cd relaykit && GOWORK=off go build ./...` 通过。
- 两套前端相关测试共 66 项通过，默认前端类型检查、改动组件 lint、七语言同步及两套生产构建通过。
- 真实 Codex CLI 0.163.0 → 当前源码编译的 New API → 本地模拟上游：OpenAI 类型、DeepSeek 类型渠道均通过完整历史、会话续接、整条思考丢失和空 reasoning 条目的多轮测试；关闭开关的对照组复现两种 400 错误。
- 31 个成功响应的 SSE data 与上游逐字节一致（包含 usage）；完整历史、回答、工具调用和真实工具结果保持不变。
- 实际嵌入页面的默认 / 经典前端 × 浅色 / 深色 × 1440 / 390 px 共 8 组检查通过，无页面异常，说明文字未越界。
- 本地 128 KiB 无需修复请求基准约 0.16 ms/op，关闭开关为 0 B/op、0 allocs/op；不代表生产吞吐保证。

以上为真实客户端与模拟上游的协议验证，尚未使用 DeepSeek 官方 Key 实测占位正文是否被官方接受。随后已按用户要求构建、验证实际镜像并上传 Docker Hub，版本及远端摘要见 [发布记录](../releases/20261021-reasoning-fallback.md)。
