# OpenCode / Sub2API / DeepSeek 思考回传验证（2026-10-22）

## 结论

当前已发布 New API 镜像在本次 OpenCode / Sub2API 本地对照测试中未发现新增转发缺陷，无需修改业务代码或重新发布镜像。需要区分“能够继续请求”和“原始思考完整回传”：某些客户端或中间转换会在请求到达 New API 前改变原文位置，缺失说明只能提供兼容性兜底。

| 路径 | 补齐关闭 | 补齐开启 | 原文情况 |
| --- | --- | --- | --- |
| OpenCode Chat → New API → DeepSeek 形状模拟上游 | 两轮工具和回答通过 | 工具、回答和续接通过 | 原始 `reasoning_content` 保留 |
| OpenCode Responses → New API → 模拟上游 | 第二轮模拟校验报缺少正文 | 工具、回答和续接通过 | 本次 OpenCode 配置没有回传明文；New API 填入缺失说明 |
| OpenCode Responses → Sub2API → New API → 模拟上游 | 第二轮模拟校验报缺少正文 | 工具、回答和续接通过 | New API 收到的历史缺少明文，使用缺失说明 |
| OpenCode Chat → Sub2API 转 Responses → New API → 模拟上游 | 第二轮模拟校验报缺少正文 | 工具、回答和续接通过 | Sub2API 把原文放入普通 assistant 正文的 `<thinking>` 标签，未生成 `reasoning.content` |
| OpenCode Chat → Sub2API 保持 Chat → New API → 模拟上游 | 两轮工具和回答通过 | 工具、回答和续接通过 | 原始 `reasoning_content` 全链路保留 |

上述错误是本地模拟上游的明确校验结果，不是调用 DeepSeek 官方得到的结果。官方是否接受各种历史占位正文仍未实测。

## 实际测试对象

- OpenCode 官方 Windows CLI `1.19.3`；下载归档 SHA-256 与 GitHub Release 提供的摘要一致：`e4ce9117f55fc068414513789f8c7d1049f855d7518bb4e350b9170a3a20d8c3`。
- 对应 OpenCode 依赖：`@ai-sdk/openai 3.0.88`、`@ai-sdk/openai-compatible 2.0.41`。
- New API：`thefoolishfox1/new-api:build-20261021-230610-reasoning-history`，DeepSeek 类型渠道。
- New API 镜像摘要：`sha256:f4899c0ddf0482951f268619ebdb35162a95e22fe6f57c93287cf7c4a5ce1447`；验证结束时远端 `latest` 仍指向该摘要。
- Sub2API：复制本机已安装镜像到隔离测试容器，镜像标记版本 `0.2.17`、revision `060d0160417782cd962a0c7f460f6cd4565d4916`，镜像 ID `sha256:44b59fc74b0b582750ef5f38146782db177767ef9d3f34351f1c75c157b1bb88`。用户下游生产版本未知，不能推定所有版本行为一致。
- 所有模型响应、思考文字、工具调用和工具结果均为合成数据；唯一实际执行工具是 OpenCode 的 `read`，读取测试目录中生成的固定文件。

## 证据

在 Chat → Sub2API → Responses 的第二轮，实际捕获：

1. OpenCode 发出 `assistant.reasoning_content = "DIAGNOSTIC_REASONING_call_fixture_1"`。
2. Sub2API 发往 New API 的请求没有 reasoning item，却有普通 assistant `output_text`：`<thinking>DIAGNOSTIC_REASONING_call_fixture_1</thinking>`。
3. New API 保留原有消息、调用和工具结果，仅在开启补齐后添加独立的缺失说明。

本地 Sub2API 源码 `backend/internal/pkg/apicompat/chatcompletions_to_responses.go` 中的 `chatAssistantToResponses` 也把 `ReasoningContent` 拼入普通文本。此源码 HEAD 为 `b0a02a3304a0a78cdd003ddda1dd23ba02c03084`，与测试镜像 revision 不同；结论以实际镜像出入站抓取为依据。

不会据此让 New API 从任意 `<thinking>` 回答文本猜测原始思考，也没有为适应某个客户端修改全局协议行为。

## 验证项目

- 10 组开关和协议对照，共 32 次上游请求；3 次预期的缺正文负例，其余 29 次成功。
- 所有正常用例均执行两次只读工具调用并完成回答；开启补齐的 5 组用例继续验证会话续接。
- 29 段成功 SSE 在 New API 入站响应与出站响应之间逐字节一致，包括原始思考事件和 usage。
- OpenCode 记录的输入 token 为 100，输出正文与思考 token 合计 20，与模拟上游一致。
- New API 转发的非思考历史、工具调用和真实工具结果保留，未增加额外上游请求。
- 原生 Chat 路径开关开启和关闭均保留原文，不需要占位说明。
- 核对发布源码 manifest：本轮没有应用代码变化；没有改动生产实例、生产配置或数据库。

## 配置建议

若目标是让 DeepSeek 原始思考持续完整回传，可以为该链路单独配置：

1. OpenCode 使用 `@ai-sdk/openai-compatible` 的 Chat Completions 模式；模型配置 `interleaved: { "field": "reasoning_content" }`。本次测试显式设置该项。
2. Sub2API 的 DeepSeek 专用 OpenAI API Key 账号中，将“Responses API 支持”设为“强制 Chat Completions”，对应 `extra.openai_responses_mode = "force_chat_completions"`。本次实际验证该配置可保持 Chat 协议。
3. New API 继续使用 DeepSeek 渠道；“思考正文补齐”可保留，用于其他缺失历史场景。

若继续使用 Responses / Chat→Responses 路径，当前镜像的补齐在本地测试可维持工具流程，但不能声称上游原始思考已完整回传。

## 复核资料与边界

本地资料位于 `.local-tests/opencode-check-20261022/`：`direct-report.json`、`sub2-report.json`、`sub2-native-report.json`、`verification-report.json` 和各测试目录的请求 / SSE / OpenCode JSON 事件。

最终断言命令：

```powershell
python .local-tests/opencode-check-20261022/verify.py
```

测试使用独立客户端配置、专用测试数据库、Redis 和应用容器；测试记录不保存请求认证头。管理控制台要求合规电子确认，因此测试账号和分组采用专用数据库的合成 fixture 初始化，没有替用户签署确认，也没有修改合规设置。临时应用与 Redis 容器均已停止，原服务保持不变。

这不是 DeepSeek 官方生产 API 或用户实际 OpenCode / Sub2API 版本的完整认证；不涉及新的 Docker 上传。
