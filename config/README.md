# 配置模板

本目录只提供不含秘密的审计模板，不会自动安装。复制后必须替换 `example.com`、
`REPLACE_*`、网段、端口和证书路径。

- `controller.example.json`：仅监听环回地址的 Controller、SQLite 与 STUN-only 配置；
- `agent.example.json`：Agent 身份、服务 allowlist、私有 CA 与密钥路径；
- `nginx-webvpn.conf.example`：Authelia `auth_request`、身份头清除与静态入口；
- `nginx-webvpn-edge.conf.example`：源站证书与 CDN/接入层客户端证书校验；
- `nginx-default-reject.conf.example`：裸 IP、错误 SNI 与未知 Host 拒绝；
- `authelia.configuration.yml.example`：Authelia 4.39.20 密码、TOTP 与 SQLite；
- `users_database.yml.example`：文件用户后端结构占位。

生产秘密必须另存于主机的受限文件：Controller 内部认证值、Authelia session/storage/
identity-validation secrets、Agent Ed25519 私钥和恢复码均不得进入仓库、命令参数或日志。
密码哈希与 TOTP 首次注册只在真实交互终端完成。

示例使用 Authelia filesystem notifier，不会发送邮件。首次注册设备时，从源站受限文件
`/var/lib/authelia/notifications.txt` 读取最新链接并在当前浏览器完成注册；生产环境可改用
受信 SMTP。通知文件可能含短期令牌，只允许 `authelia` 用户读取，使用后按运维策略清理。

Nginx 模板默认把 `$remote_addr` 传给后端。若部署在 CDN 后，应仅信任已核验的 CDN
回源地址范围，并通过 Nginx `real_ip` 模块恢复客户端地址；切勿直接信任公网请求自带的
任意厂商头。`scwebvpn-edge.conf` 中的双向 TLS 只是示例接入边界，部署者必须使用自己
的源站证书和接入层客户端 CA。

Agent 绑定后会返回 22 字符标识。将它写入 `agent.json`，再在已登录 Controller 中创建
服务，使服务 ID 与本地 allowlist 精确对应。Controller 只保存名称、类型和 `policy_ref`；
真实目标、允许端口、CIDR、私有 CA 与 SPKI 例外始终只存在于 OpenWrt。

示例网段仅用于说明。部署前应从 OpenWrt 逐个验证路由、固定租约、目标地址和端口，
不得让安装脚本自动修改默认路由、DNS、DHCP、NAT 或防火墙。临时服务同样只允许命中
本地 `temporary` allowlist；Agent 会拒绝回环、本机接口地址、网段外 DNS 结果和未列出端口。
