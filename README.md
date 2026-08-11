# SCWebVPN

SCWebVPN 是一个自托管、浏览器零安装的内网访问网关。公网服务器只负责身份认证、授权与
WebRTC 信令；业务字节通过浏览器和 OpenWrt Agent 之间的 WebRTC/DTLS 点对点通道传输。
项目严格使用 STUN，不提供 TURN 或公网中继回退。

当前代码基线已在真实环境稳定运行，并完成 Edge、OpenWrt、RDP、RustDesk、文件通道和
外部网络 P2P 的定性验收。后续改进属于可选演进，不代表当前版本处于未完成状态。

## 能力

- Authelia 密码 + TOTP 双重认证；
- Ed25519 绑定的 OpenWrt Agent 与短期授权；
- 固定服务和会话临时服务，本地 CIDR/端口/TLS allowlist；
- Wisp 2.1 over WebRTC DataChannel；
- 浏览器内访问 HTTPS、WebSocket、SSE、下载和上传；
- 可选的 Guacamole RDP、RustDesk 物理桌面与 File Browser 文件通道集成；
- 固定版本、完整对应源码、许可证清单、构建脚本和回滚脚本。

## 明确限制

- 没有 TURN：对称 NAT、UDP 阻断或企业网络策略可能导致连接失败；
- 公网控制面仍会看到登录元数据和信令，但不得承载业务正文；
- RustDesk Canvas Web 当前没有原生文件 API，文件入口由独立 `/files/` 提供；
- Guacamole 的 RDP 转换消耗 OpenWrt CPU；RustDesk 路径更适合保留物理桌面的视频接管；
- 示例不是一键生产配置。域名、网段、证书、接入层和目标服务必须按现场审计。

详见 [架构](ARCHITECTURE.md)、[威胁模型](THREAT_MODEL.md)、
[P2P 限制](P2P_LIMITATIONS.md) 与 [网络流](NETWORK_FLOWS.md)。

## 目录

```text
cmd/                         Controller 与 Agent 入口
internal/                    协议、策略、数据库与控制面实现
web/                         浏览器客户端及固定 Epoxy 源码
config/                      无 secret 的配置模板
packaging/                   systemd 与 OpenWrt 安装包装
integrations/lan-remote-access/
                             Guacamole、RustDesk、File Browser 集成
scripts/                     构建、源码分发与发布打包
third_party/                 上游来源锁与许可证
docs/PROJECT_MEMORY.md       脱敏的项目交接记忆
```

## 构建环境

- Go `1.26.0`；
- Bun（由 `web/bun.lock` 固定依赖图）；
- PowerShell 7+；
- Linux/amd64 源站与 OpenWrt Agent 是现有脚本的默认目标；
- 浏览器测试使用已有的 Chromium 内核浏览器，例如 Microsoft Edge，无需安装 Chrome。

第三方版本见 [DEPENDENCIES.md](DEPENDENCIES.md)。若使用其他 CPU 架构，请显式修改
`GOARCH`，并为所有下载物重新固定官方 SHA-256。

## 从干净仓库构建

```powershell
go test ./...
go vet ./...
.\scripts\build-controller.ps1
.\scripts\build-agent.ps1

Push-Location web
bun install --frozen-lockfile
bun test src
bun run check
bun run build
Pop-Location
```

`web/scripts/build.ts` 只消费仓库内 `web/vendor/epoxy` 与本地 lockfile 安装结果，不依赖
原始站点仓库。Epoxy 补丁与完整源码随仓库分发，以满足 AGPL 对应源码要求。

构建 RustDesk Canvas Web：

```powershell
.\integrations\lan-remote-access\scripts\build-rustdesk-web.ps1
```

该脚本下载固定提交与已锁 SHA-256 的解码器，输出不含设备 ID 和服务器公钥的
`index.html.template` 及逐文件清单。部署时才从本机配置渲染秘密或设备标识。

## 配置准备

