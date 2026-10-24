# 2026-10-21 工具结果补齐镜像发布验证

- 发布标签：`thefoolishfox1/new-api:latest`。
- 独立版本：`thefoolishfox1/new-api:build-20261021-183857-tool-output-repair`。
- 平台：`linux/amd64`。
- 两个远端标签均核对为已测试镜像摘要：`sha256:698360ff6d0d71b9131ff7db3c87b2cc9cb93d8b405f78bf09e47c209d8500d7`。
- 核对时间：2026-10-21 18:45:11（UTC+8）。

使用原始 Dockerfile 构建，包含默认和经典前端。构建来源为分支 `feat/upstream-relay-policy-20261019` 的工作区快照，基线提交 `673052d0a6865a2567626d157612dcc655ed23cd`，包含尚未提交的工具结果补齐改动。没有执行 Git 提交或推送。

快照排除无关的 `scripts/backup/` 和 Git 忽略文件，上传前逐文件 SHA-256 对照确认全部 3067 个构建源文件与工作区一致。源文件清单摘要：`0b7b20a5969072f509826e0ee1db4124668b6f379d8756da43dd406b2ed41c85`。发布后仅更新本说明及功能文档，不改变运行时代码。

## 发布前检查

- Go 1.26.3：`go test ./relay/... ./service -run 'ToolOutputRepair|Reasoning|Responses|Retry|Upstream|Stream' -count=1 -timeout 5m` 通过。
- `cd relaykit && GOWORK=off go build ./...` 独立构建通过。
- 默认前端思考正文/工具结果设置测试 60 项、经典前端对应测试 6 项全部通过；Docker 内两套前端生产构建通过。
- `go test ./controller -run '^TestReasoningRepairSettingsDatabaseMatrix$' -v -count=1` 连接实际 SQLite 3.50.4、MySQL 8.4.11、PostgreSQL 16.15，验证旧记录默认值、两个开关独立保存及邻近设置保留，三库均通过。测试 DSN 由专用测试容器环境在内存中生成，不记录凭据。无结构迁移或数据库依赖变更。

## 最终镜像实际运行

所有发布验证运行待上传镜像，没有挂载开发二进制。隔离 SQLite 实例连接真实 Codex CLI 0.163.0 和本地模拟 DeepSeek 上游，客户端和网关重试均关闭。

- 正常历史在开关关闭/开启时都能完成两轮工具调用和最终回答。
- 故意删除一个工具结果：关闭时复现 `No tool output found for tool call ...`；开启后补充结果不可用说明，完成多轮调用，没有增加上游重试次数。
- 真实空结果和图片工具结果完整保留，明确的 item ID 别名能修正，未知结果 ID 保持原样。
- 共 14 段成功 SSE 与上游逐字节一致；14 条消费日志保留上游报告的输入 100 / 输出 20 token，扣费均为正数。
- 关闭时无修复日志标记；开启后的 4 条标记准确记录 3 次缺失补齐、1 次 ID 修正，只记录数量。

另起隔离 PostgreSQL 实例验证真实嵌入前端及 API。默认/经典前端 × 深/浅主题 × 1440/390 像素共 8 种组合通过，覆盖默认关闭、开/关、保存重读、相邻设置保留及容器重启后的持久化。没有页面脚本错误，截图检查无布局或主题异常。

测试未调用 DeepSeek 官方生产 API，未验证生产 Sub2API 实例；功能适用范围见 [工具结果补齐说明](../channel/tool_output_repair.md)。开关默认关闭，部署后按需在渠道额外设置中开启。

## 上传与产物

将已验证的同一镜像导出为 OCI/Docker tar，通过官方 SHA-256 校验过的 regctl v0.11.7 和现有本地代理上传独立版本，核对摘要后再将该摘要指向 `latest`，最后重新核对两个远端标签。未改变 Docker 全局网络配置。

构建日志、源码快照、请求记录、截图、镜像归档及 `publication.json` 保存在仓库同级 `release-20261021-183857-tool-output/`。原有 localhost:3000 预览服务保持运行，本次临时测试应用容器已停止。
