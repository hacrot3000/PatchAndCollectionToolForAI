# TaskDeck Smart Terminal AI — Operator Guide & Architecture

Status: staged implementation in `feature/ai-smart-terminal-l1-l3`, not merged to main.
Updated 2026-10-13.

## AI provider and API key setup

Open **Settings → AI Assistant settings…** and choose **AI provider**. The **Model (choose or type)** field offers popular model IDs from that provider, while allowing any valid provider-specific model ID (for new releases or models enabled on your account). A suggestion is not a guarantee that your key can access that model.

Select **How to get an API key · Model list** to see concise instructions and two official links: **Open API key page** and **Official models**. Each provider uses its own fixed HTTPS endpoint; arbitrary base URLs are intentionally not accepted (to prevent credential leakage through SSRF).

| Provider | Suggested model examples | Get an API key | Official model reference |
| --- | --- | --- | --- |
| OpenAI | `gpt-5.4-mini`, `gpt-5.4`, `gpt-4.1-mini` | https://platform.openai.com/api-keys | https://developers.openai.com/api/docs/models |
| Claude (Anthropic) | `claude-sonnet-5-5`, `claude-haiku-5-5`, `claude-opus-5-5` | https://console.anthropic.com/settings/keys | https://platform.claude.com/docs/en/models/overview |
| Gemini (Google AI Studio) | `gemini-3.8-flash`, `gemini-3.5-flash-lite` | https://aistudio.google.com/api-keys | https://ai.google.dev/gemini-api/docs/models |
| DeepSeek | `deepseek-chat`, `deepseek-reasoner` | https://platform.deepseek.com/api_keys | https://api-docs.deepseek.com/ |
| GroqCloud | `llama-3.3-70b-versatile`, `openai/gpt-oss-120b` | https://console.groq.com/keys | https://console.groq.com/docs/models |
| OpenRouter | `openai/gpt-5.4-mini`, `anthropic/claude-sonnet-5-5` | https://openrouter.ai/settings/keys | https://openrouter.ai/models |

After creating a key, paste it into the write-only **API key** field, choose a model, and click **Save settings**. The backend stores credentials in the existing encrypted secret store, **separately by provider**; switching provider never deletes another provider's key. Older OpenAI credentials remain recognized under their original secret ID. The GET settings API never returns the credential, only `has_api_key` for the active provider. A newly created key might require billing/credits or model access to be enabled in the provider console. Provider API usage and consumer chat subscriptions are normally billed separately.

Backend protocol: OpenAI, Gemini, DeepSeek, Groq and OpenRouter use their documented Chat Completions-compatible endpoints. Anthropic uses its native Messages endpoint (`anthropic-version` and `x-api-key` header). Provider request formats and answer parsing are separately tested. The browser never calls providers directly. Provider support does not enable shell execution: L1 remains suggestions only, L3 execution remains disabled.



## Features delivered in this branch

### L1 — Suggest-only assistant (working code; real provider credentials required for manual E2E)
- Settings > **AI Assistant settings…** opens a dialog with enable, OpenAI model, write-only API key, timeout, max output bytes, per-user hourly request quota, consent, and context toggles.
- Settings > **Open AI Smart Terminal** opens a side panel with Explain, Suggest fix, Guided plan (preview) modes.
- AI calls are **server-side**, routed through a fixed HTTPS endpoint for the selected supported provider (OpenAI, Anthropic, Gemini, DeepSeek, Groq or OpenRouter). There is no arbitrary endpoint field or browser-side provider request.
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

## Temporary Request Debug panel

The AI Smart Terminal panel now has a **Request Debug · temporary** section with **Copy debug report** and **Clear**. It is intended for user-assisted troubleshooting, not automatic telemetry. For each context preview and provider request it captures: UTC timestamp, selected view/session identity, provider/model, sanitized request fields and terminal output, verification/progress stages, response/error/HTTP status, and success metadata. The debug report is kept **in memory in this browser tab** only.

