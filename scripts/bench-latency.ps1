# bench-latency.ps1 - paired latency: direct mock upstream vs gateway (sequential N + concurrent)
param(
    [Parameter(Mandatory=$true)][string]$DirectUrl,
    [Parameter(Mandatory=$true)][string]$GatewayUrl,
    [Parameter(Mandatory=$true)][string]$Bearer,
    [int]$N = 60
)
$ErrorActionPreference = "Stop"
$ts = Get-Date -Format "yyyyMMdd-HHmmss"
$body = '{"model":"bench-model","messages":[{"role":"user","content":"hi"}],"stream":false,"max_tokens":2}'
$headers = @{ Authorization = "Bearer $Bearer"; "Content-Type" = "application/json" }

function Invoke-One([string]$url) {
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    try { $null = Invoke-RestMethod -Uri $url -Method Post -Headers $headers -Body $body -TimeoutSec 30 } catch {}
    $sw.Stop()
    return $sw.Elapsed.TotalMilliseconds
}

function Get-Summary([double[]]$ms) {
    $sorted = @($ms | Sort-Object)
    $n = $sorted.Count
    function Get-Pct([double]$q) {
        $idx = [Math]::Min([int][Math]::Floor($q * $n), $n - 1)
        return $sorted[$idx]
    }
    return [ordered]@{
        count = $n
        min = [Math]::Round($sorted[0],2)
        p50 = [Math]::Round((Get-Pct 0.50),2)
        p90 = [Math]::Round((Get-Pct 0.90),2)
        p95 = [Math]::Round((Get-Pct 0.95),2)
        max = [Math]::Round($sorted[$n-1],2)
    }
}

function Get-Conc([string]$url, [int]$c) {
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $pb = @{c=$c; target=$url; h=$headers; b=$body}
    $jobs = 1..$c | ForEach-Object {
        Start-ThreadJob -ArgumentList $url,$headers,$body -ScriptBlock {
            param($u,$hd,$bd)
            try { $null = Invoke-RestMethod -Uri $u -Method Post -Headers $hd -Body $bd -TimeoutSec 60 } catch {}
        }
    }
    $jobs | Wait-Job | Out-Null
    $jobs | Remove-Job
    $sw.Stop()
    return $sw.Elapsed.TotalMilliseconds / $c
}

# sequential direct vs gateway
$seqDirect = @(); $seqGw = @()
for ($i=0; $i -lt $N; $i++) { $seqDirect += Invoke-One $DirectUrl }
"sequential direct done"
for ($i=0; $i -lt $N; $i++) { $seqGw += Invoke-One $GatewayUrl }
"sequential gateway done"

# concurrent 20 / 50 (total wall / count)
$c20d = Get-Conc $DirectUrl 20; $c20g = Get-Conc $GatewayUrl 20
$c50d = Get-Conc $DirectUrl 50; $c50g = Get-Conc $GatewayUrl 50

$res = [ordered]@{
    ts = $ts; direct=$DirectUrl; gateway=$GatewayUrl; n=$N
    sequential = [ordered]@{ direct=(Get-Summary $seqDirect); gateway=(Get-Summary $seqGw); overhead_p50 = [Math]::Round((((Get-Summary $seqGw).p50 - (Get-Summary $seqDirect).p50)),2) }
    concurrent20 = [ordered]@{ direct=$c20d; gateway=$c20g; gateway_overhead_s = [Math]::Round(($c20g-$c20d),2) }
    concurrent50 = [ordered]@{ direct=$c50d; gateway=$c50g; gateway_overhead_s = [Math]::Round(($c50g-$c50d),2) }
}
$respDir = Join-Path $PSScriptRoot ".." "计划书" "e2e-evidence"
New-Item -ItemType Directory -Path $respDir -Force | Out-Null
$file = Join-Path $respDir "paired-latency-bench-$ts.json"
$res | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $file -Encoding UTF8
Write-Host "written: $file"