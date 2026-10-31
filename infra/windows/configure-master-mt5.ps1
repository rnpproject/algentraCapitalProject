param(
    [Parameter(Mandatory = $true)] [string] $ProjectRoot,
    [string] $InstallRoot = 'C:\Algentra',
    [Parameter(Mandatory = $true)] [string] $Login,
    [Parameter(Mandatory = $true)] [string] $Password,
    [Parameter(Mandatory = $true)] [string] $Server,
    [bool] $AllowLiveTrading = $false
)

$ErrorActionPreference = 'Stop'
$ProjectRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
$masterRoot = Join-Path $InstallRoot 'mt5\master'
$terminalPath = Join-Path $masterRoot 'terminal64.exe'
$probeBinary = Join-Path $ProjectRoot 'algentra-mt5-login-probe.exe'
$requestPath = Join-Path $InstallRoot 'secrets\mt5-login-probe-request.json'
$resultPath = Join-Path $InstallRoot 'secrets\mt5-login-probe-result.json'
$userAccount = if ($env:USERDOMAIN) { "$env:USERDOMAIN\$env:USERNAME" } else { $env:USERNAME }

if ($Login -notmatch '^[1-9][0-9]*$' -or $Password -match '[\r\n\x00]' -or -not $Password -or $Server -match '[\r\n\x00]' -or -not $Server) {
    throw 'Data akun master MT5 tidak valid.'
}
if (-not (Test-Path -LiteralPath $terminalPath -PathType Leaf) -or -not (Test-Path -LiteralPath $probeBinary -PathType Leaf)) {
    throw 'Terminal MT5 master atau helper MT5LoginProbe tidak tersedia.'
}

$request = [pscustomobject]@{ Path = $terminalPath; Login = [long]$Login; Password = $Password; Server = $Server }
[IO.File]::WriteAllText($requestPath, ($request | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
& icacls.exe $requestPath /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' "${userAccount}:F" | Out-Null
if ($LASTEXITCODE -ne 0) { Remove-Item -LiteralPath $requestPath -Force -ErrorAction SilentlyContinue; throw 'Gagal membatasi ACL permintaan login MT5 sementara.' }
Remove-Item -LiteralPath $resultPath -Force -ErrorAction SilentlyContinue
try {
    $probeProcess = Start-Process -FilePath $probeBinary `
        -RedirectStandardInput $requestPath `
        -RedirectStandardOutput $resultPath `
        -PassThru -Wait -WindowStyle Hidden
    if ($probeProcess.ExitCode -ne 0 -or -not (Test-Path -LiteralPath $resultPath -PathType Leaf)) {
        throw 'Helper tidak mengembalikan hasil validasi login MT5.'
    }
    $probeResult = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json -ErrorAction Stop
    if (-not $probeResult.valid) {
        throw "Login MT5 master gagal divalidasi: $($probeResult.message)"
    }
    if ([string]$probeResult.symbol -notmatch '(?i)^XAUUSD') {
        throw 'MT5LoginProbe tidak berhasil memilih simbol XAUUSD broker untuk chart master.'
    }
} finally {
    Remove-Item -LiteralPath $requestPath, $resultPath -Force -ErrorAction SilentlyContinue
    $request = $null
}

$eaKeyPath = Join-Path $ProjectRoot 'backend\data\integration-secrets\ea.json'
if (-not (Test-Path -LiteralPath $eaKeyPath -PathType Leaf)) { throw 'EA API key belum dibuat oleh Core API.' }
$eaApiKey = [string](Get-Content -LiteralPath $eaKeyPath -Raw | ConvertFrom-Json -ErrorAction Stop).api_key
if ($eaApiKey.Length -lt 24) { throw 'EA API key tersimpan tidak valid.' }

# Keep the master terminal and its EA preset private to the interactive worker account.
& icacls.exe $masterRoot /inheritance:r /grant:r 'SYSTEM:(OI)(CI)F' 'BUILTIN\Administrators:(OI)(CI)F' "${userAccount}:(OI)(CI)M" /T /C | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL terminal MT5 master.' }
$configDirectory = Join-Path $masterRoot 'config'
$presetDirectory = Join-Path $masterRoot 'MQL5\Presets'
New-Item -ItemType Directory -Path $configDirectory, $presetDirectory -Force | Out-Null
$presetPath = Join-Path $presetDirectory 'algentra-master.set'
$presetLines = @(
    'ServerURL=http://127.0.0.1:8000',
    "ApiKey=$eaApiKey",
    'PollIntervalMs=1000',
    'MasterSnapshotIntervalMs=500'
)
[IO.File]::WriteAllLines($presetPath, $presetLines, [Text.UTF8Encoding]::new($false))
$eaApiKey = $null

$startupTemplatePath = Join-Path $configDirectory 'algentra-master-startup.ini'
$startupLines = @(
    '[Common]',
    "Login=$Login",
    'Password=__ALGENTRA_PASSWORD__',
    "Server=$Server",
    'KeepPrivate=1',
    'NewsEnable=0',
    '',
    '[Experts]',
    'Enabled=1',
    ('AllowLiveTrading=' + [int]$AllowLiveTrading),
    'AllowDllImport=0',
    'Account=0',
    'Profile=0',
    'WebRequest=1',
    'WebRequestUrl=http://127.0.0.1:8000',
    '',
    '[StartUp]',
    'Expert=TelegramSignalEA',
    'ExpertParameters=algentra-master.set',
    ('Symbol=' + [string]$probeResult.symbol),
    'Period=M1',
    ''
)
[IO.File]::WriteAllLines($startupTemplatePath, $startupLines, [Text.UTF8Encoding]::new($false))
$passwordFilePath = Join-Path $configDirectory 'master-password.dpapi'
$encryptedPassword = ConvertFrom-SecureString (ConvertTo-SecureString -String $Password -AsPlainText -Force)
[IO.File]::WriteAllText($passwordFilePath, $encryptedPassword, [Text.UTF8Encoding]::new($false))
$encryptedPassword = $null
$Password = $null

$runnerPath = Join-Path $PSScriptRoot 'run-mt5-master.ps1'
$taskName = 'Algentra MT5 Master'
$taskArguments = @(
    '-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File',
    "`"$runnerPath`"",
    '-TerminalPath', "`"$terminalPath`"",
    '-StartupTemplatePath', "`"$startupTemplatePath`"",
    '-EncryptedPasswordPath', "`"$passwordFilePath`"",
    '-WaitForStartup'
) -join ' '
$action = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument $taskArguments -WorkingDirectory $masterRoot
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $userAccount
$principal = New-ScheduledTaskPrincipal -UserId $userAccount -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings `
    -Description 'Login MT5 master, buka chart XAUUSD M1, dan mulai TelegramSignalEA.' -Force | Out-Null

& $runnerPath -TerminalPath $terminalPath -StartupTemplatePath $startupTemplatePath -EncryptedPasswordPath $passwordFilePath -WaitForStartup
Write-Output "Login akun MT5 master $Login di server $Server berhasil divalidasi; terminal dijadwalkan saat login Windows."
Write-Output "TelegramSignalEA dipasang otomatis pada chart $($probeResult.symbol) M1."
if ($AllowLiveTrading) { Write-Warning 'TelegramSignalEA diizinkan mengeksekusi order live sesuai konfigurasi Anda.' }
else { Write-Output 'Eksekusi order live dinonaktifkan sesuai pilihan instalasi.' }
