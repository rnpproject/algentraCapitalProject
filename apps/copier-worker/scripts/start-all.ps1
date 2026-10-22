$ErrorActionPreference = 'Stop'
$WorkerRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$ProjectRoot = (Resolve-Path (Join-Path $WorkerRoot '..\..')).Path
$launcher = Join-Path $ProjectRoot 'algentra-copier-launcher.exe'
if (-not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
    throw "Binary Go launcher tidak ditemukan: $launcher. Build dari root repo dengan go build -o algentra-copier-launcher.exe ./cmd/copier-launcher."
}
Set-Location -LiteralPath $ProjectRoot
& $launcher (Join-Path $WorkerRoot 'config/launcher.json')
exit $LASTEXITCODE
