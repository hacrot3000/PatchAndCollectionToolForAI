# COLLECT progress / compatibility v6.11.0

The normal user command remains:

```bash
./tools/run_python_patches.sh
```

v6.11.0 retains the one-line TTY progress supervisor, terminal-width safety, bounded FAIL output, signal forwarding, post-exit drain cleanup, verified result ZIP validation and one highlighted `[PRIMARY - UPLOAD THIS FILE]` path.

## Native `pack` compatibility path

COLLECT now enters `python_patch_collect_compat.py` before the private collector. If the request consists only of `pack` actions, the overlay handles it read-only itself. Exact project-relative regular files are copied to `files/<relative-path>` in a result ZIP under `artifacts/patch_tool_code_collections/`; `COLLECTION_MANIFEST.json` records path, byte size and SHA-256.

Unsafe or ambiguous `pack` inputs fail closed: absolute paths, traversal, backslash aliases, directories, symlinks and missing files are rejected. The request is archived to `patchs/patched/` only after a valid result ZIP has been created; if archival fails, the newly-created result is removed so no failed COLLECT leaves an apparently uploadable artifact.

If the request is not pack-only, the compat layer `exec`s the installed `python_patch_readonly_collector.py` with the original arguments. No private collector action is renamed, emulated or partially interpreted.
