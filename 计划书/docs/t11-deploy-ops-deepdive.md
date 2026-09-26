# 专项分析 · 部署/运维/CI 深化（T11 纵深）

> 定位：主指南 §T11 的**深挖文档**：部署链路现状、缺口、落地与回滚；只读整理。
> 生成：2026-09-25 · 锚点：`计划书/ops/deployment-sop.md`、根 `scripts/`、docker-compose.yml、Dockerfile。

## 1. 现状（证据）
- 部署目标：线上 `freeapi.tingfengai.art`（记忆链 watchtower 已修：`--restart always` + 300s 轮询 + label enable；v1.3.13 后线上复核**待验证**）。
- GitHub fork push 触发不可靠 → 须 `workflow_dispatch`（记忆证据）。
- blue-green 零停机滚动更新脚本已入库（a0d0589f5，rolling-update-newapi-v3.sh）。
- `计划书/ops/deployment-sop.md` 已更新（v1.3.28 + blue-green 衔接）。

## 2. 缺口（主指南 T11 待办）
1. SOP 与 rolling-update 脚本衔接再核对（脚本入仓库根或 scripts/ 的路径/引用）。
2. Dockerfile 瘦身 + HEALTHCHECK（多阶段构建、最终镜像去源码/测试、`/api/status` healthcheck）。
3. CI 产物校验：构建后校验 `/api/status` 版本串 + `X-New-Api-Version` 头。
4. 备份/恢复演练：DB + compose + 计划书证据目录；干跑恢复。

## 3. 落地路径（建议）
1. **SOP**：build → tag → workflow_dispatch → 服务器 `docker compose pull new-api && docker compose up -d new-api` → `/api/status` + healthcheck → 验收 → 回滚 `.bak-pre-vXXX`（现有 SOP 骨架沿用）。
2. **Dockerfile**：多阶段（builder → runtime，非 root 用户）；`HEALTHCHECK CMD` 命中 `/api/status`；镜像体积对比记录。
3. **CI 校验**：workflow 中 `curl /api/status | jq .version` 断言 tag 版本；`X-New-Api-Version` 头断言。
4. **备份演练**：`计划书/ops/deployment-sop.md` 增「备份清单 + 恢复干跑」章节；产出恢复日志/证据。

## 4. 纪律
- 推送：fork 默认 main；禁 `-f`；按主题 commit；先 fetch 后推；PR 用对应模板（AGENTS）。
- 线上操作一律需用户授权；本机无法验证处标「待线上验证」。
- 不回滚业务代码的破坏性操作；部署失败回滚到 `.bak-pre-vXXX` 镜像标签。

## 5. 验证命令（授权后）
- `docker build -t new-api:test .`（体积对比）；服务器 `curl -s https://freeapi.tingfengai.art/api/status | grep version`。
- `docker compose pull new-api && docker compose up -d new-api`；`docker inspect --format '{{.State.Health.Status}}' <ctr>`。

## 闭环状态（2026-09-27，v1.3.44 回填）
- ✅ 503 事故复盘入库：计划书/ops/503-incident-response.md（§4.1 封禁误伤 + §6 Caddyfile 损坏，含定位命令与防复发清单）。
- ✅ Dockerfile HEALTHCHECK（/api/status success:true，30s/5s/15s/3）。
- ✅ release.yml CI 产物校验（grep -a VERSION in binary）。
- ✅ 生产 v1.3.43 部署 + 双后端 + Caddy 加固（3/2/2s）实测。
