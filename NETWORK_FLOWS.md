# 网络流

## 端点与方向

| 发起方        | 接收方              | 用途                       | 是否含业务流量       |
| ------------- | ------------------- | -------------------------- | -------------------- |
| Edge          | 接入层 → Nginx      | 登录、目录、短期信令       | 否                   |
| Nginx         | Authelia 环回端口   | `auth_request`             | 否                   |
| Nginx         | Controller 环回端口 | API/信令                   | 否                   |
| OpenWrt Agent | 接入层 → Controller | 认证、轮询/信令            | 否                   |
| Edge          | STUN                | server-reflexive candidate | 否                   |
| OpenWrt Agent | STUN                | server-reflexive candidate | 否                   |
| Edge          | OpenWrt Agent       | DTLS/SCTP DataChannel      | **是，唯一业务路径** |
| OpenWrt Agent | allowlist 目标      | TCP/HTTP(S)/Guacamole      | **是**               |

没有公网服务器到 OpenWrt 内网目标的连接，也没有 Edge → Controller → Agent
的业务转发链。

## 建连顺序

1. Edge 访问 `vpn.example.com`，通过 Authelia 密码与 TOTP。
2. Nginx `auth_request` 成功后，Controller 返回服务目录和一次性连接意图。
3. Agent 主动轮询并用 Ed25519 签名领取意图。
4. Edge 和 Agent 经 Controller 交换受尺寸限制的 SDP/ICE；Controller 不持久化。
5. 双方只向 STUN 请求候选，拒绝 relay candidate。
6. DTLS 建立后，双方核对选中 candidate pair；非 host/srflx/prflx 即失败。
7. 持久服务由 Agent 按服务 ID 解析本地目标；临时服务的目标只在直连 DataChannel 鉴权后
   发送，Controller 和 SDP 均不可见，Agent 固定已验证的解析结果后才允许 Wisp 建流。
8. 关闭、租约到期、注销或策略变化时，双方销毁通道和临时能力值。

## ICE restart

- `disconnected` 进入短暂观察窗口，随后在同一 PeerConnection 上 restart ICE。
- 新 offer 带 128 位 restart ID，Agent 只接受当前会话的单调链。
- 最多两次；每次仍拒绝 relay，且不重放未确认的 POST/上传/键盘输入。
- 失败后用户需显式重连；不存在 TURN 或 HTTPS relay 回退。

## 地址与日志

Controller 只保留：时间、匿名化用户 ID、Agent ID、服务 ID、结果码、候选类型、
延迟桶和字节数桶。以下数据不进入日志或数据库：完整 SDP、candidate 地址、内网
目标地址、源站 IP、Cookie、Authorization、请求/响应正文。
