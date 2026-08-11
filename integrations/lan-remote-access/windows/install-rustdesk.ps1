[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string] $ServerKey,

    [string] $Server = '192.168.1.1',

    [string] $ResultPath = "$env:ProgramData\LANRemote\rustdesk-id.txt"
)

$ErrorActionPreference = 'Stop'
trap {
    $errorDirectory = Split-Path -Parent $ResultPath
    New-Item -ItemType Directory -Path $errorDirectory -Force | Out-Null
    $_ | Out-String | Set-Content -LiteralPath (Join-Path $errorDirectory 'install-error.txt') -Encoding utf8
    Write-Host ''
    Write-Host "安装失败：$($_.Exception.Message)" -ForegroundColor Red
    Write-Host "详细信息已写入 $errorDirectory\install-error.txt。"
    Read-Host '按 Enter 关闭窗口'
    exit 1
}

$version = '1.4.7'
$expectedHash = 'D3AF4216C653E6AC0A98810DC59080EA26FB03045B79DBB6F859F3C954402C9F'
$download = "https://github.com/rustdesk/rustdesk/releases/download/$version/rustdesk-$version-x86_64.exe"
$installer = Join-Path $env:TEMP "rustdesk-$version-x86_64.exe"
$principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())

if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw '请在“以管理员身份运行”的 PowerShell 中执行此脚本。'
}
if ($ServerKey -notmatch '^[A-Za-z0-9+/]{43}=$') {
    throw 'RustDesk 服务器公钥格式无效。'
}

$rustdesk = 'C:\Program Files\RustDesk\RustDesk.exe'
if ((Test-Path -LiteralPath $rustdesk) -and (Get-Service -Name RustDesk -ErrorAction SilentlyContinue)) {
    Write-Host '检测到现有 RustDesk 安装，跳过下载与安装。'
} else {
    Write-Host '正在下载并校验 RustDesk 官方安装程序…'
    Invoke-WebRequest -UseBasicParsing -Uri $download -OutFile $installer
    $actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $installer).Hash
    if ($actualHash -ne $expectedHash) {
        throw "RustDesk 安装程序哈希不匹配：$actualHash"
    }

    Start-Process -FilePath $installer -ArgumentList '--silent-install'
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        if ((Test-Path -LiteralPath $rustdesk) -and (Get-Service -Name RustDesk -ErrorAction SilentlyContinue)) {
            break
        }
        Start-Sleep -Seconds 1
    }
    if (-not (Test-Path -LiteralPath $rustdesk) -or -not (Get-Service -Name RustDesk -ErrorAction SilentlyContinue)) {
        throw 'RustDesk 安装完成后未找到预期的程序文件。'
    }
}

Write-Host '正在写入 OpenWrt 自托管服务器配置…'
& $rustdesk --config "rustdesk-host=$Server,key=$ServerKey.exe"
if ($LASTEXITCODE -notin 0, $null) {
    throw "RustDesk 服务器配置失败，退出码：$LASTEXITCODE"
}

$password = Read-Host '请设置视频接管固定密码（不会回显）' -AsSecureString
$passwordPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($password)
try {
    $passwordText = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPointer)
    if ($passwordText.Length -lt 10) {
        throw '固定密码至少需要 10 个字符。'
    }
    & $rustdesk --password $passwordText
    if ($LASTEXITCODE -notin 0, $null) {
        throw "RustDesk 固定密码设置失败，退出码：$LASTEXITCODE"
    }
}
finally {
    $passwordText = $null
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPointer)
}

$service = Get-Service -Name RustDesk -ErrorAction Stop
if ($service.Status -eq 'Running') {
    Restart-Service -Name RustDesk -Force
} else {
    Start-Service -Name RustDesk
}
Set-Service -Name RustDesk -StartupType Automatic
Start-Sleep -Seconds 3

$id = (& $rustdesk --get-id | Select-Object -Last 1).Trim()
if ($id -notmatch '^\d+$') {
    throw "未能读取有效的 RustDesk ID，原始输出：$id"
}

$resultDirectory = Split-Path -Parent $ResultPath
New-Item -ItemType Directory -Path $resultDirectory -Force | Out-Null
Set-Content -LiteralPath $ResultPath -Value $id -Encoding ascii -NoNewline

Write-Host ''
Write-Host "RustDesk ID: $id"
Write-Host "服务已启用；结果已写入 $ResultPath。"
Read-Host '按 Enter 关闭窗口'
