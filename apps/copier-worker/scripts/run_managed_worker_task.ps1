param(
    [Parameter(Mandatory = $true)] [string] $ProjectRoot,
    [Parameter(Mandatory = $true)] [string] $WorkerBinaryPath,
    [Parameter(Mandatory = $true)] [string] $ConfigPath,
    [Parameter(Mandatory = $true)] [string] $TempDirectory,
    [Parameter(Mandatory = $true)] [string] $SecretDirectory,
    [Parameter(Mandatory = $true)] [string] $ErrorLogPath
)

$ErrorActionPreference = 'Stop'
$ProjectRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
$WorkerBinaryPath = (Resolve-Path -LiteralPath $WorkerBinaryPath).Path
$ConfigPath = (Resolve-Path -LiteralPath $ConfigPath).Path
$TempDirectory = [IO.Path]::GetFullPath($TempDirectory)
New-Item -ItemType Directory -Path $TempDirectory -Force | Out-Null
$workerKeyPath = Join-Path $SecretDirectory 'worker.json'
$publisherTokenPath = Join-Path $SecretDirectory 'publisher.json'

try {
    $workerKey = [string](Get-Content -LiteralPath $workerKeyPath -Raw | ConvertFrom-Json -ErrorAction Stop).api_key
    $publisherToken = [string](Get-Content -LiteralPath $publisherTokenPath -Raw | ConvertFrom-Json -ErrorAction Stop).publisher_token
} catch {
    throw "Rahasia managed worker tidak dapat dibaca dari $SecretDirectory. Jalankan kembali installer task."
}
if ($workerKey.Length -lt 32 -or $publisherToken.Length -lt 32) {
    throw 'Rahasia managed worker lokal tidak valid. Jalankan kembali installer task.'
}

$env:APP_ENV = 'production'
$env:TEMP = $TempDirectory
$env:TMP = $TempDirectory
$env:COPIER_WORKER_API_KEY = $workerKey
$env:MASTER_PUBLISHER_TOKEN = $publisherToken
$workerKey = $null
$publisherToken = $null
Set-Location -LiteralPath $ProjectRoot
$workerExitCode = 1
try {
    & $WorkerBinaryPath $ConfigPath 2> $ErrorLogPath
    $workerExitCode = $LASTEXITCODE
} finally {
    Remove-Item Env:COPIER_WORKER_API_KEY, Env:MASTER_PUBLISHER_TOKEN -ErrorAction SilentlyContinue
}
exit $workerExitCode
