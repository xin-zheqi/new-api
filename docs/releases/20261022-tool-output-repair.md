# 2026-10-22 Responses 工具结果错误修复

本次修复覆盖 `No tool output found for tool call ...`：

- 带 `previous_response_id` 或 `conversation` 时，仍检查请求中明确携带的本地 `function_call` / `custom_tool_call`。
- 上游明确返回缺失调用 ID 时，在启用“工具结果补齐”的渠道内最多定向重试一次，并补入执行状态未知的诊断结果。
- 已存在但缺少 `output` 的结果项会被补成诊断结果；真实结果和未知错误保持原样。
- 普通错误、透传请求、非 Responses 请求、无法安全解析的历史均不触发该重试。

## 验证

- `go test ./relay/... -count=1 -timeout 10m`：通过。
- `cd relaykit && GOWORK=off go test ./relayconvert/... -count=1`：通过。
- 根模块 `CGO_ENABLED=0 GOWORK=off go build`：通过。
- Docker 容器启动及 `/api/status`：通过。
- HTTP 回归：模拟上游首次返回缺失工具结果错误，第二次请求收到补齐结果并返回 200；普通错误不重试。

## 镜像

- `thefoolishfox1/new-api:build-20261022-tool-output-repair`
- `thefoolishfox1/new-api:latest`
- 两个远端标签摘要：`sha256:415197cb2807f7929c0bd01e9d6a61dc82f7c99bd28042d9455b741372518bf7`

“工具结果补齐”默认关闭，需要在目标渠道额外设置中开启。补入内容只表示执行状态未知，不代表工具成功，也无法恢复已经丢失的真实工具输出。
