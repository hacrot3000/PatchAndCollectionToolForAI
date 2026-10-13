# Smart Terminal AI — Handoff / Recovery Journal

Updated: 2026-10-13
Branch: `feature/ai-smart-terminal-l1-l3`
Base: `main`
Tracking plan: [AI_SMART_TERMINAL_PLAN.md](./AI_SMART_TERMINAL_PLAN.md)

## Current checkpoints
- A0 ✓ Branch created
- A1 ✓ Plan committed: `28d6ab9c`
- A2 ✓ Handoff committed (this commit)
- L1-0 ◻ Backend service settings/provider/security
- L1-1 ◻ Settings dialog with permissions and key handling
- L1-2 ◻ Assistant panel and suggest-only requests
- L1-3 ◻ Tests and documentation
- L2-0 ◻ Context builder opt-in + redaction/preview
- L3-0 ◻ Approval-only proposal schema and audit
- L3-1 BLOCKED — automatic tool execution requires separate security review

## Exact constraints
- Never commit secrets or expose keys in browser responses.
- Do not use AI-provided content as executable command.
- Existing terminal/session/SSH behavior must not regress.
- L1 default off; no command execution.
- User may opt in to selected current terminal command/output, CWD, Git; no implicit full log upload.
- Provider request must be made from Go backend, with bounded timeout, size and credentials never in URL.
- Multi-user access checks MUST be server-side.
- No shell execution as L3 until safe action-approval system and audit exist.

## Repository landmarks
- `vscode_tasks_menu_go/internal/server/server.go` HTTP router and auth integration
- `vscode_tasks_menu_go/internal/server/ui.go` TaskMenuApp and core browser UI
- `vscode_tasks_menu_go/web/featuremods/menus.js` Settings sections
- `vscode_tasks_menu_go/web/featuremods/next.js` module entrypoint
- `vscode_tasks_menu_go/web/featuremods/shellintegration.js` OSC 7/133 CWD, command history and events
- `vscode_tasks_menu_go/web/featuremods/terminalpalette.js` unified command palette
- `vscode_tasks_menu_go/web/featuremods/terminalrestore.js` persistent tabs
- `vscode_tasks_menu_go/internal/server/shared_authorization.go` shared auth API scope

## Next steps for resumed implementation
1. Read this file, Plan and last branch commits.
2. Inspect server config/private settings conventions and multi-user authorization.
3. Implement backend secure L1 API + tests before wiring UI.
4. Update this journal with exact commit, tests and blockers after each milestone.

## Known limitations
No third-party LLM provider credentials are available in the repository. Functional model call end-to-end tests require user-configured provider credentials and cannot be claimed completed here. Manual confirmation on Firefox and real SSH production is required. Production execution policy is out of L1 scope.
