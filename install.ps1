param(
  [string]$Version = "latest",
  [string]$InstallDir = "$env:LOCALAPPDATA\Programs\cache-cleaner"
)
$ErrorActionPreference = "Stop"
$Owner = "everydaydevopsio"
$Repo = "bilge-pump"
if ($Version -eq "latest") {
  $release = Invoke-RestMethod "https://api.github.com/repos/$Owner/$Repo/releases/latest"
  $Version = $release.tag_name
}
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
$archive = "cache-cleaner-$($Version.TrimStart('v'))-windows-$arch.zip"
$url = "https://github.com/$Owner/$Repo/releases/download/$Version/$archive"
$temp = Join-Path ([System.IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $temp | Out-Null
try {
  $zip = Join-Path $temp "release.zip"
  Invoke-WebRequest -Uri $url -OutFile $zip
  Expand-Archive -Path $zip -DestinationPath $temp
  New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
  Get-ChildItem $temp -Filter "*.exe" | Copy-Item -Destination $InstallDir -Force
  Write-Host "Installed cache-cleaner tools to $InstallDir"
  Write-Host "Add this directory to your user PATH, then run: cache-cleaner scan"
} finally {
  Remove-Item -Recurse -Force $temp
}
