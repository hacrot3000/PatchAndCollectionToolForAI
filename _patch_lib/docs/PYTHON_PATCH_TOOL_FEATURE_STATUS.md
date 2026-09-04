# Python Patch Tool v6.18.0 feature status

| Capability | Status |
|---|---|
| Public zero-argument PATCH/COLLECT queue | COMPLETE |
| Exact cross-run recovery binding (`requeued_as` + SHA-256) | **COMPLETE v6.17.6** |
| Exact rollback replay protected from duplicate-history suppression | **COMPLETE v6.17.6** |
| Unbounded/no-target failed PATCH partial state fails safe as unknown | **COMPLETE v6.17.6** |
| Unsafe filesystem boundary clean fail-closed rc=2 | **COMPLETE v6.17.6** |
| POSIX `.sh` launcher | COMPLETE |
| Windows `.bat` + PowerShell launcher | COMPLETE |
| Windows Python 3.10+ discovery | COMPLETE |
| Windows internal PATCH/COLLECT routing without Bash | **COMPLETE v6.17.5** |
| POSIX fullscreen selector | COMPLETE |
| Windows fullscreen selector (`msvcrt` + VT) + line fallback | **COMPLETE v6.17.5** |
| PATCH priority / inspect / validate / health controls | COMPLETE |
| Exactly one COLLECT / no PATCH mix; no global queue lock | COMPLETE |
| Permanent PATCH in-place / SANDBOX removal | COMPLETE |
| Per-project PATCH mutation serialization (COLLECT/selector remain independent) | **COMPLETE v6.17.5** |
| Exact PATCH package schema + compatibility | COMPLETE |
| Multi-error manifest lint + migration hints | **COMPLETE v6.17.5** |
| Read-only `validate --patch` result classification | **COMPLETE v6.17.5** |
| Aggregate source SHA/existence/anchor diagnostics | **COMPLETE v6.17.5** |
| Sequential data-only OPS dry-run + managed execution timeout | **COMPLETE v6.17.5** |
| Partial-modification detection | COMPLETE |
| Metadata-driven exact-target rollback | COMPLETE |
| Batch effective-target snapshot + rollback verification | **COMPLETE v6.17.5** |
| Whole-batch mutation lock + generation-checked snapshot / POSIX dir-fd restore | **COMPLETE v6.17.5** |
| Planned-package SHA binding through batch snapshot / child spawn | **COMPLETE v6.17.5** |
| Replay snapshot SHA+size integrity before requeue | **COMPLETE v6.17.5** |
| Mutation lock symlink/reparse rejection + POSIX no-follow open | **COMPLETE v6.17.5** |
| Hardened artifact subdirectories + FAIL_HANDOFF fallback | **COMPLETE v6.17.5** |
| COLLECT regex isolated worker hard timeout | **COMPLETE v6.17.5** |
| Atomic OPS source write + explicit `already` semantics | **COMPLETE v6.17.5** |
| Windows process-tree timeout/Ctrl+C containment | **COMPLETE v6.17.5** |
| Windows reparse/junction-aware project safety | **COMPLETE v6.17.5** |
| Generic rollback without recovery metadata / Git-policy rollback | FAIL-CLOSED BY DESIGN |
| LAST_RUN / bounded history / resume / inspect | COMPLETE |
| Per-run batch summary + aggregate/detail logs | **COMPLETE v6.17.5** |
| Interactive/reopenable `report` browser + history navigation | **COMPLETE v6.17.5** |
| Structured diagnosis / FAIL_HANDOFF / narrowed source-drift COLLECT recovery | COMPLETE |
| Exact COLLECT schema + readonly actions | COMPLETE |
| Tool Health self-audit (`h`; compact on IDLE) | COMPLETE |
| Windows launchers included in Tool Health/SHA256 coverage | COMPLETE |
| Mandatory every-FAIL FAIL_HANDOFF + bounded related-source discovery | **COMPLETE v6.17.5** |
| Machine-readable `PATCH_PACKAGE_CHECKLIST.json` | **COMPLETE v6.17.5** |
| Historical private-core parity outside current contract | FAIL-CLOSED / NOT GUARANTEED |

## Windows portability notes

The dispatcher routes PATCH/COLLECT/inspect/validate/report directly through the packaged Python runtime, so native Windows zero-argument use does not require Bash. A native console gets fullscreen arrow/Space/priority controls when `msvcrt` input and VT output are available; non-TTY/unsupported consoles fall back to the stable line selector. External post-patch executables remain OS-dependent.

