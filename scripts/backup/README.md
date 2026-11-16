# New API 独立备份容器

备份服务与 New API、PostgreSQL、Redis 放在同一个 Compose 项目中，通过 `backup` profile 按需启用。它直接连接 PostgreSQL，以只读挂载读取应用文件，用 Restic 在本地加密后经 rclone 上传阿里云 OSS，不需要 Docker socket，也不会停止应用、导入数据库或修改源文件。

这里的命令是部署示例。应先在隔离环境完成备份和恢复演练，再启用生产任务；不要直接把示例中的占位配置用于生产。

## 备份范围

| 内容 | 默认来源 | 用途 |
| --- | --- | --- |
| PostgreSQL 数据库 | `postgres:5432/new-api` | 用户、令牌、渠道、额度、配置、数据库内的使用日志等 |
| 应用文件 | `./data` → `/source/data` | 审计详情、附件、发票等实际存在的持久文件 |
| 程序日志 | `./logs` → `/source/logs` | 文件形式的运行日志，与数据库中的使用日志不同 |
| 部署配置 | `.env`、`docker-compose.yml` → `/source/config` | 恢复数据库账号、原应用密钥及部署方式 |

额外的 Compose override、外部密钥文件、Nginx 配置、独立审计目录，应在 `backup.volumes` 中添加只读挂载到 `/source/config` 或 `/source` 的其他子目录。备份只包含实际挂载的文件，不会自动发现宿主机其他目录。

默认不备份 Redis。作为可重建缓存时通常无需恢复；如果你另外在 Redis 中保存了不可重建的数据，需要单独设计并验证 Redis 原生备份。不要把运行中的 PostgreSQL 数据卷直接打包作为在线数据库备份。

`pg_dump` 对单个数据库提供一致性快照。不同数据库、数据库与文件之间没有同一事务快照，manifest 会标记 `online-uncoordinated` 并记录采集时间。在线备份不等于严格时间点恢复，也不保证最后一条请求都已包含；最终迁移或要求数据库与附件严格一致时，需要协调停写。需要更小的数据丢失窗口时，应另行配置 PostgreSQL WAL 归档/PITR。

## 代码放在哪里

```text
scripts/backup/
  Dockerfile                独立备份镜像，包含 PostgreSQL 客户端、Restic、rclone
  runner.py                 配置校验、调度、导出、上传、状态和恢复入口
  test_runner.py            关键失败路径和配置校验测试
  README.md                 本说明
  compose-postgres.sh        原有宿主机备份脚本，继续保留
  postgres.conf.example     原有宿主机脚本配置示例
```

新容器不调用原宿主机脚本，不需要安装宿主机 Docker CLI。业务镜像与备份镜像分别构建，修改备份代码不需要重新编译 New API。

默认从 Docker Hub 拉取独立镜像 `thefoolishfox1/new-api-backup:latest`，不需要在服务器编译。部署目录只需保留 Compose、`.env`、要备份的目录和密码文件，不需要复制 `scripts/backup/`。生产环境建议通过 `BACKUP_IMAGE=thefoolishfox1/new-api-backup:20261116` 固定版本；`latest` 用于跟随更新。当前发布镜像仅支持 `linux/amd64`。

镜像中的 Restic、rclone 固定源码版本，并用 Go 1.26.10 编译；编译器不进入运行镜像。如需修改或自行构建，在仓库根目录执行：

```bash
docker build -t thefoolishfox1/new-api-backup:local scripts/backup
```

然后在 `.env` 设置 `BACKUP_IMAGE=thefoolishfox1/new-api-backup:local`，使用本地构建的镜像。仅这种方式需要备份源码目录；默认 Compose 不包含构建配置。

## 首次配置

如果希望全部参数直接写在 Compose 中，使用 [无需 .env 的完整模板](examples/compose.yml) 并参照 [逐项修改说明](examples/README.md)。这是独立示例，不会替换仓库原来的默认 Compose 文件。

灾后需要按某个快照恢复时，另有 [独立恢复模板](examples/recovery.compose.yml) 和 [恢复操作说明](examples/RECOVERY.md)，不依赖原应用、数据库或业务目录仍然存在。

在部署目录的 `.env` 中填写新增的 `BACKUP_*` 配置。下面仅示意必要项，其余默认值和说明见仓库的 `.env.example`：

