# Wisp v2 over WebRTC 生产绑定

上游语义：[Wisp protocol 2.1](https://github.com/MercuryWorkshop/wisp-protocol/blob/v2/protocol.md)。
本文件只定义承载，不改变 Wisp packet 编码。

## 通道

| label                 | 用途                       | ordered | 单消息上限 |
| --------------------- | -------------------------- | ------- | ---------- |
| `webvpn-control-v1`   | 心跳、租约、诊断、策略结果 | true    | 512 bytes  |
| `wisp-interactive-v2` | HTTP/API/SSE 与小响应      | true    | 64 KiB     |
| `wisp-bulk-v2`        | 上传下载                   | true    | 64 KiB     |
| `guacamole-v1`        | Guacamole 指令/WebSocket   | true    | 64 KiB     |

均使用可靠 SCTP，不设置 `maxRetransmits`/`maxPacketLifeTime`。一个 DataChannel
message 对应一个完整 Wisp packet；不会合并 packet，也不引入私有分片头。TCP
字节流由多个 DATA packet 自然重组。

## Wisp 语义

- packet 为 `type:uint8 + stream_id:uint32 little-endian + payload`；stream 0
  只用于 INFO/CONTINUE。
- 客户端 stream ID 使用 Web Crypto 随机非零 uint32，并在当前通道内查重。
- 只支持 TCP CONNECT (`0x01`)；未声明 UDP 扩展。
- 每个业务通道最多 16 个并发流，每流 64-packet FIFO；容量耗尽以 `0x49`
  关闭，不继续分配内存。
- 初始 allowance 为 64，消费 32 个 packet 后发送当前剩余容量的 CONTINUE。
- 非法 CONNECT 为 `0x41`，不可达为 `0x42`，策略拒绝为 `0x48`，主动结束为
  `0x02`，网络错误为 `0x03`。

## 背压与公平性

- Agent 从 TCP 每次读取不超过 16 KiB。
- DataChannel `bufferedAmount` 高于 4 MiB 时暂停，回落到 2 MiB 后恢复；浏览器
  写入侧恢复阈值为 1 MiB。
- bulk 和 interactive 使用独立 DataChannel 与字节/并发配额；control 不承载正文。
- 诊断只累计 byte count 和至多 800 bytes 的显式调试预览；生产默认不保留预览。

## 授权与恢复

- DataChannel 建立后，浏览器先发送 `webvpn-auth-v1:<capability>`；Agent 验证该
  能力的哈希已绑定用户、Agent、服务和有效期，并对 label 做唯一性与类型检查。
- Agent 返回 `webvpn-auth-ok-v1` 后，浏览器先挂载 Wisp 二进制消息监听器，再发送
  `webvpn-auth-ready-v1`。Agent 只在收到 ready 后启动 Wisp，避免首个 INFO packet
  在浏览器 transport 就绪前丢失。能力值只存在浏览器内存，不写日志或数据库。
- Agent 对每个 CONNECT 重新运行本地 allowlist，不能信任浏览器给出的 host/port。
- HTTPS/WSS 的合成目标在浏览器侧仅对 Epoxy 降为同端口明文语义；Agent 随后用
  Go 标准库 TLS、OpenWrt 本地私有 CA/SPKI 策略包装固定上游连接。私有 CA 和
  证书例外不离开 OpenWrt，也不会进入 Controller、SDP 或前端产物。
- ICE restart 复用同一 PeerConnection 和现有 DataChannel；成功后旧 Wisp stream
  可继续，但上层必须处理中断。若 PeerConnection 关闭，则所有 stream 取消。
- 新 PeerConnection 不恢复旧 stream，不自动重放 POST、上传、键鼠、SSH 或文件写入。

## 与 Wisp-over-WebSocket 的差异

没有 HTTP Upgrade、公共 Wisp URL、WSS endpoint 或 Controller 数据通道。
DataChannel message 取代 WebSocket binary message，公网服务器无法访问其中 packet。
任何新增服务器中继实现都违反本架构。
