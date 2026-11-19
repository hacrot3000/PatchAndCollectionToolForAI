# TaskDeck AI Agent Chat — Architecture, Handoff & Implementation Plan

> Date: 2026-11-19. Target repository: `hacrot3000/PatchAndCollectionToolForAI`, branch `main`.
> Scope: native TaskDeck AI chat, BYOK/custom provider, AI Pack bootstrap, Responses context continuity, tool-driven Patch Tool feedback loop, parallel chat sessions.
> Status legend: `[x]` implemented in source (not a claim of real-world acceptance); `[ ]` not implemented; `BLOCKED` needs a prerequisite. Update checkboxes only with code + tests/evidence.
> **Do not confuse Smart Terminal advisory AI with an execution-capable Agent.** Existing advisory API must remain suggestion-only.

## 1. Product contract and invariants

1. A user configures one or more providers, API keys and model capabilities (model IDs and reasoning efforts like VS Code `chatLanguageModels.json`); credentials remain in TaskDeck's private Secret Store. Never log or embed keys in model configuration, AI Pack, Patch ZIP, tool results or exported sessions.
2. Multiple independent Chat Sessions can coexist. Each is bound to a verified project UUID/key, workspace, owner, provider, model and execution target. Never infer context from the active browser tab alone.
3. First message initializes the AI Pack *snapshot*, mandatory system/tool instructions, immutable project identity and initial user request. Treat repository files, collected logs and patch outputs as untrusted context, not higher-priority instructions.
4. Support three context modes, negotiated/tested per endpoint:
   - `previous_response_id` + persisted response ID when the endpoint actually supports chaining.
   - `conversation` + persisted conversation ID when Conversations API is supported.
   - `stateless` sends the complete relevant history (incl. tool-call/output items) and the next input.
   Store canonical history locally **in every mode** for recovery/audit/provider switching. Do not assume every `apiType=responses` endpoint implements server-side state. Refresh required top-level instructions: they are not automatically inherited in `previous_response_id` calls. Historical tokens can still be billed in stateful mode.
5. Agent model returns *tool calls*, never trusted shell access. `taskdeck_submit_patch` accepts the existing native patch payload exactly; Tool Gateway validates schema, project identity, authorization, workspace lease, expected Git HEAD, idempotency and approval policy, then hands off to the **existing** Patch Tool queue/parser/executor. Do not implement another patch execution path.
6. Every execution emits `QUEUE`, `RUNNING`, `DONE`, `ERROR` or `CANCELLED` along with timing, command/script subitems, console view/copy actions and persistent artifact references. Collect bounded logs/diffs/results to return as `function_call_output` with the *same* `call_id`.
7. Remote execution reuses the existing TaskDeck E2EE dispatcher and server-enforced permissions. Remote hosts never need an AI key; sending context to an external model is a separate security/consent boundary from TaskDeck E2EE.
8. Enforce per-session and global limits for model calls, run iterations, wall time, tokens/cost, log bytes, payload bytes and workspace writes. Stop and surface a recoverable error on timeout or ambiguous completion; do not blindly replay a side-effecting tool call.
9. **Future only**: plan.md → validated Task Manifest → dependency DAG → scheduler with configurable concurrency → isolated Agent Sessions/worktrees → controlled integration. Do **not** implement planner/task splitting yet.

## 2. Existing codebase / do-not-regress map (verified on 2026-11-19)

| Component | Current source | Integration constraint |
|---|---|---|
| Existing Smart Terminal AI | `vscode_tasks_menu_go/internal/server/ai_assistant.go`, `ai_providers.go`, `ai_session_context.go`, `ai_plans.go` | Deliberately suggestion-only; separate namespace and no implicit execution escalation |
| AI credential storage | `internal/server/ai_assistant.go` and `internal/secretstore` | Reuse secure store; migrate/alias existing keys explicitly if needed |
| AI Pack | `internal/patchtool/ai_pack.go`, `internal/server/patch_ai_pack.go` | `BuildAIPack` produces project-bound snapshot / prompt / fingerprint |
| Patch ingestion | `internal/server/patch_paste.go`, `patch_upload.go`, `internal/patchtool/runtime.go` | Existing paste is queue packaging only; not proof that patch was executed |
| Task/console | `internal/server/task_run_history.go`, `internal/server/server.go`, Patch panel | Reuse execution IDs, logs and polling/streaming; never simulate DONE |
| Auth and browser controls | `internal/server/server.go`: sharedAuth, sharedAuthorize, browser lease, same-origin mutations | New agent APIs must be covered and reauthorize every side-effect |
| Remote | `internal/server/remote_access_dispatch.go`, `remote_access_task_run.go` | E2EE endpoint; never bypass dispatcher or remote permission checks |
| Existing UI | `web/featuremods/aiassistant.js`, `patchpanel.js` | New Agent Chat is distinct from advisory Smart Terminal |

