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
 * 字节 → 人类可读（B/KB/MB/GB/TB/PB）。
 *
 * 本 feature 内多个面板都要显示云盘体积（总览/文件浏览/每日历史）。
 * 原先每个面板各抄一份，其中两份的 `units` 数组还不一致
 * （一个到 `TB`、一个到 `PB`）—— 同一块云盘在两个面板上可能显示不同的量级。
 * 统一到这里，避免「同一数字两种读法」。
 */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B'
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  // 小数值保留两位；大数值取整，避免「5.00 TB」这种噪声。
  const rounded =
    value >= 100 ? Math.round(value) : Math.round(value * 100) / 100
  return `${rounded} ${units[unit]}`
}
