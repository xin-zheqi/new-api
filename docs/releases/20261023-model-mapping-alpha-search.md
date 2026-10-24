# 2026-10-23 模型别名与 OpenAI Alpha Search

- 验证 Chat、Responses 的流式及非流式响应使用映射前的模型名；只改模型元数据，保持正文、工具载荷、usage 和内部上游模型标识不变。
- 为 `/v1/responses/compact` 补齐同样的模型元数据处理，未配置映射时响应字节保持不变。
- 为 OpenAI 类型渠道开放 `POST /v1/alpha/search`，复用原有路由、鉴权、错误处理与搜索调用计费。
- 搜索请求无模型映射、参数覆盖时逐字节转发；配置模型映射时只修改顶层模型字段，避免任意扩展字段中的大整数丢失精度。

验证使用 Go 1.26.10：

```sh
go test ./relay/...
cd relaykit && GOWORK=off go build ./...
```

均通过。另用新镜像启动隔离容器并接入本地 HTTP 模拟上游，通过 10 项真实 HTTP 接口检查：搜索无映射/有映射透传、Chat 和 Responses 流式/非流式别名、compact 别名、搜索鉴权、模型校验、上游错误响应。未使用实际供应商密钥，不代表任意第三方上游都支持 Alpha Search。

本次仅改后端，在完整编译当前后端后，沿用已验证 `model-mapping-20261023` 镜像的经典与新版前端构建产物和运行时依赖；没有数据库结构或依赖修改。

已上传并读取 Docker Hub 清单校验：

- `thefoolishfox1/new-api:latest`
- `thefoolishfox1/new-api:alpha-search-20261023`
- Registry manifest digest: `sha256:50578c8039cd01c1b0bb60b45c023fd9468280886aa28c2bbca85dedab9c0846`
- Image config digest: `sha256:ed7c5ac099bed1376ca271ed4f4c38bd56eedb2b42a86ddd145004bfafdd539d`（与本地验证构建一致）
