# AI / ChatGPT usage contract — Python Patch Tool v6.7.12

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

Normal user operation is zero-argument only, and the public launcher rejects
manual COLLECT subcommands rather than merely documenting them as obsolete:

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

### Result artifact validity

A collector process returning zero is not enough to claim success. The public
COLLECT supervisor verifies that the reported result artifact exists locally
and is a ZIP before showing `[PRIMARY - UPLOAD THIS FILE]`. If the artifact is
missing/unusable, COLLECT returns non-zero and there is no primary-upload block.
Quoted paths and paths containing spaces are valid and must not be rewritten by
AI instructions.
