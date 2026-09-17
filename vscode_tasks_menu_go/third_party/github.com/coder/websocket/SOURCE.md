# Vendored source: github.com/coder/websocket

- Upstream: https://github.com/coder/websocket
- Version: v1.8.13
- Upstream root tree: 64d7449933124ed1f0d779c477a9e9145bbdac38
- License: ISC-style (see LICENSE.txt)
- Vendoring policy: reviewed source is committed directly; build/runtime must not download this dependency.
- Current vendored target: Linux/amd64 daemon runtime.

The committed source is copied verbatim from upstream v1.8.13 for the runtime code used by this daemon. Tests, examples, CI files, JS/WASM-only files and unrelated upstream development files are intentionally omitted to keep the review surface small.

Future updates must be performed manually: review the new upstream source, replace the committed files, update this record, then commit the reviewed result. Do not add an automatic dependency updater.
