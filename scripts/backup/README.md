# New API 独立备份容器

备份服务与 New API、PostgreSQL、Redis 放在同一个 Compose 项目中，通过 `backup` profile 按需启用。它直接连接 PostgreSQL，以只读挂载读取应用文件，用 Restic 在本地加密后经 rclone 上传 S3 兼容对象存储，支持 MinIO 和阿里云 OSS。它不需要 Docker socket，也不会停止应用、导入数据库或修改源文件。

这里的命令是部署示例。应先在隔离环境完成备份和恢复演练，再启用生产任务；不要直接把示例中的占位配置用于生产。

## 备份范围

| 内容 | 默认来源 | 用途 |
| --- | --- | --- |
| PostgreSQL 数据库 | `postgres:5432/new-api` | 用户、令牌、渠道、额度、配置、数据库内的使用日志等 |
| 应用文件 | `./data` → `/source/data` | 审计详情、附件、发票等实际存在的持久文件 |
| 程序日志 | `./logs` → `/source/logs` | 文件形式的运行日志，与数据库中的使用日志不同 |
| 部署配置 | `docker-compose.yml` → `/source/config` | 恢复数据库账号、原应用密钥及部署方式；外部配置需另加只读挂载 |

额外的 Compose override、外部密钥文件、Nginx 配置、独立审计目录，应在 `backup.volumes` 中添加只读挂载到 `/source/config` 或 `/source` 的其他子目录。备份只包含实际挂载的文件，不会自动发现宿主机其他目录。

根目录 Compose 中的 New API、PostgreSQL、Redis 和备份参数均直接填写，不需要 `.env` 或 shell 变量展开。Redis 使用直接传参的命令列表。若你自己的部署还使用额外的 `.env`、override 或密钥文件，也要将实际文件只读挂入备份，才能保存完整恢复配置；这只用于采集外部文件，不是根目录模板的配置依赖。

```yaml
# 仅在自定义部署另外使用这个现有文件时，添加到 backup.volumes。
- type: bind
  source: ./.env
  target: /source/config/deploy.env
  read_only: true
  bind: {create_host_path: false}
```

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

默认从 Docker Hub 拉取独立镜像 `thefoolishfox1/new-api-backup:20261117-s3-backup`，不需要在服务器编译。部署目录需保留 Compose、实际业务配置、要备份的目录和密码文件，不需要复制 `scripts/backup/`。镜像版本直接修改 `backup.image`；`latest` 用于跟随更新。当前发布镜像仅支持 `linux/amd64`。

镜像中的 Restic、rclone 固定源码版本，并用 Go 1.26.10 编译；编译器不进入运行镜像。如需修改或自行构建，在仓库根目录执行：

```bash
docker build -t thefoolishfox1/new-api-backup:local scripts/backup
```

然后将 `backup.image` 改为 `thefoolishfox1/new-api-backup:local`，使用本地构建的镜像。仅这种方式需要备份源码目录；默认 Compose 不包含构建配置。

## 首次配置

根目录 `docker-compose.yml` 已使用全部直接填写的配置。另有采用 lab 服务名的 [完整模板](examples/compose.yml) 和 [逐项修改说明](examples/README.md)，供核对服务器配置。请将数据库及 Redis 的原始密码、连接 URL 中对应的编码密码、原 `SESSION_SECRET` 和 Cookie 设置填入相应位置；原部署如果设置了 `CRYPTO_SECRET`，也须沿用原值。服务名、端口、数据卷和网络按服务器现有部署填写。

灾后需要按某个快照恢复时，另有 [独立恢复模板](examples/recovery.compose.yml) 和 [恢复操作说明](examples/RECOVERY.md)，不依赖原应用、数据库或业务目录仍然存在。

同一个 Compose 文件顶部已保留两套存储配置：`x-backup-storage-minio` 和 `x-backup-storage-aliyun`。先在要使用的配置块中填写桶、Endpoint 和凭据，再通过 `backup.environment` 内的 `<<` 一行选择。数据库、计划等参数仍直接填在 `backup.environment`，例如：

