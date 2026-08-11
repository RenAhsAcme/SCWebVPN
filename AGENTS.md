# SCWebVPN 协作约定

- 当前基线稳定可用且已完成验收；后续增强不改变这一状态。
- 默认做最小、可审计的改动，保留严格 P2P、STUN-only 和失败关闭语义。
- Controller 只掌握服务元数据；目标地址、端口、CIDR、CA 与例外留在 Agent。
- 不得提交秘密、生产地址、设备 ID、公钥、备份路径、SDP 或 candidate。
- OpenWrt 安装和回滚不得修改默认路由、DNS、DHCP、NAT 或防火墙。
- Guacamole RDP、RustDesk 物理桌面和独立文件通道是三种能力，不得互相冒充。
- RustDesk Canvas 更新必须保留固定上游提交、补丁、完整对应源码和逐文件哈希。
- 浏览器验证使用已有 Chromium 内核浏览器（优先 Edge），不要求安装 Chrome。
- 发布前运行 Go、Web、许可证、对应源码、秘密扫描和真实 P2P/回滚门禁。
- 后续增强写入路线图，不把可选改进表述为当前缺陷或发布阻断。
