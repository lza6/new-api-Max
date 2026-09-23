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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useStatus } from '@/hooks/use-status'
import { Link } from '@tanstack/react-router'

import { ToolIntegrationSection } from '@/features/tool-setup/tool-integration-section'

function CodeBlock(props: { label: string; code: string }) {
  return (
    <div className="bg-muted/40 space-y-2 rounded-md border p-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-muted-foreground text-xs">{props.label}</span>
        <CopyButton value={props.code} size="sm" />
      </div>
      <pre className="overflow-x-auto font-mono text-xs leading-relaxed">
        {props.code}
      </pre>
    </div>
  )
}

/**
 * 站内详细使用文档（/docs）：快速开始、协议与模型、模型效果测试、
 * 工具接入、订阅说明。替代原来跳转 GitHub 的外部文档链接。
 */
export function Docs() {
  const { t } = useTranslation()
  const { status } = useStatus()

  const baseUrl = useMemo(() => {
    const apiInfo = status?.api_info as Array<{ url?: string }> | undefined
    const candidate = status?.server_address || apiInfo?.[0]?.url
    if (typeof candidate === 'string' && candidate) {
      return candidate.replace(/\/+$/, '')
    }
    return typeof window !== 'undefined' ? window.location.origin : ''
  }, [status])

  const curlExample = `curl ${baseUrl}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -d '{
    "model": "deepseek-v4-flash",
    "messages": [{"role": "user", "content": "你好"}]
  }'`

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6 p-4 sm:p-6">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">
          {t('Usage Docs')}
        </h1>
        <p className="text-muted-foreground text-sm">
          {t('Usage docs intro')}
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('Quick Start')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <div className="grid gap-2 sm:grid-cols-2">
            <div className="bg-muted/30 rounded-md border p-3">
              <div className="text-muted-foreground text-xs">
                {t('Base URL')}
              </div>
              <div className="mt-1 flex items-center justify-between gap-2 font-mono text-xs">
                <span className="break-all">{baseUrl}/v1</span>
                <CopyButton value={`${baseUrl}/v1`} size="sm" />
              </div>
            </div>
            <div className="bg-muted/30 rounded-md border p-3">
              <div className="text-muted-foreground text-xs">
                {t('API Key')}
              </div>
              <div className="mt-1 text-xs">
                {t('Create in Console')}
              </div>
            </div>
          </div>
          <CodeBlock label={t('Chat Completions example')} code={curlExample} />
          <p className="text-muted-foreground text-xs">
            {t('Docs api key note')}
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t('Protocols & Models')}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <p className="text-muted-foreground">
            {t('Protocols & models intro')}
          </p>
          <div className="bg-muted/30 overflow-x-auto rounded-md border p-3 font-mono text-xs">
            <div className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1">
              <span className="text-muted-foreground">{t('OpenAI Compatible')}</span>
              <span>{baseUrl}/v1</span>
              <span className="text-muted-foreground">{t('Anthropic Compatible')}</span>
              <span>{baseUrl}/v1</span>
              <span className="text-muted-foreground">{t('Codex / Responses')}</span>
              <span>{baseUrl}/v1</span>
            </div>
          </div>
          <p className="text-muted-foreground text-xs">
            {t('Protocols & models note')}
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t('Model Effect Test')}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <p className="text-muted-foreground">
            {t('Model test docs body')}
          </p>
          <Link
            to="/model-test"
            search={{ model: undefined }}
            className="text-primary hover:text-primary/80 font-medium"
          >
            {t('Go to model effect test')}
          </Link>
        </CardContent>
      </Card>

      <div className="space-y-3">
        <h2 className="text-lg font-semibold tracking-tight">
          {t('Tool Integration')}
        </h2>
        <ToolIntegrationSection />
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('Subscription')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <p className="text-muted-foreground">
            {t('Subscription docs body')}
          </p>
          <p className="text-muted-foreground text-xs">
            {t('Contact WeChat Tf00798 for higher concurrency/RPM')}
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
