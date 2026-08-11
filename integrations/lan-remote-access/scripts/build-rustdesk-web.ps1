[CmdletBinding()]
param(
  [string]$OutputPath = (Join-Path $PSScriptRoot '..\rustdesk-web-open'),
  [string]$WorkPath = (Join-Path ([System.IO.Path]::GetTempPath()) 'lan-remote-rustdesk-web')
)

$ErrorActionPreference = 'Stop'
$OutputPath = [System.IO.Path]::GetFullPath($OutputPath)
$WorkPath = [System.IO.Path]::GetFullPath($WorkPath)
$sourceCommit = '525b5e561faf824850c71500adf463e4e0a504d4'
$ogvSha256 = '4575bb7984c24d8872862df764bc0fdc50a18e9d9753a44e53b0640948093078'
$yuvCanvasSha256 = 'f76381274f2e2524f8a82ce39201080a2a568f5a6c6983b7f4befac2e124c7d1'
$libopusSha256 = '12caa23773c7c84c44fa9c12fa4a9f8d319d46e3f7935d2d5fa308c12603d7dd'
$libopusWasmSha256 = '18fd261f9c18c442873e6dab73cfafc5a7560aea652f5544b4dd99fae2d1944c'
$buildFiles = Join-Path $PSScriptRoot '..\rustdesk-web-build'

function Assert-Hash([string]$Path, [string]$Expected) {
  $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
  if ($actual -ne $Expected) { throw "SHA-256 mismatch: $Path`nexpected: $Expected`nactual:   $actual" }
}

function Invoke-Download([string]$Uri, [string]$Destination) {
  Invoke-WebRequest -Uri $Uri -OutFile $Destination -UserAgent 'Mozilla/5.0 LAN-Remote-Builder'
}

foreach ($command in 'git', 'node', 'npm', 'corepack', 'python', 'tar') {
  if (-not (Get-Command $command -ErrorAction SilentlyContinue)) { throw "Missing build command: $command" }
}
if (Test-Path -LiteralPath $WorkPath) { throw "WorkPath must be absent: $WorkPath" }
if (Test-Path -LiteralPath $OutputPath) { throw "OutputPath must be absent: $OutputPath" }

$source = Join-Path $WorkPath 'source'
$deps = Join-Path $WorkPath 'deps'
New-Item -ItemType Directory -Path $WorkPath, $deps | Out-Null
# Corepack's Yarn 3 bundle is CommonJS. Isolate it from any parent repository
# that declares a root-level `"type": "module"`.
[System.IO.File]::WriteAllText(
  (Join-Path $WorkPath 'package.json'),
  '{"type":"commonjs"}',
  [System.Text.UTF8Encoding]::new($false)
)

git -c http.sslBackend=openssl clone --filter=blob:none --no-checkout https://github.com/MonsieurBiche/rustdesk-web-client $source
if ($LASTEXITCODE) { throw 'Source clone failed.' }
git -C $source config http.sslBackend openssl
if ($LASTEXITCODE) { throw 'Source TLS backend configuration failed.' }
git -C $source checkout --detach $sourceCommit
if ($LASTEXITCODE) { throw 'Source checkout failed.' }
git -C $source apply (Join-Path $buildFiles 'lan-remote.patch')
if ($LASTEXITCODE) { throw 'SCWebVPN Remote source patch failed.' }
$web = Join-Path $source 'flutter\web'
Get-ChildItem -LiteralPath (Join-Path $web 'v1') -Force | Copy-Item -Destination $web -Recurse -Force
$webJs = Join-Path $web 'js'
Copy-Item -LiteralPath (Join-Path $buildFiles 'classic-ui.js') -Destination (Join-Path $webJs 'src\ui.js') -Force
Copy-Item -LiteralPath (Join-Path $buildFiles 'classic-style.css') -Destination (Join-Path $webJs 'src\style.css') -Force

