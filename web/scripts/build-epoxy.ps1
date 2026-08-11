param(
  [string]$CacheDirectory = (Join-Path ([IO.Path]::GetTempPath()) 'webvpn-epoxy-build'),
  [string]$SourceArchivePath = '',
  [string]$RustupHome = '',
  [string]$CargoHome = '',
  [string]$RustToolchain = 'nightly-2026-08-06',
  [string]$WasmBindgenPath = '',
  [string]$ZigPath = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$sourceCommit = '93d5a726894b2f16bad54c4a3801446cbbd22d26'
$sourceSha256 = 'd5c7d191b5a5c6f7b9b0280688fe477f45dc60a599e9beef34a0f2d0e0c60fff'
$rustcCommit = '7608eb7b07eaf93f16d7cf5bcb2098eca87503df'
$wasmBindgenVersion = '0.2.100'
$wasmBindgenSha256 = '54a3fb947464388a468ade86d65ffa334d6d2c74b7982723b34ecf6ec8c213d8'
$zigVersion = '0.16.0'
$zigSha256 = '68659eb5f1e4eb1437a722f1dd889c5a322c9954607f5edcf337bc3684a75a7e'

$webRoot = Split-Path $PSScriptRoot -Parent
$patchPath = Join-Path $webRoot 'patches\epoxy-websocket-origin-form.patch'
$outputDirectory = Join-Path $webRoot 'vendor\epoxy'
$sourceDirectory = Join-Path $CacheDirectory 'e'
$targetDirectory = Join-Path $CacheDirectory 't'
$stageDirectory = Join-Path $CacheDirectory 'pkg'
$markerPath = Join-Path $sourceDirectory '.webvpn-patched'

function Test-Hash([string]$Path, [string]$Expected) {
  (Test-Path -LiteralPath $Path) -and
    ((Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() -eq $Expected)
}

function Get-VerifiedFile([string]$Uri, [string]$Path, [string]$Expected) {
  if (Test-Hash $Path $Expected) {
    return
  }
  $partial = "$Path.partial"
  for ($attempt = 1; $attempt -le 3; $attempt += 1) {
    try {
      Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $partial
      if (-not (Test-Hash $partial $Expected)) {
        throw "SHA-256 mismatch for $Uri"
      }
      Move-Item -LiteralPath $partial -Destination $Path -Force
      return
    } catch {
      if ($attempt -eq 3) {
        throw
      }
      Start-Sleep -Seconds (2 * $attempt)
    }
  }
}

New-Item -ItemType Directory -Path $CacheDirectory -Force | Out-Null
if (-not $SourceArchivePath) {
  $SourceArchivePath = Join-Path $CacheDirectory "epoxy-$sourceCommit.tar.gz"
  Get-VerifiedFile `
    "https://github.com/MercuryWorkshop/epoxy-tls/archive/$sourceCommit.tar.gz" `
    $SourceArchivePath `
    $sourceSha256
} elseif (-not (Test-Hash $SourceArchivePath $sourceSha256)) {
  throw 'The supplied Epoxy source archive failed SHA-256 verification.'
}

if (-not (Test-Path -LiteralPath $markerPath)) {
  if (Test-Path -LiteralPath $sourceDirectory) {
    throw "Unverified source directory already exists: $sourceDirectory"
  }
  New-Item -ItemType Directory -Path $sourceDirectory | Out-Null
  & tar -xzf $SourceArchivePath --strip-components=1 -C $sourceDirectory
  if ($LASTEXITCODE -ne 0) {
    throw 'Unable to extract the Epoxy source archive.'
  }
  Push-Location $sourceDirectory
  try {
    & git apply --check $patchPath
    if ($LASTEXITCODE -ne 0) {
      throw 'Epoxy patch preflight failed.'
    }
    & git apply $patchPath
    if ($LASTEXITCODE -ne 0) {
      throw 'Unable to apply the Epoxy patch.'
    }
  } finally {
    Pop-Location
  }
  Set-Content -LiteralPath $markerPath -Value $sourceCommit -NoNewline
}

if (-not $RustupHome) {
  $RustupHome = Join-Path $CacheDirectory 'rustup'
}
if (-not $CargoHome) {
  $CargoHome = Join-Path $CacheDirectory 'cargo'
}
$env:RUSTUP_HOME = $RustupHome
$env:CARGO_HOME = $CargoHome

$installed = (& rustup toolchain list) -match "^$([regex]::Escape($RustToolchain))(-|\s)"
if (-not $installed) {
  & rustup toolchain install $RustToolchain --profile minimal --component rust-src --target wasm32-unknown-unknown --no-self-update
  if ($LASTEXITCODE -ne 0) {
    throw 'Unable to install the isolated pinned Rust toolchain.'
  }
}
$rustcVersion = (& rustup run $RustToolchain rustc -Vv) -join "`n"
if ($rustcVersion -notmatch "commit-hash: $rustcCommit") {
  throw 'The Rust compiler commit does not match the pinned build.'
}

if (-not $WasmBindgenPath) {
  $archive = Join-Path $CacheDirectory "wasm-bindgen-$wasmBindgenVersion.zip.tar.gz"
  $directory = Join-Path $CacheDirectory "wasm-bindgen-$wasmBindgenVersion-x86_64-pc-windows-msvc"
  Get-VerifiedFile `
    "https://github.com/wasm-bindgen/wasm-bindgen/releases/download/$wasmBindgenVersion/wasm-bindgen-$wasmBindgenVersion-x86_64-pc-windows-msvc.tar.gz" `
    $archive `
    $wasmBindgenSha256
  if (-not (Test-Path -LiteralPath $directory)) {
    & tar -xzf $archive -C $CacheDirectory
  }
  $WasmBindgenPath = Join-Path $directory 'wasm-bindgen.exe'
}
if ((& $WasmBindgenPath --version) -ne "wasm-bindgen $wasmBindgenVersion") {
  throw 'wasm-bindgen does not match the pinned version.'
}

if (-not $ZigPath) {
  $archive = Join-Path $CacheDirectory "zig-x86_64-windows-$zigVersion.zip"
  $directory = Join-Path $CacheDirectory "zig-x86_64-windows-$zigVersion"
  Get-VerifiedFile `
    "https://ziglang.org/download/$zigVersion/zig-x86_64-windows-$zigVersion.zip" `
    $archive `
    $zigSha256
  if (-not (Test-Path -LiteralPath $directory)) {
    Expand-Archive -LiteralPath $archive -DestinationPath $CacheDirectory
  }
  $ZigPath = Join-Path $directory 'zig.exe'
}
if ((& $ZigPath version) -ne $zigVersion) {
  throw 'Zig does not match the pinned version.'
}

$env:CARGO_TARGET_DIR = $targetDirectory
$env:ZIG_GLOBAL_CACHE_DIR = Join-Path $CacheDirectory 'zig-global-cache'
$env:ZIG_LOCAL_CACHE_DIR = Join-Path $CacheDirectory 'zig-local-cache'
$env:CC_wasm32_unknown_unknown = "$ZigPath cc"
$env:AR_wasm32_unknown_unknown = "$ZigPath ar"
$env:CFLAGS_wasm32_unknown_unknown = '--target=wasm32-freestanding -O3'
$env:RUSTFLAGS = '-Zlocation-detail=none -C target-cpu=mvp --cfg getrandom_backend="wasm_js"'

& rustup run $RustToolchain cargo build `
  --locked `
  --manifest-path (Join-Path $sourceDirectory 'client\Cargo.toml') `
  --target wasm32-unknown-unknown `
  -Z build-std=panic_abort,std `
  -Z build-std-features=optimize_for_size `
  --release
if ($LASTEXITCODE -ne 0) {
  throw 'Epoxy WASM compilation failed.'
}

New-Item -ItemType Directory -Path $stageDirectory -Force | Out-Null
& $WasmBindgenPath `
  --target web `
  --out-dir $stageDirectory `
  (Join-Path $targetDirectory 'wasm32-unknown-unknown\release\epoxy_client.wasm')
if ($LASTEXITCODE -ne 0) {
  throw 'wasm-bindgen generation failed.'
}

New-Item -ItemType Directory -Path $outputDirectory -Force | Out-Null
foreach ($name in 'epoxy_client.js', 'epoxy_client.d.ts', 'epoxy_client_bg.wasm', 'epoxy_client_bg.wasm.d.ts') {
  Copy-Item -LiteralPath (Join-Path $stageDirectory $name) -Destination (Join-Path $outputDirectory $name) -Force
}
Copy-Item -LiteralPath (Join-Path $stageDirectory 'snippets') -Destination $outputDirectory -Recurse -Force
Copy-Item -LiteralPath (Join-Path $sourceDirectory 'client\LICENSE') -Destination (Join-Path $outputDirectory 'LICENSE') -Force

$wasmSha256 = (Get-FileHash -LiteralPath (Join-Path $outputDirectory 'epoxy_client_bg.wasm') -Algorithm SHA256).Hash.ToLowerInvariant()
Write-Output "Epoxy $sourceCommit built successfully; wasm_sha256=$wasmSha256"
