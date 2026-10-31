param(
    [Parameter(Mandatory = $true)] [string] $PortalUrl,
    [Parameter(Mandatory = $true)] [string] $AdminPassword,
    [Parameter(Mandatory = $true)] [string] $Phone
)

$ErrorActionPreference = 'Stop'
$base = $PortalUrl.TrimEnd('/')
$session = [Microsoft.PowerShell.Commands.WebRequestSession]::new()
$jsonHeaders = @{ 'Content-Type' = 'application/json' }
$csrfHeaders = @{}

try {
    $loginBody = @{ username = 'admin'; password = $AdminPassword } | ConvertTo-Json -Compress
    $login = Invoke-RestMethod -Uri "$base/api/core/auth/login" -Method Post -WebSession $session -Headers $jsonHeaders -Body $loginBody -TimeoutSec 30
    $csrfHeaders['X-CSRF-Token'] = [string]$login.csrf_token
    if (-not $csrfHeaders['X-CSRF-Token']) { throw 'Portal tidak mengembalikan token sesi admin untuk setup Telegram.' }

    $status = Invoke-RestMethod -Uri "$base/api/core/telegram/status" -Method Get -WebSession $session -TimeoutSec 30
    if (-not $status.connected) {
        if ([string]::IsNullOrWhiteSpace($Phone)) {
            $Phone = (Read-Host 'Nomor telepon akun Telegram sumber (format internasional, contoh +628123456789)').Trim()
        }
        if ($Phone -notmatch '^\+[1-9][0-9]{7,14}$') { throw 'Nomor telepon Telegram harus memakai format internasional.' }
        $sendCodeBody = @{ phone = $Phone } | ConvertTo-Json -Compress
        $sent = Invoke-RestMethod -Uri "$base/api/core/telegram/send-code" -Method Post -WebSession $session -Headers ($jsonHeaders + $csrfHeaders) -Body $sendCodeBody -TimeoutSec 30
        Write-Output "Kode login Telegram dikirim ke $($sent.phone)."
        $code = (Read-Host 'Masukkan kode verifikasi Telegram').Trim()
        if (-not $code) { throw 'Kode verifikasi Telegram wajib diisi.' }
        $verifyCodeBody = @{ code = $code } | ConvertTo-Json -Compress
        $verified = Invoke-RestMethod -Uri "$base/api/core/telegram/verify-code" -Method Post -WebSession $session -Headers ($jsonHeaders + $csrfHeaders) -Body $verifyCodeBody -TimeoutSec 30
        if ($verified.requires_2fa) {
            $twoFactorPassword = Read-Host 'Password Telegram 2FA' -AsSecureString
            $passwordPointer = [IntPtr]::Zero
            try {
                $passwordPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($twoFactorPassword)
                $twoFactorText = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPointer)
                $verifyPasswordBody = @{ password = $twoFactorText } | ConvertTo-Json -Compress
                Invoke-RestMethod -Uri "$base/api/core/telegram/verify-2fa" -Method Post -WebSession $session -Headers ($jsonHeaders + $csrfHeaders) -Body $verifyPasswordBody -TimeoutSec 30 | Out-Null
            } finally {
                if ($passwordPointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPointer) }
                $twoFactorText = $null
                $verifyPasswordBody = $null
                $twoFactorPassword.Dispose()
            }
        }
    }

    $groups = @(Invoke-RestMethod -Uri "$base/api/core/telegram/groups" -Method Get -WebSession $session -TimeoutSec 30)
    if ($groups.Count -eq 0) {
        Write-Output 'Telegram terhubung, tetapi tidak ada grup/channel yang dapat dipilih.'
        return
    }
    Write-Output 'Grup/channel Telegram yang tersedia:'
    foreach ($group in $groups) { Write-Output ("- {0} [{1}]" -f $group.name, $group.chat_id) }
    $selectionText = Read-Host 'Masukkan chat ID sumber sinyal yang diizinkan (pisahkan dengan koma; kosongkan untuk menonaktifkan semua)'
    $selectedIds = @($selectionText -split '[,;\s]+' | Where-Object { $_ } | Select-Object -Unique)
    $selection = @($groups | ForEach-Object {
        $chatId = [string]$_.chat_id
        [pscustomobject]@{ chat_id = $chatId; enabled = ($selectedIds -contains $chatId); alias = $null }
    })
    $selectBody = @{ groups = $selection } | ConvertTo-Json -Depth 5 -Compress
    Invoke-RestMethod -Uri "$base/api/core/telegram/groups/select" -Method Post -WebSession $session -Headers ($jsonHeaders + $csrfHeaders) -Body $selectBody -TimeoutSec 30 | Out-Null
    $enabledCount = @($selection | Where-Object enabled).Count
    Write-Output "Setup Telegram selesai; $enabledCount sumber sinyal diaktifkan."
} finally {
    $AdminPassword = $null
    $loginBody = $null
    $sendCodeBody = $null
    $verifyCodeBody = $null
    $verifyPasswordBody = $null
    $twoFactorText = $null
    $session = $null
}
