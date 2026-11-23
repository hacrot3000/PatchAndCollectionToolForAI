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
9. **Planning implemented; execution in progress**: F08.1 Markdown/canonical JSON → validated Task Manifest and read-only preview is implemented. F08.2 now has uncommitted manifest Run All/DAG orchestration, owner-bound run/provider routing, detached worktrees, task review, and fake-native Patch continuation. Real Python/provider acceptance, browser acceptance and controlled owner-branch integration remain pending; keep F08.2 open.

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

### F01 — Architecture and handoff [DONE (source): contract pinned; remote Patch has no operation] — [Tài liệu triển khai](taskdeck_ai_agent/01-architecture.md)
- [x] F01.1 Baseline inventory of existing Smart Terminal/AI Pack/Patch/remote components. — [Tài liệu triển khai](taskdeck_ai_agent/01-architecture.md)
  - [x] F01.1.1 Record real code paths and no-regression boundaries in this document. — [Tài liệu triển khai](taskdeck_ai_agent/01-architecture.md)
- [x] F01.2 Define phased plan, user flows, security invariants, entities and acceptance gates. — [Tài liệu triển khai](taskdeck_ai_agent/01-architecture.md)
  - [x] F01.2.1 Decide stateful chain/conversation/stateless fallback contract. — [Tài liệu triển khai](taskdeck_ai_agent/01-architecture.md)
- [x] F01.3 Align exact native Patch Tool schema and remote execution protocol with this proposal. — [Tài liệu triển khai](taskdeck_ai_agent/01-architecture.md)
  - [x] F01.3.1 Pin proven parser/queue/execution APIs with source-line references; no duplicate parser. — [Tài liệu triển khai](taskdeck_ai_agent/01-architecture.md)

### F02 — Model/provider settings & credentials [DONE in source for Responses + Chat Completions; Anthropic Messages deferred] — [Tài liệu triển khai](taskdeck_ai_agent/02-providers.md)
- [x] F02.1 Versioned provider/model config (custom HTTPS, provider ID, `apiType`, capabilities, reasoning support) — schema v1 supports Responses and Chat Completions. — [Tài liệu triển khai](taskdeck_ai_agent/02-providers.md)
  - [x] F02.1.1 Validate public HTTPS URL, DNS/public IP at connect, model ID, token bounds and unsupported settings; tested without live egress. — [Tài liệu triển khai](taskdeck_ai_agent/02-providers.md)
  - [x] F02.1.2 Separate secrets from exportable model JSON; explicit credential lifecycle. — [Tài liệu triển khai](taskdeck_ai_agent/02-providers.md)
- [x] F02.2 Native provider/API-type settings and basic connection test; capabilities require separate probes. — [Tài liệu triển khai](taskdeck_ai_agent/02-providers.md)
  - [x] F02.2.1 Add/list/edit/remove providers, model reasoning, default selection; no plaintext key response. — [Tài liệu triển khai](taskdeck_ai_agent/02-providers.md)
  - [x] F02.2.2 Probe text/stream/tool calling/stateful modes; never assume capability from declaration alone. — [Tài liệu triển khai](taskdeck_ai_agent/02-providers.md)
- [x] F02.3 Provider adapters (Responses + Chat Completions). — [Tài liệu triển khai](taskdeck_ai_agent/02-providers.md)
  - [x] F02.3.1 Responses request/response format and default reasoning-effort parameter for configured model. — [Tài liệu triển khai](taskdeck_ai_agent/02-providers.md)
  - [x] F02.3.2 Streaming SSE, cancellation, errors, rate limits and tool-call output parsing. — [Tài liệu triển khai](taskdeck_ai_agent/02-providers.md)
  - [x] F02.3.3 Chat Completions adapter implemented; Anthropic Messages intentionally deferred. — [Tài liệu triển khai](taskdeck_ai_agent/02-providers.md)

### F03 — Durable sessions & context [DONE in code/tests; retention and tokenizer-cost policy remain product decisions] — [Tài liệu triển khai](taskdeck_ai_agent/03-sessions-context.md)
- [x] F03.1 Independent multi-session store scoped by owner, workspace, project UUID and execution target. — [Tài liệu triển khai](taskdeck_ai_agent/03-sessions-context.md)
  - [x] F03.1.1 Atomic write/revision/size bound and session recovery after process restart (owner-bound private Go journal + tests). — [Tài liệu triển khai](taskdeck_ai_agent/03-sessions-context.md)
  - [x] F03.1.2 Session list/create/rename/archive/cancel APIs + UI tabs. — [Tài liệu triển khai](taskdeck_ai_agent/03-sessions-context.md)
- [x] F03.2 AI Pack first turn. — [Tài liệu triển khai](taskdeck_ai_agent/03-sessions-context.md)
  - [x] F03.2.1 Generate/validate bounded first-turn AI Pack text snapshot; pin fingerprint/project ID; mocked integration test. — [Tài liệu triển khai](taskdeck_ai_agent/03-sessions-context.md)
  - [x] F03.2.2 Separate mandatory system instructions from untrusted AI Pack body; validate project UUID before egress. — [Tài liệu triển khai](taskdeck_ai_agent/03-sessions-context.md)
