param(
    [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot)
)

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
$publisherToken = [Environment]::GetEnvironmentVariable('MASTER_PUBLISHER_TOKEN', 'User')
if (-not $publisherToken) { $publisherToken = [Environment]::GetEnvironmentVariable('MASTER_PUBLISHER_TOKEN', 'Machine') }
if (-not $publisherToken) {
    $secureToken = Read-Host 'Masukkan MASTER_PUBLISHER_TOKEN (disimpan pada environment User Windows)' -AsSecureString
    $tokenPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureToken)
    try { $publisherToken = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($tokenPointer) }
    finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($tokenPointer) }
    if (-not $publisherToken) { throw 'MASTER_PUBLISHER_TOKEN tidak boleh kosong.' }
    [Environment]::SetEnvironmentVariable('MASTER_PUBLISHER_TOKEN', $publisherToken, 'User')
}
if ($publisherToken.Length -lt 32) { throw 'MASTER_PUBLISHER_TOKEN minimal 32 karakter.' }
$launcher = Join-Path $repositoryRoot 'algentra-copier-launcher.exe'
if (-not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
    throw "Binary Go launcher tidak ditemukan: $launcher. Build dengan go build -o algentra-copier-launcher.exe ./cmd/copier-launcher."
}
$taskName = "CopyTrade MT5 Launcher"
$launcherCommand = "`$publisherToken = [Environment]::GetEnvironmentVariable('MASTER_PUBLISHER_TOKEN', 'User'); if (-not `$publisherToken) { `$publisherToken = [Environment]::GetEnvironmentVariable('MASTER_PUBLISHER_TOKEN', 'Machine') }; if (-not `$publisherToken) { throw 'MASTER_PUBLISHER_TOKEN tidak tersedia pada environment akun worker.' }; `$env:MASTER_PUBLISHER_TOKEN = `$publisherToken; Remove-Variable publisherToken; Set-Location -LiteralPath '$repositoryRoot'; & '$launcher' '$resolvedRoot\config\launcher.json'"
$action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -ExecutionPolicy Bypass -Command `"$launcherCommand`"" -WorkingDirectory $resolvedRoot
$trigger = New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME"
$principal = New-ScheduledTaskPrincipal -UserId "$env:USERDOMAIN\$env:USERNAME" -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Description "Menjalankan supervisor copy trade MT5 saat akun operator login" -Force | Out-Null
Write-Output "Scheduled task '$taskName' dibuat untuk $resolvedRoot"