```yaml
environment:
  <<: *backup-storage-minio
  TZ: Asia/Shanghai
  BACKUP_DB_HOST: postgres
  BACKUP_DB_PORT: "5432"
  BACKUP_DB_USER: "root"
  BACKUP_DB_PASSWORD: "REPLACE_WITH_EXISTING_POSTGRES_PASSWORD"
  BACKUP_DATABASES: new-api
  BACKUP_PASSWORD_FILE: /run/secrets/restic-password
  BACKUP_AUTO_INIT: "true"
  BACKUP_HOST: newapi
  BACKUP_TAG: new-api
  BACKUP_SCHEDULE_MODE: daily
  BACKUP_TIMES: "03:30"
```

仓库地址直接填写完整的 `rclone:s3:<bucket>/<prefix>`，不会引用同一 `environment` 中的其他字段。MinIO 的 Endpoint 要填 **S3 API 地址**，通常是 `9000` 端口，不能填管理控制台的 `9001`；通过 HTTPS 反向代理访问时填写对应的 API 域名。必须显式填写 Endpoint，不自动发现服务地址。直接配置的密码包含 `$` 时，在 Compose 中写成 `$$`。

两套存储块的字段如下，直接填写在当前 Compose 文件，不需要 `.env`：

| 配置 | MinIO | 阿里云 OSS 示例 |
| --- | --- | --- |
| `BACKUP_REPOSITORY` | `rclone:s3:你的桶/newapi/production` | `rclone:s3:你的桶/newapi/production` |
| `BACKUP_S3_PROVIDER` | `Minio` | `Alibaba` |
| `BACKUP_S3_ENDPOINT` | `https://minio.example.com` | `oss-cn-shenzhen.aliyuncs.com` |
| `BACKUP_S3_REGION` | 默认 `us-east-1`，按服务实际配置 | `cn-shenzhen`，必须与桶地域匹配 |
| `BACKUP_S3_FORCE_PATH_STYLE` | `true` | `false` |
| `BACKUP_S3_ACCESS_KEY_ID` / `BACKUP_S3_ACCESS_KEY_SECRET` | MinIO 专用访问凭据 | OSS 专用访问凭据 |
| `BACKUP_ALLOW_INSECURE_S3` | 默认 `false` | `false` |

默认使用 HTTPS 并校验证书。仅在明确选择可信网络中的 HTTP API 时，填写如 `http://minio:9000`，并显式设置 `BACKUP_ALLOW_INSECURE_S3: "true"`；该开关不关闭 HTTPS 证书校验。Provider 还接受 `AWS` 和 `Other`，均需显式配置对应 Endpoint、Region 和凭据。

使用 MinIO 时保留 `<<: *backup-storage-minio`。切换到阿里云时，只将该行改为 `<<: *backup-storage-aliyun`；不需要新增 YAML 文件或额外 `-f` 参数。两套配置可以同时保留，未选中的配置块不会创建服务或连接存储。

如果将备份服务合并进原有 Compose，请连同顶部两段 `x-backup-storage-*` 定义一起放入同一个文件，并位于引用之前。不要在 `backup.environment` 再重复填写存储字段，否则显式字段会覆盖选中的配置块。以下命令只校验、拉取和更新备份服务：

```bash
docker compose -f docker-compose.yml --profile backup config -q
docker compose -f docker-compose.yml --profile backup pull backup
docker compose -f docker-compose.yml --profile backup up -d --no-deps backup
```

旧 `rclone:oss:<bucket>/<prefix>` 与 `BACKUP_OSS_ENDPOINT`、`BACKUP_OSS_ACCESS_KEY_ID`、`BACKUP_OSS_ACCESS_KEY_SECRET`（及其 `_FILE` 形式）继续兼容，HTTP 仍需原来的 `BACKUP_ALLOW_INSECURE_OSS` 开关。已有 OSS 部署不必为了升级改仓库地址；两套变量按仓库前缀选择，不要混填。同一仓库初始化后，更新容器或迁移到新机器时应继续使用原仓库、加密密码和备份身份标识。

这些名称是传入备份容器的环境变量；值直接写在 Compose 的 `environment` 中，根目录模板不会从宿主机 `.env` 获取它们。资源限制直接修改 `backup.cpus`、`backup.mem_limit` 等服务字段。

用密码管理器生成足够长的随机备份加密密码，保存为部署目录下的 `./secrets/backup/restic-password`，不带其他配置内容。该文件及其父目录应仅允许部署管理员访问，例如 Linux 上目录权限 `700`、文件权限 `600`；含凭据的 Compose 和其他业务配置也应限制访问。不要把真实密码提交到 Git、写进 Dockerfile 或终端命令参数。