- [x] F03.3 Stateful and stateless memory. — [Tài liệu triển khai](taskdeck_ai_agent/03-sessions-context.md)
  - [x] F03.3.1 Previous-response chaining; persist response ID alongside completed assistant message (mocked two-turn test). — [Tài liệu triển khai](taskdeck_ai_agent/03-sessions-context.md)
  - [x] F03.3.2 Conversations API ID mode; explicit compatibility tests; fallback to canonical history. — [Tài liệu triển khai](taskdeck_ai_agent/03-sessions-context.md)
  - [x] F03.3.3 Canonical tool-call/results replay, context budget and compaction preserving commitments. — [Tài liệu triển khai](taskdeck_ai_agent/03-sessions-context.md)

### F04 — Agent conversation & UI [IN PROGRESS: source features/tests present; pause/resume, token/cost quotas and interactive browser acceptance remain open] — [Tài liệu triển khai](taskdeck_ai_agent/04-conversation-ui.md)
- [ ] F04.1 Turn coordinator, per-session state machine and bounded request admission. (Concurrency/rate bounds implemented; token/cost quotas and durable pause/resume remain open.) — [Tài liệu triển khai](taskdeck_ai_agent/04-conversation-ui.md)
  - [x] F04.1.1 Append user+assistant messages transactionally; handle retry/timeouts/recovery. — [Tài liệu triển khai](taskdeck_ai_agent/04-conversation-ui.md)
  - [ ] F04.1.2 Explicit cancel is implemented and tested; pause/resume semantics/API are not implemented. — [Tài liệu triển khai](taskdeck_ai_agent/04-conversation-ui.md)
- [x] F04.2 TaskDeck native Chat GUI. — [Tài liệu triển khai](taskdeck_ai_agent/04-conversation-ui.md)
  - [x] F04.2.1 Multi-tab session list, model/reasoning selector, prompt attachments and history. — [Tài liệu triển khai](taskdeck_ai_agent/04-conversation-ui.md)
  - [x] F04.2.2 Live status, per-command task subrows, console view/copy and AI final response. — [Tài liệu triển khai](taskdeck_ai_agent/04-conversation-ui.md)
  - [x] F04.2.3 Recover UI after refresh/reconnect; no duplicate sends or actions. — [Tài liệu triển khai](taskdeck_ai_agent/04-conversation-ui.md)

### F05 — Tool Gateway and Patch Tool feedback loop [DONE in source/mock tests; live Python Patch run remains NOT VERIFIED] — [Tài liệu triển khai](taskdeck_ai_agent/05-patch-feedback.md)
- [x] F05.1 Function tool schema `taskdeck_submit_patch` with strict JSON name/patch_python; human approval required; no arbitrary shell tool. — [Tài liệu triển khai](taskdeck_ai_agent/05-patch-feedback.md)
  - [x] F05.1.1 Enforce native patch schema / both project identity headers / one-time approval / exact selection; server-derived HEAD/branch/dirty and arguments digest are rechecked before side effects. Native/manual writer arbitration remains limited. — [Tài liệu triển khai](taskdeck_ai_agent/05-patch-feedback.md)
  - [x] F05.1.2 Persist per-call idempotency ledger before queueing and once-only AI result output; shared cross-process transaction/locking remains F06. — [Tài liệu triển khai](taskdeck_ai_agent/05-patch-feedback.md)
- [x] F05.2 Wire approved patches to existing Patch Paste queue and native Patch Session / prompt_response handlers; real runtime acceptance remains unverified. — [Tài liệu triển khai](taskdeck_ai_agent/05-patch-feedback.md)
  - [x] F05.2.1 Persist link from originating AI call_id to queued package SHA and native Patch Session ID (PATCH only; COLLECT future). — [Tài liệu triển khai](taskdeck_ai_agent/05-patch-feedback.md)
  - [x] F05.2.2 Collect native per-item status/RC/elapsed, bounded output, execution evidence and opaque artifact references; full artifact-file download/patch diff remains not implemented. — [Tài liệu triển khai](taskdeck_ai_agent/05-patch-feedback.md)
- [x] F05.3 Fully automatic AI feedback loop across long-running jobs/restarts (manual-confirmed continuation implemented, unattended recovery still pending). — [Tài liệu triển khai](taskdeck_ai_agent/05-patch-feedback.md)
  - [x] F05.3.1 Send evidence-backed `function_call_output` using exact call_id, manually or via bounded local opt-in scheduler after background native reconciliation. — [Tài liệu triển khai](taskdeck_ai_agent/05-patch-feedback.md)
  - [x] F05.3.2 Persist QUEUING/UNKNOWN transitions before side effects; no automatic patch resubmission; manual recovery of ambiguous runs remains pending. — [Tài liệu triển khai](taskdeck_ai_agent/05-patch-feedback.md)

### F06 — Parallel sessions & remote [F06.1 DONE; F06.2 BLOCKED: no typed remote Patch action exists] — [Tài liệu triển khai](taskdeck_ai_agent/06-parallel-remote.md)
- [x] F06.1 Independently running sessions with separate histories, quotas, cancellation tokens. — [Tài liệu triển khai](taskdeck_ai_agent/06-parallel-remote.md)
  - [x] F06.1.1 Configurable max session/model/execution concurrency; queue backpressure. — [Tài liệu triển khai](taskdeck_ai_agent/06-parallel-remote.md)
  - [x] F06.1.2 Workspace write lease and branch/HEAD optimistic check. — [Tài liệu triển khai](taskdeck_ai_agent/06-parallel-remote.md)
