# 专项分析 · 计费表达式与配额安全深挖（T4 + AGENTS 计费纪律）

> 定位：主指南 §T4 与 AGENTS 计费硬规则的**深挖文档**：必读 `pkg/billingexpr/expr.md` 的导读、安全不变量、落地锚点；只读整理。
> 生成：2026-09-25 · 未改业务代码。

## 1. 必读文件
- `pkg/billingexpr/expr.md`（表达式语言/架构/token 规范化/配额转换/版本化）——任何计费改动前第一读。
- `common/quota_math.go`（QuotaFromFloat / QuotaRound / QuotaFromDecimal + *Checked + QuotaClamp）。
- `types.PriceData.AddOtherRatio`（倍率守卫：拒绝非正/NaN/+Inf）。
- `service/log_info_generate.go`（`attachQuotaSaturation` 审计标记）。

## 2. 安全不变量（AGENTS 强制）
1. 用户可控乘数（图像 n、视频 seconds/duration、分辨率/质量、批量计数）**先 400 拒绝越界**：`dto.MaxImageN`、`relaycommon.MaxTaskDurationSeconds`、`maxTokensLimit`（relay/helper/valid_request.go）。新增格式在 validator 里同日加上界。
2. 绕行路径（`Extra["parameters"]`、任务 metadata、multipart 字段）读乘数时必须套同一界/饱和。
3. 媒体元数据/上游扣减（音频时长、Kling FinalUnitDeduction）转 token 前**饱和**。
4. 禁止裸 `int(float64(...))`/`int(math.Round(...))`/`int(decimal.IntPart())`；一律 `common.QuotaFromFloat*`/`QuotaRound*`/`QuotaFromDecimal*`（见 `service/log_traffic.go` 已改样例）。
5. 饱和事件走 `*Checked` + `attachQuotaSaturation`（admin_info 下）→ 管理员可见 + 日志 WARN；pre-consume 饱和必须额度不足失败，绝不静默回绕。
6. `*uint` 字段接收巨大正数（如 18446744073686646784 = 回绕负值）时 `>=0` 不够，必须有上界。

## 3. T4 现有状态（主指南）
- 用户入口费用解释已闭环（CostDetailPanel + 测试，c12465831）。
- 审计链路：`quota_saturation` 标记 + 请求关联 WARN；回归测试在 `relay/helper/openai_image_request_test.go`、`relay/common/relay_utils_test.go`、`common/quota_math_test.go`。
- 待办：新计费路径逐条追链（validation → EstimateBilling/OtherRatios → quota 转换 → pre-consume → settle/refund）确认不变量；用户默认可负担性提示（可选）。

## 4. 落地锚点（新增路径时对照）
1. 新增 relay 格式/DTO：validator 同日加 max_tokens 与 count 上界。
2. 任何 float 乘法：`QuotaFromFloatChecked`；任何 round：`QuotaRoundChecked`；decimal：`QuotaFromDecimalChecked`。
3. 任何倍率 map：`AddOtherRatio`（不直写 OtherRatios）。
4. 预扣费失败路径测试：巨大饱和额度 → 400/额度不足，非回绕。
5. 前端费用预估：读现有 pricing 接口（`web/src/features/model-pricing`），确认前显示区间。

## 5. 验证命令
- `go test ./pkg/billingexpr/... ./common/... ./service/ ./relay/...`
- `go test ./relay/helper/ ./relay/common/ -run "Quota|Saturat|MaxToken" -v`

## 闭环状态（2026-09-27，v1.3.44 回填）
- ✅ 裸转换审计通过（perf-ledger 0013）：全仓 3 处 int(float64(...))/int(math.Round(...)) 均为非计费路径
  （channel_health_score.go percentileOf 索引 / token_counter.go 图像像素尺寸 / common/utils.go 字节格式化）；计费转换全部走 common.Quota* 系列。无新增裸转换。
- 计费不变量回归测试已存在：relay/helper/openai_image_request_test.go、relay/common/relay_utils_test.go、common/quota_math_test.go。
