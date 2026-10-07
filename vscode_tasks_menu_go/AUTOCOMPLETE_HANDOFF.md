# Editor Autocomplete Handoff

Last updated: 2026-10-07

## Goal

Implement two autocomplete tiers in TaskDeck without npm, CDN, runtime downloads, or build-time dependency downloads. Any third-party code must be vendored into Git with provenance.

1. Tier 1: language keyword + syntax/snippet completion.
2. Tier 2: project-aware completion from indexed classes/functions/types/imports/includes/modules.

Ruby, Perl, and R remain out of scope for this batch.

## Constraints

- No npm install / npm ci / npx.
- No CDN.
- No curl/wget dependency during build/runtime.
- Reuse vendored CodeMirror 6.
- Prefer Go stdlib for backend indexing/storage.
- Keep completion bounded and safe for large projects.
- Preserve existing Project Symbols and LSP behavior.

## Existing reusable pieces

- Vendored CodeMirror 6 already includes autocomplete infrastructure and Ctrl+Space through the SQL editor integration.
- `/api/project/symbols` already extracts common project symbols but scans on demand.
- LSP bridge exists for Go/C/C++/Python/JS/TS, but currently uses an ephemeral process per action and does not expose textDocument/completion.

## Implementation plan

- [x] Add a generic CodeMirror completion hook usable by the file editor.
- [ ] Add `editorcompletion.js` with Tier 1 keyword/snippet completion.
- [ ] Extend centralized language registry with completion metadata.
- [ ] Enable Tier 1 for all currently supported programming/config languages where useful.
- [ ] Refactor Project Symbols extraction into reusable project index primitives.
- [ ] Add in-memory project symbol index with invalidation/incremental refresh.
- [ ] Add persisted versioned project index cache without dirtying the workspace.
- [ ] Add `/api/project/completions` with bounded prefix/fuzzy ranking.
- [ ] Merge current unsaved editor text into completion context.
- [ ] Add import/include/module path suggestions.
- [ ] Merge/dedupe Tier 1 + Tier 2 suggestions in the editor.
- [ ] Add tests and update documentation.

## Progress

- Handoff initialized before implementation.
- `e5d9de6`: vendored CodeMirror now exposes `autocompletion`, `completeFromList`, `snippetCompletion`, and `startCompletion` from the already committed bundle. No new dependency added.
- `6265cb1`, `139fa9a`, `f9e37f1`: refreshed immutable cache key, tests, and vendor provenance metadata for the completion hooks.