- [ ] F06.2 Bind remote target to authorized E2EE dispatcher, log and cancel. — [Tài liệu triển khai](taskdeck_ai_agent/06-parallel-remote.md)
  - [ ] F06.2.1 Validate remote scope per action, never forward API keys. — [Tài liệu triển khai](taskdeck_ai_agent/06-parallel-remote.md)
  - [ ] F06.2.2 Recover state after remote or GUI disconnection. — [Tài liệu triển khai](taskdeck_ai_agent/06-parallel-remote.md)

### F07 — Security/validation/operability [IN PROGRESS: targeted mocks/CI pass; live Patch, interactive browser acceptance and independent audit pending; remote Patch out of scope] — [Tài liệu triển khai](taskdeck_ai_agent/07-acceptance.md)
- [x] F07.1 Auth, owner/project isolation and approvals for every new route and tool. — [Tài liệu triển khai](taskdeck_ai_agent/07-acceptance.md)
  - [x] F07.1.1 Fail closed on unknown capability or untrusted patch metadata; never accept AI's `approved` flag. — [Tài liệu triển khai](taskdeck_ai_agent/07-acceptance.md)
  - [x] F07.1.2 Provider egress/SSRF/redirect policy, local/private-IP safeguards and TLS code; independent security audit remains pending. — [Tài liệu triển khai](taskdeck_ai_agent/07-acceptance.md)
- [ ] F07.2 Tests and documentation. — [Tài liệu triển khai](taskdeck_ai_agent/07-acceptance.md)
  - [x] F07.2.1 Unit tests for initial Responses request/parse, chaining, tool-call mapping, owner isolation and persistence; additional provider formats remain F02.3.3. — [Tài liệu triển khai](taskdeck_ai_agent/07-acceptance.md)
  - [ ] F07.2.2 Local native Patch bridge/retry/cancel, background reconciliation and concurrent approval tests pass with fake Session Service; real Python run is NOT VERIFIED; remote E2EE Patch is out of scope/no typed action exists. — [Tài liệu triển khai](taskdeck_ai_agent/07-acceptance.md)
  - [ ] F07.2.3 Automated CI/static UI bootstrap + Smart Terminal regression tests pass; interactive browser GUI smoke remains NOT VERIFIED. — [Tài liệu triển khai](taskdeck_ai_agent/07-acceptance.md)

### F08 — Planning + execution: F08.1 implemented; F08.2 foundations IN PROGRESS, end-to-end scheduler pending — [Tài liệu triển khai](taskdeck_ai_agent/08-future-planner.md)
- [x] F08.1 Markdown/canonical JSON → validated Task Manifest and read-only preview (no execution). — [Tài liệu triển khai](taskdeck_ai_agent/08-future-planner.md)
  - [x] F08.1.1 Schema validation, dependency cycle detection, bounded number of tasks. — [Tài liệu triển khai](taskdeck_ai_agent/08-future-planner.md)
- [ ] F08.2 Dependency DAG Scheduler and configurable concurrency. — [Tài liệu triển khai](taskdeck_ai_agent/08-future-planner.md)
  - Foundation: durable Run Manager/DAG state, fencing, success-only dependency gate and global cap=5 are implemented in the uncommitted source; owner/project-scoped Context Groups can be revision-pinned into immutable task snapshots. Owner-bound Provider Router factory is now used by Run Manager when no custom executor is injected; manifest orchestration APIs are implemented, while browser Run All/status UI remains pending.
  - [ ] F08.2.1 Worktree/task isolation and controlled merge strategy. — [Tài liệu triển khai](taskdeck_ai_agent/08-future-planner.md)
    - Partial: detached worktree binding and the Run API's explicit Agent Patch approval/queue/selection/result-continuation bridge are covered by fake-native tests. Current PLAN 02 default prompt options still advertise text-only, so model-generated Patch calls are not yet enabled in production; real Python execution and user review/apply UX remain unverified. No auto merge/branch mutation.
  - [ ] F08.2.2 Integration reports, retry/resume, explicit approval for branch mutation. — [Tài liệu triển khai](taskdeck_ai_agent/08-future-planner.md)
    - Partial: workflow persists per-task report metadata and requires explicit approve/reject before dependencies unlock; safe retry and reviewed resume are tested. A separate menu-mounted Run All/review panel is present and static-tested; browser acceptance, full diff artifact retention, conflict-aware target integration and real branch-mutation approval remain pending. No automatic branch mutation.

## 5. Incremental delivery gates

