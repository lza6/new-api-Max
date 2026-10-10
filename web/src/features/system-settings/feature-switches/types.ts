/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
/**
 * 能力开关（Feature Switch）管理端的类型契约。
 *
 * 字段名与后端 `setting/feature_switch.Snapshot` 的 json tag 一一对应；
 * `title_key` / `description_key` / `rollback_hint` 是 i18n 键（英文源串），
 * 由一个开关的元数据携带，因此**新增开关不需要改前端**。
 */
export type FeatureSwitchRisk = 'low' | 'medium' | 'high'

export type FeatureSwitchKind = 'bool' | 'enum'

export interface FeatureSwitchSnapshot {
  /** 开关的 env 名（稳定契约）。 */
  key: string
  kind: FeatureSwitchKind
  /** kind=enum 时的合法取值。 */
  options?: string[]
  /** 当前生效值。 */
  value: string
  /** env 默认值。 */
  env_default: string
  /** 管理员是否显式配置过。 */
  configured: boolean
  /** 布尔语义下是否生效（已计入依赖判定）。 */
  effective: boolean
  /** 标题 i18n 键。 */
  title_key: string
  /** 描述 i18n 键。 */
  description_key: string
  risk: FeatureSwitchRisk
  depends_on?: string[]
  /** 未满足的前置开关（非空表示此开关当前不可能生效）。 */
  unsatisfied_deps?: string[]
  /** 是否允许在管理端修改。 */
  admin_editable: boolean
  /** 为 true 时保存的值需重启进程才生效。 */
  requires_restart: boolean
  /** 可在 /metrics 中观察的指标名；空表示暂无接入指标。 */
  metric_keys?: string[]
  /** 回滚提示 i18n 键。 */
  rollback_hint: string
}

export interface FeatureSwitchListData {
  switches: FeatureSwitchSnapshot[]
  /** 与 /metrics 同名指标同源的进程内读数。 */
  metrics: Record<string, number>
  metrics_endpoint: string
}

export interface FeatureSwitchListResponse {
  success: boolean
  message?: string
  data: FeatureSwitchListData
}

export interface FeatureSwitchUpdateRequest {
  key: string
  value?: string
  reset?: boolean
}

export type FeatureSwitchUpdateResponse = FeatureSwitchListResponse
