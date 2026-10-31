param(
    [Parameter(Mandatory = $true)]
    [string] $ProjectRoot
)

$ErrorActionPreference = 'Stop'
$resolvedRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
. (Join-Path $PSScriptRoot 'Clear-ApplicationEnvironment.ps1')
Clear-AlgentraApplicationEnvironment -Preserve @('MT5_BROKER_DIRECTORY_URL')
$webRoot = Join-Path $resolvedRoot 'apps\web'
$nextCli = Join-Path $webRoot 'node_modules\next\dist\bin\next'
if (-not (Test-Path -LiteralPath $nextCli -PathType Leaf)) {
    throw "Next.js belum terpasang di $webRoot. Jalankan npm ci dan npm run build."
}
$bundledNode = Join-Path (Split-Path -Parent $resolvedRoot) 'tools\node\node.exe'
if (Test-Path -LiteralPath $bundledNode -PathType Leaf) {
    $node = $bundledNode
} else {
    $node = (Get-Command node.exe -ErrorAction Stop).Source
}
Set-Location -LiteralPath $webRoot
$env:NODE_ENV = 'production'
& $node $nextCli start --hostname 127.0.0.1 --port 3000
exit $LASTEXITCODE