```dotenv
# 启用后，正常 docker compose up -d 会同时管理备份服务。
COMPOSE_PROFILES=backup

# 生产环境建议固定版本；不设置时默认使用 latest。
BACKUP_IMAGE=thefoolishfox1/new-api-backup:20261116

# Endpoint 必须与 Bucket 所在地域相同；不填写协议、Bucket 或路径。
BACKUP_OSS_ENDPOINT=oss-cn-hangzhou.aliyuncs.com
BACKUP_OSS_BUCKET=your-private-backup-bucket
BACKUP_OSS_PREFIX=newapi/production
BACKUP_OSS_ACCESS_KEY_ID=replace-with-dedicated-ram-access-key-id
BACKUP_OSS_ACCESS_KEY_SECRET=replace-with-dedicated-ram-access-key-secret

# 不同部署必须使用不同的稳定标识，避免保留策略混在一起。
BACKUP_HOST=newapi-production
BACKUP_TAG=new-api-production
BACKUP_SCHEDULE_MODE=daily
BACKUP_TZ=Asia/Shanghai
BACKUP_TIMES=03:30
```

默认仓库地址由 Bucket 和前缀组成：`rclone:oss:<bucket>/<prefix>`。同一仓库初始化后，不要随意修改其地址、加密密码或备份身份标识。

用密码管理器生成足够长的随机备份加密密码，保存为部署目录下的 `./secrets/backup/restic-password`，不带其他配置内容。该文件及其父目录应仅允许部署管理员访问，例如 Linux 上目录权限 `700`、文件权限 `600`；`.env` 也应限制为 `600`。不要把真实密码提交到 Git、写进 Dockerfile 或终端命令参数。

**把 Restic 密码再保存一份到独立的安全位置。** 只有 OSS 数据但丢失密码时无法解密；如果恢复所需的唯一密码副本也在这个加密仓库里，就无法开始恢复。原 `SESSION_SECRET`、`CRYPTO_SECRET` 也应保留，不能在恢复时重新生成以替代原值。完整备份内包含部署配置，因此可以包含其他服务密钥，应按敏感数据管理访问权限。

如果使用 `BACKUP_OSS_ACCESS_KEY_ID_FILE`、`BACKUP_OSS_ACCESS_KEY_SECRET_FILE` 或数据库密码文件，需把文件只读挂入容器，并配置对应的容器内路径。仅填写宿主机路径不会让文件自动出现在容器中。环境变量和挂载文件均可被有权限的宿主机管理员读取，不能代替主机权限管理。

OSS 建议使用专用私有 Bucket 或专用前缀及 RAM 用户，并按前缀限制访问。正常备份需要列举、读取、写入和处理仓库锁；Restic 释放锁时也需要删除锁对象，因此不能简单禁止全部删除权限。分片上传还需要相应的 OSS 权限。若需要把日常备份与数据删除权限分开，应对 `locks/` 和数据对象分别设计并实际验证策略。

**不要给 Restic 仓库配置“按对象年龄直接删除文件”的 OSS 生命周期规则。** 新快照可能仍引用很早写入的数据块，直接删除会损坏仍在保留期内的备份。OSS 版本控制、WORM/保留锁可作为额外保护，但会影响 prune 和实际空间释放，需要单独规划。

## 拉取、初始化与启动

先确认现有数据库已运行、配置文件与目录存在，再执行以下命令。`--no-deps` 保证这些一次性命令不会顺带启动其他服务：

```bash
docker compose --profile backup pull backup

# 仅首次创建一个明确选定的空仓库时执行。
docker compose --profile backup run --rm --no-deps backup init

# 先完成一次备份，并记录成功输出中的 snapshot ID。
docker compose --profile backup run --rm --no-deps backup backup

docker compose --profile backup run --rm --no-deps backup snapshots
docker compose --profile backup run --rm --no-deps backup check

# 启动长期调度服务；不重启业务服务。
docker compose --profile backup up -d --no-deps backup
```

`init` 是显式操作。仓库连不上、密码错误或地址配置错误时，服务会失败，不会自行初始化另一个空仓库来掩盖问题。已有仓库无需再次初始化。

调度服务每次启动或重启后先备份一次，再按配置的计划运行。默认仍为每天北京时间 `03:30`。任务失败只影响备份服务，New API 不依赖它的启动或健康状态。

未启用 `backup` profile 时，原有应用启动方式保持不变。不要同时运行原宿主机脚本和新容器去备份同一仓库；如确有多机器任务，需协调任务时间，并保留 Restic 仓库锁。

## 配置备份计划

