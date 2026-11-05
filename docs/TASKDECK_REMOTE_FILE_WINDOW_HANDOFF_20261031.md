# TaskDeck Remote — authenticated binary file windows (staging)

Date: 2026-10-31. Branch: `feature/remote-e2ee-pinned-file-windows-20261031`.

## Goal

Enable a previously trusted native client to inspect **one 128-byte binary window** of a host workspace file that was too large/non-UTF-8 for the earlier 192-byte preview, without introducing a PHP/browser password form, generic file transfer, arbitrary HTTP handler, local command, Terminal or filesystem-wide capability.

## Contract

- Client: `taskdeck_remote_login -read-window=docs/notes.bin -offset=128`.
- Strict E2EE operation `project.file.window`, `path` and an integer byte `offset`, with no arbitrary arguments. Offset must be within 0..2,097,152. No extra/duplicate JSON fields, float/exponent or ambiguous response type.
- Host enforces an already authenticated in-process TaskDeck password principal, owner-approved private-key signature, `FilesRead` RBAC and **uncached** Portal grant before and after every operation. Local audit entry `remote.action.project_file_window` is mandatory before sealing data.
- Host resolves a **visible** file within the primary workspace, excludes dotfiles and all hidden path components, follows no leaf symlink escape and opens through the existing descriptor-pinned file adapter.
- Maximum file size: 2 MiB; host reads only a bounded full file version into private memory, calculates **SHA-256 over the complete version**, then returns up to 128 bytes from the requested offset. Offset after EOF fails closed; exact EOF may return an empty chunk. Buffers wiped best-effort.
- E2EE response includes `offset`, `size`, lowercase 64-hex `file_sha256`, strict canonical `content_b64`; AES-GCM record binds the request ID. Native client rejects unexpected offsets, size, extra fields, incorrect base64, invalid digest and any remote-control/cookie capability.
- Native client prints BASE64 only; it does not emit untrusted binary to a terminal or auto-write to a local file.
- Existing Relay quota remains **six stop-and-wait actions per login** and five minutes, max 15 seconds per action, with cancellation on disconnect. Large file streaming and resumable download are **not** implemented.
- Existing write/delete/rename, Shell/PTY, Patch/Collection and authenticated HTTP route remain unchanged and unavailable through this feature. `remote_control_available=false`; experimental opt-in flags stay OFF by default.

## Security and remaining tests

Protocol tests exercise AEAD round trips, strict offset boundaries, schema confusion and forged result fields. Host tests cover complete-file version changes, 2 MiB cap, binary data, hidden/overlong/outside paths, symlinks, revoked client keys, Portal-only denial, E2EE replay and trusted client consecutive windows.

Mandatory acceptance: Go + PHP workflows PASS on exact PR head SHA, existing compromised-Portal + real Relay red-team, Linux CI, native macOS compilation and on-device integration. Do **not** claim a production-ready Remote Control release based on this bounded feature. Follow-on work must separately design resumable E2EE transfer, secure local output, session renewal and 1+ MiB performance without loosening any of the three original PHP compromise / plaintext / TaskDeck RBAC safety invariants.
