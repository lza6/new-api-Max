# SOP — 生产主机暴露面收口（SSH 密钥化 / 关公网端口 / ufw）

> **最后核对**：2026-10-24（生产 `154.37.208.171`，`freeapi.tingfengai.art`）
> **对应任务卡**：`T-E2`（改进指南轨道 E）
> **证据**：`evidence/T-E2.json`（含每条命令的真实输出）
> **状态**：**已执行并验证**。密码登录已关闭，**现在只能用密钥登录** —— 登录方式见 §1。

---

## 1. 现在怎么登录（**最重要的一段**）

```bash
ssh newapi-prod
# 等价于：
ssh -i ~/.ssh/newapi_prod_ed25519 root@154.37.208.171
```

- 私钥位置（本机 Windows）：`C:\Users\Administrator.DESKTOP-EGNE9ND\.ssh\newapi_prod_ed25519`
- 公钥指纹注释：`newapi-prod-20261024`（已写入服务器 `/root/.ssh/authorized_keys`）
- 别名已写入本机 `~/.ssh/config`（`Host newapi-prod`）
- **密码登录已被关闭**（实测 `Permission denied (publickey)`）

### 如果你换机 / 密钥丢了怎么办（应急）

1. 登录服务商控制台（安徽乌云数据，实例 `#server-1085` / `ser211682755579`）→ VNC / 救援模式；
2. 在 VNC 里执行回滚命令（见 §4.1），恢复密码登录；
3. 或者把本机的 `newapi_prod_ed25519`（私钥）拷到新机 `~/.ssh/` 即可继续用。

> ⚠️ **私钥不要外传、不要提交进 git**。它等于生产 root 权限。

---

## 2. 本轮改了什么（逐项 + 加固前 → 加固后）

| # | 项 | 加固前 | 加固后 | 依据 |
|---|---|---|---|---|
| 1 | SSH 公钥认证 | `pubkeyauthentication no` | **`yes`** | `sshd -T` 前后对比 |
| 2 | SSH 密码认证 | `passwordauthentication yes` | **`no`** | 实测密码登录 → `Permission denied (publickey)` |
| 3 | root 登录方式 | `PermitRootLogin yes`（密码可登） | **`prohibit-password`**（只允许密钥） | `sshd -T` → `permitrootlogin without-password` |
| 4 | 认证尝试次数 | `MaxAuthTries 6` | **`MaxAuthTries 3`** | `sshd -T` |
| 5 | 25 端口（postfix） | `inet_interfaces = all`，监听 `0.0.0.0:25` | **`loopback-only`**，只监听 `127.0.0.1:25` | `ss -lntp \| grep :25` |
| 6 | 主机防火墙 | `ufw` **inactive**（无防火墙） | **active**：default deny incoming，仅放行 22/80/443 | `ufw status verbose` |
| 7 | compose 端口（防将来误用） | `"3001:3000"`（会绑 0.0.0.0） | **`"127.0.0.1:3001:3000"`** | `grep -A3 ports: docker-compose.yml` |

### 2.1 审计结论的纠正（重要，避免下次又按错的结论施工）

| 10-23 审计的说法 | 2026-10-24 实测 | 结论 |
|---|---|---|
| 「3001 端口 0.0.0.0 暴露公网」 | 实跑容器绑定 `127.0.0.1:3000->3000`，外网探测 3001 = **ECONNREFUSED** | **该条已不成立**（早已修好）。compose 里残留的 `3001:3000` 只是"将来误用 compose up 会暴露"，本轮一并锁到 loopback |
| 「无 fail2ban」 | **已安装**，`sshd` jail 在跑：Total failed 2310 / Total banned 70 / Currently banned 2 | **该条已不成立** |
| 「无公钥、无防火墙」 | 公钥确实关闭、ufw 确实 inactive | **成立**，本轮已修 |
| 「24h 16867 次撞库」 | 累计 `Failed password` 54428 次；当日 758 次（fail2ban 已在压制） | 数字口径需按日统计，趋势仍在 |

**教训（已写入 `known-issues.md`）**：审计结论必须标注取证时间与取证对象（仓库 / 生产机），
否则"仓库里搜不到 pg_dump"会被误当成"生产没有备份"。本轮已按生产实测纠正 P0-1。

---

## 3. 验证记录（真实输出，摘自 `evidence/T-E2.json`）

