[CmdletBinding()]
param(
    [string]$Version,
    [string]$Image = "dataworks",
    [string]$Platform = "linux/amd64"
)

$ErrorActionPreference = "Stop"

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker is not in PATH. Please install Docker Desktop or Engine first."
}

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

if (-not $Version) {
    $stamp = (Get-Date).ToString("yyyyMMdd-HHmm")
    try {
        $shortSha = (git rev-parse --short HEAD 2>$null)
        if ($LASTEXITCODE -ne 0 -or -not $shortSha) { $shortSha = "nogit" }
    } catch {
        $shortSha = "nogit"
    }
    $Version = "$stamp-$shortSha"
}

$tag = "${Image}:${Version}"
$releaseDir = Join-Path $repoRoot "release"
New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null

$safeVersion = $Version -replace "[^A-Za-z0-9._-]", "_"
$tarPath  = Join-Path $releaseDir "${Image}-${safeVersion}.tar"
$gzPath   = "$tarPath.gz"

Write-Host "[1/3] docker build  $tag  (platform=$Platform)" -ForegroundColor Cyan
docker build `
    --platform $Platform `
    --build-arg "VERSION=$Version" `
    -t $tag `
    -f Dockerfile `
    .
if ($LASTEXITCODE -ne 0) { throw "docker build failed" }

Write-Host "[2/3] docker save -> $tarPath" -ForegroundColor Cyan
docker save -o $tarPath $tag
if ($LASTEXITCODE -ne 0) { throw "docker save failed" }

Write-Host "[3/3] gzip compression -> $gzPath" -ForegroundColor Cyan
if (Test-Path $gzPath) { Remove-Item $gzPath -Force }

$inputStream  = [System.IO.File]::OpenRead($tarPath)
$outputStream = [System.IO.File]::Create($gzPath)
try {
    $gzip = New-Object System.IO.Compression.GzipStream($outputStream, [System.IO.Compression.CompressionLevel]::Optimal)
    try {
        $inputStream.CopyTo($gzip)
    } finally {
        $gzip.Dispose()
    }
} finally {
    $outputStream.Dispose()
    $inputStream.Dispose()
}
Remove-Item $tarPath -Force

Write-Host ""
Write-Host "Release completed" -ForegroundColor Green
Write-Host "  Image  : $tag"
Write-Host "  Platform: $Platform"
Write-Host "  Asset   : $gzPath"
Write-Host "  Port    : 8080"
