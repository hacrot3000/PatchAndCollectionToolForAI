# Inline Smart Terminal — implementation handoff

Updated: 2026-10-14. Target branch: `main`.

## Contract
- Browser xterm.js is the interactive terminal; do not replace the PTY, SSH transport or remove ordinary Bash/Readline editing.
- Ghost completion (faint suffix) and a floating, mouse/keyboard-selectable list above the actual cursor. Tab accepts, arrows cycle; suggestion sources rank: this terminal's recent commands, project's other active terminal commands, shell history from the relevant OS, available OS commands; safe static shell verbs as final fallback.
- Match command prefix, word/boundary, substring, and common command-next-argument positions. Deduplicate by full command; never execute suggestions without explicit user action.
- Reserved top-level lines `\\ai <question>`, `\\do reload command`, `\\do reload history` must be intercepted *before* WS/PTY forwarding. Keep normal commands, password input, TUI applications, bracketed paste, and Ctrl-C functional; never silently execute AI text as shell commands.
- `\\ai`: use existing provider settings, consent/verification, request quota and external-provider boundaries; response displayed locally in differentiated console area. Model can only advise.
- `\\do`: manually refresh OS command/history sources; SSH uses the current shell only when safe (verified idle shell), not an untrusted remote bootstrap. No third-party library/npm, no runtime source downloads.
- Security: disabled for read-only/non-controller sessions; never query other users' private command history. Handle ANSI and control sequences as untrusted; bound buffer and query sizes. Never push secrets, private history or raw terminal output to AI without per-request consent.

## Progress
- [x] Code survey: `internal/server/ui.go` owns xterm's `onData` -> `view.ws.send`; `web/featuremods/shellintegration.js` captures OSC 133 commands; `shellhistory.js` persists per-session history; `aiassistant.js` provides explicit provider approval.
- [x] Implement and unit test independent ranked completion engine (`smartterminal_engine.js`, `test_smartterminal_engine.cjs`; Node execution pending CI).
- [x] Implement first-stage keyboard/ghost/menu UI and wire bounded interception at xterm input boundary. Runtime UX/browser smoke still pending.
- [x] Implement fixed Bash OS command/history reload via active local/SSH PTY. End-to-end SSH smoke still pending.
- [x] Implement `\\ai` and `\\do` dispatch, output differentiation and error handling. Browser integration tests pending. `\\ai` direct verified-target/provider advisory requests implemented; `\\do` dispatch pending.
- [ ] Run Go tests, Node tests, JS syntax and manual xterm/SSH lifecycle smoke tests.

## Risk notes
- xterm output is not synonymous with Readline input. Only offer completion on a confirmed prompt with an end-of-line cursor and never rewrite arbitrary TUI/password content.
- SSH disconnections and shared-server read-only tabs must not inject commands. Buffered reserved input must never be sent to the server on error/fallback.
- Browser-only command history may not include an unflushed remote Bash history; explicit reload may be required.

## 2026-10-14 engine checkpoint
Strict source ordering, recency, prefix/word/substring match, contextual verbs, deduplication and basic secret/control character filtering. This engine makes suggestions only; it cannot execute.

## 2026-10-14 UI checkpoint
Completion is limited to confirmed OSC 133 prompt, end-of-line, running writable terminal. Overlay is outside the PTY buffer, using xterm cursor coordinates; no suggestion is ever auto-executed. `Tab` inserts only an explicitly chosen suggestion. Reserved commands are still pending agent implementation.

## 2026-10-14 reserved-input checkpoint
`smartterminal_agents.js` now captures reserved line prefixes before WebSocket forwarding. Plain Bash escapes that do not match are released to the PTY; multiline reserved paste is rejected. `\\ai` requires verified target, configured provider and explicit per-call confirmation. AI output is rendered as sanitized colored console text and can never execute directly. `\\do` is still safe-blocked, awaiting fixed loader implementation. Any UI/PTY race conditions require browser smoke testing.

## 2026-10-14 reload protocol checkpoint
Added a pure, bounded UTF-8/text framing and sanitization parser for fixed command inventory and shell-history refresh. It strips echoed prompt commands by requiring entire framed marker lines, handles chunk splits and limits response bytes. Wiring to the PTY loader is next.

## Parser fix checkpoint
CRLF is normalized across chunks to avoid failing to detect a complete end marker on Linux/SSH PTYs.

## PTY index loader checkpoint
Fixed Bash scripts for `\\do reload command/history` run on the active verified PTY (including SSH), never sending literal `\\do` lines. Bounded framed output is captured before xterm rendering and indexed. First-prompt auto-load caches OS command names per target in sessionStorage; history remains memory-only. Non-Bash shells are not supported in this first stage.

## CI checkpoint
Added the inline ranking and framed capture Node tests to the Go matrix workflow. Corrected a stale SSH WebSocket regression expectation (the code resets reconnection backoff after a successful socket open; second disconnect retries in 600ms, not 1200ms). Browser/SSH runtime tests are still separate from pure unit coverage.

## 2026-10-14 lifecycle checkpoint
Synchronous synthetic `\\ai`/`\\do` Enter bypasses OSC fallback tracking to prevent local-only commands from being misreported as shell execution. SSH/WebSocket disconnection now cancels active capture and outbound AI request; no replay on reconnection. Corrected the second stale reconnect test assertion: a newly scheduled retry increases attempt counter to one.

## 2026-10-14 prompt/paste/UI checkpoint
Completion now reads only characters preceding the actual xterm cursor (including a typed trailing space), resets selection when the query changes, and supports wheel navigation over the popup. Old WebSocket close events no longer cancel a new socket's agent operation; cancelled AI calls cannot inject an extra newline after reconnection.

## Cross-tab history checkpoint
In addition to live command lists on each open terminal, recent `command-finished` events are retained in bounded browser-memory project history, allowing suggestions from tabs closed in the same TaskDeck browser session. No cross-user permanent command-history store is added.
