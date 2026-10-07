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
- [x] Add `editorcompletion.js` with Tier 1 keyword/snippet completion.
- [x] Extend centralized language registry with completion metadata.
- [x] Enable Tier 1 for all currently supported programming/config languages where useful.
- [x] Refactor Project Symbols extraction into reusable project index primitives.
- [x] Add in-memory project symbol index with invalidation/incremental refresh.
- [x] Add persisted versioned project index cache without dirtying the workspace.
- [x] Add `/api/project/completions` with bounded prefix/fuzzy ranking.
- [x] Merge current unsaved editor text into completion context.
- [x] Add import/include/module path suggestions.
- [x] Merge/dedupe Tier 1 + Tier 2 suggestions in the editor.
- [x] Add backend/UI/auth/symbol extraction regression tests.
- [x] Add explicit boundary/cache-corruption/import-empty-prefix regression tests.
- [x] Update README with editor autocomplete behavior, cache location, limits, and offline dependency policy.

## Progress

- Handoff initialized before implementation.
- `e5d9de6`: vendored CodeMirror now exposes `autocompletion`, `completeFromList`, `snippetCompletion`, and `startCompletion` from the already committed bundle. No new dependency added.
- `6265cb1`, `139fa9a`, `f9e37f1`: refreshed immutable cache key, tests, and vendor provenance metadata for the completion hooks.
- `cc101e0`: centralized language registry now carries completion profile IDs and `editorOptionsForFile` accepts completion extensions.
- `58ad9db`, `08bdb1d`: added and loaded `editorcompletion.js` with automatic keyword/snippet completion, CodeMirror snippet placeholders, comment/string suppression, and Ctrl+Space through the vendored completion keymap.
- `20b97f6`: added Tier 1 regression tests and offline/no-runtime-download assertions.
- `2edc0b3`, `95e9da2`: added a bounded, versioned, gzip-persisted project symbol index under the OS user cache and attached it to `Server`; no workspace files are created.
- `10569b0`: project-wide `/api/project/symbols` searches now use the cached symbol index instead of rescanning source files per query.
- `4766b3a`: editor saves incrementally replace the saved file's symbols in the in-memory index.
- `ad387fe`: expanded project symbol extraction for Go values plus PowerShell, Nim, Lua, ActionScript, Proto and GraphQL; Dart/C#/Kotlin continue through C-family extraction.
- `b5ca056`..`9930d0c`: added bounded `/api/project/completions`, current unsaved-file symbol ranking, import/include/module path completion, lexical-context separation, route + shared-server `files.read` authorization.
- `f02d18e`, `5fdca95`: Tier 2 source is merged with Tier 1 in CodeMirror; requests carry unsaved text/cursor/line context, are cancellable on typing, and replace full import prefixes correctly.
- `b0addfb`, `e0a02cf`, `0b5f7cb`, `2371633`: added backend/UI/auth/symbol coverage tests.
- `364d53a`: incremental editor-save symbol updates are also persisted to the OS cache, so restart does not reload stale symbols.

- `3085b85`, `8525e90`, `b075163`, `b224cec`: invalidate completion indexes after project mutations, replace operations, and archive extraction; persisted caches are removed safely when invalidated.
- `c2c4fd5`: removed an accidental duplicate root-level handoff and retained this file as the canonical handoff.

## Resume checkpoint — 2026-10-07

Current implementation already contains the complete Tier 1/Tier 2 vertical path:
- vendored CodeMirror completion primitives;
- automatic keyword/snippet completion;
- cached project symbol index;
- unsaved-current-file merge;
- import/include/module suggestions;
- incremental save updates and mutation invalidation;
- shared-server authorization.

Final completion notes:
- `c30a280`: import/include/module completion now supports explicit completion with an empty typed prefix instead of falling back to the current filename.
- `9fcad1b`: regression coverage for empty-prefix import suggestions and request/document limits.
- `39150e9`: regression coverage for corrupt persisted symbol-index recovery/rebuild.
- `9c650bb`: README documents Tier 1/Tier 2 behavior, cache location, hard limits, offline vendoring policy, and the boundary with future LSP semantic completion.

## Status

This Tier 1 + Tier 2 batch is complete on `main`.

Tier 3 semantic completion (persistent LSP sessions + `textDocument/completion` for type-aware member suggestions such as `object.member`) remains intentionally deferred. The current LSP lifecycle is ephemeral per action and should not be reused for keystroke-driven completion without a persistent-session redesign.

- `650070f`: hotfix after TaskDeck self-update validation at revision `5a4075e`: removed stale unused `sort` import from `internal/server/project_symbols.go` and normalized the import order. This directly fixes the reported Go build failure.


## Smart Enter follow-up — 2026-10-07

Reported after autocomplete rollout:
- Enter at end of an indented Java line did not preserve the line indentation.
- Enter between adjacent delimiter pairs such as `{|}` moved the caret outside instead of expanding the block.

Implemented:
- `36526dd`: added editor-wide smart Enter handling. Plain Enter preserves the current line's leading whitespace; Enter between `{}`, `()`, or `[]` inserts an indented middle line and moves the closing delimiter to its own aligned line. A lone opening delimiter at end-of-line also increases indentation for the new line. Indent unit is inferred from nearby lines, preferring tabs when the surrounding file uses tabs.
- Smart Enter deliberately does not intercept Enter while CodeMirror autocomplete UI is open, so Enter continues to accept completion suggestions.
- `0a07d8b`: added regression assertions for indentation preservation, delimiter-pair expansion, selection handling, and autocomplete coexistence.
