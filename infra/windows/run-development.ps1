param([int] $Port = 8000)

$ErrorActionPreference = 'Stop'
$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$python = Join-Path $ProjectRoot '.venv\Scripts\python.exe'
if (-not (Test-Path -LiteralPath $python -PathType Leaf)) { throw 'Buat .venv dan pasang apps/core-api/requirements.txt terlebih dahulu.' }

$env:APP_ENV = 'development'
Remove-Item Env:DATABASE_URL -ErrorAction SilentlyContinue
Remove-Item Env:MIGRATION_DATABASE_URL -ErrorAction SilentlyContinue
Set-Location -LiteralPath $ProjectRoot
& $python -m uvicorn app.main:app --app-dir apps/core-api --host 127.0.0.1 --port $Port --proxy-headers --forwarded-allow-ips=127.0.0.1 --reload --reload-dir apps/core-api
exit $LASTEXITCODE
