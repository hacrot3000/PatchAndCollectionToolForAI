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
- [ ] Implement and unit test independent ranked completion engine.
- [ ] Implement keyboard/ghost/menu UI and wire bounded interception at xterm input boundary.
- [ ] Implement OS history/command loaders (local and authenticated SSH) with explicit refresh.
- [ ] Implement `\\ai` and `\\do` with safe output display, error recovery, and tests.
- [ ] Run Go tests, Node tests, JS syntax and manual xterm/SSH lifecycle smoke tests.

## Risk notes
- xterm output is not synonymous with Readline input. Only offer completion on a confirmed prompt with an end-of-line cursor and never rewrite arbitrary TUI/password content.
- SSH disconnections and shared-server read-only tabs must not inject commands. Buffered reserved input must never be sent to the server on error/fallback.
- Browser-only command history may not include an unflushed remote Bash history; explicit reload may be required.
