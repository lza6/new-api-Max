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
*/
export interface ModelTestMeta {
  /** 测试资产路径（iframe src） */
  asset: string
  /** 测试日期时间（本地展示字符串） */
  testedAt: string
  /** 测试提示词 */
  prompt: string
}

/**
 * 站点已发布「模型效果测试」的模型清单。
 * 模型广场卡片与 /model-test 页共用；新模型上线效果测试时在此登记。
 */
export const MODEL_TEST_META: Record<string, ModelTestMeta> = {
  'deepseek-v4-flash': {
    asset: '/model-test.html',
    testedAt: '2026-09-18 02:28:15',
    prompt:
      '创建一个HTML，内容是SVG绘制一个鹈鹕骑自行车的2D动画，你不需要任何测试。',
  },
}

export function getModelTestMeta(model?: string): ModelTestMeta | undefined {
  if (!model) {
    return undefined
  }
  return MODEL_TEST_META[model]
}
