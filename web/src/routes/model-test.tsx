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
import { createFileRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

export const Route = createFileRoute("/model-test")({
  component: ModelTestPage,
})

const SITE_MODEL = "deepseek-v4-flash"
const TEST_TIME = "2026-09-18 02:28:15"
const TEST_PROMPT =
  "创建一个HTML，内容是SVG绘制一个鹈鹕骑自行车的2D动画，你不需要任何测试。"

function ModelTestPage() {
  const { t } = useTranslation()
  return (
    <div className="mx-auto w-full max-w-5xl space-y-4 p-4 sm:p-6">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">{t("Model Effect Test")}</h1>
        <p className="text-muted-foreground text-sm">{t("Model test page intro")}</p>
      </div>
      <Card>
        <CardHeader><CardTitle>{t("Why this test")}</CardTitle></CardHeader>
        <CardContent className="space-y-2 text-sm">
          <p>{t("Why this test body 1")}</p>
          <p>{t("Why this test body 2")}</p>
        </CardContent>
      </Card>
      <div className="grid gap-3 sm:grid-cols-3">
        <Card><CardHeader><CardTitle className="text-base">{t("Model")}</CardTitle></CardHeader><CardContent className="text-sm">{t("Current site model")}: <span className="font-mono">{SITE_MODEL}</span></CardContent></Card>
        <Card><CardHeader><CardTitle className="text-base">{t("Test time")}</CardTitle></CardHeader><CardContent className="font-mono text-sm">{TEST_TIME}</CardContent></Card>
        <Card><CardHeader><CardTitle className="text-base">{t("Prompt")}</CardTitle></CardHeader><CardContent className="break-all font-mono text-xs">{TEST_PROMPT}</CardContent></Card>
      </div>
      <Card>
        <CardHeader><CardTitle>{t("Rendered result")}</CardTitle></CardHeader>
        <CardContent>
          <iframe
            title="pelican-bike-svg"
            src="/model-test.html"
            className="min-h-[520px] w-full rounded-lg border"
            sandbox="allow-scripts"
          />
        </CardContent>
      </Card>
    </div>
  )
}
