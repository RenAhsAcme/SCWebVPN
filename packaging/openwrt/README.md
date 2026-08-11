# OpenWrt Agent 包装

`webvpn-agent.init` 只管理 Agent 进程，不调用 `uci`、`nft`、`iptables`、`ip`、`route`、
`ifup`、`ifdown`，不重载网络服务，也不创建 WAN 监听端口。

安装顺序：

1. 记录网关、DNS、DHCP、NAT、防火墙和既有服务基线，人工确认目标网段可达；
2. 用 `scripts/build-agent.ps1` 生成目标架构的纯 Go 二进制并核对 SHA-256；
3. 执行 `install-agent-staging.sh`。它创建锁定的 `webvpn` 账户、备份账户数据库，并拒绝
   覆盖已有版本；
4. 将版本置于 `/usr/libexec/scwebvpn/<version>/webvpn-agent`，通过 `current` 链接切换；
5. 将 JSON、Ed25519 私钥和私有 CA 分别置于 `/etc/scwebvpn`、`identity` 和 `ca`；
6. 从真实 TTY 执行 `bind-agent-interactive.sh https://vpn.example.com`。绑定码从标准输入
   传给降权 Agent，不写入命令行、文件或日志；
7. 把返回的 Agent ID 写入配置，运行 `webvpn-agent check -config /etc/scwebvpn/agent.json`；
8. 手动启动并对比网络基线，确认无变化后再决定是否启用开机启动。

不要直接使用示例配置。回滚只停止精确的 procd 服务并切换 `current`，不得自动删除
Agent 私钥、远程访问服务或任何 OpenWrt 网络配置。
