[CmdletBinding()]
param([string]$HostAlias = 'OpenWrt')

$ErrorActionPreference = 'Stop'
$deploymentRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$webRoot = Join-Path $deploymentRoot 'rustdesk-web-open'
$manifest = Join-Path $deploymentRoot 'rustdesk-web-open.sha256'
$portal = Join-Path $deploymentRoot 'portal\index.html'
$remoteScript = Join-Path $PSScriptRoot 'deploy-rustdesk-web-openwrt.sh'
$archive = Join-Path ([System.IO.Path]::GetTempPath()) 'lan-remote-rustdesk-web.tar.gz'

foreach ($command in 'tar', 'scp', 'ssh') {
  if (-not (Get-Command $command -ErrorAction SilentlyContinue)) { throw "Missing command: $command" }
}

$expected = @{}
Get-Content -LiteralPath $manifest | ForEach-Object {
  if ($_ -notmatch '^([0-9a-f]{64})  (.+)$') { throw "Invalid manifest line: $_" }
  $expected[$Matches[2]] = $Matches[1]
}
$actualFiles = Get-ChildItem -LiteralPath $webRoot -Recurse -File
if ($actualFiles.Count -ne $expected.Count) { throw 'Runtime file count does not match its manifest.' }
foreach ($file in $actualFiles) {
  $relative = $file.FullName.Substring($webRoot.Length + 1).Replace('\', '/')
  $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $file.FullName).Hash.ToLowerInvariant()
  if ($expected[$relative] -ne $actual) { throw "Runtime hash mismatch: $relative" }
}

tar -czf $archive -C $webRoot .
if ($LASTEXITCODE) { throw 'Archive creation failed.' }
scp -O $archive "${HostAlias}:/tmp/lan-remote-rustdesk-web.tar.gz"
if ($LASTEXITCODE) { throw 'Runtime upload failed.' }
scp -O $manifest "${HostAlias}:/tmp/lan-remote-rustdesk-web.sha256"
if ($LASTEXITCODE) { throw 'Manifest upload failed.' }
scp -O $portal "${HostAlias}:/tmp/lan-remote-portal.html"
if ($LASTEXITCODE) { throw 'Portal upload failed.' }
scp -O $remoteScript "${HostAlias}:/tmp/lan-remote-deploy-web.sh"
if ($LASTEXITCODE) { throw 'Deployment script upload failed.' }
ssh $HostAlias 'sh /tmp/lan-remote-deploy-web.sh'
if ($LASTEXITCODE) { throw 'OpenWrt deployment failed; inspect the printed stage or backup path.' }
