param(
  [Parameter(Mandatory = $true)][string]$Root,
  [string]$Out = ""
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path $Root).Path
if ($Out -eq "") { $Out = Join-Path $Root "dist\WuluCandyHouse-Launcher.zip" }
$staging = Join-Path $env:TEMP ("wulu-launcher-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $staging | Out-Null

function Copy-Relative([string]$Relative) {
  $source = Join-Path $Root $Relative
  if (-not (Test-Path $source)) { return }
  $target = Join-Path $staging $Relative
  New-Item -ItemType Directory -Force -Path (Split-Path $target) | Out-Null
  Copy-Item $source $target -Recurse -Force
}

# Own artifacts only. The Tencent client tree is never packaged.
foreach ($item in @(
  "QQTang-Launcher.exe",
  "README.md",
  "start-server-windows.cmd",
  "start-server-linux-amd64.sh",
  "start-server-linux-arm64.sh",
  "runtime\bin",
  "configs",
  "data",
  "docs",
  "deploy"
)) { Copy-Relative $item }

New-Item -ItemType Directory -Force -Path (Split-Path $Out) | Out-Null
if (Test-Path $Out) { Remove-Item $Out -Force }
Compress-Archive -Path (Join-Path $staging "*") -DestinationPath $Out -CompressionLevel Optimal
Remove-Item $staging -Recurse -Force
Write-Output "已生成 $Out"
