$ErrorActionPreference = 'Stop'
$installRoot = 'C:\Algentra'
$expectedRoot = [IO.Path]::GetFullPath($installRoot).TrimEnd('\')
$installPrefix = $expectedRoot + '\'
if ($expectedRoot -ne 'C:\Algentra') {
    throw "Jalur instalasi tidak sesuai target uninstaller: $expectedRoot"
}

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Jalankan uninstall-one-click.cmd agar PowerShell meminta elevasi Administrator.'
}

$installDirectory = Get-Item -LiteralPath $installRoot -Force -ErrorAction SilentlyContinue
if ($installDirectory -and -not $installDirectory.PSIsContainer) {
    throw "$installRoot ada tetapi bukan folder. Tidak ada perubahan yang dilakukan."
}
if ($installDirectory -and ($installDirectory.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "$installRoot adalah junction/symlink; uninstaller berhenti agar tidak menghapus target lain."
}

Write-Output 'Uninstall Algentra One-Click akan menghentikan service/task Algentra dan menghapus seluruh C:\Algentra.'
Write-Output 'Data MT5, akun follower, konfigurasi, log, dan secret di folder tersebut ikut terhapus.'
Write-Output 'Database Supabase, domain/nameserver, RDP, dan program lain di Windows tidak dihapus.'
$confirmation = Read-Host 'Ketik UNINSTALL untuk melanjutkan'
if ($confirmation -cne 'UNINSTALL') {
    Write-Output 'Uninstall dibatalkan; tidak ada perubahan yang dilakukan.'
    exit 0
}

$cloudflareStatePath = Join-Path $installRoot 'secrets\cloudflare-tunnel.json'
$cloudflareState = $null
if (Test-Path -LiteralPath $cloudflareStatePath -PathType Leaf) {
    try { $cloudflareState = Get-Content -LiteralPath $cloudflareStatePath -Raw | ConvertFrom-Json -ErrorAction Stop }
    catch { Write-Warning 'State Tunnel tidak dapat dibaca; resource Cloudflare akan dibiarkan.' }
}
$removeCloudflareResources = $false
$cloudflareApiToken = $null
if ($cloudflareState -and $cloudflareState.created_by_installer -eq $true) {
    $removeCloudflareAnswer = (Read-Host 'Hapus Tunnel dan DNS portal yang dibuat installer dari Cloudflare juga? [y/N]').Trim()
    $removeCloudflareResources = $removeCloudflareAnswer -match '^(?i:y|yes)$'
    if ($removeCloudflareResources) {
        $secureToken = Read-Host 'Cloudflare API token (Tunnel Edit, DNS Edit, Zone Read; kosongkan untuk melewati cleanup Cloudflare)' -AsSecureString
        if ($secureToken.Length -gt 0) {
            $tokenPointer = [IntPtr]::Zero
            try {
                $tokenPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureToken)
                $cloudflareApiToken = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($tokenPointer)
            } finally {
                if ($tokenPointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($tokenPointer) }
                $secureToken.Dispose()
            }
        } else {
            $secureToken.Dispose()
            Write-Output 'Token kosong; DNS dan Tunnel Cloudflare akan dibiarkan.'
            $removeCloudflareResources = $false
        }
    }
} elseif ($cloudflareState) {
    Write-Output 'Tunnel Cloudflare yang tercatat bukan dibuat oleh installer ini; resource Cloudflare akan dibiarkan.'
}

$taskNames = @('Algentra Managed Copy Worker', 'Algentra MT5 Master')
foreach ($taskName in $taskNames) {
    $task = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if (-not $task) { continue }
    $taskDefinition = @($task.Actions | ForEach-Object { @($_.Execute, $_.Arguments, $_.WorkingDirectory) -join ' ' }) -join ' '
    if ($taskDefinition.IndexOf($expectedRoot, [StringComparison]::OrdinalIgnoreCase) -lt 0) {
        Write-Warning "Task '$taskName' tidak menunjuk ke $installRoot; task dibiarkan."
        continue
    }
    try { Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue } catch { }
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction Stop
    Write-Output "Scheduled Task dihapus: $taskName"
}

$serviceNames = @('AlgentraCoreApi', 'AlgentraWeb', 'AlgentraCloudflared')
$expectedNssmPath = Join-Path $installRoot 'tools\nssm\win64\nssm.exe'
foreach ($serviceName in $serviceNames) {
    $service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if (-not $service) { continue }
    $nativeService = Get-CimInstance -ClassName Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue
    $ownedByInstall = $nativeService -and
        $nativeService.PathName.IndexOf($expectedNssmPath, [StringComparison]::OrdinalIgnoreCase) -ge 0 -and
        $nativeService.PathName -match ('(?i)//RS//' + [regex]::Escape($serviceName) + '(?:\s|$)')
    if (-not $ownedByInstall) {
        if ($nativeService -and $nativeService.PathName.IndexOf($installPrefix, [StringComparison]::OrdinalIgnoreCase) -ge 0) {
            throw "Service '$serviceName' memakai $installRoot tetapi tidak cocok dengan service NSSM yang dikenal. Service dipertahankan dan folder belum dihapus."
        }
        Write-Warning "Service '$serviceName' tidak terbukti berasal dari $installRoot; service dibiarkan."
        continue
    }
    try {
        if ($service.Status -ne 'Stopped') {
            Stop-Service -Name $serviceName -Force -ErrorAction Stop
            $service.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
        }
    } catch {
        throw "Service '$serviceName' tidak dapat dihentikan. $installRoot belum dihapus. Detail: $($_.Exception.Message)"
    }
    & sc.exe delete $serviceName | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Pendaftaran service '$serviceName' gagal dihapus (sc.exe exit $LASTEXITCODE). $installRoot belum dihapus."
    }
    $removed = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        if (-not (Get-Service -Name $serviceName -ErrorAction SilentlyContinue)) { $removed = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $removed) { throw "Service '$serviceName' masih terdaftar. $installRoot belum dihapus; jalankan uninstaller kembali setelah Windows menyelesaikan penghapusan service." }
    Write-Output "Windows Service dihapus: $serviceName"
}

