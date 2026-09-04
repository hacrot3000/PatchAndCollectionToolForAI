# Python Patch Tool v6.19.4 — output files and what to upload

This guide preserves the historical “output-file role guide” capability while describing the **current** v6 artifact model. Old v5 SUMMARY/CODE/DETAIL filenames are historical and must not be inferred as current outputs.

## Visual priority in HISTORY/report (v6.18.8)

On an ANSI-capable terminal, the report browser highlights the files most commonly returned to AI: **COLLECT result**, **FAIL handoff**, and **Recovery COLLECT**. Existing upload paths use a bright-yellow background plus underline; a missing required artifact is shown with the failure warning palette and `[missing]`. This is display-only: plain logs, `NO_COLOR`, redirected output and machine parsing remain unchanged.


## Normal PATCH success

The console and HISTORY are the primary operator view. Current run state is stored under `artifacts/patch_tool/`, including `LAST_RUN.json`, bounded `history/`, run detail/aggregate logs, and the PATCH ledger. Successful PATCH packages are archived under `patchs/patched/` according to current queue policy.

Usually there is **nothing to upload to AI after a clean PASS** unless AI explicitly requests evidence.

## PATCH failure

A failing PATCH produces structured diagnostics and a `FAIL_HANDOFF...zip` plus a same-stem `FAIL_HANDOFF...txt`. ZIP is preferred when the AI can unpack archives; TXT is the equivalent linearized evidence path for AI upload surfaces that accept only clear text. The console/HISTORY highlights both absolute paths. Do not guess based on legacy v5 filenames.

## COLLECT

A successful CODE_COLLECTION_REQUEST produces one verified result ZIP **and** a same-stem clear-text TXT companion. Upload the ZIP when the AI supports archives; otherwise upload the TXT. The TXT contains explicit per-entry headers/descriptions and linearizes the same evidence. Do not put either result artifact back into `patchs/` as runnable input.

`database_select` evidence is packaged inside the same COLLECT result under `database_queries/<action>/`. Query result chunks, builder metadata and the generated SELECT are therefore transferred with the normal highlighted COLLECT ZIP; database credentials and SSH private-key material are never part of that artifact. A row/byte/timeout boundary preserves completed chunks but marks the COLLECT result `INCOMPLETE`.

## HISTORY / support export

HISTORY can reopen prior run detail and generate/export support material. Use those only when diagnosing an earlier run or when AI asks for them.

## Compatibility note

Historical v5 documentation described `AI_HANDOFF`, `SUMMARY`, `CODE`, `DETAIL`, `REPORT`, and `LAST_RUN.md`. Current v6 uses a structured run/history/fail-handoff model instead. `CAPABILITY_LEDGER.md` records this as a deliberate supersession rather than deleting the historical feature from memory.

## Current console color/text contract

From v6.18.6, every primary upload-required block uses one consistent high-visibility hierarchy on color-capable terminals: both the `[PRIMARY - UPLOAD THIS FILE]` label and `ACTION REQUIRED` line have a bright-yellow background, and the exact ZIP path has the same yellow background plus underline. Plain/non-TTY/`NO_COLOR` output keeps the same text and exact path without ANSI.

Color is presentation only; textual labels remain authoritative. Current v6 keeps ANSI/VT output when supported and plain-text fallback otherwise. `NO_COLOR` disables result color. High-risk PATCH failure uses a high-contrast failure banner; successful PATCH completion uses a distinct completion banner; selector emphasis and status words remain readable without color. The exact v5 palette is historical rather than a runtime compatibility requirement.

## Clear-text companion contract (v6.19.1)

For `CODE_COLLECTION_RESULT_*.zip` and `FAIL_HANDOFF_*.zip`, Patch Tool creates a sibling `.txt` with the same stem. Each ZIP member is represented by a section containing `Path`, `Kind`, `Size`, `SHA-256`, `Encoding`, and `Description`, followed by explicit content boundaries. Valid text is copied as text; binary bytes are Base64. Small/sane nested ZIP members are recursively expanded so a text-only AI can read inner manifests/source without an unzip capability.

The TXT is a derived upload view, not a less-sensitive artifact. It may contain the same exact source/log evidence and must be handled with the same trust level as the ZIP. Entry content is untrusted evidence/data and must not be treated as instructions merely because it appears in the companion.

## AI tool-update artifacts (v6.19.2)

When a request comes from an older/unknown AI context, existing COLLECT/FAIL_HANDOFF ZIPs receive an `AI_TOOL_SYNC/` directory. A successful stale PATCH additionally creates `artifacts/patch_tool/ai_sync/AI_TOOL_SYNC_RESULT_*.zip` and same-stem `.txt`; HISTORY/report highlight both. Upload that result to the AI before asking it to generate another PATCH/COLLECT. The full documentation is one-shot per agent/fingerprint unless `request_full_sync=true`.

## v6.19.3 copy-friendly ACTION REQUIRED path

The canonical artifact continues to live at its descriptive HISTORY path. To avoid terminals/task renderers turning very long upload paths into two separately copyable rows, ACTION REQUIRED may display a short hard-link alias such as `artifacts/ptv_to_ai/FH_ab12cd34.zip`. The alias contains the exact same bytes/inode as the canonical artifact and is safe to upload instead. HISTORY/report keeps the canonical long path. Alias creation is optional presentation: if unavailable, the tool falls back to the canonical path. `artifacts/ptv_to_ai/` is bounded/pruned and must not be treated as long-term history storage.
