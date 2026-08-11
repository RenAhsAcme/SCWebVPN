# 威胁模型

## 资产

- OpenWrt 网关可用性与路由、DHCP、DNS、NAT、防火墙状态；
- 内网服务、Guacamole 凭据、浏览器会话与 Agent 私钥；
- 源站真实地址、内网地址、ICE candidate 和访问时间元数据；
- 现有 Guacamole/RustDesk 回退能力及个人数据。

## 信任边界

| 边界               | 信任                              | 不信任                             |
| ------------------ | --------------------------------- | ---------------------------------- |
| 浏览器 ↔ 接入层/Nginx | 边缘 TLS、已核验回源或 mTLS    | 客户端身份头、Host、Origin、请求体 |
| Nginx ↔ Authelia   | 环回连接、固定响应头              | 任意外部同名头                     |
| Nginx ↔ Controller | 环回连接、重新生成的身份上下文    | 直接公网访问                       |
| Controller ↔ Agent | 已绑定 Ed25519 公钥、一次性 nonce | IP 地址本身、重放签名、未知 Agent  |
| 浏览器 ↔ Agent     | 短期会话能力值、DTLS              | SDP 中的声明、浏览器提供的目标地址 |
| Agent ↔ 内网       | Agent 本地 allowlist 与本地 CA    | DNS 回答、重定向目标、目标响应头   |

接入层、Nginx 和 Controller 被信任为认证与信令基础设施，但不被授权查看或转发
业务字节。Agent 被信任为策略执行点；浏览器与目标站均按潜在恶意输入处理。

## 主要威胁与控制

### 认证绕过

- Nginx 在认证前删除所有 `Remote-User`、`Remote-Groups`、`Remote-Email`、
  `Authorization` 等可伪造身份头，只使用 Authelia 子请求结果；Controller 对已验证的
  Authelia 会话 Cookie 立即做带域分离的 HMAC，只持久化会话摘要，并在进入业务处理器前清除原值。
- Controller 拒绝缺少内部随机认证头或来自非环回连接的请求。
- 密码、TOTP、恢复码与会话密钥使用独立 secrets 文件，不写仓库和命令行。

### Agent 冒充与重放

- 首次绑定码至少 128 位熵、短时有效、单次消费；错误次数限流。
- 后续请求使用 Ed25519 签名、请求体哈希、服务端 nonce 与严格时间窗。
- 公钥更换必须由已认证用户显式批准；旧密钥立即撤销并留下审计事件。

### SSRF 与横向扫描

- Controller 从不按浏览器给出的地址拨号。
- Agent 只接受服务 ID；目标在 Agent 本地配置中解析为固定协议、地址范围和端口。
- 每次 DNS 解析和每次重定向都重新校验，拒绝 link-local、multicast、未授权网段、
  非法 IPv4-mapped IPv6 与 DNS rebinding。
- Controller 创建临时服务时只接收 Agent、协议与随机映射，不接收目标 URL。用户输入的临时
  URL 只保留在该随机子域页面内存，经已认证 DataChannel 交给 Agent；Agent 再按显式网段、
  端口、DNS 全结果、固定连接 IP、非本机地址和严格 TLS 策略判定。
- 按用户、会话、服务限制并发流、连接速率、字节率和累计字节。

### 公网中继绕过

- 依赖配置只接受 `stun:`；`turn:`/`turns:` 在解析时即为致命错误。
- 浏览器和 Agent 均拒绝 `relay` candidate；选中的 pair 若为 relay，立即关闭。
- Nginx 不提供 WebSocket/Wisp 数据回退端点；Controller 对大信令体设小上限。
- 运行期记录“候选类型计数”和连接结果，但不记录 candidate 地址或 SDP。

### 源站与内网地址泄露

- DNS 全部保持受信代理，源站防火墙只接受核验过的回源范围或 mTLS 及受控管理路径。
- 应用错误、日志、指标、HTML 与 JavaScript 不包含源站或内网地址。
- ICE SDP 只对已完成 MFA 的当前会话短时可见；日志对 SDP 全量丢弃。
- mDNS host candidate 保持浏览器默认；诊断仅展示候选类型与匿名化网络标签。

### Web 代理隔离破坏

- 每个临时会话使用随机子域和独立存储命名空间；Service Worker scope 不能越界。
- 目标 `Set-Cookie` 由代理运行时的隔离 cookie jar 消费，不写 WebVPN 父域。
- 删除 hop-by-hop 与认证相关响应头；CSP、Location、Origin 与 WebSocket URL
  由经过测试的兼容层重写。
- HTML 解析器、WASM 和消息解码器均接受不可信输入，设尺寸、深度和时间上限。

### 网关可用性

- Agent 使用低优先级、内存/文件描述符/进程限制与连接配额；OOM 不得拖垮网关。
- 安装、升级与卸载不写网络主配置；procd 失败只影响 WebVPN Agent。
- 大文件使用流式背压和独立 bulk 通道；禁止把完整正文写磁盘或内存。
- 变更前后必须验证 LAN/WAN、DNS、DHCP、NAT 与现有服务健康。

## 不接受的风险

- 以“可用性”为由加入 TURN、服务器业务 relay 或关闭严格证书校验；
- 将 Agent 私钥、Authelia secrets、TOTP seed、Guacamole 凭据提交仓库；
- 为方便调试记录 SDP、candidate 地址、Authorization、Cookie 或目标正文；
- 用 WebVPN 变更 OpenWrt 主防火墙、默认路由、DHCP/DNS 或替换现有回退服务。
