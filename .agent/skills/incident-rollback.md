# Skill：生产事故响应与回滚

## 适用场景
- 线上 5xx 突增、全站不可用、容器反复重启
- 需要判断「是服务器挂了 / 是我的改动 / 是上游问题」
- 需要在不打爆生产的前提下定位与回滚

## 前置读取
1. `workflow_status.md` §0（唯一真相源：线上版本、部署时间、Caddy 上游端口、回滚镜像）
2. `freeapi-production-deploy.md`（服务器、compose、Caddy 路径）
3. `local-curl-schannel-broken.md` + `local-browser-proxy-bypass-art-tld.md`
   —— **本机直连自有域名会被 RST，绝不能用本机 curl 判断线上死活**
4. `test_ledger.md` 已知噪声表

## 处置顺序（先定性，再动手）

### 第 1 步：先分清「谁挂了」（≤2 分钟）
```bash
# 服务器本机自检（不是本机 curl！）
ssh root@HOST 'curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:3000/api/status'
ssh root@HOST 'ss -tlnp | grep -E ":300[0-9]|:443"'
ssh root@HOST 'docker ps -a --format "{{.Names}}\t{{.Status}}"'
ssh root@HOST 'free -m; df -h / | tail -1; uptime'
```
- 本机 200 但外网不通 → 链路/DNS/CDN，不是应用
- 端口不通 → 容器挂了或 Caddy 摘除
- 内存/磁盘满 → 资源问题

### 第 2 步：按分钟聚合 5xx，锁定时间窗
```bash
docker logs new-api --since 45m 2>&1 | grep -E "\| (500|502|503|504) \|" \
  | grep -oE "[0-9]{2}:[0-9]{2}" | sort | uniq -c
```
**关键**：先确认故障时间窗是否与自己的操作时段重合，再判断因果。
2026-10-23 的教训：147 个 503 全部落在 OOM 崩溃恢复的 2 分钟内，与后续的部署重启无关。

### 第 3 步：区分 502 / 503 / 500 的语义
| 码 | 含义 | 第一嫌疑 |
|---|---|---|
| 502 | 上游中断 | 渠道上游自己断流（`stream ID 503; INTERNAL_ERROR` 之类）——查 relay 日志的 `scanner error` |
| 503 | 本网关不可用 | Caddy 判定无上游（`no upstreams available`）→ 看健康检查与容器 |
| 500 | 网关内部错误 | 代码路径 / DB / Redis |

**注意**：`/api/status` 200 但 `/v1/*` 500 → 探针不会摘除，故障会被放大。这类要优先修（见 `incident` 记忆：限流 fail-closed）。

### 第 4 步：回滚
```bash
cd /opt/new-api && ./deploy-zero-downtime.sh v<PREV>
```
- 脚本自带：新容器起来 → 健康轮询 → 切 Caddy → 失败自动回滚 Caddy 上游
- 保留最近 4 个镜像（`docker images new-api`）
- 数据库只前进不回退：新增表/列保留即可（旧代码不认识会忽略）

## 验收标准
- 外网连续 20 次 200
- 近 10 分钟 5xx = 0
- 版本头符合预期（`curl -D - -o /dev/null https://.../api/status | grep -i x-new-api-version`）
- 容器 `healthy`
- 关键接口真实耗时记录在案

## 证据要求
- 故障窗口的按分钟 5xx 统计（原始命令 + 输出）
- 元凶日志原文（含 `Failed process was running` 之类的决定性证据）
- 回滚前后的版本头、外网状态码
- 若做了配置变更：`.bak-*` 文件路径

## 常见坑
1. **本机 curl 直连自有域名会 000/RST**，别据此判断线上挂了
2. `docker rm -f` 是 SIGKILL，会掐断在途流式请求（24h 内 53 个 502 由此而来）；停旧容器用 `docker stop -t 30`
3. fail2ban 装上后，**askpass 临时文件被 `/tmp` 清理会导致连续认证失败，把自己的 IP 封掉**（2026-10-23 真实踩过）。装完立刻 `maxretry=10` + `ignoreip 127.0.0.1/8 ::1`
4. `nohup ... &` / `setsid ... &` 经 ssh 仍会让本地 ssh 挂住（stdout 未脱）→ 用独立短命令轮询日志，别等长连接
5. `/tmp` 下的 askpass 会随会话被清，**每次 ssh 前重建**