1. 把 `config/*.example*` 复制到主机受限目录；
2. 全局替换 `example.com`，并替换全部 `REPLACE_*`；
3. 为 Controller 生成独立随机内部认证 secret，权限设为 `0640 root:webvpn`；
4. 为 Authelia 分别生成 session、storage encryption、identity validation secrets；
5. 在真实 TTY 使用 `scripts/set-authelia-owner-password.sh` 设置 Argon2id 密码哈希；
6. 准备覆盖 `vpn`、`auth.vpn` 与 `*.vpn` 的证书；
7. 若使用 CDN/接入代理，只信任已核验的回源地址和客户端 CA，不直接信任请求头；
8. 在 OpenWrt 人工验证目标网段和端口，不让安装器修改网关网络。

任何密码、TOTP seed、恢复码、私钥、生产 IP、设备 ID、RustDesk 公钥或数据库都不应提交。

## 源站部署

先生成完整对应源码归档，再生成运行发布包：

```powershell
.\scripts\fetch-third-party-source.ps1
.\scripts\package-third-party-source.ps1 -Version '0.1.0'
.\scripts\package-corresponding-source.ps1 `
  -Version '0.1.0' `
  -ThirdPartyArchive '..\.tmp-scwebvpn-source\0.1.0-third-party-source.tar.gz'
.\scripts\package-source-release.ps1 `
  -Version '0.1.0' `
  -CorrespondingSourceArchive '..\.tmp-scwebvpn-source\0.1.0-corresponding-source.tar.gz' `
  -AutheliaBinary '<已校验的 Authelia Linux 二进制>'
```

将发布包与 SHA-256 送到源站，在 Linux 上执行：

```sh
packaging/systemd/install-source-staging.sh 0.1.0 ./0.1.0.tar.gz <SHA256>
```

脚本只暂存不可变版本和最小权限目录，不自动启动或切流。写入 secrets 后，验证
Controller schema、Authelia 配置和 systemd 单元；再安装 Nginx 示例、执行 `nginx -t`
并 reload。完整顺序见 [DEPLOYMENT.md](DEPLOYMENT.md)。

## OpenWrt Agent

1. 记录网关网络基线；
2. 上传 Agent 与 SHA-256，运行 staging 安装器；
3. 在登录后的 Controller 生成一次性绑定码；
4. 从真实 TTY 执行：

   ```sh
   /root/webvpn-bind-agent https://vpn.example.com
   ```

5. 把输出的 Agent ID 写入 `/etc/scwebvpn/agent.json`；
6. 执行配置检查，手动启动 Agent，确认没有新监听且网关业务不变；
7. 在 Controller 创建服务，让服务 ID 与本地 `policy_ref`/allowlist 对应。

Agent init 只管理进程，不触碰 UCI、路由、防火墙、DNS、DHCP 或 NAT。详见
[OpenWrt 包装说明](packaging/openwrt/README.md)。

## 远程桌面与文件

可选集成位于 `integrations/lan-remote-access`，包含固定版本的 Guacamole、RustDesk
Server OSS、可复建 Canvas Web、Windows File Browser 安装器、状态和备份脚本。它不依赖
Tailscale，也不会安装或删除 Tailscale。按该目录 README 分别验收 RDP、物理桌面与文件。

## 安全与发布

- 运行 [TESTING.md](TESTING.md) 中的全部自动化和现场门禁；
- 对产物执行 secret、生产地址、设备标识与凭据扫描；
- 验证候选中没有 relay，服务器网络计数中没有业务正文；
- 提供当前二进制对应的完整源码与许可证；
- 升级前备份并按 [ROLLBACK.md](ROLLBACK.md) 演练恢复。

安全问题请按 [SECURITY.md](SECURITY.md) 私下报告。许可证为
[AGPL-3.0-only](LICENSE)，分发网络服务修改版本时必须向使用者提供对应源码。

## 可选路线图

- 更完整的跨 NAT 统计与长时间压力测试；
- 替换处于维护末期的独立文件服务；
- 扩展更多 OpenWrt 架构的可复现构建矩阵；
- 在不引入业务中继的前提下改善诊断与可观测性。
