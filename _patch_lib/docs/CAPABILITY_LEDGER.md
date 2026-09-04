# Python Patch Tool — cumulative capability ledger

Current release: **v6.18.2**

This is the canonical cross-version continuity ledger. It complements the current feature-status document; it must never be replaced by a current-only checklist.

Status vocabulary:

- **PRESERVED** — current runtime still provides the historical behavior or a behaviorally compatible equivalent.
- **COMPATIBILITY_RESTORED** — behavior had regressed and is restored in the named version.
- **SUPERSEDED** — intentionally replaced by a newer documented contract; old behavior is retained here for history.
- **REMOVED_BY_REQUIREMENT** — explicitly removed by a later requirement/architecture decision.
- **NOT_CURRENTLY_GUARANTEED** — historical capability existed but current self-contained v6 package does not claim equivalent parity; it must remain visible here rather than silently disappearing.

See `HISTORICAL_FEATURE_BASELINE_V5_15.md` for all 107 original names.

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

## Historical COLLECT capability mapping (#29–46)

The v5 collector was broader than the later self-contained v6 contract. v6.10 explicitly stopped claiming the old action table as a universal schema. These rows remain here so an AI cannot erase their existence.

| Historical capability | v6.18.2 disposition / closest current action |
|---|---|
| `ls`, `tree` (#29–30) | **NOT_CURRENTLY_GUARANTEED** as standalone actions; `overview` supplies bounded project structure. |
| Project overview (#31) | **PRESERVED** as `overview`. |
| Research (#32) | **SUPERSEDED/PARTIAL**; compose `overview` + verified `search`. |
| Find/glob (#33) | **PRESERVED** as `find`. |
| File/range, head/tail (#34–35) | **NOT_CURRENTLY_GUARANTEED** as named actions; `pack` can collect files but is not declared exact parity. |
| Symbol/reference/callgraph/dependency (#36, #38–40) | **NOT_CURRENTLY_GUARANTEED**; historical M3 workflows used them, so they must not disappear from the ledger. |
| Search (#37) | **PRESERVED and hardened**; v6.18 adds filesystem-first coverage, independent fallback, `must_find`, zero diagnostics and health-search. |
| Directory/multi-path pack (#41–42) | **PRESERVED/PARTIAL** through `pack`. |
| Git context (#43) | **PRESERVED** as `git`. |
| Large decompile (#44) | **NOT_CURRENTLY_GUARANTEED**. |
| JSON multi-action (#45) | **PRESERVED** for current five action types. |
| Collector path/security policy (#46) | **PRESERVED / strengthened** by project-relative path enforcement, bounded collection and search coverage diagnostics. |

## Output/UI historical capabilities (#98–107)

The v6 report/history model superseded the exact v5 split-bundle naming model. Preserve current equivalents but do not claim byte/name parity with v5.

- #98 absolute critical local paths — **PRESERVED where artifacts are printed**.
- #99–103 old AI_HANDOFF/SUMMARY/CODE/DETAIL/LAST_RUN.md role model — **SUPERSEDED** by current structured run/report/fail-handoff model.
- #104 color/text run states — **PRESERVED current equivalents**.
- #105 executed-package traceability — **PRESERVED in run/history evidence**.
- #106 old short handoff naming — **SUPERSEDED** by current FAIL_HANDOFF/report naming.
- #107 selector deletion — **PRESERVED**.

## Current critical continuity gates

The following are current-release capabilities and are protected in addition to the historical v5 baseline:

- zero-argument PATCH/COLLECT queue and single-item default;
- empty runnable queue opens HISTORY in a TTY but creates no fake run/LAST_RUN/history/ledger entry;
- Smart Resume, unresolved-failure registry, recovery menu and FAIL_HANDOFF;
- duplicate filtering and `patchs/ignore` behavior;
- current PATCH schema/preflight/rollback/batch mutation protections;
- POSIX and native-Windows launcher routing;
- current COLLECT actions (`pack`, `overview`, `find`, `search`, `git`);
- filesystem-first verified search, independent fallback, coverage/skipped-dir reporting, `must_find`, `diagnose_on_zero`, anchors/expected-files and `health-search`.

## Release rule

No future release may remove a row from this ledger. A changed capability must receive a new disposition and behavioral evidence. See `NO_SILENT_REMOVAL_POLICY.md`.
