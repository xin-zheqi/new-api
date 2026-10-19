# New API 渠道余额查询（2026-10-19）

参考上游 `7de10f2c602b50122017556e8a09cb7c488c273c`，补充渠道类型 60 的余额查询。
本次只实现向上游查询余额，不新增本站令牌读取所属用户余额的权限或数据库字段。

## 使用方式与规则

- 渠道类型选择 **New API**，填写上游站点地址和 API 密钥。
- 经典前端：点击渠道表格中的剩余额度即可刷新；标签分组中的子渠道同样支持。
  “批量操作 → 更新所有已启用通道余额”复用现有批量接口；完成后刷新列表查看批量结果。
  默认前端也复用已有余额查询接口，无需新增按钮。
- 有限额密钥：上游返回账户额度时，取密钥与账户剩余额度中的较小值；旧上游未返回
  账户额度时，使用密钥剩余额度。
- 无限额密钥：必须由上游返回账户剩余额度。需要在支持该功能的上游为该密钥开启
  “Allow querying user quota”；旧上游不支持时，应改用有限额密钥。
- 使用上游 `/api/status` 中的 `quota_per_unit` 换算美元余额，前端沿用本站货币显示设置。
- 多密钥渠道继续沿用现有“不支持余额查询”规则；批量查询跳过禁用及多密钥渠道。
  单个查询失败时批量任务继续处理其它渠道，沿用现有批量接口的结果语义。

## 异常处理

查询 `/api/usage/token/` 时携带渠道密钥；公开的 `/api/status` 不携带密钥。
两个请求均使用渠道代理设置，共享 30 秒期限，每个响应体最多读取 256 KiB。
这些限制仅用于本次新增的管理查询，不改变正常 API 转发或流式请求。

缺失/null 余额、无效 JSON、HTTP 错误、缺失或非正数换算比例、计算溢出等均报错，
不覆盖已有余额及更新时间，避免异常响应被当作零余额。明确返回的零余额和负余额
按上游实际值保存；批量查询的余额不足处理规则保持原样。
只有两个上游响应都通过校验且数据库写入成功后，才更新传入的渠道对象并返回成功。

## 验证记录

环境：Go **1.26.3**、Bun **1.4.0**；测试上游使用本地 HTTP fixture，未调用真实厂商。
数据库实例：SQLite **3.50.4**、MySQL **8.4.11**、PostgreSQL **16.15**。

三数据库矩阵全部执行，无跳过项。MySQL/PostgreSQL 连接由 `TEST_MYSQL_DSN` /
`TEST_POSTGRES_DSN` 注入 Go 测试容器，分别连接隔离实例
`newapi-sync-mysql-20261019:3306/newapi_sync` 和
`newapi-sync-postgres-20261019:5432/newapi_sync`。测试使用
`newapi_balance_test_` 表前缀并在结束后清理。

本次没有调整数据库驱动、表结构、迁移或日志数据库路径。余额保存继续使用现有
GORM 更新语句，只补充错误返回，以便新查询在写入失败时向前端报告失败。

实际执行命令（仓库挂载于 Go 容器 `/workspace`，`GOWORK=off`）：

```sh
# 在已注入上述两个测试 DSN 的 Go 容器内执行
go test ./controller -run '^TestNewAPIBalance' -count=1 -v -timeout 2m

# 相关包完整回归；此轮未注入外部数据库 DSN，数据库矩阵以上一条为准
go test ./controller ./model -count=1 -timeout 5m
go build -o /tmp/newapi-balance .

# 在 web/classic 执行
bun node_modules/prettier/bin/prettier.cjs src/components/table/channels/__tests__/newapi-balance.test.jsx --check
bun node_modules/eslint/bin/eslint.js src/components/table/channels/__tests__/newapi-balance.test.jsx
bun ../node_modules/vitest/vitest.mjs run src/components/table/channels/__tests__ src/components/table/channels/modals/__tests__/search-routes.test.jsx
bun run build

git diff --check
```

新增后端用例覆盖额度选择、上游换算比例、合法零值/负值、异常保留旧余额、认证头隔离、
站点路径前缀、代理、数据库写入失败、单渠道 API 返回结构及批量跳过规则。
经典前端用真实余额单元格、批量操作组件和数据 Hook 验证交互，仅替换 HTTP 边界；
覆盖单次刷新、标签分组零余额、失败提示保留旧值、批量确认。相关前端 **3 个测试文件、
9 个用例**通过，其中本次新增 4 个用例。后端相关包完整回归、主程序构建、经典前端构建、
格式及 lint 检查通过。

尚未部署到运行中的服务，也未进行真实上游账户联调。
