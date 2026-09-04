# Python Patch Tool — NO SILENT REMOVAL POLICY

**Mandatory for every Patch Tool modification starting with v6.18.2.**

This file is a release gate, not optional guidance.

## Core rule

> A capability that has ever been documented as COMPLETE, PRESERVED, or COMPATIBILITY_RESTORED must not be deleted, renamed, narrowed, made unreachable, or changed semantically by accident.

Before changing Patch Tool itself, AI/developer MUST read:

1. `AI_USAGE_CONTRACT.md`
2. `CAPABILITY_LEDGER.md`
3. `HISTORICAL_FEATURE_BASELINE_V5_15.md`
4. `PYTHON_PATCH_TOOL_FEATURE_STATUS.md`
5. `../../implementing.md`
6. `../../PYTHON_PATCH_TOOL_FEATURES_VI.md`

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

## Documentation preservation

Historical documentation must not be silently rewritten as if old behavior never existed. When text is no longer current, mark it `Historical — superseded by ...` instead of deleting evidence needed for future audits.

Avoid stale phrases such as `Current behavior` inside an old-version section after a newer section supersedes it.

## Release checklist

Before packaging a Patch Tool upgrade:

1. Compare public CLI routes with the previous release and historical ledger.
2. Compare PATCH and COLLECT schema fields additively.
3. Compare public helper APIs and recognized legacy package forms.
4. Run behavioral historical-compatibility regression.
5. Run current v6 regression (queue, HISTORY, recovery, batch, search, Windows routing, integrity).
6. Verify documentation chronology and capability ledger.
7. Rebuild exact `SHA256SUMS` and package-content inventory.
8. Extract the final ZIP and run the release gates from the extracted bytes.

A release MUST NOT claim full feature continuity while an unexplained historical regression remains.

## Search-specific invariant

> Zero matches is a search result, not proof of absence.

Search absence can only be trusted when coverage is VERIFIED and the independent fallback agrees.

## When uncertain

Do not remove code to simplify the tool when its historical purpose is unclear. First locate its capability in the ledger/tests/docs. If still unresolved, preserve it and mark the uncertainty for audit rather than silently deleting it.
