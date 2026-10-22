param(
    [Parameter(Mandatory = $true)]
    [string] $EnvironmentFile,

    [string] $ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'Load-EnvironmentFile.ps1')
$ProjectRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
$backup = Join-Path $ProjectRoot 'algentra-backup.exe'
if (-not (Test-Path -LiteralPath $backup -PathType Leaf)) { throw "Executable backup Go tidak ditemukan: $backup. Build dengan: go build -o algentra-backup.exe ./cmd/backup-database" }
Import-EnvironmentFile -Path $EnvironmentFile

if ($env:MIGRATION_DATABASE_URL) { throw 'Backup production tidak boleh menerima MIGRATION_DATABASE_URL.' }
[Environment]::SetEnvironmentVariable('MIGRATION_DATABASE_URL', $null, 'Process')
[Environment]::SetEnvironmentVariable('SUPABASE_DEV_PROJECT_REF', $null, 'Process')
$env:APP_ENV = 'production'
$env:PROJECT_ROOT = $ProjectRoot
Set-Location -LiteralPath $ProjectRoot
& $backup
exit $LASTEXITCODE