```text
# 加固后 sshd 生效值
maxauthtries 3
permitrootlogin without-password
pubkeyauthentication yes
passwordauthentication no

# 密码登录被拒
root@154.37.208.171: Permission denied (publickey).

# 密钥登录
KEY LOGIN OK / root

# 对外监听（仅三项）
0.0.0.0:22   *:80   *:443

# 外网探测（本机 → 生产 IP）
port 22: OPEN    port 80: OPEN    port 443: OPEN
port 25: timeout port 3000: timeout port 3001: timeout

# 服务可用性
https=200  healthz=200  readyz=200
```

---

## 4. 回滚（出问题照抄）

### 4.1 恢复密码登录（VNC 控制台里执行）

```bash
sed -i 's/^PasswordAuthentication no/PasswordAuthentication yes/; s/^PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config
sshd -t && systemctl reload ssh
sshd -T | grep -E '^(passwordauthentication|permitrootlogin) '
```

### 4.2 恢复 postfix 监听

```bash
postconf -e 'inet_interfaces = all' && systemctl restart postfix
ss -lntp | grep :25
```

### 4.3 关闭防火墙（若发现某服务被误拦）

```bash
ufw disable && ufw status
# 需要时再精确放行后重新启用：
# ufw allow <port>/tcp && ufw --force enable
```

### 4.4 完全回退 sshd（配置原件）

```bash
ls /etc/ssh/sshd_config.bak-*        # 本轮的备份文件名带时间戳
cp -a /etc/ssh/sshd_config.bak-20261024-* /etc/ssh/sshd_config
sshd -t && systemctl reload ssh
```

### 4.5 回退 compose 端口

```bash
cp -a /opt/new-api/docker-compose.yml.bak-20261024-* /opt/new-api/docker-compose.yml
# 注意：实跑容器由 deploy-zero-downtime.sh 管理，改 compose 不会立即影响运行容器
```

### 4.6 回退 postfix 配置

```bash
cp -a /etc/postfix/main.cf.bak-20261024-* /etc/postfix/main.cf && systemctl restart postfix
```

---

## 5. 本轮用的安全手法（下次改 sshd 必须照做）

1. **两阶段**：先只开 `PubkeyAuthentication yes`（保留密码）→ 验证密钥能登 → 才关密码。
2. **死手开关**：改 sshd 前挂一个后台任务，N 分钟内没有取消就自动恢复原配置并 reload。
   本轮用过 300s（ufw）与 600s（sshd）两个开关，验证通过后 `touch` 取消文件。
   ```bash
   cat > /tmp/sshd_deadman.sh <<'EOS'
   #!/bin/bash
   sleep 600; [ -f /tmp/sshd_deadman_cancel ] && exit 0
   sed -i 's/^PasswordAuthentication no/PasswordAuthentication yes/' /etc/ssh/sshd_config
   sshd -t && systemctl reload ssh
   EOS
   chmod +x /tmp/sshd_deadman.sh; setsid nohup /tmp/sshd_deadman.sh >/dev/null 2>&1 &
   ```
3. **先 `sshd -t` 再 `reload`**（不是 restart），配置语法错也不会断连。
4. **备份原件**：`cp -a <file> <file>.bak-$(date +%Y%m%d-%H%M%S)`，回滚有据。
5. **改完立刻用新连接验证**，不要只看命令回显。

---

## 6. 仍然存在的风险（本轮未处理，需你决定）

| # | 风险 | 说明 | 建议 |
|---|---|---|---|
| 1 | **root 密码 `bvspEQw9Dm` 曾以明文出现在本机记忆文件与临时 askpass 中** | 现在密码认证已关闭 → 该密码**对 SSH 已无授权作用**。但它可能还用于服务商控制台/VNC | 若控制台也用同一密码，请改掉（改 `/etc/shadow` 会牵动控制台救援通道，本轮未擅自执行） |
| 2 | **Docker 发布端口会绕过 ufw** | 若将来有人 `docker run -p 3000:3000`，会绑 0.0.0.0 并绕过 ufw 默认 deny（走 `DOCKER-USER` 链） | 在 `DOCKER-USER` 链加显式拒绝规则（未加） |
| 3 | 无入侵检测 / 内核未升级 | `reboot-required` 自 10-06 挂着（内核 5.15.0-30），需停机窗口 | 计划维护窗口重启 |
| 4 | 私钥无口令 | 便于自动化，但本机被入侵即等于生产 root | 可加 `passphrase` + `ssh-agent` |
| 5 | 无异地自动同步备份 | 见 `SOP-备份与恢复.md` §7 | 对象存储或第二台机 `rsync` |
