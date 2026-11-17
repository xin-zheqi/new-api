# 按指定快照快速恢复 New API

可以按快照 ID 恢复某个备份。文件恢复与数据库导入是两步：`restore-files` 先下载并校验选定快照，再把其中的 PostgreSQL 导出导入一个全新数据库。下面的流程不覆盖正在运行的数据库或 `data`，也不自动切换线上流量。

被攻击后应在可信的干净机器上恢复。前提是 MinIO、阿里云 OSS 等对象存储中仍保留所需快照，并且你独立保存了正确的 Restic 加密密码；备份账号如果也被盗且快照已被删除，仅靠恢复工具不能找回它们。应提前演练一次，并保留仓库地址、`BACKUP_HOST`/`BACKUP_TAG` 和密码副本。

## 1. 准备独立恢复目录

在有 Docker Compose 的 Linux 机器上新建专用目录，把本目录的 `recovery.compose.yml` 复制进去。它只启动恢复工具，不要求原来的应用、数据库、`data` 或 `logs` 目录存在。

恢复模板使用 `thefoolishfox1/new-api-backup:20261117-s3-backup`，默认以统一 S3 配置连接 MinIO。自动初始化仅适用于备份任务；这里的 `snapshots`、`check`、`restore-files` 不会创建仓库。新机器只需正确连接已有仓库，不需要先执行 `init`。找不到仓库、密码错误或本地身份记录不匹配时，应核对原地址、权限、密码及仓库迁移情况，不要通过创建空仓库来继续恢复。

```bash
mkdir -p newapi-recovery
cd newapi-recovery
# 将 recovery.compose.yml 复制到这里，然后填写其中的对象存储参数及原 HOST/TAG。
mkdir -p secrets/backup restore-output
chmod 700 secrets secrets/backup restore-output
```

把独立保存的备份加密密码写入 `secrets/backup/restic-password`，文件只含密码正文，权限设为 `600`。不要在命令参数、终端历史里直接填写真实密码。配置中的 AccessKey 可以换成新的恢复账号，但必须有读取仓库及处理仓库锁所需的权限。YAML 中的密码若含 `$`，写成 `$$`。

恢复模板在同一个文件顶部保留 MinIO 和阿里云两套 `x-backup-storage-*` 配置，不需要 `.env` 或额外 YAML 文件。先填写对应存储块，再在 `restore.environment` 将 `<<: *backup-storage-minio` 或 `<<: *backup-storage-aliyun` 选为唯一引用。`BACKUP_REPOSITORY` 必须指向**原有仓库或完整副本**：

| 字段 | MinIO | 阿里云 OSS |
| --- | --- | --- |
| `BACKUP_REPOSITORY` | `rclone:s3:原桶/原前缀` | `rclone:s3:原桶/原前缀` |
| `BACKUP_S3_PROVIDER` | `Minio` | `Alibaba` |
| `BACKUP_S3_ENDPOINT` | 实际 S3 API 地址，如 `https://minio.example.com` | 桶地域地址，如 `oss-cn-shenzhen.aliyuncs.com` |
| `BACKUP_S3_REGION` | 按服务配置，默认 `us-east-1` | 示例 `cn-shenzhen` |
| `BACKUP_S3_FORCE_PATH_STYLE` | `true` | `false` |
| `BACKUP_S3_ACCESS_KEY_ID` / `BACKUP_S3_ACCESS_KEY_SECRET` | 该仓库恢复账号 | 该仓库恢复账号 |

MinIO 通常使用 API 端口 `9000`，不要填写控制台 `9001`。HTTP 地址需显式设置 `BACKUP_ALLOW_INSECURE_S3: "true"`，仅用于已确认可信的网络；HTTPS 保持证书校验。旧 `rclone:oss:` 仓库仍可搭配原 `BACKUP_OSS_*` 使用，无需更改仓库中的任何对象。

