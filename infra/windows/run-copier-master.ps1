param(
    [string] $ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path,
    [string] $ConfigPath
)

$ErrorActionPreference = 'Stop'
$root = (Resolve-Path -LiteralPath $ProjectRoot).Path
if ([string]::IsNullOrWhiteSpace($ConfigPath)) {
    $ConfigPath = Join-Path $root 'apps\copier-worker\config\master.json'
} elseif (-not [System.IO.Path]::IsPathRooted($ConfigPath)) {
    $ConfigPath = Join-Path $root $ConfigPath
}
$ConfigPath = (Resolve-Path -LiteralPath $ConfigPath).Path
$publisher = Join-Path $root 'algentra-copier-master.exe'
if (-not (Test-Path -LiteralPath $publisher -PathType Leaf)) {
    throw "Binary publisher Go tidak ditemukan: $publisher. Build dengan go build -o algentra-copier-master.exe ./cmd/copier-master terlebih dahulu."
}
if ([string]::IsNullOrWhiteSpace($env:MASTER_PUBLISHER_TOKEN) -or $env:MASTER_PUBLISHER_TOKEN.Length -lt 32) {
    throw 'MASTER_PUBLISHER_TOKEN harus tersedia pada environment akun Windows dan minimal 32 karakter.'
}
Set-Location -LiteralPath $root
& $publisher $ConfigPath
exit $LASTEXITCODE
