# 部署细节

主流程见 `README.md`。本文件补充生产切换、权限与回滚边界。

## 前置条件

- `vpn.example.com`、`auth.vpn.example.com` 与 `*.vpn.example.com` 已解析到受信接入层；
- 边缘与源站证书覆盖上述名称，源站拒绝未经授权的直连；
- Authelia 4.39.20 二进制已按官方 SHA-256 校验；
- Controller、Agent、Web 与对应源码包从本仓库构建并生成哈希；
- 已备份 Nginx、Authelia、Controller 数据库和 OpenWrt Agent 目录；
- OpenWrt 网关、DNS、DHCP、NAT、防火墙和既有服务基线已记录。

源站证书可用 ACME DNS-01 签发。随附手动 hook 不保存 DNS 凭据；部署者应逐条添加临时
TXT、等待权威 DNS 生效，签发后删除。不要把 DNS/CDN API 凭据写入仓库或日志。

## 源站顺序

1. 构建 Controller 与 Web，准备已校验的 Authelia 二进制；
2. 运行 `scripts/package-corresponding-source.ps1` 和 `scripts/package-source-release.ps1`；
3. 在 Linux 解包并校验 `manifest.sha256` 与外层 SHA-256；
4. 执行 `packaging/systemd/install-source-staging.sh <version> <archive> <sha256>`；
5. 交互写入 Controller/Authelia secrets、密码哈希、TOTP 和恢复码；
6. 校验 schema、Authelia 配置与 systemd 单元，启动两个环回服务；
7. 复制 Nginx 示例并替换域名、证书、接入层 CA 和 real-IP 规则；配置测试通过后 reload；
8. 验证 MFA、注销、会话过期、未知 Host、裸源站拒绝和无身份头伪造。

Controller 和 Authelia 不得监听公网。Nginx 只承载小体积控制信令和静态页面，不得新增
Wisp-over-WebSocket、TCP tunnel、上传或 TURN 中继 location。

## OpenWrt 顺序

1. 构建目标架构 Agent，上传并核验 SHA-256；
2. 用 staging 脚本安装独立版本，但不启动、不启用；
3. 生成 Ed25519 身份，在 Controller 生成一次性绑定码；
4. 从真实 TTY 运行绑定助手，把返回的 Agent ID 写入配置；
5. 执行 `webvpn-agent check` 后手动启动；
6. 确认没有新监听端口，并复核全部网关业务基线；
7. 创建服务并完成真实 P2P、远程访问与回滚验收后再启用开机启动。

## 权限与迁移

- SQLite 使用 WAL、`busy_timeout`、外键和版本化 migration；迁移前做一致性快照；
- schema 高于二进制支持范围时拒绝启动；
- Authelia 与 Controller 使用不同数据库、用户和 secret；
- Nginx 只读静态文件，不能读取数据库、Agent 私钥或 TOTP seed；
- `/opt/scwebvpn/current` 只指向一个不可变版本，Controller、Authelia 与 Web 同步切换。

任何失败都按 `ROLLBACK.md` 回到上一版本。OpenWrt 回滚不得夹带网络配置改动。