### Key technical hazard / handoff
`BuildAIPack` creates a ZIP and accompanying prompt/text file with a potentially large payload. The initial AI request should use a **validated, bounded representation** of the pack, pinned to fingerprint + project identity; do not blindly inline binary/ZIP bytes. Context compaction must preserve immutable system/tool contract, project identity, pending tool calls and newest source changes. Redaction is necessary before egress but not a guarantee that every secret has been detected.

## 3. Target architecture

```text
TaskDeck UI
  ├── Agent Chat tabs / model settings / execution console
  └── native REST/SSE (CSRF + auth + owner boundary)
          ↓
Agent Session Manager (durable transcript, request state, cancellation)
          ↓
Context Manager (AI Pack first turn, project rules, delta, compaction)
          ↓
Provider Adapter (Responses first; provider capability checks)
          ↕
AI provider (custom HTTPS endpoint or built-in)
          ↓ function_call
Tool Gateway (normalize → validate → authorize → idempotency → approve)
          ↓
Existing Patch Tool queue → execution → result collector
          ↓ function_call_output (call_id preserved)
Agent continues until final answer, approval wait, failure or quota
```

Persistence entities (versioned): `ProviderConfig`, `ModelConfig`, `AgentSession`, `AgentMessage`, `AgentRun`, `AgentToolCall`, `ExecutionLink`, `WorkspaceLease`, `ContextSnapshot`. All IDs must be cryptographically random or server-generated. Store only reference + bounded/redacted context where possible. Use 0600 files / private database; avoid state in browser storage.

### Suggested HTTP routes (future implementation subject to auth review)
- `GET/PUT /api/agent/providers`, `POST /api/agent/providers/test` (write-only secret; allowlisted destination policy)
- `GET/POST /api/agent/sessions`, `GET/DELETE /api/agent/sessions/{id}`
- `POST /api/agent/sessions/{id}/messages` (idempotency + streaming event ID)
- `POST /api/agent/sessions/{id}/cancel`, `GET /api/agent/sessions/{id}/events`
- `POST /api/agent/tool-calls/{id}/decision`, `GET /api/agent/executions/{id}`
These are **design targets**, not promises that routes already exist.

### Canonical state and invariants
- Session: `CREATED → INITIALIZING → READY → REQUESTING_AI → WAITING_TOOL/WAITING_APPROVAL → TOOL_RUNNING → SUBMITTING_RESULT → REQUESTING_AI → COMPLETED`; exceptional `PAUSED/FAILED/CANCELLED/RECOVERING`.
- Tool-call ledger: `RECEIVED → VALIDATED → APPROVAL_PENDING/QUEUED → RUNNING → DONE/ERROR/CANCELLED/UNKNOWN`. Durable unique key `(session_id, model_turn_id, call_id)`; preserve run ID across retries.
- Workspace: concurrent reads allowed, **one writer per actual workspace**; eventually worktree per parallel writing task. Re-check Git HEAD and approval before side effects.
- Provider request: bounded timeout, response size, redirect refusal, SSRF/DNS rebinding defense, cancel and consistent error surface; never submit a stale tool result after session cancellation.

## 4. Features & subtask checklist (max 3 levels)

### F01 — Architecture and handoff [IN PROGRESS: F01.3 outstanding]
- [x] F01.1 Baseline inventory of existing Smart Terminal/AI Pack/Patch/remote components.
  - [x] F01.1.1 Record real code paths and no-regression boundaries in this document.
- [x] F01.2 Define phased plan, user flows, security invariants, entities and acceptance gates.
  - [x] F01.2.1 Decide stateful chain/conversation/stateless fallback contract.
