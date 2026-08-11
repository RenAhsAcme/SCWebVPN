param(
    [string]$Output = "dist/webvpn-agent-linux-amd64"
)

$ErrorActionPreference = "Stop"
$moduleRoot = Split-Path -Parent $PSScriptRoot
$destination = Join-Path $moduleRoot $Output
$destinationDirectory = Split-Path -Parent $destination
New-Item -ItemType Directory -Force -Path $destinationDirectory | Out-Null

$previousCGO = $env:CGO_ENABLED
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
$previousCache = $env:GOCACHE
Push-Location $moduleRoot
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $env:GOCACHE = Join-Path $moduleRoot ".tmp-go-cache"
    & go build -trimpath -ldflags="-s -w" -o $destination ./cmd/agent
    if ($LASTEXITCODE -ne 0) {
        throw "Agent cross-build failed"
    }
} finally {
    $env:CGO_ENABLED = $previousCGO
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:GOCACHE = $previousCache
    Pop-Location
}

$file = Get-Item -LiteralPath $destination
$hash = Get-FileHash -Algorithm SHA256 -LiteralPath $destination
[pscustomobject]@{
    Path = $file.FullName
    Bytes = $file.Length
    SHA256 = $hash.Hash
}
