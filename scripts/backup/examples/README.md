# 不使用 .env 的完整 Compose 模板

`compose.yml` 包含 New API、PostgreSQL 15、Redis、备份容器四个服务，参数直接填写在 YAML 中；没有 `.env` 挂载，也没有依赖 `.env` 的变量表达式。备份镜像使用已发布的 `thefoolishfox1/new-api-backup:20261116`。

根目录 `docker-compose.yml` 同样已采用直接填写方式，默认服务名为 `new-api`、`postgres`、`redis`；本示例使用 lab 服务名并显式复用已有卷和网络，方便对照之前的服务器部署。在已经运行的服务器上，主要合并 `backup` 服务和 `backup_state`、`backup_cache` 两个卷；其他配置值以服务器原文件为准。若使用共享密码别名，也需合并顶部 `x-credentials`，或将别名改成直接填写的原值。Redis 使用直接传参的命令列表，不需要 shell 变量展开。

需要填写、核对的内容：

1. 顶部数据库和 Redis **原始密码**，以及应用连接 URL 中对应的 **URL 编码密码**。已有 PostgreSQL 数据卷不会因修改初始化环境变量而更换密码。
2. 原 `SESSION_SECRET`、Cookie 设置、端口、镜像和其他业务参数，均按当前部署填写；原部署已设置 `CRYPTO_SECRET` 时沿用原值，不要随意新增或更换。
3. 实际 PostgreSQL 数据卷和 Docker 网络名称。模板将两者声明为 `external`，填错时会失败，不会自动创建一个空数据库。
4. OSS Bucket、地域 Endpoint、AccessKey，以及备份计划。密钥和密码中包含 `$` 时，在 YAML 中写成 `$$`；URL 密码还需另做百分号编码。
5. 应用的实际 `data`、`logs` 路径，以及配置文件本身的文件名。复制到 `docker-compose.yml` 时，把备份挂载的 `source: ./compose.yml` 相应改为 `./docker-compose.yml`。额外的 Compose override 和外部密钥文件也需只读挂入 `/source/config/`。

将模板放到实际部署目录后，相对路径才指向部署目录内的文件。继续使用原来的 Compose 项目名、启动目录及 `-p` 参数。**不要在仓库示例目录直接启动业务服务。** 填好真实凭据的部署文件留在服务器，不回写到 Git。

以下均在实际部署目录执行。假设服务器文件名为 `compose.yml`；如果原文件叫 `docker-compose.yml`，替换命令中的文件名。

```bash
# 只校验配置，不创建、重启或修改服务。
docker compose -f compose.yml --profile backup config -q

# 只拉取备份镜像。
docker compose -f compose.yml --profile backup pull backup

# 首次才初始化选定的仓库；已有仓库跳过。
docker compose -f compose.yml --profile backup run --rm --no-deps backup init

# 先执行一次备份，成功后按主说明做恢复演练。
docker compose -f compose.yml --profile backup run --rm --no-deps backup backup

# 最后只启动备份容器，不重启现有业务服务。
docker compose -f compose.yml --profile backup up -d --no-deps backup
```

备份加密密码仍通过 `./secrets/backup/restic-password` 独立文件提供。只写密码正文，限制访问权限，并在其他安全位置保存一份；它与 OSS AccessKey 无关。

已有容器的卷和网络可只读查看，下面不会修改容器：

```bash
# 容器名请换成当前实际名称；卷名称与网络名称以输出为准。
docker inspect postgres-lab --format '{{json .Mounts}}'
docker inspect postgres-lab --format '{{json .NetworkSettings.Networks}}'
```

备份范围、在线一致性、恢复和保留策略见上一级 `README.md`。

需要按某个快照恢复时，使用独立的 `recovery.compose.yml`，步骤见 `RECOVERY.md`。恢复工具不依赖原数据库或 `data`/`logs` 仍然存在。

本地验证已覆盖完整模板的 profile 开关、直接配置不受 `.env` 干扰、共享密码的 `$` 实际容器传递，以及独立恢复模板从两份加密快照中取回指定旧快照和拒绝覆盖已有恢复目录。验证未连接生产或真实 OSS。
