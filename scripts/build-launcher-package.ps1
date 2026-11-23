param(
  [Parameter(Mandatory = $true)][string]$Root,
  [string]$Out = "",
  [string]$Version = "1.3.9",
  [string]$BaseURL = "https://qqt.005656.xyz"
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path $Root).Path
if ($Out -eq "") { $Out = Join-Path $Root ("dist\WuluCandyHouse-Launcher-" + $Version + ".zip") }
$staging = Join-Path $env:TEMP ("wulu-launcher-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $staging | Out-Null

# Distributable package: our launcher, server, configs and docs only.
# The Tencent client tree is never copied. Players must supply their own
# legally obtained 5.2.1.201 client.
Copy-Item (Join-Path $Root "*") $staging -Recurse -Force -ErrorAction SilentlyContinue
foreach ($exclude in @(
  "dist",
  "runtime\data",
  "runtime\backups",
  "runtime\logs",
  "runtime\updates",
  "runtime\captures",
  "runtime\client-patched",
  "runtime\bootstrap-config.json",
  "runtime\run-state.json",
  "runtime\version.json"
)) {
  $path = Join-Path $staging $exclude
  if (Test-Path $path) { Remove-Item $path -Recurse -Force }
}
Get-ChildItem $staging -Recurse -Filter "*.bak" -ErrorAction SilentlyContinue | Remove-Item -Force

$ctlSource = Join-Path $Root "dist\qqt-launcher-ctl.exe"
if (-not (Test-Path $ctlSource)) { $ctlSource = "C:\Users\78502\WULU\project\qqtang-src\dist\qqt-launcher-ctl.exe" }
if (Test-Path $ctlSource) {
  New-Item -ItemType Directory -Force -Path (Join-Path $staging "runtime\bin") | Out-Null
  Copy-Item $ctlSource (Join-Path $staging "runtime\bin\qqt-launcher-ctl.exe") -Force
}

New-Item -ItemType Directory -Force -Path (Split-Path $Out) | Out-Null
if (Test-Path $Out) { Remove-Item $Out -Force }
Compress-Archive -Path (Join-Path $staging "*") -DestinationPath $Out -CompressionLevel Optimal
Remove-Item $staging -Recurse -Force

$hash = (Get-FileHash $Out -Algorithm SHA256).Hash.ToLower()
$size = (Get-Item $Out).Length
$json = @{
  server_ip = "1.2.3.4"
  mode = "remote-host"
  version = $Version
  package_url = "$BaseURL/" + (Split-Path $Out -Leaf)
  sha256 = $hash
  ports = @{
    directory = 18080; game = 18000; shop = 18001; secondary = 17000; tls = 18443; gm = 18100; udp = 18000
  }
} | ConvertTo-Json -Depth 4
$jsonPath = [System.IO.Path]::ChangeExtension($Out, ".json")
$json | Set-Content $jsonPath -Encoding UTF8

Write-Output ("package " + $Out + " " + $size + " bytes")
Write-Output ("sha256 " + $hash)
Write-Output ("json " + $jsonPath)
