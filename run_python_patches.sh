#!/usr/bin/env bash
# Python Patch Tool compatibility launcher.
# Canonical routing lives in python_patch_entry.py.
set -euo pipefail

TOOLS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$TOOLS_DIR/.." && pwd)"
ENTRY="$TOOLS_DIR/python_patch_entry.py"

if ! command -v python3 >/dev/null 2>&1; then
  echo "ERROR: Python 3.10+ is required but python3 was not found in PATH." >&2
  exit 2
fi
if [ ! -f "$ENTRY" ]; then
  echo "ERROR: Missing Patch Tool entrypoint: $ENTRY" >&2
  exit 2
fi

exec python3 "$ENTRY" --project-root "$PROJECT_ROOT" -- "$@"
