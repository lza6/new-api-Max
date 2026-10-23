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
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { defineConfig } from 'vitest/config'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    // Slow CI/dev machines spend 10s+ on module transform and jsdom setup;
    // the 5s default times out healthy interaction tests there.
    testTimeout: 20_000,
    environment: 'jsdom',
    server: {
      deps: { inline: [/@lobehub\//, /antd-style/] },
    },
    setupFiles: ['./src/test-setup.ts'],
    clearMocks: true,
    restoreMocks: true,
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    // Vitest 4 removed `poolOptions`; concurrency is now top-level. The default
    // worker pool fails to start on this Windows machine: jsdom setup alone
    // costs ~10s per worker, which trips vitest-pool-runner's
    // "Timeout waiting for worker to respond" before any test file runs
    // (`Test Files no tests / Errors N`).
    //
    // `threads` + a single worker is the combination verified to work here
    // (`forks` still times out on this machine). `fileParallelism: false`
    // keeps the suite deterministic at the cost of speed — correctness over
    // speed for a suite that would otherwise report zero tests.
    // Worker startup on this Windows machine costs ~57s of jsdom setup, which
    // exceeds vitest-pool-runner's startup budget. `vmThreads` is the pool
    // verified to start here (threads and forks both time out), and a single
    // worker keeps the suite deterministic.
    // 用 threads 而不是 vmThreads：vmThreads 下同 worker 共享一个 VM 上下文，
    // 一个文件的 vi.mock('sonner') 会被另一个文件复用，断言于是打在自己新建的
    // vi.fn 上（典型症状「单跑绿、合跑红」，本轮在 delete-plan-dialog / confirm-dialog /
    // model-cards / transparency-panel 上各踩一次）。threads + maxWorkers:1 实测
    // 既有真隔离又不慢（5 文件 40 用例 9.5s）。
    pool: 'threads',
    // 每个测试文件一个独立 VM 上下文：vmThreads 默认会把已加载模块（含 vi.mock
    // 的替身）在同 worker 内共享，导致 A 文件的 mock 被 B 文件复用、B 的断言打在
    // 自己新建的 vi.fn 上（表现为「单跑绿、合跑红」）。
    isolate: true,
    maxWorkers: 1,
    fileParallelism: false,
  },
})
