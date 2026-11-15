# 数据库查询复用优化 — 2026-11-15

## 改动与原有语义

本次仅调整两个读取路径，不修改模型、索引、迁移、驱动、连接池或扣费写入。

- `model/token_model_usage.go`：用量页复用已有的逐令牌聚合，计算总金额、输入/输出 token、总请求数及活跃令牌数。保留全范围的非空模型去重查询和当前页的模型明细查询，常见路径的日志查询从 5 次降为 3 次。仍先汇总全部匹配令牌并按消费额、令牌 ID 倒序排序，再分页；免费请求仍算活跃，空闲令牌仍保留，筛选、软删除、用户隔离、密钥脱敏和独立汇总入口保持原有规则。不跨主库和日志库 JOIN，不引入统计缓存。
- `model/system_task.go`：仅在查询一个任务类型时复用现有的 `GetLatestSystemTask`，直接取最大 ID 的一行；空列表和多个类型继续走原有路径。返回完整任务记录，不改变调度间隔、启用条件、待执行任务、去重、领取、锁或租约。

统计金额是读取结果，不经过单次扣费的 int32 饱和规则。跨令牌求和使用 `big.Int` 后检查最终 int64 边界；超界返回查询错误，不返回溢出或部分结果。SQL 逐令牌聚合的错误仍原样传播。一个极端边界差异是：SQLite 原来的全局 SUM 可能在中间正负抵消前就因整数溢出报错；现在只要每个逐令牌聚合及最终总和可表示，就能正确返回该历史统计值。正常非负消费记录无此差异。

原来的多个查询没有事务快照保证，本次也不增加这一保证：并发写入时后续模型数量和明细仍可能读到稍新的数据；复用的汇总字段来自同一份逐令牌结果。

## 真实数据库验证

环境为本地隔离 Docker 网络，Go 1.26.10，SQLite 3.50.4、MySQL 5.7.44（utf8mb4）、PostgreSQL 15.20。未连接、修改或重启生产服务。

新增的持久回归测试：

- `TestTokenModelUsageReusedSummaryDatabaseMatrix`：真实三库下验证分页摘要、免费请求、空模型、软删除、跨用户、LIKE 特殊字符及包含端点的时间筛选，逐项比较独立汇总入口。
- `TestTokenModelUsageTotalsSummaryPreservesInt64Range`：检查大于 int32 的统计值、最终正负溢出错误、历史正负抵消和空统计。
- `TestLatestSystemTasksDatabaseMatrix`：真实三库下验证按 ID 取最新、四种状态、完整字段、未知/空/重复/多个类型及查询不改动租约和待执行状态。

复用既有 `TestGetUserTokenModelUsage*` 和最近 20 条请求的隐私、排序测试。

配置 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN` 指向专用测试库后执行：

```sh
GOWORK=off go test -p 2 ./model -run 'TestTokenModelUsage|TestGetUserTokenModelUsage|TestLatestSystemTasksDatabaseMatrix|TestRecentTokenRequestsOnlyExposeOwnLastTwentyBillingSummaries' -count=1 -v
GOWORK=off go test -race -p 2 ./model -count=1
GOWORK=off go test -race -p 2 ./service -run 'TestSystemTask|TestEnqueueSystemTask' -count=1
GOWORK=off go vet ./model ./service
```

以上均通过。根模块所有 Go 包的常规测试通过；使用显式 Go 目录遍历，避免 `./...` 在 Windows 挂载目录中遍历前端 node_modules。命令为：

```sh
GOWORK=off go test -p 2 . ./common/... ./constant/... ./controller/... ./dto/... ./e2e/... ./i18n/... ./logger/... ./middleware/... ./model/... ./oauth/... ./pkg/... ./plugins/... ./relay/... ./router/... ./service/... ./setting/... ./types/...
```

扩展 `go test -race ./model ./service` 时发现既有视频任务测试竞态：logger 的未同步计数，以及测试读取被后台任务修改的 Task。用 `292f6d295` 的原始两个生产文件通过 Go overlay 回退后，同样可复现 `TestUpdateVideoTasksDefaultSleepDoesNotBlockOtherChannels` 和 `TestUpdateVideoTasksSlowChannelDoesNotBlockOtherChannels` 的竞态。此版没有修改这些路径，不能宣称 service 全包竞态检查通过。

镜像 `thefoolishfox1/new-api:20261115-db-query-opt` 的实际容器通过 16 项 HTTP/数据库核对：Chat/Responses 流式与非流式、HTTP 200 错误对象触发重试、失败返还预扣、模型名称及 compact 密文保持、用量页总数/筛选/分页/脱敏，以及停机后 SQLite 完整性。11 次成功请求各扣 118，令牌已用额度和消费日志合计均为 1298；失败尝试没有扣费。用量页查询前后账目不变。两套嵌入前端使用原有同一 API，本次没有修改前端。

## 隔离前后对比

同一份 10 万条确定性日志、75 个令牌、相同表结构与索引，对比 `292f6d295` 原实现和本实现。先验证返回条目、顺序、总数、每个摘要字段与独立摘要完全一致，再交替执行 7 次记录中位数；无按耗时断言的测试。

| 日志数据库 | 原用量页函数中位数 | 优化后 | 降低 |
| --- | ---: | ---: | ---: |
| SQLite 3.50.4 | 163.0 ms | 110.0 ms | 约 33% |
| MySQL 5.7.44 | 250.5 ms | 168.5 ms | 约 33% |
| PostgreSQL 15.20 | 84.4 ms | 62.4 ms | 约 26% |

另外验证 SQLite 主库/MySQL 日志库、MySQL 主库/PostgreSQL 日志库、PostgreSQL 主库/独立 SQLite 日志库；上述三种及同库三种组合均返回相同结果，日志查询均从 5 次降为 3 次。用量页每次仍读取最新日志；没有缓存失效或脏统计写回问题。

6226 行后台任务历史的独立实验中，近期常见单类型查询中位数分别从 SQLite 0.26→0.04 ms、MySQL 2.82→0.17 ms、PostgreSQL 0.29→0.11 ms。稀疏/不存在类型本已很快，PostgreSQL 本轮约增加 0.03 ms，不能声称所有分布都变快。

这些是本机 Docker 内的用量页函数或后台查询测量，包含驱动和本地网络开销；不是生产流量回放、模型首字时间或全站 QPS 承诺。

## 索引处置

真实 SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24/15.20 另行比较了正确时间索引 `(created_at,id)` 和最近令牌请求索引 `(user_id,type,token_id,created_at,id)`。24,000 行实验验证结果一致、旧索引保留、重复检查及两次 AutoMigrate 行为；收益取决于数据库及执行计划，MySQL 的最近令牌查询没有显示收益。新增两个索引使 PG15 实验的索引空间约从 7.16 MiB 增到 9.31 MiB，轻量插入实验不能证明没有写入成本。

因此这版不更改 GORM 索引标签、启动迁移或生产索引。线上旧时间索引列序漂移仍需独立维护；当前磁盘约 92% 使用时，不把建索引、重命名或删除索引隐含在镜像升级中。将来维护应先验证空间与现有计划，以新名字并发创建后校验完整定义及有效状态，保留旧索引和回退路径。

本轮原始测试日志、对比源码、执行计划和索引报告保存在 Git 忽略目录 `.local-tests/db-query-opt-20261115/`，不进入 Docker 构建上下文。
