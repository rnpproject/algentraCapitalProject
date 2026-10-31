param(
    [string] $SourceRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path,

    [string] $Domain,

    [string] $CloudflareApiToken
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
if ($env:MIGRATION_DATABASE_URL) {
    throw 'Production installer tidak boleh menerima MIGRATION_DATABASE_URL.'
}
. (Join-Path $PSScriptRoot 'Clear-ApplicationEnvironment.ps1')
Clear-AlgentraApplicationEnvironment
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Jalankan PowerShell sebagai Administrator.'
}
$operatingSystem = Get-CimInstance -ClassName Win32_OperatingSystem -ErrorAction Stop
$osVersion = [Version]$operatingSystem.Version
if (-not [Environment]::Is64BitOperatingSystem) {
    throw 'Installer memerlukan Windows 64-bit (x64).'
}
if ($operatingSystem.ProductType -eq 1) {
    if ($osVersion.Major -lt 10) { throw "Windows client $($operatingSystem.Caption) tidak didukung; gunakan Windows 10 atau lebih baru." }
} elseif ($osVersion.Major -lt 10 -or ($osVersion.Major -eq 10 -and $osVersion.Build -lt 14393)) {
    throw "Windows Server $($operatingSystem.Caption) tidak didukung; gunakan Windows Server 2016 atau lebih baru."
}
$SourceRoot = (Resolve-Path -LiteralPath $SourceRoot).Path
$installRoot = 'C:\Algentra'
$ProjectRoot = Join-Path $installRoot 'app'
$downloadsRoot = Join-Path $installRoot 'downloads'
$toolsRoot = Join-Path $installRoot 'tools'
$secretsRoot = Join-Path $installRoot 'secrets'
foreach ($path in @($installRoot, $downloadsRoot, $toolsRoot, $secretsRoot, $ProjectRoot, (Join-Path $toolsRoot 'go-cache'), (Join-Path $toolsRoot 'go-mod-cache'), (Join-Path $toolsRoot 'go-tmp'), (Join-Path $toolsRoot 'gopath'), (Join-Path $toolsRoot 'npm-cache'), (Join-Path $toolsRoot 'tmp'))) {
    New-Item -ItemType Directory -Path $path -Force | Out-Null
}
& icacls.exe $secretsRoot /inheritance:r /grant:r 'SYSTEM:(OI)(CI)F' 'BUILTIN\Administrators:(OI)(CI)F' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal mengunci ACL direktori rahasia sebelum installer berjalan.' }
$env:GOCACHE = Join-Path $toolsRoot 'go-cache'
$env:GOENV = Join-Path $toolsRoot 'go.env'
$env:GOPATH = Join-Path $toolsRoot 'gopath'
$env:GOMODCACHE = Join-Path $toolsRoot 'go-mod-cache'
$env:GOTMPDIR = Join-Path $toolsRoot 'go-tmp'
$env:npm_config_cache = Join-Path $toolsRoot 'npm-cache'
$env:TEMP = Join-Path $toolsRoot 'tmp'
$env:TMP = $env:TEMP
if ([IO.Path]::GetFullPath($SourceRoot).TrimEnd('\') -ne [IO.Path]::GetFullPath($ProjectRoot).TrimEnd('\')) {
    $excludedDirectories = @(
        (Join-Path $SourceRoot '.git'),
        (Join-Path $SourceRoot '.venv'),
        (Join-Path $SourceRoot '.mypy_cache'),
        (Join-Path $SourceRoot '.ruff_cache'),
        (Join-Path $SourceRoot '.gocache'),
        (Join-Path $SourceRoot '.pytest_cache'),
        (Join-Path $SourceRoot 'backend\data'),
        (Join-Path $SourceRoot 'backend\__pycache__'),
        (Join-Path $SourceRoot 'apps\web\node_modules'),
        (Join-Path $SourceRoot 'apps\web\.next'),
        (Join-Path $SourceRoot 'apps\copier-worker\runtime'),
        (Join-Path $SourceRoot 'apps\copier-worker\config\runtime-standalone'),
        (Join-Path $SourceRoot 'logs'),
        '.git', '.venv', '.mypy_cache', '.ruff_cache', '.gocache', '.pytest_cache',
        'node_modules', '.next', '__pycache__', 'logs'
    )
    $excludedFiles = @('.env', '.env.*', '*.env', '*.env.*', '*.exe', '*.ex5', '*.log', '*.session', '*.session-journal', '*.pyc', '*.tsbuildinfo', 'managed-worker.json', 'master.json')
    $robocopyArguments = @($SourceRoot, $ProjectRoot, '/E', '/COPY:DAT', '/DCOPY:DAT', '/R:2', '/W:1', '/XD') + $excludedDirectories + @('/XF') + $excludedFiles
    & robocopy.exe @robocopyArguments | Out-Null
    if ($LASTEXITCODE -ge 8) { throw "Gagal menyalin project ke $ProjectRoot (robocopy exit $LASTEXITCODE)." }
}
$webRoot = Join-Path $ProjectRoot 'apps\web'
$recognizedWebEnvironmentFiles = @('.env', '.env.local', '.env.production', '.env.production.local', '.env.development', '.env.development.local', '.env.test', '.env.test.local')
$staleWebEnvironmentFiles = @($recognizedWebEnvironmentFiles | ForEach-Object {
    $candidate = Join-Path $webRoot $_
    if (Test-Path -LiteralPath $candidate -PathType Leaf) { Get-Item -LiteralPath $candidate }
})
if ($staleWebEnvironmentFiles.Count -gt 0) {
    $webEnvironmentBackupRoot = Join-Path $secretsRoot ('web-env-backup-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $webEnvironmentBackupRoot -Force | Out-Null
    foreach ($environmentFile in $staleWebEnvironmentFiles) {
        Move-Item -LiteralPath $environmentFile.FullName -Destination $webEnvironmentBackupRoot
    }
    Write-Warning "File environment Next.js lama dipindahkan ke folder rahasia $webEnvironmentBackupRoot agar tidak menimpa konfigurasi production."
}
$envFile = Join-Path $secretsRoot 'core-api.env'
$pendingFile = Join-Path $secretsRoot 'install-pending.dpapi'
$legacyPendingFile = Join-Path $secretsRoot 'install-pending.json'
function Read-SecureText([string] $Prompt) {
    $secure = Read-Host $Prompt -AsSecureString
    $pointer = [IntPtr]::Zero
    try {
        $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    }
    finally {
        if ($pointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer) }
        $secure.Dispose()
    }
}
function Get-EnvironmentValue([string] $Name, [string] $Path) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return '' }
    $pattern = '^\s*' + [regex]::Escape($Name) + '\s*=(.*)$'
    $line = Get-Content -LiteralPath $Path | Where-Object { $_ -match $pattern } | Select-Object -Last 1
    if (-not $line) { return '' }
    return $line.Substring($line.IndexOf('=') + 1).Trim().Trim('"', "'")
}
$installConfigurationPath = Join-Path $secretsRoot 'install-configuration.dpapi'
$legacyInstallConfigurationPath = Join-Path $secretsRoot 'install-configuration.json'
function Save-ProtectedInstallConfiguration($Configuration, [string] $Path) {
    $serialized = ConvertTo-Json -InputObject $Configuration -Depth 4 -Compress
    $secureSerialized = ConvertTo-SecureString -String $serialized -AsPlainText -Force
    try {
        $protectedSerialized = ConvertFrom-SecureString -SecureString $secureSerialized
        [IO.File]::WriteAllText($Path, $protectedSerialized, [Text.UTF8Encoding]::new($false))
    } finally {
        $secureSerialized.Dispose()
        $serialized = $null
        $protectedSerialized = $null
    }
    & icacls.exe $Path /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL konfigurasi instalasi terenkripsi.' }
}
function Read-ProtectedInstallConfiguration([string] $Path) {
    $storedValue = (Get-Content -LiteralPath $Path -Raw -ErrorAction Stop).Trim()
    if ($storedValue.StartsWith('{')) {
        # Migrate interrupted installs from the older plaintext JSON format.
        $configuration = $storedValue | ConvertFrom-Json -ErrorAction Stop
        Save-ProtectedInstallConfiguration $configuration $Path
        return $configuration
    }
    try {
        $secureSerialized = ConvertTo-SecureString -String $storedValue -ErrorAction Stop
    } catch {
        throw 'Konfigurasi wizard terenkripsi tidak dapat dibuka. Jalankan ulang installer dengan akun Windows yang sama dengan saat konfigurasi pertama kali disimpan.'
    }
    $pointer = [IntPtr]::Zero
    try {
        $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureSerialized)
        $serialized = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
        return $serialized | ConvertFrom-Json -ErrorAction Stop
    } finally {
        if ($pointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer) }
        $secureSerialized.Dispose()
        $serialized = $null
    }
}
function Get-ExistingInstallConfiguration {
    $masterConfigDirectory = Join-Path $installRoot 'mt5\master\config'
    $startupTemplatePath = Join-Path $masterConfigDirectory 'algentra-master-startup.ini'
    $passwordFilePath = Join-Path $masterConfigDirectory 'master-password.dpapi'
    if (-not (Test-Path -LiteralPath $envFile -PathType Leaf) -or
        -not (Test-Path -LiteralPath $startupTemplatePath -PathType Leaf) -or
        -not (Test-Path -LiteralPath $passwordFilePath -PathType Leaf)) {
        return $null
    }

    $startupText = Get-Content -LiteralPath $startupTemplatePath -Raw -ErrorAction Stop
    $loginMatch = [regex]::Match($startupText, '(?im)^Login=([1-9][0-9]*)\s*$')
    $serverMatch = [regex]::Match($startupText, '(?im)^Server=(.+?)\s*$')
    if (-not $loginMatch.Success -or -not $serverMatch.Success) { return $null }

    try {
        $securePassword = ConvertTo-SecureString -String (Get-Content -LiteralPath $passwordFilePath -Raw -ErrorAction Stop).Trim() -ErrorAction Stop
    } catch {
        throw 'Password master MT5 terenkripsi tidak dapat dibuka. Jalankan installer dengan akun Windows yang memasang terminal master.'
    }
    $pointer = [IntPtr]::Zero
    try {
        $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)
        $brokerPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    } finally {
        if ($pointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer) }
        $securePassword.Dispose()
    }

    $integrationSecrets = Join-Path $ProjectRoot 'backend\data\integration-secrets'
    $geminiSettingsPath = Join-Path $integrationSecrets 'gemini.json'
    $telegramSettingsPath = Join-Path $integrationSecrets 'telegram.json'
    $optionalSettingsPath = Join-Path $integrationSecrets 'optional-integrations.json'
    $geminiSettings = if (Test-Path -LiteralPath $geminiSettingsPath -PathType Leaf) {
        Get-Content -LiteralPath $geminiSettingsPath -Raw | ConvertFrom-Json -ErrorAction Stop
    } else { $null }
    $telegramSettings = if (Test-Path -LiteralPath $telegramSettingsPath -PathType Leaf) {
        Get-Content -LiteralPath $telegramSettingsPath -Raw | ConvertFrom-Json -ErrorAction Stop
    } else { $null }
    $optionalSettings = if (Test-Path -LiteralPath $optionalSettingsPath -PathType Leaf) {
        Get-Content -LiteralPath $optionalSettingsPath -Raw | ConvertFrom-Json -ErrorAction Stop
    } else { $null }
    $telegramApiId = ''
    if ($telegramSettings -and [int]$telegramSettings.api_id -gt 0) { $telegramApiId = [string]$telegramSettings.api_id }
    $portalUrlPath = Join-Path $ProjectRoot 'portal-url.txt'
    $domain = ''
    if (Test-Path -LiteralPath $portalUrlPath -PathType Leaf) {
        try { $domain = ([Uri](Get-Content -LiteralPath $portalUrlPath -Raw).Trim()).Host } catch { $domain = '' }
    }
    $midtransServerKey = if ($optionalSettings) { [string]$optionalSettings.midtrans_server_key } else { Get-EnvironmentValue 'MIDTRANS_SERVER_KEY' $envFile }
    $midtransMerchantId = if ($optionalSettings) { [string]$optionalSettings.midtrans_merchant_id } else { Get-EnvironmentValue 'MIDTRANS_MERCHANT_ID' $envFile }
    $midtransProduction = if ($optionalSettings) { [bool]$optionalSettings.midtrans_production } else { (Get-EnvironmentValue 'MIDTRANS_IS_PRODUCTION' $envFile) -match '^(?i:true|1|yes)$' }
    $paymentBankName = if ($optionalSettings) { [string]$optionalSettings.payment_bank_name } else { Get-EnvironmentValue 'PAYMENT_BANK_NAME' $envFile }
    $paymentNumber = if ($optionalSettings) { [string]$optionalSettings.payment_number } else { Get-EnvironmentValue 'PAYMENT_NUMBER' $envFile }
    $paymentName = if ($optionalSettings) { [string]$optionalSettings.payment_name } else { Get-EnvironmentValue 'PAYMENT_NAME' $envFile }
    $smtpHost = if ($optionalSettings) { [string]$optionalSettings.email_smtp_host } else { Get-EnvironmentValue 'EMAIL_SMTP_HOST' $envFile }
    $smtpPort = if ($optionalSettings) { [string]$optionalSettings.email_smtp_port } else { Get-EnvironmentValue 'EMAIL_SMTP_PORT' $envFile }
    if (-not $smtpPort) { $smtpPort = '587' }
    $smtpUsername = if ($optionalSettings) { [string]$optionalSettings.email_smtp_username } else { Get-EnvironmentValue 'EMAIL_SMTP_USERNAME' $envFile }
    $smtpPassword = if ($optionalSettings) { [string]$optionalSettings.email_smtp_password } else { Get-EnvironmentValue 'EMAIL_SMTP_PASSWORD' $envFile }
    $smtpFrom = if ($optionalSettings) { [string]$optionalSettings.email_from } else { Get-EnvironmentValue 'EMAIL_FROM' $envFile }
    $smtpStartTls = if ($optionalSettings -and $null -ne $optionalSettings.email_smtp_starttls) { [bool]$optionalSettings.email_smtp_starttls } else { (Get-EnvironmentValue 'EMAIL_SMTP_STARTTLS' $envFile) -notmatch '^(?i:false|0|no)$' }
    $liveTradingMatch = [regex]::Match($startupText, '(?im)^AllowLiveTrading=([01])\s*$')

    return [pscustomobject]@{
        gemini_api_key = if ($geminiSettings) { [string]$geminiSettings.api_key } else { '' }
        gemini_model = if ($geminiSettings -and $geminiSettings.model) { [string]$geminiSettings.model } else { 'gemini-3.5-flash-lite' }
        telegram_api_id = $telegramApiId
        telegram_api_hash = if ($telegramSettings) { [string]$telegramSettings.api_hash } else { '' }
        telegram_phone = ''
        telegram_bot_token = if ($telegramSettings) { [string]$telegramSettings.bot_token } else { '' }
        telegram_admin_chat_id = if ($telegramSettings) { [string]$telegramSettings.admin_chat_id } else { '' }
        broker_login = $loginMatch.Groups[1].Value
        broker_password = $brokerPassword
        broker_server = $serverMatch.Groups[1].Value.Trim()
        allow_live_trading = ($liveTradingMatch.Success -and $liveTradingMatch.Groups[1].Value -eq '1')
        midtrans_server_key = $midtransServerKey
        midtrans_merchant_id = $midtransMerchantId
        midtrans_production = $midtransProduction
        payment_bank_name = $paymentBankName
        payment_number = $paymentNumber
        payment_name = $paymentName
        email_smtp_host = $smtpHost
        email_smtp_port = $smtpPort
        email_smtp_username = $smtpUsername
        email_smtp_password = $smtpPassword
        email_from = $smtpFrom
        email_smtp_starttls = $smtpStartTls
        domain = $domain
        cloudflare_api_token = ''
        database_admin_url = ''
    }
}
if (Test-Path -LiteralPath $installConfigurationPath -PathType Leaf) {
    $installConfiguration = Read-ProtectedInstallConfiguration $installConfigurationPath
} elseif (Test-Path -LiteralPath $legacyInstallConfigurationPath -PathType Leaf) {
    $installConfiguration = Get-Content -LiteralPath $legacyInstallConfigurationPath -Raw | ConvertFrom-Json -ErrorAction Stop
    Save-ProtectedInstallConfiguration $installConfiguration $installConfigurationPath
    Remove-Item -LiteralPath $legacyInstallConfigurationPath -Force
} else {
    $installConfiguration = Get-ExistingInstallConfiguration
    if ($installConfiguration) {
        Write-Output 'Konfigurasi server tersimpan ditemukan; wizard akan menggunakannya kembali tanpa meminta ulang password broker atau key integrasi.'
        Save-ProtectedInstallConfiguration $installConfiguration $installConfigurationPath
    } else {
    Write-Output 'Isi konfigurasi integrasi. Field opsional dapat dikosongkan dan dilengkapi dari dashboard nanti.'
    $geminiApiKey = Read-SecureText 'Gemini API key (opsional; kosongkan untuk mengatur nanti di Settings)'
    $geminiModel = (Read-Host 'Gemini model [gemini-3.5-flash-lite]').Trim()
    if (-not $geminiModel) { $geminiModel = 'gemini-3.5-flash-lite' }
    if ($geminiModel -notmatch '^[A-Za-z0-9._-]{2,100}$') { throw 'Gemini model harus berupa ID model 2-100 karakter.' }

    $telegramApiId = (Read-Host 'Telegram API ID (opsional; kosongkan untuk melewati Telegram user login)')
    $telegramApiHash = ''
    $telegramPhone = ''
    if ($telegramApiId) {
        $telegramApiIdValue = 0
        if (-not [int]::TryParse($telegramApiId, [ref]$telegramApiIdValue) -or $telegramApiIdValue -lt 1) { throw 'Telegram API ID harus angka positif.' }
        $telegramApiId = [string]$telegramApiIdValue
        $telegramApiHash = Read-SecureText 'Telegram API hash'
        if ($telegramApiHash -notmatch '^[A-Fa-f0-9]{32}$') { throw 'Telegram API hash harus berisi 32 karakter heksadesimal.' }
        $telegramPhone = (Read-Host 'Nomor telepon akun Telegram sumber (format internasional, contoh +628123456789)').Trim()
        if ($telegramPhone -notmatch '^\+[1-9][0-9]{7,14}$') { throw 'Nomor telepon Telegram harus memakai format internasional.' }
    }
    $telegramBotToken = Read-SecureText 'Telegram bot token (opsional; kosongkan jika tidak memakai bot pembayaran)'
    $telegramAdminChatId = ''
    if ($telegramBotToken) {
        $telegramAdminChatId = (Read-Host 'Telegram admin private chat ID')
        if ($telegramAdminChatId -notmatch '^[1-9][0-9]*$') { throw 'Telegram admin chat ID harus angka positif.' }
    }

    $midtransServerKey = Read-SecureText 'Midtrans Server Key (opsional; kosongkan jika memakai transfer manual)'
    $midtransMerchantId = ''
    $midtransProduction = $false
    if ($midtransServerKey) {
        $midtransMerchantId = (Read-Host 'Midtrans Merchant ID').Trim()
        if (-not $midtransMerchantId -or $midtransMerchantId -match '[\r\n\x00]') { throw 'Midtrans Merchant ID wajib diisi saat Server Key digunakan.' }
        $midtransProduction = (Read-Host 'Gunakan environment Midtrans production? [y/N]').Trim() -match '^(?i:y|yes)$'
    }
    $paymentBankName = ''
    $paymentNumber = ''
    $paymentName = ''
    if (-not $midtransServerKey) {
        $paymentBankName = (Read-Host 'Nama bank untuk pembayaran transfer manual (wajib tanpa Midtrans)').Trim()
        $paymentNumber = (Read-Host 'Nomor rekening pembayaran manual').Trim()
        $paymentName = (Read-Host 'Nama penerima pembayaran manual').Trim()
        if (-not $paymentBankName -or -not $paymentNumber -or -not $paymentName) { throw 'Nama bank, nomor rekening, dan nama penerima wajib diisi jika Midtrans tidak digunakan.' }
    }

    $smtpHost = (Read-Host 'SMTP host untuk verifikasi email (opsional; kosongkan untuk menonaktifkan pendaftaran client)').Trim()
    $smtpPort = '587'
    $smtpUsername = ''
    $smtpPassword = ''
    $smtpFrom = ''
    $smtpStartTls = $true
    if ($smtpHost) {
        if ($smtpHost -match '[\s\r\n\x00]') { throw 'SMTP host tidak valid.' }
        $smtpPortInput = (Read-Host 'SMTP port [587]').Trim()
        if ($smtpPortInput) { $smtpPort = $smtpPortInput }
        $smtpPortValue = 0
        if (-not [int]::TryParse($smtpPort, [ref]$smtpPortValue) -or $smtpPortValue -lt 1 -or $smtpPortValue -gt 65535) { throw 'SMTP port harus antara 1 dan 65535.' }
        $smtpUsername = (Read-Host 'SMTP username').Trim()
        $smtpPassword = Read-SecureText 'SMTP password/app password'
        $smtpFrom = (Read-Host 'Alamat pengirim email (EMAIL_FROM)').Trim()
        $smtpFromValid = $false
        if ($smtpFrom -and $smtpFrom -notmatch '[\r\n\x00]') {
            try {
                $smtpAddress = [System.Net.Mail.MailAddress]::new($smtpFrom)
                $smtpFromValid = $smtpAddress.Address -match '^[^\s@]+@[^\s@]+\.[^\s@]+$'
            } catch { $smtpFromValid = $false }
        }
        if (-not $smtpUsername -or -not $smtpPassword -or -not $smtpFromValid) {
            throw 'SMTP username/password wajib diisi dan alamat pengirim harus berupa email@domain.com atau Nama Tampilan <email@domain.com>.'
        }
        $smtpAddress = $null
        $smtpFromValid = $null
        $smtpStartTls = (Read-Host 'Gunakan SMTP STARTTLS? [Y/n]').Trim() -notmatch '^(?i:n|no)$'
    }

    $brokerLogin = (Read-Host 'MT5 master broker account login ID').Trim()
    if ($brokerLogin -notmatch '^[1-9][0-9]*$') { throw 'MT5 broker login ID harus angka positif.' }
    $brokerPassword = Read-SecureText 'MT5 master broker password'
    if (-not $brokerPassword -or $brokerPassword -match '[\r\n\x00]') { throw 'MT5 broker password wajib diisi dan tidak boleh berisi baris baru.' }
    $brokerServer = (Read-Host 'MT5 master broker server (nama persis dari broker)').Trim()
    if (-not $brokerServer -or $brokerServer -match '[\r\n\x00]') { throw 'Nama MT5 broker server wajib diisi.' }
    $liveTradingAnswer = (Read-Host 'Izinkan TelegramSignalEA membuka order live segera setelah install? [y/N]').Trim()
    $allowLiveTrading = $liveTradingAnswer -match '^(?i:y|yes)$'

    $installConfiguration = [pscustomobject]@{
        gemini_api_key = $geminiApiKey
        gemini_model = $geminiModel
        telegram_api_id = $telegramApiId
        telegram_api_hash = $telegramApiHash
        telegram_phone = $telegramPhone
        telegram_bot_token = $telegramBotToken
        telegram_admin_chat_id = $telegramAdminChatId
        broker_login = $brokerLogin
        broker_password = $brokerPassword
        broker_server = $brokerServer
        allow_live_trading = $allowLiveTrading
        midtrans_server_key = $midtransServerKey
        midtrans_merchant_id = $midtransMerchantId
        midtrans_production = $midtransProduction
        payment_bank_name = $paymentBankName
        payment_number = $paymentNumber
        payment_name = $paymentName
        email_smtp_host = $smtpHost
        email_smtp_port = $smtpPort
        email_smtp_username = $smtpUsername
        email_smtp_password = $smtpPassword
        email_from = $smtpFrom
        email_smtp_starttls = $smtpStartTls
    }
    Save-ProtectedInstallConfiguration $installConfiguration $installConfigurationPath
    }
}
if (Test-Path -LiteralPath $legacyInstallConfigurationPath -PathType Leaf) {
    Remove-Item -LiteralPath $legacyInstallConfigurationPath -Force
}
if (-not $installConfiguration.broker_login -or -not $installConfiguration.broker_password -or -not $installConfiguration.broker_server) {
    throw 'Konfigurasi MT5 master pada file instalasi tidak lengkap.'
}
foreach ($configurationProperty in @{
    domain = ''
    cloudflare_api_token = ''
    database_admin_url = ''
    midtrans_server_key = ''
    midtrans_merchant_id = ''
    midtrans_production = $false
    payment_bank_name = ''
    payment_number = ''
    payment_name = ''
    email_smtp_host = ''
    email_smtp_port = '587'
    email_smtp_username = ''
    email_smtp_password = ''
    email_from = ''
    email_smtp_starttls = $true
}.GetEnumerator()) {
    if ($null -eq $installConfiguration.PSObject.Properties[$configurationProperty.Key]) {
        $installConfiguration | Add-Member -NotePropertyName $configurationProperty.Key -NotePropertyValue $configurationProperty.Value
    }
}
if ([string]$installConfiguration.gemini_model -notmatch '^[A-Za-z0-9._-]{2,100}$') { throw 'Gemini model pada konfigurasi instalasi tidak valid.' }
if (($installConfiguration.telegram_api_id -and -not $installConfiguration.telegram_api_hash) -or (-not $installConfiguration.telegram_api_id -and $installConfiguration.telegram_api_hash)) {
    throw 'Telegram API ID dan API hash harus diisi berpasangan.'
}
if ($installConfiguration.telegram_api_id -and [string]$installConfiguration.telegram_api_hash -notmatch '^[A-Fa-f0-9]{32}$') {
    throw 'Telegram API ID diisi tetapi Telegram API hash pada konfigurasi instalasi tidak valid.'
}
if ($installConfiguration.telegram_api_id -and [string]$installConfiguration.telegram_phone -notmatch '^\+[1-9][0-9]{7,14}$') {
    throw 'Telegram API credentials diisi tetapi nomor telepon akun Telegram tidak valid.'
}
if (($installConfiguration.telegram_bot_token -and -not $installConfiguration.telegram_admin_chat_id) -or (-not $installConfiguration.telegram_bot_token -and $installConfiguration.telegram_admin_chat_id)) {
    throw 'Telegram bot token dan admin chat ID harus diisi berpasangan.'
}
$existingServices = @()
foreach ($serviceName in @('AlgentraCoreApi', 'AlgentraWeb', 'AlgentraCloudflared')) {
    if (Get-Service -Name $serviceName -ErrorAction SilentlyContinue) { $existingServices += $serviceName }
}
if ($existingServices.Count -gt 0 -and -not (Test-Path -LiteralPath $envFile -PathType Leaf)) {
    throw "Service Algentra sudah ada tetapi environment $envFile tidak ditemukan. Jangan hapus service otomatis; periksa instalasi parsial terlebih dahulu."
}
if ([string]::IsNullOrWhiteSpace($Domain)) { $Domain = [string]$installConfiguration.domain }
if ([string]::IsNullOrWhiteSpace($Domain)) { $Domain = Read-Host 'Hostname publik portal (contoh portal.example.com)' }
$Domain = $Domain.Trim().ToLowerInvariant()
if ($Domain -notmatch '^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$') {
    throw 'Masukkan hostname publik yang valid, misalnya portal.example.com.'
}
$installConfiguration.domain = $Domain
Save-ProtectedInstallConfiguration $installConfiguration $installConfigurationPath
$portalUrlPath = Join-Path $ProjectRoot 'portal-url.txt'
[IO.File]::WriteAllText($portalUrlPath, "https://$Domain", [Text.UTF8Encoding]::new($false))
$cloudflareStatePath = Join-Path $secretsRoot 'cloudflare-tunnel.json'
$cloudflareTokenPath = Join-Path $secretsRoot 'cloudflared-token'
$knownCloudflareState = $null
if (Test-Path -LiteralPath $cloudflareStatePath -PathType Leaf) {
    $knownCloudflareState = Get-Content -LiteralPath $cloudflareStatePath -Raw | ConvertFrom-Json -ErrorAction Stop
}
if (-not $knownCloudflareState -or -not $knownCloudflareState.provisioned -or $knownCloudflareState.hostname -ne $Domain -or -not (Test-Path -LiteralPath $cloudflareTokenPath -PathType Leaf)) {
    if ([string]::IsNullOrWhiteSpace($CloudflareApiToken)) { $CloudflareApiToken = [string]$installConfiguration.cloudflare_api_token }
    if ([string]::IsNullOrWhiteSpace($CloudflareApiToken)) {
        $CloudflareApiToken = Read-SecureText 'Cloudflare API token (Account: Cloudflare Tunnel Edit; Zone: DNS Edit + Zone Read)'
    }
    if ([string]::IsNullOrWhiteSpace($CloudflareApiToken)) { throw 'Cloudflare API token kosong.' }
    $installConfiguration.cloudflare_api_token = $CloudflareApiToken
    Save-ProtectedInstallConfiguration $installConfiguration $installConfigurationPath
}
$reuseEnvironment = Test-Path -LiteralPath $envFile -PathType Leaf
$adminDatabaseUrl = [string]$installConfiguration.database_admin_url
if ($reuseEnvironment -and $adminDatabaseUrl) {
    $installConfiguration.database_admin_url = ''
    Save-ProtectedInstallConfiguration $installConfiguration $installConfigurationPath
    $adminDatabaseUrl = ''
}
if (-not $reuseEnvironment) {
    if ([string]::IsNullOrWhiteSpace($adminDatabaseUrl)) {
        $adminDatabaseUrl = Read-SecureText 'DATABASE_ADMIN_URL role postgres, direct/session pooler port 5432, TLS sslmode=require'
    }
    if ([string]::IsNullOrWhiteSpace($adminDatabaseUrl)) { throw 'DATABASE_ADMIN_URL wajib diisi untuk menyiapkan schema production.' }
    $installConfiguration.database_admin_url = $adminDatabaseUrl
    Save-ProtectedInstallConfiguration $installConfiguration $installConfigurationPath
}

