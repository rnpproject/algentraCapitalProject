param(
    [string]$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path,
    [string]$PythonPath,
    [string]$ApiBaseUrl,
    [string]$TerminalTemplateDir,
    [string]$MasterTerminalPath,
    [string]$SignalDirectory
)

$resolvedRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
$venvPython = Join-Path $resolvedRoot ".venv\Scripts\python.exe"
if (-not [string]::IsNullOrWhiteSpace($PythonPath)) {
    if (Test-Path -LiteralPath $PythonPath -PathType Leaf) {
        $resolvedPython = (Resolve-Path -LiteralPath $PythonPath).Path
    } else {
        $pythonCommand = Get-Command -Name $PythonPath -ErrorAction Stop
        $resolvedPython = $pythonCommand.Source
    }
} elseif (Test-Path -LiteralPath $venvPython -PathType Leaf) {
    $resolvedPython = $venvPython
} else {
    $pythonLauncher = Get-Command -Name "py" -ErrorAction SilentlyContinue
    if (-not $pythonLauncher) {
        throw "Python 3.12 64-bit dan Python Launcher wajib tersedia untuk membuat virtual environment."
    }
    & $pythonLauncher.Source -3.12 -m venv (Join-Path $resolvedRoot ".venv")
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $venvPython -PathType Leaf)) {
        throw "Gagal membuat .venv dengan Python 3.12."
    }
    $resolvedPython = $venvPython
}
$runtimeInfo = (& $resolvedPython -c "import platform,sys; print(f'{sys.version_info.major}.{sys.version_info.minor}|{platform.architecture()[0]}')").Trim()
$runtimeParts = $runtimeInfo -split "\|", 2
if ($runtimeParts.Count -ne 2 -or [version]$runtimeParts[0] -lt [version]"3.12" -or $runtimeParts[1] -ne "64bit") {
    throw "Worker membutuhkan Python 3.12 64-bit atau lebih baru; interpreter terpilih melaporkan '$runtimeInfo'."
}
$workerScript = Join-Path $resolvedRoot "apps\copier-worker\scripts\run_managed_worker.py"
$configDirectory = Join-Path $resolvedRoot "apps\copier-worker\config"
$workerConfigPath = Join-Path $configDirectory "managed-worker.json"
$workerExamplePath = Join-Path $configDirectory "managed-worker.example.json"
$masterConfigPath = Join-Path $configDirectory "master.json"
$masterExamplePath = Join-Path $configDirectory "master.example.json"
if (-not (Test-Path -LiteralPath $workerScript -PathType Leaf)) {
    throw "Script managed worker tidak ditemukan: $workerScript"
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
    if ([string]::IsNullOrWhiteSpace($MasterTerminalPath)) {
        $MasterTerminalPath = Read-Host "Path terminal64.exe MT5 master"
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
if (-not [string]::IsNullOrWhiteSpace($ApiBaseUrl)) {
    $workerConfig.api_base_url = $ApiBaseUrl.TrimEnd("/")
}
if (-not [string]::IsNullOrWhiteSpace($TerminalTemplateDir)) {
    $workerConfig.terminal_template_dir = $TerminalTemplateDir
}
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
if ($createdConfigs -or
    -not [string]::IsNullOrWhiteSpace($ApiBaseUrl) -or
    -not [string]::IsNullOrWhiteSpace($TerminalTemplateDir)) {
    $encoding = New-Object System.Text.UTF8Encoding($false)
    $json = ConvertTo-Json -InputObject $workerConfig -Depth 12
    [IO.File]::WriteAllText((Join-Path $configDirectory "managed-worker.json"), $json, $encoding)
}
if ($createdConfigs -or
    -not [string]::IsNullOrWhiteSpace($MasterTerminalPath) -or
    -not [string]::IsNullOrWhiteSpace($SignalDirectory)) {
    $encoding = New-Object System.Text.UTF8Encoding($false)
    $json = ConvertTo-Json -InputObject $masterConfig -Depth 12
    [IO.File]::WriteAllText((Join-Path $configDirectory "master.json"), $json, $encoding)
}
if ([string]::IsNullOrWhiteSpace([string]$workerConfig.api_base_url)) {
    throw "api_base_url wajib diisi pada managed-worker.json."
}
$apiUri = $null
if (-not [Uri]::TryCreate([string]$workerConfig.api_base_url, [UriKind]::Absolute, [ref]$apiUri) -or
    $apiUri.Scheme -notin @("http", "https")) {
    throw "api_base_url harus berupa URL HTTP(S) yang valid."
}
$terminalTemplateDir = [string]$workerConfig.terminal_template_dir
if (-not (Test-Path -LiteralPath $terminalTemplateDir -PathType Container) -or
    -not (Test-Path -LiteralPath (Join-Path $terminalTemplateDir "terminal64.exe") -PathType Leaf)) {
    throw "terminal_template_dir harus menunjuk folder MT5 follower bersih yang berisi terminal64.exe."
}
if (-not (Test-Path -LiteralPath ([string]$masterConfig.terminal_path) -PathType Leaf)) {
    throw "terminal_path pada master.json harus menunjuk terminal64.exe master yang tersedia."
}
if (-not (Test-Path -LiteralPath ([string]$masterConfig.folder_sinyal) -PathType Container)) {
    throw "folder_sinyal pada master.json harus menunjuk folder sinyal yang tersedia."
}
$workerKey = [Environment]::GetEnvironmentVariable("COPIER_WORKER_API_KEY", "User")
if (-not $workerKey) {
    $workerKey = [Environment]::GetEnvironmentVariable("COPIER_WORKER_API_KEY", "Machine")
}
if (-not $workerKey) {
    $secureKey = Read-Host "Masukkan COPIER_WORKER_API_KEY yang sama dengan backend (disimpan pada environment User Windows)" -AsSecureString
    $keyPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureKey)
    try {
        $workerKey = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($keyPointer)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($keyPointer)
    }
    if (-not $workerKey) {
        throw "COPIER_WORKER_API_KEY tidak boleh kosong."
    }
    [Environment]::SetEnvironmentVariable("COPIER_WORKER_API_KEY", $workerKey, "User")
    $env:COPIER_WORKER_API_KEY = $workerKey
}
if ($workerKey.Length -lt 32) {
    throw "COPIER_WORKER_API_KEY minimal 32 karakter."
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
& $resolvedPython -m pip install --disable-pip-version-check -e (Join-Path $resolvedRoot "apps\copier-worker")
if ($LASTEXITCODE -ne 0) {
    throw "Gagal memasang dependensi copier-worker ke interpreter terpilih."
}

$taskName = "Algentra Managed Copy Worker"
$launcherCommand = "`$env:APP_ENV='production'; Set-Location -LiteralPath '$resolvedRoot'; & '$resolvedPython' '$workerScript' '$workerConfig'"
$action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -Command `"$launcherCommand`"" `
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
