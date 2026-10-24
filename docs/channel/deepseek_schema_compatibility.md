# DeepSeek JSON Schema 兼容性检查（2026-10-21）

## 问题与证据

生产错误为 `Invalid json schema: field type: unknown variant array, expected one of string, number, integer, boolean, null`。没有取得该次请求的 `tools`、`text.format` 或 `response_format`，也没有可用于验证的 DeepSeek 官方 Key，因此不能确认生产请求的具体错误字段，不能宣称已完成官方端到端验证。

DeepSeek [官方工具文档](https://api-docs.deepseek.com/guides/tool_calls/)支持普通 `type: "array"` 和 `anyOf`。公开的[联合类型问题记录](https://github.com/weave-os/router/pull/162)表明，包含 `array` / `object` 的 `type` 数组会触发这类错误，例如 `type: ["array", "null"]`。另有[Codex 直连 DeepSeek 的同类错误](https://github.com/kunchenguid/no-mistakes/issues/1207)，该链路没有 New API。

本地捕获的正常 Codex 请求样本没有这类联合类型，也没有 `text.format`，所以之前的正常多轮测试没有覆盖这一场景。

## 是否由此前修改引入

代码检查中，思考正文补齐、工具结果补齐只修改历史消息，未修改工具参数或结构化输出 schema。DeepSeek 原生请求适配器及搜索工具处理代码与本次拉取的上游 `206e0fa5c600127a314a97f88592c067f784f4a6` 一致。

还在隔离 Docker 实例中对比了两个已发布镜像：

- 补齐功能修改前：`thefoolishfox1/new-api:build-20261020-192441-responses-retry`。
- 最近发布：`thefoolishfox1/new-api:build-20261021-194833-reasoning-fallback`。

同一组 12 个请求经这两个版本转发后，工具 schema 和结构化输出 schema 都与输入一致；开启、关闭两个补齐开关均如此。此结果不支持“此前补齐功能生成了不兼容联合类型”的判断，但不能代替对生产失败请求的检查。

## 本地修复范围

仅在 **DeepSeek 类型渠道**最终发出的 Chat Completions / Responses 请求中，将包含 `array` 或 `object` 的合法 `type` 数组转换为等价 `anyOf`。处理工具参数、Responses 命名空间中的函数工具参数和结构化输出 schema；在渠道参数覆盖之后执行，不修改原始重试请求。

例如：

```json
{"type":["array","null"],"items":{"type":"string"}}
```

转换为：

```json
{"anyOf":[
  {"type":"array","items":{"type":"string"}},
  {"type":"null","items":{"type":"string"}}
]}
```

每个分支保留原有约束，避免丢失 `enum` / `const` 等约束后意外允许本来不合法的值。数组约束对 `null` 不生效，这是 JSON Schema 的原有语义。此转换不会修复其他不受 DeepSeek 支持的 schema 特性，也不会关闭 strict、删除工具、移除 null 或修改 required。

- 普通数组、纯基础类型联合保持原样；不遍历 default、enum、examples 中的业务数据。
- 其他渠道及透传请求保持原样；原生 Anthropic 请求不在此次转换范围内。
- 不依赖思考是否开启，也不依赖思考/工具结果补齐开关。Schema 验证在非思考模式下也可能失败。
- 包含引用、资源标识或锚点的整个 schema 保持原样，避免移动节点后破坏 JSON Pointer 或资源作用域。未知 schema 方言也保持原样。
- 每次请求最多检查 256 个工具、4096 个 schema 节点，递归深度上限 64；输入扫描及生成分支采用合计 2 MiB 的预算，超限 schema 保持原样。没有新增网络、数据库或缓存访问。

实现位于 `relay/common/deepseek_schema.go`，公共发送入口为 `NewChannelJSONBody`。无前端、数据库结构或数据库访问改动；经典前端无需增加设置。

## 验证

- `GOWORK=off CGO_ENABLED=0 go test -json ./relay/... -count=1`：20 个有测试的包通过，982 项测试/子测试通过，无失败。
- `GOWORK=off CGO_ENABLED=0 go build -o /tmp/newapi-schema-check .`：通过。
- 单元及转发准备测试覆盖普通数组、nullable 数组/对象、已有组合约束、enum 排除 null、嵌套节点、命名空间、引用保护、未知方言、预算边界、参数覆盖和跨渠道重试。
- 两个历史镜像及当前源码二进制，各在隔离 Docker 中通过 12 个真实 HTTP 转发用例，共 36 个；原有预览实例未改动。每个请求只访问一次本地模拟上游，返回 SSE 逐字节一致，已有思考正文及工具结果保持一致。
- 用独立的 Python `jsonschema 4.26.0` 校验接收端实际抓到的 schema，比较每例 21 种明确的合法/非法输入；工具和结构化输出合计 1512 次前后有效性对比一致。
- 模拟服务只捕获请求并返回固定响应，不模拟或证明 DeepSeek 官方对转换结果的接受性。

测试脚本、原始请求、实际出站请求及报告保存在本地忽略目录 `.local-tests/deepseek-schema-diagnosis/`。已在最终镜像中再次验证，并发布 `thefoolishfox1/new-api:build-20261021-215750-deepseek-compat` 与 `latest`，详见[发布记录](../releases/20261021-deepseek-compat.md)。

## 上游可复用修复

检查上游 `main` 至 `206e0fa5c`，没有发现对本次复杂联合类型错误的专门修复。以下三组修复已按后续请求单独移植并验证，详见[上游修复移植记录](deepseek_upstream_compatibility.md)：

| 上游提交 | 修复 | 本地状态与适用链路 |
| --- | --- | --- |
| [ba0e478e0](https://github.com/QuantumNous/new-api/commit/ba0e478e061ee0c824922d89c05620de878103cf) | Claude → Chat 转换同时保留 assistant 正文和工具调用 | 已移植；涉及转换到 DeepSeek Chat 等渠道，不是原生 Responses schema 修复 |
| [783735607](https://github.com/QuantumNous/new-api/commit/783735607d863bc182a00b939e498cc22e92f2f2) | Chat → Claude 合并相邻 assistant 消息时保留双方工具调用 | 已移植；可避免工具结果找不到对应调用，影响 Anthropic 格式转换链路 |
| [62f8db775](https://github.com/QuantumNous/new-api/commit/62f8db775bcdaa5da144d9c4a36f5ead82de3ac5) + [fc8f7d6fe](https://github.com/QuantumNous/new-api/commit/fc8f7d6fef519938d31e17f81035001890272330) | DeepSeek 余额统一美元；美元钱包为零时回退人民币钱包换算 | 已成组移植，并通过三库写入及失败保护验证；不改变请求计费 |

已具备，无需重复加入：`ca2cae676` 的 Responses reasoning 空 summary 输出修复，以及 `6cf0d90b5` 的 DeepSeek Flash、Search/Fetch、思考控制核心逻辑。本地额外保留旧模型名，避免影响已有配置。
