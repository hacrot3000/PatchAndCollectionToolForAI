# COLLECT progress / self-contained collector v6.14.0

The normal command remains:

```bash
./tools/run_python_patches.sh
```

v6.14.0 keeps one-line TTY progress, terminal-width safety, bounded FAIL output, signal forwarding, post-exit drain cleanup, verified result-ZIP validation and one highlighted `[PRIMARY - UPLOAD THIS FILE]` path.

The collector is now self-contained for the exact schema in `COLLECT_ACTION_SCHEMA.json`. Supported actions are `pack`, `overview`, `find`, `search`, and fixed-section `git`. Unsupported actions/fields fail preflight before execution; there is no private collector delegation.
