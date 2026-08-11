[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string] $Address,

    [string] $OpenWrtAddress = '192.168.1.1'
)

$ErrorActionPreference = 'Stop'
$principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw '请在“以管理员身份运行”的 PowerShell 中执行此脚本。'
}
if ($Address -notmatch '^192\.168\.(?:1|3)\.(?:[1-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-4])$') {
    throw 'Address 必须是本机规范的 192.168.1.x 或 192.168.3.x 地址。'
}
if (-not (Get-NetIPAddress -AddressFamily IPv4 -IPAddress $Address -ErrorAction SilentlyContinue)) {
    throw "本机未配置地址 $Address。"
}
if ($OpenWrtAddress -ne '192.168.1.1') {
    throw 'OpenWrtAddress 必须保持为经过明确路由的 OpenWrt LAN 地址。'
}

$rustDesk = 'C:\Program Files\RustDesk\RustDesk.exe'
if (-not (Test-Path -LiteralPath $rustDesk)) {
    throw '未找到现有 RustDesk 安装。'
}
$serverKey = (& ssh.exe OpenWrt 'cat /opt/lan-remote-access/data/rustdesk/id_ed25519.pub').Trim()
if ($LASTEXITCODE -ne 0 -or $serverKey -notmatch '^[A-Za-z0-9+/]{43}=$') {
    throw '无法从 OpenWrt 读取有效的 RustDesk 服务器公钥。'
}

$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$backupDirectory = Join-Path $env:ProgramData "LANRemote\MigrationBackup\$stamp"
New-Item -ItemType Directory -Path $backupDirectory -Force | Out-Null
& netsh.exe advfirewall export (Join-Path $backupDirectory 'firewall.wfw') | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw 'Windows 防火墙备份失败。'
}
$terminalServer = 'HKLM:\SYSTEM\CurrentControlSet\Control\Terminal Server'
$rdpDeny = (Get-ItemProperty -LiteralPath $terminalServer -Name fDenyTSConnections).fDenyTSConnections
@{
    address = $Address
    openwrt_address = $OpenWrtAddress
    rdp_deny_before = $rdpDeny
} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $backupDirectory 'state.json') -Encoding utf8

$fileBrowserDirectory = Join-Path $env:ProgramData 'LANRemote\FileBrowser'
if (Test-Path -LiteralPath $fileBrowserDirectory) {
    Copy-Item -LiteralPath $fileBrowserDirectory -Destination (Join-Path $backupDirectory 'FileBrowser') -Recurse
}
foreach ($config in @(
        @{ Path = (Join-Path $env:ProgramData 'RustDesk'); Name = 'RustDesk-ProgramData' },
        @{ Path = (Join-Path $env:APPDATA 'RustDesk'); Name = 'RustDesk-User' }
    )) {
    $configDirectory = $config.Path
    if (Test-Path -LiteralPath $configDirectory) {
        Copy-Item -LiteralPath $configDirectory -Destination (Join-Path $backupDirectory $config.Name) -Recurse
    }
}

Set-ItemProperty -LiteralPath $terminalServer -Name fDenyTSConnections -Type DWord -Value 0
$rdpRule = 'WebVPN RDP from OpenWrt'
Get-NetFirewallRule -DisplayName $rdpRule -ErrorAction SilentlyContinue | Remove-NetFirewallRule
New-NetFirewallRule `
    -DisplayName $rdpRule `
    -Direction Inbound `
    -Action Allow `
    -Protocol TCP `
    -LocalAddress $Address `
    -LocalPort 3389 `
    -RemoteAddress $OpenWrtAddress `
    -Profile Any | Out-Null

Get-Process -Name filebrowser -ErrorAction SilentlyContinue | Stop-Process -Force
& (Join-Path $PSScriptRoot 'install-filebrowser.ps1') `
    -Address $Address `
    -OpenWrtAddress $OpenWrtAddress
if ($LASTEXITCODE -notin 0, $null) {
    throw "File Browser 迁移失败，退出码：$LASTEXITCODE"
}

& $rustDesk --config "rustdesk-host=$OpenWrtAddress,key=$serverKey.exe"
if ($LASTEXITCODE -notin 0, $null) {
    throw "RustDesk LAN 配置失败，退出码：$LASTEXITCODE"
}
Restart-Service -Name RustDesk -Force

$rdpListener = Get-NetTCPConnection -State Listen -LocalPort 3389 -ErrorAction SilentlyContinue
if (-not $rdpListener) {
    throw 'RDP 未在 3389 端口监听。'
}
$fileResponse = Invoke-WebRequest `
    -UseBasicParsing `
    -Uri "http://${Address}:18080/files/" `
    -Headers @{ 'X-WebVPN-User' = 'webvpn' } `
    -TimeoutSec 5
if ($fileResponse.StatusCode -ne 200) {
    throw "File Browser 本机验证失败：HTTP $($fileResponse.StatusCode)"
}

Write-Host ''
Write-Host 'Windows 侧 WebVPN 迁移已暂存。'
Write-Host "备份：$backupDirectory"
Write-Host '本脚本不会修改 Windows Tailscale；OpenWrt 侧下线必须在 RDP、文件和 RustDesk 验证后单独执行。'
