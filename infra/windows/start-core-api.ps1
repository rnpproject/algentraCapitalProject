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

$backend = Join-Path $resolvedRoot 'algentra-backend.exe'
if (-not (Test-Path -LiteralPath $backend -PathType Leaf)) {
    throw "Binary Go Core API tidak ditemukan: $backend. Build dengan go build terlebih dahulu."
}

Set-Location -LiteralPath $resolvedRoot
& $backend
exit $LASTEXITCODE
