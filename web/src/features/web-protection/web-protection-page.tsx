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
/* oxlint-disable react/no-array-index-key -- string detail values may repeat; index keeps keys unique */

import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Switch } from "@/components/ui/switch"
import { Badge } from "@/components/ui/badge"
import { handleServerError } from "@/lib/handle-server-error"

import {
  type WebProtectionSettings,
  formatBytes,
  getServerStats,
  getWebProtectionSettings,
  unbanIP,
  type ServerStats,
  updateWebProtectionSettings,
} from "./api"
import { BannedIPsTable } from "./components/banned-ips-table"
import { WebRequestLogsTable } from "./components/web-request-logs-table"

const PRESETS = [
  { id: "relaxed", label: "Relaxed (10/s, burst 60)", perSec: 10, burst: 60 },
  { id: "standard", label: "Standard (5/s, burst 40)", perSec: 5, burst: 40 },
  { id: "strict", label: "Strict (2/s, burst 20)", perSec: 2, burst: 20 },
]

function listToText(values: string[] | undefined): string {
  return (values ?? []).join("\n")
}

function textToList(value: string): string[] {
  return value
    .split(/[,\n]/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
}

function ServerStatsCard({ t }: { t: (k: string) => string }) {
  const [s, setS] = useState<ServerStats | null>(null)
  useEffect(() => {
    let alive = true
    const load = () => { getServerStats().then((d) => { if (alive) {setS(d)} }).catch(() => undefined) }
    load()
    const timer = setInterval(load, 1500)
    return () => { alive = false; clearInterval(timer) }
  }, [])
  if (!s) {return null}
  const inst = s.instance
  const res = inst?.resources
  const cpu = res?.cpu?.usage_percent
  const mem = res?.memory?.usage_percent
  const st = res?.storage
  return (
    <Card>
      <CardHeader><CardTitle>{t("Server realtime status")}</CardTitle></CardHeader>
      <CardContent className="grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <div><p className="text-muted-foreground">{t("Network out")}</p><p className="text-2xl font-semibold">{s.network_out_mbps.toFixed(2)} MB/s</p><p className="text-muted-foreground">{t("Network in")} {s.network_in_mbps.toFixed(2)} MB/s</p></div>
        <div><p className="text-muted-foreground">{t("CPU")}</p><p className="text-lg font-medium">{cpu != null ? cpu.toFixed(1) + "%" : "-"}</p></div>
        <div><p className="text-muted-foreground">{t("Memory")}</p><p className="text-lg font-medium">{mem != null ? mem.toFixed(1) + "%" : "-"}</p></div>
        <div><p className="text-muted-foreground">{t("Disk")}</p><p className="text-lg font-medium">{st ? formatBytes(st.used_bytes || 0) + " / " + formatBytes(st.total_bytes || 0) : "-"}</p></div>
        <div><p className="text-muted-foreground">{t("Host")}</p><p className="break-all font-mono">{inst?.host?.hostname || "-"}</p></div>
        <div><p className="text-muted-foreground">{t("Runtime")}</p><p>{inst?.runtime?.version || "-"} {inst?.runtime?.goos || ""}/{inst?.runtime?.goarch || ""}</p></div>
        <div><p className="text-muted-foreground">{t("Uptime")}</p><p>{s.uptime_seconds ? formatUptime(s.uptime_seconds) : "-"}</p></div>
        <div><p className="text-muted-foreground">{t("Active bans")}</p><p className="text-lg font-medium">{s.banned_count ?? 0}</p></div>
        <div>
          <p className="text-muted-foreground">{t("Today requests")}</p>
          <p className="text-lg font-medium">{s.today_request_count ?? 0}</p>
          <p className="text-muted-foreground">↓ {formatBytes(s.today_bytes_received || 0)} / ↑ {formatBytes(s.today_bytes_sent || 0)}</p>
        </div>
        <div>
          <p className="text-muted-foreground">{t("In-flight requests")}</p>
          <p className="text-2xl font-semibold">{s.in_flight ?? 0}</p>
        </div>
      </CardContent>
      <CardContent className="border-t pt-3">
        <p className="mb-2 text-sm font-medium">{t("Recent bans")}</p>
        {s.recent_bans && s.recent_bans.length > 0 ? (
          <ul className="space-y-1.5 text-sm">
            {s.recent_bans.map((b) => (
              <li key={b.id} className="flex flex-wrap items-center gap-2 rounded-md border p-2">
                <span className="font-mono">{b.ip}</span>
                <Badge variant="outline">{t("Banned")}</Badge>
                <span className="min-w-0 flex-1 truncate text-muted-foreground">{b.reason}</span>
                <span className="text-muted-foreground">{new Date(b.banned_at * 1000).toLocaleString()}</span>
                <Button type="button" size="sm" variant="outline" onClick={() => {
                  void (async () => {
                    try {
                      await unbanIP(b.ip)
                      toast.success(t("IP unbanned"))
                      const next = await getServerStats()
                      setS(next)
                    } catch (e) {
                      handleServerError(e)
                    }
                  })()
                }}>
                  {t("Unban")}
                </Button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-muted-foreground text-sm">{t("No recent bans")}</p>
        )}
      </CardContent>
    </Card>
  )
}

function formatUptime(sec: number): string {
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return (d > 0 ? d + "d " : "") + h + "h " + m + "m"
}

function SettingsTab({ t }: { t: (k: string) => string }) {
  const [s, setS] = useState<WebProtectionSettings | null>(null)
  const [saving, setSaving] = useState(false)
  const [allowedText, setAllowedText] = useState("")
  const [blockedText, setBlockedText] = useState("")
  const [uaText, setUaText] = useState("")
  const [ipText, setIpText] = useState("")

  useEffect(() => {
    getWebProtectionSettings()
      .then((d) => {
        setS(d)
        setAllowedText(listToText(d.allowed_paths))
        setBlockedText(listToText(d.blocked_paths))
        setUaText(listToText(d.ua_allowlist))
        setIpText(listToText(d.ip_allowlist))
      })
      .catch((e) => handleServerError(e))
  }, [])

  const set = (patch: Partial<WebProtectionSettings>) => setS((prev) => (prev ? { ...prev, ...patch } : prev))

  const save = async () => {
    if (!s) {return}
    setSaving(true)
    try {
      const res = await updateWebProtectionSettings({
        enabled: s.enabled,
        limit_per_second: Number(s.limit_per_second) || 0,
        burst: Number(s.burst) || 0,
        auto_ban: s.auto_ban,
        auto_ban_threshold_per_minute: Number(s.auto_ban_threshold_per_minute) || 0,
        auto_ban_minutes: Number(s.auto_ban_minutes) || 0,
        log_enabled: s.log_enabled,
        window_seconds: Number(s.window_seconds) || 0,
        allowed_paths: textToList(allowedText),
        blocked_paths: textToList(blockedText),
        ua_allowlist: textToList(uaText),
        ip_allowlist: textToList(ipText),
      })
      if (res.data?.success) { toast.success(t("Web protection settings saved")) }
    } catch (e) { handleServerError(e) } finally { setSaving(false) }
  }

  if (!s) {return null}
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("Web Protection")}</CardTitle>
        <p className="text-muted-foreground text-sm">{t("Web protection scope note")}</p>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-2">
          <Switch checked={s.enabled} onCheckedChange={(v) => set({ enabled: v })} />
          <Label>{t("Enabled")}</Label>
        </div>
        {/* B1-3 两套限流叠加说明：Web 防护（本页）与既有全局 Web 限流 */}
        <div className="rounded-md border bg-muted/40 p-3 text-xs text-muted-foreground">
          <p className="font-medium text-foreground">{t("Rate limit stacking note title")}</p>
          <ul className="mt-1 list-disc space-y-0.5 pl-4">
            <li>{t("Rate limit stacking web note")}</li>
            <li>{t("Rate limit stacking api note")}</li>
            <li>{t("Rate limit stacking login note")}</li>
            <li>{t("Rate limit stacking global note")}</li>
          </ul>
        </div>
        <div className="flex flex-wrap gap-2">
          {PRESETS.map((p) => (
            <Button key={p.id} type="button" variant="outline" size="sm"
              onClick={() => set({ limit_per_second: p.perSec, burst: p.burst })}>
              {t(p.label)}
            </Button>
          ))}
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <div><Label>{t("Requests per second per IP")}</Label><Input type="number" value={s.limit_per_second} onChange={(e) => set({ limit_per_second: Number(e.target.value) })} /></div>
          <div><Label>{t("Burst capacity")}</Label><Input type="number" value={s.burst} onChange={(e) => set({ burst: Number(e.target.value) })} /></div>
          <div><Label>{t("Auto-ban after rejects per minute")}</Label><Input type="number" value={s.auto_ban_threshold_per_minute} onChange={(e) => set({ auto_ban_threshold_per_minute: Number(e.target.value) })} /></div>
          <div><Label>{t("Auto-ban duration (minutes)")}</Label><Input type="number" value={s.auto_ban_minutes} onChange={(e) => set({ auto_ban_minutes: Number(e.target.value) })} /></div>
        </div>
        <div className="flex items-center gap-2"><Switch checked={s.auto_ban} onCheckedChange={(v) => set({ auto_ban: v })} /><Label>{t("Auto ban")}</Label></div>
        <div className="flex items-center gap-2"><Switch checked={s.log_enabled} onCheckedChange={(v) => set({ log_enabled: v })} /><Label>{t("Web request logging")}</Label></div>
        <div className="space-y-2">
          <Label htmlFor="wp-allowed-paths">{t("Allowed paths")}</Label>
          <Textarea id="wp-allowed-paths" value={allowedText} placeholder={t("Allowed paths placeholder")} onChange={(e) => setAllowedText(e.target.value)} />
          <p className="text-muted-foreground text-xs">{t("Allowed paths description")}</p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="wp-blocked-paths">{t("Blocked paths")}</Label>
          <Textarea id="wp-blocked-paths" value={blockedText} placeholder={t("Blocked paths placeholder")} onChange={(e) => setBlockedText(e.target.value)} />
          <p className="text-muted-foreground text-xs">{t("Blocked paths description")}</p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="wp-ua-allowlist">{t("User-Agent allowlist")}</Label>
          <Textarea id="wp-ua-allowlist" value={uaText} placeholder={t("User-Agent allowlist placeholder")} onChange={(e) => setUaText(e.target.value)} />
          <p className="text-muted-foreground text-xs">{t("User-Agent allowlist description")}</p>
        </div>
        <Button type="button" onClick={save} disabled={saving}>{saving ? t("Saving...") : t("Save")}</Button>
      </CardContent>
    </Card>
  )
}

export function WebProtectionPage() {
  const { t } = useTranslation()
  const [tab, setTab] = useState("settings")
  const tabBtn = (id: string, label: string) => (
    <Button type="button" variant={tab === id ? "default" : "outline"} onClick={() => setTab(id)}>{t(label)}</Button>
  )
  return (
    <div className="space-y-4 p-4 sm:p-6">
      <ServerStatsCard t={t} />
      <div className="flex flex-wrap items-center gap-2">
        {tabBtn("settings", "Web Protection")}
        {tabBtn("logs", "Web request logs")}
        {tabBtn("banned", "Banned IPs")}
      </div>
      {tab === "settings" ? <SettingsTab t={t} /> : null}
      {tab === "logs" ? <WebRequestLogsTable /> : null}
      {tab === "banned" ? <BannedIPsTable /> : null}
    </div>
  )
}
