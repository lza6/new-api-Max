# N16 侦察报告：Agent 安全治理与护栏（已完成，20 项）

## 关键项目

### casbin-gateway（go|large）— 业务相似度最高 5/5
- 本机编码 Agent 统一网关（代理+控制台），Casbin 强制访问控制
- 每 Agent ~40 个权限开关（工具/模型/供应商三维），每请求 Casbin enforce（README+agentauth/agentauth.go）
- 凭据托管：供应商 key 存 Gateway，Agent 只持中继 token（README 261 行）
- **Authenticity 上游真实性探测（杀手锏）**：provider 新增/端点变化/密钥变更/报告过期时自动探针——"唯一正确答案题库"+协议包络检查（logprobs 是否被静默丢弃、tool schema 是否存活、cache 是否真计费）打分 0-100 评 A-F，专抓"转卖便宜模型冒充旗舰、假 cache 命中"（README 232-236）
- 评分：业务 5 / 架构 5 / 工程 4 / 迁移 5

### doorman（node|medium，Rust）
- Rust API 网关+控制平面：滑动窗限流加权插值（gateway-rs/src/policy/rate_limit.rs）；带宽按天窗口；per-API IP 黑白名单+XFF 显式信任（ip.rs）；credits/quota/tier/subscription 四层配额
- 评分：5/4/4/4

### SafeLine 雷池（node|large）
- 长亭 WAF：检测引擎与数据面解耦（t1k 旁路协议）+ nginx/kong/traefik/ingress 四适配器；fail-open/closed 可配；mcp_server 带 token init/rotate
- 评分：4/5/5/3

### destructive_command_guard（Rust）— 工程最成熟命令护栏
- executable_spans/data_spans 上下文分类（grep "rm -rf" 放行 vs rm -rf / 拦截）（src/context.rs）
- 两级检测：触发词快筛 <100μs → AST 匹配 <5ms（src/heredoc.rs）；135 个 pack 按需启用；fail-closed 可配；安装带 SHA256+minisign+cosign；豁免留痕
- 评分：2/5/5/4

### 其他要点
- agent-guardrails：deny 数组 45 条破坏性模式 + hook 双层 + 事故复盘文档。2/3/4/3
- arbiterForge__codeArbiter：18 条证据驱动 lane；H-09b/H-10b 阻断式密钥提交门（staged added_lines 扫描 exit 2）；唯一豁免通道+强制审计。2/4/4/3
- OWASP-MCP-Governance：Tier 0-4 五级分类（按最高风险工具定级）+ 8 因子打分 + 四条硬规则（No owner=No approval、No logging=No production…）+"Pre-Approved Catalog 铺好路"。3/5/4/4
- SkillSpector（NVIDIA）：71 漏洞模式/17 类、33 分析器（含 mcp_rug_pull 安装后行为变更检测、mcp_least_privilege）；两阶段静态+LLM；fail-closed 资源上限。3/4/5/4
- agentseal：guard 六级管线（签名→**去混淆**：Unicode tags/Base64/BiDi/零宽/TR39→语义相似→SHA-256 基线 rug-pull→MCP 信任分→YAML 规则）；scan 用 canary 字符串确定性判定（225+ 探针，无 LLM 抖动）。3/4/4/4
- memguard-agent：记忆投毒 eBPF 写入瞬间检测+FS 补偿读；默认 dry-run 上线。原则可迁（GORM hook 落库前过滤）。3/4/3/2
- guardian-cli：ScopeValidator 硬编码 RFC1918/环回/169.254.169.254 黑名单+每步重验 scope——SSRF 纵深防御范本。2/3/4/3
- trivy：镜像 CVE+secret 扫描入 CI。2/4/5/4
- web-check：安全头评级清单（CSP/HSTS）→ new-api 自检端点。3/3/4/3
- vps-audit："检查防护是否真的在生效"（fail2ban 端口对齐）独特审计视角。2/2/3/3
- obscura：反检测无头浏览器=对手面参照——bot 防护不能依赖 UA/指纹单点。2/3/4/2
- Sparrowgate 补注：4 条自修复（明文→加密、MD5→SHA-256、无 TTL→TTL、竞态→锁）是提示缓存安全底线

## AI 网关安全加固 Top 清单

### 密钥管理（P0）
1. **密钥泄露审计与脱敏**：日志/错误返回路径全量排查 key 明文；日志中密钥 HMAC 指纹化；CI 阻断式 secret 提交门
2. **凭据托管模型**（casbin-gateway）：渠道 key 只存服务端，任何响应不回显完整 key；导出强制二次认证+审计
3. **破坏性操作硬闸**（dcg/agent-guardrails）：deny 模式清单+原因码+唯一豁免通道（不静默）

### 滥用检测（P0）
4. **滑动窗口限流升级**（doorman）：固定窗补滑动窗插值，per-token+per-IP 双维；XFF 信任显式配置
5. **上游真实性探测**（casbin-gateway Authenticity）：渠道健康分升级为"模型真实性"题库+协议包络检查，A-F 评分防以次充好
6. **异常用量画像**：token 级基线+突增告警、同 IP 多 token 关联、失效 token 重放监控
7. **SSRF 硬防线**（guardian）：所有服务端外呼强制内网+环回+169.254.169.254 硬编码黑名单，配置不可绕过

### 内容审计（P1，8-9 可提至 P0.5）
8. **上行 prompt 三级过滤**（agentseal）：签名→去混淆→语义分类，旁路检测服务（SafeLine t1k 解耦），fail-open/closed 可配
9. **注入探测回归集**（agentseal canary）：canary 确定性判定（不用 LLM judge）对自身系统提示词做 225+ 探针回归
10. **插件产物写入侧检测**（memguard 时点原则）：Sobek JS 插件落库前"伪造信任背书"签名扫描（GORM hook），dry-run 灰度
11. **插件市场治理**（OWASP MCP 框架）：owner/数据范围/动作能力/日志要求登记，Tier 驱动审查；装前 SkillSpector 扫描+装后基线重扫防 rug-pull
12. **管理台安全头自检**：CSP/HSTS/cookie 评级端点
13. **部署供应链**（trivy+vps-audit）：镜像扫描入 CI；"防护是否真生效"类检查
14. **Bot 防护分层**（obscura）：不依赖 UA/指纹单点，行为+配额+Turnstile 多层

评分汇总：casbin-gateway 5/5/4/5 · doorman 5/4/4/4 · SafeLine 4/5/5/3 · dcg 2/5/5/4 · OWASP MCP 3/5/4/4 · SkillSpector 3/4/5/4 · agentseal 3/4/4/4 · codeArbiter 2/4/4/3

关键证据：casbin-gateway\README.md · doorman\gateway-rs\src\policy\rate_limit.rs、ip.rs · destructive_command_guard\src\context.rs、heredoc.rs · agentseal\python\agentseal\deobfuscate.py、canaries.py · SkillSpector\src\skillspector\nodes\analyzers\ · guardian-cli\utils\scope_validator.py · OWASP-MCP-Governance-and-Risk-Project\mcp-governance-risk-framework-v1.0.md · arbiterForge__codeArbiter\core\pysrc\_bashguardlib.py