function Install-GoIfMissing {
    $goDirectory = Join-Path $toolsRoot 'go'
    $goPath = Join-Path $goDirectory 'bin\go.exe'
    if (Test-Path -LiteralPath $goPath -PathType Leaf) {
        $env:Path = "$goDirectory\bin;$env:Path"
        $versionText = (& $goPath version) -join ' '
        if ($versionText -match 'go1\.(\d+)' -and [int]$Matches[1] -ge 25) { return }
    }
    Write-Output 'Menyiapkan Go di C:\Algentra\tools...'
    $release = Invoke-RestMethod 'https://go.dev/dl/?mode=json'
    $archive = $release[0].files | Where-Object { $_.os -eq 'windows' -and $_.arch -eq 'amd64' -and $_.kind -eq 'archive' } | Select-Object -First 1
    if (-not $archive) { throw 'Arsip Go Windows amd64 tidak tersedia dari go.dev.' }
    $path = Join-Path $downloadsRoot $archive.filename
    Invoke-WebRequest -Uri "https://go.dev/dl/$($archive.filename)" -OutFile $path
    $extract = Join-Path $downloadsRoot 'go-extract'
    if (Test-Path -LiteralPath $extract) { Remove-Item -LiteralPath $extract -Recurse -Force }
    Expand-Archive -LiteralPath $path -DestinationPath $extract -Force
    if (Test-Path -LiteralPath $goDirectory) { Remove-Item -LiteralPath $goDirectory -Recurse -Force }
    Move-Item -LiteralPath (Join-Path $extract 'go') -Destination $goDirectory
    Remove-Item -LiteralPath $extract -Recurse -Force
    Remove-Item -LiteralPath $path -Force
    $env:Path = "$goDirectory\bin;$env:Path"
}