**把 Restic 密码再保存一份到独立的安全位置。** 只有对象存储数据但丢失密码时无法解密；如果恢复所需的唯一密码副本也在这个加密仓库里，就无法开始恢复。原 `SESSION_SECRET`、`CRYPTO_SECRET` 也应保留，不能在恢复时重新生成以替代原值。完整备份内包含部署配置，因此可以包含其他服务密钥，应按敏感数据管理访问权限。

如果使用 `BACKUP_S3_ACCESS_KEY_ID_FILE`、`BACKUP_S3_ACCESS_KEY_SECRET_FILE`、旧版 OSS 凭据文件或数据库密码文件，需把文件只读挂入容器，并配置对应的容器内路径。仅填写宿主机路径不会让文件自动出现在容器中。环境变量和挂载文件均可被有权限的宿主机管理员读取，不能代替主机权限管理。

对象存储建议使用专用私有 Bucket 或专用前缀及访问账号；阿里云可使用 RAM 用户，按前缀限制访问。正常备份需要列举、读取、写入和处理仓库锁；Restic 释放锁时也需要删除锁对象，因此不能简单禁止全部删除权限。分片上传还需要相应权限。若需要把日常备份与数据删除权限分开，应对 `locks/` 和数据对象分别设计并实际验证策略。

**不要给 Restic 仓库配置“按对象年龄直接删除文件”的对象存储生命周期规则。** 新快照可能仍引用很早写入的数据块，直接删除会损坏仍在保留期内的备份。对象版本控制、WORM/保留锁可作为额外保护，但会影响 prune 和实际空间释放，需要单独规划。

## 拉取与启动

先确认现有数据库已运行、配置文件与目录存在，再执行以下命令。`--no-deps` 保证这些一次性命令不会顺带启动其他服务：

```bash
docker compose --profile backup pull backup

# 只启动或更新备份容器；仓库检查及必要的初始化由首轮任务处理。
docker compose --profile backup up -d --no-deps backup

docker compose --profile backup logs --tail=100 backup
```

先确认日志中的首次备份已经成功，再查看快照和做完整检查、恢复演练。这些操作与正在进行的备份共用任务锁；任务仍在运行时应等待它完成。

```bash
docker compose --profile backup run --rm --no-deps backup snapshots
docker compose --profile backup run --rm --no-deps backup check
```

## 自动检查与初始化仓库

`BACKUP_AUTO_INIT` 默认是 `true`。每次 `backup` 或调度任务开始时，先检查仓库，再导出数据库：

- 已有可解密的有效仓库：校验仓库 ID 后直接复用，更新或重建容器不需要重复初始化。
- 明确返回“仓库不存在”，并确认所选前缀为空：自动创建仓库，再验证其配置和 ID，成功后才继续备份。
- 网络、权限、密码错误，或者前缀非空却没有可识别的仓库：任务失败，不会重新初始化或覆盖已有内容。

初始化写操作只执行一次；即使返回“已存在”或网络错误，也只在停止/超时前重新读取配置，验证密码与仓库 ID 后才能复用。回读失败则保留初始化失败，不会仅凭“已存在”忽略错误。仓库核验通过后仍须实际备份和验证快照，才会记录备份成功。

MinIO 或 OSS 的 Bucket 必须提前创建，Endpoint、访问凭据和独立保存的备份加密密码仍需人工配置。程序不会创建 Bucket、生成密码或修改代理；rclone 会跳过 CreateBucket，避免已有桶返回 `BucketAlreadyExists` 被误判为初始化失败。缺失的桶仍会在列举/访问阶段失败，不会被当作成功。新机器没有旧的本地状态时，也能直接接入已有有效仓库；如果填写的是另一个合法但空的前缀，则会被当作新仓库，因此首次使用或迁移时必须核对完整仓库地址。

状态卷会记录已使用仓库的 ID。已有记录时，仓库消失或 ID 改变会触发保护，避免将原备份丢失误当成首次安装。从旧版升级时，已存在的使用记录同样会阻止把缺失仓库自动当成全新安装。更换 Endpoint 或复制仓库后，只要仓库 ID 保持一致即可通过校验。保留状态卷有助于发现异常；清空状态会丢失这份本地身份记录，不能用来绕过错误。遇到保护错误时，先核对存储位置、权限和仓库迁移情况，再人工处理。