| Milestone | Evidence required | Status |
|---|---|---|
| M0 handoff | This document with verified code map and statuses | DONE (source) |
| M1 protocol core | Stateful/stateless request building, parse response, tests | DONE (code + CI) |
| M2 session API + provider settings | Persisted sessions, BYOK secrets, capability testing | DONE in code/tests (Responses + Chat Completions; live providers not verified) |
| M3 chat GUI | Real messages with streaming, cancel and recovery | DONE in source/automated tests (browser GUI acceptance remains NOT VERIFIED) |
| M4 Patch loop | Real queue run + tool results + idempotency + approvals | IN PROGRESS (background reconciliation + fake-native tests; real Python runtime/final live SHA evidence NOT VERIFIED) |
| M5 remote & multi-session | E2EE live tests, lock safety, session isolation | BLOCKED for remote Patch (no typed operation); local admission/lease tests PASS |
| M6 release acceptance | Regression suite, real provider smoke, security review | TODO |
| M7 planner | F08.1 validation + F08.2 run/DAG/worktree execution evidence | F08.1 DONE; F08.2 foundation PARTIAL/IN PROGRESS (uncommitted; provider/Patch/UI integration pending) |

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

**Historical M1–M3 source snapshot (retained for traceability; current native Tool Gateway is documented separately below):**
- `internal/agentchat/responses.go`, `responses_test.go`: canonical context modes `stateless`, `previous_response`, `conversation` (conversation ID reuse only); tool-call/result encoding/parsing, mandatory instructions every request.
- `internal/agentchat/store.go`, `store_test.go`: owner/project-bound 0600 canonical history, random session IDs, CAS revision guard, atomic saves, restart recovery.
- `internal/agentchat/providers.go`, `providers_test.go`: provider/model schema v1, public HTTPS validation, DNS resolution public-IP checks on each connection, no proxy/redirect, bounded Responses HTTP request, owner-scoped metadata store. Agent keys are **not** stored in provider JSON.
- `internal/server/agent_chat_providers.go`, `agent_chat_providers_test.go`: `GET/PUT /api/agent/providers` and opt-in `POST /api/agent/providers/test`. Credentials go through existing encrypted `ConnectionSecrets` with separate owner/workspace/provider keys. The probe tests **only basic Responses text**, not tool calling, streaming, stateful history or reasoning capabilities.
- `internal/server/agent_chat_sessions.go`, `agent_chat_sessions_test.go`: `GET/POST /api/agent/sessions`, `GET /api/agent/sessions/{id}`; explicit custom provider/model selection and project UUID binding. Existing advisory settings remain backward-compatible, but legacy advisory provider keys do not automatically grant Agent Chat capabilities.
- `internal/server/agent_chat_messages.go`, `agent_chat_messages_test.go`: `POST /api/agent/sessions/{id}/messages`; explicit external egress consent, per-session in-flight lock, revision compare-and-swap, AI Pack on first turn with size bound and fingerprint, mandatory system instructions, persisted user+assistant history, previous response chaining, tool-call recognition with `waiting_tool` status. M1–M3 had no executable gateway at that time; the separate native Tool Gateway continuation below supersedes this historical scope. Error leaves a durable failed session; automatic retry deliberately disabled.
- `internal/server/server.go`, `shared_authorization.go`: routing and shared-server RBAC, including `files.read` before external project-context submission.
- `web/featuremods/agentchat.js`, `next.js`, `menus.js`, `internal/server/agent_chat_ui_test.go`: integrated native UI under Settings → AI Assistant: Open AI Agent Chat / AI Agent providers; configure model/key, create/select sessions, show transcript, send/review external message, cancel active browser request, review/approve native PATCH and explicitly submit native results to AI. Browser abort is not native process cancellation.
- `.github/workflows/taskdeck_agentchat_core.yml`: Go race/unit/vet, server Agent Chat integration tests and `node --check` for new UI.

**Historical M1–M3 CI:** [run 44464121465](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/44464121465) PASS for then-selected tests. The old selector `'^TestAgentChat'` missed 10/25 Agent server tests. **Current selector fixed** in `7d6c6bed`; [run 44479964926](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/44479964926) PASS for all `TestAgent*` tests on that commit. Both are mock/unit evidence, not live provider/native Python/browser/remote acceptance.

**Known limitations / must not overclaim:**
1. **Patch Tool gateway exists in code**: with a user-declared `ToolCalling=true`, the model can propose `taskdeck_submit_patch`; user review/approval can queue/select native PATCH. However capability is **not probe-verified**, live Python runtime has not been accepted, and unattended server-side AI continuation remains **NOT IMPLEMENTED**.
2. Basic Chat is synchronous JSON only: SSE streaming, pause/resume, cross-daemon locks, request idempotency, retry/recovery UI, session rename/delete and log console integration are incomplete.
3. Conversations mode falls back to canonical stateless replay until a verified Conversations API create/ID negotiation path exists. `previous_response_id` mode assumes the provider supports chaining when selected; automatic capability negotiation/fallback on provider error is not implemented yet.
4. The first AI Pack request sends a bounded/redacted text companion, not a ZIP. Server rejects oversize packs instead of silently truncating the tool contract. Automatic context compaction and an AI Pack preview/advanced egress policy are not yet implemented.
5. Metadata editing and key creation/rotation exist, but delete provider, revoke API key, model discovery, pricing/token accounting and provider-specific reasoning validation require further work.
6. Browser UI now has chat, patch review/approve/reject, native status polling while open, and explicit verified tool-result continuation. Real endpoint/network, live browser, benign Python runtime and remote E2EE acceptance remain untested.

