param(
  [Parameter(Mandatory = $true)][string]$Root,
  [Parameter(Mandatory = $true)][string]$Package,
  [int]$Keep = 5,
  [string]$Ctl = ""
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path $Root).Path
$Package = (Resolve-Path $Package).Path
if ($Ctl -eq "") { $Ctl = Join-Path $Root "runtime\bin\qqt-launcher-ctl.exe" }
if (-not (Test-Path $Ctl)) { $Ctl = Join-Path $Root "dist\qqt-launcher-ctl.exe" }
if (-not (Test-Path $Ctl)) { throw "找不到 qqt-launcher-ctl.exe，请用 -Ctl 指定" }

$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$backup = Join-Path $Root "runtime\backups\hot-$stamp"
New-Item -ItemType Directory -Force -Path $backup | Out-Null

function Backup-Item([string]$Relative) {
  $source = Join-Path $Root $Relative
  if (-not (Test-Path $source)) { return }
  $target = Join-Path $backup $Relative
  New-Item -ItemType Directory -Force -Path (Split-Path $target) | Out-Null
  Copy-Item $source $target -Recurse -Force
}

foreach ($item in @("runtime\data", "runtime\bin", "configs", "runtime\client-patched\config")) {
  Backup-Item $item
}
Write-Output "已备份到 $backup"

& $Ctl -root $Root -action stop-clients | Out-Null
& $Ctl -root $Root -action stop-server | Out-Null

$temp = Join-Path $env:TEMP ("qqt-hot-" + $stamp)
New-Item -ItemType Directory -Force -Path $temp | Out-Null
Expand-Archive -Path $Package -DestinationPath $temp -Force
$sourceRoot = $temp
$children = Get-ChildItem $temp
if ($children.Count -eq 1 -and $children[0].PSIsContainer) { $sourceRoot = $children[0].FullName }
Copy-Item (Join-Path $sourceRoot "*") $Root -Recurse -Force

$result = & $Ctl -root $Root -action start-server | ConvertFrom-Json
if (-not $result.ok) {
  Write-Warning "升级后启动失败：$($result.message)，开始回滚"
  foreach ($relative in @("runtime\data", "runtime\bin", "configs")) {
    $source = Join-Path $backup $relative
    if (Test-Path $source) {
      $target = Join-Path $Root $relative
      Remove-Item $target -Recurse -Force -ErrorAction SilentlyContinue
      Copy-Item $source $target -Recurse -Force
    }
  }
  & $Ctl -root $Root -action start-server | Out-Null
  throw "升级失败，已回滚到 $backup"
}

Get-ChildItem (Join-Path $Root "runtime\backups") -Directory -Filter "hot-*" |
  Sort-Object Name | Select-Object -SkipLast $Keep | ForEach-Object { Remove-Item $_.FullName -Recurse -Force }

Write-Output "升级完成，备份保留在 $backup"
