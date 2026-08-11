# STUN-only P2P 限制

## 明确不支持

- UDP 被组织网络、运营商或本机策略完全阻断；
- 双方均处于无法打洞的对称 NAT/严格 CGNAT；
- 只允许经企业 HTTP 代理出站的网络；
- 需要 TURN/TCP、TURN/TLS 或公网业务 relay 才能连通的环境；
- 浏览器/系统策略禁用 WebRTC DataChannel 或隐藏全部可用 candidate。

系统不会在这些场景静默降级。UI 应明确说明“未建立直连，且没有中继回退”，
并保留现有 Guacamole/RustDesk 独立访问方式，不把它们伪装成 WebVPN 数据面。

## 可能受影响

- NAT 映射老化或 Wi-Fi/蜂窝切换会触发 ICE restart；最多两次。
- srflx 可达性依赖 NAT hairpin、端口映射保持时间与防火墙 UDP 状态。
- 大量并发 SCTP 流仍共享一个 PeerConnection；固定通道拆分只能降低而非消除
  拥塞耦合。
- Service Worker、CSP、跨源隔离、WebAuthn、下载行为和复杂 WebSocket 应用
  需要逐站兼容验证，不能宣称任意网站透明可用。

## 最终网络矩阵

按当前执行顺序，矩阵放在产品实现之后、上线之前：

1. 家庭宽带 ↔ OpenWrt；
2. 蜂窝热点 ↔ OpenWrt；
3. 至少一个不同运营商/不同 NAT 类型的外部网络；
4. 明确 UDP 阻断的负向样本；
5. Agent 丢失、网络切换与 NAT 映射变化。

每个正向样本记录 candidate 类型、选中 pair 类型、建立时间、重启次数与功能结果，
但不保存 IP。任一结果出现 relay 或公网服务器业务字节即为安全失败；负向样本
若错误地进入“已连接”同样失败。