**Next ordered implementation:** (A) audit internal-handler authorization and correlate native run/SHA result to exact approved package, (B) Git HEAD/dirty + multi-process write lease and crash recovery, (C) isolated benign Python Patch live test, provider capability probes, SSE and backend feedback continuation, (D) remote/artifacts/console, (E) release gates. Existing native Tool Gateway code must not be implemented a second time. Keep future plan DAG deferred.

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

**CI evidence:** [Run 44477297240](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/44477297240) — PASS on commit `ccfac9b`; Go race tests/core, server Agent tests including a fake native Session Service that checks exact item selection through TaskDeck's actual handler chain, and JavaScript syntax check. No production endpoint credentials or live Python Patch runtime were executed in these tests.

**Historical outstanding-work snapshot (superseded by hardening below):** at that point feedback required GUI polling and final native run/SHA evidence was not correlated. Current code adds a local opt-in server scheduler, background native-state reconciliation and final queue SHA/size verification; these have fake-native tests but are **not** live Python Patch/provider/browser evidence. Remote E2EE Patch remains out of scope/no typed operation. **Code implemented ≠ live integration verified.**

### Historical documentation re-audit — “New deck AI document” (2026-11-19 baseline; current state superseded below)

- Reviewed commit: `b2f2234a` and all 9 Markdown files under `docs/taskdeck_ai_agent/`. Structure F01–F08 and relative links are valid; **feature is NOT release ready**.
- P0: CI selector originally skipped 10/25 Agent Server tests; PR paths were incomplete. Fixed at [7d6c6bed](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/7d6c6bedaccb7436ce3fa730551f7a30368e6fa9) and [CI 44479964926](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/44479964926) PASS for the corrected test selection. Still not live Python/browser/provider evidence.
- Historical findings at that baseline: declared tool capability, missing final SHA/run correlation, source drift and feedback-attempt ambiguity. Current source below contains probe gating, Git snapshot, package SHA verification, explicit feedback state and background local continuation. Remaining boundary: internal handler invocation does not implicitly re-run router middleware; each handler's explicit permission/lease checks and the unshared native/manual workspace lease remain audited risks, not a security-review pass.
- Detailed findings and source/state matrix: [Source audit](taskdeck_ai_agent/09-source-audit.md). Keep F05/F06/F07 checkboxes blocked until the specific acceptance evidence exists; do not label mock CI as live-ready.

### Implementation continuation — P0 hardening (2026-11-19, source + unit/integration mock only)

