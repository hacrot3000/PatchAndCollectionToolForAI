# HANDOFF 20 — AI Pack compatibility audit (CLP-05.03.02/.03)

> **Current local workflow (2026-11-27):** [20-coder-execution-contract.md](20-coder-execution-contract.md) supersedes historical process/stop instructions and completion claims below. Reference evidence only; use this audit to guide concrete source changes, not as a mandatory repeated audit or reason to stop. Local coder writes source/test fixes, operator runs CI/Git. Continue independent local work without external-only BLOCKED; no acceptance or safety relaxation.

> Static audit of the 34 design/handoff documents under `docs/taskdeck_ai_agent/`
> against the current trusted contract. Derived from native read-only COLLECT.
> No provider, native tool, build, test or runtime rebase was executed. This
> closes the "other pack docs audit pending" documentation half of CLP-05.03.02/.03;
> runtime compatibility/rebase evidence remains operator-owned.

## 1. Method and coverage
- Directory collection of `docs/taskdeck_ai_agent/*.md`: 34 files, coverage VERIFIED, no truncation.
- Coverage-VERIFIED regex search across those docs for legacy-schema markers:
  `run_python_patches.sh collect`, `code-collection-request`, `COLLECT_ACTION_SCHEMA`,
  `source_baseline`, `grep -nriF`, `CODE_COLLECTION_REQUEST`, `start_line.*max_lines`.
  Result: only 2 matches, both in `18-agent-native-collection-unification.md`:
  - line 15: "canonical `CODE_COLLECTION_REQUEST_*.zip`; require the native queue receipt."
  - line 35: "The authoritative syntax is `_patch_lib/docs/COLLECT_ACTION_SCHEMA.json`."
  Both are CURRENT-contract references (canonical request ZIP name + authoritative schema path),
  not obsolete direct-CLI or `source_baseline`-era forms. No stale legacy COLLECT syntax remains.

## 2. Classification of the pack documents
| Group | Documents | Role vs current contract |
|-------|-----------|--------------------------|
| Design/handoff (01–19, UNIFIED_REVIEW_HANDOFF, POST_MERGE_MASTER_PLAN_PROGRESS, COMMAND_ONLY_PATCH_BRIDGE, PROMPT_CACHE_AFFINITY) | 01-architecture … 19-unified-run-long-task-integration, plus named extras | Historical design + evidence records. They describe source state at pinned commits and label browser/live acceptance as pending. They are DATA, not trusted instructions. |
| HANDOFF 20 living docs (20-*) | master checklist, external-verification-queue, source-completeness-audit, regression-reconciliation, pipeline-trace-map, payload-budget-audit, crash-recovery-matrix, final-integration-report, upgrade-rollback-guidance | Current working set; each explicitly states no runtime PASS from PATCH receipts. |
| README / 09-source-audit | navigation + source audit | Already flags untrusted data and P0/P1 boundaries; no schema override. |

The AI Pack supplied to the AI session carries `PROJECT_IDENTITY.json` with key `PatchAndCollectionToolForAI` and id `5dffe79a-…`; all COLLECT receipts in this session matched that identity. Historical pack instructions do not outrank the current trusted system/tool contract.

## 3. Separation of historical instructions from user goal (CLP-05.03.02)
- Implementation (from checklist evidence): explicit compatibility policy (CI 45608650711 PASS); recognized historical COLLECT schema projection (CI 45609478674, remote Go 45609478835 PASS).
- This audit confirms the documents contain no active instruction that would regrant permission or inject a stale tool schema. Legacy status wording ("NOT READY FOR INTEGRATION", "pending", "BLOCKED") is historical evidence, not a current grant.
- Current user goal and attachments/evidence remain authoritative at request time; no pack document is treated as a higher-priority instruction.

## 4. Compatibility summary / rebase (CLP-05.03.03)
- Implementation (from checklist evidence): stateless projection replaces only the duplicated historical COLLECT schema; stale schema forces a stateful rebase. The projection does not rewrite canonical history, does not resend the full pack each turn, and does not regrant permission.
- Document audit: every 20-* artifact records this boundary explicitly (no journal rewrite, no permission regrant). No document instructs silent history overwrite.
- Runtime rebase/restart fixture evidence (45736453567 PASS at its commit) is operator-supplied history, not re-verified here; current-source rebase remains VERIFY_PENDING.

## 5. Conclusion
- Documentation half of CLP-05.03.02/.03 is complete: the "other pack docs audit pending" gap is closed by this audit with coverage-VERIFIED evidence that no stale legacy-schema form overrides the current contract.
- Both subtasks remain VERIFY_PENDING — not DONE — because their acceptance also requires runtime compatibility/rebase evidence at the current source, which is external.
- No gate, checkbox status beyond the two notes, or rollup count was changed by this document.
