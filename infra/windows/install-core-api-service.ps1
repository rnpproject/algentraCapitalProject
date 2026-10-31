param(
    [string] $ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path,

    [Parameter(Mandatory = $true)]
    [string] $EnvironmentFile,

    [Parameter(Mandatory = $true)]
    [string] $NssmPath,

    [string] $ServiceName = 'AlgentraCoreApi'
)

$ErrorActionPreference = 'Stop'
$currentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
$currentPrincipal = [Security.Principal.WindowsPrincipal]::new($currentIdentity)
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Jalankan installer dari PowerShell administrator agar ACL dan service Windows dapat dikonfigurasi.'
}
if (-not (Test-Path -LiteralPath $NssmPath -PathType Leaf)) { throw "nssm.exe tidak ditemukan: $NssmPath" }
if (-not (Test-Path -LiteralPath $EnvironmentFile -PathType Leaf)) { throw "Environment file tidak ditemukan: $EnvironmentFile" }
$ProjectRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
$EnvironmentFile = (Resolve-Path -LiteralPath $EnvironmentFile).Path
$backend = Join-Path $ProjectRoot 'algentra-backend.exe'
if (-not (Test-Path -LiteralPath $backend -PathType Leaf)) { throw "Binary Go Core API tidak ditemukan: $backend. Build sebelum memasang service." }
$serviceAccount = "NT SERVICE\$ServiceName"
$powershellExe = (Get-Command powershell.exe -ErrorAction Stop).Source
$startScript = Join-Path $PSScriptRoot 'start-core-api.ps1'
$logDirectory = Join-Path $ProjectRoot 'logs\core-api'
New-Item -ItemType Directory -Path $logDirectory -Force | Out-Null

function Quote-NssmArgument([string] $Value) {
    return '"' + $Value.Replace('"', '\"') + '"'
}

function Set-NssmSetting([string[]] $Arguments) {
    & $NssmPath set @Arguments | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "NSSM gagal mengatur '$($Arguments[1])'." }
}

