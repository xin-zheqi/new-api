# OpenAI 渠道的 Alpha Search 转发

`POST /v1/alpha/search` 支持 OpenAI、Sub2API、New API、Codex 和高级自定义渠道。OpenAI 类型表示通过 OpenAI 适配器接入的上游，具体上游仍需实现此接口。

## 使用

1. 在 OpenAI 类型渠道中填写上游站点的根地址和上游 Key，例如 `https://upstream.example`，不重复追加 `/v1`。
2. 将请求中的 `model` 配置到渠道模型列表，确保下游令牌拥有相应分组、模型权限和额度。
3. 下游使用 New API 令牌请求 New API 的 `/v1/alpha/search`，请求体必须包含 `model`。

## 转发边界

- 未配置模型映射或参数覆盖时，请求体逐字节转发，保留未知字段；不转换为 Chat Completions 或 Responses 请求。
- OpenAI 渠道保留请求路径和查询参数，使用渠道 Key 替换下游鉴权；原有请求头覆盖配置继续生效。
- 配置模型映射时，仅修改请求体的顶层 `model`；其他字段（包括大整数）保持原样。显式参数覆盖继续按现有配置执行。
- 搜索成功响应的状态、Content-Type 和响应体沿用现有 Alpha Search 透传逻辑。上游非 2xx 响应进入现有错误和重试处理，并非绕过 New API 的错误处理。
- 沿用现有鉴权、模型路由、限流和搜索调用计费；无 token usage 的成功搜索仍记一次 `web_search_preview` 调用，失败不结算成功调用。

模型映射后的别名恢复用于 Chat、Responses、compact、Claude 和 Gemini 等模型响应的协议元数据；不会替换正文、工具结果或搜索结果中出现的模型名称。

## 验证

`go test ./relay/...` 包含 OpenAI 搜索透传、查询参数、上游鉴权、映射后未知字段/大整数保留、配置覆盖、上游错误不结算、原有渠道兼容性，以及 Chat/Responses 流式和非流式、compact 的模型别名回归检查。
