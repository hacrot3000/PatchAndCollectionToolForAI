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

### F01 — Architecture and handoff [IN PROGRESS]
- [x] F01.1 Baseline inventory of existing Smart Terminal/AI Pack/Patch/remote components.
  - [x] F01.1.1 Record real code paths and no-regression boundaries in this document.
- [x] F01.2 Define phased plan, user flows, security invariants, entities and acceptance gates.
  - [x] F01.2.1 Decide stateful chain/conversation/stateless fallback contract.
- [ ] F01.3 Align exact native Patch Tool schema and remote execution protocol with this proposal.
  - [ ] F01.3.1 Pin proven parser/queue/execution APIs with source-line references; no duplicate parser.

### F02 — Model/provider settings & credentials [TODO]
- [ ] F02.1 Versioned provider/model config (custom HTTPS, provider ID, `apiType`, capabilities, reasoning support).
  - [ ] F02.1.1 Validate URL/pinning rules, model ID, token bounds and unsupported settings without runtime assumptions.
  - [ ] F02.1.2 Separate secrets from exportable model JSON; explicit credential lifecycle.
- [ ] F02.2 Native settings UI and test connection.
  - [ ] F02.2.1 Add/list/edit/remove providers, model reasoning, default selection; no plaintext key response.
  - [ ] F02.2.2 Probe text/stream/tool calling/stateful modes; never assume capability from declaration alone.
- [ ] F02.3 Provider adapters.
  - [ ] F02.3.1 Responses request/response format and reasoning parameter translation.
  - [ ] F02.3.2 Streaming SSE, cancellation, errors, rate limits and tool-call output parsing.
  - [ ] F02.3.3 Optional Chat Completions/Messages adapters (later).

### F03 — Durable sessions & context [TODO]
- [ ] F03.1 Independent multi-session store scoped by owner, workspace, project UUID and execution target.
  - [ ] F03.1.1 Atomic write/revision/size bound and session recovery after process restart.
  - [ ] F03.1.2 Session list/create/rename/archive/cancel APIs + UI tabs.
- [ ] F03.2 AI Pack first turn.
  - [ ] F03.2.1 Generate/validate bounded AI Pack snapshot on first user message and pin fingerprint/identity.
  - [ ] F03.2.2 Separate system instructions from untrusted pack/docs/logs; block cross-project context.
- [ ] F03.3 Stateful and stateless memory.
  - [ ] F03.3.1 Previous-response chaining; persist response ID only after successful append.
  - [ ] F03.3.2 Conversations API ID mode; explicit compatibility tests; fallback to canonical history.
  - [ ] F03.3.3 Canonical tool-call/results replay, context budget and compaction preserving commitments.

### F04 — Agent conversation & UI [TODO]
- [ ] F04.1 Streaming backend turn coordinator, per-session state machine and quotas.
  - [ ] F04.1.1 Append user+assistant messages transactionally; handle retry/timeouts/recovery.
  - [ ] F04.1.2 Stop/cancel/pause/resume and bounded loop iteration.
- [ ] F04.2 TaskDeck native Chat GUI.
  - [ ] F04.2.1 Multi-tab session list, model/reasoning selector, prompt attachments and history.
  - [ ] F04.2.2 Live status, per-command task subrows, console view/copy and AI final response.
  - [ ] F04.2.3 Recover UI after refresh/reconnect; no duplicate sends or actions.

### F05 — Tool Gateway and Patch Tool feedback loop [BLOCKED on F02/F03/F04]
- [ ] F05.1 Function tool schema `taskdeck_submit_patch` wrapping native patch text (never arbitrary shell).
  - [ ] F05.1.1 Validate exact patch schema, project UUID, scope, approval, source drift, expected HEAD.
  - [ ] F05.1.2 Idempotency ledger + durable queue binding before tool success acknowledgement.
- [ ] F05.2 Reuse actual Patch Tool queue / preflight / execution.
  - [ ] F05.2.1 Bind each patch/collection/command state to originating chat and tool call.
  - [ ] F05.2.2 Collect exit code/status/times, bounded console/diff/artifact refs, tool result.
- [ ] F05.3 AI feedback loop.
  - [ ] F05.3.1 Send `function_call_output` with matching call ID; resume model until final/limit.
  - [ ] F05.3.2 Reconcile unknown execution on disconnect; **never blindly rerun** a mutation.

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
  - [ ] F07.1.2 Provider egress/SSRF/redirect policy, local/private-IP safeguards and TLS.
- [ ] F07.2 Tests and documentation.
  - [ ] F07.2.1 Unit tests for provider formats, chaining, tool-call mapping, owner isolation and persistence.
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
| M0 handoff | This document with verified code map and statuses | IN PROGRESS |
| M1 protocol core | Stateful/stateless request building, parse response, tests | TODO |
| M2 session API + provider settings | Persisted concurrent sessions, BYOK secrets, capability testing | TODO |
| M3 chat GUI | Real messages with streaming, cancel and recovery | TODO |
| M4 Patch loop | Real queue run + tool results + idempotency + approvals | TODO |
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

## 7. First implementation change boundaries and handoff to next engineer

1. Create a small standard-library core package for Responses payloads and parsing; do not mix this with `ai_assistant.go` advisory handlers.
2. Add precise unit tests first, especially state reference vs stateless replay, mandatory instructions and malformed provider result.
3. Introduce provider settings and durable session APIs **only after** owner/auth checks are reviewed.
4. Connect actual `BuildAIPack` only when first-turn payload budget and secret scanning are defined; packaging a ZIP is not automatically a valid model input.
5. Wire `taskdeck_submit_patch` to a **real existing** queue with tool-call ledger; do not count the existing `/api/patch/paste` package creation as execution.
6. Update this document checkbox-by-checkbox after commit, tests and any actual live evidence. If only source is present and real-provider testing is absent, record both clearly.

## 8. Commands for verification

```sh
cd vscode_tasks_menu_go
go test ./internal/agentchat
go test ./internal/server ./internal/patchtool
go test ./...
node test_ai_smart_terminal.cjs
```

Run platform/remote smoke tests as available. External API requests incur cost: never perform paid provider calls or remote command execution merely to make tests green; use mocked HTTP servers and fake task executors until explicitly testing with real credentials.
