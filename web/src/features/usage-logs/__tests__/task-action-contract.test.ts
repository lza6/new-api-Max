/*
Copyright (C) 2023-2026 QuantumNous
Licensed under GNU AGPL v3. See LICENSE.
*/
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, test } from 'vitest'

import { TASK_ACTION_MAPPINGS, TASK_ACTIONS } from '../constants'

/**
 * 前后端任务 action 枚举的**契约守卫**。
 *
 * ## 为什么要有这个测试
 *
 * 曾经的真实缺陷：后端把 action 归一化为 canonical snake_case
 * （`constant/task.go` 的 `TaskActionImageToVideo` = `image_to_video` 等，
 * 由 `constant.NormalizeTaskAction` 在 `relay.TaskModel2Dto` 里应用），
 * 而前端映射表仍只有 legacy camelCase 键（`generate` / `textGenerate` …）。
 * 两边**零重叠** → 任务日志与详情弹窗的「操作类型」整列静默退化成 `Unknown`，
 * 页面不报错、前端测试也不失败（测试用的还是 legacy 值，与真实契约脱节）。
 *
 * ## 为什么直接读 Go 源文件
 *
 * 只在前端这边写一份"期望值清单"是无效守卫：后端改了值它照样绿。
 * 这里直接解析 `constant/task.go` 里 `TaskAction*` 常量的字面量，
 * 因此**后端一旦新增或改名，这个测试立刻失败**，逼迫前端同步。
 * （同类做法在服务端也有：`controller/spa_seo_test.go` 的 FAQ 用例直接读前端源码。）
 */
function readBackendCanonicalActions(): string[] {
  // vitest 的 cwd 是 web/，后端源码在上一级。
  const goSource = readFileSync(
    resolve(process.cwd(), '..', 'constant', 'task.go'),
    'utf8'
  )
  const values = new Set<string>()
  // 形如：TaskActionImageToVideo     = "image_to_video"
  const re = /TaskAction[A-Za-z]+\s*=\s*"([^"]+)"/g
  for (const m of goSource.matchAll(re)) {
    values.add(m[1])
  }
  return [...values].sort()
}

describe('task action 前后端契约', () => {
  const canonical = readBackendCanonicalActions()

  test('能从后端 constant/task.go 读到 canonical action 常量', () => {
    // 若解析失败（文件被移动/改名/正则失效），必须显式失败而不是"零断言通过"。
    expect(canonical.length).toBeGreaterThan(0)
  })

  test.each(readBackendCanonicalActions())(
    '前端映射表必须覆盖后端 canonical 值 %s',
    (action) => {
      expect(
        TASK_ACTION_MAPPINGS[action],
        `后端下发 "${action}" 但前端没有对应映射 —— 页面上会显示 Unknown`
      ).toBeDefined()
      expect(TASK_ACTION_MAPPINGS[action].label).not.toBe('Unknown')
    }
  )

  test('legacy 别名仍保留映射（兼容旧数据）', () => {
    for (const legacy of [
      'generate',
      'textGenerate',
      'firstTailGenerate',
      'referenceGenerate',
      'remixGenerate',
    ]) {
      expect(TASK_ACTION_MAPPINGS[legacy], `legacy 值 ${legacy} 不应失去映射`).toBeDefined()
    }
  })

  test('TASK_ACTIONS 常量本身就包含全部 canonical 值', () => {
    const declared = new Set<string>(Object.values(TASK_ACTIONS))
    for (const action of readBackendCanonicalActions()) {
      expect(declared.has(action), `TASK_ACTIONS 缺少 ${action}`).toBe(true)
    }
  })
})
