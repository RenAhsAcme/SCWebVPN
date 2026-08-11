# 许可证合规

| 组件 | 许可证 | 分发边界 |
| --- | --- | --- |
| SCWebVPN 自有代码 | AGPL-3.0-only | 完整对应源码、构建脚本和许可证 |
| Pion WebRTC | MIT | 保留版权和许可文本 |
| Authelia | Apache-2.0 | 保留 LICENSE/NOTICE 与变更说明 |
| Scramjet / Controller | AGPL-3.0-only | 固定完整上游源码，不以 npm 产物替代 |
| Epoxy TLS | AGPL-3.0-only | 固定源码、补丁、工具链和 WASM 哈希 |
| RustDesk Web fork | AGPL-3.0 | 固定提交、补丁、构建脚本和对应源码 |
| Guacamole | Apache-2.0 | 保留镜像与上游通知 |
| RustDesk Server OSS | AGPL-3.0 | 固定镜像 digest，提供上游来源 |
| OGV.js | MIT | 运行时携带 COPYING 与组件许可证 |
| YUV Canvas | BSD-2-Clause | 保留上游许可文本 |
| File Browser | Apache-2.0 | 固定官方二进制与许可来源 |

该表是工程门禁，不替代法律意见。实际发布必须从 lockfile、Go 模块图、容器 digest 和
运行时文件重新生成 SBOM 与许可证报告。

## 完整对应源码

`scripts/package-third-party-source.ps1` 获取并核验 Scramjet 与 Epoxy 的固定上游源码；
`scripts/package-corresponding-source.ps1` 把自有代码、Web 源码、补丁、vendor 和第三方
源码包组合为当前版本的对应源码。公网运行修改版本时，应从无需额外授权的 Source 页面
向使用者提供该归档及 SHA-256。

Scramjet 的固定提交为 `c26bfc6d7f7c7f4dac52ce182a2ceab90e851823`。其上游归档缺少
许可证正文，因此对应源码包必须附带本仓库根目录的标准 AGPLv3 全文。

RustDesk Canvas 的来源、补丁和解码器哈希记录在
`integrations/lan-remote-access/rustdesk-web-build/SOURCE.md`。仓库内运行时可由该记录
重复构建，并通过 `rustdesk-web-open.sha256` 逐文件核验；设备 ID 与服务器公钥只在部署
主机渲染，不属于源码或发布产物。

## 发布门禁

1. 生成 Go、Web、容器与二进制 SBOM；
2. 阻止未知或不兼容许可证；
3. 从公开 Source 页面下载对应源码并在干净环境重建；
4. 比较关键产物与逐文件哈希；
5. 把许可证、对应源码和回滚版本一同备份。
