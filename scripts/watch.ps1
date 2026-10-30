#!/usr/bin/env pwsh
# Rebuilds build/mayoi.exe when its Go sources change, and restarts it.
# Usage: pwsh scripts/watch.ps1 [folder]
param([string]$Folder = (Get-Location).Path)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$exe = Join-Path $root "build\mayoi.exe"
$dirs = @(
    (Join-Path $root "cmd\mayoi"),
    (Join-Path $root "internal"),
    (Join-Path $root "locales"),
    (Join-Path $root "third_party\mygo\ui")
)

function Newest {
    Get-ChildItem $dirs -Recurse -File -Filter *.go -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending | Select-Object -First 1 -ExpandProperty LastWriteTime
}

function Build {
    Push-Location $root
    try { & go build -o $exe ./cmd/mayoi } finally { Pop-Location }
}

function Restart {
    Get-Process mayoi -ErrorAction SilentlyContinue | Stop-Process -Force
    Start-Process -FilePath $exe -ArgumentList $Folder
}

Restart
$last = Newest
Write-Host "watching; edit a .go file to rebuild"

while ($true) {
    Start-Sleep -Seconds 1
    $now = Newest
    if ($now -eq $last) { continue }
    $last = $now
    Build
    if ($LASTEXITCODE -eq 0) {
        Restart
        Write-Host "rebuilt and restarted"
    } else {
        Write-Host "build failed; the running app stays"
    }
}
