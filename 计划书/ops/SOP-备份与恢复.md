# SOP — 数据库备份与恢复演练（PostgreSQL）

> **最后核对**：2026-10-24（基线 `VERSION=v1.3.195`，生产 `v1.3.192`）
> **对应任务卡**：`T-E1`（改进指南 §11.2 轨道 E）
> **脚本**：`计划书/ops/backup-postgres.example.sh`
> **状态**：**脚本已写、语法自检通过（`bash -n` = OK）；生产上从未执行过**（本轮无 SSH 凭据）。
> 按本文做完第 3 节才算 `Done`——**"有备份文件"不等于"能恢复"**。

---

## 0. 为什么现在必须做

| 事实 | 证据 |
|---|---|
| 生产**没有任何自动备份** | 2026-10-24 全仓检索：`rg 'pg_dump|数据库备份|backup' 计划书/ops scripts deploy` → **0 命中** |
| 也**没有 WAL 归档**，即无法做时间点恢复（PITR） | 同上；`docker-compose.yml` 无 `archive_mode` 相关配置 |
| 已经历过一次 PG 崩溃恢复 | 2026-10-23：`logs` 表 30 天全窗口聚合打爆 256MB 容器 → `oom_kill` → 崩溃恢复后统计信息被清空 → 全站 147 个 503 |
| 单点结构 | 单宿主 + 单 new-api + 单 PG + 单 Redis，Caddy 只有单一上游（`evidence/audit-20261023-ops.json`） |

**结论**：这不是"优化项"，是"数据丢了就找不回来"的项。备份的价值 = 恢复演练通过的那一刻起才算存在。

---

## 1. 备份内容与边界（先说清楚"备份了不等于都能回来"）

| 对象 | 在库里？ | 备份能否覆盖 | 说明 |
|---|---|---|---|
| 用户、额度、令牌、渠道、订阅 | 是 | ✅ | `pg_dump` 全库 |
| 调用日志 `logs` | 是（主库；除非配了 `LOG_SQL_DSN`） | ✅ | 量大，备份体积主要来自它 |
| `log_daily_stats` 日汇总 | 是 | ✅ | v1.3.189 新增 |
| `.env` 里的密钥（`SESSION_SECRET`、`CRYPTO_SECRET` 等） | 否 | ❌ **必须单独备份** | 见 §2.2 |
| 渠道 Key 明文（`CHANNEL_KEY_ENCRYPTION=false` 时） | 是（明文） | ✅（但备份文件本身即敏感） | 备份文件必须 `0600` |
| 云盘/外部存储里的训练数据产物 | 否 | ❌ | 不在本 SOP 范围；由 `SOP-排障-训练数据采集.md` 管 |
| Caddy 证书、`Caddyfile` | 否 | ❌ | 建议一并纳入 §2.2 |

---

## 2. 首次落地（服务器上执行，约 15 分钟）

### 2.1 安装备份脚本与定时任务

```bash
# 1) 上传/落地脚本（来源：计划书/ops/backup-postgres.example.sh）
install -m 0700 /path/to/backup-postgres.example.sh /opt/new-api/backup-postgres.sh

# 2) 手动跑一次（必需，先验证再上 cron）
/opt/new-api/backup-postgres.sh; echo "exit=$?"
ls -lh /opt/backup/

# 3) 装 cron（每日 03:30）
cat > /etc/cron.d/new-api-backup <<'EOF'
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
30 3 * * * root /opt/new-api/backup-postgres.sh >> /var/log/new-api-backup.log 2>&1
EOF
chmod 644 /etc/cron.d/new-api-backup

# 4) 确认 cron 生效（次日看日志）
tail -20 /var/log/new-api-backup.log     # 期望末尾是 "OK"
```

**预期输出**（正常时的关键行）：

```
[backup 2026-10-25 03:30:01] 剩余空间 41xxx MB，开始备份 new-api
[backup ...] 备份完成 /opt/backup/newapi-db-2026-10-25_030001.dump（xxxMB）
[backup ...] 结构校验通过：可解析 NN 张表的数据段
[backup ...] 轮转完成：删除 0 个超过 7 天的备份
[backup ...] OK
```

**参数可覆盖**（见脚本头部）：`PG_CONTAINER` / `PG_USER` / `PG_DB` / `BACKUP_DIR` / `KEEP_DAYS`（默认 7）/ `MIN_FREE_MB`（默认 2048，剩余低于它就跳过并报错）。

> ⚠️ **容器名必须先核对**：`docker ps --format '{{.Names}}'`。脚本默认 `postgres`，与本仓库
> `docker-compose.yml` 一致；生产若是 `new-api-postgres-1` 之类，用 `PG_CONTAINER=` 覆盖。

### 2.2 备份"不在库里"的那部分（密钥与配置）

```bash
install -d -m 0700 /opt/backup/config
cp /opt/new-api/.env                /opt/backup/config/env-$(date +%F)          && chmod 600 /opt/backup/config/env-*
cp /etc/caddy/Caddyfile             /opt/backup/config/Caddyfile-$(date +%F)    && chmod 600 /opt/backup/config/Caddyfile-*
# 这两者极小，纳入同一个 7 天轮转即可
```

**为什么要单独做**：`.env` 里的 `SESSION_SECRET`/`CRYPTO_SECRET` 丢了，数据库恢复了用户也全登不上/解不开密文。

### 2.3 确定恢复路径（先想清楚，别等出事再想）

| 场景 | 恢复动作 |
|---|---|
| 单表被误删/误改 | `pg_restore -t <table> -d <db> backup.dump`（表级恢复，不动全库） |
| 整库损坏 | 见 §3.2 全量恢复 |
| 只想找回某天的某几条数据 | 把备份恢复到一个**临时库**，查询后手工回灌（不要直接覆盖生产） |

