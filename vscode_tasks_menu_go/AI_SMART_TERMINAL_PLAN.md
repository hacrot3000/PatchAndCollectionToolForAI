# TaskDeck Smart Terminal AI — Implementation Plan

Date: 2026-10-13
Base: `main`
Working branch: `feature/ai-smart-terminal-l1-l3`
Owner: TaskDeck
Status: L1 advisory MVP + L2 opt-in preview implemented; L3 preview/approval-only. Autonomous or approved command execution NOT IMPLEMENTED. Incremental commits on isolated feature branch.

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
- [x] Inventory existing shell integration and auth/secret storage
- [x] Capability/deployment threat model for L1/L2 and plan-only L3 (see operator guide); L3 execution threat model still pending

### L1 — Smart assistant (suggest-only)
- [x] Settings > AI Assistant modal: enabled, OpenAI model, write-only encrypted API key, timeout, output cap, per-user quota, context flags and consent. Provider endpoint fixed to OpenAI; custom provider selection NOT implemented.
- [x] Backend validation, server auth, fixed endpoint (no open-proxy/SSRF), bounded payload/deadlines, encrypted key, no key on GET; local per-user quota.
- [x] Terminal-contextual Explain/Suggest + plan preview; independent side panel with answer and copy; never auto execute.
- [~] Error diagnostics, unavailable key/provider responses and cancel implemented; separate connection-test button, streaming and real provider E2E still pending.
- [x] Server-side security/unit tests, UI static tests, Node plan/parser tests, operator guide and feature-branch CI.

### L2 — Context aware assistant
- [~] Bounded current terminal command/output, CWD, recent command history, Git UI summary; full server-verified session context/exit code not yet completed.
- [~] Local/SSH indicator, shared authorization; server-attested target identity required before any future execution.
- [x] Explicit preview, best-effort redaction, scope flags and bounded/truncated context. Users must inspect for secrets manually.
- [~] Consent/auth/redaction/budget tests completed; more adversarial prompt-injection and stale-session tests pending.

### L3 — Guided agent
- [~] Private persistent plan/step schema, pending/approved/rejected decisions and user-scoped audit. No execution task state yet.
- [~] Captured proposal target and risk labels. No real tool registry or server-attested execution identity yet.
- [~] Per-step explicit confirmation and record-only decision; execution globally disabled so all commands remain inert.
- [x] Plan preview and durable per-user decision journal; proposals not executed.
- [ ] EXECUTION BLOCKED until security review, server-authenticated target, sandbox/typed broker, approval binding and E2E tests.
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
