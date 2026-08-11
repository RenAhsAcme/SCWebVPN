# 生产架构

## 目标

系统为单用户提供浏览器内的 LuCI、HTTP(S) 管理页和 Guacamole 入口。登录、
目录与 SDP/ICE 信令通过受信 CDN/接入层和 Nginx；业务流量只走浏览器与 OpenWrt
Agent 的 WebRTC DataChannel。没有 TURN，也没有以 HTTPS/WebSocket 冒充
“临时中继”的回退路径。

## 组件

### 接入层与 Nginx

- `vpn.example.com`：登录、服务目录、诊断和持久服务入口。
- `*.vpn.example.com`：每个临时会话使用 128 位随机、不可枚举的子域。
- 域名全部保持接入代理；源站只接受已验证的回源地址或客户端证书，未知 Host
  在最前置的默认 server 中拒绝。
- Nginx 清除客户端提供的身份头，再以 `auth_request` 调用 Authelia；只有
  2xx 进入 Controller。Nginx 官方模块将 401/403 直接视为拒绝，行为依据见
  [ngx_http_auth_request_module](https://nginx.org/en/docs/http/ngx_http_auth_request_module.html)。

### Authelia 4.39.20

- 密码 + TOTP 二次认证；用户、TOTP、会话与恢复码不由 Controller 管理。
- SQLite 只适用于本单节点、单用户部署；数据库和密钥均置于独立权限目录。
- 会话 Cookie 为 `Secure`、`HttpOnly`，采用受控的父域作用域以覆盖临时子域；
  业务目标站 Cookie 不进入该父域 Cookie jar。
- Nginx 只在 Authelia `auth_request` 成功后把当前会话 Cookie 送入环回 Controller；Controller
  立即转换为不可逆 HMAC 会话摘要并清除原值。P2P 能力值和临时映射绑定此摘要，而审计使用
  独立的稳定用户摘要，另一条同用户名登录会话不能复用该临时映射。
- 集成遵循 [Authelia Nginx 指南](https://www.authelia.com/integration/proxies/nginx/)、
  [SQLite 配置](https://www.authelia.com/configuration/storage/sqlite/) 与
  [TOTP 配置](https://www.authelia.com/configuration/second-factor/time-based-one-time-password/)。

### Controller

Go 单二进制，仅监听 `127.0.0.1`。它负责：

- 读取 Nginx 已验证且重新注入的用户身份和当前 Authelia 会话摘要；
- 服务目录、临时子域、会话租约与撤销；
- Agent 一次性绑定、Ed25519 挑战应答、短期信令交换；
- SQLite WAL、显式 schema migration 与不含目标地址的审计元数据；
- 只交付 `stun:` 配置，并对 SDP/ICE 执行 relay/私有数据检查。

Controller 不建立到内网服务的连接，也不接受 Wisp 业务帧。

### OpenWrt Agent

Go/Pion WebRTC 单二进制，以 procd 服务运行，私钥 `0600`。它：

- 仅主动访问 Controller 的 HTTPS 信令端点；不开放 WAN 监听端口；
- 验证浏览器会话、运行 Agent 本地 allowlist，并建立 STUN-only WebRTC；
- 为 Wisp v2、HTTP(S) 语义适配和 Guacamole 提供相互隔离的 DataChannel；
- 在本机执行目标 DNS/IP/端口、重绑定和证书策略，是最终授权点；
- 不修改路由、DHCP、DNS、NAT、防火墙或已有 Guacamole/RustDesk 配置。

### Edge 浏览器运行时

- 支持当前 Chromium Edge，使用浏览器原生 WebRTC 与标准沙箱。
- 静态壳、Service Worker、Scramjet/Epoxy 兼容层由版本化构建产物提供。
- 持久服务有稳定子域；临时服务使用随机子域，隔离 Cache Storage、
  IndexedDB、Service Worker 与目标站 Cookie jar。
- 临时目标 URL 不进入控制面、信令或地址栏；它只在随机子域页面内存中存在，并在 P2P
  DataChannel 鉴权完成后交给 Agent 做最终策略判定。
- 浏览器不获得 OpenWrt 私钥、内网私有 CA 文件或证书例外配置。

## 数据面

固定创建四类有界通道，避免大文件占满交互流：

1. `control`：心跳、租约、策略结果和诊断；
2. `interactive`：HTML/API、SSE 与低延迟请求；
3. `bulk`：上传下载，使用独立背压和窗口；
4. `guacamole`：Guacamole WebSocket/指令流。

Wisp v2 负责通用 TCP 语义。HTTPS 私有 CA 与例外只保存在 OpenWrt；浏览器兼容
层对合成目标保留 HTTPS Origin，但在加密 DataChannel 内向 Agent 发送同端口明文
HTTP/WebSocket 语义。Agent 用 Go 标准库 TLS 和本地 CA/SPKI 策略包装固定上游
socket。因此无需自研 HTTP 协议或 TLS MITM，WebSocket 升级仍沿用成熟实现。

## 身份与生命周期

- Agent 首次绑定使用一次性、短时、只显示一次的绑定码；成功后立即消费。
- Agent 以 Ed25519 对 `version | agent_id | nonce | issued_at | body_hash`
  签名；nonce 只能使用一次，时钟窗口有限。
- 浏览器会话票据是不可读、短时、绑定用户/服务/Agent 的随机能力值；数据库只
  存哈希。空闲 30 分钟、绝对 2 小时，到期或注销即撤销。
- ICE restart 复用同一 PeerConnection，最多两次；失败后关闭全部通道并要求
  用户显式重连，不自动重放业务请求。

## 失败语义

- 无直连：显示“当前网络无法建立直连；本系统没有中继回退”，不切换公网通道。
- Agent 离线：目录可见但服务不可进入，已有会话立即失效。
- Controller/Authelia 离线：既有 P2P 租约最多维持到当前短期到期，不延长。
- 策略拒绝：返回稳定错误码，不向浏览器透露额外内网拓扑。
