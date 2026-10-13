# TaskDeck Smart Terminal AI — Operator Guide & Architecture

Status: staged implementation in `feature/ai-smart-terminal-l1-l3`, not merged to main.
Updated 2026-10-13.

## Features delivered in this branch

### L1 — Suggest-only assistant (working code; real provider credentials required for manual E2E)
- Settings > **AI Assistant settings…** opens a dialog with enable, OpenAI model, write-only API key, timeout, max output bytes, per-user hourly request quota, consent, and context toggles.
- Settings > **Open AI Smart Terminal** opens a side panel with Explain, Suggest fix, Guided plan (preview) modes.
- OpenAI calls are **server-side**, over a fixed `https://api.openai.com/v1/chat/completions` URL. There is no arbitrary endpoint field or browser-side provider request.
- Keys use the existing TaskDeck `secretstore.Store`; settings are private per-workspace data in the TaskDeck user configuration directory (mode 0600), not under the Git repository. GET never includes the API key.
- Assistant starts **disabled**; enabling requires explicit provider consent.
- The user previews selected context and explicitly confirms each outbound request. The model returns **text**, never an executable tool call. Suggestions have a copy action but no Run button.
- Model errors, timeout and oversize responses return bounded errors without exposing the key or provider response body.
- Each authenticated user has one in-flight request and a configurable rolling one-hour request limit. For multiple TaskDeck daemon replicas, the quota is process-local and must be centralized before use.

### L2 — Explicit context selection and redaction
- Shell Integration already emits OSC 7/133 metadata, latest commands, exit statuses and CWD, subject to shell support.
- By default the assistant sends only the current user question, most recently recorded command and bounded output from the selected terminal. It does **not** read whole PTY scrollback, entire directories, files or database contents.
- Optional toggles: CWD, last eight command texts, Git summary visibly shown in UI.
- Context preview masks common tokens/password/key blocks; **Go backend masks again**, strips ANSI and truncates.
- Remote/SSH vs local is indicated. Changing active tab invalidates a stored guided-plan target.
- Logs and files can contain secrets in unexpected formats: the redactor is best-effort, not a data-loss prevention guarantee. Users must review preview carefully; do not enable external AI for secrets or production logs without separate security policy.
- Prompt injection is treated as untrusted terminal data and forbidden from overriding the assistant's system instruction, but model compliance is not a security boundary.

### L3 — Guided planning / approval only (partial)
- A Guided Plan prompt requests up to eight JSON command proposals with reasons and risk levels.
- **Settings > Guided AI plans (review only)…** opens a dialog. It can import the latest AI plan tied to the original terminal target or accept manually reviewed JSON.
- Each proposal is stored in a private mode-0600 plan journal, scoped to user ID and workspace, with immutable host, session ID, CWD and remote flag.
- Each individual step can be approved or rejected once; the decision and timestamps are persisted.
- Plan response and APIs explicitly return `execution_enabled:false`.
- **NO EXECUTION BROKER** exists in this branch. Approval records intent but never runs a command. SSH production, git, shell and database mutations are deliberately unavailable from the AI API.

## How to configure

1. Update TaskDeck to this branch only in a test environment; do not merge/activate in production until validation.
2. Open **Settings → AI Assistant settings…**. Enable AI Assistant, enter an OpenAI Chat Completions-capable model and supply a key. Check external-provider consent. Select the context flags only when needed. Save.
3. Confirm that GET `/api/ai/settings` returns `has_api_key:true` but no `api_key` value.
4. Select a local non-sensitive test terminal. Open **Settings → Open AI Smart Terminal**; type a question, click **Preview context**, read it, then click **Ask AI** and confirm the external request.
5. For a guided plan, select **Guided plan (preview only)** before requesting. Use **Guided AI plans (review only)…** to import, inspect and record per-step decisions. No commands execute.
6. Cancel the provider request from AI panel if it takes too long. Save settings with **Remove stored API key** to rotate/reset credentials.

## Security / deployment limitations

- Use HTTPS with a correctly configured reverse proxy and authentication on shared-server mode. Browser-to-TaskDeck PUT of API credentials must not travel over cleartext except loopback used for development.
- API access policy: `/api/ai/settings` uses settings.read/settings.write; `/api/ai/assist`, `/api/ai/plans` and `/api/ai/plans/decision` require terminal.view_own in multi-user shared mode.
- Plan journal additionally filters by `UserID` inside backend. **Real session ownership verification per target** is not implemented for arbitrary client-supplied context (safe because it is suggestion-only); must be added before any terminal tool execution.
- The provider endpoint and model compatibility can change. The current implementation supports **OpenAI Chat Completions only**, not arbitrary local/custom endpoints.
- No model streaming, token-cost meter, centralized rate limiter, offline inference or persistent agent run queue yet.
- No automatic file mutation, shell execution, SSH command, git mutation, DB write, deployment, self-update or rollback is controlled by AI.
- Provider key and submitted context must be treated as private; no key is committed or included in config backups.
- Prompt injection, imperfect secret scrubbing, external-model hallucinations and irreversible remote operations remain real risks.

## L3 execution gate — must pass before implementation is enabled

- Dedicated **execution principal**, immutable server-verified session owner/host/project/CWD and tool-specific permissions; never reuse browser admin shell credentials blindly.
- Limited tool registry (typed argument parsing, no shell interpolation), sandbox/container or isolated Git worktree, filesystem/network scope and explicit prod deny-by-default.
- Approval binds exact command arguments, target, revision, TTL, max steps/time/cost; changes invalidate consent.
- Durable journal with idempotency keys and no automatic retry after uncertain side effects. Cancellation, output capture, redaction, locks and audit.
- Cross-user, cross-session, forged approval, prompt-injection, SSRF, secret exfiltration, long process, service restart and malformed JSON tests.
- Manual security review + user acceptance. **Do not remove `execution_enabled:false` before this review.**

## Code map

- Backend: `internal/server/ai_assistant.go`, `ai_plans.go` and their tests
- UI: `web/featuremods/aiassistant.js`, `aiplans.js`, `menus.js`, `terminalpalette.js`
- Router/ACL: `internal/server/server.go`, `shared_authorization.go`
- Documentation: `AI_SMART_TERMINAL_PLAN.md`, `AI_SMART_TERMINAL_HANDOFF.md`

## Validation

Go 1.19 + 1.23 workflow on the feature branch runs JS syntax and server/UI regression tests. Provider end-to-end requires a user-configured credential and manual consent; CI intentionally does not call or bill any AI provider.
