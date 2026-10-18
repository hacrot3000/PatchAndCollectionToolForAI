# Vendored source: github.com/creack/pty

- Upstream: https://github.com/creack/pty
- Version: v1.1.24
- Upstream tree: edfbf75025b0ba4ee17c19f52d9b600fad80a787
- License: MIT (see LICENSE)
- Vendoring policy: source is committed directly for review; build/runtime must not download this dependency.
- Supported vendored PTY targets: Linux/amd64, Linux/arm64, macOS/amd64 and macOS/arm64.
- Darwin additions are verbatim upstream v1.1.24: pty_darwin.go, ioctl_bsd.go and ztypes_arm64.go.

The Go files in this directory are copied verbatim from upstream v1.1.24 for the listed build paths. Future updates must be performed manually: review the new upstream source, replace the committed files, update this record, then commit the reviewed result.
