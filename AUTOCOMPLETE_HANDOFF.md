# TaskDeck Editor Autocomplete Handoff

Last updated: 2026-10-07
Target branch: main

## Goal

Implement editor autocomplete without npm/CDN/runtime dependency downloads.

Two levels:
1. Language keyword + syntax/snippet completion.
2. Project-aware completion from an incrementally maintained project symbol/dependency index.

Dependency policy:
- No npm install / npm ci / npx.
- No build-time or runtime dependency download.
- Any future third-party source must be vendored into Git with license/version/source metadata.
- Prefer existing vendored CodeMirror 6 and Go standard library.

## Current architecture already available

- Vendored CodeMirror 6 bundle with autocompletion support already used by SQL editor.
- Central editorLanguageRegistry in web/featuremods/editor.js.
- /api/project/symbols backend and Project Symbols UI.
- LSP bridge exists for Go/C/C++/Python/JS/TS but textDocument/completion is intentionally deferred to a later semantic Tier 3 because current LSP lifecycle is ephemeral per action.

## Implementation plan

- [ ] Expose/reuse generic CodeMirror completion primitives from vendored bundle.
- [ ] Add editorcompletion.js with Tier 1 completion source and UI integration.
- [ ] Add keyword/snippet completion metadata for all currently supported programming/config languages.
- [ ] Add editor completion tests, including Ctrl+Space and automatic trigger behavior.
- [ ] Refactor project symbol extraction into reusable index primitives.
- [ ] Add persistent project index cache outside the workspace.
- [ ] Add incremental invalidation/reindex hooks for file create/save/rename/delete and external changes.
- [ ] Add /api/project/completions with ranking/deduplication.
- [ ] Integrate Tier 2 suggestions with editor completion.
- [ ] Add include/import/module path completion.
- [ ] Add bounded performance/memory tests and update docs.

## Progress log

### 2026-10-07 — start
- User approved Tier 1 + Tier 2 implementation.
- Handoff file created before code changes to protect progress from timeout.

## Next action

Inspect vendored CodeMirror completion exports and the editor module loading path, then implement the smallest Tier 1 vertical slice first.
