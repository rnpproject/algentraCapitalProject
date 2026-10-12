$ErrorActionPreference = 'Stop'
$WorkerRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$ProjectRoot = (Resolve-Path (Join-Path $WorkerRoot '..\..')).Path
$python = Join-Path $ProjectRoot '.venv\Scripts\python.exe'
if (-not (Test-Path -LiteralPath $python -PathType Leaf)) { throw "Virtual environment tidak ditemukan: $python" }

$env:PYTHONPATH = (Join-Path $WorkerRoot 'src') + $(if ($env:PYTHONPATH) { [IO.Path]::PathSeparator + $env:PYTHONPATH } else { '' })
Set-Location -LiteralPath $WorkerRoot
& $python -m copytrade.launcher config/launcher.json
exit $LASTEXITCODE
