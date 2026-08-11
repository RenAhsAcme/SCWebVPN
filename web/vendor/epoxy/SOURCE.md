# Patched Epoxy TLS runtime

The checked-in runtime is built from the source commit embedded in npm package `@mercuryworkshop/epoxy-tls@2.1.19-1`.

- Repository: `https://github.com/MercuryWorkshop/epoxy-tls`
- Commit: `93d5a726894b2f16bad54c4a3801446cbbd22d26`
- Source archive SHA-256: `d5c7d191b5a5c6f7b9b0280688fe477f45dc60a599e9beef34a0f2d0e0c60fff`
- Patch: `../../patches/epoxy-websocket-origin-form.patch`
- Rust compiler commit: `7608eb7b07eaf93f16d7cf5bcb2098eca87503df`
- `wasm-bindgen`: `0.2.100`
- Zig: `0.16.0`
- `epoxy_client_bg.wasm` SHA-256: `cdc114b6fa1e1838d621487641df356728e8e8200aba0e7277429603313cbfd1`

The patch separates the HTTP request target from the TCP/TLS connection target. Direct WebSocket handshakes now use origin-form path/query and an authority-valued `Host` header, while Epoxy still connects to the full remote URI.

Rebuild with `bun run build:epoxy` from the parent `web` directory. The script downloads only pinned official archives, verifies their SHA-256 values, uses an isolated cache, applies the patch with a preflight check, and refuses toolchain drift.

Epoxy TLS is licensed under `AGPL-3.0-only`; the corresponding license is included next to the generated runtime. Public distribution must also make the complete corresponding patched source and build instructions available.
