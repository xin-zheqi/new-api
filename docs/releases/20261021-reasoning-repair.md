# 2026-10-21 思考正文补齐发布验证

## 镜像

- 分支：`feat/upstream-relay-policy-20261019`。
- 版本标签：`thefoolishfox1/new-api:build-20261021-171539-reasoning-repair`。
- 发布标签：`thefoolishfox1/new-api:latest`。
- 平台：`linux/amd64`。
- 验证镜像索引：`sha256:423ed50e1348b71fee850d363280de969ecbe10e05dfa781f9ac4fd0f006d3ea`。
- 使用仓库原始 Dockerfile 构建，包含经典前端和默认前端。

构建输入是当前工作区的独立快照，排除未关联的 `scripts/backup/` 和被 Git 忽略的文件。发布前逐文件 SHA-256 对照确认运行时代码与构建输入一致；构建后仅格式化了一个前端测试文件并增加本验证文档，不影响镜像产物。

## 回归检查

```sh
# Go 1.26.3，容器内 /workspace，CGO_ENABLED=0 GOWORK=off
go test ./relay/... ./controller ./service \
  -run 'Reasoning|Responses|Retry|Upstream|Stream' -count=1 -timeout 5m
cd relaykit && GOWORK=off go build ./...

# 默认前端：思考补齐设置及既有 WebSocket 设置，共 53 项
cd web
bun run test src/features/channels/components/__tests__/reasoning-content-repair-setting.test.tsx \
  src/features/channels/components/__tests__/responses-websocket-setting.test.tsx

# 经典前端：渠道、原设置入口、支付设置，共 39 项
cd classic
bun --bun ../node_modules/vitest/vitest.mjs run \
  src/components/table/channels/__tests__ \
  src/pages/Setting/Operation/__tests__ \
  src/pages/Setting/Payment/__tests__ src/pages/Setting/__tests__
```

以上检查通过。本次发现经典前端开关状态文字误写为 `?`，改为复用既有“开／关”翻译；先通过新增显示断言复现，再修复并重跑该文件 3 项测试，全部通过。相关文件 Prettier 检查及经典前端 i18n 同步通过，修复后重新构建并验证最终镜像。

完整后端回归、默认前端类型检查、三库兼容性及性能数据见 [渠道思考正文补齐](../channel/reasoning_content_repair.md)。实际数据库版本为 SQLite 3.50.4、MySQL 8.4.11、PostgreSQL 16.15；设置仅增加现有 TEXT 列中的 JSON 属性，没有结构迁移。

## 实际 Docker 与 Codex 链路

使用最终镜像、新建 SQLite 数据库、真实 Codex CLI 0.163.0 和本地模拟 DeepSeek 上游：

- 原有完整思考正文：两轮工具调用、最终回答、会话续接通过。
- 上游 reasoning delta 有正文，但最终 item 缺少 `content`：开启补齐后两轮工具调用及最终回答通过；关闭时第二轮复现上游 400。
- 历史完全没有可恢复正文：仍返回缺正文错误，不生成占位内容。
- 7 段正常 SSE 上下游逐字节相同；3 段故障 SSE 仅补最终 item/response 缺失的 `content`。原有事件、usage 和其他字段完全一致。
- 10 次 Codex 请求的下游输入历史与发往上游的输入历史一致。

## 浏览器 UI 验证

最终镜像另起隔离实例，连接实际 PostgreSQL 16.15 和 Redis。使用 Chromium 对真实 API、真实嵌入页面执行检查：

- 经典／默认前端 × 明／暗主题 × 1440／390 像素，共 8 种组合通过。
- “思考正文补齐”默认关闭、开启、保存、重新加载、关闭、相邻设置保留均正常。
- 重启容器后，渠道和补齐设置仍保留。
- 开关与说明不超出视口，未出现页面脚本错误；截图检查确认主题、换行和周边样式一致。
- 经典优先级弹窗额外 4 项浏览器测试通过：空列表／长列表、长名称、底部 20–32 像素间距、预览、保存及撤销。此弹窗布局测试仅模拟 API 数据，使用镜像中的真实前端组件和样式。

自动化连续创建浏览器上下文会集中触发登录及静态资源限流，因此仅在隔离实例提高相关限额；没有修改应用默认限流或现有预览服务。

原始报告、截图、构建日志和本地验证脚本保存在仓库同级 `release-20261021-171539-reasoning/`。测试覆盖本地协议回归，不代表 DeepSeek 官方、Codex Desktop 或 Sub2API 的所有真实生产场景均已实测。