Changed source:
- `internal/agentchat/lockfile_unix.go` / `lockfile_other.go` + `store.go`: journal read-compare-write now holds a Go mutex **and** an OS advisory `flock` on a private `.agent-store.lock` (Linux/macOS). Non-Unix builds keep in-process only, so multi-daemon on those platforms is still unsupported and must be refused by deployment policy.
- `internal/server/agent_chat_drift.go`: server-derived approval snapshot (canonical HEAD, branch, `git status --porcelain=v1 -z --untracked-files=all` digest). Proposal time captures it; approval re-verifies before the queue write **and** before native selection. Non-Git workspaces are recorded as such and are not silently treated as equal.
- `internal/server/agent_chat_tools.go`: approval now carries a 10-minute expiry, binds the exact model `call_id`/turn and `arguments_sha256`, refuses edited arguments, and parks drift/expiry as `unknown` with no queue package created. Workspace execution lease (`agent_chat_workspace_lease.go`) admits one Agent execution per canonical workspace.
- `internal/server/agent_chat_native.go`: `agentExactPatchSelection` returns prompt ID + index; `agentPatchResultFromProtocol` requires name **and** approved index **and** a non-empty native `run_id`, so a reused/renamed item or a different run cannot satisfy the call. Reconcile refuses a run-ID mismatch.
- `internal/server/agent_chat_sync.go`: read-only startup reconciliation adopting only unambiguous final evidence (never queues/launches/selects); new owner-scoped `GET /api/agent/sessions/{id}/tools/{call_id}/execution` exposing native status, bounded command summary and opaque artifacts without log paths/argv.
- `GET /api/agent/sessions/{id}/pack-preview`: renders the same bounded/scrubbed first-turn AI Pack text with `redaction:"best_effort"`, `oversize` and `truncated`, without contacting a provider or mutating the session; UI button "Preview context".
- `request_id` idempotency: `agent_chat_messages.go` persists `LastRequestKey/SHA/Code` before egress and replays a retry with the same key (even with a now-stale revision) instead of a second provider call; same key + different message → 409.
- Regression gate `TestAgentChatDoesNotEscalateAdvisoryOrNativePatchSurfaces`: Agent UI cannot reach terminal/patch-upload execution surfaces, advisory `aiassistant.js` has no Agent session/tool API, and `patchpanel.js` has no Agent shortcut. CI now also runs `node test_ai_smart_terminal.cjs`. This is a static/contract gate, not a browser smoke test.
- Multi-tab UI: `agentTabs` + `agentDrafts` give each open session its own tab and composer draft; the server remains the source of truth and closing a tab never deletes a session.
- Durable event journal: `Session.Events`/`EventSeq` (≤128 events, ≤120-char machine detail, no prompt/result content) appended on `turn_started`, `turn_finished`, `turn_cancelled`, `turn_failed`, `tool_queuing/queued/running/rejected`, `feedback_prepared/acknowledged` and `tool_final`. `GET /api/agent/sessions/{id}/events?since=<seq>` returns the delta plus status/revision cursor; UI `syncAgentEvents` refreshes the transcript only when a terminal event arrives. Store validation enforces monotonic sequence and bounded detail.
- Refresh/reconnect recovery: selecting a session whose durable status is `requesting_ai` shows the server-side turn as still running, disables Send, and offers Cancel (verified by `TestAgentRefreshDuringRequestOffersCancelNotDuplicate`: duplicate send → 409, cancel reports `cancelled:true` and persists `cancelled`).
- `POST /api/agent/sessions/{id}/auto-feedback` + `internal/server/agent_chat_auto_feedback.go`: explicit per-session opt-in with a 30-minute bound (`AutoFeedback`, `AutoFeedbackUntil`). A bounded scheduler pass delivers only a verified final tool result once via the shared `agentDeliverToolResult`, under the session turn lock, re-checking owner (`local` only), project identity, provider and expiry at egress. It never approves, queues, starts, selects or retries a patch; transport/cancel → `unknown`, not resent; 429 defers to the next pass. UI exposes an "Auto-send results" toggle with an explicit consent dialog stating approve-per-patch still applies.
- `internal/server/agent_chat_attachments.go`: text/JSON-only attachments (≤4 files, 32 KiB each, 64 KiB total, UTF-8, no NUL) rendered inside an explicit untrusted-data block; binary/office/image MIME types are refused. UI stages files via `FileReader` only — no path read, no base64.
- Integration tests: `TestAgentNativePatchCancelAndReconcileKeepSingleExecution` (cancel never claims the native run stopped; RUNNING is not adopted; final evidence adopted once; second sync is a no-op) and `TestAgentConcurrentApprovalsStartOneNativeExecution` (6 parallel approvals → exactly one 200 and one native start), both also run under `-race`.
- `internal/server/agent_chat_limits.go`: bounded per-owner (2) and global (4) model-turn admission with a capped waiter queue and 2-minute timeout, returning 429; complements the existing hourly rate quota.
- Session `target` accepts only empty or `local`; any remote target is refused with 400 because no typed E2EE Patch operation exists.
- Probes now verify every mode except Chat Completions: `text`, `tool_calling` (schema acceptance), `previous_response` (two turns, expects recalled context), `conversation` (two turns, reuses returned ID) and `streaming` (real SSE adapter, requires provider `stream:true`). Outcomes are `verified`/`unsupported`/`inconclusive`.
- Conversations mode now uses the stored conversation ID ONLY when a probe verified `conversation` for that exact endpoint/model/fingerprint; otherwise it computes a stateless request from canonical history while leaving the stored session mode immutable.
- Sessions: immutable per-session `reasoning_effort`, validated against the model's advertised efforts at creation (400 otherwise) and used for both normal and tool-continuation requests; UI exposes a per-model effort selector.
- `internal/agentchat/stream.go` + `providers.go`: shared hardened HTTP client (no proxy/redirect, per-connection public-IP pinning) for JSON and SSE; bounded per-event/total/event-count parser; tool-call arguments assembled from `delta` fragments and only accepted when the call ID, name and JSON are complete; completion required, EOF/`response.failed` are errors; cancellation propagates via ctx. Streaming is opt-in per provider and only used when a verified streaming probe exists, otherwise JSON is used.
- `internal/agentchat/compact.go`: bounded compaction that drops only completed plain-text turns before any tool exchange and outside the protected tail; it refuses when nothing safe remains (413) instead of truncating a pending commitment. Wired into both message and tool-continuation paths above 200 items.
- `internal/server/agent_chat_tool_feedback.go` + `store.go`: `FeedbackStatus` (`prepared` → `sending` → `acknowledged` | `unknown`) makes clear that a persisted attempt is not provider acknowledgement; transport/cancel failures become `unknown` and are never resent. Store validation now rejects orphan `function_call_output`, duplicate call IDs and invalid feedback states.
- Providers: `DELETE /api/agent/providers` removes metadata and, only when explicitly requested, the local encrypted credential; response states `vendor_revocation:false` and reports partial failure. `POST /api/agent/providers/probe` records server-side capability evidence keyed by endpoint/model/API-type/config fingerprint (`internal/agentchat/capabilities.go`). Tool execution is enabled only when a probe returned `tool_calling: verified` for that exact fingerprint; a declared `tool_calling` flag alone no longer advertises the execution tool. Probe failures/timeouts/429 are `inconclusive`, never `unsupported`; streaming probe reports `unsupported: streaming_adapter_not_implemented`.
- Sessions: schema version, `title`, `archived`, `target` (only `local`); archive/rename via `PATCH /api/agent/sessions/{id}/metadata`; archiving a session with an unresolved patch call is refused; the 32-session limit counts active sessions only. `POST /api/agent/sessions/{id}/cancel` cancels the registered server-side provider context and records `cancelled`; a started native Patch execution is explicitly left to native reconciliation.
- UI (`web/featuremods/agentchat.js`): per-session busy state instead of one global lock, server-side cancel, rename/archive, text-only session list, provider probe/delete, and an evidence/console panel with explicit copy. No HTML insertion, no command execution.

