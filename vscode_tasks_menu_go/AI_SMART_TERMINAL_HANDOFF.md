# Smart Terminal AI — Handoff / Recovery Journal

Updated: 2026-10-13
Branch: `feature/ai-smart-terminal-l1-l3`
Base: `main`
Tracking plan: [AI_SMART_TERMINAL_PLAN.md](./AI_SMART_TERMINAL_PLAN.md)

## Current checkpoints
- A0 ✓ Branch created
- A1 ✓ Plan committed: `28d6ab9c`
- A2 ✓ Handoff committed (this commit)
- L1-0 ✓ Backend OpenAI Chat Completions adapter, encrypted write-only secret and private workspace settings; `d64e70b5`, `2b494512`, `656a5af1`.
- L1-1 ✓ Settings > AI Assistant modal (enabled, consent, model, key, timeout, output cap, request/hour, context opt-ins); `9d3352ad`, `956b924a`, `0d7de8de`.
- L1-2 ✓ Side panel with explicit context preview, Explain/Suggest/Plan modes and no execution; `9d3352ad`, `751ce715`.
- L1-3 ✓ API/UI tests and operator docs; `5ece2774`, `af1ccf77`, `e3e154b1`. Provider E2E with real credential NOT run.
- L2-0 ✓ Opt-in CWD, Git summary, last eight commands, SSH marker, frontend/backend redaction, size bound and explicit external confirmation; manual sensitive-log verification remains.
- L3-0 ✓ Per-user private plan journal with immutable target snapshot, approve/reject-once workflow and explicit `execution_enabled:false`; `d3c3b336`, `9eb8f182`, `84418275`.
- L3-1 BLOCKED — actual command execution / autonomous retries not implemented. Must pass sandbox, target authorization, approval integrity and security review; see Implementation Guide.
- Quota guard ✓ `a2ba80cd`, `da6b5b3b` (process-local, not cluster-wide).

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

## Latest engineering notes
- CI enabled on the feature branch by `a47a4bbf` (Go 1.19 and 1.23). Intermediate commit `0d28c6d` failed because static UI assertions had stale expected strings; fixed by `af1ccf77`. Earlier commits `84418275`, `0a57c72e` passed; check latest workflow before promotion.
- Real external provider call intentionally untested: no OpenAI secret in repository and CI must not incur charges.
- No provider URL configurability yet; OpenAI Chat Completions fixed endpoint eliminates arbitrary URL/SSRF in this phase.
- L2 context comes from browser-visible Shell Integration, NOT a server-attested session target; do not treat its session ID as authorization to execute.
- L3 plan journal per user, but proposed target is not server-attested. Execution remains disabled.
- Settings are persisted outside repository in TaskDeck user-config dir; API key stored in existing encrypted `ConnectionSecrets` store. Multi-user admin should control settings.write.
- Next action if continuing: inspect latest CI logs; add server-side target ownership binding, execution principal, typed tools, isolated sandbox, approval hash/TTL, and durable audit for real L3 execution. Do NOT bypass gates.

## Current API contracts
- `GET/PUT /api/ai/settings` with write-only `api_key`; GET never echoes credential.
- `POST /api/ai/assist` -> `{answer,mode,provider,executed:false}` (explicit per-request consent). Modes explain/suggest/plan only.
- `GET/POST /api/ai/plans` -> user-scoped proposal list/create, 1–8 steps.
- `POST /api/ai/plans/decision` -> per-step pending to approved/rejected, no executor.
