# AI / ChatGPT usage contract — Python Patch Tool v6.9.4

This document is intended to be uploaded or quoted to an AI that creates PATCH
or COLLECT artifacts for this tool. The rules below override obsolete examples
from older conversations/documents.

## COLLECT REQUEST DELIVERY — STRICT

If more source/evidence is needed, the AI must deliver a **ZIP file**, not a
standalone JSON file.

Required artifact:

```text
CODE_COLLECTION_REQUEST_<short-purpose>_<timestamp>.zip
└── CODE_COLLECTION_REQUEST_<short-purpose>_<timestamp>.json
```

The archive must contain exactly one file whose basename matches
`CODE_COLLECTION_REQUEST*.json`. The request JSON may describe one or multiple
readonly collection actions.

### What the AI must tell the user

```text
1. Put the downloaded request ZIP directly in <project>/patchs/.
2. Run: ./tools/run_python_patches.sh
3. In the normal queue, select the [COLLECT] item (a sole item is preselected).
4. Send the resulting source/evidence collection ZIP back to the AI.
```

### What the AI must NOT tell the user

Do not provide a loose request `.json`.

Do not tell the user to copy a JSON request into `patchs/`.

Do not tell the user to invoke any manual COLLECT subcommand or to append a
request path/extra COLLECT arguments to the launcher. Internal routing syntax
is intentionally omitted here so an AI cannot mistake it for user guidance.

Normal user operation is zero-argument only:

```bash
./tools/run_python_patches.sh
```

### Important distinction

- **Request ZIP**: created by the AI; contains the request JSON; placed in
  `patchs/`; readonly instructions only.
- **Result collection ZIP**: created by the tool after COLLECT succeeds;
  contains collected files/evidence and is sent back to the AI.

Never confuse those two ZIPs, and never substitute the inner JSON for the
request ZIP.

## PATCH delivery

PATCH artifacts remain packages recognized by the normal zero-argument queue.
Do not instruct the user to re-enable SANDBOX/worktree transaction modes.
SANDBOX is permanently removed from the supported workflow.

## PATCH duplicate behavior

The normal zero-argument queue performs a local-only duplicate check before the
selector. A PATCH is skipped only when its exact package SHA-256 matches a file
already stored in the same project's `patchs/patched/`. This is not a global or
server-side history check, so the same PATCH may still be run on a different
machine/project that does not have that local history. AI-generated instructions
must not tell users to bypass this check by renaming an identical package.

## COLLECT PASS CONSOLE — RESULT ZIP IDENTIFICATION

On a successful COLLECT, the tool owns the final user-facing artifact summary.
The result collection archive is shown exactly once in a highlighted block:

```text
================ COLLECT RESULT ================
[PRIMARY - UPLOAD THIS FILE]
<absolute-path-to-result-collection.zip>
Destination: ChatGPT / AI server
================================================
[INFO] REQUEST ARCHIVED: <request-zip-path>
```

The path under `[PRIMARY - UPLOAD THIS FILE]` is the only artifact the user
normally uploads for the next AI analysis step. The archived request path is
informational and must not be mistaken for the collection result.

Legacy collector output may internally emit both a labelled `ZIP` line and the
same bare path. The supervisor deduplicates those variants and must not replay
the result ZIP path twice.


### v6.9.4 result verification note

If legacy collector output mentions multiple candidate result archives, the tool
validates them newest-first and exposes only one verified `[PRIMARY - UPLOAD THIS FILE]`.
A zero-argument COLLECT is not considered fully successful until its request ZIP
is archived out of the runnable queue into `patchs/patched/`.

### Duplicate-local boundary (v6.9.4)

Treat duplicate history as machine/project-local only. Symlinked or shared
`patchs/patched/` history must not cause a PATCH to be skipped. A PATCH can run
on another machine/project even when identical bytes have run elsewhere.


## Preserved COLLECT requests inside HANDOFF evidence

A HANDOFF/support ZIP may preserve an old `CODE_COLLECTION_REQUEST*.json` for
diagnosis. The ZIP itself is **not** a new COLLECT request and must not be placed
into the runnable queue for collection merely because that evidence file exists.
The dispatcher resolves structural HANDOFF identity before COLLECT routing.

### v6.9.4 local queue-session safety

Do not interpret a `BUSY` message as a PATCH failure. It means another local
zero-argument Patch Tool session already owns this project queue, and this
second invocation intentionally executed nothing. This does not create or use
any global/cross-machine history.


## v6.9.4 UI regression repair
- Fullscreen selector rows are clipped to terminal cell width before ANSI styling; the current row is bold/reverse highlighted and the header always shows `CON TRỎ i/N`, preventing loss of position with very long filenames.
- Successful COLLECT uses a high-contrast `ACTION REQUIRED` upload banner. The verified result ZIP path is printed exactly once; the archived request remains informational.
