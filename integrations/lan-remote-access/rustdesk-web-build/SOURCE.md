# RustDesk Web source record

The files in `../rustdesk-web-open/` are built from the AGPL-3.0 RustDesk Web client fork at:

- repository: `https://github.com/MonsieurBiche/rustdesk-web-client`
- commit: `525b5e561faf824850c71500adf463e4e0a504d4`
- patch: `lan-remote.patch`
- Yarn: `3.2.0`, using the source tree's immutable lock file
- OGV: npm `ogv@1.8.6`, archive SHA-256 `4575bb7984c24d8872862df764bc0fdc50a18e9d9753a44e53b0640948093078`
- YUV Canvas: npm `yuv-canvas@1.2.6`, archive SHA-256 `f76381274f2e2524f8a82ce39201080a2a568f5a6c6983b7f4befac2e124c7d1`
- Opus worker: RustDesk's published `libopus.js` and `libopus.wasm`, verified by the hashes embedded in `build-rustdesk-web.ps1`

The runtime deliberately uses `classic-ui.js`, a small Canvas UI built against the same JavaScript core, rather than the fork's newer Flutter UI. This avoids a cross-generation PeerInfo/RGBA bridge and leaves one direct path from decoded VP9 frames to the browser canvas.

`lan-remote.patch` and the classic UI make seven deployment-specific changes:

1. hides the account surface because RustDesk Server OSS has no account API;
2. restricts ID and relay WebSockets to same-origin `/ws/id` and `/ws/relay`;
3. accepts only the non-secret RustDesk ID from the `id` query parameter and never puts the permanent password in the URL;
4. explicitly advertises VP9 as the only supported video codec because this Web core does not decode H.264, H.265 or AV1 frames;
5. disables public-server probing, preserves the deployment-pinned server key, and removes Firebase Analytics and Service Worker registration;
6. draws decoded frames directly to YUV Canvas and sends pointer and keyboard input directly through the matching connection core.
7. initializes the pinned OGV VP9 decoder before rendezvous and uses its single-threaded WASM path because Worker startup is not reliable inside the Scramjet compatibility frame.

Run `../scripts/build-rustdesk-web.ps1` from a normal Windows PowerShell session to produce a fresh sibling directory and SHA-256 manifest. The script refuses to overwrite either its work directory or output directory.

The Web core implements screen, audio, keyboard and pointer control. Its file-transfer methods are not implemented. File transfer is therefore provided as a separate WebVPN P2P service and must not be represented as a RustDesk Web capability.
