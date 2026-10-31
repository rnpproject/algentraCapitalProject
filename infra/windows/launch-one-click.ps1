$ErrorActionPreference = 'Stop'
$installerPath = Join-Path $PSScriptRoot 'install-one-click.ps1'
if (-not (Test-Path -LiteralPath $installerPath -PathType Leaf)) {
    throw "Installer tidak ditemukan: $installerPath"
}

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
$isAdministrator = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if ($isAdministrator) {
    & $installerPath
    Read-Host 'Simpan password/key yang ditampilkan, lalu tekan Enter untuk menutup installer'
    exit 0
}

$powerShellPath = (Get-Process -Id $PID).Path
$quotedInstallerPath = '"' + $installerPath.Replace('"', '\"') + '"'
$arguments = "-NoLogo -NoProfile -ExecutionPolicy Bypass -NoExit -File $quotedInstallerPath"
$elevatedProcess = Start-Process -FilePath $powerShellPath -Verb RunAs -ArgumentList $arguments -PassThru
$elevatedProcess.WaitForExit()
exit $elevatedProcess.ExitCode
