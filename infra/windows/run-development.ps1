param([int] $Port = 8000)

$ErrorActionPreference = 'Stop'
$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) { throw 'Go tidak ditemukan di PATH.' }

$env:APP_ENV = 'development'
Remove-Item Env:MIGRATION_DATABASE_URL -ErrorAction SilentlyContinue
Set-Location -LiteralPath $ProjectRoot
if (-not $env:HTTP_ADDR) { $env:HTTP_ADDR = "127.0.0.1:$Port" }
& $go.Source run ./cmd/algentra-backend
exit $LASTEXITCODE