**把配置从 OSS 改成一个空的 MinIO 前缀，不会复制原仓库或旧快照。** 保留历史需要在协调停止仓库写入后，完整复制原 Restic 仓库的全部对象，保留原加密密码，再验证仓库 ID 和恢复结果；只复制 `snapshots/` 或 `config` 不足以恢复数据。若确实要开始一个全新备份历史，核对空目标及原仓库已妥善保留后，可以显式执行 `init` 创建新仓库并更新本地身份记录。不要删除旧对象、删除状态卷或执行 `unlock` 来绕过仓库身份保护；以后恢复旧快照仍需连接旧仓库或其完整副本。

首次使用**同一个空仓库**的多个节点，应只让一个节点完成初始化并确认备份成功，再启动其他节点。首次创建仓库不是跨主机原子操作，本地任务锁不能替代这一步协调。仓库建立后，各节点更新、重建或在新机器接入时会复用该仓库。

将 `BACKUP_AUTO_INIT` 设为 `false` 可以关闭备份任务的自动创建行为；已有有效仓库仍会复用，显式 `init` 仍可使用。`init` 重复执行会检查并复用有效仓库，不会重新创建它；人工确认切换到另一个有效仓库后，该命令也会更新本地仓库身份记录，因此不能将它当作忽略保护错误的通用重试。非空但损坏的仓库和损坏的本地身份记录仍会拒绝处理，需先查明原因。手动执行一次备份同样会走仓库检查：

```bash
# 可选：显式准备仓库；正常启动不再要求先运行此命令。
docker compose --profile backup run --rm --no-deps backup init

# 可选：手动立即备份一次。
docker compose --profile backup run --rm --no-deps backup backup
```

`snapshots`、`check` 和 `restore-files` 不会自动创建仓库。恢复或检查时遇到不存在、密码错误等问题，应先查明原因，不要将空仓库当作旧备份。

调度服务每次启动或重启后先备份一次，再按配置的计划运行。默认仍为每天北京时间 `03:30`。任务失败只影响备份服务，New API 不依赖它的启动或健康状态。

日志依次显示 `repository_ready`、每个数据库的 `exporting` / `exported`、`uploading`、`verifying`，最后才是 `success` 和快照 ID。导出日志只记录数据库序号、字节数和时间；`repository_ready` 仅表示仓库可打开，不代表数据已备份。`waiting` 只表示正在等待下一次计划，不表示上一轮已成功。失败日志会显示可识别的固定错误码（例如 `AccessDenied`、`NoSuchBucket`、`NoSpaceLeft`），不会输出原始错误中的凭据或签名 URL。

Restic 使用 `--json --quiet` 并固定 `RESTIC_PROGRESS_FPS=0`，关闭高频周期进度，保留最终快照回执和错误。这样避免长任务仅因进度输出累计超过捕获上限而失败。`uploading` 阶段可能持续较久，可据阶段时间判断导出与存储传输的耗时；不要因为没有逐秒进度反复重启任务。

未启用 `backup` profile 时，原有应用启动方式保持不变。不要同时运行原宿主机脚本和新容器去备份同一仓库；如确有多机器任务，需协调任务时间，并保留 Restic 仓库锁。

## 配置备份计划

在 `backup.environment` 中选择一种模式；`daily` 和 `interval` 不会同时执行。

每天在指定时区的多个时间点备份，例如北京时间 `03:30`、`15:30`、`23:00`：

```yaml
TZ: Asia/Shanghai
BACKUP_SCHEDULE_MODE: daily
BACKUP_TIMES: "03:30,15:30,23:00"
```

时间点以英文逗号分隔，重复时间会合并，并按时间排序。`BACKUP_TIMES` 非空时优先使用；未设置或留空时，继续读取原来的 `BACKUP_TIME`，默认 `03:30`，所以已有单时间点配置仍可使用。时区直接填写容器的 `TZ`。

每天的时间点按当地日历计算。上一轮仍在运行时经过的时间点会跳过，不会积压或并发补跑。使用夏令时地区时，不存在的当地时间向后归一化；归一化后重合的时间只执行一次，秋季重复出现的当地时间只使用第一次。

按固定间隔备份，例如每轮结束后等待 6 小时：

