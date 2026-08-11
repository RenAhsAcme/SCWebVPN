[CmdletBinding()]
param(
    [string] $Root = "$env:PUBLIC\RemoteFiles",
    [Parameter(Mandatory)]
    [string] $Address,
    [string] $OpenWrtAddress = '192.168.1.1',
    [ValidateRange(1024, 65535)]
    [int] $Port = 18080,
    [string] $ArchivePath
)

$ErrorActionPreference = 'Stop'

$version = '2.63.23'
$expectedHash = 'FDB1D86DFAFFF8B3861867C7797CE786570013088678E03DE17CFD9476C72384'
$archiveName = 'windows-amd64-filebrowser.zip'
$download = "https://github.com/filebrowser/filebrowser/releases/download/v$version/$archiveName"
$installDirectory = "$env:ProgramData\LANRemote\FileBrowser"
$dataDirectory = Join-Path $installDirectory 'data'
$database = Join-Path $dataDirectory 'filebrowser.db'
$log = Join-Path $dataDirectory 'filebrowser.log'
$executable = Join-Path $installDirectory 'filebrowser.exe'
$archive = Join-Path $env:TEMP "filebrowser-$version-windows-amd64.zip"
$extractDirectory = Join-Path $env:TEMP "filebrowser-$version-windows-amd64"
$taskPath = '\LANRemote\'
$taskName = 'FileBrowser'
$firewallRule = 'SCWebVPN Remote Files from OpenWrt'
$authHeader = 'X-WebVPN-User'
$proxyUser = 'webvpn'
$principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())

Start-Transcript -LiteralPath "$env:PUBLIC\LANRemoteFileBrowserInstall.log" -Force | Out-Null
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Run this script from an elevated PowerShell session.'
}
if ($Address -notmatch '^192\.168\.(?:1|3)\.(?:[1-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-4])$') {
    throw "Address must be this PC's canonical 192.168.1.x or 192.168.3.x IPv4 address."
}
if (-not (Get-NetIPAddress -AddressFamily IPv4 -IPAddress $Address -ErrorAction SilentlyContinue)) {
    throw "LAN address $Address is not configured on this PC."
}
if ($OpenWrtAddress -ne '192.168.1.1') {
    throw 'OpenWrtAddress must remain the explicitly routed OpenWrt LAN address.'
}

Write-Host 'Downloading and verifying the official File Browser release...'
if ($ArchivePath) {
    Copy-Item -LiteralPath $ArchivePath -Destination $archive -Force
} else {
    & curl.exe `
        --fail `
        --location `
        --retry 3 `
        --retry-all-errors `
        --connect-timeout 20 `
        --max-time 300 `
        --output $archive `
        $download
    if ($LASTEXITCODE -ne 0) {
        throw "File Browser download failed with curl exit code $LASTEXITCODE."
    }
}
$actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash
if ($actualHash -ne $expectedHash) {
    throw "File Browser archive hash mismatch: $actualHash"
}

if (Test-Path -LiteralPath $extractDirectory) {
    Remove-Item -LiteralPath $extractDirectory -Recurse -Force
}
Expand-Archive -LiteralPath $archive -DestinationPath $extractDirectory
$downloadedExecutable = Join-Path $extractDirectory 'filebrowser.exe'
if (-not (Test-Path -LiteralPath $downloadedExecutable)) {
    throw 'filebrowser.exe is missing from the release archive.'
}

$existingTask = Get-ScheduledTask -TaskPath $taskPath -TaskName $taskName -ErrorAction SilentlyContinue
if ($existingTask) {
    Stop-ScheduledTask -TaskPath $taskPath -TaskName $taskName -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1
}

New-Item -ItemType Directory -Path $installDirectory, $dataDirectory, $Root -Force | Out-Null
Copy-Item -LiteralPath $downloadedExecutable -Destination $executable -Force

if (-not (Test-Path -LiteralPath $database)) {
    & $executable config init `
        --database $database `
        --address $Address `
        --port $Port `
        --root $Root `
        --baseURL /files `
        --auth.method proxy `
        --auth.header $authHeader `
        --branding.name 'SCWebVPN Remote Files' `
        --branding.disableExternal `
        --disableExec `
        --log $log
    if ($LASTEXITCODE -ne 0) {
        throw "File Browser database initialization failed with exit code $LASTEXITCODE."
    }

} else {
    & $executable config set `
        --database $database `
        --address $Address `
        --port $Port `
        --root $Root `
        --baseURL /files `
        --auth.method proxy `
        --auth.header $authHeader `
        --branding.name 'SCWebVPN Remote Files' `
        --branding.disableExternal `
        --disableExec `
        --log $log
    if ($LASTEXITCODE -ne 0) {
        throw "File Browser configuration update failed with exit code $LASTEXITCODE."
    }
}

$savedErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = 'SilentlyContinue'
& $executable users find $proxyUser --database $database *> $null
$userFindExitCode = $LASTEXITCODE
$ErrorActionPreference = $savedErrorActionPreference
if ($userFindExitCode -ne 0) {
    $passwordBytes = [byte[]]::new(32)
    $randomNumberGenerator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $randomNumberGenerator.GetBytes($passwordBytes)
    } finally {
        $randomNumberGenerator.Dispose()
    }
    $unusedPassword = [Convert]::ToBase64String($passwordBytes)
    & $executable users add $proxyUser $unusedPassword `
        --database $database `
        --scope . `
        --lockPassword `
        --perm.admin=false `
        --perm.execute=false `
        --perm.share=false
    if ($LASTEXITCODE -ne 0) {
        throw "File Browser proxy user creation failed with exit code $LASTEXITCODE."
    }
} else {
    & $executable users update $proxyUser `
        --database $database `
        --scope . `
        --lockPassword `
        --perm.admin=false `
        --perm.execute=false `
        --perm.share=false
    if ($LASTEXITCODE -ne 0) {
        throw "File Browser proxy user update failed with exit code $LASTEXITCODE."
    }
}

# The service can modify only its runtime data and exchange directory.
& icacls.exe $dataDirectory /grant '*S-1-5-19:(OI)(CI)M' /T /C | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw 'Could not grant LOCAL SERVICE access to the runtime data directory.'
}
& icacls.exe $Root /grant '*S-1-5-19:(OI)(CI)M' /T /C | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw 'Could not grant LOCAL SERVICE access to the file exchange directory.'
}

Get-NetFirewallRule -DisplayName $firewallRule -ErrorAction SilentlyContinue | Remove-NetFirewallRule
New-NetFirewallRule `
    -DisplayName $firewallRule `
    -Direction Inbound `
    -Action Allow `
    -Protocol TCP `
    -LocalAddress $Address `
    -LocalPort $Port `
    -RemoteAddress $OpenWrtAddress `
    -Profile Any | Out-Null

$action = New-ScheduledTaskAction `
    -Execute $executable `
    -Argument "--database `"$database`"" `
    -WorkingDirectory $installDirectory
$trigger = New-ScheduledTaskTrigger -AtStartup
$servicePrincipal = New-ScheduledTaskPrincipal `
    -UserId 'NT AUTHORITY\LOCAL SERVICE' `
    -LogonType ServiceAccount `
    -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -StartWhenAvailable

Register-ScheduledTask `
    -TaskPath $taskPath `
    -TaskName $taskName `
    -Action $action `
    -Trigger $trigger `
    -Principal $servicePrincipal `
    -Settings $settings `
    -Description 'Low-privilege high-speed file channel for SCWebVPN Remote.' `
    -Force | Out-Null
Start-ScheduledTask -TaskPath $taskPath -TaskName $taskName

for ($attempt = 0; $attempt -lt 20; $attempt++) {
    try {
        $response = Invoke-WebRequest `
            -UseBasicParsing `
            -Uri "http://${Address}:$Port/files/" `
            -Headers @{ $authHeader = $proxyUser } `
            -TimeoutSec 2
        if ($response.StatusCode -eq 200) {
            break
        }
    } catch {
        if ($attempt -eq 19) {
            throw
        }
    }
    Start-Sleep -Milliseconds 500
}

@(
    "version=$version"
    "archive_sha256=$expectedHash"
    "root=$Root"
    "listen=$Address`:$Port"
    'base_url=/files'
) | Set-Content -LiteralPath (Join-Path $installDirectory 'INSTALL.txt') -Encoding ascii

Remove-Item -LiteralPath $archive -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $extractDirectory -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ''
Write-Host "File Browser $version is running."
Write-Host "Exchange directory: $Root"
Write-Host "Restricted listener: http://${Address}:$Port/files/"
