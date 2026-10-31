param(
    [Parameter(Mandatory = $true)] [string] $TerminalPath,
    [Parameter(Mandatory = $true)] [string] $StartupTemplatePath,
    [Parameter(Mandatory = $true)] [string] $EncryptedPasswordPath,
    [switch] $WaitForStartup
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'Clear-ApplicationEnvironment.ps1')
Clear-AlgentraApplicationEnvironment

$TerminalPath = (Resolve-Path -LiteralPath $TerminalPath).Path
$StartupTemplatePath = (Resolve-Path -LiteralPath $StartupTemplatePath).Path
$EncryptedPasswordPath = (Resolve-Path -LiteralPath $EncryptedPasswordPath).Path
$securePassword = ConvertTo-SecureString -String (Get-Content -LiteralPath $EncryptedPasswordPath -Raw).Trim()
$passwordPointer = [IntPtr]::Zero
$password = ''
$configPath = Join-Path (Split-Path -Parent $StartupTemplatePath) ('algentra-startup-' + [guid]::NewGuid().ToString('N') + '.ini')
try {
    $passwordPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)
    $password = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPointer)
    $template = [IO.File]::ReadAllText($StartupTemplatePath)
    if ($template -notmatch '(?m)^Password=__ALGENTRA_PASSWORD__$') { throw 'Template startup MT5 tidak memiliki penanda password yang diharapkan.' }
    $passwordMarker = [regex]::new('(?m)^Password=__ALGENTRA_PASSWORD__$')
    $activeConfig = $passwordMarker.Replace($template, ('Password=' + $password), 1)
    [IO.File]::WriteAllText($configPath, $activeConfig, [Text.UTF8Encoding]::new($false))
    $activeConfig = $null

    $terminalProcess = Start-Process -FilePath $TerminalPath `
        -ArgumentList @('/portable', "/config:$configPath") `
        -WorkingDirectory (Split-Path -Parent $TerminalPath) `
        -PassThru
    if ($WaitForStartup) {
        Start-Sleep -Seconds 25
        $terminalProcess.Refresh()
        if ($terminalProcess.HasExited) { throw 'Terminal master berhenti saat proses startup; periksa log MT5 di folder master.' }
    }
} finally {
    if ($passwordPointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPointer) }
    $password = $null
    $securePassword.Dispose()
    if (Test-Path -LiteralPath $configPath -PathType Leaf) {
        for ($attempt = 0; $attempt -lt 5; $attempt++) {
            try { Remove-Item -LiteralPath $configPath -Force -ErrorAction Stop; break }
            catch { if ($attempt -eq 4) { throw 'File startup MT5 berisi password masih terkunci dan tidak dapat dihapus.' }; Start-Sleep -Seconds 2 }
        }
    }
}
