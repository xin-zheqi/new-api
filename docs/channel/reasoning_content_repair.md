# 渠道思考正文补齐

渠道设置 `reasoning_content_repair` 默认关闭。经典前端和默认前端均在渠道的额外设置中提供“思考正文补齐”开关，所有渠道均可保存该设置。

## 生效范围

- 仅处理最终发往上游的 Chat Completions / Responses JSON，且最终参数启用了思考。判断发生在模型映射、协议转换和参数覆盖之后；显式关闭思考优先。
- 未显式指定思考时，只识别 DeepSeek 渠道的 `deepseek-reasoner` / V4 系列默认行为，不猜测其他网关的模型默认值。
- Chat 历史：同一条 assistant 消息缺少 `reasoning_content` 时，可从其已有的字符串 `reasoning` 补齐。
- Responses 历史：同一条 reasoning item 缺少 `content` 时，可从其已有的字符串 `reasoning_content` 或 `reasoning_text` 补为 `reasoning_text` 内容块。
- 原生 Responses HTTP 流和 WebSocket 流：按 `item_id`、`output_index`、`content_index` 收集已有正文，补齐 `response.output_item.done` 和成功的最终 response 中缺失的正文。保留已有正文及其他扩展字段。
- 请求体透传优先，跳过修复；Claude / Gemini 原生格式不修改。转换器已经丢弃的扩展字段无法在最终 JSON 阶段恢复。

## 边界与性能

不会从普通回答、用户消息、工具结果或思考摘要生成思考正文，不修改加密或签名内容，不修改上游 token 用量、错误事件和重试判定。无可用正文的历史保持原样，包括之前未开启思考、上游从未返回正文、客户端已经丢弃正文等情况；这类历史仍可能被要求回传正文的上游拒绝。

每次渠道尝试只修改独立的出站 JSON，原始请求可安全用于重试。流缓存仅存在于当前响应内，不访问 Redis、数据库或额外网络接口。正文缓存预算为 2 MiB，最多追踪 256 个输出位置、每项 64 个正文块；遇到超限、条目身份冲突或可检测的序号缺口时，不注入不完整的正文。历史请求最多补齐 256 项、2 MiB 正文。JSON 仅在实际修复时合并一次，避免每修复一条历史都复制整个请求。

已有的“思考内容转换”是另一个设置：它将 Chat 响应的 `reasoning_content` 拼入普通 `content` 的 `<think>` 标签中，用于展示兼容，不负责历史补齐。本设置不会从 `<think>` 展示文本猜测原始思考。

## 2026-10-21 验证记录

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
