# 项目交接记忆

本文件是从原工作目录迁移的脱敏上下文，用于后续会话快速恢复；不含密码、生产域名、
公网/内网生产地址、设备 ID、RustDesk 公钥、证书、备份路径或会话材料。

## 当前结论

- SCWebVPN 当前稳定运行并已完成验收；旧的“待验收”状态作废。
- 外部网络 P2P 已做定性测试，成功率表现良好；不能据此保证所有 NAT 环境。
- 数据面严格为 WebRTC/DTLS P2P，只有 STUN，无 TURN 或公网业务中继。
- OpenWrt Agent 已绑定且网关业务保持正常；安装和版本切换不应修改网络配置。
- RustDesk 物理桌面、Guacamole RDP 与高速文件入口均按独立通道设计。

## 已确认架构决定

- Authelia 负责密码 + TOTP；Nginx 清除客户端伪造身份头后注入本地可信身份。
- Controller 仅监听环回，保存 Agent、服务名称、类型和策略引用，不保存真实目标。
- Agent 持有目标主机、端口、CIDR、私有 CA、SPKI 例外和 Ed25519 私钥。
- 不能打洞时明确失败；不得为了“可用率”暗中加入 TURN、TCP 隧道或 WebSocket 中继。
- Guacamole 终止 RDP，会消耗 OpenWrt CPU；RustDesk 视频由 Windows 编码，OpenWrt 不转码。
- RustDesk Web 没有文件 API，工具栏的文件入口转到独立 File Browser 通道。
- File Browser 使用低权限账户、单一目录、关闭命令执行，仅接受 OpenWrt 来源。

## 固定来源

- Guacamole `1.6.0`；
- RustDesk Server OSS `1.1.15`（镜像 digest 见 compose）；
- RustDesk Canvas Web fork commit `525b5e561faf824850c71500adf463e4e0a504d4`；
- File Browser `2.63.23`；
- Authelia、Pion、Epoxy、Scramjet 等见 `DEPENDENCIES.md`。

## 操作偏好与边界

- 浏览器测试使用已有 Edge，不在电脑上安装 Chrome。
- 不保存交互密码；需要凭据时只在用户真实 TTY 隐藏输入。
- Windows 侧其他网络软件不是项目依赖，部署脚本不得擅自安装、停用或删除。
- OpenWrt 以网关网络业务为最高保护边界；Nginx 和应用允许可恢复的短暂中断。
- 任何发布都要区分“本地构建通过”和“实际生产状态”，并保留可恢复备份。

## 后续工作方式

从本目录开始新对话时，先读 `AGENTS.md`、本文件、`README.md`、`THREAT_MODEL.md` 和
相关模块实际消费者。当前没有发布阻断项；新增工作应按具体需求建立小范围计划和回归证据。