Verification actually run (Go 1.20.14, Linux):
- `go test ./internal/server -run '^TestAgent' -count=1` PASS
- `go test ./internal/agentchat -count=1` and `-race` PASS
- `go test ./internal/patchtool -count=1` PASS
- `go vet ./internal/server ./internal/agentchat` clean
- `node --check web/featuremods/agentchat.js` PASS
- `go build ./cmd/vscode_tasks_menu` PASS

**Not verified / still blocked:** no live provider call, no live Python Patch run, no browser GUI acceptance and no remote E2EE Patch operation (none exists). Running the whole `go test ./internal/server` package still reports **13 pre-existing failures verified on clean HEAD `c25d48be`** (foreground-lock/UI contract strings, grouped menus, restricted-role integration test, real-WSS app ping, terminal path detection, restart detection) plus one unrelated environment timeout earlier; they are outside Agent Chat scope and were not modified. The native Patch Panel does not take the Agent workspace lease, so Agent-vs-manual exclusion relies on the Git/dirty snapshot guard, and external writers remain out of scope. Cross-process flock semantics on NFS/SMB are not guaranteed.

### Implementation continuation — audit fixes, Chat Completions and F08.1 (2026-11-19)

**A1–A5 audit fixes:**
- Provider turns now use the daemon lifecycle context, not the HTTP request context. Browser disconnect no longer cancels a started provider call; explicit Cancel still cancels the registered request, and daemon shutdown cancels all active turns. See `agent_chat_turn_context.go`, `server.go`.
- Auto-feedback scheduler polls the native Session Service itself for `running` linked Patch calls, reads `ProtocolState`, adopts only a final result matching native session, queue item name/index, call ledger and `run_id`, and verifies the persisted queue ZIP SHA256/size again before feedback. Missing native session or missing/changed digest becomes `UNKNOWN`; no requeue/retry. GUI `/sync` is no longer needed for the local opt-in scheduler.
- UI session switching now stores the outgoing draft before awaiting fetch, uses a generation counter to discard stale responses, keeps draft/attachments/request ID per session, preserves them on failure, and guards response/status updates against changing the active tab.
- Message body limit is 512 KiB to cover the full decoded 16 KiB message + 64 KiB attachments with JSON escaping overhead. Raw request UTF-8 is checked before JSON decoding. Idempotency digest includes exact validated attachment name/MIME/text content; same key with changed attachment → 409.
- Backpressure now holds one waiter position for the full wait, uses a condition notification (no sleep loop), honors cancellation/deadlines, rejects waiter 9 and releases slots once.

**C — Chat Completions adapter:**
- Provider schema now accepts `responses` and `chat_completions`, preserving existing Responses config. Dedicated `chat_completions.go` maps the existing canonical Responses request/history to Chat Completions messages/functions and parses result into the existing canonical `ResponsesResult`; all tool calls still go through the same approval/Gateway.
- Dedicated `chat_completions_stream.go` parses SSE text/tool fragments under byte/event bounds; finish reason and `[DONE]` are required. Streaming is enabled only after verified probe. Chat Completions context is stateless only; previous-response/Conversations probes are `unsupported`, and reasoning options are not guessed for generic providers.
- Tests cover canonical multi-turn/tool result mapping, fragmented tool arguments, finish/error handling, timeout/cancel boundaries, shared hardened transport path and API type/capability behavior. Anthropic Messages remains deferred.

**D — F08.1 planning-only Task Manifest:**
- New `internal/taskmanifest` parses bounded Markdown and strict canonical JSON (≤1 MiB, ≤100 tasks), validates stable IDs, duplicate/self/unknown dependencies, acceptance warnings, schema/project identity and cycles, and returns roots/blocked/topological preview/diagnostics.
- `POST /api/agent/task-manifests/validate` remains planning-only: owner/permission checked, derives project identity on server, and performs no provider call/file write/task creation. UI supports Markdown/canonical JSON preview/download. F08.2 adds separate Run All workflow APIs and a menu-mounted workflow panel; approval-controlled Patch continuation is fake-native tested. Auto-merge/owner-branch mutation remains disabled.

**Verification on merged HEAD `8d75b299` (Go 1.20.14/Linux):**
- `go test -race -count=1 ./internal/agentchat` PASS
- `go test -race -count=1 ./internal/taskmanifest` PASS
- `go test -race ./internal/server -run '^TestAgent' -count=1 -timeout 900s` PASS
- `go test ./internal/patchtool -count=1` PASS
- `go vet ./internal/agentchat ./internal/server ./internal/taskmanifest` clean
- `node --check web/featuremods/agentchat.js` PASS
- `node web/agentchat_bootstrap_test.cjs` PASS
- `node test_ai_smart_terminal.cjs` PASS (4/4)
- `go build ./cmd/vscode_tasks_menu ./internal/...` PASS
- `go test ./internal/server -count=1` on baseline `bba5eea6` failed 22 unrelated auth/editor/English UI/foreground/grouped-menu/Patch-panel/terminal/restart/restricted-role tests; the same 22 names fail in a clean worktree at that exact commit. The diagnostics commit `8d75b299` changes Agent Chat/provider diagnostics only; targeted `^TestAgent` passes afterward.