$env:COREPACK_HOME = Join-Path $WorkPath 'corepack'
corepack prepare yarn@3.2.0 --activate
if ($LASTEXITCODE) { throw 'Yarn activation failed.' }
$pythonPath = (Get-Command python).Source
"@`"$pythonPath`" %*" | Set-Content -LiteralPath (Join-Path $WorkPath 'python3.cmd') -Encoding ascii
$env:PATH = "$WorkPath;$env:PATH"
Push-Location $webJs
try {
  yarn install --immutable
  if ($LASTEXITCODE) { throw 'Web core dependency install failed.' }
  yarn build
  if ($LASTEXITCODE) { throw 'Web core build failed.' }
} finally {
  Pop-Location
}

Push-Location $deps
try {
  npm pack ogv@1.8.6 --ignore-scripts --registry=https://registry.npmjs.org
  if ($LASTEXITCODE) { throw 'OGV package download failed.' }
  npm pack yuv-canvas@1.2.6 --ignore-scripts --registry=https://registry.npmjs.org
  if ($LASTEXITCODE) { throw 'YUV Canvas package download failed.' }
} finally {
  Pop-Location
}
$ogvArchive = Join-Path $deps 'ogv-1.8.6.tgz'
$yuvArchive = Join-Path $deps 'yuv-canvas-1.2.6.tgz'
Assert-Hash $ogvArchive $ogvSha256
Assert-Hash $yuvArchive $yuvCanvasSha256
New-Item -ItemType Directory -Path (Join-Path $deps 'ogv'), (Join-Path $deps 'yuv') | Out-Null
tar -xf $ogvArchive -C (Join-Path $deps 'ogv')
tar -xf $yuvArchive -C (Join-Path $deps 'yuv')

Copy-Item -LiteralPath (Join-Path $deps 'yuv\package\src') -Destination (Join-Path $webJs 'yuv-src') -Recurse
Copy-Item -LiteralPath (Join-Path $deps 'yuv\package\build') -Destination (Join-Path $webJs 'build') -Recurse
Copy-Item -LiteralPath (Join-Path $buildFiles 'build-yuv-vite.mjs') -Destination $webJs
Push-Location $webJs
try {
  node .\build-yuv-vite.mjs
  if ($LASTEXITCODE) { throw 'YUV Canvas browser bundle failed.' }
} finally {
  Pop-Location
}

New-Item -ItemType Directory -Path (Join-Path $OutputPath 'js') | Out-Null
Copy-Item -LiteralPath (Join-Path $buildFiles 'classic-index.html') -Destination (Join-Path $OutputPath 'index.html.template')
Copy-Item -LiteralPath @(
  (Join-Path $web 'yuv.js'),
  (Join-Path $web 'yuv.wasm'),
  (Join-Path $webJs 'runtime\yuv-canvas-1.2.6.js')
) -Destination $OutputPath
Copy-Item -LiteralPath (Join-Path $webJs 'dist') -Destination (Join-Path $OutputPath 'js') -Recurse
Copy-Item -LiteralPath (Join-Path $deps 'ogv\package\dist') -Destination (Join-Path $OutputPath 'ogvjs-1.8.6') -Recurse

Invoke-Download 'https://rustdesk.com/web/libopus.js' (Join-Path $OutputPath 'libopus.js')
Invoke-Download 'https://rustdesk.com/web/libopus.wasm' (Join-Path $OutputPath 'libopus.wasm')
Assert-Hash (Join-Path $OutputPath 'libopus.js') $libopusSha256
Assert-Hash (Join-Path $OutputPath 'libopus.wasm') $libopusWasmSha256

$versionFiles = @(
  'js\dist\index.js',
  'js\dist\vendor.js',
  'js\dist\index.css',
  'ogvjs-1.8.6\ogv.js',
  'yuv-canvas-1.2.6.js'
)
$versionMaterial = $versionFiles | ForEach-Object {
  (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $OutputPath $_)).Hash.ToLowerInvariant()
}
$versionBytes = [System.Text.Encoding]::UTF8.GetBytes($versionMaterial -join "`n")
$versionHash = [System.Security.Cryptography.SHA256]::HashData($versionBytes)
$runtimeVersion = ([System.Convert]::ToHexString($versionHash)).ToLowerInvariant().Substring(0, 16)
$indexPath = Join-Path $OutputPath 'index.html.template'
$indexHtml = [System.IO.File]::ReadAllText($indexPath)
if (-not $indexHtml.Contains('__RUSTDESK_WEB_VERSION__')) { throw 'Runtime index has no version placeholder.' }
$indexHtml = $indexHtml.Replace('__RUSTDESK_WEB_VERSION__', $runtimeVersion)
[System.IO.File]::WriteAllText($indexPath, $indexHtml, [System.Text.UTF8Encoding]::new($false))

$entryPath = Join-Path $OutputPath 'js\dist\index.js'
$entryScript = [System.IO.File]::ReadAllText($entryPath)
$versionedEntry = $entryScript.Replace('./vendor.js', "./vendor.js?v=$runtimeVersion")
if ($versionedEntry -eq $entryScript) { throw 'Runtime entry has no vendor import to version.' }
[System.IO.File]::WriteAllText($entryPath, $versionedEntry, [System.Text.UTF8Encoding]::new($false))

$manifest = "$OutputPath.sha256"
Get-ChildItem -LiteralPath $OutputPath -Recurse -File | Sort-Object FullName | ForEach-Object {
  $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
  $relative = $_.FullName.Substring($OutputPath.Length + 1).Replace('\', '/')
  "$hash  $relative"
} | Set-Content -LiteralPath $manifest -Encoding utf8NoBOM

Write-Host "Built: $OutputPath"
Write-Host "Manifest: $manifest"
