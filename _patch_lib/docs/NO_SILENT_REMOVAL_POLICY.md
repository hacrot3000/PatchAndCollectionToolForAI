# Python Patch Tool — NO SILENT REMOVAL POLICY

**Mandatory for every Patch Tool modification starting with v6.18.2.**

This file is a release gate, not optional guidance.

## Core rule

> A capability that has ever been documented as COMPLETE, PRESERVED, or COMPATIBILITY_RESTORED must not be deleted, renamed, narrowed, made unreachable, or changed semantically by accident.

Before changing Patch Tool itself, AI/developer MUST read:

1. `AI_USAGE_CONTRACT.md`
2. `CAPABILITY_LEDGER.md`
3. `HISTORICAL_FEATURE_BASELINE_V5_15.md` and `HISTORICAL_FEATURE_STATUS_V5_15.json`
4. `CURRENT_CAPABILITY_DISPOSITION.json`
5. `PYTHON_PATCH_TOOL_FEATURE_STATUS.md`
6. `../../implementing.md`
7. `../../PYTHON_PATCH_TOOL_FEATURES_VI.md`

## Removal / replacement protocol

A historical capability may change only when one of these is true:

- the user explicitly requested its removal/change; or
- a later documented contract explicitly superseded it for a safety/architecture reason.

In either case, the same release MUST update `CAPABILITY_LEDGER.md` with:

- historical capability ID/name;
- old behavior;
- new status: `PRESERVED`, `COMPATIBILITY_RESTORED`, `SUPERSEDED`, `REMOVED_BY_REQUIREMENT`, or `NOT_CURRENTLY_GUARANTEED`;
- replacement/equivalent, if any;
- reason and first version of the change;
- a behavioral regression test or an explicit statement that no current equivalent exists.

Never erase a historical row merely because the current implementation no longer has that module/action.

## Additive-first compatibility

- CLI flags, schema fields, helper APIs and accepted package forms are additive by default.
- Do not reuse an existing public flag with a narrower meaning without a migration entry.
- Do not delete a schema field simply because current AI output does not use it.
- Do not delete compatibility code solely because a current self-test does not call it.
- If an old feature conflicts with a newer safety contract, preserve the safer current default and add a bounded compatibility lane when possible.

## Tests must prove behavior, not presence

A compatibility test is insufficient if it only checks that a string, function name, flag, or schema field exists.

For user-visible/public behavior, regression tests MUST execute the path and assert the result. Examples:

- `--all` must actually run all intended queue items;
- repeated `--patch` must actually execute every selected package;
- a legacy multi-script archive must execute every recognized script in deterministic order;
- command-only packages must actually run the permitted command;
- a forbidden strict-compatibility command must actually fail preflight;
- empty zero-argument queue must actually open HISTORY in an interactive TTY without creating fake run state.

## Historical status is not binary

The v5.15 107-row list is an inventory, not 107 completed requirements. Before restoring/removing a historical row, read `HISTORICAL_FEATURE_STATUS_V5_15.json`:

- historical `COMPLETE` rows are preservation obligations unless later explicitly `SUPERSEDED` or `REMOVED_BY_REQUIREMENT`;
- historical `PARTIAL` and `NOT STARTED` rows remain evidence only and must not be silently promoted to current requirements;
- a later `PRESERVED` or `COMPATIBILITY_RESTORED` capability becomes a current preservation obligation even if it was not part of v5.15.

## COLLECT capability continuity

COLLECT capability and public invocation syntax are separate compatibility dimensions. A later workflow may supersede a direct CLI such as `collect <command>` while the read-only action semantics remain protected. Therefore a CLI/workflow supersession is **not** permission to delete an action, alias, schema field, report contract, or evidence behavior.

Starting with v6.18.3, the authoritative protected COLLECT action/alias surface includes the action names in `COLLECT_ACTION_SCHEMA.json`, including restored historical actions and compatibility aliases such as `search_files`, `content`, and `symbol_graph`. Search-like aliases MUST retain the coverage/false-zero safeguards of the canonical `search` action. Decompile compatibility MUST remain read-only and bounded.

If an action is intentionally replaced, `CAPABILITY_LEDGER.md` must name the replacement and a semantic test must demonstrate either the preserved equivalent or the deliberate fail-closed disposition.

## Documentation preservation

Historical documentation must not be silently rewritten as if old behavior never existed. When text is no longer current, mark it `Historical — superseded by ...` instead of deleting evidence needed for future audits.

Avoid stale phrases such as `Current behavior` inside an old-version section after a newer section supersedes it.

## Release checklist

Before packaging a Patch Tool upgrade:

1. Compare public CLI routes with the previous release and historical ledger.
2. Compare PATCH and COLLECT schema fields additively.
3. Compare public helper APIs and recognized legacy package forms.
4. Compare the complete protected COLLECT action/alias set with `COLLECT_ACTION_SCHEMA.json` and run its behavioral fixture.
5. Run behavioral historical-compatibility regression.
6. Run current v6 regression (queue, HISTORY, recovery, batch, search, Windows routing, integrity).
7. Verify documentation chronology and capability ledger.
8. Rebuild exact `SHA256SUMS` and package-content inventory.
9. Extract the final ZIP and run the release gates from the extracted bytes.

A release MUST NOT claim full feature continuity while an unexplained historical regression remains.

## Complete-ID coverage gate (v6.18.4+)

Before packaging, compare `HISTORICAL_FEATURE_STATUS_V5_15.json.complete_ids` with `CURRENT_CAPABILITY_DISPOSITION.json.entries[].id`. The sets MUST be identical. No historical COMPLETE capability may be left as an unexplained gap. The disposition file is machine-readable release evidence; `CAPABILITY_LEDGER.md` remains the human explanation.

A test that imports a private function is not sufficient for a public-entry contract when the launcher/parser is part of the feature. Public CLI/zero-argument behavior must also have a smoke/semantic test through the real entry path.

## Search-specific invariant

> Zero matches is a search result, not proof of absence.

Search absence can only be trusted when coverage is VERIFIED and the independent fallback agrees.

## When uncertain

Do not remove code to simplify the tool when its historical purpose is unclear. First locate its capability in the ledger/tests/docs. If still unresolved, preserve it and mark the uncertainty for audit rather than silently deleting it.

## v6.19.0 database safety capability

`database_select` is a protected current capability. Preservation includes its negative/safety guarantees, not only its existence: raw SQL is not accepted; generated DB statements are SELECT-only; profiles do not contain passwords; SQLite stays read-only; remote MySQL uses SSH tunneling; partial output remains usable and explicitly INCOMPLETE. AI must treat weakening any of these guarantees as a contract change requiring explicit user approval, ledger disposition, and behavioral regression updates.
