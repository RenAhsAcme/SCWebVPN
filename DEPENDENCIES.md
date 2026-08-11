# 锁定依赖

版本基线核验日期：2026-08-10。容器必须锁镜像 digest，下载的二进制必须锁 SHA-256，
不得使用 `latest`。

| 组件 | 固定基线 | 用途 |
| --- | --- | --- |
| Authelia | `4.39.20` | 密码、TOTP、SQLite |
| Pion WebRTC | `github.com/pion/webrtc/v4 v4.2.18` | ICE、DTLS、SCTP |
| SQLite | `modernc.org/sqlite v1.56.0` | CGo-free Controller 数据库 |
| Wisp | protocol `2.1` | DataChannel TCP 多路语义 |
| Epoxy TLS | `2.1.19-1`, commit `93d5a726894b2f16bad54c4a3801446cbbd22d26` | 浏览器 TLS |
| Scramjet | `2.0.67-alpha.2` | 浏览器页面兼容层 |
| Scramjet Controller | `0.0.14` | 浏览器控制器 |
| proxy-transports | `1.0.2` | transport 接口 |
| TypeScript | `5.9.3` | 类型与构建检查 |

浏览器依赖由 `web/bun.lock` 解析。干净构建在 `web/` 执行
`bun install --frozen-lockfile`；构建器只使用同目录 `node_modules` 和仓库内固定的
`web/vendor/epoxy`，不依赖原项目或机器上的共享目录。

## 引入规则

- Go 依赖只由 `go.mod/go.sum` 决定；Controller 保持 CGo-free；
- 发布不包含 `node_modules`，但 AGPL 组件的精确源码、补丁、构建脚本和许可证必须同行；
- Scramjet 两个上游标签锁定提交 `c26bfc6d7f7c7f4dac52ce182a2ceab90e851823`，来源记录在
  `third_party/scramjet/SOURCE.lock`；
- 每次更新记录 release URL、校验值、兼容结果和回滚版本；
- Pion 模块图包含 TURN 库不等于运行时启用 TURN；配置解析和候选检查仍明确拒绝 relay。

## 官方依据

- [Authelia 4.39.20](https://github.com/authelia/authelia/releases/tag/v4.39.20)
- [Pion WebRTC 4.2.18](https://github.com/pion/webrtc/releases/tag/v4.2.18)
- [Wisp 2.1](https://github.com/MercuryWorkshop/wisp-protocol/blob/v2/protocol.md)
