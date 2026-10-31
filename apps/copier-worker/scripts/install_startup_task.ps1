param(
    [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot)
)

$ErrorActionPreference = 'Stop'
$resolvedRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
$repositoryRoot = $resolvedRoot
while (-not (Test-Path -LiteralPath (Join-Path $repositoryRoot 'go.mod') -PathType Leaf)) {
    $parent = Split-Path -Parent $repositoryRoot
    if (-not $parent -or $parent -eq $repositoryRoot) { throw 'Tidak dapat menemukan root repo Go (go.mod).' }
    $repositoryRoot = $parent
}
$publisher = Join-Path $repositoryRoot 'algentra-copier-master.exe'
if (-not (Test-Path -LiteralPath $publisher -PathType Leaf)) {
    throw "Binary publisher Go tidak ditemukan: $publisher. Build dari root repo dengan go build -o algentra-copier-master.exe ./cmd/copier-master."
}
$launcher = Join-Path $repositoryRoot 'algentra-copier-launcher.exe'
if (-not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
    throw "Binary Go launcher tidak ditemukan: $launcher. Build dengan go build -o algentra-copier-launcher.exe ./cmd/copier-launcher."
}
$configPath = Join-Path $resolvedRoot 'config\launcher.json'
if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
    throw "Konfigurasi launcher tidak ditemukan: $configPath. Siapkan config/launcher.json sebelum membuat task."
}

$workerAccount = if ($env:USERDOMAIN) { "$env:USERDOMAIN\$env:USERNAME" } else { $env:USERNAME }
$runtimeDirectory = Join-Path $resolvedRoot 'config\runtime'
$secretDirectory = Join-Path $runtimeDirectory 'secrets'
$taskTempDirectory = Join-Path $runtimeDirectory 'temp'
New-Item -ItemType Directory -Path $taskTempDirectory -Force | Out-Null
& icacls.exe $taskTempDirectory /inheritance:r /grant:r 'SYSTEM:(OI)(CI)F' 'BUILTIN\Administrators:(OI)(CI)F' "${workerAccount}:(OI)(CI)M" /T /C | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Gagal mengunci ACL folder sementara task: $taskTempDirectory" }
$publisherTokenPath = Join-Path $secretDirectory 'publisher.json'
New-Item -ItemType Directory -Path $secretDirectory -Force | Out-Null
$publisherToken = ''
$writePublisherToken = -not (Test-Path -LiteralPath $publisherTokenPath -PathType Leaf)
if (Test-Path -LiteralPath $publisherTokenPath -PathType Leaf) {
    try { $publisherToken = [string](Get-Content -LiteralPath $publisherTokenPath -Raw | ConvertFrom-Json -ErrorAction Stop).publisher_token }
    catch { throw "Token publisher tersimpan tidak valid: $publisherTokenPath" }
}
$generatedToken = $false
if (-not $publisherToken) {
    foreach ($scope in @('User', 'Machine')) {
        $legacyToken = [Environment]::GetEnvironmentVariable('MASTER_PUBLISHER_TOKEN', $scope)
        if ($legacyToken) { $publisherToken = $legacyToken; break }
    }
}
if (-not $publisherToken) {
    $randomBytes = [byte[]]::new(32)
    $randomGenerator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $randomGenerator.GetBytes($randomBytes) } finally { $randomGenerator.Dispose() }
    $publisherToken = [BitConverter]::ToString($randomBytes).Replace('-', '').ToLowerInvariant()
    [Array]::Clear($randomBytes, 0, $randomBytes.Length)
    $generatedToken = $true
    $writePublisherToken = $true
}
if ($publisherToken.Length -lt 32) { throw 'MASTER_PUBLISHER_TOKEN minimal 32 karakter.' }

& icacls.exe $taskTempDirectory /grant "${workerAccount}:(OI)(CI)M" /T /C | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Gagal membatasi ACL folder sementara task: $taskTempDirectory" }
if ($writePublisherToken) {
    & icacls.exe $secretDirectory /inheritance:r /grant:r 'SYSTEM:(OI)(CI)F' 'BUILTIN\Administrators:(OI)(CI)F' "${workerAccount}:(OI)(CI)M" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Gagal membatasi ACL direktori rahasia: $secretDirectory" }
    [IO.File]::WriteAllText($publisherTokenPath, (@{ publisher_token = $publisherToken } | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
}
& icacls.exe $publisherTokenPath /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' "${workerAccount}:R" | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Gagal mengunci ACL file token publisher: $publisherTokenPath" }
foreach ($scope in @('User', 'Machine')) {
    $legacyToken = [Environment]::GetEnvironmentVariable('MASTER_PUBLISHER_TOKEN', $scope)
    if ($legacyToken) {
        [Environment]::SetEnvironmentVariable('MASTER_PUBLISHER_TOKEN', $null, $scope)
    }
}

$taskName = 'CopyTrade MT5 Launcher'
$taskRunner = Join-Path $PSScriptRoot 'run_startup_task.ps1'
$runnerArguments = @(
    '-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File',
    "`"$taskRunner`"",
    '-LauncherPath', "`"$launcher`"",
    '-ConfigPath', "`"$configPath`"",
    '-SecretPath', "`"$publisherTokenPath`"",
    '-TempDirectory', "`"$taskTempDirectory`"",
    '-ProjectRoot', "`"$repositoryRoot`""
) -join ' '
$action = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument $runnerArguments -WorkingDirectory $repositoryRoot
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $workerAccount
$principal = New-ScheduledTaskPrincipal -UserId $workerAccount -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings `
    -Description 'Menjalankan supervisor copy trade MT5 saat akun operator login.' -Force | Out-Null

if ($generatedToken) { Write-Output 'MASTER_PUBLISHER_TOKEN dibuat otomatis.' }
else { Write-Output 'MASTER_PUBLISHER_TOKEN dimigrasikan atau dipakai kembali dari file rahasia lokal.' }
Write-Output "PublisherToken untuk input EA: $publisherToken"
Write-Output "Scheduled task '$taskName' dibuat untuk $resolvedRoot"
$publisherToken = $null
