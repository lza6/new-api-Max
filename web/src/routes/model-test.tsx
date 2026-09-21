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
import {
  getModelTestMeta,
  MODEL_TEST_META,
} from "@/features/pricing/lib/model-test-meta"

export const Route = createFileRoute("/model-test")({
  validateSearch: (search: Record<string, unknown>) => ({
    model: typeof search.model === "string" ? search.model : undefined,
  }),
  component: ModelTestPage,
})

const DEFAULT_MODEL = "deepseek-v4-flash"

function ModelTestPage() {
  const { t } = useTranslation()
  const { model } = Route.useSearch()
  const meta = getModelTestMeta(model) ?? MODEL_TEST_META[DEFAULT_MODEL]
  const displayModel = model || DEFAULT_MODEL
  const hasMeta = Boolean(getModelTestMeta(model))
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
        <Card><CardHeader><CardTitle className="text-base">{t("Model")}</CardTitle></CardHeader><CardContent className="text-sm">{t("Current site model")}: <span className="font-mono">{displayModel}</span></CardContent></Card>
        <Card><CardHeader><CardTitle className="text-base">{t("Test time")}</CardTitle></CardHeader><CardContent className="font-mono text-sm">{meta.testedAt}</CardContent></Card>
        <Card><CardHeader><CardTitle className="text-base">{t("Prompt")}</CardTitle></CardHeader><CardContent className="break-all font-mono text-xs">{meta.prompt}</CardContent></Card>
      </div>
      {hasMeta ? (
        <Card>
          <CardHeader><CardTitle>{t("Rendered result")}</CardTitle></CardHeader>
          <CardContent>
            <iframe
              title="model-effect-test"
              src={meta.asset}
              className="min-h-[520px] w-full rounded-lg border"
              sandbox="allow-scripts"
            />
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardContent className="py-8 text-center text-sm">
            {t("No published effect test for this model yet")}
          </CardContent>
        </Card>
      )}
    </div>
  )
}
