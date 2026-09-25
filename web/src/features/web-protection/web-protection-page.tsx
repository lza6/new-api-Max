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

import { useCallback, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Switch } from "@/components/ui/switch"
import { Badge } from "@/components/ui/badge"
import { Checkbox } from "@/components/ui/checkbox"
import { handleServerError } from "@/lib/handle-server-error"

import {
  type BannedIPRow,
  type WebProtectionSettings,
  type WebRequestLogRow,
  banIP,
  formatBytes,
  getBannedIPs,
  getServerStats,
  getWebProtectionSettings,
  getWebRequestLogDetail,
  getWebRequestLogs,
  unbanIP,
  unbanIPsBatch,
  type ServerStats,
  updateWebProtectionSettings,
} from "./api"

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

  useEffect(() => {
    getWebProtectionSettings()
      .then((d) => {
        setS(d)
        setAllowedText(listToText(d.allowed_paths))
        setBlockedText(listToText(d.blocked_paths))
        setUaText(listToText(d.ua_allowlist))
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

function LogsTab({ t }: { t: (k: string) => string }) {
  const [rows, setRows] = useState<WebRequestLogRow[]>([])
  const [total, setTotal] = useState(0)
  const [filter, setFilter] = useState("")
  const [sort, setSort] = useState("rate")
  const [order, setOrder] = useState("desc")
  const [detail, setDetail] = useState<Record<string, string[]>>({})
  const [banMinutes, setBanMinutes] = useState(1440)

  const load = useCallback(async () => {
    try {
      const d = await getWebRequestLogs({ page: 1, size: 50, ip: filter || undefined, sort, order })
      setRows(d.items); setTotal(d.total);
    } catch (e) { handleServerError(e) }
  }, [filter, sort, order])
  useEffect(() => { void load() }, [load])

  const toggleDetail = async (row: WebRequestLogRow) => {
    if (detail[row.ip]) { const n = { ...detail }; delete n[row.ip]; setDetail(n); return }
    try {
      const d = await getWebRequestLogDetail(row.ip)
      setDetail((prev) => ({ ...prev, [row.ip]: (d.items || []).map((x: { path: string; method: string; status: number; request_count: number; bytes_sent: number; window_start: number }) => x.method + " " + x.path + " [" + x.status + "] x" + x.request_count + " " + formatBytes(x.bytes_sent) + " @" + new Date(x.window_start * 1000).toLocaleString()) }))
    } catch (e) { handleServerError(e) }
  }

  const doBan = async (target: string) => {
    try { await banIP(target, Number(banMinutes) || 1440, "admin_ban"); toast.success(t("IP banned")); void load(); } catch (e) { handleServerError(e) }
  }

    const sortBtn = (key: string, label: string) => {
    let indicator: string | null = null
    if (sort === key) {
      indicator = order === "desc" ? " ↓" : " ↑"
    }
    return (
      <Button type="button" size="sm" variant={sort === key ? "default" : "outline"}
        onClick={() => { if (sort === key) {setOrder(order === "desc" ? "asc" : "desc");} else { setSort(key); setOrder("desc") } }}
        className="text-xs">
        {t(label)}{indicator}
      </Button>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("Web request logs")} ({total})</CardTitle>
        <div className="flex flex-wrap items-center gap-2">
          {sortBtn("rate", "Rate")}{sortBtn("count", "Count")}{sortBtn("bytes", "Bandwidth")}
          <Input placeholder={t("Filter by IP")} value={filter} onChange={(e) => setFilter(e.target.value)} className="max-w-52" />
          <Button type="button" size="sm" variant="outline" onClick={() => void load()}>{t("Refresh")}</Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        {rows.map((r) => (
          <div key={r.ip} className="rounded-md border p-3">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="font-mono">{r.ip}</span>
              <span>{r.request_count} req</span>
              <span>{r.rate_per_second.toFixed(2)}/s</span>
              <span>{formatBytes(r.bytes_total)}</span>
              <span className="text-muted-foreground">{new Date(r.last_request_at * 1000).toLocaleString()}</span>
              {r.banned ? <Badge variant="destructive">{t("Banned")}</Badge> : null}
              <div className="ml-auto flex items-center gap-2"><Label className="text-xs">{t("Ban minutes")}</Label>
                <Input type="number" placeholder={t("minutes")} value={banMinutes} onChange={(e) => setBanMinutes(Number(e.target.value))} className="w-24 h-8" />
                <Button type="button" size="sm" variant="destructive" onClick={() => void doBan(r.ip)}>{t("Ban IP")}</Button>
                <Button type="button" size="sm" variant="outline" onClick={() => void toggleDetail(r)}>{detail[r.ip] ? t("Hide") : t("Details")}</Button>
              </div>
            </div>
            {detail[r.ip] ? (
              <ul className="mt-2 space-y-1 border-t pt-2 text-xs text-muted-foreground">
                {detail[r.ip].map((d, i) => <li key={i} className="break-all">{d}</li>)}
              </ul>
            ) : null}
          </div>
        ))}
        {rows.length === 0 ? <p className="text-muted-foreground text-sm">{t("No web request logs yet")}</p> : null}
      </CardContent>
    </Card>
  )
}

function BannedTab({ t }: { t: (k: string) => string }) {
  const [rows, setRows] = useState<BannedIPRow[]>([])
  const [minutes, setMinutes] = useState(1440)
  const [reason, setReason] = useState("admin_ban")
  const [ip, setIp] = useState("")
  const [selected, setSelected] = useState<Set<string>>(new Set())

  const load = async () => {
    try { const d = await getBannedIPs(1, 50); setRows(d.items || []); setSelected(new Set()) } catch (e) { handleServerError(e) }
  }
  useEffect(() => { void load() }, [])

  const toggleSelect = (target: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(target)) {next.delete(target)}
      else {next.add(target)}
      return next
    })
  }
  const toggleAll = () => {
    setSelected((prev) => prev.size === rows.length ? new Set() : new Set(rows.map((r) => r.ip)))
  }

  const doUnban = async (target: string) => {
    try { await unbanIP(target); toast.success(t("IP unbanned")); void load() } catch (e) { handleServerError(e) }
  }
  // B1-3 批量解封：勾选多个 IP 一键解封。
  const doUnbanSelected = async () => {
    if (selected.size === 0) {return}
    try {
      await unbanIPsBatch([...selected])
      toast.success(t("IPs unbanned"))
      void load()
    } catch (e) { handleServerError(e) }
  }
  const doBan = async () => {
    if (!ip) {return}
    try { await banIP(ip, Number(minutes) || 1440, reason); toast.success(t("IP banned")); setIp(""); void load() } catch (e) { handleServerError(e) }
  }

  return (
    <Card>
      <CardHeader><CardTitle>{t("Banned IPs")}</CardTitle></CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-end gap-2">
          <div><Label>{t("IP")}</Label><Input value={ip} onChange={(e) => setIp(e.target.value)} className="w-44" /></div>
          <div><Label>{t("Minutes")}</Label><Input type="number" value={minutes} onChange={(e) => setMinutes(Number(e.target.value))} className="w-24" /></div>
          <div><Label>{t("Reason")}</Label><Input value={reason} onChange={(e) => setReason(e.target.value)} className="w-40" /></div>
          <Button type="button" onClick={() => void doBan()}>{t("Ban IP")}</Button>
        </div>
        <div className="flex items-center justify-between">
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={rows.length > 0 && selected.size === rows.length} onCheckedChange={() => toggleAll()} />
            <span>{t("Select all")} ({selected.size}/{rows.length})</span>
          </label>
          <Button type="button" size="sm" variant="destructive" disabled={selected.size === 0} onClick={() => void doUnbanSelected()}>
            {t("Unban selected")}
          </Button>
        </div>
        <div className="space-y-2">
          {rows.map((r) => (
            <div key={r.id} className="flex flex-wrap items-center gap-2 rounded-md border p-3 text-sm">
              <Checkbox checked={selected.has(r.ip)} onCheckedChange={() => toggleSelect(r.ip)} aria-label={r.ip} />
              <span className="font-mono">{r.ip}</span>
              <span className="text-muted-foreground">{r.reason}</span>
              <span className="text-muted-foreground">by {r.banned_by}</span>
              <span className="text-muted-foreground">{new Date(r.banned_at * 1000).toLocaleString()}</span>
              <span className="text-muted-foreground">{r.expires_at ? "expires " + new Date(r.expires_at * 1000).toLocaleString() : t("Permanent")}</span>
              <div className="ml-auto"><Button type="button" size="sm" variant="outline" onClick={() => void doUnban(r.ip)}>{t("Unban")}</Button></div>
            </div>
          ))}
          {rows.length === 0 ? <p className="text-muted-foreground text-sm">{t("No banned IPs")}</p> : null}
        </div>
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
      {tab === "logs" ? <LogsTab t={t} /> : null}
      {tab === "banned" ? <BannedTab t={t} /> : null}
    </div>
  )
}
