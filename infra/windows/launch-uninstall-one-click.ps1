param([switch] $Elevated)

$ErrorActionPreference = 'Stop'
$uninstallerPath = Join-Path $PSScriptRoot 'uninstall-one-click.ps1'
if (-not (Test-Path -LiteralPath $uninstallerPath -PathType Leaf)) {
    throw "Uninstaller tidak ditemukan: $uninstallerPath"
}

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) -or $Elevated) {
    try {
        & $uninstallerPath
        $exitCode = 0
    } catch {
        Write-Error $_
        $exitCode = 1
    }
    Read-Host 'Tekan Enter untuk menutup uninstaller'
    exit $exitCode
}

$powerShellPath = (Get-Process -Id $PID).Path
$launcherPath = $PSCommandPath.Replace('"', '\"')
$arguments = "-NoLogo -NoProfile -ExecutionPolicy Bypass -File `"$launcherPath`" -Elevated"
$elevatedProcess = Start-Process -FilePath $powerShellPath -Verb RunAs -ArgumentList $arguments -PassThru
$elevatedProcess.WaitForExit()
exit $elevatedProcess.ExitCode