```yaml
BACKUP_SCHEDULE_MODE: interval
BACKUP_INTERVAL: "6h"
```

间隔支持正整数加单位：`30m` 表示 30 分钟，`6h` 表示 6 小时，`1d` 表示 24 小时；范围是 1 分钟至 30 天。间隔从上一轮任务**结束时**开始计算，成功和失败都遵循这一规则，因此备份耗时不包含在等待间隔中，也不会重叠执行。等待使用单调时钟，不受系统校时或时区变化影响；容器重启仍先立即备份一次，然后重新计算间隔。此模式忽略 `BACKUP_TIMES`/`BACKUP_TIME`，每天定点模式则忽略 `BACKUP_INTERVAL`。

这里的“上一轮”指该调度容器执行的任务。另外运行一次手动备份不会重置正在等待的计时器；所有备份仍共用任务锁，避免同时导出。

已有部署升级时，先修改 `backup.image` 选择需要的镜像版本，再拉取并重新创建备份容器：

```bash
docker compose --profile backup pull backup
docker compose --profile backup up -d --no-deps backup
```

之后只修改 `backup.environment` 中的计划时，执行第二条命令即可使新配置生效，不需要重启 New API。使用 `:local` 自建镜像时，应先重新执行前面的 `docker build`，不需要执行 `pull`。

## 数据库与资源设置

默认 PostgreSQL 客户端主版本为 15，与当前 Compose 的 PostgreSQL 15 匹配。客户端不能导出比自身更新的服务端；升级数据库主版本时应先同步备份镜像并重新演练。数据库连接默认使用 Compose 内网，不需要新增公网端口。

在 `BACKUP_DB_PASSWORD` 填写数据库现有密码；它不会自动引用其他服务或宿主机的变量。不同部署可配置 `BACKUP_DB_HOST`、`BACKUP_DB_PORT`、`BACKUP_DB_USER`、`BACKUP_DB_PASSWORD_FILE`、`BACKUP_DATABASES`。例如数据库服务实际名为 `postgres-lab`，就把 `BACKUP_DB_HOST` 改成 `postgres-lab`，无需改业务容器名称。多个数据库用英文逗号分隔；如果 `LOG_SQL_DSN` 指向同一实例的另一个库，也应列入。日志库位于其他服务器或使用其他数据库引擎时，需要独立任务，不能用本任务假装已覆盖。

可使用专用导出账号，但它必须有完整导出所需的库、schema、表、序列等权限。新表、扩展、大对象和 RLS 都可能影响权限；PostgreSQL 的 `pg_read_all_data` 不会自动绕过 RLS。应以实际恢复验证证明导出完整，不能通过启用行过滤来消除权限错误。此服务不自动修改数据库权限、不导出集群级角色/表空间；恢复前应按部署配置重建账号，自定义角色及其他集群级配置需另行保存。

| 配置 | 默认值 | 作用 |
| --- | --- | --- |
| `BACKUP_TIMEOUT_SECONDS` | `7200` | 限制任务/操作耗时，避免永久挂住 |
| `BACKUP_RETRIES` | `2` | 网络上传等可重试操作的额外重试次数 |
| `BACKUP_RETRY_DELAY_SECONDS` | `10` | 重试间隔基数 |
| `BACKUP_MIN_FREE_MB` | `1024` | 工作目录最低剩余空间 |
| `BACKUP_MAX_DUMP_MB` | `4096` | 导出文件空间上限 |
| `BACKUP_MAX_CACHE_MB` | `512` | 本地 Restic 缓存预算 |
| `BACKUP_TEMP_DIR` | 与 `BACKUP_CACHE_DIR` 相同，默认 `/cache` | Restic 临时加密数据块目录，可选择独立挂载的内存盘 |
| `BACKUP_BW_LIMIT` | `8M` | 对象存储传输带宽限制 |
| `BACKUP_HEALTH_MAX_AGE_HOURS` | 留空时自动计算 | 最近成功备份允许的最大间隔；每天定点默认 30 小时，间隔模式取 30 小时与“间隔 + 任务超时 + 1 小时”中较大值 |

数据库导出先写私有临时文件，验证成功后再上传；不会另做一份完整的 `data`/`logs` tar 包。每次仍需生成 PostgreSQL 完整逻辑导出，Restic 则对文件和数据块去重，不能把它等同于数据库 WAL 增量备份。临时导出与缓存存放在独立卷中，不能把该卷再次挂到备份源里。

