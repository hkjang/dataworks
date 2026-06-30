# Data Works Rollback Script
# Usage: .\scripts\rollback.ps1
#
# This script restores the most recent database backup and stops the service.

param(
    [string]$DataDir = "data",
    [string]$DbFile = "gateway.db"
)

$ErrorActionPreference = "Stop"

Write-Host "[Data Works Rollback]" -ForegroundColor Yellow

# Find the most recent backup
$backups = Get-ChildItem -Path $DataDir -Filter "$DbFile.bak.*" -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending

if ($backups.Count -eq 0) {
    Write-Host "No backup files found in $DataDir" -ForegroundColor Red
    exit 1
}

$latestBackup = $backups[0]
Write-Host "Found backup: $($latestBackup.Name)" -ForegroundColor Cyan

# Stop any running Data Works process
$procs = Get-Process -Name "dataworks","clustara" -ErrorAction SilentlyContinue
if ($procs) {
    Write-Host "Stopping running processes..." -ForegroundColor Yellow
    $procs | Stop-Process -Force
    Start-Sleep -Seconds 2
}

# Restore backup
$targetPath = Join-Path $DataDir $DbFile
Write-Host "Restoring $($latestBackup.FullName) -> $targetPath" -ForegroundColor Cyan
Copy-Item -Path $latestBackup.FullName -Destination $targetPath -Force

Write-Host "Rollback complete." -ForegroundColor Green
Write-Host "To restart, run: go run ./cmd/dataworks" -ForegroundColor Gray
