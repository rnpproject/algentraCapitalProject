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
$backend = Join-Path $ProjectRoot 'algentra-backend.exe'
if (-not (Test-Path -LiteralPath $backend -PathType Leaf)) { throw "Binary Go Core API tidak ditemukan: $backend. Build sebelum memasang service." }
if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    throw "Service $ServiceName sudah terdaftar. Installer tidak menimpa service yang ada; verifikasi dan hapus instalasi parsial sebelum mencoba lagi."
}

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
Set-NssmSetting @($ServiceName, 'Start', 'SERVICE_DISABLED')
Set-NssmSetting @($ServiceName, 'AppDirectory', $ProjectRoot)
$parameters = @(
    '-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File',
    (Quote-NssmArgument $startScript), '-ProjectRoot', (Quote-NssmArgument $ProjectRoot),
    '-EnvironmentFile', (Quote-NssmArgument $EnvironmentFile)
) -join ' '
Set-NssmSetting @($ServiceName, 'AppParameters', $parameters)
Set-NssmSetting @($ServiceName, 'DisplayName', 'Algentra Capital Core API')
Set-NssmSetting @($ServiceName, 'Description', 'Go Core API; schema must be provisioned before release.')
Set-NssmSetting @($ServiceName, 'AppExit', 'Default', 'Restart')
Set-NssmSetting @($ServiceName, 'AppRestartDelay', '5000')
Set-NssmSetting @($ServiceName, 'AppStdout', (Join-Path $logDirectory 'stdout.log'))
Set-NssmSetting @($ServiceName, 'AppStderr', (Join-Path $logDirectory 'stderr.log'))
Set-NssmSetting @($ServiceName, 'AppRotateFiles', '1')
Set-NssmSetting @($ServiceName, 'AppRotateBytes', '10485760')
Set-NssmSetting @($ServiceName, 'ObjectName', $serviceAccount, '')

& sc.exe sidtype $ServiceName unrestricted | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal mengaktifkan service SID.' }
$readOnlyPaths = @(
	$backend,
    $PSScriptRoot
)
foreach ($path in $readOnlyPaths) {
    if (Test-Path -LiteralPath $path) {
        if ((Get-Item -LiteralPath $path).PSIsContainer) {
            & icacls.exe $path /grant "${serviceAccount}:(OI)(CI)RX" /T /C | Out-Null
        } else {
            & icacls.exe $path /grant "${serviceAccount}:R" | Out-Null
        }
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
& icacls.exe $logDirectory /grant "${serviceAccount}:(OI)(CI)M" /T /C | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal memberi hak tulis pada folder log.' }

Set-NssmSetting @($ServiceName, 'Start', 'SERVICE_AUTO_START')

Write-Output "Service $ServiceName terpasang dengan akun virtual $serviceAccount. Pastikan ACL environment file hanya memberi akses kepada administrator dan akun service."
Write-Output 'Jalankan service setelah DNS, kredensial, schema production, dan health check diverifikasi.'