function Install-NodeIfMissing {
    $nodeDirectory = Join-Path $toolsRoot 'node'
    $nodePath = Join-Path $nodeDirectory 'node.exe'
    if (Test-Path -LiteralPath $nodePath -PathType Leaf) {
        $version = (& $nodePath --version).Trim()
        if ($version -match '^v22\.') { $env:Path = "$nodeDirectory;$env:Path"; return }
    }
    Write-Output 'Menyiapkan Node.js 22 di C:\Algentra\tools...'
    $releases = Invoke-RestMethod 'https://nodejs.org/dist/index.json'
    $release = $releases | Where-Object { $_.version -match '^v22\.' } | Select-Object -First 1
    if (-not $release) { throw 'Node.js 22 tidak ditemukan pada nodejs.org.' }
    $archiveName = "node-$($release.version)-win-x64.zip"
    $path = Join-Path $downloadsRoot $archiveName
    Invoke-WebRequest -Uri "https://nodejs.org/dist/$($release.version)/$archiveName" -OutFile $path
    $extract = Join-Path $downloadsRoot 'node-extract'
    if (Test-Path -LiteralPath $extract) { Remove-Item -LiteralPath $extract -Recurse -Force }
    Expand-Archive -LiteralPath $path -DestinationPath $extract -Force
    if (Test-Path -LiteralPath $nodeDirectory) { Remove-Item -LiteralPath $nodeDirectory -Recurse -Force }
    Move-Item -LiteralPath (Join-Path $extract "node-$($release.version)-win-x64") -Destination $nodeDirectory
    Remove-Item -LiteralPath $extract -Recurse -Force
    Remove-Item -LiteralPath $path -Force
    $env:Path = "$nodeDirectory;$env:Path"
}