不要在 `restore.environment` 重复填写存储字段，以免覆盖顶部选中的配置。从 OSS 切换到空的 MinIO 不会带来旧快照。恢复旧数据必须连接原仓库或已完整复制、保留原密码的仓库，不能靠 `init`、删除状态卷或强制解锁使空仓库拥有历史备份。

```bash
chmod 600 secrets/backup/restic-password recovery.compose.yml
docker compose -f recovery.compose.yml config -q
docker compose -f recovery.compose.yml pull restore
```

磁盘需要同时容纳恢复的文件、数据库导出和新 PostgreSQL 数据卷；恢复目录与数据库若在同一磁盘，要合计预留空间。

## 2. 选择并恢复一个快照

```bash
# 列出快照 ID 和时间；选择事件发生之前的明确 ID。
docker compose -f recovery.compose.yml run --rm --no-deps restore snapshots

# 将 REPLACE_WITH_SNAPSHOT_ID 换成选定快照的完整 ID 或至少 8 位十六进制前缀。
docker compose -f recovery.compose.yml run --rm --no-deps restore \
  restore-files REPLACE_WITH_SNAPSHOT_ID /restore/incident-001
```

`incident-001` 必须尚不存在。工具会执行 `restic restore --verify`，并验证快照属于配置的原 HOST/TAG；它不会把恢复内容覆盖到业务目录，也不会导入数据库。失败会返回非零状态；不要把部分恢复目录当作完整备份，处理原因后换一个新的目标子目录重试。

默认备份布局恢复后为：

```text
restore-output/incident-001/
  source/data/                    应用文件
  source/logs/                    程序日志
  source/config/                  备份时的 Compose、配置文件
  state/job-随机目录/
    manifest.json                 数据库名称、导出文件名、校验和
    database-000.dump
    ...
```

如果原任务改过 `BACKUP_STATE_DIR` 或 `BACKUP_SOURCE_DIR`，恢复的目录层级也会相应变化。配置文件可能含密钥，先限制目录权限、人工审阅；不要直接执行恢复出来的 Compose 或脚本。

在宿主机用 Python 3 校验全部导出并打印“库名 → 导出路径”，不会输出对话或密钥：

```bash
python3 - <<'PY'
import hashlib
import json
from pathlib import Path

root = Path("restore-output/incident-001").resolve()
manifests = list((root / "state").glob("job-*/manifest.json"))
if len(manifests) != 1:
    raise SystemExit("未找到唯一导出清单；如修改过 BACKUP_STATE_DIR，请先核对恢复目录布局")
manifest = manifests[0]
data = json.loads(manifest.read_text(encoding="utf-8"))
if data.get("format") != 1 or not data.get("databases"):
    raise SystemExit("导出清单格式不正确")
for item in data["databases"]:
    dump = (manifest.parent / item["file"]).resolve()
    if dump.parent != manifest.parent.resolve() or not dump.is_file():
        raise SystemExit("导出文件路径不正确")
    digest = hashlib.sha256()
    with dump.open("rb") as stream:
        while chunk := stream.read(1024 * 1024):
            digest.update(chunk)
    if digest.hexdigest() != item["sha256"]:
        raise SystemExit("导出文件校验失败")
    print(item["name"], "->", dump)
print("全部数据库导出校验通过")
PY
```

## 3. 导入全新的 PostgreSQL

以下 Bash 示例使用原项目兼容的 PostgreSQL 15。**容器名和卷名必须是本次新名称**，不能使用生产数据库的名称或数据卷。每条命令成功后再继续；不要在失败后直接执行下一步。

```bash
PG_CONTAINER=newapi-incident-20261116-01-postgres
PG_VOLUME=newapi-incident-20261116-01-pgdata

```

下面先拒绝已有的同名容器或卷，再创建新卷和新数据库；若名称冲突，先换一个新的事件编号。交互输入一个新的恢复库密码，并另外妥善保存；`-e POSTGRES_PASSWORD` 从当前进程环境传入，不把密码放进命令参数。括号内的步骤任一步失败就停止。

