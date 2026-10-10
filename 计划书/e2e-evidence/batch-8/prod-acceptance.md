# Batch-8 / G1 —— **生产环境**验收（freeapi.tingfengai.art，v1.3.124）

- 时间：2026-10-10
- 版本：部署前后 `X-New-Api-Version` 均为 `v1.3.124`
- 账号：`e2e_adm4`（既有 root 测试账号，id=977）；**未新建任何生产账号**

## 1. 部署（蓝绿零停机）

```
./deploy-zero-downtime.sh v1.3.124
[deploy] tag=v1.3.124 old=3001 new=3000
[deploy] 新容器 200
[deploy] 自检通过(200)，停旧容器并改名
[deploy] 完成：new-api:v1.3.124 在端口 3000
```

部署后实测：

| 检查 | 结果 |
|---|---|
| 容器 | `new-api:v1.3.124` **healthy** |
| 直连应用 `:3000` | `healthz=200` `readyz=200` `X-New-Api-Version: v1.3.124` |
| Caddy 上游 | `reverse_proxy 127.0.0.1:3000`（已随蓝绿切换） |
| 经 Caddy 走公网域名与真实 TLS | 首页 **200**、`/api/status` **200**、版本头 `v1.3.124` |
| 新接口鉴权边界（无凭据） | `/api/option/feature-switches` → **401** |
| 内存 | 64.6 MiB / 900 MiB |
| 重启次数 | **0** |

## 2. 真实浏览器验收（生产）

脚本：`g1-feature-switch-browser-e2e.cjs`（`E2E_PROD=1`）
方式：SSH 隧道到生产 Caddy 的 443（本地 443 → `127.0.0.1:443`），Chromium
`--host-resolver-rules=MAP freeapi.tingfengai.art 127.0.0.1`，**保持真实域名/SNI/Origin**。

**结果：20/20 PASS**（`prod-RESULTS.md`，截图 `prod-01-*.png` / `prod-02-*.png`）

关键断言：

- 真实 UI 表单登录 → 落到 dashboard（未被踢回 sign-in）
- 直达 `/system-settings/feature-switches/switches`：**13 个开关全部渲染**、
  效果度量区块、风险徽章、回滚提示、**中文 i18n 生效**
- `RELAY_AUDIT_ENABLED` 初始为关、重置按钮**不渲染**；打开后 → 重置按钮**出现**
  （该按钮由服务端 `configured=true` 决定）+ 度量出现数字
- 刷新后仍为开且显示「已在管理端配置」
- 「重置为默认」→ 回到「使用环境默认值」，开关回关

**最有分量的一条**：开关打开期间，线上真实流量让
`relay_audit_findings_total` 从 **4 → 11** —— 这不是 UI 假象，是**开关在生产链路里真的生效了**。

> ⚠️ 验收结束时已通过 UI 的「重置为默认」把开关复位（`configured=false`，回 env 默认），
> **生产行为与部署前完全一致**。

## 3. 压测（并发 30×5 与 60×4，经生产实例）

| 目标 | 并发30×5 | 并发60×4 | 5xx |
|---|---|---|---|
| 公开 `/api/status` | **150/150** 200，p50 252ms p95 955ms | **240/240** 200，p95 1089ms | 0 |
| **新增 `/api/option/feature-switches`** | **150/150** 200，p50 240ms p95 **293ms** | 208/240 200 + 32×429 | **0** |
| 对照 `/v1/pricing` | 60×200 + 90×429 | 240×429 | 0 |

- 新增接口是本次唯一「每次调用都查 DB 渠道表 + 遍历渠道聚合健康分/熔断状态」的重读路径，
  在并发 60 下 p95 仍 **307ms**、**零 5xx**。
- 429 是既有 `PublicReadRateLimit`（60/min/IP）在单 IP 高频下**正常工作**，不是缺陷。
- 压测后容器：**mem 126.1 MiB / 900 MiB、restarts 0、health healthy**；
  Caddy 近 3 分钟 **0 个 5xx**。

## 4. 边界与说明

- 压测全部来自**单一 IP 并经 SSH 隧道**，因此 429 出现得比真实多来源流量更早；
  这不改变「0 个 5xx」的结论，但也**不能**据此推断多来源下的绝对吞吐。
- 生产验收用 `--host-resolver-rules` 把真实域名指向本地隧道；TLS 用的是生产真实证书
  （CN=freeapi.tingfengai.art）。**未经公网链路**，公网可达性另由部署期的
  `curl --resolve … https://freeapi.tingfengai.art` 单独验证（200 + 版本头）。
