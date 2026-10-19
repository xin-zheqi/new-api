# API 与数据库性能检查（2026-10-19）

基于 `feat/upstream-relay-policy-20261019` 的 `440b17e45`，检查请求鉴权、渠道选择、
限流、请求体复用、空响应识别、日志记录和配置保存。以下改动针对代码中可以确认的
额外开销；基准测试衡量局部操作，不代表上游模型生成速度或线上端到端吞吐量。

## 优化内容

| 位置 | 原来的开销 | 改动与行为约束 |
| --- | --- | --- |
| 缓存渠道筛选 | 高级自定义路由在每次请求、每个候选渠道上重新解析 JSON；每轮过滤都复制 ID 列表 | 复用同一渠道缓存锁保护的路由配置；遇到首个排除项才复制列表。保留筛选顺序、失败归因和共享候选列表 |
| 数据库渠道筛选 | 读出所有候选渠道的全部列，再单独读取最终渠道；聚合密钥和密钥状态也被反复传输、解码 | 候选阶段只读取 ID、类型和两类设置；最终选中后仍读取完整渠道，保留密钥、覆盖参数和其它字段 |
| 配置批量保存 | 每项执行查询与保存，已有 20 项配置产生 40 条业务 SQL | 使用 GORM 跨数据库 upsert，按最多 100 行及约 1 MiB 原始键值分批；单个大值独立发送。所有批次仍在同一事务中，提交成功后才发布运行配置 |
| 成功请求限流记录 | LPUSH、LTRIM、EXPIRE 各执行一次 Redis 往返；检查窗口时格式化再解析当前时间 | 用事务 pipeline 按原顺序发送三条命令；直接截断到毫秒。保留窗口、TTL、分组隔离与禁用规则；写入失败记录日志，不改写已完成的响应 |
| 响应格式识别 | 为整个正文的每一行分配切片，再读取第一条非空行 | 使用惰性逐行遍历；NDJSON 验证同样按需遍历。保持空响应、SSE 和原始正文回放语义 |

窄列查询还暴露了旧的异常修复风险：`GetSetting` / `GetOtherSettings` 遇到损坏 JSON
会保存整个渠道对象。现在只更新损坏字段，并在 WHERE 中匹配读到的旧值，避免部分字段
快照覆盖密钥、额度统计，或覆盖管理员随后修正的配置。实库测试覆盖了这两种情况。

## 基准结果

环境：Linux amd64 Go 1.26.3，Intel Core i7-13700K。相同基准在修改前后分别执行三次，
输入固定；未访问生产数据或真实模型厂商。SQL 数量不包含事务控制语句。

| 基准场景 | 修改前 | 修改后 |
| --- | --- | --- |
| 32 个普通渠道的路径与插件筛选 | 672 B、4 次分配/操作 | 160 B、2 次分配/操作 |
| 32 个高级自定义渠道的路径与插件筛选 | 32,160 B、644 次分配/操作 | 672 B、34 次分配/操作 |
| 保存 20 项已有配置 | 40 条业务 SQL/操作 | 1 条业务 SQL/操作 |
| 约 2,000 行 JSON 的响应格式识别 | 57,344 B、1 次分配/操作 | 0 B、0 次分配/操作 |

Redis 的成功记录由三次命令往返合并为一次批量往返，业务命令仍为三条，
外加 MULTI/EXEC 两条事务控制命令。同日故障复核将普通 pipeline 改为事务 pipeline，
修复客户端丢失响应后重放 LPUSH 的重复计数问题，详见
[Redis 限流故障验证](redis-rate-limit-verification-20261019.md)。
没有把局部基准的改善比例换算成整体 API 加速比例。

## 数据库兼容性与回归验证

真实实例：SQLite **3.50.4**、MySQL **8.4.11**、PostgreSQL **16.15**。
本次没有调整数据库驱动、表结构、索引或迁移，也未修改单独日志数据库的读写路径。
upsert 由项目现有 GORM 方言生成，不依赖新增数据库版本特性。

数据库测试连接通过 `TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN` 传入；分别连接隔离的
`newapi-sync-mysql-20261019:3306/newapi_sync` 和
`newapi-sync-postgres-20261019:5432/newapi_sync`。新增矩阵使用 `api_perf_test_` 表前缀，
结束后清理测试表。

```sh
# 三种真实数据库：路由、局部字段修复、配置事务
go test ./model -run 'Test(APIOptimizationDatabaseMatrix|RequestPolicyDatabaseMatrix|UpdateOptionsBulk)' -count=1 -v -timeout 4m

# 受影响的后端请求路径与原有回归
go test ./model ./middleware ./controller ./router ./service ./relay/... -count=1 -timeout 5m

# 首轮 WebSocket 超时后的定向重复验证与独立控制器复测
go test ./controller -run '^TestResponsesStreamOutcomesPreserveAccounting$/^websocket$/^error-after-created$' -count=3 -v -timeout 3m
go test ./controller -count=1 -timeout 5m

# 路由筛选、限流、流式验证的竞态检测
go test -race ./model ./middleware ./relay/helper -run 'Test(FilterCandidateIDs|ChannelSatisfiesFilters|ModelRedis|ValidateUpstream)' -count=1 -timeout 4m

# 局部基准（修改前、后使用相同命令）
go test ./model ./relay/helper -run '^$' -bench 'Benchmark(RelayChannelFiltering|UpdateOptionsBulk|ResponseFramingDetection)$' -benchmem -count=3

go build -o /tmp/newapi-perf .
git diff --check
```

新增矩阵覆盖：缓存开关下的普通渠道、高级自定义路径、WebSocket 与插件身份筛选；
完整凭据返回；路由更新立即生效；损坏配置修复不覆盖其它列或较新的配置；
空字符串、`0`、`false` 与大配置原样保存；重复保存不新增重复行；
跨多个批次的失败完整回滚且不发布内存配置。原有正常流式、空响应和重试测试一起运行。

本轮 Redis 窗口测试使用 miniredis，检查列表截断顺序、TTL、过期窗口放行、当前窗口拒绝与
零限制不创建记录。随后补充的真实 Redis 故障测试见上述验证报告。
未进行线上负载测试或真实上游调用。

结果：三种数据库矩阵、定向竞态检测及后端回归涉及的各包均通过；最终基准、主程序构建、
gofmt 格式检查与 `git diff --check` 通过。首次合并运行时，
控制器包中的 `TestResponsesStreamOutcomesPreserveAccounting/websocket/error-after-created`
发生一次读取超时；该项随后连续三次通过，整个控制器包单独复测也通过。没有修改超时、
测试断言或 WebSocket 逻辑来绕过失败；未确认首次超时的根因，记录保留这一验证限制。
