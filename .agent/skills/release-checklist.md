# Skill — 发版与热更新（new-api 主站）

> 适用：任何要把代码送上生产的一批改动。生产机 `154.37.208.171`（root，askpass 走密码）。

## 版本纪律（三处必须一致）
`VERSION` / `CHANGELOG.md` 首条 / git tag。另加 `workflow_status.md §0` 唯一真相表。
校验：`node scripts/check-version-consistency.mjs`。
**并发写同一仓库时先 `git log` 确认版本号归属**，让号比重号便宜（本会话就让了 v1.3.183/184 给并行工作流）。

## 步骤
1. 本地验证（按改动面选最小充分集）：
   `cd web && bun run typecheck`、`bunx oxlint`（改动文件）、定向 `vitest`、`bun run build`；
   门禁：`node scripts/check-missing-i18n.cjs`、`node scripts/check-terminology.mjs`。
2. 提交（原子、中文正文写清"为什么"与验证命令），推送 `main`（禁 force）。
3. 打 tag：`git tag -a vX.Y.Z -m "…"`，推送 tag。
4. **构建（不依赖 push 成功）**：
   ```bash
   git archive --format=tar HEAD | gzip -6 > /tmp/newapi-vX.Y.Z.tar.gz
   scp /tmp/newapi-vX.Y.Z.tar.gz root@154.37.208.171:/opt/new-api-src-vX.Y.Z.tar.gz
   ssh root@… 'rm -rf /opt/new-api-src-vX.Y.Z && mkdir -p … && tar -xzf … -C … && cd … && (nohup docker build -t new-api:vX.Y.Z . > /tmp/build-vX.Y.Z.log 2>&1 &)'
   ```
   约 8–13 分钟；轮询 `docker images | grep vX.Y.Z`。
5. 部署：`ssh root@… 'cd /opt/new-api && ./deploy-zero-downtime.sh vX.Y.Z'`（蓝绿 3000↔3001，自带自检与回滚）。
6. 验收（服务器侧，**本机直连域名 TLS 会被 RST**）：
   ```bash
   curl -sI --resolve freeapi.tingfengai.art:443:127.0.0.1 https://freeapi.tingfengai.art | grep -i x-new-api-version
   curl -s --resolve … /healthz ; curl -s --resolve … /readyz ; curl -s --resolve … /api/status -o /dev/null -w '%{http_code}'
   docker logs --tail 300 new-api | grep -c ERROR ; docker inspect -f '{{.RestartCount}} restarts' new-api
   ```
7. 回滚：`./deploy-zero-downtime.sh <上一版>`（镜像保留最近 4 个）；把回滚点写进 `workflow_status.md §0`。

## 常见坑
1. `ssh 'nohup docker build &'` 可能让 ssh 挂住 → 用 `(nohup … &)` 包一层，或先 `setsid`。
2. `docker build | tail` 会吞掉错误输出 → 必须重定向到服务端日志文件再读。
3. GitHub HTTPS 会整段断（TLS eof）→ 部署不必等 push；先构建部署，push 后补。
4. 生产 CSS 经 lightningcss 降级，**grep 不到 `oklch(`** → 用令牌名（`transition-quiet`/`surface-grid`）+ axe 做证据。
5. 部署清理会删最旧镜像，回滚点只剩约 4 个版本窗口。