```bash
(
  set -eu
  if docker container inspect "$PG_CONTAINER" >/dev/null 2>&1 || \
     docker volume inspect "$PG_VOLUME" >/dev/null 2>&1; then
    printf '%s\n' '已有同名容器或卷，请换一个事件编号后重试。' >&2
    exit 1
  fi
  read -r -s -p "请输入新的恢复数据库密码: " POSTGRES_PASSWORD
  printf '\n'
  test -n "$POSTGRES_PASSWORD"
  export POSTGRES_PASSWORD
  docker volume create --label new-api.recovery=true "$PG_VOLUME"
  docker run -d --name "$PG_CONTAINER" --network none --label new-api.recovery=true \
    --mount "type=volume,src=$PG_VOLUME,dst=/var/lib/postgresql/data" \
    -e POSTGRES_USER=root -e POSTGRES_DB=new-api -e POSTGRES_PASSWORD \
    postgres:15.20-bookworm
)

# 等待 PostgreSQL 的 TCP 就绪检测成功再导入；失败时稍后重试这条检测。
docker exec "$PG_CONTAINER" pg_isready -h 127.0.0.1 -U root -d new-api
```

将 `APP_DUMP` 换成上一步输出中 `new-api` 对应的完整路径：

```bash
APP_DUMP='/absolute/path/to/database-000.dump'
docker cp "$APP_DUMP" "$PG_CONTAINER:/tmp/new-api.dump"
docker exec "$PG_CONTAINER" pg_restore --no-password --exit-on-error \
  --no-owner --no-acl --username=root --dbname=new-api /tmp/new-api.dump

docker exec "$PG_CONTAINER" psql -X -U root -d new-api -v ON_ERROR_STOP=1 \
  -c 'SELECT count(*) AS users FROM users;' \
  -c 'SELECT count(*) AS channels FROM channels;'
```

`pg_restore` 成功后检查关键记录、用户额度和渠道配置；表数量正常不等于业务全部正常。导出包含独立日志库等多个数据库时，按 manifest 为每个库新建空库，并分别导入对应文件，不能遗漏。

导出不含集群级角色、表空间；上面的 `--no-owner --no-acl` 使用新账号恢复对象，因此自定义角色、授权和原来使用的扩展也要按部署要求恢复。若导入中途失败，换新的空库/卷重试，不要对部分导入的库反复叠加导入。

## 4. 准备应用文件并人工切换

在恢复目录中创建一个全新的应用目录，保留所选快照中的文件：

```bash
# mkdir 不加 -p：同名目录已存在时先换名字，避免混入旧数据。
(
  set -eu
  mkdir recovered-app-001
  cp -a restore-output/incident-001/source/data recovered-app-001/data
  cp -a restore-output/incident-001/source/logs recovered-app-001/logs
)
```

在新应用目录重新审阅并填写可信的部署配置，指向新恢复库和新 `data`/`logs`。沿用原 `CRYPTO_SECRET` 才能解密存量渠道等数据；不能随意生成一个新值替代。恢复启动时保留原应用密钥及必要配置，后续轮换应使用应用支持的流程；若需使旧登录失效，再有计划地轮换会话密钥。

优先使用与所选备份对应、已经验证的 New API 镜像版本或 digest。若原部署使用 `latest`，需先核对对应版本；备份时最好在部署配置中固定镜像版本，避免恢复同时变成一次未经验证的应用升级。

当前示例恢复库用 `--network none` 隔离，没有对外端口。实际切换时再接入可信的应用网络，使用新的数据库主机名和密码配置 SQL_DSN；不要与旧库使用同一个网络别名。先验证登录、附件读取、渠道和少量真实请求，再切换代理流量。保留原数据，直到恢复确认完成。

恢复会将数据库回退至所选导出的时间点。快照之后的请求、充值和额度变更不会自动补回，恢复接单前应对账。在线 PostgreSQL 与文件采集也不是同一原子快照；详细一致性边界和恢复演练要求见上一级 `README.md`。
