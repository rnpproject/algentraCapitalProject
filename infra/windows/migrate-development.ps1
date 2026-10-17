$ErrorActionPreference = 'Stop'
$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Go belum terpasang atau tidak ada di PATH.' }

Remove-Item Env:APP_ENV -ErrorAction SilentlyContinue
Remove-Item Env:DATABASE_URL -ErrorAction SilentlyContinue
Remove-Item Env:MIGRATION_DATABASE_URL -ErrorAction SilentlyContinue
Set-Location -LiteralPath $ProjectRoot
& go run ./cmd/migrate-development
exit $LASTEXITCODE
