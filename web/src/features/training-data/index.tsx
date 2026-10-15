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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { AnalysisPanel } from './components/analysis-panel'
import { CaptureOverviewPanel } from './components/overview-panel'
import { FileBrowserPanel } from './components/file-browser-panel'

/**
 * 训练数据采集管理页（Spec-008 管理端）。
 *
 * 三个面板：
 *  - 总览：总开关 + 今日统计 + 云盘配额（含队列/在传实时值）
 *  - 文件：云盘目录下钻浏览 + 分片预览 + 删除 + 一键送去分析
 *  - 分析：把某个会话送去模型做深度分析，并可继续追问
 *
 * 仅管理员可见（路由层已用 AdminAuth 对应角色拦截）。
 */
export function TrainingDataPage() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'overview' | 'files' | 'analysis'>('overview')
  // 在「文件」里点「分析」时，把目标路径带过去。
  const [analysisPath, setAnalysisPath] = useState<string | null>(null)

  const tabBtn = (id: typeof tab, label: string) => (
    <Button
      type='button'
      variant={tab === id ? 'default' : 'outline'}
      onClick={() => setTab(id)}
    >
      {t(label)}
    </Button>
  )

  return (
    <div className='space-y-4 p-4 sm:p-6'>
      <div className='flex flex-wrap items-center gap-2'>
        {tabBtn('overview', 'Overview')}
        {tabBtn('files', 'Cloud files')}
        {tabBtn('analysis', 'Analysis')}
      </div>

      {tab === 'overview' && <CaptureOverviewPanel />}

      {tab === 'files' && (
        <FileBrowserPanel
          onAnalyze={(path) => {
            setAnalysisPath(path)
            setTab('analysis')
          }}
        />
      )}

      {tab === 'analysis' && <AnalysisPanel path={analysisPath} />}
    </div>
  )
}
