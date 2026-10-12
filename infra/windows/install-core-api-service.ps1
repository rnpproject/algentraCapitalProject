param(
    [string] $ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path,

    [Parameter(Mandatory = $true)]
    [string] $EnvironmentFile,

    [Parameter(Mandatory = $true)]
    [string] $NssmPath,

    [string] $ServiceName = 'AlgentraCoreApi'
)

$ErrorActionPreference = 'Stop'
$currentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
$currentPrincipal = [Security.Principal.WindowsPrincipal]::new($currentIdentity)
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Jalankan installer dari PowerShell administrator agar ACL dan service Windows dapat dikonfigurasi.'
}
if (-not (Test-Path -LiteralPath $NssmPath -PathType Leaf)) { throw "nssm.exe tidak ditemukan: $NssmPath" }
if (-not (Test-Path -LiteralPath $EnvironmentFile -PathType Leaf)) { throw "Environment file tidak ditemukan: $EnvironmentFile" }
$ProjectRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
$EnvironmentFile = (Resolve-Path -LiteralPath $EnvironmentFile).Path
$python = Join-Path $ProjectRoot '.venv\Scripts\python.exe'
if (-not (Test-Path -LiteralPath $python -PathType Leaf)) { throw "Virtual environment tidak ditemukan: $python" }

$serviceAccount = "NT SERVICE\$ServiceName"
$powershellExe = (Get-Command powershell.exe -ErrorAction Stop).Source
$startScript = Join-Path $PSScriptRoot 'start-core-api.ps1'
$logDirectory = Join-Path $ProjectRoot 'logs\core-api'
New-Item -ItemType Directory -Path $logDirectory -Force | Out-Null

function Quote-NssmArgument([string] $Value) {
    return '"' + $Value.Replace('"', '\"') + '"'
}

function Set-NssmSetting([string[]] $Arguments) {
    & $NssmPath set @Arguments | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "NSSM gagal mengatur '$($Arguments[1])'." }
}

& $NssmPath install $ServiceName $powershellExe
if ($LASTEXITCODE -ne 0) { throw 'NSSM gagal mendaftarkan service.' }
Set-NssmSetting @($ServiceName, 'AppDirectory', $ProjectRoot)
$parameters = @(
    '-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File',
    (Quote-NssmArgument $startScript), '-ProjectRoot', (Quote-NssmArgument $ProjectRoot),
    '-EnvironmentFile', (Quote-NssmArgument $EnvironmentFile)
) -join ' '
Set-NssmSetting @($ServiceName, 'AppParameters', $parameters)
Set-NssmSetting @($ServiceName, 'DisplayName', 'Algentra Capital Core API')
Set-NssmSetting @($ServiceName, 'Description', 'FastAPI core service; schema must be provisioned before release.')
Set-NssmSetting @($ServiceName, 'Start', 'SERVICE_AUTO_START')
Set-NssmSetting @($ServiceName, 'AppExit', 'Default', 'Restart')
Set-NssmSetting @($ServiceName, 'AppRestartDelay', '5000')
Set-NssmSetting @($ServiceName, 'AppStdout', (Join-Path $logDirectory 'stdout.log'))
Set-NssmSetting @($ServiceName, 'AppStderr', (Join-Path $logDirectory 'stderr.log'))
Set-NssmSetting @($ServiceName, 'AppRotateFiles', '1')
Set-NssmSetting @($ServiceName, 'AppRotateBytes', '10485760')
Set-NssmSetting @($ServiceName, 'AppKillProcessTree', '1')
Set-NssmSetting @($ServiceName, 'ObjectName', $serviceAccount, '')

& sc.exe sidtype $ServiceName unrestricted | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal mengaktifkan service SID.' }
$readOnlyPaths = @(
    (Join-Path $ProjectRoot '.venv'),
    (Join-Path $ProjectRoot 'apps\core-api'),
    (Join-Path $ProjectRoot 'mt5\experts'),
    $PSScriptRoot
)
foreach ($path in $readOnlyPaths) {
    if (Test-Path -LiteralPath $path) {
        & icacls.exe $path /grant "${serviceAccount}:(OI)(CI)RX" /T /C | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak baca pada $path." }
    }
}
$directory = Get-Item -LiteralPath (Split-Path -Parent $ProjectRoot)
while ($directory) {
    & icacls.exe $directory.FullName /grant "${serviceAccount}:(X)" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak traversal pada $($directory.FullName)." }
    $parent = Split-Path -Parent $directory.FullName
    if (-not $parent -or $parent -eq $directory.FullName) { break }
    $directory = Get-Item -LiteralPath $parent
}
& icacls.exe $ProjectRoot /grant "${serviceAccount}:(RX)" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal memberi hak traversal pada folder proyek.' }
$environmentDirectory = Split-Path -Parent $EnvironmentFile
& icacls.exe $environmentDirectory /grant "${serviceAccount}:(X)" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal memberi hak traversal pada direktori environment file.' }
& icacls.exe $EnvironmentFile /inheritance:r /grant 'SYSTEM:F' 'BUILTIN\Administrators:F' "${serviceAccount}:R" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL environment file.' }
$dataDirectory = Join-Path $ProjectRoot 'backend\data'
New-Item -ItemType Directory -Path $dataDirectory -Force | Out-Null
& icacls.exe (Split-Path -Parent $dataDirectory) /grant "${serviceAccount}:(X)" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal memberi hak traversal pada folder backend.' }
& icacls.exe $dataDirectory /grant "${serviceAccount}:(OI)(CI)M" /T /C | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal memberi hak tulis pada backend\data.' }
& icacls.exe $logDirectory /grant "${serviceAccount}:(OI)(CI)M" /T /C | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal memberi hak tulis pada folder log.' }

Write-Output "Service $ServiceName terpasang dengan akun virtual $serviceAccount. Pastikan ACL environment file hanya memberi akses kepada administrator dan akun service."
Write-Output 'Jalankan service setelah DNS, kredensial, schema production, dan health check diverifikasi.'