- [ ] F01.3 Align exact native Patch Tool schema and remote execution protocol with this proposal.
  - [ ] F01.3.1 Pin proven parser/queue/execution APIs with source-line references; no duplicate parser.

### F02 — Model/provider settings & credentials [IN PROGRESS: core delivered, delete/key revoke and capability probes outstanding]
- [x] F02.1 Versioned provider/model config (custom HTTPS, provider ID, `apiType`, capabilities, reasoning support) — Responses-only schema v1.
  - [x] F02.1.1 Validate public HTTPS URL, DNS/public IP at connect, model ID, token bounds and unsupported settings; tested without live egress.
  - [ ] F02.1.2 Separate secrets from exportable model JSON; explicit credential lifecycle.
- [x] F02.2 Native settings UI and basic Responses text connection test (stream/tools/statefulness not yet probed).
  - [ ] F02.2.1 Add/list/edit/remove providers, model reasoning, default selection; no plaintext key response.
  - [ ] F02.2.2 Probe text/stream/tool calling/stateful modes; never assume capability from declaration alone.
- [ ] F02.3 Provider adapters.
  - [x] F02.3.1 Responses request/response format and default reasoning-effort parameter for configured model.
  - [ ] F02.3.2 Streaming SSE, cancellation, errors, rate limits and tool-call output parsing.
  - [ ] F02.3.3 Optional Chat Completions/Messages adapters (later).

### F03 — Durable sessions & context [IN PROGRESS]
- [ ] F03.1 Independent multi-session store scoped by owner, workspace, project UUID and execution target.
  - [x] F03.1.1 Atomic write/revision/size bound and session recovery after process restart (owner-bound private Go journal + tests).
  - [ ] F03.1.2 Session list/create/rename/archive/cancel APIs + UI tabs.
- [ ] F03.2 AI Pack first turn.
  - [x] F03.2.1 Generate/validate bounded first-turn AI Pack text snapshot; pin fingerprint/project ID; mocked integration test.
  - [x] F03.2.2 Separate mandatory system instructions from untrusted AI Pack body; validate project UUID before egress.
- [ ] F03.3 Stateful and stateless memory.
  - [x] F03.3.1 Previous-response chaining; persist response ID alongside completed assistant message (mocked two-turn test).
  - [ ] F03.3.2 Conversations API ID mode; explicit compatibility tests; fallback to canonical history.
  - [ ] F03.3.3 Canonical tool-call/results replay, context budget and compaction preserving commitments.

### F04 — Agent conversation & UI [IN PROGRESS: synchronous chat exists; streaming, resume and execution UI pending]
- [ ] F04.1 Streaming backend turn coordinator, per-session state machine and quotas.
  - [ ] F04.1.1 Append user+assistant messages transactionally; handle retry/timeouts/recovery.
  - [ ] F04.1.2 Stop/cancel/pause/resume and bounded loop iteration.
- [ ] F04.2 TaskDeck native Chat GUI.
  - [ ] F04.2.1 Multi-tab session list, model/reasoning selector, prompt attachments and history.
  - [ ] F04.2.2 Live status, per-command task subrows, console view/copy and AI final response.
  - [ ] F04.2.3 Recover UI after refresh/reconnect; no duplicate sends or actions.

### F05 — Tool Gateway and Patch Tool feedback loop [IN PROGRESS: native path implemented, live validation / automatic recovery outstanding]
- [x] F05.1 Function tool schema `taskdeck_submit_patch` with strict JSON name/patch_python; human approval required; no arbitrary shell tool.
  - [ ] F05.1.1 Validate exact patch schema, project UUID, scope, approval, source drift, expected HEAD.
  - [ ] F05.1.2 Idempotency ledger + durable queue binding before tool success acknowledgement.
- [x] F05.2 Wire approved patches to existing Patch Paste queue and native Patch Session / prompt_response handlers; real runtime acceptance remains unverified.
  - [x] F05.2.1 Persist link from originating AI call_id to queued package SHA and native Patch Session ID (PATCH only; COLLECT future).
  - [ ] F05.2.2 Collect exit code/status/times, bounded console/diff/artifact refs, tool result.