Restic 按 `host,tags` 从相同主机、包含当前标签的快照中选择父快照，避免每轮随机导出目录让未变化的应用文件也重新读取。同一仓库中的独立部署应使用不同的 `BACKUP_HOST`；标签匹配要求包含当前标签，不要求整个标签集合完全相同。这保留了当前源目录的变更和删除检测，既不会将旧源文件混入新快照，也不会让多个数据库与文件采集变成同一原子快照。

Restic 的临时加密数据块默认使用磁盘上的缓存卷（`BACKUP_TEMP_DIR` 默认等于 `BACKUP_CACHE_DIR`），避免多个约 16 MiB 的并发数据块耗尽 Compose 的 64 MiB `/tmp` 内存盘。应保证默认缓存卷由磁盘提供，不要把它改挂为小容量 tmpfs。临时数据块由 Restic 关闭后释放；即使文件名已被取消链接，磁盘剩余空间检查仍能计入磁盘目录的实际占用。

有足够主机内存时，可只为备份服务显式配置独立的临时内存盘，例如将以下字段合并到现有 `backup` 服务。数据库导出和 Restic 缓存仍保存在原磁盘卷中，应用和数据库服务的配置不变。

```yaml
services:
  backup:
    mem_limit: 1g
    environment:
      BACKUP_TEMP_DIR: /restic-tmp
    tmpfs:
      - /tmp:size=64m,mode=1777
      - /restic-tmp:size=256m,mode=0700
```

这里的 `1g` 是备份容器总内存上限，包含实际使用的 tmpfs 内存；应先确认主机能承受这份预算。不要只把目录改回 64 MiB 的 `/tmp`。独立临时目录必须与源目录、状态、缓存及本地仓库分离，或直接等于缓存目录；容量不足时任务仍会失败。内存盘只可能减少临时块的磁盘读写，不能解决 OSS 链路丢包、网络限速或 CPU 配额瓶颈，也不保证备份更快。

`BACKUP_MAX_DUMP_MB` 限制本次全部数据库导出的合计大小；导出和缓存运行中会检查阈值，超过即停止，检查之间仍可能有少量增长。`BACKUP_MAX_CACHE_MB` 是缓存保护阈值，超限时停止任务并清理专用缓存，不能当作文件系统硬配额。重试只作用于明确识别的临时网络、仓库锁或数据库启动连接失败，不会因为上传失败重新导出数据库。

按数据库实际大小为导出留出空间，尤其不要在磁盘接近满时直接启用。最大导出值是保护阈值，需要随数据库增长调整。备份会使用数据库读取、磁盘、CPU、内存和带宽；Compose 限制资源可减小争用，但不能保证零影响，应在业务低峰进行首次全量备份和完整检查。

独立 Docker 卷不等于独立物理磁盘。默认仍共享 Docker 所在磁盘；条件允许时可把状态和缓存卷放到专用磁盘。空间保护只限制备份自身，其他服务把宿主机磁盘完全写满时，仍需要先释放空间才能恢复任务及状态写入。

## 失败处理和检查

任务使用本地锁防止重叠，Restic 同时使用远端仓库锁。不会自动执行 `unlock`；看到锁错误时，应先确认是否还有其他备份或维护任务正在运行。只终止本服务拥有的子进程，不会终止应用或数据库进程。

导出失败、读取源文件失败、Restic 非零退出、无法确认成功快照等均不应作为成功备份。失败后清理本次临时导出，保留既有远端快照；不会因失败自动删除旧备份。对象存储不可用或凭据错误时，应处理原因，不能依靠无限重试。

查看服务状态与日志：

```bash
docker compose --profile backup ps backup
docker compose --profile backup logs --tail=100 backup
docker compose --profile backup exec backup python3 /opt/backup/runner.py health
```

健康检查关注最近一次成功时间和任务失败状态。Docker 的 `unhealthy` 标记本身不会自动重启容器；重启策略主要处理进程异常退出。应将备份失败/过期纳入现有监控，并定期检查对象存储可访问性。容器存活、`pg_restore --list` 成功都不代表整份备份可恢复。

