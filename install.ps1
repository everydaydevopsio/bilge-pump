param(
  [string]$Version = "latest",
  [string]$InstallDir = "$env:LOCALAPPDATA\Programs\bilge"
)
$ErrorActionPreference = "Stop"
$Owner = "everydaydevopsio"
$Repo = "bilge-pump"
if ($Version -eq "latest") {
  $release = Invoke-RestMethod "https://api.github.com/repos/$Owner/$Repo/releases/latest"
  $Version = $release.tag_name
}
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
$versionCore = $Version.TrimStart('v') -replace '[-+].*$', ''
$releaseVersion = [version]$versionCore
$archivePrefix = if ($releaseVersion -ge [version]'2.1.0') { "bilge" } else { "cache-cleaner" }
$launcher = if ($releaseVersion -ge [version]'2.1.0') { "bilge" } else { "cache-cleaner" }
$archive = "$archivePrefix-$($Version.TrimStart('v'))-windows-$arch.zip"
$url = "https://github.com/$Owner/$Repo/releases/download/$Version/$archive"
$temp = Join-Path ([System.IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $temp | Out-Null
try {
  $zip = Join-Path $temp "release.zip"
  Invoke-WebRequest -Uri $url -OutFile $zip
  Expand-Archive -Path $zip -DestinationPath $temp
  New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
  Get-ChildItem $temp -Filter "*.exe" | Copy-Item -Destination $InstallDir -Force
  Write-Host "Installed bilge tools to $InstallDir"
  Write-Host "Add this directory to your user PATH, then run: $launcher scan"
} finally {
  Remove-Item -Recurse -Force $temp
}
