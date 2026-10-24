# 2026-10-21 思考历史漏补修复发布

已于北京时间 2026-10-21 23:12:59 上传，并核对远端标签与实际测试镜像摘要一致。

- 最新标签：`thefoolishfox1/new-api:latest`
- 固定标签：`thefoolishfox1/new-api:build-20261021-230610-reasoning-history`
- 平台：`linux/amd64`
- 摘要：`sha256:f4899c0ddf0482951f268619ebdb35162a95e22fe6f57c93287cf7c4a5ce1447`
- 上一版：`thefoolishfox1/new-api:build-20261021-215750-deepseek-compat`
- 上一版摘要：`sha256:2db69eeb747d88457f5d49779747a17561bd9e4032ebeab399dd5ccf11ad39e5`

## 修复范围

用户报告 WorkBuddy 使用 `/v1/responses`，由 OpenAI 类型的 DeepSeek 兼容渠道切换到 DeepSeek 官方渠道后，仍收到缺少 `reasoning_text` 的错误。本次修复的是已在上一版实际镜像复现的出站正文遗漏，尚无生产失败载荷可证明具体命中了哪个分支。

- 空 `reasoning_text` 块、纯空白正文和别名不再误判为已有思考。优先使用同一历史项中真实的别名正文；没有原文时按现有开关条件填入明确的缺失说明。
- 保留空块的扩展属性、其他内容块及已有真实正文；签名和未知格式保持原样。
- 历史回答 / 思考的 `incomplete` 状态保持原样，并允许补入缺失说明；未完成或参数损坏的工具调用组不修改，也不再阻止其他独立轮次补齐。
- DeepSeek 原生 Responses 不支持远端会话状态，故客户端同时携带 `previous_response_id` / `conversation` 时仍可修复明确提供的本地历史。兼容网关保留原有远端历史保护，不推测缺失的远端记录。
- SSE 已收集到真实正文但终止事件只有空文本块时，恢复真实正文；不向响应插入占位说明。

仍受原有渠道开关、思考启用、工具声明、透传及资源上限约束。本次没有改动数据库行为、依赖、前端源码或 Sub2API。

## 验证

Go 环境：`newapi-sync-go-20261019`，`/workspace`，`GOWORK=off`；根模块测试另设 `CGO_ENABLED=0`。

```sh
go test -json ./relay/... ./controller ./service -count=1
go test ./relay/common -run '^$' -bench '^BenchmarkReasoningRepairRequest$' -benchmem -count=1
cd relaykit && GOWORK=off go build ./...
```

- 根模块 22 个有测试的包通过，2,245 个测试/子测试通过；relaykit 独立构建通过。条件式外部 MySQL / PostgreSQL / Redis 用例未配置环境而跳过，无测试失败。本次相对上一镜像没有数据库代码、依赖或 schema 变化，未重新执行完整三库矩阵。
- 原始 Dockerfile 构建，包含默认和经典前端；直接运行镜像，未挂载开发二进制。
- 新增 22 个实际 HTTP/SSE 用例通过，验证空块、空白别名、中断历史、状态参数、签名保护、开关关闭、关闭思考、无工具、透传、兼容渠道及流式原文恢复。
- 22 次请求的消费日志保留上游输入 100 / 输出 20 的测试用量与正数扣费。所有工具结果和非思考历史保留，占位说明没有进入下游响应。
- Codex CLI 0.163.0 的关闭/开启对照、多轮工具及续接回归通过；31 个成功响应的 SSE data 与模拟上游逐字节一致。
- 12 个 schema HTTP 回归及 504 次独立 schema 语义比较通过；3 个跨协议 HTTP 回归通过。
- 默认/经典前端 × 浅/深色 × 1440/390 px，共 8 组实际嵌入页面检查通过，包括开关保存、重读及重启；人工检查代表性截图无异常。
- i7-13700K 本地微基准：关闭开关 0 B/op、0 allocs/op；开启后的 128 KiB 无需修复请求约 0.167 ms/op。这不是生产吞吐保证。

上游均为本地可控模拟服务，未进行 WorkBuddy 客户端或 DeepSeek 官方 API 实测，也不能据此证明官方接受所有占位正文。Sub2API 在前一跳删除的原文仍无法凭空恢复。

## 可追溯性

构建基于分支 `feat/upstream-relay-policy-20261019`、HEAD `673052d0a6865a2567626d157612dcc655ed23cd` 及当时工作区变更；没有执行 Git 提交或推送。保留此前功能，排除无关 `scripts/backup/`。

发布资料位于仓库同级 `release-20261021-230610-reasoning-history/`：源码快照、manifest、实际请求、SSE、测试报告、UI 截图、镜像归档和上传结果。

- 3,081 个快照文件；manifest SHA-256：`70cfe1fb243a17f6861a3e4f2c1f7d92b798722a26ebcee6efd220e5c0a36440`
- 镜像归档 SHA-256：`03fd8bc323b6c3c3a869702ab984a8ab843eaa26ba6844367797b6d497f59a9e`
- 发布门禁先核对测试、源码和镜像身份，上传固定标签并验证摘要后再更新 `latest`；两标签均再次核对一致。
- 临时应用容器均已停止，原本地预览及其他服务未改动。本发布记录在上传完成后添加，不改变已发布运行代码。

部署时拉取新镜像并重建 New API 容器，保持渠道“思考正文补齐”开启。
