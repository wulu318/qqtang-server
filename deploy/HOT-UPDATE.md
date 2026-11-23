# 热更与备份

## 自动数据库备份

服务端每次通过启动器启动前，会把 `runtime/data/qqtang.sqlite` 复制到：

```
runtime/backups/qqtang-YYYYMMDD-HHMMSS.sqlite
```

只保留最近 10 份，旧的自动清理。备份失败会写 `launcher-ui.log`，但不会阻止启动。

## 热更脚本

`deploy/hot-update.ps1` 做三件事：备份 → 停服 → 覆盖 → 启动，失败自动回滚。

```powershell
.\deploy\hot-update.ps1 -Root "D:\QQTang-Local" -Package "D:\QQTang-Local-1.3.5.zip"
```

可选参数：

1. `-Keep 5`：保留最近 5 份热更备份，默认 5。
2. `-Ctl <path>`：手动指定 `qqt-launcher-ctl.exe`。

回滚内容：`runtime/data`、`runtime/bin`、`configs`、`runtime/client-patched/config`。

## 手动回滚

```powershell
& .\runtime\bin\qqt-launcher-ctl.exe -root . -action stop-server
Copy-Item .\runtime\backups\hot-20260101-120000\runtime\data .\runtime\data -Recurse -Force
& .\runtime\bin\qqt-launcher-ctl.exe -root . -action start-server
```

## 注意

1. 热更只保证服务端不丢存档，不保证在线玩家不掉线；正式服建议在低峰期执行。
2. 客户端热更需要重新分发 `QQTang-Local.zip`，启动器目前不会自动下载新客户端。
