# TaskDeck Smart Terminal AI — Implementation Plan

Date: 2026-10-13
Base: `main`
Working branch: `feature/ai-smart-terminal-l1-l3`
Owner: TaskDeck
Status: L1 advisory MVP + L2 opt-in preview implemented; L3 preview/approval-only. Autonomous or approved command execution NOT IMPLEMENTED. Incremental commits on isolated feature branch.

## Product goals
Keep xterm.js/PTY behavior unchanged. Add an optional Smart Terminal assistant with an accessible Settings > AI Assistant dialog, scoped context, cost/timeout controls, and an independently controlled AI side panel. API keys may be entered once in the Settings dialog over the authenticated HTTP connection but are NEVER returned by GET, copied into logs, Git, clipboard, snapshots or shared sessions.

## Architecture and invariants

- AI remains opt-in; disabled by default. Backend enforces authorization, not UI alone.
- L1 (suggest-only): explicit submit -> configured supported provider API (OpenAI-compatible Chat Completions or Anthropic native Messages) -> text answer. No shell/Git/file/database side effects. Support explain-error, explain-command, suggest-next-step via bounded recent command/output.
- L2 (context-aware): user-selectable, bounded CWD, Git branch/status, file reference and session command history. Every context source labeled untrusted; secrets redact before transmission. Never auto-upload files/projects/logs. Explicit disclosure before external model calls.
- L3 (guided): plan proposals represented as structured actions, with per-command allow/deny, immutable target (user/project/session/host/cwd), approval and audit. Never permit LLM raw shell escape to execute, never auto-approve destructive commands, and never run on production without dedicated explicit approval. Begin with preview-only proposal pipeline before executing in isolated, permission-checked tool runner.
- Every action associated with auth user, project, workspace, host and session; no permissions derived from prompt text.
- A stopped/failed/reloaded task must not retry side effects silently. Idempotency keys + durable journals required for real L3 execution.
- Sensitive content (ANSI escapes, terminal control codes, passwords, bearer tokens, SSH private keys) excluded or masked before model submission. Untrusted output is DATA, never policy.
- No remote provider access before the user configures a provider and consent; provider HTTPS endpoints are fixed allowlisted constants, not user-editable URLs. No hardcoded keys.

## Milestones, deliverables, acceptance gates

### Milestone A — foundation & planning
- [x] Branch created from main
- [x] PLAN and HANDOFF, first independent commits
- [x] Inventory existing shell integration and auth/secret storage
- [x] Capability/deployment threat model for L1/L2 and plan-only L3 (see operator guide); L3 execution threat model still pending

### L1 — Smart assistant (suggest-only)
- [x] Settings > AI Assistant modal: provider selection for OpenAI/Anthropic/Gemini/DeepSeek/Groq/OpenRouter; popular editable model suggestions, official key-generation instructions, separately encrypted provider keys, timeout, output cap, quota, context flags and consent. Custom API endpoint URLs remain intentionally unsupported.
- [x] Backend validation, server auth, provider-specific fixed HTTPS endpoints (no open-proxy/SSRF), bounded payload/deadlines, provider-scoped encrypted keys, no key on GET; local per-user quota.
- [x] Terminal-contextual Explain/Suggest + plan preview; independent side panel with answer and copy; never auto execute.
- [~] Error diagnostics, unavailable key/provider responses and cancel implemented; separate connection-test button, streaming and real provider E2E still pending.
- [x] Server-side security/unit tests, UI static tests, Node plan/parser tests, operator guide and feature-branch CI.

### L2 — Context aware assistant
- [~] Bounded command/output, CWD, history, Git UI; backend-attested terminal identity/host/launch CWD and read-only preview now implemented. Shell output and live interactive CWD are still client-derived/untrusted.
- [x] Backend checks live terminal kind/status, authenticated owner/project visibility and SSH profile; provider requests and plans reject missing/foreign/stale sessions. Future execution still needs a distinct execution principal.
- [x] Explicit preview, best-effort redaction, scope flags and bounded/truncated context. Users must inspect for secrets manually.
- [~] Consent/auth/redaction/budget tests completed; more adversarial prompt-injection and stale-session tests pending.

### L3 — Guided agent
- [~] Private persistent plan/step schema, pending/approved/rejected decisions and user-scoped audit. No execution task state yet.
- [~] Server-attested immutable target, digest/30-min TTL, deterministic conservative risk labels and exact typed read-only candidate registry. No real execution principal/broker.
- [x] Per-step confirmation requires matching saved plan digest, non-expired window and unchanged attested target. All approvals remain non-executable.
- [x] Plan preview, private per-user decision journal and read-only candidate validation API/UI; execution_enabled:false persists.
- [ ] EXECUTION BLOCKED until security review, isolated typed execution broker, exact per-tool permissions, idempotent journal/cancel, production deny policy and manual E2E tests. Target attestation and approval digest/TTL are now in place.
- [ ] Operation Center integration and SSH/production manual review pending.
- [x] Gate enforced: all plan responses have execution_enabled:false; no AI shell executor.

## Implementation sequencing
1. Branch + documents and handoff commits.
2. Server-side settings and private provider adapter; tests. Commit.
3. Settings UI + modal; tests. Commit.
4. Smart terminal side panel, context extraction and L1 calls; tests. Commit.
5. L2 context gathering with opt-in preview; tests. Commit.
6. L3 planning + approval only, isolated execution behind feature gate; tests. Commit.
7. Follow through safety review; update handoff each step.

## Validation
- JavaScript syntax, Node UI regression tests, Go 1.19/1.23 CI, Go unit tests.
- No outbound AI requests from frontend. Refuse arbitrary provider endpoints where SSRF can occur.
- Verify server never returns API keys, and settings writes do not log credentials.
- Verify disabled-by-default behavior and session role enforcement.
- Negative tests for prompt-injection/credential-bearing output.
- No claim that production-safe agent execution exists without passing end-to-end approval tests.

## Scope limits
This roadmap is ambitious and intentionally staged. A checked-in L1/L2 scaffold does not imply L3 execution is finished. Retain a truthful completion ledger in HANDOFF.
