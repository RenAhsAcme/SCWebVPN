# 测试与验收

当前代码基线已完成真实环境部署验收并稳定运行；后续改进不改变下列发布门槛。

## 自动化

- Go：`go test ./...`、`go vet ./...`、`gofmt`；
- 浏览器：单元测试、TypeScript 检查、固定依赖构建；
- 协议：拒绝 `turn:`、`turns:`、relay candidate、超长或重复候选；
- 身份：Ed25519 challenge、单次 nonce、时钟窗口、body hash 和撤销；
- 策略：服务 allowlist、DNS rebinding、重定向再校验、临时服务边界；
- 数据：Wisp 帧、背压、并发上限、取消以及 SQLite migration。

## 浏览器与端到端

只使用系统已有的 Chromium 内核浏览器（例如 Edge）；不要求安装 Chrome。自动化应使用
一次性 profile，不修改个人浏览器配置。

至少验证 LuCI、Guacamole、SPA、WebSocket、SSE、下载上传、ICE restart，以及大文件传输
时的交互与内存上限。远程访问集成另按其 README 验证 RDP、RustDesk 与文件通道。

## P2P 网络矩阵

正向环境必须选择 host、srflx 或 prflx 直连并完成真实业务；UDP 被阻断或 NAT 无法打洞
时必须明确失败，不得出现 TURN、relay 或服务器业务回退。Agent 丢失后通道应在限定时间
关闭，网络恢复只能通过受限 ICE restart 或用户显式重连。

本项目的当前基线已在外部网络完成定性 P2P 测试，成功率表现良好。该事实不构成对所有
NAT、运营商或企业网络的可用性保证；详见 `P2P_LIMITATIONS.md`。

## 发布门禁

- 测试、类型检查、构建和依赖审计通过；
- 仓库、产物与日志不含凭据、生产地址、SDP 或 candidate；
- AGPL 对应源码、第三方许可证和离线重建材料完整；
- Controller/Authelia 仅环回，Nginx 清除伪造身份头，未知 Host 拒绝；
- OpenWrt 的网关、DNS、DHCP、NAT、防火墙和既有服务基线不变；
- 数据库与版本回滚在隔离路径实际恢复。
