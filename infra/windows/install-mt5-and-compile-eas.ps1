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
    $metaEditor = Join-Path $Destination 'metaeditor64.exe'
    if ((Test-Path -LiteralPath $terminal -PathType Leaf) -and (Test-Path -LiteralPath $metaEditor -PathType Leaf)) { return }

    if (-not (Test-Path -LiteralPath $installerPath -PathType Leaf)) {
        Invoke-WebRequest -Uri 'https://download.terminal.free/cdn/web/metaquotes.ltd/mt5/mt5setup.exe' -OutFile $installerPath
    }
    $signature = Get-AuthenticodeSignature -FilePath $installerPath
    if ($signature.Status -ne [System.Management.Automation.SignatureStatus]::Valid) {
        throw "Tanda tangan installer MetaTrader tidak valid: $($signature.Status)."
    }

    New-Item -ItemType Directory -Path $Destination -Force | Out-Null
    $attemptLimit = 2
    $lastExitCode = 'tidak tersedia'
    for ($attempt = 1; $attempt -le $attemptLimit; $attempt++) {
        if ($attempt -gt 1) {
            $refreshPath = Join-Path $DownloadsRoot 'mt5setup-refresh.exe'
            try {
                Write-Output 'Mengunduh ulang installer MetaTrader untuk percobaan kedua...'
                Invoke-WebRequest -Uri 'https://download.terminal.free/cdn/web/metaquotes.ltd/mt5/mt5setup.exe' -OutFile $refreshPath
                $refreshSignature = Get-AuthenticodeSignature -FilePath $refreshPath
                if ($refreshSignature.Status -ne [System.Management.Automation.SignatureStatus]::Valid) {
                    throw "Tanda tangan installer unduhan ulang tidak valid: $($refreshSignature.Status)."
                }
                Move-Item -LiteralPath $refreshPath -Destination $installerPath -Force
            } catch {
                Write-Warning "Unduhan ulang gagal; mencoba kembali dengan installer tersimpan. Detail: $($_.Exception.Message)"
                if (Test-Path -LiteralPath $refreshPath) { Remove-Item -LiteralPath $refreshPath -Force }
            }
        }

        $logBase = Join-Path $LogRoot ("{0}-installer-attempt-{1}" -f (Split-Path -Leaf $Destination), $attempt)
        $stdoutLog = "$logBase.stdout.log"
        $stderrLog = "$logBase.stderr.log"
        foreach ($log in @($stdoutLog, $stderrLog)) {
            if (Test-Path -LiteralPath $log) { Remove-Item -LiteralPath $log -Force }
        }
        Write-Output "Memasang MetaTrader 5 ke $Destination (percobaan $attempt/$attemptLimit)..."
        $arguments = '/auto /path:"{0}"' -f $Destination
        $process = Start-Process -FilePath $installerPath -ArgumentList $arguments -Wait -PassThru -WindowStyle Hidden -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog
        $lastExitCode = $process.ExitCode
        $terminalReady = (Test-Path -LiteralPath $terminal -PathType Leaf) -and (Get-Item -LiteralPath $terminal).Length -gt 0
        $editorReady = Test-Path -LiteralPath $metaEditor -PathType Leaf
        if ($lastExitCode -eq 0 -and $terminalReady -and $editorReady) { return }

        if ($terminalReady -and $editorReady) {
            Write-Warning "Installer mengembalikan exit $lastExitCode, tetapi terminal64.exe dan metaeditor64.exe tersedia di $Destination; melanjutkan."
            return
        }

        if ($attempt -lt $attemptLimit) {
            Write-Warning "Installer MetaTrader gagal (exit $lastExitCode). Log: $stdoutLog dan $stderrLog. Mencoba ulang otomatis."
        }
    }

    $stdoutDetails = if (Test-Path -LiteralPath $stdoutLog -PathType Leaf) { (Get-Content -LiteralPath $stdoutLog -Raw).Trim() } else { '' }
    $stderrDetails = if (Test-Path -LiteralPath $stderrLog -PathType Leaf) { (Get-Content -LiteralPath $stderrLog -Raw).Trim() } else { '' }
    $details = (@($stdoutDetails, $stderrDetails) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }) -join "`n"
    if (-not $details) { $details = 'Installer tidak menulis detail ke stdout/stderr; periksa koneksi internet keluar dan log installer Windows.' }
    throw "Installer MetaTrader gagal (exit $lastExitCode) setelah $attemptLimit percobaan pada $Destination.`nLog: $stdoutLog`n$stderrLog`n$details"
}

