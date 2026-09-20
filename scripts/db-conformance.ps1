# db-conformance.ps1 — run the three-database conformance contract tests.
#
# Verifies SQLite + MySQL + PostgreSQL against real instances:
#   1. Starts local MySQL/PostgreSQL services when they are stopped.
#   2. Creates the dedicated conformance databases (idempotent).
#   3. Sets TEST_MYSQL_DSN / TEST_POSTGRES_DSN and runs the conformance tests.
#   4. Reports PASS/FAIL/SKIP per dialect.
#
# Requirements (choose one path):
#   - Local services: MySQL + PostgreSQL installed as Windows services, plus
#     mysql.exe / psql.exe on PATH or standard install locations.
#   - Docker:  -Docker  (expects `docker compose` to be available)
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts/db-conformance.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/db-conformance.ps1 -Docker
#
# Options:
#   -MysqlRootUser  default "root"
#   -PgSuperUser    default "postgres"
param(
  [switch]$Docker,
  [string]$MysqlRootUser = "root",
  [string]$PgSuperUser = "postgres"
)

$ErrorActionPreference = "Stop"
$ConformanceDB = "newapi_conformance_test"
$mysqlClient = "C:\tools\mysql\current\bin\mysql.exe"
$psqlClient  = "C:\Program Files\PostgreSQL\16\bin\psql.exe"

function Find-Client([string]$name, [string]$candidate) {
  if ($candidate -and (Test-Path $candidate)) { return $candidate }
  $cmd = Get-Command $name -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  throw "Cannot locate $name client. Install it or pass its path."
  return ""
}

Write-Host "=== DB conformance (SQLite + MySQL + PostgreSQL) ===" -ForegroundColor Cyan

if ($Docker) {
  Write-Host "[docker] starting mysql + postgres compose services..."
  if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw "docker is required with -Docker" }
  docker compose -f docker-compose.dev.yml up -d mysql postgres 2>&1 | Out-Host
  Start-Sleep -Seconds 8
  $mysql = "root@tcp(127.0.0.1:3306)/${ConformanceDB}?charset=utf8mb4&parseTime=true&loc=Local"
  $pg    = "postgres://${PgSuperUser}@127.0.0.1:5432/${ConformanceDB}"
  $env:TEST_MYSQL_DSN = $mysql
  $env:TEST_POSTGRES_DSN = $pg
  try {
    docker compose -f docker-compose.dev.yml exec -T mysql mysql -u root -e "CREATE DATABASE IF NOT EXISTS $ConformanceDB CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" 2>&1 | Out-Host
  } catch { Write-Host "[docker] mysql create db skipped: $_" -ForegroundColor Yellow }
  try {
    docker compose -f docker-compose.dev.yml exec -T postgres psql -U root -d postgres -c "CREATE DATABASE $ConformanceDB;" 2>&1 | Out-Host
  } catch { Write-Host "[docker] postgres create db skipped: $_" -ForegroundColor Yellow }
} else {
  $mysql = Find-Client "mysql" $mysqlClient
  $psql  = Find-Client "psql" $psqlClient

  # Start stopped services.
  foreach ($svc in @("MySQL", "postgresql-x64-16")) {
    $s = Get-Service -Name $svc -ErrorAction SilentlyContinue
    if ($s -and $s.Status -ne "Running") {
      Write-Host "[svc] starting $svc ..."
      Start-Service -Name $svc
    }
  }
  Start-Sleep -Seconds 3

  # Create conformance databases (idempotent).
  & $mysql -u $MysqlRootUser -e "CREATE DATABASE IF NOT EXISTS $ConformanceDB CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" 2>&1 | Out-Host
  & $psql -U $PgSuperUser -h localhost -p 5432 -d postgres -c "SELECT 1 FROM pg_database WHERE datname='$ConformanceDB'" | Out-Null 2>&1
  if ($LASTEXITCODE -ne 0) { & $psql -U $PgSuperUser -h localhost -p 5432 -d postgres -c "CREATE DATABASE $ConformanceDB" 2>&1 | Out-Host }

  $env:TEST_MYSQL_DSN = "${MysqlRootUser}@tcp(127.0.0.1:3306)/${ConformanceDB}?charset=utf8mb4&parseTime=true&loc=Local"
  $env:TEST_POSTGRES_DSN = "postgres://${PgSuperUser}@127.0.0.1:5432/${ConformanceDB}"
}

Write-Host ""
Write-Host "[run] go test ./model/ -run TestDBConformance -v" -ForegroundColor Cyan
Write-Host "  TEST_MYSQL_DSN    = $env:TEST_MYSQL_DSN"
Write-Host "  TEST_POSTGRES_DSN = $env:TEST_POSTGRES_DSN"
Write-Host ""

$output = & go test ./model/ -run "TestDBConformance" -v -count=1 2>&1
$output | Out-Host
$joined = $output -join "`n"
$fail = [regex]::Matches($joined, "--- FAIL").Count
$skip = [regex]::Matches($joined, "--- SKIP").Count
$pass = [regex]::Matches($joined, "--- PASS").Count

Write-Host ""
Write-Host "=== Results: PASS=$pass FAIL=$fail SKIP=$skip ===" -ForegroundColor Cyan
if ($fail -gt 0) {
  Write-Host "DB conformance FAILED" -ForegroundColor Red
  exit 1
}
Write-Host "DB conformance OK" -ForegroundColor Green
exit 0