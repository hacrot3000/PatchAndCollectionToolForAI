# Historical capability baseline — Python Patch Tool v5.15

**Immutable historical inventory.** This file records the 107 capabilities that the v5.15 feature-status document carried forward. It exists so later refactors cannot make a previously documented capability disappear from history. Do not delete rows; append errata or disposition in `CAPABILITY_LEDGER.md` instead.

Source basis: `PYTHON_PATCH_TOOL_FEATURE_STATUS.md` from v5.15.0.

| # | Historical capability |
|---:|---|
| 1 | Zero-argument patch workflow |
| 2 | Single public entry point |
| 3 | Organized `_patch_lib` layout |
| 4 | Portable release layout |
| 5 | Automatic queue ordering |
| 6 | PASS/FAIL input handling |
| 7 | Git add isolation |
| 8 | Patch-aware commit/push |
| 9 | Manifest and ZIP standard |
| 10 | Data-only patches |
| 11 | Syntax preflight |
| 12 | Process isolation and tree cleanup |
| 13 | Transaction sandbox |
| 14 | Rollback/apply conflict protection |
| 15 | Idempotency check |
| 16 | Source drift detection |
| 17 | Stale-anchor diagnostics |
| 18 | Syntax suggestions |
| 19 | Structured diagnostics |
| 20 | Root-cause clustering |
| 21 | Smart console filtering |
| 22 | Raw evidence preservation |
| 23 | Advanced secret redaction |
| 24 | Environment fingerprint |
| 25 | Diagnostic quality report |
| 26 | AI summary/code/detail bundles |
| 27 | Unified AI handoff |
| 28 | Failure delta/history |
| 29 | `ls` collector |
| 30 | `tree` collector |
| 31 | Project overview collector |
| 32 | Research collector |
| 33 | Find/glob collector |
| 34 | File/range collector |
| 35 | Head/tail collector |
| 36 | Symbol collector |
| 37 | Search collector |
| 38 | Reference collector |
| 39 | Callgraph context |
| 40 | Dependency collector |
| 41 | Directory collector |
| 42 | Multi-path pack collector |
| 43 | Safe Git context collector |
| 44 | Large decompile collector |
| 45 | JSON multi-action request |
| 46 | Collector path/security policy |
| 47 | Semantic-safe source blocks |
| 48 | Explicit token budgets |
| 49 | Bundle deduplication |
| 50 | Source-aware handoff selection |
| 51 | Multi-machine history policy |
| 52 | Project identity key |
| 53 | Local duplicate detection |
| 54 | Non-patch ZIP filtering |
| 55 | Relative-path reporting |
| 56 | Project-key migration |
| 57 | Validation profiles |
| 58 | Delta-based validation selection |
| 59 | Safe diagnostic rerun |
| 60 | Validation levels |
| 61 | Push quality gate |
| 62 | Interrupted-run recovery |
| 63 | Disk-space preflight |
| 64 | Resource limits |
| 65 | Signed patch manifest |
| 66 | Reproducible package bytes |
| 67 | Declarative post-patch commands |
| 68 | Change-gated command execution |
| 69 | Command-only package |
| 70 | Restricted no-change override |
| 71 | Basic command allowlist |
| 72 | Project-local script boundary |
| 73 | Inline/shell execution rejection |
| 74 | Command timeout/process supervision |
| 75 | Command-aware delta and validation |
| 76 | Command argument secret guard |
| 77 | Idempotency-before-command ordering |
| 78 | Extract-and-run installation |
| 79 | Correct public-runner placement |
| 80 | Portable direct upgrade |
| 81 | Optional controlled installer |
| 82 | Portable-layout regression test |
| 83 | Interactive patch selection default |
| 84 | TTY checkbox multi-select |
| 85 | Line-mode multi-select fallback |
| 86 | Repeated explicit `--patch` |
| 87 | Unselected-package preservation |
| 88 | Selection-aware identity adoption |
| 89 | Explicit non-interactive automation |
| 90 | Legacy v4 standalone patch execution |
| 91 | Legacy v4 archive execution |
| 92 | v4 helper API compatibility |
| 93 | Strict-policy legacy exception |
| 94 | Unscoped legacy project safety |
| 95 | Mixed v4/v5 selected queue |
| 96 | Legacy-vs-handoff discrimination |
| 97 | Legacy report metadata |
| 98 | Absolute critical console paths |
| 99 | Output-file role guide |
| 100 | Primary handoff highlighting |
| 101 | ANSI color roles |
| 102 | REPORT/DETAIL alias clarification |
| 103 | Persistent LAST_RUN file guide |
| 104 | Color-coded run states |
| 105 | Executed-patch list |
| 106 | Short handoff bundle names |
| 107 | Selector patch deletion |

Historical status values and priorities remain in the original v5.15 status document. Current disposition is authoritative only in `CAPABILITY_LEDGER.md`.