在部署目录 `.env` 中选择一种模式；`daily` 和 `interval` 不会同时执行。

每天在指定时区的多个时间点备份，例如北京时间 `03:30`、`15:30`、`23:00`：

```dotenv
BACKUP_SCHEDULE_MODE=daily
BACKUP_TZ=Asia/Shanghai
BACKUP_TIMES=03:30,15:30,23:00
```

时间点以英文逗号分隔，重复时间会合并，并按时间排序。`BACKUP_TIMES` 非空时优先使用；未设置或留空时，继续读取原来的 `BACKUP_TIME`，默认 `03:30`，所以已有单时间点配置仍可使用。Compose 将 `BACKUP_TZ` 传入容器的 `TZ`；直接运行镜像时应设置 `TZ`。

每天的时间点按当地日历计算。上一轮仍在运行时经过的时间点会跳过，不会积压或并发补跑。使用夏令时地区时，不存在的当地时间向后归一化；归一化后重合的时间只执行一次，秋季重复出现的当地时间只使用第一次。

按固定间隔备份，例如每轮结束后等待 6 小时：

```dotenv
BACKUP_SCHEDULE_MODE=interval
BACKUP_INTERVAL=6h
```

间隔支持正整数加单位：`30m` 表示 30 分钟，`6h` 表示 6 小时，`1d` 表示 24 小时；范围是 1 分钟至 30 天。间隔从上一轮任务**结束时**开始计算，成功和失败都遵循这一规则，因此备份耗时不包含在等待间隔中，也不会重叠执行。等待使用单调时钟，不受系统校时或时区变化影响；容器重启仍先立即备份一次，然后重新计算间隔。此模式忽略 `BACKUP_TIMES`/`BACKUP_TIME`，每天定点模式则忽略 `BACKUP_INTERVAL`。

这里的“上一轮”指该调度容器执行的任务。另外运行一次手动备份不会重置正在等待的计时器；所有备份仍共用任务锁，避免同时导出。

已有部署升级时，先在 `.env` 选择需要的镜像版本，再拉取并重新创建备份容器：

```bash
docker compose --profile backup pull backup
docker compose --profile backup up -d --no-deps backup
```

之后只修改 `.env` 中的计划时，执行第二条命令即可使新配置生效，不需要重启 New API。使用 `:local` 自建镜像时，应先重新执行前面的 `docker build`，不需要执行 `pull`。

## 数据库与资源设置

默认 PostgreSQL 客户端主版本为 15，与当前 Compose 的 PostgreSQL 15 匹配。客户端不能导出比自身更新的服务端；升级数据库主版本时应先同步备份镜像并重新演练。数据库连接默认使用 Compose 内网，不需要新增公网端口。

默认从现有 `POSTGRES_PASSWORD` 取得数据库密码。不同部署可配置 `BACKUP_DB_HOST`、`BACKUP_DB_PORT`、`BACKUP_DB_USER`、`BACKUP_DB_PASSWORD_FILE`、`BACKUP_DATABASES`。例如数据库服务实际名为 `postgres-lab`，就把 `BACKUP_DB_HOST` 改成 `postgres-lab`，无需改业务容器名称。多个数据库用英文逗号分隔；如果 `LOG_SQL_DSN` 指向同一实例的另一个库，也应列入。日志库位于其他服务器或使用其他数据库引擎时，需要独立任务，不能用本任务假装已覆盖。

可使用专用导出账号，但它必须有完整导出所需的库、schema、表、序列等权限。新表、扩展、大对象和 RLS 都可能影响权限；PostgreSQL 的 `pg_read_all_data` 不会自动绕过 RLS。应以实际恢复验证证明导出完整，不能通过启用行过滤来消除权限错误。此服务不自动修改数据库权限、不导出集群级角色/表空间；恢复前应按部署配置重建账号，自定义角色及其他集群级配置需另行保存。

| 配置 | 默认值 | 作用 |
| --- | --- | --- |
| `BACKUP_TIMEOUT_SECONDS` | `7200` | 限制任务/操作耗时，避免永久挂住 |
| `BACKUP_RETRIES` | `2` | 网络上传等可重试操作的额外重试次数 |
| `BACKUP_RETRY_DELAY_SECONDS` | `10` | 重试间隔基数 |
| `BACKUP_MIN_FREE_MB` | `1024` | 工作目录最低剩余空间 |
| `BACKUP_MAX_DUMP_MB` | `4096` | 导出文件空间上限 |
| `BACKUP_MAX_CACHE_MB` | `512` | 本地 Restic 缓存预算 |
| `BACKUP_BW_LIMIT` | `8M` | 对象存储传输带宽限制 |
| `BACKUP_HEALTH_MAX_AGE_HOURS` | 留空时自动计算 | 最近成功备份允许的最大间隔；每天定点默认 30 小时，间隔模式取 30 小时与“间隔 + 任务超时 + 1 小时”中较大值 |