If server session verification fails, the panel also probes the user's authorized `GET /api/sessions` listing and records the visible session count, whether the requested ID exists, and (if visible) the matching session's kind/status/task ID. It deliberately does **not** include other session contents or other users' session IDs. This helps distinguish stale tabs after TaskDeck restarts from the legacy session-kind mismatch.

A legacy single-user interactive terminal can have an empty `session.Metadata.Kind` while still being valid. `ai_target.go` now permits only such single-user sessions with `task_id=0` and a known `local`/`ssh` target. Multi-user authorization remains strict; Patch Tool/task sessions are rejected. HTTP 404 means the session is absent; HTTP 409 means the session exists but is not an eligible interactive terminal or has changed state.

The button copies a readable JSON report with common tokens/passwords/private keys automatically masked. **Redaction is best-effort**: users must review the copied text, especially command output, before sharing. No API credentials from Settings or provider Authorization headers are recorded. Request Debug must not be used for automatic server-side log retention without a separate privacy/security review.

## Security / deployment limitations

- Use HTTPS with a correctly configured reverse proxy and authentication on shared-server mode. Browser-to-TaskDeck PUT of API credentials must not travel over cleartext except loopback used for development.
- API access policy: `/api/ai/settings` uses settings.read/settings.write; `/api/ai/assist`, `/api/ai/plans` and `/api/ai/plans/decision` require both terminal.view_own and settings.read in multi-user shared mode.
- Plan journal additionally filters by `UserID` inside backend. **Real session ownership verification per target** is not implemented for arbitrary client-supplied context (safe because it is suggestion-only); must be added before any terminal tool execution.
- The provider endpoint and model compatibility can change. The current implementation supports **OpenAI Chat Completions only**, not arbitrary local/custom endpoints.
- No model streaming, token-cost meter, centralized rate limiter, offline inference or persistent agent run queue yet.
- Model suggestion lists are curated shortcuts, not live account-specific discovery; custom model strings must match the selected provider's own naming, API support and account entitlements.
- No automatic file mutation, shell execution, SSH command, git mutation, DB write, deployment, self-update or rollback is controlled by AI.
- Provider key and submitted context must be treated as private; no key is committed or included in config backups.
- Prompt injection, imperfect secret scrubbing, external-model hallucinations and irreversible remote operations remain real risks.

## Provider request diagnostics and waiting-state handling (2026-10-14)

The Gemini debug capture with `model=gemini-3.8-flash`, `status=in_progress`,
`stage=provider_request_started`, `response=null` establishes **only that
terminal verification passed and a provider request began**. It does not
establish a provider timeout or an error. Google's Gemini documentation lists
`gemini-3.8-flash` as a valid model and supports the OpenAI-compatible
`https://generativelanguage.googleapis.com/v1beta/openai/chat/completions`
endpoint: https://ai.google.dev/gemini-api/docs/openai.

Changes made to make the next capture actionable:

- **Request Debug** now includes `elapsed_ms`, configured `timeout_seconds`,
  `provider_started_at`, and `expected_timeout_at`. While a provider
  request is in progress, copying the report recomputes elapsed time.
  Browser UI refreshes the timer while visible.
- Session verification without a provider call finishes with `status=ready`.
  Starting the provider call clears the earlier verification response and
  changes status to `in_progress`. The final stage becomes
  `success`, `failed`, or `cancelled`.
- Successful calls include upstream `provider_http_status=200` and
  `provider_duration_ms`; failures preserve TaskDeck HTTP status, upstream
  provider status (e.g. Gemini `429`), a restricted safe error code (e.g.
  `RESOURCE_EXHAUSTED`), and elapsed duration.
- The Go backend **never forwards raw provider error text**, because it may
  contain credentials or echoed terminal content. Known HTTP statuses receive
  safe summaries (401 invalid key, 403 access denied, 404 unavailable model,
  429 rate/quota, 5xx service error). Transport timeout returns TaskDeck HTTP
  504 with error code `provider_timeout`; other network errors return 502.
- If the browser has not received the server response 15 seconds **after**
  the configured server timeout, it aborts the client request and reports
  `client_timeout`. Manual Cancel produces a cancelled report. No automatic
  provider retry is performed (avoid duplicate billing).