function Set-WindowsVirtualServiceAccount([string] $AccountName) {
    # Configure virtual accounts through SCM; NSSM's account lookup can reject NT SERVICE SIDs.
    $startInfo = [Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = (Get-Command 'sc.exe' -ErrorAction Stop).Source
    # A virtual account must be passed without a password argument (NULL to ChangeServiceConfig).
    $startInfo.Arguments = ('config "{0}" obj= "{1}"' -f $ServiceName, $AccountName)
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $startInfo
    try {
        if (-not $process.Start()) { throw 'nssm.exe tidak dapat dimulai.' }
        $stdoutTask = $process.StandardOutput.ReadToEndAsync()
        $stderrTask = $process.StandardError.ReadToEndAsync()
        $process.WaitForExit()
        $output = @($stdoutTask.GetAwaiter().GetResult(), $stderrTask.GetAwaiter().GetResult()) -join "`n"
        if ($process.ExitCode -ne 0) { throw "Windows gagal mengatur akun service '$AccountName'. $output" }
    } finally {
        $process.Dispose()
    }
}

$service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($service) {
    $nativeService = Get-CimInstance -ClassName Win32_Service -Filter "Name='$ServiceName'" -ErrorAction Stop
    $registeredExecutable = $null
    if ($nativeService -and $nativeService.PathName -match '^\s*"([^"]+\.exe)"') {
        $registeredExecutable = $Matches[1]
    } elseif ($nativeService -and $nativeService.PathName -match '^\s*(.+?\.exe)(?:\s|$)') {
        $registeredExecutable = $Matches[1]
    }
    $nssmParametersPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName\Parameters"
    $registeredApplication = (Get-ItemProperty -LiteralPath $nssmParametersPath -Name Application -ErrorAction SilentlyContinue).Application
    $isExpectedNssm = $false
    if ($registeredExecutable -and $registeredApplication) {
        $registeredExecutable = [IO.Path]::GetFullPath($registeredExecutable)
        $registeredApplication = [IO.Path]::GetFullPath($registeredApplication)
        $expectedNssmPath = [IO.Path]::GetFullPath((Resolve-Path -LiteralPath $NssmPath).Path)
        $isExpectedNssm = [string]::Equals($registeredExecutable, $expectedNssmPath, [StringComparison]::OrdinalIgnoreCase) -and
            [string]::Equals($registeredApplication, $powershellExe, [StringComparison]::OrdinalIgnoreCase)
    }
    if (-not $isExpectedNssm) {
        throw "Service $ServiceName sudah ada tetapi bukan service NSSM Algentra; installer tidak akan mengubahnya."
    }
    if ($service.Status -ne 'Stopped') {
        Stop-Service -Name $ServiceName -Force
        $service.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
    }
} else {
    & $NssmPath install $ServiceName $powershellExe
    if ($LASTEXITCODE -ne 0) { throw 'NSSM gagal mendaftarkan service.' }
}
# Enable the service SID before resolving its NT SERVICE\<name> virtual account.
& sc.exe sidtype $ServiceName unrestricted | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal mengaktifkan service SID.' }
Set-NssmSetting @($ServiceName, 'Start', 'SERVICE_DISABLED')
Set-NssmSetting @($ServiceName, 'AppDirectory', $ProjectRoot)
$parameters = @(
    '-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File',
    (Quote-NssmArgument $startScript), '-ProjectRoot', (Quote-NssmArgument $ProjectRoot),
    '-EnvironmentFile', (Quote-NssmArgument $EnvironmentFile)
) -join ' '
Set-NssmSetting @($ServiceName, 'AppParameters', $parameters)
Set-NssmSetting @($ServiceName, 'DisplayName', 'Algentra Capital Core API')
Set-NssmSetting @($ServiceName, 'Description', 'Go Core API; schema must be provisioned before release.')
Set-NssmSetting @($ServiceName, 'AppExit', 'Default', 'Restart')
Set-NssmSetting @($ServiceName, 'AppRestartDelay', '5000')
Set-NssmSetting @($ServiceName, 'AppStdout', (Join-Path $logDirectory 'stdout.log'))
Set-NssmSetting @($ServiceName, 'AppStderr', (Join-Path $logDirectory 'stderr.log'))
Set-NssmSetting @($ServiceName, 'AppRotateFiles', '1')
Set-NssmSetting @($ServiceName, 'AppRotateBytes', '10485760')
Set-WindowsVirtualServiceAccount $serviceAccount
$readOnlyPaths = @(
	$backend,
    $PSScriptRoot,
    (Split-Path -Parent $NssmPath)
)
foreach ($path in $readOnlyPaths) {
    if (Test-Path -LiteralPath $path) {
        if ((Get-Item -LiteralPath $path).PSIsContainer) {
            & icacls.exe $path /grant "${serviceAccount}:(OI)(CI)RX" /T /C | Out-Null
        } else {
            & icacls.exe $path /grant "${serviceAccount}:R" | Out-Null
        }
        if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak baca pada $path." }
    }
}
$directory = Get-Item -LiteralPath (Split-Path -Parent $ProjectRoot)
while ($directory) {
    & icacls.exe $directory.FullName /grant "${serviceAccount}:(X)" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Gagal memberi hak traversal pada $($directory.FullName)." }
    $parent = Split-Path -Parent $directory.FullName
    if (-not $parent -or $parent -eq $directory.FullName) { break }
    $directory = Get-Item -LiteralPath $parent
}
& icacls.exe $ProjectRoot /grant "${serviceAccount}:(RX)" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal memberi hak traversal pada folder proyek.' }
$environmentDirectory = Split-Path -Parent $EnvironmentFile
& icacls.exe $environmentDirectory /grant "${serviceAccount}:(X)" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal memberi hak traversal pada direktori environment file.' }
& icacls.exe $EnvironmentFile /inheritance:r /grant 'SYSTEM:F' 'BUILTIN\Administrators:F' "${serviceAccount}:R" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal membatasi ACL environment file.' }
& icacls.exe $logDirectory /grant "${serviceAccount}:(OI)(CI)M" /T /C | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Gagal memberi hak tulis pada folder log.' }

Set-NssmSetting @($ServiceName, 'Start', 'SERVICE_AUTO_START')

Write-Output "Service $ServiceName terpasang dengan akun virtual $serviceAccount. Pastikan ACL environment file hanya memberi akses kepada administrator dan akun service."
Write-Output 'Jalankan service setelah DNS, kredensial, schema production, dan health check diverifikasi.'
