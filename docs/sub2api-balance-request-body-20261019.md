# Sub2API 余额查询与请求体错误修复（2026-10-19）

参考上游提交：

- `63cecab824bad1a421d19950004c309bb2054c09`：Sub2API 余额查询。
- `d10eafb57e289e971453eb68e882addaa29f8cc9`、
  `ef60ef27f36e418143fa4abf0c9025ba6f17fba7`：请求体读取错误原因及状态码修复。

## Sub2API 使用规则

渠道类型选择 **Sub2API（59）**，填写站点基础地址和 API 密钥。查询通过渠道代理向
`<base_url>/v1/usage` 发起 GET，请求携带 Bearer 密钥，支持站点路径前缀。

经典前端直接点击渠道表格中的剩余额度刷新；标签分组的子渠道及
“批量操作 → 更新所有已启用通道余额”沿用现有入口。默认前端使用同一组后端接口。

- 优先使用 `remaining`，缺失或为 null 时使用 `balance`；明确返回的零值正常保存。
- `isValid=false`、缺少金额、非法 JSON/字段类型、HTTP 错误均返回查询失败。
- 若存在 `subscription`，且日/周/月额度均缺失或为 null，按上游规则视为无限订阅，
  不把其 `remaining=-1` 保存为余额不足。无限订阅暂不展示为数值余额。
- 没有订阅对象的负钱包余额是有效值，继续保存；有明确订阅限额时也沿用上游数值。
- 失败保留原余额及更新时间。只有写库成功后才更新传入的渠道对象并返回成功。
- 多密钥渠道继续不支持余额查询；批量跳过禁用和多密钥渠道，单个查询失败继续处理
  其它渠道。成功查询的余额小于或等于零时，沿用原批量停用规则。

复用已有 New API 查询的 HTTP 实现：30 秒总期限、256 KiB 响应上限、渠道代理和
错误脱敏。New API 自身的额度选择和换算规则保持不变。

## 请求体错误行为

第一次读取失败会消耗并关闭上传流。现在将该错误保存在当前请求的 Gin 上下文中，
后续读取继续返回原始原因，避免变成“读取已关闭请求体”或误读为空请求。
有效的请求体缓存仍优先使用，保留正常请求的重放和磁盘 JSON 流式解码逻辑。

普通转发、Midjourney、共享任务协议、插件路由及 `/v1/tasks` 的超限请求返回 **413**；
上传中断和 JSON 错误返回 **400** 并保留原因，去除重复的 `Invalid request:` 前缀。
插件路由原有的 **415** 媒体类型校验保持不变。音频转录/翻译继续保留模型默认值逻辑，
后续协议校验可以识别先前读取失败的真正原因。

## 验证记录

Go **1.26.3**。真实数据库矩阵：SQLite **3.50.4**、MySQL **8.4.11**、
PostgreSQL **16.15**；Sub2API 和已有 New API 余额查询全部通过，无数据库跳过项。
上游使用本地 HTTP 测试服务，覆盖余额选择、零值/负值、无限订阅、异常响应、代理、
数据库写入失败、单渠道 API、批量查询及旧值保留。

MySQL/PostgreSQL 通过 `TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN` 注入 Go 容器，连接隔离实例
`newapi-sync-mysql-20261019:3306/newapi_sync` 和
`newapi-sync-postgres-20261019:5432/newapi_sync`。
复用 `newapi_balance_test_` 表前缀，测试结束后清理。本次未改动模型、数据库驱动、
表结构、迁移或日志数据库路径。

执行命令（Go 容器内，工作目录 `/workspace`，`GOWORK=off`）：

```sh
# 此条注入两个测试 DSN，三种数据库均实际执行
go test ./controller -run 'Test(Sub2API|NewAPI)Balance' -count=1 -v -timeout 3m

go test ./common ./middleware ./relay/helper -count=1 -timeout 5m
go test ./controller ./model -count=1 -timeout 5m
go build -o /tmp/newapi-sub2api-body-fix .
```

请求体回归覆盖内存/磁盘读取失败、后续读取保留错误、失败磁盘文件清理、正常
`stream:true` 请求重复解码与原文重放、路由错误状态，以及音频模型回退后的再次校验。
`common`、`middleware`、`relay/helper`、`controller`、`model` 完整回归及主程序构建通过；
完整包测试未注入外部数据库 DSN，三数据库结果以上述专门矩阵为准。

经典前端使用 Bun **1.4.0**，真实余额单元格、批量操作和数据 Hook 仅替换 HTTP 边界；
对 New API / Sub2API 分别验证手动刷新、标签分组零余额、失败保留旧值和批量确认。
相关 **2 个测试文件、11 个用例**通过，格式及 ESLint 检查通过。
测试显式补发 jsdom 不会产生的关闭动画事件，清理 Semi UI 命令式弹窗，避免用例互相干扰。
本次没有修改前端生产代码或新增翻译键。

在 `web/classic` 执行：

```sh
bun ../node_modules/vitest/vitest.mjs run src/components/table/channels/__tests__
bun node_modules/prettier/bin/prettier.cjs src/components/table/channels/__tests__/newapi-balance.test.jsx --check
bun node_modules/eslint/bin/eslint.js src/components/table/channels/__tests__/newapi-balance.test.jsx
```

`git diff --check` 通过。

尚未与真实 Sub2API 账户联调；本次未发布 Docker 镜像或部署运行中的服务。
