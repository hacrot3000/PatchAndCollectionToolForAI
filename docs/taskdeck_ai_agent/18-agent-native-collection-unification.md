# Agent Chat COLLECT — native Patch Tool only

Date: 2026-11-23. Scope: TaskDeck AI Agent Chat source inspection.

## Non-negotiable architecture

Agent Chat MUST NOT implement or invoke a separate file reader/Collection
engine. A tool call `taskdeck_collect_workspace` MUST use the existing
Python Patch Tool COLLECT contract and the SAME native queue workflow as
manual **Patch Tool → Paste text → COLLECT**:

1. Parse the model's JSON transport envelope and bind exact workspace
   `project.key` + `project.id`; reject any supplied mismatch.
2. Invoke the existing `patchPaste` handler (`kind:collect`) to create the
   canonical `CODE_COLLECTION_REQUEST_*.zip`; require the native queue receipt.
3. Start the existing `kind:patch / patch_mode:queue / patch_ui:native` session.
4. Select exactly the newly queued `COLLECT` item by name/index through the
   existing native protocol `queue_selection` handler; NEVER select unrelated
   packages.
5. Wait for native `PASS / FAIL / INCOMPLETE / PREFLIGHT_FAIL` protocol evidence
   and collect `collect_result_zip` / `collect_result_text` artifacts bound to
   the exact item and run.
6. Verify native ZIP `COLLECTION_MANIFEST.json` identity and native TXT sibling.
   Return their relative artifact paths and a bounded, clearly marked excerpt
   to the AI, while retaining the original ZIP/TXT in Patch Tool History.

The production Go server no longer contains the previous custom
`executeAgentCollection` reader. Its historical implementation is moved to
`agent_chat_collection_legacy_test.go` solely for old protocol tests.
`AgentCollectionCall` is an explicit test-only injection and MUST NEVER
be configured by production startup.

## Native functionality and large source

The authoritative syntax is `_patch_lib/docs/COLLECT_ACTION_SCHEMA.json`.
Agent Chat's tool declaration is only a permissive transport schema;
the Python collector performs all action and limit validation. This preserves
native `decompile`/`ida`/`ghidra`, `symbol`, `references`, `range`,
`head`, `tail`, `search`, `directory` and its native partial results.

For a ~150 MB source/decompile dump, do not assume that `pack` or
`search` can inline the entire source. The model must select native targeted
actions, honor hard limits and use continuation requests when necessary.

## INCOMPLETE rc=3 is NOT no result

A runner may report rc=3 and outer status `failed`, even though its
native `COLLECTION_MANIFEST.json` says `INCOMPLETE` and ZIP/TXT exist.
The Agent must preserve and relay the partial evidence with explicit coverage
limitations. Missing evidence is never proof of absence. Never turn a partial
result into a false PASS.

Full ZIP/TXT artifacts remain on disk. The AI API cannot accept a 150 MB
inline tool response, so the provider receives a bounded excerpt, status,
diagnosis, source identity, native session/run, and exact artifact references.
If more is needed, the model must request a more targeted native COLLECT.

## Recovery and safety

- No automatic re-execution after ambiguous queue/selection or missing
  native final evidence. Inspect the existing native queue/History.
- Stop cancels the current read-only native COLLECT session only.
- The initial implementation is enabled for the local-owner workspace.
  Shared-server native execution is refused until equivalent authenticated
  owner/permission propagation is implemented.
- Protected native output paths, symlinks, mismatched project IDs and mismatched
  ZIP/TXT stems are not accepted.
- A standalone arbitrary JSON or Python snippet from model output is never
  executed as code.

## Tests

`go test -count=1 ./internal/server -run 'TestAgent'`

Additional tests assert native request identity, native-specific 150 MB
decompile action transport, rc=3/INCOMPLETE ZIP/TXT preservation, 150 MB sparse
companion excerpt memory bounds, protected artifact paths and no fallback when
the native session service is unavailable.

**Release verification:** CI and a live manual-versus-Agent comparison
on the user's 150 MB workspace are required before calling the integration
production-verified.
