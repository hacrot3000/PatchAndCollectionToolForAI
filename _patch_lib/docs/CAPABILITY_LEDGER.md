# Python Patch Tool — cumulative capability ledger

## Current additive capability — cryptographic provenance / signature trust

- **COMPLETE (current capability; historical #65 was NOT STARTED):** PATCH manifests may carry a strict Ed25519 provenance signature (`ptv-patch-signature-v1`).
- **FAIL-CLOSED:** a present signature is verified against operator-local trusted public keys before payload/source mutation; tampered manifest/package content, malformed signature data, or an untrusted signer is rejected.
- **LOCAL POLICY:** `.python_patch_tool.json` may set `provenance.require_signature=true`, which also rejects unsigned recognized legacy PATCHes. Default remains compatible with existing unsigned PATCHes.
- **SCOPE BOUNDARY:** verifier/trust policy only. Private-key generation/management, PKI, remote trust registry/network lookup, COLLECT signing and reproducible raw ZIP-byte signing remain out of scope.
- Behavioral gate: `self_test_provenance_signature_v6_21_0.py`. Contract: `PROVENANCE_SIGNATURE_TRUST.md`.
- Historical `CURRENT_CAPABILITY_DISPOSITION.json` remains 95/95 COMPLETE coverage; historical #65 is not rewritten as if it had been COMPLETE in v5.


## v6.20.0 safe Git + human-only manual execution

- **PRESERVED + HARDENED (#43):** COLLECT Git context now uses a strict operation allowlist only: `status`, `current_branch`, `branches`, `log`, `show`, `diff_worktree`, `diff_staged`, `diff_refs`, `diff_ref_worktree`, and safe `switch`. Nested repositories are supported through an explicit project-relative `repo`. There is no raw command/argv Git escape hatch.
- **REMOVED_BY_REQUIREMENT (#7–8):** PATCH-side automatic Git mutation (`add`, `commit`, `push`, and related mutation automation) is intentionally retired by the v6.20.0 safety requirement. Historical rows/evidence remain in this ledger and the machine-readable disposition file; legacy manifest fields are parsed only to return a precise rejection.
- **ADDITIVE:** PATCH packages may declare `manual_execution` with structured argv steps. The tool only renders instructions and verifies operator-provided/captured evidence; it never executes those step commands. Results are aggregated into ZIP + TXT companions and retained in HISTORY/FAIL_HANDOFF evidence.
- **ADDITIVE:** `payload=manual_only` permits a workflow package that intentionally performs no source mutation. Non-interactive execution fails before payload mutation when a manual workflow is required.
- Behavioral gates: `self_test_git_safe_v6_20_0.py`, `self_test_manual_execution_v6_20_0.py`.
- User-facing contracts: `GIT_SAFE_OPERATIONS.md`, `MANUAL_EXECUTION_WORKFLOW.md`.


## v6.20.0 current capability — persistent failed queue state

- **PRESERVED + EXTENDED:** v6.19.4 normal-queue grouping is now backed by persistent unresolved state rather than LAST_RUN.
- **COMPATIBILITY_RESTORED:** failed COLLECT items persist across unrelated successful runs and v6.19.4 HISTORY is migrated.
- **PRESERVED:** exact SHA identity and PATCH planner safety; COLLECT failures do not constrain PATCH dependencies.
- Behavioral gate: `self_test_failed_queue_persistence_v6_20_0.py`.


## v6.19.4 current capability — failed-work grouping without startup hijack

- `PRESERVED/UX-SUPERSEDED`: Smart Resume recovery operations remain available through the explicit `resume` command.
- `COMPATIBILITY_RESTORED/UX`: ordinary zero-argument queue startup is no longer replaced by a recovery prompt; immediately previous failed/replay items are grouped below new queue work.
- Grouping is presentation-only and must never create a second execution/delete/inspection implementation.
- Behavioral gate: `self_test_failed_queue_grouping_v6_20_0.py`.

Current release: **v6.20.2**

This is the canonical cross-version continuity ledger. It complements the current feature-status document; it must never be replaced by a current-only checklist.

Status vocabulary:

- **PRESERVED** — current runtime still provides the historical behavior or a behaviorally compatible equivalent.
- **COMPATIBILITY_RESTORED** — behavior had regressed and is restored in the named version.
- **SUPERSEDED** — intentionally replaced by a newer documented contract; old behavior is retained here for history.
- **REMOVED_BY_REQUIREMENT** — explicitly removed by a later requirement/architecture decision.
- **NOT_CURRENTLY_GUARANTEED** — historical capability existed but current self-contained v6 package does not claim equivalent parity; it must remain visible here rather than silently disappearing.

See `HISTORICAL_FEATURE_BASELINE_V5_15.md` for all 107 original names. Machine-readable complete-ID coverage is enforced by `CURRENT_CAPABILITY_DISPOSITION.json`.

## v6.18.8 report/history AI-upload highlighting

- **PRESERVED + EXTENDED:** capability #100 primary handoff highlighting now also applies when the same COLLECT result / FAIL handoff / recovery COLLECT is viewed later through report/HISTORY.
- **PRESERVED:** capability #101 accessible color roles keeps `NO_COLOR`/non-TTY plain output; ANSI is presentation only.
- Problem states `INCOMPLETE` and `PREFLIGHT_FAIL` receive visible status emphasis without changing persisted report data.
- Semantic gate: `self_test_history_artifact_highlight_v6_20_0.py`.

## v6.18.7 search timeout / bounded-result preservation

- **PRESERVED + HARDENED:** historical/current Search collector remains coverage-aware and now preserves already found evidence across regex watchdog timeout. Timeout becomes `PARTIAL`/COLLECT `INCOMPLETE`, not destructive action failure.
- **PERFORMANCE-PRESERVING CONTRACT:** `fallback_search=true` verifies zero/error independently; a positive primary result skips the second full content scan by default while retaining coverage inventory. `verify_nonzero_with_fallback=true` restores explicit positive-result backend cross-checking when required.
- **BOUNDED OUTPUT IS EXPLICIT:** when `max_matches` omits match detail, search status is PARTIAL/INCOMPLETE; fully scanned/untruncated actions report `Search execution status: COMPLETED`. Discovery-driven file quotas keep the prefix already copied and mark omitted remainder INCOMPLETE; exact-file `pack` stays fail-closed.
- Semantic evidence: `self_test_search_partial_timeout_v6_18_7.py`, `self_test_search_discovery_v6_18_7.py`, and historical COLLECT continuity gates.

## v6.18.6 upload-required highlight preservation

- **PRESERVED/strengthened:** primary upload artifact labels remain authoritative in plain text.
- **Presentation additive:** TTY/VT output highlights `[PRIMARY - UPLOAD THIS FILE]`, `ACTION REQUIRED`, and the exact ZIP path with a high-contrast yellow background; the path is underlined.
- **Compatibility:** `NO_COLOR`, redirected output, Windows/non-VT fallback and copyable artifact paths retain plain-text semantics.
- **Behavioral gate:** `self_test_upload_action_highlight_v6_18_7.py`.

## v6.18.5 discovery/glob false-zero hardening

- **PRESERVED + HARDENED:** historical/current `find` remains filename/path discovery but now evaluates patterns against basename, each requested scope-relative path, and the full project-relative path. This fixes false zero for `paths=[".../jdqs_server"]` with `patterns=["src/main/java/.../*.java"]` without removing any prior matching view.
- **PRESERVED + HARDENED:** `directory` include/exclude and `find` glob patterns now implement `**/` as zero-or-more directory levels, so `**/*.java` includes direct children as well as nested files.
- **PRESERVED + HARDENED:** `find` traversal uses `limits.max_search_files`; `limits.max_files` remains a packaging quota only. A discovery-budget truncation reports `Coverage status: PARTIAL` and marks the COLLECT result `INCOMPLETE` instead of silently claiming trustworthy zero coverage.
- Semantic evidence: `self_test_find_discovery_v6_18_7.py` plus the existing historical COLLECT/search/continuity gates. No historical COMPLETE disposition is removed or narrowed.

## v6.18.4 final continuity closure

Diagnostics/validation items marked below are **COMPATIBILITY_RESTORED v6.18.4** unless a row explicitly records a supersession.

| Historical IDs | v6.18.4 disposition | Evidence / notes |
|---|---|---|
| 18–22, 24–25, 28 | **COMPATIBILITY_RESTORED** | FAIL_HANDOFF now adds redacted detail, normalized diagnostics, root-cause clustering, smart filtering, environment fingerprint, quality metrics and failure delta under `compat_diagnostics/`. |
| 23 | **SUPERSEDED + redacted derivative retained** | v5 redact-before-persistence conflicts with the later exact-evidence requirement. Exact bytes remain with warning; redacted derivative supports safe sharing/analysis. |
| 26 | **SUPERSEDED physically / logical layers preserved** | Separate SUMMARY/CODE/DETAIL ZIPs are not resurrected; compact/deep evidence lives inside unified FAIL_HANDOFF. |
| 27 | **PRESERVED** | Unified FAIL_HANDOFF remains the primary PATCH-failure upload artifact. |
| 58 | **COMPATIBILITY_RESTORED** | Trusted local `validation.selection` computes profiles from actual changed paths after payload/post commands. |
| 59 | **COMPATIBILITY_RESTORED** | Safe bounded diagnostic rerun after primary validation failure; never converts FAIL to PASS. |
| 1 / HISTORY landing behavior | **PRESERVED / hardened** | Zero-work zero-argument invocation opens HISTORY in TTY and prints HISTORY without blocking in captured/non-TTY task runners; no fake run state. |

Every one of the 95 historical COMPLETE IDs now has exactly one explicit current disposition in `CURRENT_CAPABILITY_DISPOSITION.json`; omission is a release-gate failure.

## v6.18.2 compatibility restoration

| Historical ID | Capability | v6.18.2 disposition | Evidence / notes |
|---:|---|---|---|
| 69 | Command-only package | **COMPATIBILITY_RESTORED** | Manifest-only package is accepted when `post_patch.run_when_no_changes=true`, a 20–500 char `no_change_reason` is supplied, and exactly one strict-compatible command is requested. |
| 70 | Restricted no-change override | **COMPATIBILITY_RESTORED** | `no_change_reason` restored to schema; command-only lane is bounded to one command. |
| 71–73 | Basic allowlist / project-local script boundary / inline rejection | **COMPATIBILITY_RESTORED** | `legacy_strict` safety lane; command-only packages automatically use it. Source-changing v6 PATCHes retain the newer v6 post-patch contract unless `safety_profile=legacy_strict` is explicitly requested. |
| 74 | Command timeout/process supervision | **PRESERVED** | Current managed command execution retains timeout/process supervision. |
| 75 | Command-aware delta/validation | **PRESERVED (current v6 semantics)** | Current runner evaluates project changes around post commands; old SANDBOX-specific semantics are not resurrected. |
| 76 | Secret-like command-argument guard | **COMPATIBILITY_RESTORED** | Strict compatibility lane rejects credential-like argv. |
| 77 | Idempotency-before-command ordering | **SUPERSEDED** | The exact v5 behavior depended on the transaction SANDBOX, which was later removed by requirement. Do not claim exact parity. |
| 86 | Repeated explicit `--patch` | **COMPATIBILITY_RESTORED v6.18.2** | Dispatcher executes every explicitly selected package; behavioral test required. |
| 89 | Explicit non-interactive `--all` | **COMPATIBILITY_RESTORED v6.18.2** | `--all`/`-a` once again selects runnable PATCH items non-interactively; config-driven selection remains supported by current policy. |
| — | `--select` public route | **COMPATIBILITY_RESTORED v6.18.2** | Accepts numeric/range selection through the dispatcher instead of being routed to a runner that cannot parse it. |
| 90 | Legacy v4 standalone execution | **PRESERVED** | Positive v4 recognition remains supported. |
| 91 | Legacy v4 archive execution | **COMPATIBILITY_RESTORED v6.18.2** | Multiple recognized Python scripts execute in sorted relative-path order; old all-Python fallback remains limited to positively patch-named archives. |
| 92 | v4 helper API compatibility | **COMPATIBILITY_RESTORED v6.18.2** | `run_patch`, old `apply_ops(root,name,ops)`, failed-file ZIP helpers and legacy command flags are available additively alongside current OPS API. |
| 93 | Strict-policy legacy exception | **PRESERVED / RESTORED** | Recognized v4 packages remain explicitly unscoped and use compatibility handling instead of current manifest requirements. |
| 94 | Unscoped legacy project safety | **PRESERVED** | Runtime reports `PROJECT_SCOPE_VERIFIED: FALSE` for legacy input. |
| 95 | Mixed v4/v5 selected queue | **PRESERVED within current queue model** | Explicit multi-selection may contain recognized legacy and current packages. |
| 96 | Legacy-vs-handoff discrimination | **PRESERVED** | Positive recognition is required; report/handoff/tool distributions are not treated as legacy patches. |
| 97 | Legacy report metadata | **COMPATIBILITY_RESTORED v6.18.2** | Package format, compatibility flag and project-scope verification are emitted/recorded. |

## Major historical transitions that must not be forgotten

| Historical area | Current disposition | Reason / replacement |
|---|---|---|
| #13 Transaction SANDBOX | **REMOVED_BY_REQUIREMENT** | v6 moved to explicitly in-place PATCH execution with preflight, mutation locks and optional exact-target rollback. Never silently reintroduce SANDBOX as default. |
| #5 stop-on-first-failure default | **SUPERSEDED** | Current batch policy is `continue_independent`; related/dependent successors block, `fail_fast` is explicit. |
| #52 automatic project-key adoption | **SUPERSEDED** | Current Project Identity Guard is stricter; project identity must agree with local configuration when used. |
| historical global queue/process lock | **REMOVED_BY_REQUIREMENT** | Per-project PATCH mutation serialization replaced global locking; selector/COLLECT remain independent. |

## Historical COLLECT capability mapping (#29–46) — restored in v6.18.3

The v5 collector was broader than the later self-contained v6 contract. v6.10 intentionally stopped treating the v5 action table as authoritative because the then-installed private collector did not implement it. v6.18.3 restores the useful read-only capabilities into the packaged self-contained collector **without restoring the old public `collect <command>` CLI**; delivery remains request-ZIP + zero-argument queue.

| Historical capability | v6.18.3 disposition / current behavior |
|---|---|
| `ls`, `tree` (#29–30) | **COMPATIBILITY_RESTORED v6.18.3** as bounded standalone actions. |
| Project overview (#31) | **PRESERVED** as `overview`. |
| Research (#32) | **COMPATIBILITY_RESTORED v6.18.3** as `overview` + coverage-aware verified search. |
| Find/glob (#33) | **PRESERVED** as `find`. |
| File/range, head/tail (#34–35) | **COMPATIBILITY_RESTORED v6.18.3** as bounded line/content readers. |
| Symbol extraction (#36) | **COMPATIBILITY_RESTORED v6.18.3** with function/class/struct-like block extraction from an exact source file. |
| Search (#37) | **PRESERVED and hardened**; v6.18 adds filesystem-first coverage, independent fallback, `must_find`, zero diagnostics and health-search. |
| References (#38) | **COMPATIBILITY_RESTORED v6.18.3** through the current coverage-aware filesystem search engine. |
| Callgraph (#39) | **COMPATIBILITY_RESTORED v6.18.3** as bounded source-level references/callers plus heuristic callees. |
| Dependencies (#40) | **COMPATIBILITY_RESTORED v6.18.3** as bounded include/import/use/require inventory. |
| Directory collector (#41) | **COMPATIBILITY_RESTORED v6.18.3** with include/exclude globs and automatic sensitive-file avoidance. |
| Multi-path pack (#42) | **PRESERVED for exact files / intentionally narrowed for directories**. v6.11 explicitly made guaranteed `pack` exact-file evidence; use restored `directory` for recursive subtrees. Historical `zip` is accepted as an exact-file alias. |
| Git context (#43) | **PRESERVED** as `git`. |
| Large decompile (#44) | **COMPATIBILITY_RESTORED v6.18.3** using a temporary SQLite index, address/name/regex lookup, neighbors and optional text references. |
| JSON multi-action (#45) | **PRESERVED / EXPANDED** across the current and restored action set. |
| Collector path/security policy (#46) | **PRESERVED / strengthened** by project-relative containment, no arbitrary request shell, bounded collection, sensitive auto-collection avoidance and search coverage diagnostics. |

### Additional historical/private request aliases

Real historical request ZIPs used `search_files`, `content`, and `symbol_graph`. v6.18.3 marks these **COMPATIBILITY_RESTORED** and protects them with semantic regression tests. `search_files`/`content` route through the hardened search engine; `symbol_graph` preserves the multi-symbol reference/caller/callee/dependency investigation shape.

### Public CLI disposition

The old direct public form `./tools/run_python_patches.sh collect <command> ...` remains **SUPERSEDED** by the later queue contract. This is deliberate and is not a missing capability: the research/collection *actions* are restored inside request ZIPs while the user continues to run the zero-argument launcher.

## Portable install and selection continuity (#78–89)

| Historical ID | Capability | Current disposition | Evidence / notes |
|---:|---|---|---|
| 78 | Extract-and-run installation | **PRESERVED** | Primary install/upgrade remains direct extraction of the portable `tools/` tree. |
| 79 | Correct public-runner placement | **PRESERVED** | `tools/run_python_patches.sh` remains the public POSIX entry point; `_patch_lib` stays private. |
| 80 | Portable direct upgrade | **PRESERVED** | Release ZIP does not include `.python_patch_tool.json`; extraction replaces only Patch-Tool-managed package paths. |
| 81 | Optional controlled installer | **COMPATIBILITY_RESTORED v6.18.3** | `install_python_patch_tool_v6.py` plus historical filename wrapper `install_python_patch_tool_v5.py`; fixed-list stale-file backup/removal, dry-run and create-config-without-overwrite. Normal use does not require it. |
| 82 | Portable-layout regression test | **PRESERVED / EXPANDED** | Package/version/checksum tests plus `self_test_portable_installer_v6_18_7.py` validate direct layout and controlled migration semantics. |
| 83–85 | Interactive / TTY / line multi-select | **PRESERVED** | Current selector keeps fullscreen and line-mode selection contracts. |
| 86 | Repeated explicit `--patch` | **COMPATIBILITY_RESTORED v6.18.2** | Dispatcher-level semantic test, not launcher-string inspection. |
| 87 | Unselected-package preservation + audit | **COMPATIBILITY_RESTORED v6.18.3** | Unselected runnable packages remain in `patchs/` and `LAST_RUN.json` records `user_not_selected`. |
| 88 | Selection-aware automatic identity adoption | **SUPERSEDED** | Current Project Identity Guard intentionally does not auto-adopt identity from a selected PATCH; configured local identity must already agree. |
| 89 | Explicit non-interactive automation | **COMPATIBILITY_RESTORED v6.18.2** | `--all`/`-a` and current config automation both remain available. |

## Output/UI historical capabilities (#98–107)

The v6 report/history model superseded the exact v5 split-bundle naming model. Preserve current equivalents but do not claim byte/name parity with v5.

- #98 absolute critical local paths — **PRESERVED where actionable artifacts are printed**.
- #99 output-file role guide — **COMPATIBILITY_RESTORED v6.18.3** as `OUTPUT_FILES_GUIDE.md`, describing the current v6 artifacts instead of resurrecting obsolete bundle names.
- #100 primary upload highlighting — **PRESERVED** for COLLECT results and PATCH FAIL handoff paths.
- #101 ANSI color roles — **PRESERVED as current accessible equivalents / exact v5 palette SUPERSEDED**; `NO_COLOR` and text fallback remain supported.
- #102 REPORT/DETAIL alias clarification — **SUPERSEDED** because current v6 no longer exposes that split-bundle alias model; `OUTPUT_FILES_GUIDE.md` explicitly records the historical distinction.
- #103 persistent LAST_RUN file guide — **PRESERVED as a current equivalent** for `LAST_RUN.json`/HISTORY; the exact `LAST_RUN.md` artifact is **SUPERSEDED**.
- #104 color/text run states — **PRESERVED current equivalents**.
- #105 executed-package traceability — **PRESERVED in run/history evidence**.
- #106 old short handoff naming — **SUPERSEDED** by current FAIL_HANDOFF/report naming.
- #107 selector deletion — **PRESERVED**.

## Current critical continuity gates

The following are current-release capabilities and are protected in addition to the historical v5 baseline:

- zero-argument PATCH/COLLECT queue and single-item default;
- empty runnable queue lands on HISTORY in TTY and prints HISTORY in non-TTY task runners, while creating no fake run/LAST_RUN/history/ledger entry;
- Smart Resume, unresolved-failure registry, recovery menu and FAIL_HANDOFF;
- duplicate filtering and `patchs/ignore` behavior;
- current PATCH schema/preflight/rollback/batch mutation protections;
- POSIX and native-Windows launcher routing;
- current/restored COLLECT actions and aliases defined by `COLLECT_ACTION_SCHEMA.json`, including `pack`, `overview`, `find`, `search`, `git`, historical read-only actions, and protected `search_files`/`content`/`symbol_graph`;
- filesystem-first verified search, independent fallback, coverage/skipped-dir reporting, `must_find`, `diagnose_on_zero`, anchors/expected-files and `health-search`.

## Release rule

No future release may remove a row from this ledger. A changed capability must receive a new disposition and behavioral evidence. See `NO_SILENT_REMOVAL_POLICY.md`.

### v6.18.7 bounded COLLECT final status

A COLLECT that preserves usable evidence but cannot prove full coverage (timeout, result/report truncation, or discovery output quota) exits with `rc=3`, writes the result ZIP, and reports `SUMMARY: INCOMPLETE` rather than `SUMMARY: FAIL`. `FAIL` remains reserved for execution/schema/integrity failures.


## v6.19.1 current capability — ZIP clear-text companions

| Capability | State | Behavioral evidence |
|---|---|---|
| COLLECT result ZIP + same-stem TXT | PRESERVED/NEW | `self_test_cleartext_companion_v6_20_0.py` |
| FAIL_HANDOFF ZIP + same-stem TXT | PRESERVED/NEW | `self_test_cleartext_companion_v6_20_0.py` |
| Per-entry path/type/size/hash/description/content boundaries | PRESERVED/NEW | `self_test_cleartext_companion_v6_20_0.py` |
| Binary evidence retained as Base64 | PRESERVED/NEW | `self_test_cleartext_companion_v6_20_0.py` |
| Safe nested ZIP recursive expansion | PRESERVED/NEW | `self_test_cleartext_companion_v6_20_0.py` |
| HISTORY/report exposes and highlights TXT alternate | PRESERVED/NEW | `self_test_cleartext_companion_v6_20_0.py` |

**No-silent-removal rule:** the companion is part of the AI handoff contract, not temporary presentation. ZIP remains preferred, but a later release must not remove the TXT alternate or silently omit ZIP members from its representation.

## v6.19.0 current capability — SELECT-only database evidence

| Capability | State | Behavioral evidence |
|---|---|---|
| `database_select` active builder; no raw SQL | PRESERVED/NEW | `self_test_database_select_v6_20_0.py` |
| SQLite read-only DB collection | PRESERVED/NEW | `self_test_database_select_v6_20_0.py` |
| MySQL loopback + login-path auth | PRESERVED/NEW | `self_test_database_select_v6_20_0.py` |
| MySQL SSH-tunnel transport | PRESERVED/NEW | `self_test_database_select_v6_20_0.py` |
| JOIN/subquery/AND-OR/GROUP BY/HAVING/CASE/window AST | PRESERVED/NEW | `self_test_database_select_v6_20_0.py` |
| DB partial output => COLLECT INCOMPLETE, evidence retained | PRESERVED/NEW | `self_test_database_select_v6_20_0.py` |
| Local DB profile hard exclusion from COLLECT/search/FAIL_HANDOFF evidence | PRESERVED/NEW | `self_test_database_select_v6_20_0.py` |

**No-silent-removal rule:** later releases may extend the active builder additively, but must not introduce raw SQL, write-capable statements, embedded password fields, or direct remote MySQL TCP as a silent replacement for this safety boundary.

## v6.19.2 current capability — stateful AI tool-context synchronization

- **PRESERVED + EXTENDED:** old PATCH/COLLECT requests remain executable when compatible, but the first AI-facing result after a newer tool/document fingerprint carries the current authoritative tool contract under `AI_TOOL_SYNC/`.
- **COMPLETE:** `ai_context.known_tool_version` + `sync_token` provide an explicit acknowledgement handshake; `agent_id` separates independent AI agents; `request_full_sync=true` can force refresh.
- **COMPLETE:** legacy COLLECT without `ai_context` and legacy PATCH using `compatibility.max_tested_version` receive one-shot synchronization without making new fields mandatory.
- **COMPLETE:** successful stale PATCH publishes `AI_TOOL_SYNC_RESULT_*.zip/.txt`; stale PATCH failure embeds sync docs inside FAIL_HANDOFF; stale COLLECT embeds them inside the collection result.
- **COMPLETE:** delivery state suppresses repeated full documentation for the same agent/fingerprint until the fingerprint changes, preserving token budget.
- Behavioral gate: `self_test_ai_sync_v6_20_0.py`.

## v6.19.3 current capability — copy-friendly upload aliases

- **PRESERVED/NEW:** canonical COLLECT/FAIL_HANDOFF/AI-sync artifacts keep their descriptive long filenames for HISTORY, audit and integrity evidence.
- **PRESERVED/NEW:** ACTION REQUIRED additionally creates a short hard-link alias under `artifacts/ptv_to_ai/` (`CR_<token>.zip/.txt`, `FH_<token>.zip/.txt`, `AS_<token>.zip/.txt`) and prints the pathname on its own output row.
- **PRESERVED/NEW:** aliases share the exact inode/bytes with the canonical artifact on normal local filesystems; they do not duplicate large ZIP/TXT payloads.
- **FAIL-OPEN PRESENTATION ONLY:** if the alias directory is unsafe, symlinked, cross-device, or hard-link creation is unavailable, execution/result semantics remain unchanged and the canonical artifact path is printed instead.
- Behavioral gate: `self_test_copyable_upload_path_v6_20_0.py`.

**No-silent-removal rule:** future releases must not reintroduce tool-generated hard wrapping/clipping of upload artifact paths. Canonical artifact identity remains authoritative; the short alias is an additive copyability aid, not a replacement for HISTORY metadata.