数据库导出先写私有临时文件，验证成功后再上传；不会另做一份完整的 `data`/`logs` tar 包。每次仍需生成 PostgreSQL 完整逻辑导出，Restic 则对文件和数据块去重，不能把它等同于数据库 WAL 增量备份。临时导出与缓存存放在独立卷中，不能把该卷再次挂到备份源里。

`BACKUP_MAX_DUMP_MB` 限制本次全部数据库导出的合计大小；导出和缓存运行中会检查阈值，超过即停止，检查之间仍可能有少量增长。`BACKUP_MAX_CACHE_MB` 是缓存保护阈值，超限时停止任务并清理专用缓存，不能当作文件系统硬配额。重试只作用于明确识别的临时网络、仓库锁或数据库启动连接失败，不会因为上传失败重新导出数据库。

按数据库实际大小为导出留出空间，尤其不要在磁盘接近满时直接启用。最大导出值是保护阈值，需要随数据库增长调整。备份会使用数据库读取、磁盘、CPU、内存和带宽；Compose 限制资源可减小争用，但不能保证零影响，应在业务低峰进行首次全量备份和完整检查。

独立 Docker 卷不等于独立物理磁盘。默认仍共享 Docker 所在磁盘；条件允许时可把状态和缓存卷放到专用磁盘。空间保护只限制备份自身，其他服务把宿主机磁盘完全写满时，仍需要先释放空间才能恢复任务及状态写入。

## 失败处理和检查

任务使用本地锁防止重叠，Restic 同时使用远端仓库锁。不会自动执行 `unlock`；看到锁错误时，应先确认是否还有其他备份或维护任务正在运行。只终止本服务拥有的子进程，不会终止应用或数据库进程。

导出失败、读取源文件失败、Restic 非零退出、无法确认成功快照等均不应作为成功备份。失败后清理本次临时导出，保留既有远端快照；不会因失败自动删除旧备份。OSS 不可用或凭据错误时，应处理原因，不能依靠无限重试。

查看服务状态与日志：

```bash
docker compose --profile backup ps backup
docker compose --profile backup logs --tail=100 backup
docker compose --profile backup exec backup python3 /opt/backup/runner.py health
```

健康检查关注最近一次成功时间和任务失败状态。Docker 的 `unhealthy` 标记本身不会自动重启容器；重启策略主要处理进程异常退出。应将备份失败/过期纳入现有监控，并定期检查 OSS 可访问性。容器存活、`pg_restore --list` 成功都不代表整份备份可恢复。

`BACKUP_HEALTH_MAX_AGE_HOURS` 可显式覆盖自动阈值。使用较长备份间隔时，不要把它设置得比正常间隔还短，并应为任务运行预留时间，否则可能在下一轮正常备份前被标记为过期。留空时，间隔模式会自动放宽阈值，避免例如每 2 天备份却沿用 30 小时阈值的问题；任务本身一旦失败仍会标记不健康。

较长的自动阈值只适用于已有成功备份的任务。首次备份尚未成功时，初始健康宽限期最多仍为 30 小时，反复重启不会重置这段宽限期。

`check` 会读取仓库数据进行完整性检查，可能消耗较多 OSS 流量和时间，应单独在低峰执行。最有意义的验证仍是定期恢复到隔离环境，检查数据库记录、文件与应用行为。

## 保留策略

默认保留最近 7 个日备份、4 个周备份、6 个月备份，并至少保留最近两份。日/周/月是 Restic 的时间分组保留规则，不等于分别精确保留这些天数；这些规则取并集。可用周期不足时，Restic 还可能保留最旧快照以延长可恢复历史，因此实际数量可能更多。

不会在每次备份后自动删除历史数据。先查看计划，再确认执行：

```bash
# 仅展示即将保留/删除的快照，不执行删除。
docker compose --profile backup run --rm --no-deps backup retention

# 确认计划、存在近期成功备份且已完成恢复演练后，才执行。
docker compose --profile backup run --rm --no-deps backup retention --apply
```

