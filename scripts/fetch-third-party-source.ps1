param(
    [string]$OutputRoot = '../.tmp-scwebvpn-source'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$scramjetCommit = 'c26bfc6d7f7c7f4dac52ce182a2ceab90e851823'
$epoxyCommit = '93d5a726894b2f16bad54c4a3801446cbbd22d26'
$output = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot $OutputRoot))
$scramjet = Join-Path $output 'scramjet-c26bfc6d'
$epoxy = Join-Path $output 'epoxy-93d5a726'

if (Test-Path -LiteralPath $output) {
    throw "Third-party source output already exists: $output"
}
New-Item -ItemType Directory -Path $output | Out-Null

git -c http.sslBackend=openssl clone --filter=blob:none --no-checkout `
    https://github.com/MercuryWorkshop/scramjet.git $scramjet
if ($LASTEXITCODE) { throw 'Scramjet source clone failed.' }
git -C $scramjet checkout --detach $scramjetCommit
if ($LASTEXITCODE) { throw 'Scramjet source checkout failed.' }

git -c http.sslBackend=openssl clone --filter=blob:none --no-checkout `
    https://github.com/MercuryWorkshop/epoxy-tls.git $epoxy
if ($LASTEXITCODE) { throw 'Epoxy source clone failed.' }
git -C $epoxy checkout --detach $epoxyCommit
if ($LASTEXITCODE) { throw 'Epoxy source checkout failed.' }

[pscustomobject]@{
    Scramjet = $scramjet
    ScramjetCommit = (git -C $scramjet rev-parse HEAD).Trim()
    Epoxy = $epoxy
    EpoxyCommit = (git -C $epoxy rev-parse HEAD).Trim()
}
