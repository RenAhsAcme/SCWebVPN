# 自托管与维护

## 所有权

生产系统由以下可审计资产组成：

- 本仓库中的 Controller、Agent、浏览器运行时、配置模板、迁移、安装与回滚脚本；
- 锁定版本和哈希的第三方源码/构建产物；
- 源站上的 Authelia、Controller、Nginx 配置与 SQLite 状态；
- OpenWrt 上的 Agent 二进制、本地策略、CA 公钥证书和 Ed25519 私钥。

不依赖第三方 SaaS 业务控制面。可选 CDN/接入层只承载认证与信令入口，STUN 只提供
ICE 地址发现；两者均无权转发内网业务。

## 状态目录

建议生产布局如下，实际路径在部署清单中固定：

```text
/opt/scwebvpn/             只读版本目录与 current 软链接
/etc/scwebvpn/             非秘密配置，root:root 0750
/var/lib/scwebvpn/         Controller SQLite 与迁移状态 0750
/var/log/scwebvpn/         最小化审计日志
/etc/authelia/                      Authelia 配置与 secrets 文件
/var/lib/authelia/                  Authelia SQLite
/etc/scwebvpn/                      OpenWrt Agent 配置、策略、CA、私钥 0700
```

Secrets 不通过环境变量、URL、命令行或仓库传递。部署脚本只创建占位文件并
拒绝权限过宽；密码、TOTP 初始化和绑定码由操作者在交互终端完成。

## 构建与升级

1. 从干净的锁文件构建 Controller、Agent 和浏览器静态产物。
2. 验证上游签名/哈希、SBOM、许可证和仓库内补丁可重放性。
3. 交叉编译与目标 OpenWrt 架构、libc 无关的静态 Agent。
4. 在临时目录运行单元、协议、浏览器和安全门禁。
5. 生成带版本号的不可变发布目录；切换 `current` 前先备份数据库与配置。
6. 数据库只执行向前迁移；回滚需要兼容检查或恢复切换前快照。

Authelia 和 Pion 的新版本先在独立分支更新锁定项并跑完整矩阵，不跟随
`latest` 标签。安全修复可以加急，但仍不得跳过来源、哈希和回滚验证。

## 日常运维

- 每日：服务健康、认证失败率、Agent 在线状态、P2P 成功率与异常字节桶。
- 每周：SQLite 一致性检查、加密备份恢复抽查、源站泄露扫描。
- 每月：依赖与安全公告检查、Edge 当前稳定版兼容测试、密钥权限检查。
- 每月：检查 `scwebvpn-origin` 剩余有效期；在到期前至少 30 天按人工或受审计的
  DNS-01 流程续期。未配置 DNS API 凭据的系统定时器不能视为自动续期。
- 事件后：撤销所有会话/绑定码；必要时轮换 Agent 密钥、Authelia secrets 与
  源站管理凭据。轮换不能写入业务日志。

## 能力边界

本系统不承诺在 UDP 被封锁、对称 NAT/CGNAT 无可用打洞路径时可用。由于明确
禁止 TURN，自托管的含义不是“提供中继兜底”，而是完整持有源码、配置、状态、
验证和回滚能力。
