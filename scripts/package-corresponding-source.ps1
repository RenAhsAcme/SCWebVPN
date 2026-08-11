param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$')]
    [string]$Version,
    [Parameter(Mandatory = $true)]
    [string]$ThirdPartyArchive,
    [string]$OutputRoot = '../.tmp-scwebvpn-source'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$moduleRoot = Split-Path -Parent $PSScriptRoot
$license = Join-Path $moduleRoot 'LICENSE'
$agplText = Join-Path $moduleRoot 'web/vendor/epoxy/LICENSE'
$thirdParty = Resolve-Path (Join-Path $PSScriptRoot $ThirdPartyArchive)
$outputBase = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot $OutputRoot))
$staging = Join-Path $outputBase "$Version-corresponding-source"
$archive = Join-Path $outputBase "$Version-corresponding-source.tar.gz"
$tar = (Get-Command tar.exe -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $license -PathType Leaf)) {
    throw 'LICENSE is required before public distribution.'
}
$licenseText = [IO.File]::ReadAllText($license)
if ($licenseText -notmatch 'GNU AFFERO GENERAL PUBLIC LICENSE' -or
    $licenseText -notmatch 'Version 3, 19 November 2007' -or
    -not (Test-Path -LiteralPath $agplText -PathType Leaf)) {
    throw 'SCWebVPN does not contain the required AGPL version 3 grant and text.'
}
$fullLicenseText = [IO.File]::ReadAllText($agplText)
if ($fullLicenseText -notmatch 'GNU AFFERO GENERAL PUBLIC LICENSE' -or $fullLicenseText -notmatch 'Version 3, 19 November 2007') {
    throw 'The canonical AGPL version 3 text is invalid.'
}
if ((Test-Path -LiteralPath $staging) -or (Test-Path -LiteralPath $archive)) {
    throw "Corresponding-source output already exists: $Version"
}

$thirdPartyEntries = @(& $tar -tzf $thirdParty.Path)
if ($LASTEXITCODE -ne 0 -or
    $thirdPartyEntries -notcontains './manifest.sha256' -or
    -not ($thirdPartyEntries -match '^\./upstream/scramjet-[0-9a-f]{40}\.tar\.gz$') -or
    -not ($thirdPartyEntries -match '^\./upstream/epoxy-tls-[0-9a-f]{40}\.tar\.gz$')) {
    throw 'The third-party corresponding-source input is incomplete.'
}

New-Item -ItemType Directory -Path $staging | Out-Null
$project = Join-Path $staging 'project'
$thirdPartyOutput = Join-Path $staging 'third_party'
New-Item -ItemType Directory -Path $project, $thirdPartyOutput | Out-Null

foreach ($directory in 'cmd', 'internal', 'config', 'packaging', 'scripts') {
    Copy-Item -LiteralPath (Join-Path $moduleRoot $directory) -Destination (Join-Path $project $directory) -Recurse
}
foreach ($directory in 'src', 'public', 'scripts', 'patches', 'vendor') {
    $destination = Join-Path $project "web/$directory"
    New-Item -ItemType Directory -Path (Split-Path -Parent $destination) -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $moduleRoot "web/$directory") -Destination $destination -Recurse
}
foreach ($file in 'go.mod', 'go.sum', 'LICENSE') {
    Copy-Item -LiteralPath (Join-Path $moduleRoot $file) -Destination (Join-Path $project $file)
}
Copy-Item -LiteralPath $agplText -Destination (Join-Path $project 'AGPL-3.0-only.txt')
foreach ($file in 'package.json', 'bun.lock', 'tsconfig.json') {
    Copy-Item -LiteralPath (Join-Path $moduleRoot "web/$file") -Destination (Join-Path $project "web/$file")
}
Copy-Item -LiteralPath $thirdParty.Path -Destination (Join-Path $thirdPartyOutput ([IO.Path]::GetFileName($thirdParty.Path)))

$building = @"
# Building the WebVPN release

Release: $Version

This source archive contains the project-owned Controller, Agent, browser
adapter, configuration templates, packaging scripts, and the complete pinned
third-party source bundle. It intentionally excludes generated binaries,
node_modules, databases, credentials, private keys, and operational state.

Requirements are pinned in project/go.mod, project/web/bun.lock, and the source
metadata inside third_party. Build the Go binaries with
project/scripts/build-controller.ps1 and build-agent.ps1. From project/web run
`bun install --frozen-lockfile`, `bun run check`, and `bun run build`. Rebuild
the patched Epoxy runtime using the script and toolchain described in the
third-party source bundle.
"@
[IO.File]::WriteAllText((Join-Path $staging 'BUILDING.md'), $building, [Text.UTF8Encoding]::new($false))

$forbidden = @(
    '-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----',
    'proxy_set_header X-WebVPN-Internal-Auth "[A-Za-z0-9_-]{32,}"',
    'AUTHELIA_[A-Z0-9_]+=(?!/etc/)[^\s]+'
)
foreach ($file in Get-ChildItem -LiteralPath $staging -Recurse -File | Where-Object { $_.Extension -notin '.gz', '.wasm' }) {
    $text = [IO.File]::ReadAllText($file.FullName)
    foreach ($pattern in $forbidden) {
        if ($text -match $pattern) {
            throw "Potential secret in corresponding source: $($file.FullName)"
        }
    }
}

$manifest = foreach ($file in Get-ChildItem -LiteralPath $staging -Recurse -File | Sort-Object FullName) {
    $relative = [IO.Path]::GetRelativePath($staging, $file.FullName).Replace('\', '/')
    $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $relative"
}
[IO.File]::WriteAllText((Join-Path $staging 'manifest.sha256'), (($manifest -join "`n") + "`n"), [Text.UTF8Encoding]::new($false))

& $tar -czf $archive -C $staging .
if ($LASTEXITCODE -ne 0) {
    throw 'Unable to create corresponding-source archive.'
}

$file = Get-Item -LiteralPath $archive
[pscustomobject]@{
    Version = $Version
    Archive = $file.FullName
    Bytes = $file.Length
    SHA256 = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    ThirdPartySHA256 = (Get-FileHash -LiteralPath $thirdParty.Path -Algorithm SHA256).Hash.ToLowerInvariant()
}