- [ ] F05.3 Fully automatic AI feedback loop across long-running jobs/restarts (manual-confirmed continuation implemented, unattended recovery still pending).
  - [x] F05.3.1 Send evidence-backed `function_call_output` using exact call_id in a once-only opt-in continuation endpoint; further automatic scheduling pending.
  - [x] F05.3.2 Persist QUEUING/UNKNOWN transitions before side effects; no automatic patch resubmission; manual recovery of ambiguous runs remains pending.

### F06 — Parallel sessions & remote [BLOCKED on F03/F05]
- [ ] F06.1 Independently running sessions with separate histories, quotas, cancellation tokens.
  - [ ] F06.1.1 Configurable max session/model/execution concurrency; queue backpressure.
  - [ ] F06.1.2 Workspace write lease and branch/HEAD optimistic check.
- [ ] F06.2 Bind remote target to authorized E2EE dispatcher, log and cancel.
  - [ ] F06.2.1 Validate remote scope per action, never forward API keys.
  - [ ] F06.2.2 Recover state after remote or GUI disconnection.

### F07 — Security/validation/operability [TODO]
- [ ] F07.1 Auth, owner/project isolation and approvals for every new route and tool.
  - [ ] F07.1.1 Fail closed on unknown capability or untrusted patch metadata; never accept AI's `approved` flag.
  - [x] F07.1.2 Provider egress/SSRF/redirect policy, local/private-IP safeguards and TLS code; independent security audit remains pending.
- [ ] F07.2 Tests and documentation.
  - [x] F07.2.1 Unit tests for initial Responses request/parse, chaining, tool-call mapping, owner isolation and persistence; additional provider formats remain F02.3.3.
  - [ ] F07.2.2 Integration tests for native Patch run/retry/cancel, concurrent sessions and remote E2EE.
  - [ ] F07.2.3 CI/GUI smoke acceptance with zero regressions to suggestion-only Smart Terminal.

### F08 — Future task planner (NOT IMPLEMENTING NOW)
- [ ] F08.1 Markdown → validated Task Manifest (dependencies, acceptance criteria, resources, task IDs).
  - [ ] F08.1.1 Schema validation, dependency cycle detection, bounded number of tasks.
- [ ] F08.2 Dependency DAG Scheduler and configurable concurrency.
  - [ ] F08.2.1 Worktree/task isolation and controlled merge strategy.
  - [ ] F08.2.2 Integration reports, retry/resume, explicit approval for branch mutation.

## 5. Incremental delivery gates

| Milestone | Evidence required | Status |
|---|---|---|
| M0 handoff | This document with verified code map and statuses | DONE (source) |
| M1 protocol core | Stateful/stateless request building, parse response, tests | DONE (code + CI) |
| M2 session API + provider settings | Persisted sessions, BYOK secrets, capability testing | IN PROGRESS (BYOK and basic probe done; advanced capability tests/revoke pending) |
| M3 chat GUI | Real messages with streaming, cancel and recovery | IN PROGRESS (synchronous chat + GUI done; streaming/recovery pending) |
| M4 Patch loop | Real queue run + tool results + idempotency + approvals | IN PROGRESS (native code and mocked test PASS; no real runtime/live end-to-end or unattended feedback) |
| M5 remote & multi-session | E2EE live tests, lock safety, session isolation | TODO |
| M6 release acceptance | Regression suite, real provider smoke, security review | TODO |
| M7 planner | Future; intentionally deferred | DEFERRED |

## 6. Acceptance matrix

- A1: First request contains project-bound AI Pack + explicit system/tool policy; subsequent stateful request sends only delta **but refreshes required instructions**.
- A2: A custom Responses-compatible endpoint with NO previous-response persistence passes stateless fallback test.
- A3: Two sessions can use different models/targets without sharing history or credentials.
- A4: A provider timeout after patch execution does not run the same command twice.
- A5: Every patch command shows queued/running/final state with expandable console and copy action from chat and task run.
- A6: Changing branch/HEAD/project UUID between proposal and execution fails closed.
- A7: Remote disconnect/restart produces recoverable UNKNOWN/FAILED state rather than false DONE.
- A8: No secret or raw private key appears in provider payload/log/export; explicit opt-in egress.
- A9: Existing Smart Terminal suggestion-only behavior and Patch Tool manual behavior remain intact.
- A10: After reopening GUI or restarting backend, session history and execution identities are recoverable.