- Default timeout for new configurations is 45 seconds; Settings accepts
  5–180 seconds. Existing saved timeout values are preserved. Gemini thinking
  responses may need a longer timeout depending on context.
- Blank Question is replaced by a contextual explain/suggest/plan prompt;
  backend enforces a default too. While waiting for provider, Ask AI is
  disabled to prevent overlapping requests.
- The Copy Debug report remains in-memory, redacted on a best-effort basis
  and should be reviewed before sharing.

**Next manual repro:** Use a non-sensitive terminal, open AI Assistant,
choose Gemini / `gemini-3.8-flash`, check that API key is configured,
ask a short question, and wait until the debug status is no longer
`in_progress`. If it fails, send the copied report containing both
`http_status` and `provider_http_status`. A provider `404` may reflect
account/model availability despite the model name being valid. Functional
external API calls have not been tested with private credentials in CI.

## Security hardening checkpoint: server-verified sessions and dry-run typed tools

The feature branch now has three additional backend enforcement layers:

1. **Live terminal attestation:** `ai_target.go` checks session existence, terminal kind, running status, owner/project visibility, and broker capability (shared mode). It derives host, SSH flag and initial launch CWD from the server's `session.Metadata`, ignoring client-supplied labels. Advisory requests must include a valid `session_id`; cross-user and stale sessions are rejected before any provider call. `GET /api/ai/session-context?session_id=...` returns only safe, non-sensitive metadata for the preview.
2. **Approval fingerprint and expiry:** every newly saved plan contains a SHA-256 digest over exact normalized step text/reasons/risk, verified target, owner, ID and creation time, plus an approval deadline 30 minutes after creation. Each Approve/Reject request must present the matching digest. The server recalculates it from the saved plan and refuses expired, tampered, or changed-target plans. These approvals remain **non-executable records**.
3. **Conservative risk classification and candidate inspection:** model-provided read-only labels are never trusted for unknown commands. Only exact, argument-free diagnostics (for example `pwd`, `git status --short`, `git diff --check`) are recognized as future typed-tool candidates. `POST /api/ai/plans/validate` reviews each step without running anything. Remote/SSH targets cannot qualify. UI has **Check read-only candidates** for this read-only review.

**Important:** server launch CWD is not necessarily the current interactive shell CWD after `cd`; even after attestation, terminal output and history remain untrusted. The provider preview reports this distinction. The classifier is not a security parser or authorization to execute. A future sandboxed tool runner must enforce permissions and isolation independently.

## L3 execution gate — must pass before implementation is enabled

- Dedicated **execution principal**, immutable server-verified session owner/host/project/CWD and tool-specific permissions; never reuse browser admin shell credentials blindly.
- Limited tool registry (typed argument parsing, no shell interpolation), sandbox/container or isolated Git worktree, filesystem/network scope and explicit prod deny-by-default.
- Approval binds exact command arguments, target, revision, TTL, max steps/time/cost; changes invalidate consent.
- Durable journal with idempotency keys and no automatic retry after uncertain side effects. Cancellation, output capture, redaction, locks and audit.
- Cross-user, cross-session, forged approval, prompt-injection, SSRF, secret exfiltration, long process, service restart and malformed JSON tests.
- Manual security review + user acceptance. **Do not remove `execution_enabled:false` before this review.**

## Code map

- Backend: `internal/server/ai_assistant.go`, `ai_target.go`, `ai_session_context.go`, `ai_plans.go`, `ai_plan_validate.go`, `ai_risk.go` and tests
- UI: `web/featuremods/aiassistant.js`, `aiplans.js`, `menus.js`, `terminalpalette.js`
- Router/ACL: `internal/server/server.go`, `shared_authorization.go`
- Documentation: `AI_SMART_TERMINAL_PLAN.md`, `AI_SMART_TERMINAL_HANDOFF.md`

## Validation

Go 1.19 + 1.23 workflow on the feature branch runs JS syntax and server/UI regression tests. Provider end-to-end requires a user-configured credential and manual consent; CI intentionally does not call or bill any AI provider.
