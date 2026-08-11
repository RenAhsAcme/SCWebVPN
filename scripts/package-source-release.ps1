param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$')]
    [string]$Version,
    [Parameter(Mandatory = $true)]
    [string]$CorrespondingSourceArchive,
    [string]$AutheliaBinary = "../.tmp-authelia/extracted/authelia",
    [string]$OutputRoot = "../.tmp-scwebvpn-release"
)

$ErrorActionPreference = "Stop"
$moduleRoot = Split-Path -Parent $PSScriptRoot
$repositoryRoot = Resolve-Path $moduleRoot
$authelia = Resolve-Path (Join-Path $PSScriptRoot $AutheliaBinary)
$controller = Resolve-Path (Join-Path $moduleRoot "dist/webvpn-controller-linux-amd64")
$web = Resolve-Path (Join-Path $moduleRoot "web/dist")
$correspondingSource = Resolve-Path (Join-Path $PSScriptRoot $CorrespondingSourceArchive)
$outputBase = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot $OutputRoot))
$staging = Join-Path $outputBase $Version
$archive = Join-Path $outputBase "$Version.tar.gz"

if ((Test-Path -LiteralPath $staging -PathType Any) -or (Test-Path -LiteralPath $archive -PathType Any)) {
    throw "Release output already exists: $Version"
}

New-Item -ItemType Directory -Path $staging | Out-Null
New-Item -ItemType Directory -Path (Join-Path $staging "web") | Out-Null
New-Item -ItemType Directory -Path (Join-Path $staging "web/source") | Out-Null
New-Item -ItemType Directory -Path (Join-Path $staging "config-examples") | Out-Null
New-Item -ItemType Directory -Path (Join-Path $staging "systemd") | Out-Null

Copy-Item -LiteralPath $controller -Destination (Join-Path $staging "webvpn-controller")
Copy-Item -LiteralPath $authelia -Destination (Join-Path $staging "authelia")
Copy-Item -Path (Join-Path $web "*") -Destination (Join-Path $staging "web") -Recurse
$sourceName = "webvpn-corresponding-source-$Version.tar.gz"
Copy-Item -LiteralPath $correspondingSource.Path -Destination (Join-Path $staging "web/source/$sourceName")
$sourceHash = (Get-FileHash -LiteralPath $correspondingSource.Path -Algorithm SHA256).Hash.ToLowerInvariant()
$sourceIndex = @"
<!doctype html>
<html lang="zh-CN">
  <head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="color-scheme" content="light dark"><title>WebVPN 源代码</title></head>
  <body><main><h1>WebVPN 源代码与许可证</h1><p>此归档包含当前运行版本的完整对应源码、构建脚本与第三方许可证。</p><p><a href="/source/$sourceName" download>下载对应源码</a></p><pre>SHA-256: $sourceHash</pre><p><a href="/">返回 WebVPN</a></p></main></body>
</html>
"@
[IO.File]::WriteAllText((Join-Path $staging "web/source/index.html"), $sourceIndex, [Text.UTF8Encoding]::new($false))
Copy-Item -Path (Join-Path $moduleRoot "config/*.example*") -Destination (Join-Path $staging "config-examples")
Copy-Item -Path (Join-Path $moduleRoot "packaging/systemd/*") -Destination (Join-Path $staging "systemd")

$manifest = foreach ($file in Get-ChildItem -LiteralPath $staging -Recurse -File | Sort-Object FullName) {
    $relative = [System.IO.Path]::GetRelativePath($staging, $file.FullName).Replace('\', '/')
    $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $relative"
}
$manifestText = ($manifest -join "`n") + "`n"
[System.IO.File]::WriteAllText((Join-Path $staging "manifest.sha256"), $manifestText, [System.Text.UTF8Encoding]::new($false))

& tar.exe -czf $archive -C $staging .
if ($LASTEXITCODE -ne 0) {
    throw "Release archive creation failed"
}

$archiveFile = Get-Item -LiteralPath $archive
$archiveHash = Get-FileHash -LiteralPath $archive -Algorithm SHA256
[pscustomobject]@{
    Version = $Version
    Archive = $archiveFile.FullName
    Bytes = $archiveFile.Length
    SHA256 = $archiveHash.Hash
    Repository = $repositoryRoot.Path
}
