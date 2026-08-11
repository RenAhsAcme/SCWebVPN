# 源站与地址保护

## DNS 与边缘

- `vpn.example.com` 和 `*.vpn.example.com` 必须处于受信 CDN/接入代理状态；禁止
  为排障临时切成 DNS-only。
- 边缘证书与源站证书分别验证。源站必须有同时覆盖 `vpn.example.com`、
  `auth.vpn.example.com` 和 `*.vpn.example.com` 的证书；现有
  `*.example.com` 不能覆盖后二者，不得以关闭源站证书校验代替签发。
- 公网源站防火墙只允许当前接入层官方回源网段访问 80/443，或要求接入层 mTLS；管理入口使用独立、
  已验证的受控路径，不从历史记录硬编码地址。
- Nginx 仅信任精确的接入代理网段写入真实客户端地址；其他来源的转发头删除。
- 默认虚拟主机拒绝未知 Host、裸 IP、错误 SNI 和非预期协议。

## 应用输出

- 前端构建、source map、错误页、健康检查与响应头不能包含源站/内网 IP。
- Controller API 只返回 Agent/服务的随机 ID、状态和候选类型，不返回地址。
- 不把 SDP、ICE candidate、Agent 策略、私有 CA 或目标 URL写入数据库、指标、
  Nginx access log、浏览器诊断包。
- CSP/reporting、遥测和第三方资源默认关闭，避免把私有子域或路径发给外部站点。

## WebRTC 的固有限制

已完成 MFA 的两个 WebRTC 对端需要获知可尝试的 ICE 地址；这无法在不使用
中继的前提下完全隐藏。控制措施是：

- 信令能力值短时、单次、绑定当前用户/Agent/服务；
- Controller 不记录 SDP，错误响应不回显；
- 浏览器保留 mDNS host candidate 默认行为；
- 诊断 UI 只显示 `host/srflx/prflx` 类型和匿名化网络标签；
- 未认证访问者、接入层日志使用者和普通站点访客不能获取 ICE 内容。

## 验证

上线前从至少两个外部解析器检查代理状态，并扫描 DNS 历史、证书透明度中不应
出现的主机名、HTML/JS/source map、响应头和错误页。源站防火墙需以实际包过滤
验证，而不能仅以接入层控制台状态作为证据。还要分别检查边缘对外证书与 Nginx
实际加载的源站证书 SAN，不能用其中一层的成功推断另一层。
