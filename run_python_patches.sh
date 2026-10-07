#!/usr/bin/env bash
# Python Patch Tool compatibility launcher.
# Canonical routing lives in python_patch_entry.py.
set -euo pipefail

TOOLS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$TOOLS_DIR/.." && pwd)"
ENTRY="$TOOLS_DIR/python_patch_entry.py"

PYTHON_CMD=""
python_candidates=()
if [ -n "${TASKDECK_PATCH_PYTHON:-}" ]; then
  python_candidates=("$TASKDECK_PATCH_PYTHON")
elif [ -n "${TASKDECK_PYTHON:-}" ]; then
  python_candidates=("$TASKDECK_PYTHON")
else
  python_candidates=(python3.13 python3.12 python3.11 python3.10 python3 python)
fi

for candidate in "${python_candidates[@]}"; do
  if [[ "$candidate" == */* ]]; then
    [ -x "$candidate" ] || continue
    resolved="$candidate"
  else
    resolved="$(command -v "$candidate" 2>/dev/null || true)"
    [ -n "$resolved" ] || continue
  fi
  if "$resolved" -c 'import sys; raise SystemExit(0 if sys.version_info >= (3, 10) else 3)' >/dev/null 2>&1; then
    PYTHON_CMD="$resolved"
    break
  fi
done

if [ -z "$PYTHON_CMD" ]; then
  echo "ERROR: Python 3.10+ was not found. Set TASKDECK_PATCH_PYTHON or TASKDECK_PYTHON, or install python3.10+ in PATH." >&2
  exit 2
fi
if [ ! -f "$ENTRY" ]; then
  echo "ERROR: Missing Patch Tool entrypoint: $ENTRY" >&2
  exit 2
fi

exec "$PYTHON_CMD" "$ENTRY" --project-root "$PROJECT_ROOT" -- "$@"
