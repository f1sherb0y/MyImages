# Fetches MyGo at the release MyImages builds against into third_party\mygo
# and applies MyImages's changes to it (patches\mygo.patch). Run once after
# cloning; go.mod points at the result.
$ErrorActionPreference = 'Stop'
$commit = '4811a2869b5a66d2b7e21a9eb5993fcff544460b' # v0.9.0
$root = Split-Path $PSScriptRoot -Parent
$dir = Join-Path $root 'third_party\mygo'
if (Test-Path (Join-Path $dir 'go.mod')) {
    Write-Host "third_party\mygo is there already; delete it to fetch it again."
    exit 0
}
New-Item -ItemType Directory -Force $dir | Out-Null
git -C $dir init -q
git -C $dir remote add origin https://github.com/egoist/mygo.git
git -C $dir fetch -q --depth 1 origin $commit
git -C $dir checkout -q FETCH_HEAD
git -C $dir apply (Join-Path $root 'patches\mygo.patch')
Write-Host "MyGo $($commit.Substring(0,7)) with MyImages's patch is in third_party\mygo."
