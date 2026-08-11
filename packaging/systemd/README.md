# 源站 systemd 单元

Controller 单元只允许访问 `/var/lib/scwebvpn`、自身只读版本目录和配置文件，
并把网络范围限制为本机环回。Authelia 单元独立使用 `/var/lib/authelia` 与
`/etc/authelia`，不能读取 Controller secret。两者都不应拥有 Nginx 私钥、站点其他
目录或业务数据的写权限。

投入生产前必须在目标 systemd 版本上运行：

```text
systemd-analyze verify scwebvpn-controller.service scwebvpn-authelia.service
systemctl show scwebvpn-controller.service
systemctl show scwebvpn-authelia.service
```

如果目标内核或 systemd 不支持某个加固项，应逐项记录兼容性决定；不得为了启动
方便而整体删除沙箱配置。Controller 配置和内部认证 secret 分开保存，secret 文件
权限为 0640 且只允许 `webvpn` 组读取。

Authelia 的三个 secret 只通过 `*_FILE` 变量读取；单元中仅出现文件路径，不出现值。
`/opt/scwebvpn/current` 是同一源站发布的唯一版本链接，Controller、Authelia
和浏览器静态文件必须从同一不可变版本目录切换与回滚。
