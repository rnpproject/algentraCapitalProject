param(
    [Parameter(Mandatory = $true)]
    [string] $ProjectRoot,

    [Parameter(Mandatory = $true)]
    [string] $EnvironmentFile
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'Load-EnvironmentFile.ps1')
$resolvedRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
Import-EnvironmentFile -Path $EnvironmentFile

if ($env:MIGRATION_DATABASE_URL) {
    throw 'Production tidak boleh menerima MIGRATION_DATABASE_URL.'
}
[Environment]::SetEnvironmentVariable('MIGRATION_DATABASE_URL', $null, 'Process')
[Environment]::SetEnvironmentVariable('SUPABASE_DEV_PROJECT_REF', $null, 'Process')
$env:APP_ENV = 'production'

$python = Join-Path $resolvedRoot '.venv\Scripts\python.exe'
if (-not (Test-Path -LiteralPath $python -PathType Leaf)) {
    throw "Python environment tidak ditemukan: $python"
}

Set-Location -LiteralPath $resolvedRoot
& $python -m uvicorn app.main:app --app-dir apps/core-api --host 127.0.0.1 --port 8000 --workers 1 --proxy-headers --forwarded-allow-ips=127.0.0.1
exit $LASTEXITCODE
