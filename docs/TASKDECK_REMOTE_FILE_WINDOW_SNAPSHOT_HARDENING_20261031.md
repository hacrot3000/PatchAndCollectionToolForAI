# TaskDeck Remote — file-window snapshot integrity hardening (2026-10-31)

## Finding

The E2EE `project.file.window` reader used a pinned directory descriptor and
computed a SHA-256 of all returned bytes, but `readCurrent` could return fewer
bytes than the original file's pre-read size if another local process truncated
it during the read. A same-size local rewrite or leaf replacement could also
produce a misleading file-version hash. Cryptography authenticates the bytes
transported, **not the consistency of a concurrently edited source file**.

## Change

- After a bounded descriptor-pinned read of a visible regular file (max 2 MiB),
  reopen the leaf through the same pinned parent descriptor.
- Reject an incomplete read, different inode, size, mode, or modification time.
- Refuse the read on failed reopening, stat, close, cancellation, or metadata
  mismatch. Wipe the intermediate read buffer best-effort on any path.
- Regression tests cover stable files, incomplete reads, truncation, same-size
  rewrites, inode swaps and permission changes.
- Retain host-local TaskDeck password + approved client-key proof + per-action
  `FilesRead` + live uncached Portal check + audit and E2EE sealing.
- No PHP change, no new public endpoint, no authority or quota increase.

## Limitations and release decision

This is a **best-effort concurrent modification detector**, not a filesystem
snapshot, exclusive lock or protection against a malicious *local* OS writer
who can modify data and restore metadata between checks. The final native
download still requires a complete SHA-256 match before atomic publication.
A PHP/Relay compromise alone cannot supply the TaskDeck password or
host-approved client private key, or decrypt this application payload.

Both Go and PHP checks must complete successfully against the same PR SHA
before merging. Production Remote Control remains **NO-GO** until the
full HTTP/UI/PTY/Patch/Collection transport, operational deployment, external
security verification and all documented release gates pass. The
`remote_control_available=false` marker and default-OFF flags are unchanged.
