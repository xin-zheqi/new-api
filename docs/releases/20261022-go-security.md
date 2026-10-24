# 2026-10-22 Go 证书校验安全更新

已于北京时间 2026-10-22 11:04 上传，远端固定标签和 `latest` 摘要均与实际验证镜像一致。

- 最新标签：`thefoolishfox1/new-api:latest`
- 固定标签：`thefoolishfox1/new-api:build-20261022-103721-go-security`
- 平台：`linux/amd64`
- Go 编译版本：`go1.26.10`
- 镜像摘要：`sha256:15683402bda0c27873ea5cf0ab461e7df83edad412f200c86ee3f07ffc91d75f`
- 上一版本：`thefoolishfox1/new-api:build-20261021-230610-reasoning-history`
- 上一摘要：`sha256:f4899c0ddf0482951f268619ebdb35162a95e22fe6f57c93287cf7c4a5ce1447`

## 修复范围

参考上游 [PR #7942](https://github.com/QuantumNous/new-api/pull/7942) 和 Go 官方公告 [GO-2026-6809 / CVE-2026-111943](https://pkg.go.dev/vuln/GO-2026-6809)，单独更新构建工具链。Go 的 `crypto/x509` 会拒绝证书链中已不再可信的一组公钥。

- 正式和开发 Dockerfile 均使用 Go 1.26.10，并固定官方镜像摘要。
- 根模块 `go.mod` 的最低 Go 版本与 Heroku 构建提示统一为 1.26.10。
- Release 和 Electron 工作流从 `go.mod` 读取版本，与已有 CI 保持一致。
- 相对上一发布快照，只有上述五个构建配置文件及文档不同；保留工作区此前的业务改动。Go 模块依赖版本、数据库模型和查询、协议转换、思考补齐、计费及前端源码没有新增改动。

运行中的旧二进制不会因为宿主机升级 Go 而自动获得修复，需要拉取新镜像并重建 New API 容器。本次没有替换用户正在运行的本地预览或其他服务。

## 验证

官方 Go Linux amd64 压缩包已校验 SHA-256。从最终镜像提取程序，`go version -m` 确认是 `go1.26.10`，并确认二进制包含官方补丁中的全部 8 个撤信公钥哈希。

在隔离源码快照中使用 Go 1.26.10 执行：

```sh
GOWORK=off GOTOOLCHAIN=local CGO_ENABLED=0 go test -json ./... -count=1
cd relaykit
GOWORK=off GOTOOLCHAIN=local CGO_ENABLED=0 go build ./...
GOWORK=off GOTOOLCHAIN=local CGO_ENABLED=0 go test -json ./... -count=1
GOWORK=off GOTOOLCHAIN=local CGO_ENABLED=0 go test -json crypto/x509 -count=1
```

- 主模块：48 个有测试的包、3,800 个测试及子测试通过。
- relaykit：独立构建通过，10 个有测试的包、405 个测试及子测试通过。
- Go 证书库：520 个测试及子测试通过；不适用当前平台或执行环境的用例按原测试条件跳过。
- 主模块中依赖外部服务配置的条件用例有跳过。另使用专用数据库执行 `database_matrix.py`，覆盖 DeepSeek、New API、Sub2API 余额查询，209 个测试及子测试通过，无跳过。实际版本为 SQLite 3.50.4、MySQL 8.4.11、PostgreSQL 16.15。这是余额查询回归，不是全量历史数据库迁移矩阵；本次未修改数据库依赖、schema 或迁移。
- 通过正式 Dockerfile 完整编译默认和经典前端，再运行实际发布镜像，不挂载替代二进制。
- 实际 HTTPS 渠道：可信证书请求成功；未知 CA 和域名不匹配的证书均被拒绝，后两种情况下模拟上游收到的 HTTP 请求数为 0。消费日志只记录成功请求的输入 100 / 输出 20 和正数扣费。
- 22 个思考历史 HTTP/SSE 回归通过；非思考历史和工具结果保持，缺失说明未进入下游响应，日志中的用量与扣费保持。
- 12 个 schema 回归、504 次 schema 语义比较及 3 个跨协议 HTTP 回归通过。
- 真实 OpenCode 1.19.3、隔离 Sub2API 0.2.17 与新镜像的 10 组对照通过，包括 3 个关闭补齐的预期负例。5 组开启补齐的续接成功；29 段成功 SSE 在 New API 前后逐字节一致，工具结果、用量和正常历史保持，无额外上游请求。
- 默认/经典前端 × 浅色/深色 × 1440/390 px，共 8 组页面检查通过，覆盖设置保存、重新读取和重启；浏览器无脚本错误。代表性截图人工复核未发现新显示问题。

模型上游均为可控模拟服务，未调用 DeepSeek 官方，也未验证用户生产 Sub2API 版本。已有的 Sub2API Chat → Responses 思考位置转换差异保持原状，详见 OpenCode 验证文档。证书安全更新依赖正常的证书验证；显式关闭 TLS 验证的既有设置语义没有改变。

## 发布记录

构建基于分支 `feat/upstream-relay-policy-20261019`、HEAD `673052d0a6865a2567626d157612dcc655ed23cd` 和当时工作区改动，没有执行 Git 提交或推送。无关 `scripts/backup/` 未纳入构建。

资料位于仓库同级 `release-20261022-103721-go-security/`，包含源码快照、测试报告、请求与 SSE、页面截图、镜像归档和上传结果。

- 源码 manifest SHA-256：`dc1a2b3e45b12ba6d383b77f3d6c1f6694d63e5219fc69b9020f0132670c7521`
- 镜像归档 SHA-256：`75a6025f1d29fdf5d9118fe098dbdad298e3b084ebd40b47beb58625e3056b19`
- 发布前核对源码和测试镜像身份，先上传固定版本并验证摘要，再更新 `latest`，最后再次核对两个远端标签。
- 临时测试应用和专用构建器已停止。本记录在发布后添加，不影响已发布运行内容。