## 7. Implementation log and verified evidence (2026-11-19; updated)

**Implemented on `main` (code-level scope):**
- `internal/agentchat/responses.go`, `responses_test.go`: canonical context modes `stateless`, `previous_response`, `conversation` (conversation ID reuse only); tool-call/result encoding/parsing, mandatory instructions every request.
- `internal/agentchat/store.go`, `store_test.go`: owner/project-bound 0600 canonical history, random session IDs, CAS revision guard, atomic saves, restart recovery.
- `internal/agentchat/providers.go`, `providers_test.go`: provider/model schema v1, public HTTPS validation, DNS resolution public-IP checks on each connection, no proxy/redirect, bounded Responses HTTP request, owner-scoped metadata store. Agent keys are **not** stored in provider JSON.
- `internal/server/agent_chat_providers.go`, `agent_chat_providers_test.go`: `GET/PUT /api/agent/providers` and opt-in `POST /api/agent/providers/test`. Credentials go through existing encrypted `ConnectionSecrets` with separate owner/workspace/provider keys. The probe tests **only basic Responses text**, not tool calling, streaming, stateful history or reasoning capabilities.
- `internal/server/agent_chat_sessions.go`, `agent_chat_sessions_test.go`: `GET/POST /api/agent/sessions`, `GET /api/agent/sessions/{id}`; explicit custom provider/model selection and project UUID binding. Existing advisory settings remain backward-compatible, but legacy advisory provider keys do not automatically grant Agent Chat capabilities.
- `internal/server/agent_chat_messages.go`, `agent_chat_messages_test.go`: `POST /api/agent/sessions/{id}/messages`; explicit external egress consent, per-session in-flight lock, revision compare-and-swap, AI Pack on first turn with size bound and fingerprint, mandatory system instructions, persisted user+assistant history, previous response chaining, tool-call recognition with `waiting_tool` status and **zero execution**. Error leaves a durable failed session; automatic retry deliberately disabled.
- `internal/server/server.go`, `shared_authorization.go`: routing and shared-server RBAC, including `files.read` before external project-context submission.
- `web/featuremods/agentchat.js`, `next.js`, `menus.js`, `internal/server/agent_chat_ui_test.go`: integrated native UI under Settings → AI Assistant: Open AI Agent Chat / AI Agent providers; configure model/key, create/select sessions, show transcript, send/review external message, cancel active browser request, no command execution.
- `.github/workflows/taskdeck_agentchat_core.yml`: Go race/unit/vet, server Agent Chat integration tests and `node --check` for new UI.

**Latest CI:** [Agent Chat protocol core run 44464121465](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/44464121465) — PASS. Includes Go race tests, provider/session/message mock tests, server RBAC/UI tests and JavaScript syntax check. These tests do not count as a live custom-endpoint or browser acceptance test.

**Known limitations / must not overclaim:**
1. Patch Tool execution/auto feedback is **NOT implemented**. Function calls received from a provider are stored as `waiting_tool`; they are not executed. No `taskdeck_submit_patch` tool is yet advertised to the model.
2. Basic Chat is synchronous JSON only: SSE streaming, pause/resume, cross-daemon locks, request idempotency, retry/recovery UI, session rename/delete and log console integration are incomplete.
3. Conversations mode falls back to canonical stateless replay until a verified Conversations API create/ID negotiation path exists. `previous_response_id` mode assumes the provider supports chaining when selected; automatic capability negotiation/fallback on provider error is not implemented yet.
4. The first AI Pack request sends a bounded/redacted text companion, not a ZIP. Server rejects oversize packs instead of silently truncating the tool contract. Automatic context compaction and an AI Pack preview/advanced egress policy are not yet implemented.
5. Metadata editing and key creation/rotation exist, but delete provider, revoke API key, model discovery, pricing/token accounting and provider-specific reasoning validation require further work.
6. Current browser UI only demonstrates basic chat and multi-session switching. Real endpoint/network smoke, browser usability and remote E2EE acceptance remain untested.