function Download-InstallerTools {
    $nssmPath = Join-Path $toolsRoot 'nssm\win64\nssm.exe'
    if (-not (Test-Path -LiteralPath $nssmPath -PathType Leaf)) {
        $zip = Join-Path $downloadsRoot 'algentra-nssm.zip'
        Invoke-WebRequest 'https://nssm.cc/release/nssm-2.24.zip' -OutFile $zip
        Expand-Archive -LiteralPath $zip -DestinationPath $toolsRoot -Force
        Remove-Item -LiteralPath $zip -Force
        $nssmDirectory = Join-Path $toolsRoot 'nssm'
        New-Item -ItemType Directory -Path $nssmDirectory -Force | Out-Null
        Move-Item -LiteralPath (Join-Path $toolsRoot 'nssm-2.24\win64') -Destination $nssmDirectory -Force
        Remove-Item -LiteralPath (Join-Path $toolsRoot 'nssm-2.24') -Recurse -Force
    }
    $cloudflaredPath = Join-Path $toolsRoot 'cloudflared\cloudflared.exe'
    if (-not (Test-Path -LiteralPath $cloudflaredPath -PathType Leaf)) {
        New-Item -ItemType Directory -Path (Split-Path -Parent $cloudflaredPath) -Force | Out-Null
        Invoke-WebRequest 'https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe' -OutFile $cloudflaredPath
    }
    if (-not (Test-Path -LiteralPath $nssmPath -PathType Leaf) -or -not (Test-Path -LiteralPath $cloudflaredPath -PathType Leaf)) {
        throw 'NSSM atau cloudflared tidak ditemukan setelah instalasi.'
    }
    return @{ Nssm = $nssmPath; Cloudflared = $cloudflaredPath }
}

