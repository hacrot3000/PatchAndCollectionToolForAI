# Python Patch Tool v6.18.4 — output files and what to upload

This guide preserves the historical “output-file role guide” capability while describing the **current** v6 artifact model. Old v5 SUMMARY/CODE/DETAIL filenames are historical and must not be inferred as current outputs.

## Normal PATCH success

The console and HISTORY are the primary operator view. Current run state is stored under `artifacts/patch_tool/`, including `LAST_RUN.json`, bounded `history/`, run detail/aggregate logs, and the PATCH ledger. Successful PATCH packages are archived under `patchs/patched/` according to current queue policy.

Usually there is **nothing to upload to AI after a clean PASS** unless AI explicitly requests evidence.

## PATCH failure

A failing PATCH produces structured diagnostics and a `FAIL_HANDOFF...zip` when the failure path supports handoff generation. The console highlights the relevant absolute local path. Upload the highlighted FAIL_HANDOFF/result requested by the tool; do not guess based on legacy v5 filenames.

## COLLECT

A successful CODE_COLLECTION_REQUEST produces one verified result ZIP and marks it as the primary file to upload. Upload that result ZIP. Do not put the result ZIP back into `patchs/` as runnable input.

## HISTORY / support export

HISTORY can reopen prior run detail and generate/export support material. Use those only when diagnosing an earlier run or when AI asks for them.

## Compatibility note

Historical v5 documentation described `AI_HANDOFF`, `SUMMARY`, `CODE`, `DETAIL`, `REPORT`, and `LAST_RUN.md`. Current v6 uses a structured run/history/fail-handoff model instead. `CAPABILITY_LEDGER.md` records this as a deliberate supersession rather than deleting the historical feature from memory.

## Current console color/text contract

Color is presentation only; textual labels remain authoritative. Current v6 keeps ANSI/VT output when supported and plain-text fallback otherwise. `NO_COLOR` disables result color. High-risk PATCH failure uses a high-contrast failure banner; successful PATCH completion uses a distinct completion banner; selector emphasis and status words remain readable without color. The exact v5 palette is historical rather than a runtime compatibility requirement.
