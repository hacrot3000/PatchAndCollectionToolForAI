# TaskDeck project identity migration / cross-project AI evidence guard

Date: 2026-10-15 · branch: `main` · implementation commits are intentionally small.

## Problem
Existing PATCH preflight uses only `manifest.project.key` and fails `project_identity_unconfigured` if `.python_patch_tool.json` lacks a local key. Several existing workspaces have no such configuration. When TaskDeck users run simultaneous tasks/projects, a FAIL_HANDOFF / CODE_COLLECTION_RESULT from another project can be uploaded into an unrelated chat.

## Implemented code
- `vscode_tasks_menu_go/internal/patchtool/project_identity.go`: ensure both project key (directory name by default) and persistent UUID, preserving all existing JSON config fields; exclusive migration lock; invalid identity fails closed. UUID remains stable across restarts, and projects named alike receive different IDs.
- `cmd/vscode_tasks_menu/main.go`: call migration after workspace resolution, including daemon startup and existing projects; Patch commands are migrated as well.
- `_patch_lib/python_patch_project_state.py`: equivalent migration in portable runtime; PATCH project UUID mismatch rejected preflight.
- `vscode_tasks_menu_go/internal/patchtool/ai_pack.go`: Copy Prompt contains key and UUID; cached ZIP now includes `PROJECT_IDENTITY.json` and `PROJECT_IDENTITY_INSTRUCTIONS.txt` and binds the ZIP fingerprint to the UUID.
- `_patch_lib/docs/AI_STANDARD_PROMPTS.json`: Vietnamese/English/Russian prompts require AI to compare both fields, stop on missing/mismatched evidence and ask for explicit user confirmation before proceeding. Every newly generated PATCH and COLLECT must carry `project:{key,id}`.
- `_patch_lib/python_patch_queue_dispatcher.py`: PATCH FAIL_HANDOFF metadata, ZIP and TXT carry identity and refusal/confirmation instructions; automatic recovery COLLECT request carries project key+UUID. LAST_RUN/history metadata carries UUID.
- `_patch_lib/python_patch_collect_compat.py`: COLLECT results and partial incomplete handoffs have identity, instructions, and ZIP members; incoming explicitly bound request is checked before collecting source.
- `_patch_lib/docs/PATCH_PACKAGE_SCHEMA.json` and `COLLECT_ACTION_SCHEMA.json`: support the UUID; COLLECT validates `project:{key,id}` when provided.
- `web/featuremods/patchpanel.js` and `internal/server/patch_ai_pack.go`: all Copy failure / Copy error log actions resolve current identity at copy-time from POST /api/patch/project-identity; refuse to copy when it cannot be verified; include incomplete COLLECT artifact paths.
- `_patch_lib/python_patch_ai_sync.py`: embedded/standalone AI_TOOL_SYNC result bundles also carry the workspace UUID and matching instructions.
- `_patch_lib/python_patch_manual_workflow.py`: manual execution result ZIP/TXT and metadata include project identity and instructions.
- Runtime docs and bilingual README updated.

## Compatibility and safety policy
- Existing configs with valid `project.key` are preserved and receive an ID. Missing config is initialized. Invalid JSON, malformed existing IDs and symlinks fail closed; existing IDs are never silently rotated.
- Legacy PATCHes with **no** project block retain the historical runtime compatibility path for existing integrations; they are not identity-verified and AI is instructed to refuse reusing their evidence until confirmation. An explicit `project` block with key but no UUID now fails preflight (`project_identity_missing`); a mismatched key/ID fails preflight (`project_mismatch`). Newly generated AI requests **must** include both. Legacy handoffs without project UUID are unverified until user confirms both source and target, followed by fresh identity-bound evidence; do not silently relabel old evidence.
- An identity guard reduces accidental cross-project evidence reuse; it does **not** authenticate an adversarial user who can change files and should not be presented as cryptographic provenance.
- Shared-server authentication identity DB is separate from this project-local source/evidence identity.

## Tests and rollout
- New AI Pack ZIP carries `PROJECT_IDENTITY.json` and instructions, and its cache fingerprint includes the identity (so prompt-less ZIP uploads are still attributable to one workspace).
- New Go tests: `internal/patchtool/project_identity_test.go` (migration, persistence, same-name collision, invalid config, concurrency).
- Updated Go AI Pack tests and server endpoint tests; Python regression `_patch_lib/self_test_project_identity_v6_20_2.py`.
- **Static checks passed** via local JavaScript parsing of the fetched updated `web/featuremods/patchpanel.js` and JSON parsing of the 4 updated PATCH/COLLECT/prompt/checklist documents.
- **Execution verification pending** (repository tool exposes file edits but no command runner): run `go test ./internal/patchtool ./internal/server ./cmd/vscode_tasks_menu` from `vscode_tasks_menu_go/`, `python3 _patch_lib/self_test_project_identity_v6_20_2.py`, `python3 -m py_compile` for touched Python files, `node --check vscode_tasks_menu_go/web/featuremods/patchpanel.js`, then functional TaskDeck/Patch/COLLECT smoke on two same-named workspaces. No GitHub Actions were triggered as part of these commits.
- Review startup behavior on an actively running workspace after upgrading TaskDeck. A running *old* binary must be restarted/updated before its UI can expose the new endpoint.