function New-RandomHex([int] $ByteCount) {
    $bytes = New-Object byte[] $ByteCount
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    return ([BitConverter]::ToString($bytes)).Replace('-', '').ToLowerInvariant()
}

function Get-SupabaseProjectRef([string] $DatabaseUrl) {
    try { $parsed = [Uri]$DatabaseUrl } catch { return '' }
    if ($parsed.Host -match '^db\.([a-z0-9]{10,30})\.supabase\.co$') { return $Matches[1] }
    if ($parsed.Host -like '*.pooler.supabase.com' -and $parsed.UserInfo) {
        $userInfo = [Uri]::UnescapeDataString($parsed.UserInfo)
        $username = $userInfo.Split(':', 2)[0]
        if ($username -match '^(?:algentra_api|postgres)\.([a-z0-9]{10,30})$') { return $Matches[1] }
    }
    return ''
}

function New-RandomPassword([int] $Length) {
    $alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%'
    $bytes = New-Object byte[] $Length
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    $password = foreach ($byte in $bytes) { $alphabet[[int]$byte % $alphabet.Length] }
    return -join $password
}

Install-GoIfMissing
Install-NodeIfMissing
$tools = Download-InstallerTools
$webRoot = Join-Path $ProjectRoot 'apps\web'
$backendBinary = Join-Path $ProjectRoot 'algentra-backend.exe'
$webBuildId = Join-Path $webRoot '.next\BUILD_ID'
$nextCli = Join-Path $webRoot 'node_modules\next\dist\bin\next'
$envDirectory = Split-Path -Parent $envFile
New-Item -ItemType Directory -Path $envDirectory -Force | Out-Null
& icacls.exe $envDirectory /inheritance:r /grant:r 'SYSTEM:(OI)(CI)F' 'BUILTIN\Administrators:(OI)(CI)F' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL direktori rahasia installer.' }

if ($reuseEnvironment) {
    . (Join-Path $ProjectRoot 'infra\windows\Load-EnvironmentFile.ps1')
    Remove-Item Env:DATABASE_URL, Env:SUPABASE_PROJECT_REF, Env:APP_SECRET_KEY, Env:ADMIN_PASSWORD, Env:EA_API_KEY, Env:WEBSOCKET_ALLOWED_ORIGINS -ErrorAction SilentlyContinue
    Import-EnvironmentFile -Path $envFile
    $projectRef = $env:SUPABASE_PROJECT_REF
    if (-not $projectRef) { $projectRef = Get-SupabaseProjectRef $env:DATABASE_URL }
    $adminPassword = $env:ADMIN_PASSWORD
    if (-not $projectRef -or -not $env:DATABASE_URL) {
        throw "Environment file $envFile tidak lengkap untuk melanjutkan instalasi."
    }
    if ($env:WEBSOCKET_ALLOWED_ORIGINS -and $env:WEBSOCKET_ALLOWED_ORIGINS -ne "https://$Domain") {
        throw "Domain installer tidak cocok dengan WEBSOCKET_ALLOWED_ORIGINS pada $envFile."
    }
} else {
    if (Test-Path -LiteralPath $pendingFile -PathType Leaf) {
        $pending = Read-ProtectedInstallConfiguration $pendingFile
    } elseif (Test-Path -LiteralPath $legacyPendingFile -PathType Leaf) {
        $pending = Get-Content -LiteralPath $legacyPendingFile -Raw | ConvertFrom-Json -ErrorAction Stop
        Save-ProtectedInstallConfiguration $pending $pendingFile
        Remove-Item -LiteralPath $legacyPendingFile -Force
    } else {
        $projectRef = Get-SupabaseProjectRef $adminDatabaseUrl
        if ($projectRef -notmatch '^[a-z0-9]{10,30}$') {
            throw 'Project ref tidak dapat dibaca dari DATABASE_ADMIN_URL. Gunakan koneksi langsung db.<PROJECT_REF>.supabase.co atau Session Pooler dengan username postgres.<PROJECT_REF>.'
        }
        $apiRoleCredential = New-RandomHex 32
        $adminPassword = New-RandomPassword 30
        $pending = [pscustomobject]@{
            project_ref = $projectRef
            api_password = $apiRoleCredential
            admin_password = $adminPassword
        }
        Save-ProtectedInstallConfiguration $pending $pendingFile
    }
    if (Test-Path -LiteralPath $legacyPendingFile -PathType Leaf) {
        Remove-Item -LiteralPath $legacyPendingFile -Force
    }
    if ($pending) {
        $projectRef = [string]$pending.project_ref
        $apiRoleCredential = [string]$pending.api_password
        $adminPassword = [string]$pending.admin_password
    }
    if ($projectRef -notmatch '^[a-z0-9]{10,30}$' -or $apiRoleCredential.Length -lt 32 -or $adminPassword.Length -lt 12) {
        throw 'File rahasia sementara installer tidak valid.'
    }
    if (-not $adminDatabaseUrl) { $adminDatabaseUrl = Read-SecureText 'DATABASE_ADMIN_URL role postgres, direct/session pooler port 5432, TLS sslmode=require' }
}

$adminProjectRef = Get-SupabaseProjectRef $adminDatabaseUrl
if ($adminProjectRef -and $adminProjectRef -ne $projectRef) {
    throw "DATABASE_ADMIN_URL menunjuk project Supabase $adminProjectRef, sedangkan secret installer dibuat untuk $projectRef. Masukkan URL owner project yang benar."
}

