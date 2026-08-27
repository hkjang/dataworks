[CmdletBinding()]
param(
    [string]$Version,
    [string]$PrevVersion,
    [switch]$Edit  # update an existing release's notes instead of creating it (no asset upload)
)

$ErrorActionPreference = "Stop"

if (-not $Version) {
    throw "Version parameter is required. Example: pwsh -File scripts/gh_release.ps1 -Version vVERSION -PrevVersion vPREVIOUS"
}
if (-not $PrevVersion) {
    throw "PrevVersion parameter is required. Example: pwsh -File scripts/gh_release.ps1 -Version vVERSION -PrevVersion vPREVIOUS"
}

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$cleanVer = $Version.TrimStart('v')

# Load changelog file
$changelogPath = Join-Path $PSScriptRoot "changelog.txt"
if (-not (Test-Path $changelogPath)) {
    throw "changelog.txt file is missing at $changelogPath"
}
$utf8 = [System.Text.Encoding]::UTF8
$changelogLines = [System.IO.File]::ReadAllLines($changelogPath, $utf8)

# Verify versions exist and extract all logs between Version and PrevVersion (exclusive)
$foundStart = $false
$foundEnd = $false
$extractedLogs = @()

foreach ($line in $changelogLines) {
    if ($line -match "^$Version`:") {
        $foundStart = $true
        $note = $line.Substring($line.IndexOf(':') + 1).Trim()
        if ($note) { $extractedLogs += $note }
        continue
    }
    
    if ($foundStart) {
        if ($line -match "^$PrevVersion`:") {
            $foundEnd = $true
            break
        }
        $extractedLogs += $line
    }
}

if (-not $foundStart) {
    throw "Target version $Version is not documented in scripts/changelog.txt."
}
if (-not $foundEnd) {
    throw "Previous version $PrevVersion is not documented in scripts/changelog.txt."
}

$targetChangelog = $extractedLogs -join "`r`n"

$notes = "## Data Works v" + $cleanVer + "`r`n`r`n"
$notes += [regex]::Unescape("### \uc8fc\uc694 \ubcc0\uacbd \uc0ac\ud56d`r`n")
$notes += $targetChangelog + "`r`n`r`n"

$notes += [regex]::Unescape("### \ubc30\ud3ec \ud30c\uc77c`r`n")
$notes += [regex]::Unescape("| \ud30c\uc77c | \uc124\uba85 |`r`n")
$notes += "|------|------|`r`n"
$notes += "| dataworks-v" + $cleanVer + [regex]::Unescape(".tar.gz | \uc624\ud504\ub77c\uc778 \uc801\uc7ac \uac00\ub2a5\ud55c Data Works Docker \uc774\ubbf8\uc9c0 (linux/amd64) |`r`n`r`n")

$notes += [regex]::Unescape("### \ube60\ub978 \uc2dc\uc791`r`n")
$notes += '```' + "bash`r`n"
$notes += [regex]::Unescape("# \uc774\ubbf8\uc9c0 \ub85c\ub4dc`r`n")
$notes += "gunzip -c dataworks-" + $Version + ".tar.gz | docker load`r`n`r`n"
$notes += [regex]::Unescape("# \uc2e4\ud589`r`n")
$notes += "docker run -d --name dataworks --restart=always \`r`n"
$notes += "  -p 8080:8080 \`r`n"
$notes += "  -e POSTGRES_DSN='postgres://dataworks:change-me@postgres:5432/dataworks?sslmode=disable' \`r`n"
$notes += "  -e BOOTSTRAP_ADMIN='admin@dataworks.local' \`r`n"
$notes += "  -e BOOTSTRAP_ADMIN_PASSWORD='change-me' \`r`n"
$notes += "  -e ENCRYPTION_KEY='replace-with-64-hex-characters' \`r`n"
$notes += "  dataworks:" + $Version + "`r`n"
$notes += '```'

$notesPath = Join-Path $repoRoot "release\release-notes.txt"
# Ensure the release directory exists
$releaseDir = Split-Path -Parent $notesPath
if (-not (Test-Path $releaseDir)) {
    New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null
}

# Write as UTF-8 WITHOUT BOM via .NET so the GitHub release body has no leading BOM
# and is identical regardless of which PowerShell edition runs this script.
[System.IO.File]::WriteAllText($notesPath, $notes, (New-Object System.Text.UTF8Encoding($false)))

if ($Edit) {
    # Re-publish corrected notes for an already-created release (no asset re-upload).
    gh release edit $Version --repo hkjang/dataworks --notes-file $notesPath
} else {
    $assetPath = "release\dataworks-$Version.tar.gz"
    if (-not (Test-Path $assetPath)) {
        throw "Release asset is missing: $assetPath"
    }
    $assets = @($assetPath)
    gh release create $Version $assets --repo hkjang/dataworks --verify-tag --title "$Version - Data Works" --notes-file $notesPath
}

Remove-Item $notesPath -ErrorAction SilentlyContinue