$ownedProcesses = @(Get-CimInstance -ClassName Win32_Process -ErrorAction SilentlyContinue | Where-Object {
    $_.ExecutablePath -and $_.ExecutablePath.StartsWith($installPrefix, [StringComparison]::OrdinalIgnoreCase)
})
foreach ($process in $ownedProcesses) {
    Stop-Process -Id $process.ProcessId -Force -ErrorAction SilentlyContinue
}

if ($removeCloudflareResources) {
    $apiRoot = 'https://api.cloudflare.com/client/v4'
    $apiHeaders = @{ Authorization = "Bearer $cloudflareApiToken" }
    function Invoke-AlgentraCloudflareApi([string] $Method, [string] $Path) {
        $response = Invoke-RestMethod -Method $Method -Uri ($apiRoot + $Path) -Headers $apiHeaders -TimeoutSec 20
        if ($response.success -eq $false) {
            $messages = @($response.errors | ForEach-Object { $_.message }) -join '; '
            throw "Cloudflare API menolak $Method $Path`: $messages"
        }
        return $response.result
    }

    try {
        $hostname = [string]$cloudflareState.hostname
        $zoneId = [uri]::EscapeDataString([string]$cloudflareState.zone_id)
        $tunnelId = [uri]::EscapeDataString([string]$cloudflareState.tunnel_id)
        $accountId = [uri]::EscapeDataString([string]$cloudflareState.account_id)
        $dnsPath = "/zones/$zoneId/dns_records?type=CNAME&name=$([uri]::EscapeDataString($hostname))&per_page=100"
        $records = @(Invoke-AlgentraCloudflareApi 'GET' $dnsPath)
        $expectedTarget = "$($cloudflareState.tunnel_id).cfargotunnel.com"
        $ownedRecord = $records | Where-Object {
            $_.name -eq $hostname -and $_.type -eq 'CNAME' -and
            $_.content.TrimEnd('.').Equals($expectedTarget, [StringComparison]::OrdinalIgnoreCase) -and
            $_.comment -eq 'Algentra portal Cloudflare Tunnel'
        } | Select-Object -First 1
        if ($ownedRecord) {
            $recordId = [uri]::EscapeDataString([string]$ownedRecord.id)
            Invoke-AlgentraCloudflareApi 'DELETE' "/zones/$zoneId/dns_records/$recordId" | Out-Null
            Write-Output "DNS CNAME portal dihapus: $hostname"
        } else {
            Write-Output "DNS $hostname tidak cocok dengan record buatan installer; record dibiarkan."
        }

        try {
            Invoke-AlgentraCloudflareApi 'DELETE' "/accounts/$accountId/cfd_tunnel/$tunnelId" | Out-Null
            Write-Output 'Cloudflare Tunnel buatan installer dihapus.'
        } catch {
            Write-Warning "Tunnel Cloudflare tidak dapat dihapus (mungkin masih punya koneksi aktif). Hapus dari dashboard Cloudflare bila memang tidak dipakai. Detail: $($_.Exception.Message)"
        }
    } catch {
        Write-Warning "Cleanup DNS/Tunnel Cloudflare gagal; uninstall lokal tetap dilanjutkan. Detail: $($_.Exception.Message)"
    } finally {
        $apiHeaders.Clear()
        $apiHeaders = $null
        $cloudflareApiToken = $null
    }
}

foreach ($account in @('NT SERVICE\AlgentraCoreApi', 'NT SERVICE\AlgentraWeb', 'NT SERVICE\AlgentraCloudflared')) {
    & icacls.exe 'C:\' /remove:g $account /C | Out-Null
}

if (Test-Path -LiteralPath $installRoot -PathType Container) {
    $resolvedInstallRoot = [IO.Path]::GetFullPath((Get-Item -LiteralPath $installRoot -Force).FullName).TrimEnd('\')
    if ($resolvedInstallRoot -ne $expectedRoot -or $resolvedInstallRoot -eq 'C:') {
        throw "Jalur terhapus tidak aman: $resolvedInstallRoot"
    }
    Remove-Item -LiteralPath $resolvedInstallRoot -Recurse -Force
}

if (Test-Path -LiteralPath $installRoot) {
    throw "Folder $installRoot belum berhasil dihapus. Periksa proses/file yang masih terkunci lalu jalankan uninstaller lagi."
}
Write-Output 'Uninstall lokal selesai. Database Supabase dan setup RDP Anda tetap tidak berubah.'