保留规则仅针对相同 `BACKUP_HOST` 和 `BACKUP_TAG` 的任务。`--apply` 会删除符合条件的历史快照并清理不再被引用的数据，属于不可随意撤销的维护操作。不要在有其他主机备份时并行维护，也不要在备份失败后为了腾空间立即清除仅存的可用恢复点。

## 恢复演练

先查出需要的明确快照 ID。恢复命令要求至少 8 位十六进制 ID，不使用随时间变化的 `latest`；目标必须是绝对路径且尚不存在。把宿主机专用演练目录挂到 `/restore`，恢复到它的一个新子目录：

```bash
mkdir -p restore-test
docker compose --profile backup run --rm --no-deps \
  -v "$PWD/restore-test:/restore" \
  backup restore-files REPLACE_WITH_SNAPSHOT_ID /restore/rehearsal-001
```

这里仅执行 `restic restore --verify`，不会导入数据库或覆盖线上 `data`/`logs`。恢复结果保留备份中的绝对路径层级；在目标目录里找到 `manifest.json`、对应的 `.dump` 及 `source` 文件，按 manifest 对齐数据库和文件。校验和用于检查导出文件是否完整，不替代真正恢复。

接下来在**另一个全新、空的测试数据库**上使用匹配版本的 `pg_restore`，例如：

```bash
# 事先通过 PGHOST/PGPORT/PGUSER/PGPASSFILE 指向隔离测试实例。
# TEST_DATABASE 仅为新建空库的名称，不能填写生产数据库连接。
pg_restore --no-password --no-owner --no-acl \
  --exit-on-error --dbname="$TEST_DATABASE" \
  /path/to/restored/database.dump
```

`--no-owner --no-acl` 便于在演练账号下恢复，意味着需要按部署要求另行恢复角色、所有权和授权，不能据此认为原权限已自动还原。验证关键表数量、用户/渠道/额度等记录和附件读取，最后才规划正式恢复。正式恢复还需协调写入窗口与流量切换，不能直接将旧备份导入仍在接单的服务。

## 备份容器放在另一台服务器

可以远程连接 PostgreSQL，但 `/source` 的只读挂载只能读取备份容器所在主机的文件。仅在 `.env` 填写应用服务器地址，无法读取另一台机器的 `data`/`logs`。跨服务器部署还需要可靠的共享存储或单独的 SSH/SFTP 文件采集流程；当前默认方案更适合放在应用宿主机，将最终加密数据存入异地 OSS。

数据库跨公网连接应使用可信 CA 和服务端身份校验，优先走私网/VPN，并限制来源地址；不要为方便备份直接向公网开放数据库。OSS 仓库加密保护的是上传备份，不替代数据库传输链路保护。

## 验证边界

开发验证可在仓库根目录执行（只创建隔离测试资源）：

```bash
docker build -t newapi-backup:test scripts/backup
docker run --rm --entrypoint python3 \
  -v "$PWD/scripts/backup:/tests:ro" \
  newapi-backup:test -m unittest discover -s /tests -p test_runner.py
python scripts/backup/test_integration.py --image newapi-backup:test
```

依赖下载缓慢时，可在构建时指定可信的 Go module proxy，例如 `--build-arg GOPROXY=https://goproxy.cn`；仍保留 Go 校验和数据库验证。集成测试需要本机 Docker，临时启动 PostgreSQL 15 与 MinIO，不开放宿主机端口，结束后只清理本次创建的 Docker 资源；测试报告留在已忽略的 `.local-tests/` 中。

2026-11-16 本地 Linux/amd64 验证：38 项单元测试、75 项集成检查通过。数据库及客户端为 PostgreSQL 15.20，对象存储为 MinIO 2024.6.13；Restic 0.18.1、rclone 1.71.2 均使用 Go 1.26.10。实际完成三轮双库恢复（含中文、数值、二进制数据和索引）、文件校验及清理后的再次恢复；错误凭据、空间不足和超时均未删除已有恢复点。两种计划均验证了真实容器启动备份、下一次执行时间、健康检查和等待期间正常停止；时间选择、跨日、夏令时及防积压使用确定性单元测试验证。

本地隔离的 PostgreSQL 与 S3 兼容对象存储可用于验证完整的备份、加密、恢复和失败处理流程。它不能代替阿里云 OSS 的真实验证。正式使用前，还应以独立测试 Bucket 验证实际地域 Endpoint、RAM 权限、上传/下载、锁释放、保留清理和恢复，不需要连接生产数据来完成这一步。
