# 「网站打不开 / ERR_CONNECTION_RESET」排查 SOP

> 定性：**2026-10-04**。结论：`ERR_CONNECTION_RESET` 与本网关/服务器**基本无关**，
> 根因是「中国大陆直连香港源站 IP:443 被 RST」的跨境链路问题。**先按本 SOP 三步排除，别改服务器。**

## 一句话结论

域名 `freeapi.tingfengai.art` / `japi.tingfengai.art` 的 DNS 解析到**源站公网 IP 直连**
（`tingfengai.art` 的 NS 至今是 **dnspod 腾讯**，不是 Cloudflare —— 当年计划的橙云代理从未落地）。
大陆用户未挂代理时直连香港 IP:443，TLS 握手被 RST → 浏览器报 `ERR_CONNECTION_RESET`。
**挂稳定代理即正常，所以是「有时候」——取决于是否走代理 / 代理节点是否稳。**

## 排查三步（按序，别跳）

```bash
# 1) 服务器侧是否正常 —— 从服务器本机 curl 公网域名（服务器在香港，不受大陆链路影响）
ssh root@103.233.252.213 'curl -s -o /dev/null -w "%{http_code}\n" https://freeapi.tingfengai.art/api/status'
#    200 = 服务器/Caddy/网关全正常 → 问题在「用户→服务器」链路，跳到第 2 步
#    非 200 = 才是真服务器故障，走 503-incident-response.md

# 2) 对比「走代理 vs 直连」——定位是否跨境 RST
curl -s -o /dev/null -w "proxy=%{http_code}\n" https://freeapi.tingfengai.art/api/status          # 走代理
curl -s -o /dev/null -w "direct=%{http_code}\n" --noproxy '*' --ssl-no-revoke https://freeapi.tingfengai.art/api/status  # 直连
curl -s -o /dev/null -w "plain=%{http_code}\n"  --noproxy '*' http://103.233.252.213:3000/api/status  # 明文直连
#    直连挂 + 明文直连通 + 走代理通 = 典型「境外 IP:443 TLS 被 RST」，确认跨境链路问题

# 3) 确认是否接了 CDN
nslookup freeapi.tingfengai.art 8.8.8.8
#    返回源站 IP（103.233.252.213 / japi 103.110.80.198）= 未接 CDN，直连源站
#    返回 Cloudflare IP（104.x / 172.67.x 等）= 已接 CDN，另查 CDN 侧
```

## 判据速查（2026-10-04 本机实测）

| 测试 | 结果 | 含义 |
|---|---|---|
| 走代理 `https://freeapi.../api/status` | 15/15 = 200 | 经代理正常 |
| 直连（`--noproxy '*'`）同域名 | 15/15 全 ERR | 直连必 reset |
| 直连 IP（pin 源站） | 7/8 reset | 非域名问题（是 IP:443） |
| 直连 `http://IP:3000` 明文 | 6/6 = 200 | TCP 通、明文通 |
| 直连 IP:443 TLS（换任何 SNI） | 全 reset | 按目的 IP:443 触发，与 SNI 无关 |
| 直连 cloudflare.com / baidu.com | 200 / 200 | 本机直连本身可用 |

服务器侧同期实测：load 0.68、内存余 926MB、`new-api` healthy、上游 3000→200(2.4ms)、
Caddy 近 200 条访问日志 **0 个 5xx**、防火墙 inactive → **服务器 100% 健康**。

## 彻底解法（需用户授权；2026-10-04 用户选择「先维持现状」）

唯一根治 = **让流量不直连香港源站 IP**：

1. **Cloudflare 橙云接入**：把 `freeapi/japi` 的 A 记录改为 *proxied*（橙云），
   或把 NS 从 dnspod 迁到 Cloudflare。大陆用户连 Cloudflare 边缘 IP，TLS 由 CF 终结，绕开 RST。
2. **Cloudflare Tunnel**：源站装 `cloudflared`（当前**未安装**），源站不暴露公网 IP。

两者都需用户的 Cloudflare / dnspod 账号 + 改 DNS，属生产基础设施改动。

**临时办法**：访问时挂稳定代理 / 换代理节点。

## ⚠️ 纪律

- **别把 `ERR_CONNECTION_RESET` 当服务器故障处理、别盲目重启/改服务器配置**：先跑上面三步。
- 本机（开发机）curl 直连香港 IP 常返 `000`；**验收线上健康一律「服务器本机 curl」**，勿用本机直连判断。
- 本机存在本地代理（`HTTP(S)_PROXY=127.0.0.1:10808`，v2rayN 默认端口）：
  代理时 curl 的 `remote_ip` 会显示 `127.0.0.1`（normal），要测「真实直连」必须加 `--noproxy '*'`。
- 本机 curl 走 Windows schannel 遇 `CRYPT_E_REVOCATION_OFFLINE`（CRL 离线）→ 加 `-k` 或 `--ssl-no-revoke`。
