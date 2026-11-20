# 致谢与来源说明

本项目基于以下开源项目修改：

- **QQTang（QQ堂怀旧版）**
  https://github.com/kuuhaku1314/qqtang
  作者：kuuhaku1314 及贡献者
  许可：Apache License 2.0

原项目的服务端协议实现、地图/道具数据、启动器与客户端补丁方案均来自上述仓库。本仓库仅在其基础上做增量修改，未重新实现核心协议。

## 本仓库的修改

1. 登录防暴力破解：按来源 IP 限制失败次数（`internal/server/probe/auth_ratelimit.go`）。
2. 聊天与建房限速（`internal/server/probe/action_limits.go`）。
3. TCP `SetNoDelay`，修复小包被 Nagle 延迟的问题（`internal/server/probe/server.go`）。
4. 端口集中配置：`configs/ports.json` 同时驱动服务端监听、客户端 INI、登录辅助与 GM URL。
5. 启动器支持从 HTTP(S) 拉取服务器地址（`-config-url`），便于换机。
6. WebSocket 网关（`cmd/qqt-ws-gateway`）与浏览器协议实现（`web-client/`）。
7. 压测与冒烟工具（`cmd/qqt-loadtest`、`cmd/qqt-smoke`）。

## 版权声明

- 服务端与工具代码遵循 Apache License 2.0，保留原项目版权与 NOTICE。
- 《QQ堂》客户端、美术资源、商标归腾讯公司所有，本仓库不包含也不分发原始客户端。
- 使用本项目产生的任何风险由使用者自行承担，请勿用于商业运营。
