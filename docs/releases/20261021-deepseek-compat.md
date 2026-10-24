# 2026-10-21 DeepSeek 兼容修复镜像发布

- 镜像：`thefoolishfox1/new-api:latest`。
- 固定版本：`thefoolishfox1/new-api:build-20261021-215750-deepseek-compat`。
- 平台：`linux/amd64`。
- 远端两个标签均核对为实际测试镜像摘要：`sha256:2db69eeb747d88457f5d49779747a17561bd9e4032ebeab399dd5ccf11ad39e5`。
- 发布核对时间：2026-10-21 22:04:42（UTC+8）。

## 内容

包含 [DeepSeek schema 兼容转换](../channel/deepseek_schema_compatibility.md)及[三组上游兼容修复](../channel/deepseek_upstream_compatibility.md)：Claude → Chat 保留正文与工具调用，Chat → Claude 合并时保留全部工具调用，以及 DeepSeek 余额的美元优先、人民币回退换算和写库失败保护。保留此前已发布的思考历史及工具结果补齐功能。

## 构建来源

使用原始 Dockerfile，包含默认和经典前端。分支为 `feat/upstream-relay-policy-20261019`，基线提交 `673052d0a6865a2567626d157612dcc655ed23cd`，包含已验证的未提交修复；没有执行 Git 提交或推送。

从工作区生成独立快照，排除无关的 `scripts/backup/` 和 Git 忽略文件。上传前逐文件核对 3079 个源文件，确认工作区、快照及 SHA-256 清单一致。

- 源码清单 SHA-256：`6f258df98177655f0a008b0a77741a0f763fa9abac0b2acb654766c427c41121`。
- 镜像归档 SHA-256：`80455338d11802c1dc0e8ce6e9189cb06a24a64bb693328165bd5b80634d0197`。

本记录与功能文档的发布状态在上传后更新，不改变运行时代码。

## 验证结果

源码回归：根模块 1810 项测试/子测试通过；独立 relaykit 405 项通过，`GOWORK=off go build ./...` 通过；余额三库矩阵 209 项通过，无跳过。数据库版本为 SQLite 3.50.4、MySQL 8.4.11、PostgreSQL 16.15，精确命令见上游移植记录。没有数据库模型、迁移或驱动变更。

以下验证全部直接运行本次上传的镜像，没有挂载开发二进制：

- 12 个真实 HTTP schema 转发用例：包括 nullable 数组/对象、嵌套联合类型、enum、普通数组和引用保护，覆盖思考及补齐开关开启/关闭。实际收到的出站 schema 用独立 Python jsonschema 校验，504 次有效性对比一致；已有思考与工具结果保持一致，返回 SSE 逐字节一致。
- 3 个跨协议 HTTP 用例：Claude → Chat 保留同条消息的正文与工具调用；Chat → Claude 保留两个相邻消息的工具调用、结果对应关系及真实省略号；普通文本合并保持原样。
- 真实 Codex CLI 0.163.0，经 OpenAI/DeepSeek 类型渠道执行完整历史、会话续接、移除思考历史和空 reasoning 项场景。修复开启时成功；关闭时，故障注入用例按预期报错，正常会话成功。
- 31 段 Codex/HTTP 成功响应流与模拟上游逐字节一致；31 条消费日志保留输入 100 / 输出 20 token 及正数扣费。
- 默认/经典前端 × 浅色/深色 × 1440/390 px 共 8 项检查通过：设置保存、重读、重启后持久化及布局边界正常，无页面脚本异常。检查了截图；使用独立 PostgreSQL 数据库。

首次 Codex 测试因缺少测试目录的本地模型目录文件而在发送请求前退出；补齐测试 fixture 后完整重跑通过。该失败日志已保留。

协议上游为本地可控模拟服务，没有调用 DeepSeek 官方生产 API，也没有验证官方是否接受每种转换后的 schema。部署后余额显示在下一次成功查询时更新为正确基础单位。

## 上传与回滚

先上传固定版本并核对摘要，再将 `latest` 更新为同一摘要，最后重新读取远端两个标签确认一致。通过已核对 SHA-256 的官方 regctl v0.11.7 与现有本地代理上传，未改变 Docker 全局配置。

上一版固定标签：`thefoolishfox1/new-api:build-20261021-194833-reasoning-fallback`，摘要 `sha256:c2871f40a295ea8db3db431fab0c65d692d439fac8babd287378aef331d806f0`，可用于回滚。

构建日志、源码快照、请求记录、截图、测试报告、镜像归档及 `publication.json` 位于仓库同级 `release-20261021-215750-deepseek-compat/`。本次临时应用容器已停止，原有预览服务未改动。
