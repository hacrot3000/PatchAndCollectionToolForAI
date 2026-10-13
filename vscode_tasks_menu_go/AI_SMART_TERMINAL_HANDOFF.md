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
 
## Follow-up implementation checkpoint (2026-10-13)

- L3 modal `web/featuremods/aiplans.js` added: parse proposed JSON steps, manually approve/reject, persisted journal, execution hard-disabled. `9eb8f182`, `681c9fe0`, `0a57c72e`; fence parsing fixed `264bc2cd`.
- AI plan request remembers the selected terminal target; switching tabs invalidates it. `6b78be23`.
- Multi-user per-user request quotas and one in-flight provider call: `654090fb`, `a2ba80cd`, `0d7de8de`, `da6b5b3b`. Quota is process-local, not multi-daemon.
- Regression tests: backend `ai_assistant_test.go`, `ai_plans_test.go`, `ai_ui_test.go`, `test_ai_smart_terminal.cjs`; CI enabled for branch. Initial UI assertion mismatch `0d28c6d` corrected by `af1ccf77`. One unrelated intermittent SFTP TempDir cleanup failure observed in an earlier run, newer builds passed.
- Latest permissions refinement: Settings-read prerequisite for AI panel and plan UI, read-only Settings form without settings.write; `b17679fc`, `9d9e4eb1`, `e7c70330`.
- Operator manual: `AI_SMART_TERMINAL_IMPLEMENTATION.md`, commit `e3e154b1`.
- **Do not merge to main or enable production L3 execution without additional review.** User has not provided a live API credential for endpoint smoke testing.
- Next steps: verify latest branch CI, run manual Settings test and OpenAI provider end-to-end with test credential, harden per-session backend authorization, implement typed/sandboxed execution broker, exact command/target/TTL approvals and Operation Center integration. These last items are OUTSTANDING, not implicitly completed.

## Continuation checkpoint — server-side attestation and L3 review hardening

Work resumed on branch `feature/ai-smart-terminal-l1-l3`; `main` remains untouched.

### New verified milestones

- **Authoritative terminal identity:** `ai_target.go` validates non-empty session ID, live terminal status, session kind, exact owner/project visibility, shared broker ownership capability and server-side SSH profile. Client-supplied host/CWD/remote are discarded during plan creation. Commits `92242767`, `f180ff8f`.
- **L1/L2 provider gate:** `POST /api/ai/assist` now requires session_id and validates target *before* accessing provider credentials or billing. UI supplies exact active session ID. Commits `de76a0b4`, `dc4694a6`.
- **Context preview:** `GET /api/ai/session-context?session_id=...` returns only backend-verified session identity and launch CWD (not transcript, environment or secrets). The AI panel must verify this before the external-send confirmation. Commits `59aa0bbc`, `0b229833`, `4c910d64`, `1ca7e2cf`.
- **Plan approval integrity:** server binds exact normalized steps, risk, owner, session target, createdAt and expiresAt into SHA-256 digest; approve/reject requires matching digest, current live attested target and unexpired 30-minute window. Commits `ed284df4`, `136a7250`, `01ca34b2`.
- **Risk and typed diagnostics:** `ai_risk.go` overrides untrusted model risk labels with high/critical for unknown or dangerous commands; a small exact-command read-only candidate registry supports `POST /api/ai/plans/validate` and **Check read-only candidates** UI. SSH is never eligible; no execution. Commits `1b633428`, `d19f2e9a`, `bb29f35c`, `897c2412`, `e09d183e`.
- **Bounded journal:** read/write share a 1 MiB size cap; `7dad7419`.

### Test coverage added
- `ai_target_test.go`: cross-user/project, nonexistent, wrong kind, stopped/stale, forged host/CWD, advisory-provider gate.
- `ai_plan_integrity_test.go`: wrong/missing fingerprint, immutable command text, expiry and forged expiry extension.
- `ai_session_context_test.go`: read-only verified context and denial cases.
- `ai_risk_test.go`: model downgrades and shell metacharacter cases.
- `ai_plan_validate_test.go`: exact typed candidate, unknown/remote command refusal, cross-owner validation.
- `ai_ui_test.go`: new UI wiring and permission routes.

### Limits and next secure steps

