param(
    [string]$ProjectRoot = $(if (Test-Path -LiteralPath "C:\Algentra\app" -PathType Container) { "C:\Algentra\app" } else { (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path }),
    [string]$ApiBaseUrl,
    [string]$TerminalTemplateDir = $(if (Test-Path -LiteralPath "C:\Algentra\mt5\follower-template" -PathType Container) { "C:\Algentra\mt5\follower-template" } else { "" }),
    [string]$MasterTerminalPath,
    [string]$SignalDirectory = "C:\Algentra\signals",
    [string]$RuntimeDirectory
)

$resolvedRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
$originalWorkerKey = [Environment]::GetEnvironmentVariable("COPIER_WORKER_API_KEY", "Process")
$originalPublisherToken = [Environment]::GetEnvironmentVariable("MASTER_PUBLISHER_TOKEN", "Process")
$workerBinary = Join-Path $resolvedRoot "algentra-copier-worker.exe"
$configDirectory = Join-Path $resolvedRoot "apps\copier-worker\config"
$workerConfigPath = Join-Path $configDirectory "managed-worker.json"
$workerExamplePath = Join-Path $configDirectory "managed-worker.example.json"
$masterConfigPath = Join-Path $configDirectory "master.json"
$masterExamplePath = Join-Path $configDirectory "master.example.json"
if ([string]::IsNullOrWhiteSpace($ApiBaseUrl)) {
    $portalUrlFile = Join-Path $resolvedRoot "portal-url.txt"
    if (Test-Path -LiteralPath $portalUrlFile -PathType Leaf) {
        $ApiBaseUrl = (Get-Content -LiteralPath $portalUrlFile -Raw).Trim()
    }
}
if (-not (Test-Path -LiteralPath $workerBinary -PathType Leaf)) {
	throw "Binary managed worker Go tidak ditemukan: $workerBinary. Build dengan go build -o algentra-copier-worker.exe ./cmd/copier-worker terlebih dahulu."
}
$masterPublisher = Join-Path $resolvedRoot "algentra-copier-master.exe"
if (-not (Test-Path -LiteralPath $masterPublisher -PathType Leaf)) {
    throw "Binary publisher Go tidak ditemukan: $masterPublisher. Build dengan go build -o algentra-copier-master.exe ./cmd/copier-master terlebih dahulu."
}
$createdConfigs = $false
foreach ($pair in @(
    @{ Path = $workerConfigPath; Example = $workerExamplePath },
    @{ Path = $masterConfigPath; Example = $masterExamplePath }
)) {
    if (-not (Test-Path -LiteralPath $pair.Path -PathType Leaf)) {
        if (-not (Test-Path -LiteralPath $pair.Example -PathType Leaf)) {
            throw "File konfigurasi dan contoh tidak ditemukan: $($pair.Example)"
        }
        Copy-Item -LiteralPath $pair.Example -Destination $pair.Path
        $createdConfigs = $true
    }
}
if ($createdConfigs) {
    if ([string]::IsNullOrWhiteSpace($ApiBaseUrl)) {
        $ApiBaseUrl = Read-Host "URL backend API yang dapat dijangkau dari worker"
    }
    if ([string]::IsNullOrWhiteSpace($TerminalTemplateDir)) {
        $TerminalTemplateDir = Read-Host "Path folder template MT5 follower bersih (berisi terminal64.exe)"
    }
    if ([string]::IsNullOrWhiteSpace($SignalDirectory)) {
        $SignalDirectory = Read-Host "Path folder signal yang dibaca publisher"
    }
}

try {
    $workerConfig = Get-Content -LiteralPath $workerConfigPath -Raw | ConvertFrom-Json -ErrorAction Stop
} catch {
    throw "JSON konfigurasi managed worker tidak valid: $workerConfigPath"
}
if ([string]$workerConfig.worker_api_key_env -ne "COPIER_WORKER_API_KEY") {
    throw "worker_api_key_env harus berisi nama variabel COPIER_WORKER_API_KEY, bukan nilai secret."
}
if (-not [string]::IsNullOrWhiteSpace($ApiBaseUrl)) {
    $workerConfig.api_base_url = $ApiBaseUrl.TrimEnd("/")
}
if (-not [string]::IsNullOrWhiteSpace($TerminalTemplateDir)) {
    $workerConfig.terminal_template_dir = $TerminalTemplateDir
}
if (-not [string]::IsNullOrWhiteSpace($RuntimeDirectory)) {
    if (-not [IO.Path]::IsPathRooted($RuntimeDirectory)) {
        throw "RuntimeDirectory harus berupa path absolut: $RuntimeDirectory"
    }
    $workerConfig.runtime_dir = [IO.Path]::GetFullPath($RuntimeDirectory)
}
$workerConfig.execution_mode = "mt5_ea"
$workerConfig.feed_address = "127.0.0.1:8766"
$masterConfigValue = [string]$workerConfig.master_config
if ([string]::IsNullOrWhiteSpace($masterConfigValue)) {
    throw "master_config wajib diisi pada managed-worker.json."
}
$masterConfigPath = if ([IO.Path]::IsPathRooted($masterConfigValue)) {
    $masterConfigValue
} else {
    Join-Path $configDirectory $masterConfigValue
}
if (-not (Test-Path -LiteralPath $masterConfigPath -PathType Leaf)) {
    throw "Konfigurasi master tidak ditemukan: $masterConfigPath"
}
try {
    $masterConfig = Get-Content -LiteralPath $masterConfigPath -Raw | ConvertFrom-Json -ErrorAction Stop
} catch {
    throw "JSON konfigurasi master tidak valid: $masterConfigPath"
}
if (-not [string]::IsNullOrWhiteSpace($MasterTerminalPath)) {
    $masterConfig.terminal_path = $MasterTerminalPath
}
if (-not [string]::IsNullOrWhiteSpace($SignalDirectory)) {
    $masterConfig.folder_sinyal = $SignalDirectory
}
if ([string]::IsNullOrWhiteSpace([string]$masterConfig.folder_sinyal)) {
    $masterConfig.folder_sinyal = "C:/Algentra/signals"
}
if ($createdConfigs -or
    -not [string]::IsNullOrWhiteSpace($ApiBaseUrl) -or
    -not [string]::IsNullOrWhiteSpace($TerminalTemplateDir) -or
    -not [string]::IsNullOrWhiteSpace($RuntimeDirectory)) {
    $encoding = New-Object System.Text.UTF8Encoding($false)
    $json = ConvertTo-Json -InputObject $workerConfig -Depth 12
    [IO.File]::WriteAllText((Join-Path $configDirectory "managed-worker.json"), $json, $encoding)
}
$encoding = New-Object System.Text.UTF8Encoding($false)
$json = ConvertTo-Json -InputObject $masterConfig -Depth 12
[IO.File]::WriteAllText((Join-Path $configDirectory "master.json"), $json, $encoding)
if ([string]::IsNullOrWhiteSpace([string]$workerConfig.api_base_url)) {
    throw "api_base_url wajib diisi pada managed-worker.json."
}
$apiUri = $null
if (-not [Uri]::TryCreate([string]$workerConfig.api_base_url, [UriKind]::Absolute, [ref]$apiUri) -or
    $apiUri.Scheme -notin @("http", "https")) {
    throw "api_base_url harus berupa URL HTTP(S) yang valid."
}
$terminalTemplateDir = [string]$workerConfig.terminal_template_dir
if (-not [IO.Path]::IsPathRooted($terminalTemplateDir)) {
    $terminalTemplateDir = Join-Path $configDirectory $terminalTemplateDir
}
if (-not (Test-Path -LiteralPath $terminalTemplateDir -PathType Container) -or
    -not (Test-Path -LiteralPath (Join-Path $terminalTemplateDir "terminal64.exe") -PathType Leaf)) {
    throw "terminal_template_dir harus menunjuk folder MT5 follower bersih yang berisi terminal64.exe."
}
$probeExpertSource = Join-Path $resolvedRoot "mt5\experts\Algentra\MT5LoginProbe.ex5"
if (-not (Test-Path -LiteralPath $probeExpertSource -PathType Leaf)) {
    throw "Expert login probe tidak ditemukan: $probeExpertSource. Jalankan installer one-click agar EA dikompilasi otomatis."
}
$followerExpertSource = Join-Path $resolvedRoot "mt5\experts\MT5FollowerCopyEA.ex5"
if (-not (Test-Path -LiteralPath $followerExpertSource -PathType Leaf)) {
    throw "EA follower tidak ditemukan: $followerExpertSource. Jalankan installer one-click agar EA dikompilasi otomatis."
}
$followerExpertTarget = Join-Path $terminalTemplateDir "MQL5\Experts\MT5FollowerCopyEA.ex5"
New-Item -ItemType Directory -Path (Split-Path -Parent $followerExpertTarget) -Force | Out-Null
Copy-Item -LiteralPath $followerExpertSource -Destination $followerExpertTarget -Force
$probeExpertTarget = Join-Path $terminalTemplateDir "MQL5\Experts\Algentra\MT5LoginProbe.ex5"
New-Item -ItemType Directory -Path (Split-Path -Parent $probeExpertTarget) -Force | Out-Null
Copy-Item -LiteralPath $probeExpertSource -Destination $probeExpertTarget -Force
if ([string]$workerConfig.master_source -eq "publisher_ea" -and -not (Test-Path -LiteralPath ([string]$masterConfig.terminal_path) -PathType Leaf)) {
    throw "terminal_path pada master.json harus menunjuk terminal64.exe master yang tersedia."
}
$signalPath = [string]$masterConfig.folder_sinyal
if (-not [IO.Path]::IsPathRooted($signalPath)) {
    $signalPath = Join-Path $resolvedRoot $signalPath
}
New-Item -ItemType Directory -Path $signalPath -Force | Out-Null
$workerAccount = if ($env:USERDOMAIN) { "$env:USERDOMAIN\$env:USERNAME" } else { $env:USERNAME }
$runtimeDirectory = [string]$workerConfig.runtime_dir
if (-not [IO.Path]::IsPathRooted($runtimeDirectory)) {
    $runtimeDirectory = Join-Path $configDirectory $runtimeDirectory
}
foreach ($directory in @("terminals", "states", "reports", "heartbeats", "logs")) {
    New-Item -ItemType Directory -Path (Join-Path $runtimeDirectory $directory) -Force | Out-Null
}
$workerTempDirectory = Join-Path $runtimeDirectory "temp"
New-Item -ItemType Directory -Path $workerTempDirectory -Force | Out-Null
foreach ($directoryPath in @($runtimeDirectory, $signalPath)) {
    & icacls.exe $directoryPath /grant "${workerAccount}:(OI)(CI)M" /T /C | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak tulis managed worker pada $directoryPath." }
}
foreach ($readOnlyPath in @($configDirectory, $PSScriptRoot, $terminalTemplateDir)) {
    if (Test-Path -LiteralPath $readOnlyPath -PathType Container) {
        & icacls.exe $readOnlyPath /grant "${workerAccount}:(OI)(CI)RX" /T /C | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak baca managed worker pada $readOnlyPath." }
    }
}
foreach ($accessPath in @($workerBinary, $PSScriptRoot, $terminalTemplateDir, $signalPath, $runtimeDirectory)) {
    $directoryPath = if (Test-Path -LiteralPath $accessPath -PathType Container) { $accessPath } else { Split-Path -Parent $accessPath }
    while ($directoryPath) {
        & icacls.exe $directoryPath /grant "${workerAccount}:(X)" | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak traversal managed worker pada $directoryPath." }
        $parentPath = Split-Path -Parent $directoryPath
        if (-not $parentPath -or $parentPath -eq $directoryPath) { break }
        $directoryPath = $parentPath
    }
}
& icacls.exe $workerTempDirectory /inheritance:r /grant:r 'SYSTEM:(OI)(CI)F' 'BUILTIN\Administrators:(OI)(CI)F' "${workerAccount}:(OI)(CI)M" /T /C | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Gagal mengunci ACL folder sementara worker: $workerTempDirectory" }
foreach ($workerFile in @($workerBinary, $masterPublisher)) {
    & icacls.exe $workerFile /grant "${workerAccount}:RX" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak eksekusi managed worker pada $workerFile." }
}
$workerKey = [Environment]::GetEnvironmentVariable("COPIER_WORKER_API_KEY", "User")
if (-not $workerKey) {
    $workerKey = [Environment]::GetEnvironmentVariable("COPIER_WORKER_API_KEY", "Machine")
}
if (-not $workerKey) {
    $workerSecretPath = Join-Path $resolvedRoot "backend\data\integration-secrets\copier-worker.json"
    if (Test-Path -LiteralPath $workerSecretPath -PathType Leaf) {
        try { $workerKey = [string](Get-Content -LiteralPath $workerSecretPath -Raw | ConvertFrom-Json -ErrorAction Stop).api_key }
        catch { throw "Key worker otomatis tidak dapat dibaca dari $workerSecretPath." }
        if ($workerKey) {
            $env:COPIER_WORKER_API_KEY = $workerKey
        }
    }
}
if (-not $workerKey) {
    $secureKey = Read-Host "Masukkan COPIER_WORKER_API_KEY yang sama dengan backend (disimpan pada file rahasia worker lokal)" -AsSecureString
    $keyPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureKey)
    try {
        $workerKey = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($keyPointer)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($keyPointer)
    }
    if (-not $workerKey) {
        throw "COPIER_WORKER_API_KEY tidak boleh kosong."
    }
    $env:COPIER_WORKER_API_KEY = $workerKey
}
if ($workerKey.Length -lt 32) {
    throw "COPIER_WORKER_API_KEY minimal 32 karakter."
}
$publisherToken = [Environment]::GetEnvironmentVariable("MASTER_PUBLISHER_TOKEN", "User")
if (-not $publisherToken) {
    $publisherToken = [Environment]::GetEnvironmentVariable("MASTER_PUBLISHER_TOKEN", "Machine")
}
if (-not $publisherToken) {
    $randomBytes = [byte[]]::new(32)
    $randomGenerator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $randomGenerator.GetBytes($randomBytes) } finally { $randomGenerator.Dispose() }
    $publisherToken = [BitConverter]::ToString($randomBytes).Replace('-', '').ToLowerInvariant()
    [Array]::Clear($randomBytes, 0, $randomBytes.Length)
    $env:MASTER_PUBLISHER_TOKEN = $publisherToken
}
if ($publisherToken.Length -lt 32) {
    throw "MASTER_PUBLISHER_TOKEN minimal 32 karakter."
}
try {
    $health = Invoke-RestMethod -Uri ($apiUri.AbsoluteUri.TrimEnd("/") + "/health") -TimeoutSec 10 -ErrorAction Stop
} catch {
    throw "Backend API tidak dapat dijangkau dari mesin worker. Periksa api_base_url dan koneksi jaringan."
}
if ($health.status -ne "ok") {
    throw "Backend API merespons, tetapi status health bukan OK."
}
try {
    $workerHealth = Invoke-RestMethod `
        -Uri ($apiUri.AbsoluteUri.TrimEnd("/") + "/api/mt5/copier/health") `
        -Headers @{ "X-Copier-Worker-Key" = $workerKey } `
        -TimeoutSec 10 `
        -ErrorAction Stop
} catch {
    throw "Backend menolak COPIER_WORKER_API_KEY atau endpoint worker tidak tersedia. Cocokkan key tanpa membagikannya."
}
if ($workerHealth.status -ne "ok") {
    throw "Backend tidak mengonfirmasi key worker."
}
$secretDirectory = Join-Path $runtimeDirectory "secrets"
New-Item -ItemType Directory -Path $secretDirectory -Force | Out-Null
& icacls.exe $secretDirectory /inheritance:r /grant:r 'SYSTEM:(OI)(CI)F' 'BUILTIN\Administrators:(OI)(CI)F' "${workerAccount}:(OI)(CI)R" | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Gagal mengunci ACL rahasia managed worker: $secretDirectory" }
$secretEncoding = [System.Text.UTF8Encoding]::new($false)
$workerSecret = [pscustomobject]@{ api_key = $workerKey }
$publisherSecret = [pscustomobject]@{ publisher_token = $publisherToken }
[IO.File]::WriteAllText((Join-Path $secretDirectory "worker.json"), ($workerSecret | ConvertTo-Json -Compress), $secretEncoding)
[IO.File]::WriteAllText((Join-Path $secretDirectory "publisher.json"), ($publisherSecret | ConvertTo-Json -Compress), $secretEncoding)
foreach ($secretFile in @((Join-Path $secretDirectory "worker.json"), (Join-Path $secretDirectory "publisher.json"))) {
    & icacls.exe $secretFile /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' "${workerAccount}:R" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Gagal mengunci ACL file rahasia managed worker: $secretFile" }
}
$workerSecret = $null
$publisherSecret = $null
$secretEncoding = $null
foreach ($legacySecret in @(
    @{ Name = "COPIER_WORKER_API_KEY" },
    @{ Name = "MASTER_PUBLISHER_TOKEN" }
)) {
    foreach ($scope in @("User", "Machine")) {
        $storedSecret = [Environment]::GetEnvironmentVariable($legacySecret.Name, $scope)
        if ($storedSecret) {
            [Environment]::SetEnvironmentVariable($legacySecret.Name, $null, $scope)
        }
    }
    $storedSecret = $null
}
if ($null -eq $originalWorkerKey) { Remove-Item Env:COPIER_WORKER_API_KEY -ErrorAction SilentlyContinue } else { $env:COPIER_WORKER_API_KEY = $originalWorkerKey }
if ($null -eq $originalPublisherToken) { Remove-Item Env:MASTER_PUBLISHER_TOKEN -ErrorAction SilentlyContinue } else { $env:MASTER_PUBLISHER_TOKEN = $originalPublisherToken }
$originalWorkerKey = $null
$originalPublisherToken = $null
$workerKey = $null
$publisherToken = $null
$taskName = "Algentra Managed Copy Worker"
$taskLogDirectory = Join-Path $runtimeDirectory "logs"
New-Item -ItemType Directory -Path $taskLogDirectory -Force | Out-Null
$taskErrorLog = Join-Path $taskLogDirectory "managed_worker-task-error.log"
$taskRunner = Join-Path $PSScriptRoot "run_managed_worker_task.ps1"
$runnerArguments = @(
    "-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$taskRunner`"",
    "-ProjectRoot `"$resolvedRoot`"",
    "-WorkerBinaryPath `"$workerBinary`"",
    "-ConfigPath `"$workerConfigPath`"",
    "-TempDirectory `"$workerTempDirectory`"",
    "-SecretDirectory `"$secretDirectory`"",
    "-ErrorLogPath `"$taskErrorLog`""
) -join " "
$action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument $runnerArguments `
    -WorkingDirectory $resolvedRoot
$trigger = New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME"
$principal = New-ScheduledTaskPrincipal `
    -UserId "$env:USERDOMAIN\$env:USERNAME" `
    -LogonType Interactive `
    -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet `
    -RestartCount 5 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -StartWhenAvailable `
    -MultipleInstances IgnoreNew

Register-ScheduledTask `
    -TaskName $taskName `
    -Action $action `
    -Trigger $trigger `
    -Principal $principal `
    -Settings $settings `
    -Description "Menjalankan Algentra managed copier saat Windows startup." `
    -Force | Out-Null
Start-ScheduledTask -TaskName $taskName
Start-Sleep -Seconds 3
$task = Get-ScheduledTask -TaskName $taskName -ErrorAction Stop
if ($task.State -ne "Running") {
    $taskInfo = Get-ScheduledTaskInfo -TaskName $taskName
    throw "Worker task tidak berjalan (state $($task.State), result $($taskInfo.LastTaskResult)). Periksa konfigurasi dan log Task Scheduler."
}
Write-Output "Scheduled task '$taskName' dibuat dan dimulai untuk $resolvedRoot"
