# SCWebVPN 内网远程访问集成

该集成把三类互补服务接入 SCWebVPN 的 P2P 数据面。OpenWrt 承载协议转换、RustDesk
Server OSS 和回环入口；Windows 仅开放给 OpenWrt 的 RDP、RustDesk 被控端与受限文件通道。

| 入口 | 实现 | 适用场景 | 文件能力 |
| --- | --- | --- | --- |
| `/guacamole/` | Guacamole 1.6.0 → Windows RDP | 独立 RDP 会话、剪贴板、声音 | RDPDR `Remote Files` |
| `/web/` | 固定源码的 RustDesk Canvas Web | 保留物理桌面与原分辨率 | 打开独立 `/files/` |
| `/files/` | OpenWrt Nginx → Windows File Browser | 高速交换目录 | 上传与下载 |

RustDesk Web 核心当前不实现文件 API，因此不能把 Canvas 工具栏当作原生文件传输。
门户明确跳转到独立 `/files/`。Windows 与 RustDesk 密码始终由用户交互输入，不写进
Controller、页面、仓库或 OpenWrt 配置。

## 网络与运行边界

- Nginx、Guacamole、`guacd` 和 PostgreSQL 只绑定 OpenWrt 环回地址；
- RustDesk 原生端口只允许被控 PC 的 LAN 地址进入；
- File Browser 只绑定 PC 的指定 LAN 地址，Windows 防火墙仅允许 OpenWrt；
- 浏览器业务数据只走 WebRTC/DTLS P2P，不提供公网 WSS 或 TURN 回退；
- Guacamole 在 OpenWrt 上终止 RDP，性能受路由器 CPU 影响；RustDesk 视频由 Windows
  捕获编码，OpenWrt 只处理信令与加密 WebSocket 转发。

定制 OpenWrt 内核可能没有匹配的 `kmod-veth`。随附安装器使用固定哈希的 Docker 静态
运行时、host network、`bridge: none` 和 `iptables: false`，避免创建 `docker0` 或改写
网关防火墙。不要跨内核 ABI 强制安装官方 kmod。

## 目录

- `compose.yaml`：Guacamole、PostgreSQL 和 RustDesk Server 的固定版本；
- `openwrt/`：Docker、OpenRC 与 Nginx 配置；
- `portal/`：支持深色模式的入口页；
- `rustdesk-web-build/`：固定上游提交、补丁、锁文件和来源记录；
- `scripts/`：安装、构建、状态、备份与管理脚本；
- `windows/`：Windows RDP、RustDesk 与 File Browser 暂存脚本；
- `sql/bootstrap.sql`：Guacamole 连接和 RDPDR 参数。

运行时数据位于 `/opt/lan-remote-access/{data,secrets,docker}`，不得提交。Canvas Web 运行时
由 `scripts/build-rustdesk-web.ps1` 生成；部署脚本从本机 RustDesk Server 公钥渲染
`index.html.template`，设备 ID 由安装时参数写入，二者都不属于源码。

## 部署

1. 按主 README 构建 Canvas Web，并核对逐文件清单；
2. 在 OpenWrt 运行 `scripts/install-openwrt.sh`，确认它没有改写现有网关网络；
3. 在 Windows 管理员 PowerShell 运行：

   ```powershell
   .\windows\stage-webvpn-windows.ps1 -Address '<PC_LAN_ADDRESS>'
   ```

   脚本备份相关状态，不修改现有 RustDesk 固定密码，也不依赖或管理 Tailscale；
4. 把 PC LAN 地址与 RustDesk 设备 ID 写入 OpenWrt 集成配置，启动服务；
5. 将 OpenWrt 回环入口注册为 Controller 服务，完成跨设备验收。

File Browser 固定为 2.63.23，以 `LOCAL SERVICE` 运行，关闭命令执行和外部链接，只开放
一个交换目录。它是隔离的可替换组件；出现安全问题时应替换文件服务，不应扩大监听或
授予管理权限。

## 管理与备份

```sh
lan-remote ps
lan-remote logs -f guacamole
lan-remote logs -f hbbs hbbr
/opt/lan-remote-access/scripts/status.sh
/opt/lan-remote-access/scripts/backup.sh /root
```

备份包含数据库一致性导出、RustDesk 身份和运行资源，权限为 `0600`。恢复应在隔离路径
实际演练，不能只确认压缩包存在。

## 验收

从另一台已完成 MFA 且建立 SCWebVPN P2P 的设备分别验证：

- RDP：原机锁屏、键鼠、声音、剪贴板以及 `Remote Files` 上传下载；
- RustDesk：物理桌面继续显示、原分辨率、键鼠和声音；
- 高速文件：`/files/` 无二次登录，并能在专用目录上传下载；
- OpenWrt：默认路由、DNS、DHCP、NAT、防火墙与既有服务基线不变。

不要在被控 Windows 本机做 RDP 自连接验收；它会锁定当前会话。

## 更新规则

版本集中在 `compose.yaml` 和安装脚本。更新前阅读上游发布说明，修改版本、摘要和 SHA-256，
重建 Canvas Web，复核对应源码包，再执行状态检查和三类真实会话测试。静态运行时不会随
OpenWrt 包管理器自动更新，必须显式升级并重验“无网桥、无防火墙改写”。