Batch execution defaults to `continue_independent`; explicit `fail_fast` remains available. After a contained failure, unrelated PATCHes continue automatically. Declared dependency failures and failed effective-target overlap render successors `BLOCKED` by default. Ctrl+C, rollback failure, or partial/unknown state still safety-stop. Smart Resume uses arrow-key descriptions and failed-PATCH multi-select for Retry/COLLECT/Delete. v6.17.9 extends the same rule to item-local read-only `PREFLIGHT_FAIL`: unrelated PATCHes continue under per-PATCH transactions; related successors are `BLOCKED`, while global/atomic preflight failures remain fail-closed.

## Stop condition

v6.17.6 completes the current robustness/data-integrity audit scope, aggregate/detail log browsing, diagnostics, Windows robustness/fullscreen parity and final audit scope. No new capability is started automatically after this release.

## v6.17.7 planning/policy state

- Project Identity Guard: COMPLETE.
- Trusted local validation profiles: COMPLETE.
- Persistent unresolved-failure registry: COMPLETE. It survives LAST_RUN replacement without blocking unrelated PATCHes; dependency/target-related or explicitly declared successors still require previous-failure handling; enforcement is per related selected PATCH, not the first batch item. Exact-SHA PASS is required for automatic registry resolution.
- Static effective-target conflict analysis: COMPLETE.
- Read-only batch `plan` + OPS diff preview: COMPLETE.
- Local patch ledger / ID reuse warning: COMPLETE.
- SHA-bound reproducible batch recipe: COMPLETE.
- Disk/resource preflight: COMPLETE.
- Queue search/filter: COMPLETE.
- Cryptographic signatures / PKI / remote provenance trust: NOT IMPLEMENTED.

## v6.17.8 execution audit

- Failure-only `manifest.on_failure.commands`: COMPLETE.
- Original PATCH failure rc remains authoritative; failure-command result is secondary evidence: COMPLETE.
- Managed timeout vs explicit exit 124 separation: COMPLETE.
- Post-exit descendant detection/cleanup on POSIX: COMPLETE.
- Git auto-policy timeout/process-tree containment and no-prompt mode: COMPLETE.
- Ctrl+C propagation through post/on-failure command sequences: COMPLETE.
- Non-interactive managed stdin and internal `PTV_*` control-env isolation: COMPLETE.
- Batch validate timeout graceful child cleanup: COMPLETE.
- COLLECT Git-context failure visibility/helper suppression: COMPLETE.
- Windows native runtime execution evidence for these changes: REQUIRES REAL WINDOWS HOST.

## v6.17.10 contract consistency

- Unresolved predecessor checks cover all persisted failures and the actually related successor.
- Multiple related unresolved predecessors fail closed until Smart Resume resolves/retries them.
- `on_dependency_failure=run_anyway` remains schema-readable only for compatibility; runtime blocking is mandatory.
- `plan` and exported recipes preserve effective batch policies and validate batch-transaction compatibility.
- Local project config parsing is unified across policy/identity/validation paths.
- `run --recipe` rejects CLI batch-policy overrides; the stored recipe policies are replay semantics.
- `patch.version/phase/phase_under_test/summary/regression_scope` are descriptive metadata, not implicit gates.
- `post_patch.run_when_no_changes=false` skips post commands for no-op/idempotent PATCHes; explicit `true` opts in.

## v6.17.12 zero-argument history + important artifact paths + live status

- Zero-argument interactive selector/Smart Resume expose HISTORY; an idle zero-argument TTY opens history after warnings/status/health.
- History defaults to the newest meaningful PASS and reuses the normal persisted report browser/artifacts.
- Best-effort fixed live PATCH status header is COMPLETE for supported TTYs with automatic plain-console fallback; `PTV_DISABLE_LIVE_STATUS=1` disables it. Raw saved logs remain authoritative.


- v6.17.13 history browser hides IDLE, renders package-first rows and pauses after duplicate-only queue cleanup. v6.17.14 corrects zero-work semantics: a genuinely empty zero-argument queue creates no run/log/state and does not auto-open HISTORY; automatic SMART RESUME requires a failed LAST_RUN with concrete recovery work still present in the current queue, while persistent predecessor safety remains enforced for related successors.


## v6.18.0 search discovery

- COMPLETE: filesystem-first search; untracked/gitignored visibility by default.
- COMPLETE: separate search budgets, coverage/skipped diagnostics, module inventory, zero diagnostics.
- COMPLETE: auto primary + independent fallback consistency check and `SEARCH_INCONSISTENCY`.
- COMPLETE: `must_find`, `anchor_paths`, `expected_files`, `COLLECT INCOMPLETE` diagnostic ZIP.
- COMPLETE: `health-search` disposable fixture.
