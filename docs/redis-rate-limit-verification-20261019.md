# Redis 限流故障验证（2026-10-19）

复核 `9508f3efb` 中的成功请求记录优化，以及同一路径使用的 Redis 总请求限流器。
没有修改 Redis 部署配置、Go 依赖、限流键名、列表格式或令牌桶计算规则。

## 已复现的问题与修复

1. **普通 Pipeline 在响应丢失后重复计数。** 使用真实 Redis 执行完三条命令，
   再在客户端读取回复时注入一次 EOF。原实现将一次记录写成两条，下一次独立记录后
   总数变成三条。原因是 go-redis v8.11.5 会重试普通 Pipeline 的读响应错误。
   现改用 `TxPipelined`：MULTI、LPUSH、LTRIM、EXPIRE、EXEC 同批发送；该版本客户端
   不会重试事务流水线的读响应错误。故障测试确认已执行的记录只保留一条且有 TTL，
   下一次调用可重新连接并正常记录。
2. **脚本缓存丢失后持续 NOSCRIPT。** 原实现只加载一次 Lua，随后一直执行缓存的 SHA。
   真实 Redis 的 SCRIPT FLUSH 保留限流桶但移除脚本后，原测试收到 NOSCRIPT。
   改为共享 `redis.Script`，常规使用 EVALSHA，缓存缺失时自动使用 EVAL。
   容量为两次的桶在清空脚本缓存前后合计仍只能通过两次，不会重置已有计数。
   同时移除绑定第一个客户端的全局单例，确保传入新 Redis 客户端后不会继续访问旧连接。
3. **窗口在检查期间过期被误报为服务故障。** LLEN 读到完整列表后，键可能在 LINDEX
   之前过期。通过 miniredis 的命令钩子确定性复现，原检查返回 `redis: nil`。
   现在将不存在的窗口正常放行，其它读取错误仍返回上层处理。
4. **成功请求记录失败未留日志。** 现在返回 Redis 写入错误并记录带请求上下文的日志。
   此时上游响应已经完成，不追加错误正文或修改 HTTP 状态。故障测试确认完整 SSE
   正文和 200 状态保持不变；请求进入上游前检查失败时，仍返回原有 500 错误。

## 环境和验证范围

- Go **1.26.3**，Linux amd64；go-redis **v8.11.5**，miniredis **v2.38.0**。
- 真实 Redis **8.10.2**，独立容器 `newapi-redis-verify-20261019`，镜像 `redis:8-alpine`。
- 仅加入测试网络 `newapi-sync-test-20261019`，未映射宿主机端口；关闭测试实例持久化。
- 真实实例测试使用唯一的 `test:` 前缀键并清理这些键。脚本恢复测试会执行
  SCRIPT FLUSH，**只能对独立测试 Redis 运行，不能传入生产地址**。
- 另外覆盖：EXEC 发送前连接断开不产生半条记录；错误键类型返回 WRONGTYPE；
  列表顺序与截断、TTL、零限制、分组隔离、客户端替换、正常窗口拒绝与过期放行。

在现有 Go 测试容器中执行以下命令（工作目录 `/workspace`，`GOWORK=off`）：

```sh
TEST_REDIS_ADDR=newapi-redis-verify-20261019:6379 \
  go test -p 1 ./middleware ./common/limiter -count=1 -v -timeout 2m

TEST_REDIS_ADDR=newapi-redis-verify-20261019:6379 \
  go test -race -p 1 ./middleware ./common/limiter \
  -run 'Test(Model|Redis)' -count=1 -v -timeout 3m

go test ./controller -count=1 -timeout 5m
# 首轮控制器回归出现一次 SSE 超时后的定向复测及完整复测
go test ./controller \
  -run '^TestResponsesStreamOutcomesPreserveAccounting$/^http-sse$/^failed-null-fixed$' \
  -count=3 -v -timeout 2m
go test ./controller -count=1 -timeout 5m
go build -o /tmp/newapi-redis-verified .
git diff --check
```

结果：中间件与限流器完整测试（含真实 Redis 故障注入）、定向竞态检测、
主程序构建、gofmt 与差异格式检查通过。控制器首次完整运行的
`TestResponsesStreamOutcomesPreserveAccounting/http-sse/failed-null-fixed`
发生一次本地 HTTP 请求等待响应头超时。该项随后独立连续三次通过，整个控制器包
独立复测也通过。没有修改该用例的超时、断言或 SSE 处理逻辑；首次超时的根因
尚未确认，因此不将这次复测描述为已经排除所有偶发问题。

验证日志位于本机 `%TEMP%/newapi-sync-20261019/`：
`redis-fault-before.log`、`redis-script-before.log` 为修复前复现；
`redis-verification.log`、`redis-race.log`、`redis-controller-regression.log`、
`redis-controller-timeout-recheck.log`、`redis-controller-recheck.log`、
`redis-build.log` 记录专项测试、首次控制器失败、后续复测及构建结果。

## 保证边界

这是对上述故障路径的修复和回归验证，不能保证 Redis 服务或网络永远不故障。
请求进入上游前限流检查失败仍返回 500；响应完成后的记录失败仅记录日志，
可能少记一次或无法确认是否记入，不会把已成功的响应改写成失败。

Redis MULTI/EXEC 不会回滚单条命令的运行时错误；事务流水线也不等于任意网络故障下
的严格恰好一次执行，客户端写入阶段仍有自身的重试规则。本次没有改变全局重试设置。
成功数仍按请求完成后记录，未改成并发请求的预先占位。

本次没有改变 SQL 数据库路径，不重复此前已完成的三数据库验证；也没有进行生产
故障注入、线上压测或 Redis Cluster/Sentinel 切换测试。