**Next ordered implementation:** (A) precise idempotent Tool Gateway + Patch Tool execution binding, (B) output/result collector and auto feedback loop, (C) SSE/context recovery + advanced provider probes, (D) remote/writer locks and task console, (E) release hardening. Keep future plan DAG deferred.

### Implementation continuation — native Tool Gateway (2026-11-19)

Committed source:
- `internal/agentchat/responses.go`: opt-in Responses function tool `taskdeck_submit_patch` with strict `name` and `patch_python` parameters; parallel tool calls disabled.
- `internal/agentchat/store.go`: durable `ToolCallRecord` with call ID, native queue path, package SHA256, Patch Session ID, lifecycle status, and once-only `feedback_sent` marker.
- `internal/server/agent_chat_tools.go`: strict arguments, project key/ID header validation through **existing** `pastedPythonPackage`, explicit per-call approval/reject, idempotency state persisted **before** queue side effects. Invokes existing `patchPaste` handler, not a new Python executor.
- `internal/server/agent_chat_native.go`: launch **existing** `sessionsRoot(kind=patch, native queue)` handler, then select the exact approved package using **existing** `sessionItem(prompt-response)`; refuses stale/ambiguous prompts and duplicates, verifies staged package SHA. Uses native per-command `ProtocolState.Items` PASS/FAIL/PREFLIGHT_FAIL/INCOMPLETE only, strips command argv/private log paths from model feedback.
- `internal/server/agent_chat_sync.go`: explicit `POST /api/agent/sessions/{id}/sync` reconciles authoritative native status into the durable ledger; never derives success solely from process exit.
- `internal/server/agent_chat_tool_feedback.go`: `POST /api/agent/sessions/{id}/tools/{call_id}/continue` sends verified result as matching `function_call_output` and records it before contacting provider; a retry cannot re-send it silently.
- `web/featuremods/agentchat.js`: review Python source, approve/reject, show queue and native Patch Session ID, poll result while Chat is open, send result to AI using explicit consent. Does NOT expose an arbitrary shell command route.
- `internal/server/agent_chat_tools_test.go`, `agent_chat_native_test.go`, `agent_chat_sync_test.go`, UI/RBAC tests: native queue integration (without live execution), duplicate approval rejection, wrong project identity, exact package selection, protocol result truthfulness, filtered output and once-only AI tool feedback.

**CI evidence:** [Run 44476766966](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/44476766966) — PASS on commit `2d6271c`, Go race tests/core, server Agent tests, JavaScript syntax check. No production endpoint credentials or live Python Patch runtime were executed in these tests.

**Important outstanding work:** GUI-only polling is not an unattended server-side agent loop; after closing the browser no automatic model continuation occurs. A finished job requires the user to reopen the session and explicitly submit the verified result to AI. Real native Patch run, remote E2EE, workspace write leases/Git HEAD check, process restart reconciliation, streaming, provider fallback, artifact console previews, full regression testing, error recovery and security hardening remain open. **Code implemented ≠ live integration verified.** Do not mark unattended auto-run or end-to-end autonomous Agent as DONE.

## 8. First implementation change boundaries and handoff to next engineer

1. Create a small standard-library core package for Responses payloads and parsing; do not mix this with `ai_assistant.go` advisory handlers.
2. Add precise unit tests first, especially state reference vs stateless replay, mandatory instructions and malformed provider result.
3. Introduce provider settings and durable session APIs **only after** owner/auth checks are reviewed.
4. Connect actual `BuildAIPack` only when first-turn payload budget and secret scanning are defined; packaging a ZIP is not automatically a valid model input.
5. Wire `taskdeck_submit_patch` to a **real existing** queue with tool-call ledger; do not count the existing `/api/patch/paste` package creation as execution.
6. Update this document checkbox-by-checkbox after commit, tests and any actual live evidence. If only source is present and real-provider testing is absent, record both clearly.

## 9. Commands for verification

```sh
cd vscode_tasks_menu_go
go test ./internal/agentchat
go test ./internal/server ./internal/patchtool
go test ./...
node test_ai_smart_terminal.cjs
```

Run platform/remote smoke tests as available. External API requests incur cost: never perform paid provider calls or remote command execution merely to make tests green; use mocked HTTP servers and fake task executors until explicitly testing with real credentials.