**Still NOT VERIFIED / blocked:** real benign Python Patch runtime end-to-end, real external provider smoke, interactive browser GUI acceptance, independent security review. Remote Patch remains out of scope and blocked: no typed E2EE Patch operation. No credentials or remote execution were used. `origin/main` was fetched and merged without force; current `HEAD` equals `origin/main` (`bba5eea6`).

## 8. Remaining source-change boundaries and handoff

1. Preserve the separate Smart Terminal advisory and Agent execution surfaces; do not create another Patch parser/executor.
2. Remaining P0 review: internal handler invocation bypasses router middleware re-entry. Verify the caller's auth/origin/browser lease and each callee's explicit permission/mutation-lease checks; consider a typed service boundary before changing any route. No independent security audit has been completed.
3. Final evidence now binds queue SHA/size + native session + item index + run ID and revalidates Git snapshot; remaining local live test and shared lease with manual/native writers are still open. Do not claim remote support.
4. Provider probes, request/attachment idempotency, daemon-owned turns, durable event cursor and local bounded auto-feedback are implemented/tested. Provider ACK is still unknowable after transport ambiguity; never resend `unknown`.
5. Current targeted verification: `go test -race ./internal/agentchat`, `go test -race ./internal/taskmanifest`, `go test -race ./internal/server -run '^TestAgent'`, patchtool, vet and JS regressions. Keep these separate from live provider/native/browser/remote acceptance. Full server package has 22 baseline failures verified at `bba5eea6` in a clean worktree.
6. F08.1 is planning-only and implemented. F08.2 Run All/DAG orchestration and fake-native Patch continuation are in progress/uncommitted; real runtime, browser and controlled branch-integration acceptance remain open. Anthropic Messages remains deferred.

## 9. Commands for verification

```sh
cd vscode_tasks_menu_go
go test ./internal/agentchat
go test ./internal/server ./internal/patchtool
go test ./...
node test_ai_smart_terminal.cjs
```

Run platform/remote smoke tests as available. External API requests incur cost: never perform paid provider calls or remote command execution merely to make tests green; use mocked HTTP servers and fake task executors until explicitly testing with real credentials.

### UI continuation — Agent Chat dock (source + DOM bootstrap; browser acceptance pending)

- `web/featuremods/agentchat.js` now mounts Agent Chat as a non-modal region inside the existing `main > section`, preserving the real `#tabs` and `#panes` nodes. Right/Bottom layout and a dedicated splitter do not modify sidebar grid or terminal split-tree.
- Agent Chat has a persistent collapsed strip, per-workspace dock preference, separate right width/bottom height, Auto Hide/Pin state, outside-click/Esc collapse, and a responsive title More menu that moves existing action nodes (no duplicate handlers). Hide does not call cancel; reopening reloads authoritative session state.
- DOM bootstrap test exercises mount, toolbar presence, duplicate-control guard, and Right→Bottom plus Auto Hide state changes. JavaScript syntax and focused Go UI contract tests pass.
- **Not verified:** real browser Playwright smoke, keyboard/focus navigation end-to-end, light/dark and mobile visual acceptance, editor/terminal resize behavior, and full cross-browser regression. Do not claim GUI acceptance from the DOM bootstrap test.

### Debug continuation — provider diagnostics and capture (unit/mock evidence; GUI acceptance pending)

- Added typed provider error codes/stages for HTTP status, basic network failure classes, response parsing, invalid stream content type and incomplete SSE; non-2xx JSON extracts bounded provider type/code/message, Retry-After and HTTP status. Provider credentials and auth headers are not included. Message failures return structured JSON with trace IDs while retaining the existing no-retry/ambiguous semantics.
- Added an owner-scoped, in-memory server trace ring (64 traces/owner, 256 events/trace, 30-minute retention) with settings-read/write gates, no-store endpoints, redacted Safe payload metadata and explicit RAW consent/grant (30-minute TTL, 256 KiB/body, 2 MiB/owner). Raw buffers are never written to session journals or application logs; they are cleared on disable/expiry/clear. Raw GET requires settings-write permission in addition to an active owner grant.
- Instrumented Responses and Chat Completions JSON/SSE transport at actual serialized request/response/frame boundaries. Trace records safe endpoint, provider/model, HTTP status, selected safe headers, provider request ID, timing, size/hash, context mode, history/tool counts, parse/error stage, and native approval/queue/final/feedback lifecycle summaries. Tool lifecycle events carry call IDs but no patch source or output bodies in safe events.
- Agent Chat title now has a direct AI Debug capture toggle and an AI Debug view with Timeline, Console, HTTP, Payload, Tools and Configuration filters; raw payload viewing has a separate confirmation. Export is sanitized by default. Browser bootstrap, focused server/API ownership/RAW-boundary tests and provider error/sink tests are the available evidence.
- **Not verified / remaining:** full configuration/preflight error tracing before provider-call entry, detailed DNS/TLS/TTFB timings, all UI filtering/search/follow-tail interactions, browser/keyboard/mobile smoke, live provider tests and full server suite. On baseline `bba5eea6`, `go test ./internal/server -count=1` completed with 22 failures; the exact same 22 named tests failed in a clean detached worktree at that commit. The subsequent `8d75b299` commit changes Agent Chat diagnostics/UI only; focused `^TestAgent` tests pass after it. No claim of full-package PASS. Do not mark the feature release-ready.
