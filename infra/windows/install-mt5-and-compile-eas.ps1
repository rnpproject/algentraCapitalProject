param(
    [string] $ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path,
    [string] $Mt5Root = 'C:\Algentra\mt5',
    [string] $DownloadsRoot = 'C:\Algentra\downloads',
    [string] $LogRoot = 'C:\Algentra\logs\mt5-compile'
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$ProjectRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
$masterRoot = Join-Path $Mt5Root 'master'
$templateRoot = Join-Path $Mt5Root 'follower-template'
$installerPath = Join-Path $DownloadsRoot 'mt5setup.exe'
foreach ($directory in @($Mt5Root, $DownloadsRoot, $LogRoot)) {
    New-Item -ItemType Directory -Path $directory -Force | Out-Null
}

function Install-MetaTrader([string] $Destination) {
    $terminal = Join-Path $Destination 'terminal64.exe'
    if (Test-Path -LiteralPath $terminal -PathType Leaf) { return }

    if (-not (Test-Path -LiteralPath $installerPath -PathType Leaf)) {
        Invoke-WebRequest -Uri 'https://download.terminal.free/cdn/web/metaquotes.ltd/mt5/mt5setup.exe' -OutFile $installerPath
    }
    $signature = Get-AuthenticodeSignature -FilePath $installerPath
    if ($signature.Status -ne [System.Management.Automation.SignatureStatus]::Valid) {
        throw "Tanda tangan installer MetaTrader tidak valid: $($signature.Status)."
    }

    New-Item -ItemType Directory -Path $Destination -Force | Out-Null
    Write-Output "Memasang MetaTrader 5 ke $Destination..."
    $pathArgument = '/path:"' + $Destination + '"'
    $process = Start-Process -FilePath $installerPath -ArgumentList @('/auto', $pathArgument) -Wait -PassThru -WindowStyle Hidden
    if ($process.ExitCode -ne 0) { throw "Installer MetaTrader gagal (exit $($process.ExitCode)) pada $Destination." }
    if (-not (Test-Path -LiteralPath $terminal -PathType Leaf)) {
        throw "MetaTrader tidak menghasilkan terminal64.exe di $Destination."
    }
}

Install-MetaTrader $masterRoot
Install-MetaTrader $templateRoot
$metaEditor = Join-Path $masterRoot 'metaeditor64.exe'
if (-not (Test-Path -LiteralPath $metaEditor -PathType Leaf)) {
    throw "MetaEditor tidak ditemukan: $metaEditor. Pastikan installer MetaTrader menyelesaikan pemasangan penuh."
}

$expertDefinitions = @(
    @{ Source = 'mt5\experts\TelegramSignalEA.mq5'; Target = 'MQL5\Experts\TelegramSignalEA.mq5'; Compiled = 'mt5\experts\TelegramSignalEA.ex5' },
    @{ Source = 'mt5\experts\MT5MasterPublisherEA.mq5'; Target = 'MQL5\Experts\MT5MasterPublisherEA.mq5'; Compiled = 'mt5\experts\MT5MasterPublisherEA.ex5' },
    @{ Source = 'mt5\experts\MT5FollowerCopyEA.mq5'; Target = 'MQL5\Experts\MT5FollowerCopyEA.mq5'; Compiled = 'mt5\experts\MT5FollowerCopyEA.ex5' },
    @{ Source = 'mt5\experts\Algentra\MT5LoginProbe.mq5'; Target = 'MQL5\Experts\Algentra\MT5LoginProbe.mq5'; Compiled = 'mt5\experts\Algentra\MT5LoginProbe.ex5' }
)
$compiledOutputs = @{}
foreach ($expert in $expertDefinitions) {
    $source = Join-Path $ProjectRoot $expert.Source
    if (-not (Test-Path -LiteralPath $source -PathType Leaf)) { throw "Source EA tidak ditemukan: $source" }
    $installedSource = Join-Path $masterRoot $expert.Target
    New-Item -ItemType Directory -Path (Split-Path -Parent $installedSource) -Force | Out-Null
    Copy-Item -LiteralPath $source -Destination $installedSource -Force
    $binary = [IO.Path]::ChangeExtension($installedSource, '.ex5')
    $compileLog = "$installedSource.log"
    Remove-Item -LiteralPath $binary, $compileLog -Force -ErrorAction SilentlyContinue

    Write-Output "Compile EA: $([IO.Path]::GetFileName($source))"
    $arguments = '/compile:"{0}" /log /include:"{1}"' -f $installedSource, (Join-Path $masterRoot 'MQL5')
    $process = Start-Process -FilePath $metaEditor -ArgumentList $arguments -Wait -PassThru -WindowStyle Hidden
    $savedLog = Join-Path $LogRoot ([IO.Path]::GetFileNameWithoutExtension($source) + '.log')
    if (Test-Path -LiteralPath $compileLog -PathType Leaf) {
        Move-Item -LiteralPath $compileLog -Destination $savedLog -Force
    }
    if ($process.ExitCode -ne 0 -or -not (Test-Path -LiteralPath $binary -PathType Leaf) -or (Get-Item -LiteralPath $binary).Length -eq 0) {
        $details = if (Test-Path -LiteralPath $savedLog -PathType Leaf) { Get-Content -LiteralPath $savedLog -Raw } else { 'Log MetaEditor tidak tersedia.' }
        throw "Compile gagal untuk $source. Log: $savedLog`n$details"
    }

    $projectBinary = Join-Path $ProjectRoot $expert.Compiled
    New-Item -ItemType Directory -Path (Split-Path -Parent $projectBinary) -Force | Out-Null
    Copy-Item -LiteralPath $binary -Destination $projectBinary -Force
    $compiledOutputs[$expert.Target] = $binary
}

foreach ($expert in $expertDefinitions) {
	if ($expert.Target -notlike '*MT5FollowerCopyEA.mq5' -and $expert.Target -notlike '*MT5LoginProbe.mq5') { continue }
	$templateBinary = Join-Path $templateRoot ([IO.Path]::ChangeExtension($expert.Target, '.ex5'))
	New-Item -ItemType Directory -Path (Split-Path -Parent $templateBinary) -Force | Out-Null
	Copy-Item -LiteralPath $compiledOutputs[$expert.Target] -Destination $templateBinary -Force
	$templateSource = Join-Path $templateRoot $expert.Target
	New-Item -ItemType Directory -Path (Split-Path -Parent $templateSource) -Force | Out-Null
	Copy-Item -LiteralPath (Join-Path $masterRoot $expert.Target) -Destination $templateSource -Force
}

Write-Output "EA berhasil dikompilasi ke $ProjectRoot\mt5\experts."
Write-Output "Terminal master: $(Join-Path $masterRoot 'terminal64.exe')"
Write-Output "Template follower bersih: $templateRoot"