if (-not $reuseEnvironment) {
    $env:DATABASE_ADMIN_URL = $adminDatabaseUrl
    $env:SUPABASE_PROJECT_REF = $projectRef
    $env:ALGENTRA_API_PASSWORD = $apiRoleCredential
    try {
        Push-Location $ProjectRoot
        try { $validationOutput = & go run ./cmd/setup-production-db -validate-only }
        finally { Pop-Location }
        if ($LASTEXITCODE -ne 0) { throw 'Validasi koneksi Supabase gagal. Periksa URL owner, password, TLS, dan akses jaringan sebelum build.' }
        $validation = (($validationOutput -join "`n") | ConvertFrom-Json -ErrorAction Stop)
        if (-not $validation.validated) { throw 'Koneksi Supabase belum berhasil divalidasi.' }
    } finally {
        Remove-Item Env:DATABASE_ADMIN_URL, Env:SUPABASE_PROJECT_REF, Env:ALGENTRA_API_PASSWORD -ErrorAction SilentlyContinue
        $validationOutput = $null
        $validation = $null
    }
}

& (Join-Path $ProjectRoot 'infra\windows\install-mt5-and-compile-eas.ps1') -ProjectRoot $ProjectRoot -Mt5Root (Join-Path $installRoot 'mt5') -DownloadsRoot $downloadsRoot -LogRoot (Join-Path $installRoot 'logs\mt5-compile')
foreach ($path in @((Join-Path $installRoot 'signals'), (Join-Path $installRoot 'mt5\followers'), (Join-Path $ProjectRoot 'apps\copier-worker\config\runtime'))) {
    New-Item -ItemType Directory -Path $path -Force | Out-Null
}

$buildBackend = $existingServices.Count -eq 0 -or -not (Test-Path -LiteralPath $backendBinary -PathType Leaf)
$buildWeb = $existingServices.Count -eq 0 -or -not (Test-Path -LiteralPath $webBuildId -PathType Leaf) -or -not (Test-Path -LiteralPath $nextCli -PathType Leaf)
if (-not $buildBackend -and -not $buildWeb) {
    Write-Output 'Artifact production sudah tersedia; wizard melanjutkan instalasi service yang belum siap.'
} else {
    Push-Location $ProjectRoot
    try {
        if ($buildBackend) {
            $env:CGO_ENABLED = '0'
            & go build -ldflags='-s -w' -o $backendBinary ./cmd/algentra-backend
            if ($LASTEXITCODE -ne 0) { throw 'Build Core API gagal.' }
        }
        if ($buildWeb) {
            Push-Location $webRoot
            try {
                & npm ci
                if ($LASTEXITCODE -ne 0) { throw 'Install dependency portal gagal.' }
                & npm run build
                if ($LASTEXITCODE -ne 0) { throw 'Build portal gagal.' }
            } finally { Pop-Location }
        }
    } finally { Pop-Location }
}

foreach ($workerBuild in @(
    @{ Output = 'algentra-copier-master.exe'; Package = './cmd/copier-master' },
    @{ Output = 'algentra-copier-launcher.exe'; Package = './cmd/copier-launcher' },
    @{ Output = 'algentra-copier-worker.exe'; Package = './cmd/copier-worker' },
    @{ Output = 'algentra-mt5-login-probe.exe'; Package = './cmd/mt5-login-probe' }
)) {
    Push-Location $ProjectRoot
    try {
        & go build -ldflags='-s -w' -o (Join-Path $ProjectRoot $workerBuild.Output) $workerBuild.Package
        if ($LASTEXITCODE -ne 0) { throw "Build $($workerBuild.Output) gagal." }
    } finally { Pop-Location }
}

if (-not $reuseEnvironment) {
    $env:DATABASE_ADMIN_URL = $adminDatabaseUrl
    $env:SUPABASE_PROJECT_REF = $projectRef
    $env:ALGENTRA_API_PASSWORD = $apiRoleCredential
    try {
        Push-Location $ProjectRoot
        try { $setupOutput = & go run ./cmd/setup-production-db }
        finally { Pop-Location }
        if ($LASTEXITCODE -ne 0) { throw 'Pembuatan schema production gagal.' }
        $databaseUrl = (($setupOutput -join "`n") | ConvertFrom-Json -ErrorAction Stop).database_url
        if (-not $databaseUrl) { throw 'Installer database tidak menghasilkan DATABASE_URL runtime.' }
    } finally {
        Remove-Item Env:DATABASE_ADMIN_URL, Env:ALGENTRA_API_PASSWORD -ErrorAction SilentlyContinue
        $adminDatabaseUrl = $null
        $apiRoleCredential = $null
    }
    $envLines = @(
        "DATABASE_URL=$databaseUrl",
        "ADMIN_PASSWORD=$adminPassword"
    )
    [IO.File]::WriteAllLines($envFile, $envLines, [Text.UTF8Encoding]::new($false))
    & icacls.exe $envFile /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL file environment.' }
    Remove-Item -LiteralPath $pendingFile -Force
    $installConfiguration.database_admin_url = ''
    Save-ProtectedInstallConfiguration $installConfiguration $installConfigurationPath
    $envLines = $null
    $databaseUrl = $null
    $adminDatabaseUrl = $null
    $apiRoleCredential = $null
}