- L3 **EXECUTION IS STILL DISABLED**. Typed diagnostics are candidates only, never invoked. Do not interpret Approve as permission to run arbitrary commands.
- Browser-provided shell output, prompt history and "current" CWD remain untrusted. Verified backend metadata refers to the session's **launch CWD**, not necessarily the shell's post-`cd` directory.
- Plan fingerprint prevents accidental stale approvals in normal authenticated flow; it is not a substitute for sandbox, RBAC or an immutable server-side execution authorization.
- API key and live OpenAI provider test still require user configuration. No real SSH production/sandbox execution tests have been run.
- Remaining implementation: isolated non-SSH read-only typed executor with strict allowlist and no shell interpolation; independent auth principal; per-tool permission matrix; resource/network/FS limits; durable task journal; Operation Center; idempotency and cancellation; audit; then separately reviewed production/SSH policy.
- CI: multiple preceding revisions PASSED Go 1.19/1.23. Some intermediate revisions failed until updated tests were committed. A separate legacy FTP idle-deadline test intermittently failed on an otherwise unrelated commit; verify newest workflow before promotion.

## Git worktree compatibility / self-update blocker (2026-10-13)

- On revision 57d2d80c, candidate Go tests failed because the installed Git rejects `git worktree list --porcelain -z` with `unknown switch 'z'`.
- `e5a73e94`: prefer NUL-separated records; retry plain `--porcelain` only if the error identifies the unsupported z option. Parse newline/blank-line porcelain on fallback. Both listing and Add/Remove/Open reference resolution use this helper.
- `16a45b6d`: parser and end-to-end integration tests with a Git wrapper rejecting `-z`, including opaque ID lookup and remove.
- `f1185168` and `929e329c`: preserve and test trailing spaces in worktree paths, keeping the opaque ID stable.
- Normal Git errors are not retried. Public API still returns only relative/display paths and opaque IDs.
- CI on the new revision plus a manual update on the affected older-Git host remain necessary before claiming resolution. Do not merge this feature branch into main as part of this fix.

## Multi-provider AI Assistant settings (2026-10-13)

User request: offer popular model suggestions without preventing manual model ID entry, and show instructions for obtaining API credentials for each supported provider.

Implementation on this branch, independently committed:
- `d4b8bd55`: fixed HTTPS provider adapters for OpenAI, Anthropic Claude native Messages API, Google Gemini's OpenAI-compatible Chat Completions API, DeepSeek, GroqCloud and OpenRouter. No arbitrary endpoint URLs, no browser-side API requests, no AI shell execution.
- `ca459d85`: backend provider routing and encrypted API keys stored separately per workspace/provider; old OpenAI key secret ID retained. New provider field defaults to OpenAI for old settings. Custom model IDs accepted with validated characters (including slash for OpenRouter).
- `c1a4a1b7`: Settings dialog provider dropdown and editable HTML datalist of suggested model IDs, context-sensitive **How to get an API key** instructions, official key-generation and model-documentation HTTPS links. No live model discovery, so availability depends on provider account and can change.
- `69305894`, `18c89b53`: refresh suggestions with latest Claude Sonnet 5.5 API model ID and recommended defaults.
- `f8736eab`: repaired accidental source duplication from a string replacement while editing the Go model regex. This fix is required; do not attempt to activate any intermediate revision between `ca459d85` and `f8736eab`.
- `4fe035da`: provider defaults/endpoints/protocol/response parsing/secret isolation and manual model tests.
- `a5be0ba6`: full provider API key and model guidance in operator document.
- `c5b4d4fa`, `e550cc81`: Node regression for editable model suggestions, provider switching, official help links, and CI wiring.

Supported providers:
OpenAI (Chat Completions), Anthropic (native Messages), Google Gemini (OpenAI-compatibility), DeepSeek (OpenAI-compatible), GroqCloud (OpenAI-compatible), OpenRouter (OpenAI-compatible).

IMPORTANT limitations:
- Real provider credentials are intentionally absent; API request end-to-end against all six vendors requires user-provided keys and valid account billing.
- A suggested model identifier is not account entitlement; custom model IDs may fail if wrong or unsupported.
- Only the currently selected provider has `has_api_key` reflected in GET settings; switching provider does not overwrite other credentials.
- Provider endpoints stay fixed server-side for security; custom third-party base URLs not supported.
- L3 shell/SSH execution remains disabled; provider responses are suggestions only.

Follow-up verification: confirm newest CI Go 1.19/1.23 including `test_ai_model_suggestions.cjs` succeeds; manually open Settings → AI Assistant settings and switch providers, type a model ID, expand API help and verify links; submit a safe explain request with a test key. Do not merge to main until these tests and user acceptance.

- `9613ce29`: move per-provider API key creation help directly below the secret input in the Settings dialog for easier discovery. Multi-provider Go build and model suggestions tests have passed on intermediate revisions. Final workflow and real provider E2E still require verification.
