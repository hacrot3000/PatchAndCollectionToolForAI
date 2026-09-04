#!/usr/bin/env bash
# Python Patch Tool v6.13.0 public launcher.
# SANDBOX/worktree transaction mode is permanently disabled at this boundary.
set -euo pipefail
TOOLS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$TOOLS_DIR/.." && pwd)"
LIB_DIR="$TOOLS_DIR/_patch_lib"
RUNNER="$LIB_DIR/python_patch_runner.py"
COLLECTOR="$LIB_DIR/python_patch_readonly_collector.py"
COLLECT_COMPAT="$LIB_DIR/python_patch_collect_compat.py"
DISPATCHER="$LIB_DIR/python_patch_queue_dispatcher.py"
COLLECT_PROGRESS="$LIB_DIR/python_patch_collect_progress_v6_7.py"

export PYTHONPATH="$LIB_DIR${PYTHONPATH:+:$PYTHONPATH}"

if [ "$#" -eq 0 ]; then
  if [ ! -f "$DISPATCHER" ]; then
    echo "ERROR: Missing queue dispatcher: $DISPATCHER" >&2
    exit 2
  fi
  exec python3 "$DISPATCHER" --project-root "$PROJECT_ROOT"
fi

if [ "${1:-}" = "collect" ]; then
  if [ ! -f "$COLLECT_COMPAT" ]; then
    echo "ERROR: Missing COLLECT compatibility layer: $COLLECT_COMPAT" >&2
    exit 2
  fi
  if [ ! -f "$COLLECT_PROGRESS" ]; then
    echo "ERROR: Missing collect progress supervisor: $COLLECT_PROGRESS" >&2
    exit 2
  fi
  shift
  exec python3 "$COLLECT_PROGRESS" --project-root "$PROJECT_ROOT" --collector "$COLLECT_COMPAT" -- "$@"
fi

if [ ! -f "$RUNNER" ]; then
  echo "ERROR: Missing Patch Tool core: $RUNNER" >&2
  exit 2
fi

# v6.13.0 invariant: SANDBOX/Git-worktree transaction execution is removed.
# The installed private core may still expose historical transaction options,
# so every documented PATCH execution route is forced to --transaction off.
# Utility-only routes such as paths/help remain untouched.
filtered=()
force_inplace=0
skip_transaction_value=0
stripped_legacy_transaction=0
for arg in "$@"; do
  arg_lower="${arg,,}"
  if [ "$skip_transaction_value" -eq 1 ]; then
    skip_transaction_value=0
    # Historical --transaction accepts only these values.  Do not consume a
    # following PATCH option if the caller supplied malformed syntax such as
    # "--transaction --all"; swallowing it could drop the in-place guard.
    case "$arg_lower" in
      off|auto|required)
        continue
        ;;
    esac
  fi
  case "$arg_lower" in
    --transaction)
      stripped_legacy_transaction=1
      skip_transaction_value=1
      ;;
    --transaction=*)
      stripped_legacy_transaction=1
      ;;
    --keep-failed-sandbox|--keep-failed-sandbox=*)
      stripped_legacy_transaction=1
      ;;
    --patch|--all|--select)
      filtered+=("$arg")
      force_inplace=1
      ;;
    *.zip|*.py|*.tar.gz|*.tgz)
      filtered+=("$arg")
      force_inplace=1
      ;;
    *)
      filtered+=("$arg")
      ;;
  esac
done

# Any non-utility legacy invocation may execute PATCH work even when it uses
# short/historical flags unknown to this overlay (for example `-a -y`).
# Fail closed toward in-place execution: only a small documented utility
# allowlist is permitted to reach the core without `--transaction off`.
if [ "$force_inplace" -eq 0 ] && [ "${#filtered[@]}" -gt 0 ]; then
  first_lower="${filtered[0],,}"
  case "$first_lower" in
    paths|help|--help|-h|version|--version)
      ;;
    *)
      force_inplace=1
      ;;
  esac
fi

if [ "$force_inplace" -eq 1 ]; then
  exec python3 "$RUNNER" "${filtered[@]}" --transaction off
fi

# Fail closed if an invocation contained only obsolete transaction/SANDBOX
# switches.  Never strip them and then fall through to the legacy core with
# zero arguments, because that core may consult an old transaction default.
if [ "$stripped_legacy_transaction" -eq 1 ] && [ "${#filtered[@]}" -eq 0 ]; then
  echo "ERROR: obsolete transaction/SANDBOX flags cannot be used as a standalone command." >&2
  echo "Use ./tools/run_python_patches.sh with no arguments for the normal queue." >&2
  exit 2
fi

# Non-execution utility commands (for example paths/help) are passed through
# without adding execution-only arguments.
exec python3 "$RUNNER" "${filtered[@]}"
