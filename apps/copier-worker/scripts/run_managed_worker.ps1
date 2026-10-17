param(
    [string]$ConfigPath = (Join-Path $PSScriptRoot "..\config\managed-worker.json")
)

$copyTradingRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$projectRoot = (Resolve-Path (Join-Path $copyTradingRoot "..")).Path
$configDirectory = Split-Path -Parent $ConfigPath
New-Item -ItemType Directory -Path $configDirectory -Force | Out-Null
$createdConfigs = $false
foreach ($name in @("managed-worker", "master")) {
    $destination = Join-Path $configDirectory "$name.json"
    $example = Join-Path $configDirectory "$name.example.json"
    if (-not (Test-Path -LiteralPath $destination -PathType Leaf)) {
        if (-not (Test-Path -LiteralPath $example -PathType Leaf)) {
            throw "Contoh konfigurasi tidak ditemukan: $example"
        }
        Copy-Item -LiteralPath $example -Destination $destination
        $createdConfigs = $true
    }
}
$runtimeDirectory = Join-Path $configDirectory "runtime"
foreach ($directory in @("signals", "terminals", "states", "reports", "heartbeats", "logs")) {
    New-Item -ItemType Directory -Path (Join-Path $runtimeDirectory $directory) -Force | Out-Null
}
if ($createdConfigs) {
    Write-Output "Konfigurasi lokal dibuat pada $configDirectory. Atur api_base_url dan terminal_template_dir pada managed-worker.json bila nilainya belum sesuai, lalu jalankan lagi."
    exit 0
}

if (-not $env:COPIER_WORKER_API_KEY -and $env:APP_ENV -ne "production") {
    $envFile = Join-Path $projectRoot ".env.local"
    if (Test-Path -LiteralPath $envFile -PathType Leaf) {
        $line = Get-Content -LiteralPath $envFile | Where-Object { $_ -match '^\s*COPIER_WORKER_API_KEY\s*=' } | Select-Object -Last 1
        if ($line) {
            $value = ($line -replace '^\s*COPIER_WORKER_API_KEY\s*=\s*', '').Trim()
            if ($value.Length -ge 2 -and (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'")))) {
                $value = $value.Substring(1, $value.Length - 2)
            }
            $env:COPIER_WORKER_API_KEY = $value
        }
    }
}

$workerBinary = Join-Path $projectRoot "algentra-copier-worker.exe"
if (-not (Test-Path -LiteralPath $workerBinary -PathType Leaf)) {
    throw "Binary managed worker Go tidak ditemukan: $workerBinary. Build dengan go build -o algentra-copier-worker.exe ./cmd/copier-worker."
}
if (-not $env:MASTER_PUBLISHER_TOKEN -and $env:APP_ENV -ne "production") {
    throw "MASTER_PUBLISHER_TOKEN belum ada di environment worker."
}
& $workerBinary $ConfigPath
exit $LASTEXITCODE
