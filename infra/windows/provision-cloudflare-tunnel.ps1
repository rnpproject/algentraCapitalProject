param(
    [Parameter(Mandatory = $true)] [string] $Hostname,
    [Parameter(Mandatory = $true)] [string] $ApiToken,
    [Parameter(Mandatory = $true)] [string] $StatePath,
    [Parameter(Mandatory = $true)] [string] $TunnelTokenPath
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$Hostname = $Hostname.Trim().TrimEnd('.').ToLowerInvariant()
if ($Hostname -notmatch '^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$') {
    throw 'Hostname publik tidak valid.'
}
if ([string]::IsNullOrWhiteSpace($ApiToken)) { throw 'Cloudflare API token kosong.' }

$apiRoot = 'https://api.cloudflare.com/client/v4'
$headers = @{ Authorization = "Bearer $ApiToken" }
$ApiToken = $null

function Invoke-CloudflareApi([string] $Method, [string] $Path, $Body = $null) {
    $parameters = @{
        Method = $Method
        Uri = "$apiRoot$Path"
        Headers = $headers
        ErrorAction = 'Stop'
    }
    if ($null -ne $Body) {
        $parameters.ContentType = 'application/json'
        $parameters.Body = $Body | ConvertTo-Json -Depth 20 -Compress
    }
    try { $response = Invoke-RestMethod @parameters }
    catch {
        $detail = $_.ErrorDetails.Message
        if ($detail) {
            try {
                $decoded = $detail | ConvertFrom-Json -ErrorAction Stop
                $messages = @($decoded.errors | ForEach-Object { $_.message }) -join '; '
                if ($messages) { throw "Cloudflare API $Method $Path gagal: $messages" }
            } catch {
                if ($_.Exception.Message -like 'Cloudflare API *') { throw }
            }
        }
        throw "Cloudflare API $Method $Path gagal. Pastikan token memiliki Account: Cloudflare Tunnel Edit dan Zone: DNS Edit serta Zone Read."
    }
    if (-not $response.success) {
        $messages = @($response.errors | ForEach-Object { $_.message }) -join '; '
        if (-not $messages) { $messages = 'respons API menandai operasi gagal' }
        throw "Cloudflare API $Method $Path gagal: $messages"
    }
    return $response.result
}

$state = $null
$createdByInstaller = $false
if (Test-Path -LiteralPath $StatePath -PathType Leaf) {
    $state = Get-Content -LiteralPath $StatePath -Raw | ConvertFrom-Json -ErrorAction Stop
    if ($state.hostname -ne $Hostname -or -not $state.account_id -or -not $state.zone_id -or -not $state.tunnel_id) {
        throw "State Cloudflare $StatePath tidak cocok dengan hostname $Hostname."
    }
}

if (-not $state) {
    $zones = @()
    for ($page = 1; $page -le 20; $page++) {
        $pageResult = @(Invoke-CloudflareApi 'GET' "/zones?per_page=50&page=$page&status=active")
        $zones += $pageResult
        if ($pageResult.Count -lt 50) { break }
    }
    $zone = $zones |
        Where-Object { $Hostname -eq $_.name -or $Hostname.EndsWith(".$($_.name)", [StringComparison]::OrdinalIgnoreCase) } |
        Sort-Object { $_.name.Length } -Descending |
        Select-Object -First 1
    if (-not $zone) { throw "Zone Cloudflare aktif untuk $Hostname tidak ditemukan pada token ini." }
    $accountId = [string]$zone.account.id
    if (-not $accountId) { throw 'Cloudflare tidak mengembalikan account ID untuk zone.' }
    $zoneId = [string]$zone.id
    $tunnelName = "algentra-portal-$($Hostname.Replace('.', '-'))"
    $tunnels = @()
    for ($page = 1; $page -le 20; $page++) {
        $pageTunnels = @(Invoke-CloudflareApi 'GET' "/accounts/$accountId/cfd_tunnel?per_page=100&page=$page")
        $tunnels += $pageTunnels
        if ($pageTunnels.Count -lt 100) { break }
    }
    $matchingTunnels = @($tunnels | Where-Object { $_.name -eq $tunnelName -and -not $_.deleted_at })
    if ($matchingTunnels.Count -gt 1) { throw "Lebih dari satu Tunnel bernama $tunnelName ditemukan; hapus duplikat atau pilih Tunnel secara manual." }
    $tunnel = $matchingTunnels | Select-Object -First 1
    if ($tunnel -and $tunnel.config_src -ne 'cloudflare') {
        throw "Tunnel $tunnelName sudah ada tetapi dikelola lokal. Hapus/rename Tunnel itu atau siapkan Tunnel remote-managed."
    }
    if (-not $tunnel) {
        $secretBytes = New-Object byte[] 32
        $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
        try { $rng.GetBytes($secretBytes) } finally { $rng.Dispose() }
        $tunnelSecret = [Convert]::ToBase64String($secretBytes)
        [Array]::Clear($secretBytes, 0, $secretBytes.Length)
        $tunnel = Invoke-CloudflareApi 'POST' "/accounts/$accountId/cfd_tunnel" @{
            name = $tunnelName
            config_src = 'cloudflare'
            tunnel_secret = $tunnelSecret
        }
        $tunnelSecret = $null
        $createdByInstaller = $true
    }
    if (-not $tunnel.id) { throw 'Cloudflare tidak mengembalikan tunnel ID.' }
    $state = [pscustomobject]@{
        hostname = $Hostname
        zone_id = $zoneId
        account_id = $accountId
        tunnel_id = [string]$tunnel.id
        tunnel_name = $tunnelName
        created_by_installer = $createdByInstaller
    }
    [IO.File]::WriteAllText($StatePath, ($state | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
}

if (-not (Test-Path -LiteralPath $TunnelTokenPath -PathType Leaf)) {
    $tunnelCredential = Invoke-CloudflareApi 'GET' "/accounts/$($state.account_id)/cfd_tunnel/$($state.tunnel_id)/token"
    if ($tunnelCredential -isnot [string] -or $tunnelCredential.Length -lt 40) {
        throw 'Cloudflare tidak mengembalikan token Tunnel yang valid.'
    }
    [IO.File]::WriteAllText($TunnelTokenPath, $tunnelCredential, [Text.UTF8Encoding]::new($false))
    $tunnelCredential = $null
}

$tunnelId = [uri]::EscapeDataString([string]$state.tunnel_id)
Invoke-CloudflareApi 'PUT' "/accounts/$($state.account_id)/cfd_tunnel/$tunnelId/configurations" @{
    config = @{
        ingress = @(
            @{ hostname = $Hostname; service = 'http://127.0.0.1:3000' },
            @{ service = 'http_status:404' }
        )
    }
} | Out-Null

$dnsRecords = @(Invoke-CloudflareApi 'GET' "/zones/$($state.zone_id)/dns_records?type=CNAME&name=$([uri]::EscapeDataString($Hostname))&per_page=100")
$dnsTarget = "$($state.tunnel_id).cfargotunnel.com"
$existingRecord = $dnsRecords | Where-Object { $_.name -eq $Hostname } | Select-Object -First 1
if ($existingRecord) {
    if ($existingRecord.content.TrimEnd('.').ToLowerInvariant() -ne $dnsTarget) {
        throw "DNS $Hostname sudah ada dengan tujuan berbeda. Ubah/hapus record itu di Cloudflare lalu jalankan installer lagi."
    }
    if (-not $existingRecord.proxied) {
        Invoke-CloudflareApi 'PATCH' "/zones/$($state.zone_id)/dns_records/$($existingRecord.id)" @{
            proxied = $true
            ttl = 1
        } | Out-Null
    }
} else {
    Invoke-CloudflareApi 'POST' "/zones/$($state.zone_id)/dns_records" @{
        type = 'CNAME'
        name = $Hostname
        content = $dnsTarget
        ttl = 1
        proxied = $true
        comment = 'Algentra portal Cloudflare Tunnel'
    } | Out-Null
}

$state | Add-Member -NotePropertyName provisioned -NotePropertyValue $true -Force
[IO.File]::WriteAllText($StatePath, ($state | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
$headers.Clear()
Write-Output "Cloudflare Tunnel dan DNS siap untuk $Hostname."
