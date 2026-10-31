param(
    [Parameter(Mandatory = $true)] [string]$LauncherPath,
    [Parameter(Mandatory = $true)] [string]$ConfigPath,
    [Parameter(Mandatory = $true)] [string]$SecretPath,
    [Parameter(Mandatory = $true)] [string]$TempDirectory,
    [Parameter(Mandatory = $true)] [string]$ProjectRoot
)

$ErrorActionPreference = 'Stop'
$LauncherPath = (Resolve-Path -LiteralPath $LauncherPath).Path
$ConfigPath = (Resolve-Path -LiteralPath $ConfigPath).Path
$SecretPath = (Resolve-Path -LiteralPath $SecretPath).Path
$TempDirectory = [IO.Path]::GetFullPath($TempDirectory)
$ProjectRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
try {
    $publisherToken = [string](Get-Content -LiteralPath $SecretPath -Raw | ConvertFrom-Json -ErrorAction Stop).publisher_token
} catch {
    throw "Token publisher tidak dapat dibaca dari file rahasia lokal: $SecretPath"
}
if ($publisherToken.Length -lt 32) { throw 'Token publisher lokal tidak valid.' }

$env:MASTER_PUBLISHER_TOKEN = $publisherToken
$env:TEMP = $TempDirectory
$env:TMP = $TempDirectory
$publisherToken = $null
Set-Location -LiteralPath $ProjectRoot
$launcherExitCode = 1
try {
    & $LauncherPath $ConfigPath
    $launcherExitCode = $LASTEXITCODE
} finally {
    Remove-Item Env:MASTER_PUBLISHER_TOKEN -ErrorAction SilentlyContinue
}
exit $launcherExitCode
