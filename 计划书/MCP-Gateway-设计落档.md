# MCP Gateway 设计落档（G11-13.2.4，P3）

> 状态：**设计落档，未实现**。指南 §13.2.4 明确要求本项「只输出设计落档」——
> 它是独立产品面，跨度过大，应单独立项。本文件**不声称任何功能已可用**。

## 1. 为什么单独立项

MCP（Model Context Protocol）是**服务端**能力：把本站已聚合的上游模型能力以
MCP tool / resource 的形式暴露给 MCP 客户端（Claude Code、Cursor、Cline 等）。
现有代码库有 Skills、Agent 预设、任务插件，但**没有任何 MCP 服务端实现**
（`grep -ri "mcp" --include=*.go` 只命中零散注释与第三方依赖，无协议层）。

它跨越的边界比"再加一个 `/v1/*` 端点"大得多：

| 维度 | 普通 relay 端点 | MCP Gateway |
|---|---|---|
| 协议 | HTTP + JSON/SSE 一问一答 | JSON-RPC 2.0 双向（`initialize` / `tools/list` / `tools/call` / notifications） |
| 会话 | 无状态 | 有会话（`Mcp-Session-Id`，需生命周期与超时治理） |
| 能力协商 | 无 | `capabilities` 握手决定可用 tool 集 |
| 传输 | HTTP | stdio / Streamable HTTP（SSE）/ 可选的旧式 HTTP+SSE |
| 工具审批 | 无此概念 | 需动作级授权（与 G5-3 同源） |

因此**不塞进本轮**。

## 2. 目标能力映射（设计草案）

### 2.1 Tool 映射

把已有的 relay 能力包成 MCP tool，**不新建后端逻辑**，只做协议适配：

| MCP tool | 映射到 | 备注 |
|---|---|---|
| `chat_completion` | `POST /v1/chat/completions` | 非流式优先；流式需 MCP progress notification |
| `list_models` | `GET /v1/models` | 结果按调用者分组过滤（**不能泄露他人分组的模型**） |
| `create_image` | `POST /v1/images/generations` | 付费类，默认不暴露（见 §4） |
| `embed_text` | `POST /v1/embeddings` | |
| `web_search` | `POST /v1/alpha/search` | 若该部署启用 |

### 2.2 Resource 映射

只读资源，**必须按调用者身份过滤**：

| MCP resource URI | 内容 | 授权 |
|---|---|---|
| `newapi://me/quota` | 当前凭据的额度/用量 | 仅本人 |
| `newapi://me/logs` | 当前凭据的调用日志 | 仅本人 |
| `newapi://models` | 可用模型清单 | 按分组过滤 |
| `newapi://pricing` | 公开价目 | 公开 |

## 3. 复用既有基础设施（这是它可行的原因）

| 关注点 | 复用 | 位置 |
|---|---|---|
| 认证 | **现有 API token**（`Authorization: Bearer sk-...`） | `middleware/auth.go` 既有链路 |
| 用户/分组解析 | 既有 relay 前置中间件 | `middleware/distributor.go` |
| 速率限制 | 既有 `user_base_rpm_limit` / 并发桶 | `middleware/rate_limit.go` |
| 额度与预扣 | 既有 `EstimateBilling` → 预扣 → 结算 | `service/` 计费链路 |
| 日志 | 既有 `logs` 表 + `RecordConsumeLog` | `service/log_info_generate.go` |
| 指标 | 既有 gauge / `loop` 心跳 | `common/metrics.go` |
| 能力开关 | G1 开关注册表（灰度 + 一键回滚） | `setting/feature_switch/` |

**不新增任何计费/鉴权旁路** —— 这是硬约束。任何 MCP tool 调用都必须走与
HTTP relay 完全相同的 `EstimateBilling → 预扣 → 结算`，否则会出现
"绕过 relay handler 自己计费"那一类缺陷（本项目已有血泪教训）。

## 4. 必须在实现前定下的决策（不建议顺手决定）

1. **付费类 tool 默认是否暴露**？
   - 若暴露，MCP 客户端可能在用户无感知的情况下反复调用付费 tool（图像/视频）。
   - 建议：默认**仅暴露只读 + 文本类**；付费类需管理员显式开关 + 每 tool 单独授权。
2. **流式如何映射**？
   - MCP 没有 SSE token 流；只能用 progress notification 或分块返回。
   - 建议：v1 只支持非流式，流式标记为不支持并给出明确错误。
3. **动作级授权（与 G5-3 的关系）**：`tools/call` 本质就是"动作"，与指南
   G5-3 要求的**动作级 deny-by-default** 是同一个问题。建议两者**合并设计**，
   否则会做出两套互不相容的审批。
4. **会话与多实例**：`Mcp-Session-Id` 需要粘性或共享存储。当前部署是
   单实例（`new-api` 容器），但蓝绿部署期间会短暂双实例 —— 会话必须能跨实例，
   否则滚动更新会打断进行中的会话。
5. **传输选择**：stdio 只适用于本机（要用户在本地跑一个进程），
   Streamable HTTP 才是本网关的形态。旧式 HTTP+SSE 已不推荐。

## 5. 验收门（实现时适用）

沿用指南 §13.3 的 G11 验收总门，额外补：

- [ ] `tools/call` 的计费与 HTTP relay **逐字一致**（有对照用例）
- [ ] 认证/分组过滤有**跨用户越权用例**（A 的凭据不得列出 B 的模型/日志）
- [ ] 会话超时与清理有界（防内存累积）
- [ ] 能力开关可一键关闭且**关闭后端点完全不可达**
- [ ] 未实现的能力（流式等）返回**明确错误**，不静默降级

## 6. 结论

MCP Gateway 可行且能复用绝大部分既有基础设施，但需要先定 §4 的 5 个决策。
**本文件只做落档，代码零改动** —— 与指南 §13.2.4 的要求一致。
