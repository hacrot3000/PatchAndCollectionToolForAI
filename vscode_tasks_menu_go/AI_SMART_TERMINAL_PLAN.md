# TaskDeck Smart Terminal AI — Implementation Plan

Date: 2026-10-13
Base: `main`
Working branch: `feature/ai-smart-terminal-l1-l3`
Owner: TaskDeck
Status: IN PROGRESS — incremental commits, never claim L3 ready until security/functional gates pass.

## Product goals
Keep xterm.js/PTY behavior unchanged. Add an optional Smart Terminal assistant with an accessible Settings > AI Assistant dialog, scoped context, cost/timeout controls, and an independently controlled AI side panel. No AI API key is ever sent to browser, logs, Git, clipboard, workspace snapshots or shared sessions.

## Architecture and invariants

- AI remains opt-in; disabled by default. Backend enforces authorization, not UI alone.
- L1 (suggest-only): explicit submit -> configured OpenAI-compatible API -> text answer. No shell/Git/file/database side effects. Support explain-error, explain-command, suggest-next-step via bounded recent command/output.
- L2 (context-aware): user-selectable, bounded CWD, Git branch/status, file reference and session command history. Every context source labeled untrusted; secrets redact before transmission. Never auto-upload files/projects/logs. Explicit disclosure before external model calls.
- L3 (guided): plan proposals represented as structured actions, with per-command allow/deny, immutable target (user/project/session/host/cwd), approval and audit. Never permit LLM raw shell escape to execute, never auto-approve destructive commands, and never run on production without dedicated explicit approval. Begin with preview-only proposal pipeline before executing in isolated, permission-checked tool runner.
- Every action associated with auth user, project, workspace, host and session; no permissions derived from prompt text.
- A stopped/failed/reloaded task must not retry side effects silently. Idempotency keys + durable journals required for real L3 execution.
- Sensitive content (ANSI escapes, terminal control codes, passwords, bearer tokens, SSH private keys) excluded or masked before model submission. Untrusted output is DATA, never policy.
- No remote provider access before user configured endpoint/provider and consent; no hardcoded keys.

## Milestones, deliverables, acceptance gates

### Milestone A — foundation & planning
- [x] Branch created from main
- [x] PLAN and HANDOFF, first independent commits
- [ ] Inventory existing shell integration and auth/secret storage
- [ ] Capability- and deployment-specific threat model

### L1 — Smart assistant (suggest-only)
- [ ] Settings > AI Assistant opens modal with enable, provider, endpoint, model, API key storage (server-only), timeout, token/output limits, privacy preferences and consent.
- [ ] Backend request validation, auth/permission, no open-proxy/SSRF, bounded payload and deadlines; private server key; no sensitive settings in GET.
- [ ] Terminal contextual Explain command, Explain error, Suggest fix; independent panel with answer and copy. Never auto execute.
- [ ] AI connection test, error diagnostics, unavailable provider handling, cancel and accessibility.
- [ ] Unit/integration tests + docs and CI gate.

### L2 — Context aware assistant
- [ ] Backend bounded context builder: command + exit code + selected output, session/CWD, Git metadata as opt-in.
- [ ] SSH/prod identity explicit; refuse cross-project/cross-user access.
- [ ] Explicit context preview, redaction, scope toggles and truncation indicator.
- [ ] Tests for permissions, token redaction, large outputs, stale session and hostile log instructions.

### L3 — Guided agent
- [ ] Plan schema and task-state machine (draft -> approval -> execution -> results -> completed/aborted).
- [ ] Backend tool registry and permission matrix, immutable execution target, risk levels.
- [ ] Per-step user confirmation, reject unsafe commands, read-only by default.
- [ ] Dry-run/plan preview with evidence and audit log first.
- [ ] Only then optional execution broker, with timeout/cancel/locks/limits/journaling and no automatic retries.
- [ ] Operation Center integration, integration tests, manual prod/SSH security review.
- [ ] Gate: real L3 execution remains disabled until security review and explicit acceptance.

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