---

## 3. 恢复演练（**这一节做完才算 T-E1 完成**）

> 演练原则：**永远先恢复到临时库/临时容器**，确认能起来、行数对得上，再谈生产恢复。
> 演练频率：首次 1 次，之后每季度 1 次（写进本 SOP 的执行记录表）。

### 3.1 结构校验（轻量，可每日跑）

```bash
docker cp /opt/backup/newapi-db-2026-10-25_030001.dump postgres:/tmp/v.dump
docker exec postgres pg_restore --list /tmp/v.dump | grep -c 'TABLE DATA'   # 期望 > 10
docker exec postgres rm -f /tmp/v.dump
```

### 3.2 全量恢复演练（临时库，不碰生产）

```bash
BK=/opt/backup/newapi-db-2026-10-25_030001.dump

# 1) 建临时库
docker exec -e PGPASSWORD=<pg密码> postgres createdb -U root restore_drill

# 2) 灌进去（--no-owner 避免角色不存在报错）
docker cp "$BK" postgres:/tmp/drill.dump
docker exec -e PGPASSWORD=<pg密码> postgres \
  pg_restore -U root -d restore_drill --no-owner --no-privileges /tmp/drill.dump

# 3) 核对（三个数必须与生产的同一时刻同量级）
docker exec -e PGPASSWORD=<pg密码> postgres psql -U root -d restore_drill -c "
  select 'users' t, count(*) from users
  union all select 'tokens', count(*) from tokens
  union all select 'logs', count(*) from logs
  union all select 'log_daily_stats', count(*) from log_daily_stats;"

# 4) 生产侧对照（同一 SQL，在 new-api 库里跑一遍，比较差值是否在"备份时间差 × 增长率"内）
#    注意：写操作仍在进行，logs 一定比备份多一点，这是正常的；users/tokens 应当完全一致或极接近

# 5) 清理
docker exec postgres rm -f /tmp/drill.dump
docker exec -e PGPASSWORD=<pg密码> postgres dropdb -U root restore_drill
```

**通过标准（缺一不算通过）**：

| 判据 | 标准 |
|---|---|
| 恢复过程 | `pg_restore` 退出码 0，无 "errors ignored on restore" 之外的致命错误 |
| 表数量 | 与 `db_structure.md` 记录的表数一致（当前 **55**） |
| `users` / `tokens` | 与生产**完全一致**（演示期间若有注册则差 ≤ 演练期间新增数） |
| `logs` | 与生产同量级（差值 = 备份时间点到现在的增量） |
| `log_daily_stats` | 行数 > 0（证明 v1.3.189 的表也在备份里） |

### 3.3 真出事时的生产恢复（仅在负责人明确指令下）

```bash
# 1) 先停写：把 Caddy 上游指向维护页，或 docker stop new-api
# 2) 再备份一次"当前状态"（哪怕已损坏，也不要直接覆盖，保留取证）
docker exec postgres pg_dump -U root -d new-api -Fc -f /tmp/pre-restore-$(date +%s).dump
# 3) 重建库并恢复（--clean 会 drop 现有对象，所以第 2 步不可省）
docker exec -e PGPASSWORD=<pg密码> postgres pg_restore -U root -d new-api --clean --if-exists --no-owner /tmp/drill.dump
# 4) 起服务 → /readyz 200 → /api/status 200 → 抽查登录与一次 relay 调用
# 5) 记录：恢复用的备份文件名、时间点、丢失的数据时间窗
```

**回滚**：第 2 步的 `pre-restore-*.dump` 就是回滚点（回到"恢复前那个损坏状态"，用于二次取证或换一个备份重试）。

---

## 4. 已知边界（不要高估这套备份）

1. **不是 PITR**：只能恢复到"最后一次备份时刻"，两次备份之间的数据会丢（最长约 24 小时）。要 RPO≈0 需开 WAL 归档（`archive_mode=on` + `archive_command`），成本与运维复杂度都更高，**属于下一步决策（见 §5）**。
2. **备份与数据库在同一台机器**：磁盘坏/整机被删 → 备份一起没。**必须做异地副本**（对象存储 / 另一台机 `rsync`）。
3. **`pg_dump` 会占 IO**：186 万行的 `logs` 表当前保留 7 天，dumps 体积应可控；若将来放开保留天数，注意备份窗口的 IO 影响。
4. **备份文件本身是敏感数据**（含用户、明文渠道 Key）→ 目录 `0700`、文件 `0600`、异地副本也必须加密。

---

## 5. 待决策（需要负责人拍板，不阻塞 §2/§3）

| # | 决策 | 选项 |
|---|---|---|
| 1 | 异地副本怎么做 | ①同机备份 + 每日 `rsync` 到另一台 ②直接传对象存储（S3/R2/B2，需密钥）③暂不做（明确接受"整机挂=全丢"） |
| 2 | 保留天数 | 当前 7 天；可提到 14/30（磁盘代价线性增长） |
| 3 | 是否要 PITR（WAL 归档） | 要则 RPO 从 ~24h 降到分钟级，代价是配置 + 存储 + 恢复复杂度 |
| 4 | 备份加密 | 是否用 `age`/`gpg` 加密落盘（异地副本场景**建议加密**） |

---

## 6. 执行记录（做完就填，不要把计划当完成）

| 日期 | 动作 | 结果 | 证据 |
|---|---|---|---|
| 2026-10-24 | 写脚本 + `bash -n` 语法校验 | OK | 本轮命令输出（脚本未在服务器执行） |
| — | 首次手动备份 | 待执行 | — |
| — | cron 安装 | 待执行 | — |
| — | 恢复演练（§3.2） | **待执行（T-E1 的完成判据）** | — |
