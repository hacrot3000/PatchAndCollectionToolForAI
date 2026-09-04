# Python Patch Tool v6.14.0 portable usage

Install/update directly at project root:

```bash
unzip -o python_patch_tool_v6.14.0.zip -d "$PWD"
./tools/run_python_patches.sh
```

The release is self-contained for its v6.14.0 documented PATCH/COLLECT contract. It ships the PATCH runner, patch utilities and readonly collector; no older private core is required.

Public workflow remains zero-argument. Put PATCH or `CODE_COLLECTION_REQUEST_*.zip` under `patchs/`, then run the command above.

Before working with AI, send all current `tools/_patch_lib/docs/`. For Patch Tool development also send `tools/implementing.md` and `tools/PYTHON_PATCH_TOOL_FEATURES_VI.md`.

COLLECT actions are defined exclusively by `docs/COLLECT_ACTION_SCHEMA.json`: `pack`, `overview`, `find`, `search`, `git`.

The end-user HTML guide intentionally hides internal COLLECT action details. AI/tool integrations use `docs/COLLECT_ACTION_SCHEMA.json` directly.

## v6.14.0 PATCH preflight / recovery / audit

Normal operation remains zero-argument. PATCH package construction must follow `PATCH_PACKAGE_SCHEMA.json` and `PATCH_PACKAGE_GUIDE.md`. Failed PATCH runs write `artifacts/patch_tool/LAST_RUN.json` and normally a `fail_handoffs/FAIL_HANDOFF_*.zip`; source drift can prepare a next-run COLLECT request. Run history is local/bounded and never used as cross-machine duplicate state. Interactive PATCH inspect is available with `i` without executing or archiving the package.

## v6.14.0 safe rollback / Tool Health

PATCH rollback is opt-in and requires the exact metadata contract in `PATCH_PACKAGE_GUIDE.md`; it never reintroduces SANDBOX/worktree transactions and never guesses a Git rollback. In the zero-argument selector press `h` for read-only Tool Health; an empty queue prints a compact health summary automatically.
