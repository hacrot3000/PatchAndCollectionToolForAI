# Agent Chat — strict command-only PATCH bridge

Incident: Agent could inspect an existing regression script but could not submit a manifest-only PATCH, and needed verified diagnostics and predecessor disposition for a failed atomic PATCH.

## Contract

The existing native runtime already supports command-only ZIPs under `legacy_strict`. The Agent Chat bridge now accepts `taskdeck_submit_patch` with `name`, `patch_python`, and `patch_manifest` (JSON **string** containing the manifest). Exactly one body is nonempty. New strict function schemas require both string fields; legacy Python-only saved proposals remain readable. Text fallback uses the existing `taskdeck_patch_proposal` envelope with the same fields and exact project identity.

Example manifest (substitute the actual identity and existing reviewed script):
```json
{
  "schema_version": 1,
  "project": {"key": "ACTUAL_KEY", "id": "ACTUAL_ID"},
  "patch": {"id": "run-existing-regression"},
  "post_patch": {
    "run_when_no_changes": true,
    "no_change_reason": "Run the existing regression without changing project source.",
    "safety_profile": "legacy_strict",
    "commands": [{"argv": ["bash", "scripts/regression.sh"], "cwd": ".", "timeout_seconds": 300}]
  }
}
```

The Go bridge validates the narrow structural subset, binds project key/id, and uses the existing native paste/queue/approval/SHA receipt path. ZIP contains only PATCH_TOOL_MANIFEST.json. Native preflight checks actual executable, script existence, paths, arguments, project policy, failure lineage, and transaction policy. It is not a sandbox for script internals. Review existing script and use optional native preflight.files SHA-256 pinning where possible. No inline evaluator, sudo, arbitrary PATH executable, payload, git, SSH, validation, on_failure, or batch.previous_failure override is admitted by this lane. Use project root cwd to avoid ambiguity between reviewed script and actual execution path. No new script is generated or run in the same package. No automatic retry of uncertain execution.

A passing command-only regression does not resolve an earlier failed source-changing PATCH. The failure evidence projection retains the prior relevant failure across a successful active command-only call; explicit run references remain supported by the existing CLP-10 resolver. Missing/unverified artifacts remain explicit. This does not automatically retry/delete predecessors or prove rollback. Related source-changing successors still obey native lineage requirements; this narrow bridge does not implement general source payload + arbitrary manifest support.

## Progress and evidence (2026-11-25)

- [x] Agent manifest schema, text fallback, review rendering and existing approval/queue path: implemented in 8fc659fe.
- [x] Conservative argv shape, root cwd, and prior failure projection: implemented in 83bc345f.
- [x] Added regression source for schema, packaging, malformed fields/argv, identity, approval/duplicate admission, text fallback, failure context and UI.
- [x] Isolated helper test: exact Git blob 281a6fd91bb094723346a092f34872056083c7c3 copied to temporary Go module with stub Python branch and identity type; three tests PASS (manifest-only ZIP, unsafe argv, unsafe fields/identity), go vet PASS. This is not full-server or native runtime verification.
- [ ] Full Go and browser bootstrap regression execution. Container checkout blocked by DNS; no full-suite PASS claimed.
- [ ] Native end-to-end command execution and failure receipt delivery on the user's actual project. Actual atomic FAIL artifact and regression script are not available here.

Suggested checks from repo root:
```sh
cd vscode_tasks_menu_go
go test ./internal/server -run 'TestAgentCommandManifest|TestAgentCommandOnlyPass'
go test ./internal/agentchat -run TestPatchToolAdvertisesBothProposalLanes
node web/agentchat_bootstrap_test.cjs
go test ./internal/server ./internal/agentchat/...
```

Status: IMPLEMENTED_UNVERIFIED for full integration. No UAG dependency gates changed.


## 2026-11-27 — instruction clarity and manual-execution avoidance

- Agent Chat refreshes the same explicit automatic local script workflow on each new request, including tool feedback and continuation. Function and text-only examples are built from the verified project key/ID and use a JSON **string** for `patch_manifest`.
- Direct shell/terminal tools remain unavailable. This is **not** a prohibition on reviewed, pre-existing project scripts through the approved native command-only PATCH lane. Prefer that lane for eligible compile, build, test and diagnostic scripts. A genuinely required new script is created in a separate successful PATCH before a second command-only PATCH can invoke it; never use a wrapper to bypass the whitelist.
- Tool descriptions, unsupported-tool feedback and Ask User guidance distinguish execution from approval; manual execution is a last resort for a real out-of-scope operation or verified blocker. Run Manager current capability guidance explicitly supersedes older snapshot tool guidance, and permits the lane only when its Patch tool is actually advertised. Existing pinned prepared request bytes are not rewritten by this change.
- Regression source: `TestAgentExistingScriptInstructionsUseAcceptedManifest` and `TestAgentExistingScriptInstructionsDistinguishDirectShellFromNative`. Native parser, execution policy, script origin guard, approval, failure lineage and SSH whitelist are unchanged.
- Full Go/CI and live provider behavior are not verified in this environment. An instruction improves model behavior but cannot guarantee compliance.


## 2026-11-27 — automatic failed-test predecessor recovery

- A native test/compile failure is diagnostic evidence, not an automatic model BLOCKED state. A command-only package is identified only from the exact SHA-bound archive and strict manifest; it has no PATCH payload, but the script may still have side effects. Nim source hashes are scoped.
- The native batch planner must not resurrect an exactly retired predecessor from stale LAST_RUN: exclusion requires the exact native registry identity and run, resolved retirement, absent queue file and matching archived SHA. Unresolved, mismatched, requeued or rolled-back evidence remains protected. Retirement is never a rollback or a claim that source was repaired.
- The local Agent reconciles eligible failed packages on subsequent fresh messages, tool feedback and native COLLECT continuation, including recovery when native archive/unlink happened but registry or Agent receipt persistence was interrupted. Absence alone is never proof of retirement. No failed build is replayed automatically. The native retirement command is bounded; an actual refusal is not ignored.
- Failure projections remain transient, tied to the original ledger call. Exact predecessor disposition is included through subsequent read-only COLLECT and Ask User feedback, and a known retirement plus verified archive remains visible after bounded History pruning. No historical canonical tool receipt is duplicated.
- AI must inspect current relevant source and use native PATCH proposals/preflight/planning rather than require a separate human planning confirmation. Only ask the owner when a genuine native constraint or a decision requiring the owner remains. Real dependency, rollback, source integrity, transaction and approval rules remain in effect.
- Regression source: test_agent_failed_queue.py and agent_chat_failure_evidence_test.go. Full Go/Python and actual reported run 1795779890654600_1055702 remain unverified in this environment; do not report that test as PASS.