& (Join-Path $ProjectRoot 'infra\windows\install-core-api-service.ps1') -ProjectRoot $ProjectRoot -EnvironmentFile $envFile -NssmPath $tools.Nssm
$dataDirectory = Join-Path $ProjectRoot 'backend\data'
New-Item -ItemType Directory -Path $dataDirectory -Force | Out-Null
& icacls.exe $dataDirectory /grant 'NT SERVICE\AlgentraCoreApi:(OI)(CI)M' /T /C | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal memberi hak tulis service API pada folder data.' }
$integrationSecretsDirectory = Join-Path $dataDirectory 'integration-secrets'
New-Item -ItemType Directory -Path $integrationSecretsDirectory -Force | Out-Null
& icacls.exe $integrationSecretsDirectory /inheritance:r /grant:r 'SYSTEM:(OI)(CI)F' 'BUILTIN\Administrators:(OI)(CI)F' 'NT SERVICE\AlgentraCoreApi:(OI)(CI)M' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL folder rahasia integrasi.' }
$telegramSettingsPath = Join-Path $integrationSecretsDirectory 'telegram.json'
$telegramSettings = [pscustomobject]@{ api_id = 0; api_hash = ''; bot_token = ''; admin_chat_id = '' }
if (Test-Path -LiteralPath $telegramSettingsPath -PathType Leaf) {
    $storedTelegramSettings = Get-Content -LiteralPath $telegramSettingsPath -Raw | ConvertFrom-Json -ErrorAction Stop
    foreach ($name in @('api_id', 'api_hash', 'bot_token', 'admin_chat_id')) {
        if ($null -ne $storedTelegramSettings.$name) { $telegramSettings.$name = $storedTelegramSettings.$name }
    }
}
$legacyEnvironmentLines = @(Get-Content -LiteralPath $envFile)
foreach ($entry in @(
    @{ Name = 'TELEGRAM_API_ID'; Property = 'api_id' },
    @{ Name = 'TELEGRAM_API_HASH'; Property = 'api_hash' },
    @{ Name = 'TELEGRAM_BOT_TOKEN'; Property = 'bot_token' },
    @{ Name = 'TELEGRAM_ADMIN_CHAT_ID'; Property = 'admin_chat_id' }
)) {
    $pattern = '^\s*' + [regex]::Escape($entry.Name) + '\s*=(.*)$'
    $legacyLine = $legacyEnvironmentLines | Where-Object { $_ -match $pattern } | Select-Object -Last 1
    if ($legacyLine -and -not $telegramSettings.($entry.Property)) {
        $telegramSettings.($entry.Property) = $legacyLine.Substring($legacyLine.IndexOf('=') + 1)
    }
    $providedValue = switch ($entry.Property) {
        'api_id' { [string]$installConfiguration.telegram_api_id }
        'api_hash' { [string]$installConfiguration.telegram_api_hash }
        'bot_token' { [string]$installConfiguration.telegram_bot_token }
        'admin_chat_id' { [string]$installConfiguration.telegram_admin_chat_id }
    }
    if ($providedValue) { $telegramSettings.($entry.Property) = $providedValue }
}
if ($telegramSettings.api_id) { $telegramSettings.api_id = [int]$telegramSettings.api_id }
[IO.File]::WriteAllText($telegramSettingsPath, ($telegramSettings | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
& icacls.exe $telegramSettingsPath /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' 'NT SERVICE\AlgentraCoreApi:R' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL konfigurasi Telegram.' }
$environmentWithoutTelegram = @($legacyEnvironmentLines | Where-Object { $_ -notmatch '^\s*TELEGRAM_(?:API_ID|API_HASH|BOT_TOKEN|ADMIN_CHAT_ID)\s*=' })
if ($environmentWithoutTelegram.Count -ne $legacyEnvironmentLines.Count) {
    [IO.File]::WriteAllLines($envFile, $environmentWithoutTelegram, [Text.UTF8Encoding]::new($false))
    & icacls.exe $envFile /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' 'NT SERVICE\AlgentraCoreApi:R' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Gagal membersihkan kredensial Telegram dari environment.' }
}
$storedTelegramSettings = $null
$telegramSettings = $null
$legacyEnvironmentLines = $null
$environmentWithoutTelegram = $null
$optionalIntegrationsPath = Join-Path $integrationSecretsDirectory 'optional-integrations.json'
$previousOptionalIntegrations = if (Test-Path -LiteralPath $optionalIntegrationsPath -PathType Leaf) {
    Get-Content -LiteralPath $optionalIntegrationsPath -Raw | ConvertFrom-Json -ErrorAction Stop
} else { $null }
$midtransServerKey = [string]$installConfiguration.midtrans_server_key
if (-not $midtransServerKey -and $previousOptionalIntegrations) { $midtransServerKey = [string]$previousOptionalIntegrations.midtrans_server_key }
if (-not $midtransServerKey) { $midtransServerKey = Get-EnvironmentValue 'MIDTRANS_SERVER_KEY' $envFile }
$midtransMerchantId = [string]$installConfiguration.midtrans_merchant_id
if (-not $midtransMerchantId -and $previousOptionalIntegrations) { $midtransMerchantId = [string]$previousOptionalIntegrations.midtrans_merchant_id }
if (-not $midtransMerchantId) { $midtransMerchantId = Get-EnvironmentValue 'MIDTRANS_MERCHANT_ID' $envFile }
$midtransProduction = [bool]$installConfiguration.midtrans_production
if (-not $installConfiguration.midtrans_server_key -and $previousOptionalIntegrations) { $midtransProduction = [bool]$previousOptionalIntegrations.midtrans_production }
if (-not $installConfiguration.midtrans_server_key -and -not $previousOptionalIntegrations) { $midtransProduction = (Get-EnvironmentValue 'MIDTRANS_IS_PRODUCTION' $envFile) -match '^(?i:true|1|yes)$' }
$paymentBankName = [string]$installConfiguration.payment_bank_name
if (-not $paymentBankName -and $previousOptionalIntegrations) { $paymentBankName = [string]$previousOptionalIntegrations.payment_bank_name }
if (-not $paymentBankName) { $paymentBankName = Get-EnvironmentValue 'PAYMENT_BANK_NAME' $envFile }
$paymentNumber = [string]$installConfiguration.payment_number
if (-not $paymentNumber -and $previousOptionalIntegrations) { $paymentNumber = [string]$previousOptionalIntegrations.payment_number }
if (-not $paymentNumber) { $paymentNumber = Get-EnvironmentValue 'PAYMENT_NUMBER' $envFile }
$paymentName = [string]$installConfiguration.payment_name
if (-not $paymentName -and $previousOptionalIntegrations) { $paymentName = [string]$previousOptionalIntegrations.payment_name }
if (-not $paymentName) { $paymentName = Get-EnvironmentValue 'PAYMENT_NAME' $envFile }
$smtpHost = [string]$installConfiguration.email_smtp_host
if (-not $smtpHost -and $previousOptionalIntegrations) { $smtpHost = [string]$previousOptionalIntegrations.email_smtp_host }
if (-not $smtpHost) { $smtpHost = Get-EnvironmentValue 'EMAIL_SMTP_HOST' $envFile }
$smtpPort = ''
if ($installConfiguration.email_smtp_host) { $smtpPort = [string]$installConfiguration.email_smtp_port }
elseif ($previousOptionalIntegrations -and $previousOptionalIntegrations.email_smtp_host) { $smtpPort = [string]$previousOptionalIntegrations.email_smtp_port }
else { $smtpPort = Get-EnvironmentValue 'EMAIL_SMTP_PORT' $envFile }
if (-not $smtpPort) { $smtpPort = '587' }
$smtpUsername = [string]$installConfiguration.email_smtp_username
if (-not $smtpUsername -and $previousOptionalIntegrations) { $smtpUsername = [string]$previousOptionalIntegrations.email_smtp_username }
if (-not $smtpUsername) { $smtpUsername = Get-EnvironmentValue 'EMAIL_SMTP_USERNAME' $envFile }
$smtpPassword = [string]$installConfiguration.email_smtp_password
if (-not $smtpPassword -and $previousOptionalIntegrations) { $smtpPassword = [string]$previousOptionalIntegrations.email_smtp_password }
if (-not $smtpPassword) { $smtpPassword = Get-EnvironmentValue 'EMAIL_SMTP_PASSWORD' $envFile }
$smtpFrom = [string]$installConfiguration.email_from
if (-not $smtpFrom -and $previousOptionalIntegrations) { $smtpFrom = [string]$previousOptionalIntegrations.email_from }
if (-not $smtpFrom) { $smtpFrom = Get-EnvironmentValue 'EMAIL_FROM' $envFile }
$smtpStartTls = [bool]$installConfiguration.email_smtp_starttls
if (-not $installConfiguration.email_smtp_host -and $previousOptionalIntegrations -and $null -ne $previousOptionalIntegrations.email_smtp_starttls) { $smtpStartTls = [bool]$previousOptionalIntegrations.email_smtp_starttls }
if (-not $installConfiguration.email_smtp_host -and -not $previousOptionalIntegrations) { $smtpStartTls = (Get-EnvironmentValue 'EMAIL_SMTP_STARTTLS' $envFile) -notmatch '^(?i:false|0|no)$' }
$optionalIntegrations = [pscustomobject]@{
    payment_bank_name = $paymentBankName
    payment_number = $paymentNumber
    payment_name = $paymentName
    midtrans_server_key = $midtransServerKey
    midtrans_merchant_id = $midtransMerchantId
    midtrans_production = $midtransProduction
    midtrans_notification_url = if ($midtransServerKey) { "https://$Domain/api/payments/midtrans/notification" } else { '' }
    email_smtp_host = $smtpHost
    email_smtp_port = [int]$smtpPort
    email_smtp_username = $smtpUsername
    email_smtp_password = $smtpPassword
    email_from = $smtpFrom
    email_smtp_starttls = $smtpStartTls
}
[IO.File]::WriteAllText($optionalIntegrationsPath, ($optionalIntegrations | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
& icacls.exe $optionalIntegrationsPath /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' 'NT SERVICE\AlgentraCoreApi:R' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL konfigurasi pembayaran dan email.' }
$environmentBeforeIntegrationCleanup = @(Get-Content -LiteralPath $envFile)
$environmentWithoutOptionalIntegrations = @($environmentBeforeIntegrationCleanup | Where-Object { $_ -notmatch '^\s*(?:PAYMENT_(?:BANK_NAME|NUMBER|NAME)|MIDTRANS_(?:SERVER_KEY|MERCHANT_ID|IS_PRODUCTION|NOTIFICATION_URL)|EMAIL_SMTP_(?:HOST|PORT|USERNAME|PASSWORD|STARTTLS)|EMAIL_FROM)\s*=' })
if ($environmentWithoutOptionalIntegrations.Count -ne $environmentBeforeIntegrationCleanup.Count) {
    [IO.File]::WriteAllLines($envFile, $environmentWithoutOptionalIntegrations, [Text.UTF8Encoding]::new($false))
    & icacls.exe $envFile /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' 'NT SERVICE\AlgentraCoreApi:R' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Gagal membersihkan konfigurasi pembayaran dan email dari environment.' }
}
Remove-Item Env:PAYMENT_BANK_NAME, Env:PAYMENT_NUMBER, Env:PAYMENT_NAME, Env:MIDTRANS_SERVER_KEY, Env:MIDTRANS_MERCHANT_ID, Env:MIDTRANS_IS_PRODUCTION, Env:MIDTRANS_NOTIFICATION_URL, Env:EMAIL_SMTP_HOST, Env:EMAIL_SMTP_PORT, Env:EMAIL_SMTP_USERNAME, Env:EMAIL_SMTP_PASSWORD, Env:EMAIL_FROM, Env:EMAIL_SMTP_STARTTLS -ErrorAction SilentlyContinue
$optionalIntegrations = $null
$previousOptionalIntegrations = $null
$midtransServerKey = $null
$smtpPassword = $null
$environmentBeforeIntegrationCleanup = $null
$environmentWithoutOptionalIntegrations = $null
$geminiSettingsPath = Join-Path $integrationSecretsDirectory 'gemini.json'
if (-not (Test-Path -LiteralPath $geminiSettingsPath -PathType Leaf)) {
    $geminiSettings = [pscustomobject]@{
        api_key = [string]$installConfiguration.gemini_api_key
        model = [string]$installConfiguration.gemini_model
    }
    [IO.File]::WriteAllText($geminiSettingsPath, ($geminiSettings | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
    & icacls.exe $geminiSettingsPath /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' 'NT SERVICE\AlgentraCoreApi:R' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL pengaturan Gemini.' }
    $geminiSettings = $null
}
if ((Get-Service -Name 'AlgentraCoreApi').Status -ne 'Running') { Start-Service AlgentraCoreApi }
$ready = $false
for ($attempt = 0; $attempt -lt 30; $attempt++) {
    try {
        $result = Invoke-RestMethod 'http://127.0.0.1:8000/ready' -TimeoutSec 3
        if ($result.status -eq 'ready') { $ready = $true; break }
    } catch { Start-Sleep -Seconds 2 }
    Start-Sleep -Seconds 2
}
if (-not $ready) { throw "Core API belum siap. Periksa $ProjectRoot\logs\core-api\stderr.log dan schema Supabase." }

if ($adminPassword) {
    Write-Output "Password awal admin (simpan sekarang): $adminPassword"
} else {
    Write-Output 'Password admin tidak berubah. Gunakan password yang sudah tersimpan; installer tidak dapat membacanya kembali.'
}

# The API migrates legacy secrets and bootstraps the admin hash before reporting
# ready. Remove one-time secrets from the runtime environment after that step.
$originalEnvLines = @(Get-Content -LiteralPath $envFile)
$envWithoutBootstrapSecrets = @($originalEnvLines | Where-Object { $_ -notmatch '^\s*(?:EA_API_KEY|ADMIN_PASSWORD|APP_SECRET_KEY)\s*=' })
if ($envWithoutBootstrapSecrets.Count -ne $originalEnvLines.Count) {
    [IO.File]::WriteAllLines($envFile, $envWithoutBootstrapSecrets, [Text.UTF8Encoding]::new($false))
    & icacls.exe $envFile /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' 'NT SERVICE\AlgentraCoreApi:R' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Gagal mempertahankan ACL environment setelah menghapus secret bootstrap.' }
}
$originalEnvLines = $null
$envWithoutBootstrapSecrets = $null
Remove-Item Env:EA_API_KEY, Env:ADMIN_PASSWORD, Env:APP_SECRET_KEY -ErrorAction SilentlyContinue

