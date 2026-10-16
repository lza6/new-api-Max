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

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { AnalysisPanel } from './components/analysis-panel'
import { CaptureOverviewPanel } from './components/overview-panel'
import { ExportPanel } from './components/export-panel'
import { FileBrowserPanel } from './components/file-browser-panel'

/**
 * 训练数据采集管理页（Spec-008 管理端）。
 *
 * 四个面板：
 *  - 总览：总开关 + 今日统计 + 云盘配额（含队列/在传实时值）
 *  - 文件：云盘目录下钻浏览 + 分片预览 + 删除 + 一键送去分析
 *  - 导出：按日期/用户/模型打包下载（支持断点续传）
 *  - 分析：把某个会话送去模型做深度分析，并可继续追问
 *
 * 仅管理员可见（路由层对应角色拦截）。
 * 布局用 `SectionPageLayout`：提供页面标题与**页脚容器**
 * （表格分页控件要 portal 到那里，缺了它分页会渲染为 null）。
 */
export function TrainingDataPage() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'overview' | 'files' | 'export' | 'analysis'>(
    'overview'
  )
  // 在「文件」里点「分析」时，把目标路径带过去。
  const [analysisPath, setAnalysisPath] = useState<string | null>(null)

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Training Data')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {/* 复用项目 Tabs 组件，不手搓 Button 切换（AGENTS.md 复用优先） */}
        <Tabs value={tab} onValueChange={(v) => setTab(v as typeof tab)}>
          <TabsList>
            <TabsTrigger value='overview'>{t('Overview')}</TabsTrigger>
            <TabsTrigger value='files'>{t('Cloud files')}</TabsTrigger>
            <TabsTrigger value='export'>{t('Export')}</TabsTrigger>
            <TabsTrigger value='analysis'>{t('Analysis')}</TabsTrigger>
          </TabsList>

          <TabsContent value='overview'>
            <CaptureOverviewPanel />
          </TabsContent>

          <TabsContent value='files'>
            <FileBrowserPanel
              onAnalyze={(path) => {
                setAnalysisPath(path)
                setTab('analysis')
              }}
            />
          </TabsContent>

          <TabsContent value='export'>
            <ExportPanel />
          </TabsContent>

          <TabsContent value='analysis'>
            <AnalysisPanel path={analysisPath} />
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