`BACKUP_HEALTH_MAX_AGE_HOURS` 可显式覆盖自动阈值。使用较长备份间隔时，不要把它设置得比正常间隔还短，并应为任务运行预留时间，否则可能在下一轮正常备份前被标记为过期。留空时，间隔模式会自动放宽阈值，避免例如每 2 天备份却沿用 30 小时阈值的问题；任务本身一旦失败仍会标记不健康。

较长的自动阈值只适用于已有成功备份的任务。首次备份尚未成功时，初始健康宽限期最多仍为 30 小时，反复重启不会重置这段宽限期。

`check` 会读取仓库数据进行完整性检查，可能消耗较多对象存储流量和时间，应单独在低峰执行。最有意义的验证仍是定期恢复到隔离环境，检查数据库记录、文件与应用行为。

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

可以远程连接 PostgreSQL，但 `/source` 的只读挂载只能读取备份容器所在主机的文件。仅在 `backup.environment` 填写应用服务器地址，无法读取另一台机器的 `data`/`logs`。跨服务器部署还需要可靠的共享存储或单独的 SSH/SFTP 文件采集流程；当前默认方案更适合放在应用宿主机，将最终加密数据存入异地 OSS。

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

2026-11-16 本地 Linux/amd64 验证：59 项单元测试、122 项集成检查通过。数据库及客户端为 PostgreSQL 15.20，对象存储为 MinIO 2024.6.13；Restic 0.18.1、rclone 1.71.2 均使用 Go 1.26.10。完整集成使用明确禁止 CreateBucket、允许桶内操作的账号，验证跳过建桶后正常备份，以及真实初始化已完成但响应丢失时的配置回读和后续备份。实际完成三轮双库恢复（含中文、数值、二进制数据和索引）、文件校验及清理后的再次恢复；错误凭据、空间不足和超时均未删除已有恢复点。两种计划均验证了真实容器启动备份、下一次执行时间、健康检查和等待期间正常停止。自动初始化还覆盖首次空前缀启动、重复初始化、节点更新重建、全新本地状态复用旧仓库，以及已使用仓库丢失、前缀非空、Bucket 缺失、关闭自动创建和恢复命令禁止初始化等保护；同一空仓库跨主机并发首次初始化不在支持范围内。

本地隔离的 PostgreSQL 与 S3 兼容对象存储可用于验证完整的备份、加密、恢复和失败处理流程。它不能代替阿里云 OSS 的真实验证。正式使用前，还应以独立测试 Bucket 验证实际地域 Endpoint、RAM 权限、上传/下载、锁释放、保留清理和恢复，不需要连接生产数据来完成这一步。

2026-11-17 统一 S3 版本验证：64 项单元测试通过；以下三种配置各通过 127 项真实集成检查（后端均为隔离 MinIO 2024.6.13，PostgreSQL 15.20）：

```bash
python scripts/backup/test_integration.py --image newapi-backup:s3-test --postgres-image postgres:15.20-bookworm --storage-provider minio
python scripts/backup/test_integration.py --image newapi-backup:s3-test --postgres-image postgres:15.20-bookworm --storage-provider s3-alibaba
python scripts/backup/test_integration.py --image newapi-backup:s3-test --postgres-image postgres:15.20-bookworm --storage-provider alibaba
```

覆盖统一 S3/MinIO、统一 S3/Alibaba 和旧 OSS 配置的初始化、备份、双数据库及文件恢复、留存清理、定时任务、重部署及失败保护；160 MiB 文件分别使用磁盘暂存和独立 256 MiB 内存盘备份、恢复后 SHA256 一致。另用真实 Restic 验证 quiet/FPS=0 保留唯一成功 summary 且不产生周期 status，父快照按主机和标签选择、删除文件不复活、更换源目录不混入旧文件。测试未连接生产或真实阿里云；三种模式的本地对象存储测试不等于真实 OSS 网络与权限验证。

2026-11-17 临时空间修复验证：60 项单元测试、124 项集成检查通过，仍使用上述客户端、PostgreSQL 15.20 和 MinIO 版本。完整 runner 在 512 MiB 内存和 64 MiB `/tmp` 下完成 160 MiB 不可压缩文件的备份与恢复，SHA256 一致。另在本地限速 REST 存储做容量对照：临时数据写 `/tmp` 时明确因空间耗尽退出 `1`，只改用 `/cache` 磁盘卷后成功。该验证证明旧默认配置存在容量缺陷，不能据此认定此前生产上传失败的具体原因。