& (Join-Path $ProjectRoot 'infra\windows\configure-master-mt5.ps1') `
    -ProjectRoot $ProjectRoot `
    -InstallRoot $installRoot `
    -Login ([string]$installConfiguration.broker_login) `
    -Password ([string]$installConfiguration.broker_password) `
    -Server ([string]$installConfiguration.broker_server) `
    -AllowLiveTrading ([bool]$installConfiguration.allow_live_trading)
$installConfiguration.broker_password = $null

if (Test-Path -LiteralPath $cloudflareStatePath -PathType Leaf) {
    $cloudflareState = Get-Content -LiteralPath $cloudflareStatePath -Raw | ConvertFrom-Json -ErrorAction Stop
} else { $cloudflareState = $null }
if (-not $cloudflareState -or -not $cloudflareState.provisioned -or -not (Test-Path -LiteralPath $cloudflareTokenPath -PathType Leaf)) {
    try {
        & (Join-Path $ProjectRoot 'infra\windows\provision-cloudflare-tunnel.ps1') -Hostname $Domain -ApiToken $CloudflareApiToken -StatePath $cloudflareStatePath -TunnelTokenPath $cloudflareTokenPath
    } finally { $CloudflareApiToken = $null }
    & icacls.exe $cloudflareStatePath /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL state Cloudflare.' }
    & icacls.exe $cloudflareTokenPath /inheritance:r /grant:r 'SYSTEM:F' 'BUILTIN\Administrators:F' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL token Cloudflare Tunnel.' }
}
if ($cloudflareState.hostname -ne $Domain) { throw 'Hostname tidak cocok dengan Cloudflare Tunnel yang sudah dibuat.' }
$installConfiguration.cloudflare_api_token = ''
Save-ProtectedInstallConfiguration $installConfiguration $installConfigurationPath
& (Join-Path $ProjectRoot 'infra\windows\install-web-and-tunnel-services.ps1') -ProjectRoot $ProjectRoot -Domain $Domain -NssmPath $tools.Nssm -CloudflaredPath $tools.Cloudflared -TunnelTokenPath $cloudflareTokenPath

$portalReady = $false
for ($attempt = 0; $attempt -lt 40; $attempt++) {
    try {
        $localPortal = Invoke-WebRequest 'http://127.0.0.1:3000/' -UseBasicParsing -TimeoutSec 5
        $publicPortal = Invoke-WebRequest "https://$Domain/" -UseBasicParsing -TimeoutSec 8
        $publicReady = Invoke-RestMethod "https://$Domain/ready" -TimeoutSec 8
        if ($localPortal.StatusCode -eq 200 -and $publicPortal.StatusCode -eq 200 -and $publicReady.status -eq 'ready') {
            $portalReady = $true
            break
        }
    } catch { Start-Sleep -Seconds 3 }
    Start-Sleep -Seconds 3
}
if (-not $portalReady) {
    throw "Service sudah terdaftar, tetapi https://$Domain/ready belum siap. Periksa konfigurasi Tunnel (origin http://127.0.0.1:3000), DNS Cloudflare, AlgentraCloudflared, dan log; wizard dapat dijalankan ulang setelah masalah diperbaiki."
}

if ($installConfiguration.telegram_api_id) {
    $telegramAdminPassword = $adminPassword
    if (-not $telegramAdminPassword) { $telegramAdminPassword = Read-SecureText 'Password admin portal untuk menyelesaikan setup Telegram' }
    & (Join-Path $ProjectRoot 'infra\windows\configure-telegram.ps1') `
        -PortalUrl "https://$Domain" `
        -AdminPassword $telegramAdminPassword `
        -Phone ([string]$installConfiguration.telegram_phone)
    $telegramAdminPassword = $null
}

$workerTaskInstaller = Join-Path $ProjectRoot 'apps\copier-worker\scripts\install_managed_worker_task.ps1'
& $workerTaskInstaller -ProjectRoot $ProjectRoot -ApiBaseUrl "https://$Domain" -TerminalTemplateDir (Join-Path $installRoot 'mt5\follower-template') -SignalDirectory (Join-Path $installRoot 'signals') -RuntimeDirectory (Join-Path $ProjectRoot 'apps\copier-worker\config\runtime')
if ($LASTEXITCODE -ne 0) { throw 'Pemasangan managed copier task gagal. Jalankan kembali one-click installer setelah memperbaiki error worker.' }

Remove-Item -LiteralPath $installConfigurationPath -Force -ErrorAction SilentlyContinue
$installConfiguration.gemini_api_key = $null
$installConfiguration.telegram_api_hash = $null
$installConfiguration.telegram_bot_token = $null
$installConfiguration.broker_password = $null
$installConfiguration.midtrans_server_key = $null
$installConfiguration.email_smtp_password = $null
$installConfiguration = $null
$geminiApiKey = $null
$telegramApiHash = $null
$telegramBotToken = $null
$brokerPassword = $null
$midtransServerKey = $null
$smtpPassword = $null
$optionalIntegrations = $null

Write-Output "Instalasi selesai: https://$Domain"
Write-Output 'Admin login: admin'
Write-Output 'EA API key dibuat otomatis; salin dari Dashboard > Settings setelah login.'
Write-Output "Environment rahasia tersimpan di: $envFile"
Write-Output 'Managed copier worker task terpasang dan dijalankan otomatis saat sesi Windows interaktif.'
Write-Output 'Installer membuat schema pada project Supabase baru; project dan kredensial owner tetap perlu disiapkan lebih dahulu.'
$adminPassword = $null
