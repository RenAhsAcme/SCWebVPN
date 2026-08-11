# 回滚

## 原则

回滚只移除 WebVPN 自己的入口、服务和状态引用，不触碰 OpenWrt 网关配置、
Guacamole、RustDesk、站点其他虚拟主机或个人数据。版本目录和数据库快照在验证
完成前保留。

## 源站回滚

尚未启动服务的 staging 失败只允许清理精确版本目录、精确 systemd 单元和空 secret
占位；先确认 `current` 未指向该版本、服务未启用且端口未监听。不得用递归清理覆盖
`/opt`、`/etc` 或 `/var/lib` 的其他内容。

1. 在 Nginx 中禁用 WebVPN 虚拟主机或切回上一份已验证配置，运行配置检查后
   reload；不得覆盖整个 Nginx 配置目录。
2. 停止 Controller 和 Authelia WebVPN 实例，确认环回端口释放。
3. 把 `current` 原子切回上一版本；若新版本迁移不向后兼容，先停止写入，再恢复
   切换前 SQLite 快照。
4. 撤销新版本创建的会话、绑定码和 Agent key registration。
5. 验证同一源站的其他虚拟主机和既有远程访问仍正常。

## OpenWrt 回滚

1. 停止且禁用 WebVPN Agent 的 procd 服务；确认只终止该服务的精确 PID。
2. 切回上一 Agent 版本或移除 WebVPN 专属启动项。
3. 默认保留 `/etc/scwebvpn` 以便调查；需要删除时先导出加密备份并由用户
   明确确认。私钥销毁属于不可恢复操作，不在自动回滚中执行。
4. 检查没有新增 WAN 监听或残留临时进程。
5. 验证 LAN/WAN、默认路由、DNS、DHCP、NAT、防火墙、Guacamole、RustDesk。

## 数据库恢复

- 备份必须同时包含 SQLite 主文件和一致性快照，不直接复制活跃 WAL 的单个文件。
- 恢复到隔离路径运行 `integrity_check` 和 schema 版本检查，再停止服务并原子切换。
- Authelia 与 Controller 分别恢复；禁止用一个数据库覆盖另一个。
- 恢复后撤销快照之后产生的所有浏览器会话和一次性能力值，避免状态回退重放。

## 回滚成功标准

- WebVPN 公网入口不可用或回到上一版本；未知 Host 仍拒绝；
- OpenWrt 网关路径和既有远程访问没有回归；
- 没有孤立 Controller/Agent 进程、监听端口或可复用绑定码；
- 回滚记录只包含版本、时间、结果码和操作者，不包含 secret/IP/SDP。
