param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$')]
    [string]$Version,
    [string]$ScramjetSource = '../.tmp-scwebvpn-source/scramjet-c26bfc6d',
    [string]$EpoxySource = '../.tmp-scwebvpn-source/epoxy-93d5a726',
    [string]$OutputRoot = '../.tmp-scwebvpn-source'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$scramjetCommit = 'c26bfc6d7f7c7f4dac52ce182a2ceab90e851823'
$epoxyCommit = '93d5a726894b2f16bad54c4a3801446cbbd22d26'
$moduleRoot = Split-Path -Parent $PSScriptRoot
$scramjet = Resolve-Path (Join-Path $PSScriptRoot $ScramjetSource)
$epoxy = Resolve-Path (Join-Path $PSScriptRoot $EpoxySource)
$outputBase = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot $OutputRoot))
$staging = Join-Path $outputBase "$Version-third-party-source"
$archive = Join-Path $outputBase "$Version-third-party-source.tar.gz"
$git = (Get-Command git.exe -ErrorAction Stop).Source
$tar = (Get-Command tar.exe -ErrorAction Stop).Source

if ((Test-Path -LiteralPath $staging) -or (Test-Path -LiteralPath $archive)) {
    throw "Third-party source output already exists: $Version"
}

function Assert-Commit([string]$Repository, [string]$Expected) {
    $headOutput = @(& $git -c "safe.directory=$Repository" -C $Repository rev-parse HEAD)
    if ($LASTEXITCODE -ne 0 -or $headOutput.Count -ne 1) {
        throw "Unexpected source commit in $Repository"
    }
    $head = $headOutput[0].Trim()
    if ($head -ne $Expected) {
        throw "Unexpected source commit in $Repository"
    }
    if (& $git -c "safe.directory=$Repository" -C $Repository status --porcelain) {
        throw "Source checkout is not clean: $Repository"
    }
    if (Test-Path -LiteralPath (Join-Path $Repository '.gitmodules')) {
        throw "Unpinned submodules are not allowed: $Repository"
    }
}

Assert-Commit $scramjet.Path $scramjetCommit
Assert-Commit $epoxy.Path $epoxyCommit

$tags = @(& $git -c "safe.directory=$($scramjet.Path)" -C $scramjet.Path tag --points-at HEAD)
foreach ($tag in 'v2.0.67-alpha.2', 'v0.0.14-controller') {
    if ($tags -notcontains $tag) {
        throw "Scramjet source is missing required tag: $tag"
    }
}
$core = Get-Content -LiteralPath (Join-Path $scramjet.Path 'packages/core/package.json') -Raw | ConvertFrom-Json
$controller = Get-Content -LiteralPath (Join-Path $scramjet.Path 'packages/controller/package.json') -Raw | ConvertFrom-Json
if ($core.name -ne '@mercuryworkshop/scramjet' -or $core.version -ne '2.0.67-alpha.2' -or $core.license -ne 'AGPL-3.0-only') {
    throw 'Scramjet package metadata does not match SOURCE.lock.'
}
if ($controller.name -ne '@mercuryworkshop/scramjet-controller' -or $controller.version -ne '0.0.14' -or $controller.license -ne 'AGPL-3.0-only') {
    throw 'Scramjet Controller metadata does not match SOURCE.lock.'
}

$patch = Join-Path $moduleRoot 'web/patches/epoxy-websocket-origin-form.patch'
& $git -c "safe.directory=$($epoxy.Path)" -C $epoxy.Path apply --check $patch
if ($LASTEXITCODE -ne 0) {
    throw 'The production Epoxy patch no longer applies to the pinned source.'
}

New-Item -ItemType Directory -Path $staging | Out-Null
foreach ($directory in 'upstream', 'patches', 'build', 'licenses', 'metadata') {
    New-Item -ItemType Directory -Path (Join-Path $staging $directory) | Out-Null
}

$scramjetArchive = Join-Path $staging "upstream/scramjet-$scramjetCommit.tar.gz"
$epoxyArchive = Join-Path $staging "upstream/epoxy-tls-$epoxyCommit.tar.gz"
& $git -c "safe.directory=$($scramjet.Path)" -C $scramjet.Path archive --format=tar.gz --prefix="scramjet-$scramjetCommit/" --output=$scramjetArchive HEAD
if ($LASTEXITCODE -ne 0) {
    throw 'Unable to archive Scramjet source.'
}
& $git -c "safe.directory=$($epoxy.Path)" -C $epoxy.Path archive --format=tar.gz --prefix="epoxy-tls-$epoxyCommit/" --output=$epoxyArchive HEAD
if ($LASTEXITCODE -ne 0) {
    throw 'Unable to archive Epoxy source.'
}

Copy-Item -LiteralPath $patch -Destination (Join-Path $staging 'patches/epoxy-websocket-origin-form.patch')
Copy-Item -LiteralPath (Join-Path $moduleRoot 'web/scripts/build-epoxy.ps1') -Destination (Join-Path $staging 'build/build-epoxy.ps1')
Copy-Item -LiteralPath (Join-Path $epoxy.Path 'client/LICENSE') -Destination (Join-Path $staging 'licenses/AGPL-3.0-only.txt')
Copy-Item -LiteralPath (Join-Path $epoxy.Path 'wisp/LICENSE') -Destination (Join-Path $staging 'licenses/MIT-epoxy-wisp.txt')
Copy-Item -LiteralPath (Join-Path $moduleRoot 'third_party/scramjet/SOURCE.lock') -Destination (Join-Path $staging 'metadata/scramjet.SOURCE.lock')
Copy-Item -LiteralPath (Join-Path $moduleRoot 'third_party/scramjet/README.md') -Destination (Join-Path $staging 'metadata/scramjet.README.md')
Copy-Item -LiteralPath (Join-Path $moduleRoot 'web/vendor/epoxy/SOURCE.md') -Destination (Join-Path $staging 'metadata/epoxy.SOURCE.md')

$readme = @"
# WebVPN third-party corresponding source inputs

Release: $Version

This archive contains the complete pinned upstream trees used for Scramjet,
Scramjet Controller, and Epoxy TLS, plus the WebVPN Epoxy patch, deterministic
build script, license texts, and a SHA-256 manifest. The project-owned WebVPN
source and its owner-selected license must be added before this becomes the
public AGPL corresponding-source offer.

No npm tarball is treated as complete source. Both upstream Git trees are
archived from verified commits with no submodules.
"@
[IO.File]::WriteAllText((Join-Path $staging 'README.md'), $readme, [Text.UTF8Encoding]::new($false))

$manifest = foreach ($file in Get-ChildItem -LiteralPath $staging -Recurse -File | Sort-Object FullName) {
    $relative = [IO.Path]::GetRelativePath($staging, $file.FullName).Replace('\', '/')
    $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $relative"
}
[IO.File]::WriteAllText((Join-Path $staging 'manifest.sha256'), (($manifest -join "`n") + "`n"), [Text.UTF8Encoding]::new($false))

& $tar -czf $archive -C $staging .
if ($LASTEXITCODE -ne 0) {
    throw 'Unable to create third-party source archive.'
}

$file = Get-Item -LiteralPath $archive
[pscustomobject]@{
    Version = $Version
    Archive = $file.FullName
    Bytes = $file.Length
    SHA256 = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    ScramjetCommit = $scramjetCommit
    EpoxyCommit = $epoxyCommit
}
