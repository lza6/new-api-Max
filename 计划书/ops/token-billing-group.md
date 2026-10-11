# 按 token 计费分组（token计费）运维说明

> 适用版本：v1.3.114+
> 最后更新：2026-10-09

## 1. 这是什么

同一模型可以有**两种计费方式并存**：

| 分组 | 计费方式 |
|---|---|
| `token计费` | 按输入上下文长度阶梯计价（每次调用固定价） |
| 其他分组（default / 你最爱的破jia / 优质渠道 …） | 保持原有**按次**计费，完全不变 |

实现方式：给计费表达式加 `group` 变量做分支，**不新增存储维度、不改表、不迁移**。
`group` 变量由 v1.3.114 引入（同时还有 `channel`）。

## 2. 当前生效的阶梯价

| 模型 | <200K | 200K~500K | ≥500K | 其他分组（按次） |
|---|---|---|---|---|
| `deepseek-v4.1-flash` | $0.001 | $0.0015 | $0.002 | $0.002 |
| `glm-5.3-flash` | $0.004 | $0.005 | $0.006 | $0.006 |

对应表达式（存于 `options.billing_setting.billing_expr`）：

```
group == "token计费" ? (len < 200000 ? tier("tok_lt200k", fixed(0.001))
  : (len < 500000 ? tier("tok_200k_500k", fixed(0.0015)) : tier("tok_ge500k", fixed(0.002))))
  : tier("per_request", fixed(0.002))
```

> `len` 是**输入上下文总长度**（非 Claude 格式等于 `prompt_tokens`）。阶梯边界用 `len` 而非 `p`，
> 避免缓存命中导致 `p` 变小而误判档位。

## 3. 新增/修改一个模型的 token 计费阶梯

必须同时改动 **4 处**，缺一会导致「分组下无可用渠道」或「价格不变」：

1. **计费表达式**（`options.billing_setting.billing_expr`）
   - 键 = 模型名，值 = 上面的表达式（替换阶梯价格）
   - **关键**：`:` 后的 else 分支必须写**该模型当前的按次价**，否则会改动其他分组的扣费。
2. **计费模式**（`options.billing_setting.billing_mode`）
   - `{ "<模型名>": "tiered_expr" }`
3. **渠道分组归属**（`channels."group"`）
   - 目标渠道的分组列表要追加 `token计费`（逗号分隔），例如 `default,token计费`
   - **渠道缓存按 `Channel.Group` 建索引**（`model/channel_cache.go:InitChannelCache`），不在这里加就找不到渠道。
4. **渠道能力**（`abilities` 表）
   - 为 `("token计费", <模型>, <渠道ID>)` 插入 enabled=true 的行。
   - 渠道更新（管理端保存）会自动重建；直接改库需触发一次渠道更新或重启。

**用户侧生效**：把该用户的 **Token 分组**设为 `token计费`。
注意 `middleware/auth.go` 逻辑：token 的分组**必须存在于该用户的「可用分组」白名单**
（`options.UserUsableGroups`），否则会被拒绝或回落到用户自己的分组。

## 4. 变更安全流程（**必须遵守**）

> 2026-10-09 曾因直接改生产计费表达式导致 225 次请求多扣费、补偿 $20.86。
> 以下流程就是为了不再发生。

1. **本地先跑测试**：
   ```bash
   go test ./pkg/billingexpr/ -run "TestTokenBilling|TestOtherGroups|TestElseBranch" -count=1
   ```
   `TestElseBranchEqualsLegacyPerRequestPrice` 会断言 else 分支与现状按次价**逐分组等值** ——
   这是防止「改价误伤其他分组」的自动化防线。
2. **只对测试 Token 生效**：新建一个仅自己可用的临时 token，分组设为 `token计费`，用它验证。
3. **对照验证**：用 default 分组的 token 打同一个模型，确认扣费与改动前一致。
4. **恢复测试痕迹**：删临时 token、把渠道分组改回原样（若为测试而加）。

## 5. 排障

| 现象 | 原因 | 处理 |
|---|---|---|
| `分组 X 下模型 Y 无可用渠道` | 渠道 `group` 未加 `token计费`，或 `abilities` 缺行 | 补齐第 3 节第 3/4 步 |
| 扣费仍按次价 | token 分组不是 `token计费`，或不在此用户可用分组白名单 | 检查 `tokens."group"` 与 `UserUsableGroups` |
| 日志显示固定 0.002 | 该请求走的是 else 分支（分组不匹配） | 属正常；看 `other.matched_tier` 确认 |
| 改价后其他分组扣费变了 | else 分支没写成当前按次价 | 立即回滚表达式 |

## 6. 日志字段（排查用）

消费日志 `other` 里记录了计费决策，可用于核对：

- `billing_mode`：`tiered_expr` 表示走表达式
- `matched_tier`：命中的档位（`tok_lt200k` / `tok_200k_500k` / `tok_ge500k` / `per_request`）
- `billing_unit`：`request`（按次）或 `token`
- `fixed_price`：**本次实际单价（USD）** —— 前端价格列优先显示它
- `group_ratio`：分组倍率（`token计费` 配置为 1）
