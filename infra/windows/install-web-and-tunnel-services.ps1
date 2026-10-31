param(
    [string] $ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path,
    [Parameter(Mandatory = $true)] [string] $Domain,
    [Parameter(Mandatory = $true)] [string] $NssmPath,
    [Parameter(Mandatory = $true)] [string] $CloudflaredPath,
    [Parameter(Mandatory = $true)] [string] $TunnelTokenPath
)

$ErrorActionPreference = 'Stop'
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Jalankan installer sebagai Administrator.' }
foreach ($path in @($NssmPath, $CloudflaredPath, $TunnelTokenPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "File tidak ditemukan: $path" }
}
$ProjectRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
$webRoot = Join-Path $ProjectRoot 'apps\web'
if (-not (Test-Path -LiteralPath (Join-Path $webRoot '.next\BUILD_ID') -PathType Leaf)) { throw 'Build Next.js tidak ditemukan.' }
$logRoot = Join-Path $ProjectRoot 'logs'
$webLogs = Join-Path $logRoot 'web'
$tunnelLogs = Join-Path $logRoot 'cloudflared'
foreach ($path in @($webLogs, $tunnelLogs)) { New-Item -ItemType Directory -Path $path -Force | Out-Null }
$powershellExe = (Get-Command powershell.exe -ErrorAction Stop).Source
$webScript = Join-Path $PSScriptRoot 'start-web.ps1'
$webAccount = 'NT SERVICE\AlgentraWeb'
$tunnelAccount = 'NT SERVICE\AlgentraCloudflared'

function Set-NssmSetting([string[]] $Arguments) {
    & $NssmPath set @Arguments | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "NSSM gagal mengatur '$($Arguments[1])'." }
}
function Prepare-NssmService([string] $Name, [string] $Application, [string[]] $Parameters) {
    $service = Get-Service -Name $Name -ErrorAction SilentlyContinue
    if ($service) {
        $nativeService = Get-CimInstance Win32_Service -Filter "Name='$Name'" -ErrorAction Stop
        $expectedCommand = '(?i)nssm\.exe.*//RS//' + [regex]::Escape($Name) + '(?:\s|$)'
        if (-not $nativeService -or $nativeService.PathName -notmatch $expectedCommand) { throw "Service $Name sudah ada tetapi bukan service NSSM Algentra." }
        if ($service.Status -ne 'Stopped') {
            Stop-Service $Name -Force
            $service.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
        }
        return
    }
    & $NssmPath install $Name $Application @Parameters | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "NSSM gagal mendaftarkan $Name." }
}

Prepare-NssmService 'AlgentraWeb' $powershellExe @()
Set-NssmSetting @('AlgentraWeb', 'Start', 'SERVICE_AUTO_START')
Set-NssmSetting @('AlgentraWeb', 'AppDirectory', $ProjectRoot)
Set-NssmSetting @('AlgentraWeb', 'AppParameters', "-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$webScript`" -ProjectRoot `"$ProjectRoot`"")
Set-NssmSetting @('AlgentraWeb', 'DisplayName', 'Algentra Capital Web Portal')
Set-NssmSetting @('AlgentraWeb', 'AppExit', 'Default', 'Restart')
Set-NssmSetting @('AlgentraWeb', 'AppRestartDelay', '5000')
Set-NssmSetting @('AlgentraWeb', 'AppStdout', (Join-Path $webLogs 'stdout.log'))
Set-NssmSetting @('AlgentraWeb', 'AppStderr', (Join-Path $webLogs 'stderr.log'))
Set-NssmSetting @('AlgentraWeb', 'AppRotateFiles', '1')
Set-NssmSetting @('AlgentraWeb', 'AppRotateBytes', '10485760')
Set-NssmSetting @('AlgentraWeb', 'ObjectName', $webAccount, '')
& sc.exe sidtype AlgentraWeb unrestricted | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal mengaktifkan service SID AlgentraWeb.' }

Prepare-NssmService 'AlgentraCloudflared' $CloudflaredPath @('tunnel', '--no-autoupdate', '--logfile', (Join-Path $tunnelLogs 'cloudflared.log'), 'run', '--token-file', $TunnelTokenPath)
Set-NssmSetting @('AlgentraCloudflared', 'Start', 'SERVICE_AUTO_START')
Set-NssmSetting @('AlgentraCloudflared', 'AppDirectory', (Split-Path -Parent $CloudflaredPath))
Set-NssmSetting @('AlgentraCloudflared', 'DisplayName', 'Algentra Cloudflare Tunnel')
Set-NssmSetting @('AlgentraCloudflared', 'AppExit', 'Default', 'Restart')
Set-NssmSetting @('AlgentraCloudflared', 'AppRestartDelay', '5000')
Set-NssmSetting @('AlgentraCloudflared', 'AppStdout', (Join-Path $tunnelLogs 'stdout.log'))
Set-NssmSetting @('AlgentraCloudflared', 'AppStderr', (Join-Path $tunnelLogs 'stderr.log'))
Set-NssmSetting @('AlgentraCloudflared', 'AppRotateFiles', '1')
Set-NssmSetting @('AlgentraCloudflared', 'AppRotateBytes', '10485760')
Set-NssmSetting @('AlgentraCloudflared', 'ObjectName', $tunnelAccount, '')
& sc.exe sidtype AlgentraCloudflared unrestricted | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal mengaktifkan service SID AlgentraCloudflared.' }

foreach ($grant in @(
    @{ Path = $webRoot; Account = $webAccount },
    @{ Path = $PSScriptRoot; Account = $webAccount },
    @{ Path = (Split-Path -Parent $NssmPath); Account = $webAccount },
    @{ Path = (Split-Path -Parent $NssmPath); Account = $tunnelAccount },
    @{ Path = (Join-Path (Split-Path -Parent $ProjectRoot) 'tools\node'); Account = $webAccount },
    @{ Path = (Split-Path -Parent $CloudflaredPath); Account = $tunnelAccount }
)) {
    & icacls.exe $grant.Path /grant "$($grant.Account):(OI)(CI)RX" /T /C | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak baca/execute pada $($grant.Path)." }
}
& icacls.exe $TunnelTokenPath /grant "${tunnelAccount}:R" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal memberi cloudflared hak baca token.' }
foreach ($grant in @(@{ Path = $webLogs; Account = $webAccount }, @{ Path = $tunnelLogs; Account = $tunnelAccount })) {
    & icacls.exe $grant.Path /grant "$($grant.Account):(OI)(CI)M" /T /C | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak tulis pada $($grant.Path)." }
}
foreach ($account in @($webAccount, $tunnelAccount)) {
    $directory = Get-Item -LiteralPath (Split-Path -Parent $ProjectRoot)
    while ($directory) {
        & icacls.exe $directory.FullName /grant "${account}:(X)" | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak traversal $account." }
        $parent = Split-Path -Parent $directory.FullName
        if (-not $parent -or $parent -eq $directory.FullName) { break }
        $directory = Get-Item -LiteralPath $parent
    }
    & icacls.exe $ProjectRoot /grant "${account}:(X)" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak traversal $account pada project." }
}

Start-Service AlgentraWeb
Start-Service AlgentraCloudflared
Write-Output "Portal lokal aktif pada http://127.0.0.1:3000 dan Cloudflare Tunnel service tersambung untuk $Domain."
