# TaskDeck Remote application transport contract — staged, NOT ENABLED

Updated 2026-10-29 · source HEAD at preparation: `9a670bc9075e3703a72cdd3a960157d4ca867b8a`
Branch: `feature/taskdeck-remote-access-architecture`

> **NO-GO FOR REMOTE CONTROL.** The code in this phase is an *unmounted*
> native read-only E2EE ping transport and opaque Relay mailbox. Neither
> establishes application access over the Internet today, nor grants terminal,
> HTTP, files, editor, Git, Patch or Collection capabilities.

## Security invariants

1. **PHP Portal and Relay are considered fully compromised.** Portal may
   issue forged admin tickets, tamper with SQLite, signer/JS and invent
   `allowed=true`. It must not approve host client keys, obtain TaskDeck
   passwords or native private keys, mint an in-process TaskDeck principal,
   or induce an unauthenticated TaskDeck action.
2. A trusted **native** client independently pins the exact Agent Ed25519
   Device ID (outside PHP), verifies its signed ephemeral P-256 handshake
   and AES-GCM ready record, and proves possession of an owner-approved
   Ed25519 client key. Password input appears only after the authenticated
   approved-key E2EE preflight. Host approval remains exclusively via the
   same-user `taskdeck remote-key --workspace ... add/list/revoke` CLI.
3. **One-shot login diagnostic is not an application session.** The existing
   `VerifyEncryptedRemoteLogin` seals only a boolean and destroys its
   in-process principal. It must remain available solely to the explicit
   controlled-test opt-in. **Do not use its result as a bearer token.**
4. A future application session must call `StartEncryptedRemoteSession`
   only on the pinned Agent, and must preserve the returned private,
   password-verified `remoteTaskDeckSession` bound to the **same** E2EE
   channel, immutable local SQLite User.ID/project, approved client key,
   Portal grant and fixed five-minute lifetime. Never serialize it.
5. Each action requires TaskDeck-local principal/membership/RBAC, Portal role
   ceiling and **uncached** live Portal introspection before and after work;
   local audit must fail closed. Portal roles never replace TaskDeck roles.
   A canceled operation must not report success or replay side effects.
6. No generic HTTP proxy, unrestricted WebSocket passthrough, arbitrary
   callback/command/path routing, Portal-served executable or sharing of
   TaskDeck cookies/session tokens.

## Staged application transport components

### Native stop-and-wait ping

`internal/remotetransport/ping_client.go` supplies `PingClient`, accepting
an already authenticated `Channel` and an internal `PingExchange`.

- Generates a fresh cryptographic 128-bit request ID per call; operation
  is **only** `session.ping` and the result must echo that exact ID.
- Only one in-flight call; 15-second deadline, concurrent `Close`,
  cancellation on lost E2EE channel, permanent poison on invalid reply,
  network error, oversize record or deadline breach.
- Maximum encrypted ping record = 512 plaintext bytes + 9 header bytes +
  16 AEAD tag bytes. Both directions remain authenticated AES-GCM.
- The adjacent server tests exchange ciphertext through
  `executeRemoteE2EEPing` with a real local TaskDeck principal; they are
  *in-process tests*, NOT Internet routing.
- `PingExchange` implementers must respect context cancellation; the
  client refuses late replies even when a test implementation ignores
  context. Other action types cannot be introduced by a caller-provided
  string.

### Relay opaque mailbox (not mounted)

`internal/remoterelay/app_mailbox.go` contains an authenticated Agent
readiness gate and bounded, opaque queue. The Relay never sees decrypted
TaskDeck credentials, action types, responses or permissions.

- A previously ACKed login is **not** proof of TaskDeck password validity.
  Before `ActivateAppPing`, the Agent must establish a valid *application*
  session with its local TaskDeck credential verifier and sign
  `taskdeck-e2ee-app-ready-v1\n<DeviceID>\n<AgentNonce>\n<ProbeID>`.
  This proof is separate from connection presence, ticket and login result.
- Verify the Agent public key hashes to the pinned Device ID, verify the
  Ed25519 signature, and bind to the exact authenticated device socket,
  browser probe and current exclusive reservation generation. The readiness
  permission is **one-use**; the same signature cannot reset a quota.
- The mailbox admits at most one request at a time, at most four total
  requests, with a 20-second per-request deadline and five-minute mailbox
  TTL. It has a fresh Relay-generated response correlation ID and bounded
  ciphertext. Agent ACK must match device, connection, probe, request ID,
  and current generation before returning opaque bytes.
- Release, authenticated Agent disconnect, explicit close or expiry must
  cancel pending replies and forbid stale ACKs crossing into another
  session. No plaintext log or persistent network address.
- A valid Agent readiness signature is **not authorization to execute
  commands**. Even a fully compromised Relay that pretends to have one
  must still fail without the real E2EE client private key, host-approved
  key, TaskDeck password and in-process authorized principal.

## What is NOT implemented yet (must remain blocked)

- No Agent production callback wires the in-process application session to
  this mailbox; no production native client's `PingExchange` uses a Relay
  application route; no public device/browser dispatcher accepts app frames.
- No live E2EE Remote-Control test over CGNAT/TLS/WSS, no multiplex,
  cancellation/streaming/backpressure validation against production-like
  Relay, no HTTP-origin isolation, no terminal/PTTY, files, Git, Patch,
  Collection or settings applications.
- No independently verified signed native release on physical Linux/macOS
  yet, and no independent hostile-PHP-and-Relay penetration test.
- RC-03/RC-04 are therefore **PARTIAL for internal components only**,
  **NOT PASS**. RC-05 to RC-13 remain release blockers; RC-02 remains partial.

## Acceptance tests before introducing a network route

Verify cryptographic Agent identity and client-key preflight, wrong user or
project, old password, username recycled with different immutable User.ID,
replayed app-ready signature, duplicate JSON fields, frame order/replay,
one-slot concurrency, quota, oversized bytes, wrong ACK, browser cancel,
Agent reconnect, TTL and per-request timeout, portal fully compromised,
local key/password/account revoke during a stalled Portal introspection,
and TaskDeck audit fail-closed. Test with real Linux/macOS hosts and
CGNAT/outbound 443, then obtain independent security approval.

The existing Relay health response and all diagnostic results must retain
`remote_control_available=false`. Nothing here justifies publishing
a TaskDeck command endpoint.
