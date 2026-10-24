# Docker 运行时层整理 — 2026-10-24

## 原因

此前的临时发布 Dockerfile 以上一版 New API 镜像作为运行时基础，再执行
`COPY /build/new-api /new-api`。Docker 的父层不可变，覆盖文件不会删除父层
中的旧二进制。因此每次发布都会继续保留以前版本的 `/new-api`，下载归档和
本地展开体积不断增加。

仓库正式 [Dockerfile](../../Dockerfile) 已明确从固定摘要的
`debian:bookworm-slim` 开始运行时阶段，新增注释说明不能以旧 New API 镜像
作为运行时父镜像。本次同时移除生产运行时没有使用的 `wget` 和 `libasan8`；
开发 Dockerfile 仍保留 `wget`，不影响开发容器。

由于构建时 Docker Hub 基础镜像暂时返回 EOF，本次发布使用本机已有的上一版
干净运行时文件系统导出后压平，再复制当前已验证的 Go 二进制。该方法只用于
本次离线发布；后续网络正常时，根目录 Dockerfile 可以直接从官方固定摘要
重建相同的独立运行时层。

## 验证

镜像为 linux/amd64，Go 1.26.10。新镜像启动后 `/api/status` 返回 200。
协议和计费回归使用本地模拟上游、临时 SQLite 和随机测试凭据：

- 加密历史、思考修复、透传、compact、上游失败：16 项；
- 工具历史和重复调用 ID：26 项；
- 模型映射、响应模型归一化和管理员日志：27 项；
- `/v1/alpha/search` 及 Chat/Responses：10 项；
- 缓存字段、模型映射、流式/非流式和扣费对照：29 项；
- 约 1.45 MB 历史跨渠道重试、参数隔离和扣费：6 项。

以上 114 项全部通过。此前完成的主仓库和 relaykit 测试、`-race` 深拷贝测试
结果保持有效；本次只改变运行时镜像层和无业务意义的生产工具包。

## 体积对比

| 镜像 | Docker 归档大小 |
| --- | ---: |
| `config-cache-20261024` | 391,517,696 字节 |
| `bulk-copy-20261024` | 444,263,936 字节 |
| `docker-rebase-20261024` | 88,399,360 字节 |

本次镜像只有 3 个运行时层，未继承旧版本二进制层。Docker 的本地展开大小
会高于压缩归档大小，用户侧下载主要受归档和注册表压缩层影响。

## 发布

- 固定标签：`thefoolishfox1/new-api:docker-rebase-20261024`
- `latest` 已同步到相同摘要；远端 digest：
  `sha256:1da24a9df8cbdd837f7aeb482891292eeb01424d268f9985d32cc40d349d7cb7`
- Config digest：
  `sha256:599ffc1ae7cac736d7544ddf988bf27e797be3e15f84196a52644afe5e4ef33f`
- 本地验证镜像 ID：
  `sha256:b1de0842aa87428307d0927bf2c9dc819a419a70c7d62d0d99d2e38cdb89eb65`

旧版 `bulk-copy-20261024` 仍可作为回滚镜像，远端 digest 为
`sha256:5ab8121750e7c964cab7084c5744c5024bf70bc3a0cc56ad86922dffa03fae92`。

## 最终提交镜像

提交当前分支后重新从无旧二进制的扁平运行时层构建，并以固定标签和
`latest` 发布。归档大小为 82,028,032 字节，启动后 `/api/status` 返回 200，
二进制中未包含首页临时 footer 开关代码。

- 固定标签：`thefoolishfox1/new-api:docker-rebase-20261024-final`
- `latest` 与固定标签 digest：
  `sha256:6994d79526707c492238b9351e19530b4c6b067b66745e3e9a08839f25c20dbc`
- 本地镜像 ID：
  `sha256:46472777cecf204a2cd916bfb16c3df1f055dca1492058d50a8a4ff32e3a4df0`
