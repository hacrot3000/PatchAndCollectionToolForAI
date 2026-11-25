# Unified run-bound long-task integration

Status: implementation candidate on integration branch; compile, race, native runtime and end-to-end regression verification remain required before main merge.

## Behavior

- A provider-bound run retains provider/model/configuration version/credential fingerprint and execution workspace for native read-only Collection and automatic text continuation.
- The existing native COLLECT queue and schema are reused. Only one unresolved, authorized Collection call is handled automatically. Results and the new immutable snapshot are persisted before the next attempt. Mixed or unsupported calls require review. Shared-server native Collection remains disabled.
- Handoff 18 TurnDisposition is reused for run completion, with explicit pinned and live owner consent, owner task-plan milestones, no-work brake, and ambiguity/concurrent-run fences.
- Handoff 17 exact native Patch evidence is preserved: provider-bound Patch records carry the originating run ID, exact argument digest and final result; feedback is prepared before scheduling and acknowledged only after a successful provider response to the bound run. Existing owner acceptance and exact-proof validators remain authoritative.
- Existing native Collection results that have already been reconciled do not block subsequent Patch proposals.
- The legacy session scheduler cannot create new provider egress for a run-managed conversation. Existing durable legacy PREPARED recovery remains separate.
- The main output budget and 600-second provider timeout / HTTP 499 retry policy are reused.
- UI shows task progress separately from run completion.

## Remaining verification and limitations

- Run-native Collection ZIP/TXT receipts are not equivalent to the older full_file_read proof schema. Do not accept a full-file milestone without an exact artifact/path/page proof. A secure canonical evidence adapter for these receipts is still required.
- End-to-end verification of auto-check on run-bound Patch receipts, the native Collection-to-Patch chain, manual recovery, and provider-switch concurrency is required. Legacy receipts without a provable run binding fail closed.
- Automatic Patch execution remains subject to explicit existing approval controls. Do not weaken these controls to unblock a long task.
- Run journal/history bounds, compaction, and snapshot drift after workspace mutations remain separate validation concerns. No unlimited-run claim is made.

## Regression commands

```sh
cd vscode_tasks_menu_go
go test -count=1 ./internal/server -run '^(TestAgentRunCollection|TestAgentRunContinuation|TestProviderToolCall)'
go test -race -count=1 ./internal/agentrun/... ./internal/agentchat/...
go test -race -count=1 ./internal/server -run '^(TestAgentRunCollection|TestAgentRunContinuation)'
go vet ./internal/server ./internal/agentrun/... ./internal/agentchat/...
node --check web/featuremods/agentchat_runs_ui.js
node web/agentchat_bootstrap_test.cjs
```

The tests added here use the existing mock provider/native Collection seam. They are not proof of a live provider call or real native Collection execution. No test result is asserted by this document.
