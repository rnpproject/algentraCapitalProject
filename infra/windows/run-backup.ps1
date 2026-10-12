param(
    [Parameter(Mandatory = $true)]
    [string] $EnvironmentFile,

    [string] $ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'Load-EnvironmentFile.ps1')
$ProjectRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
$python = Join-Path $ProjectRoot '.venv\Scripts\python.exe'
if (-not (Test-Path -LiteralPath $python -PathType Leaf)) { throw "Virtual environment tidak ditemukan: $python" }
Import-EnvironmentFile -Path $EnvironmentFile

if ($env:MIGRATION_DATABASE_URL) { throw 'Backup production tidak boleh menerima MIGRATION_DATABASE_URL.' }
[Environment]::SetEnvironmentVariable('MIGRATION_DATABASE_URL', $null, 'Process')
[Environment]::SetEnvironmentVariable('SUPABASE_DEV_PROJECT_REF', $null, 'Process')
$env:APP_ENV = 'production'
Set-Location -LiteralPath (Join-Path $ProjectRoot 'apps\core-api')
& $python -m app.backup_database
exit $LASTEXITCODE