function Initialize-MetaTraderPortableData {
    $mql5Root = Join-Path $masterRoot 'MQL5'
    $tradeInclude = Join-Path $mql5Root 'Include\Trade\Trade.mqh'
    if (Test-Path -LiteralPath $tradeInclude -PathType Leaf) { return $mql5Root }

    $terminalPath = Join-Path $masterRoot 'terminal64.exe'
    Write-Output 'Menyiapkan data portable MT5 master dan library MQL5 standar...'
    $terminalProcess = Start-Process -FilePath $terminalPath -ArgumentList '/portable' `
        -WorkingDirectory $masterRoot -PassThru -WindowStyle Hidden
    try {
        for ($attempt = 0; $attempt -lt 90; $attempt++) {
            if (Test-Path -LiteralPath $tradeInclude -PathType Leaf) { break }
            $terminalProcess.Refresh()
            if ($terminalProcess.HasExited) { break }
            Start-Sleep -Seconds 2
        }
    } finally {
        $terminalProcess.Refresh()
        if (-not $terminalProcess.HasExited) {
            try { [void]$terminalProcess.CloseMainWindow(); [void]$terminalProcess.WaitForExit(5000) } catch { }
            $terminalProcess.Refresh()
            if (-not $terminalProcess.HasExited) { Stop-Process -Id $terminalProcess.Id -Force -ErrorAction SilentlyContinue }
        }
        $terminalProcess.Dispose()
    }

    if (-not (Test-Path -LiteralPath $tradeInclude -PathType Leaf)) {
        throw "Library standar MQL5 tidak tersedia setelah inisialisasi terminal portable: $tradeInclude. Pastikan koneksi internet keluar ke server MetaQuotes tersedia, lalu jalankan installer lagi."
    }
    return $mql5Root
}

Install-MetaTrader $masterRoot
Install-MetaTrader $templateRoot
$mql5Root = Initialize-MetaTraderPortableData
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
    $compileLogs = @([IO.Path]::ChangeExtension($installedSource, '.log'), "$installedSource.log")
    Remove-Item -LiteralPath $binary -Force -ErrorAction SilentlyContinue
    foreach ($compileLog in $compileLogs) { Remove-Item -LiteralPath $compileLog -Force -ErrorAction SilentlyContinue }

    Write-Output "Compile EA: $([IO.Path]::GetFileName($source))"
    $arguments = '/compile:"{0}" /log /include:"{1}"' -f $installedSource, $mql5Root
    $process = Start-Process -FilePath $metaEditor -ArgumentList $arguments -Wait -PassThru -WindowStyle Hidden
    $savedLog = Join-Path $LogRoot ([IO.Path]::GetFileNameWithoutExtension($source) + '.log')
    if (Test-Path -LiteralPath $savedLog) { Remove-Item -LiteralPath $savedLog -Force }
    $generatedLog = $compileLogs | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf } | Select-Object -First 1
    if ($generatedLog) {
        Move-Item -LiteralPath $generatedLog -Destination $savedLog -Force
    }
    $compileDetails = if (Test-Path -LiteralPath $savedLog -PathType Leaf) { Get-Content -LiteralPath $savedLog -Raw } else { '' }
    $compileHasErrors = $compileDetails -match '(?im)\berror\s+\d+\s*:|^\s*Result:\s*[1-9]\d*\s+errors?'
    if (-not (Test-Path -LiteralPath $binary -PathType Leaf) -or (Get-Item -LiteralPath $binary).Length -eq 0 -or $compileHasErrors -or -not $compileDetails) {
        $details = if (Test-Path -LiteralPath $savedLog -PathType Leaf) { Get-Content -LiteralPath $savedLog -Raw } else { 'Log MetaEditor tidak tersedia.' }
        throw "Compile gagal untuk $source (MetaEditor exit $($process.ExitCode)). Log: $savedLog`n$details"
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
