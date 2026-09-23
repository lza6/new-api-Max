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


/** 站内主模型：网关在 OpenAI/Anthropic/Codex 协议下同名映射到上游。 */
const SITE_MODEL = 'deepseek-v4-flash'

interface ToolPreset {
  key: string
  titleKey: string
  introKey: string
  configLang: string
  config: (base: string, model: string) => string
}

function buildPresets(): ToolPreset[] {
  return [
    {
      key: 'claude-code',
      titleKey: 'Claude Code',
      introKey: 'Claude Code intro',
      configLang: 'bash',
      config: (b, m) => [
        '# Claude Code（Anthropic 原生协议）',
        '# 推荐写入 ~/.claude/settings.json 的 env 或 shell profile：',
        `export ANTHROPIC_BASE_URL="${b}/v1"`,
        'export ANTHROPIC_AUTH_TOKEN="<YOUR_API_KEY>"',
        `export ANTHROPIC_MODEL="${m}"`,
        '',
        '# 验证：claude "hi"',
      ].join('\n'),
    },
    {
      key: 'cursor',
      titleKey: 'Cursor',
      introKey: 'Cursor intro',
      configLang: 'json',
      config: (b, m) => [
        '// Cursor → Settings → Models → 自定义 API（OpenAI Compatible）',
        '// 或写入项目 .cursor/settings.json：',
        '{',
        `  "api": {`,
        `    "baseUrl": "${b}/v1",`,
        '    "headers": { "Authorization": "Bearer <YOUR_API_KEY>" },',
        `    "model": "${m}"`,
        '  }',
        '}',
      ].join('\n'),
    },
    {
      key: 'opencode',
      titleKey: 'OpenCode',
      introKey: 'OpenCode intro',
      configLang: 'json',
      config: (b, m) => [
        '// ~/.config/opencode/opencode.json',
        '{',
        '  "provider": {',
        '    "site": {',
        '      "type": "openai-compatible",',
        `      "base_url": "${b}/v1",`,
        '      "api_key": "<YOUR_API_KEY>"',
        '    }',
        '  },',
        `  "model": "${m}"`,
        '}',
      ].join('\n'),
    },
    {
      key: 'codex',
      titleKey: 'Codex CLI',
      introKey: 'Codex intro',
      configLang: 'toml',
      config: (b, m) => [
        '# ~/.codex/config.toml',
        '[model_providers.site]',
        'name = "site"',
        `base_url = "${b}/v1"`,
        'wire_api = "responses"',
        'env_key = "SITE_API_KEY"',
        '',
        '# env',
        'export SITE_API_KEY="<YOUR_API_KEY>"',
        `# 用法：codex --provider site --model ${m}`,
      ].join('\n'),
    },
    {
      key: 'cline',
      titleKey: 'Cline',
      introKey: 'Cline intro',
      configLang: 'text',
      config: (b, m) => [
        '// Cline → API Provider：OpenAI Compatible',
        `// Base URL: ${b}/v1`,
        '// API Key: <YOUR_API_KEY>',
        `// Model ID: ${m}`,
        '',
        '// 也可直接填入 Chat Completions 端点：',
        `// ${b}/v1/chat/completions`,
      ].join('\n'),
    },
  ]
}

export function ToolIntegrationSection() {
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

  const presets = useMemo(() => buildPresets(), [])

  return (
    <div className="mx-auto w-full max-w-5xl space-y-4 p-4 sm:p-6">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">
          {t('Tool Integration Presets')}
        </h1>
        <p className="text-muted-foreground text-sm">
          {t('Tool integration intro')}
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('Three steps')}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3 text-sm sm:grid-cols-3">
          <div className="bg-muted/30 rounded-md border p-3">
            <div className="font-medium">{t('Step 1')}</div>
            <div className="text-muted-foreground">{t('Step 1 body')}</div>
          </div>
          <div className="bg-muted/30 rounded-md border p-3">
            <div className="font-medium">{t('Step 2')}</div>
            <div className="text-muted-foreground">{t('Step 2 body')}</div>
          </div>
          <div className="bg-muted/30 rounded-md border p-3">
            <div className="font-medium">{t('Step 3')}</div>
            <div className="text-muted-foreground">{t('Step 3 body')}</div>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-3 md:grid-cols-2">
        {presets.map((preset) => (
          <Card key={preset.key}>
            <CardHeader className="flex flex-row items-start justify-between gap-2">
              <CardTitle className="text-base">{t(preset.titleKey)}</CardTitle>
              <CopyButton value={preset.config(baseUrl, SITE_MODEL)} size="sm" />
            </CardHeader>
            <CardContent className="space-y-2 text-sm">
              <p className="text-muted-foreground">{t(preset.introKey)}</p>
              <pre className="bg-muted/40 overflow-x-auto rounded-md border p-3 font-mono text-xs leading-relaxed">
                {preset.config(baseUrl, SITE_MODEL)}
              </pre>
            </CardContent>
          </Card>
        ))}

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('Model mapping')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <p className="text-muted-foreground">{t('Model mapping intro')}</p>
            <div className="bg-muted/30 overflow-x-auto rounded-md border p-3 font-mono text-xs">
              <div className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1">
                <span className="text-muted-foreground">{t('OpenAI / Responses')}</span>
                <span>{SITE_MODEL}</span>
                <span className="text-muted-foreground">{t('Anthropic')}</span>
                <span>{SITE_MODEL}</span>
                <span className="text-muted-foreground">{t('Codex')}</span>
                <span>{SITE_MODEL}</span>
              </div>
            </div>
            <p className="text-muted-foreground text-xs">
              {t('Model mapping note')}
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}