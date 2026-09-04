#!/usr/bin/env bash
# Python Patch Tool v6.7.11 public launcher.
# SANDBOX/worktree transaction mode is permanently disabled at this boundary.
set -euo pipefail
TOOLS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$TOOLS_DIR/.." && pwd)"
LIB_DIR="$TOOLS_DIR/_patch_lib"
RUNNER="$LIB_DIR/python_patch_runner.py"
COLLECTOR="$LIB_DIR/python_patch_readonly_collector.py"
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
  if [ ! -f "$COLLECTOR" ]; then
    echo "ERROR: Missing readonly collector: $COLLECTOR" >&2
    exit 2
  fi
  if [ ! -f "$COLLECT_PROGRESS" ]; then
    echo "ERROR: Missing collect progress supervisor: $COLLECT_PROGRESS" >&2
    exit 2
  fi
  shift
  exec python3 "$COLLECT_PROGRESS" --project-root "$PROJECT_ROOT" --collector "$COLLECTOR" -- "$@"
fi

if [ ! -f "$RUNNER" ]; then
  echo "ERROR: Missing Patch Tool core: $RUNNER" >&2
  exit 2
fi

# v6.7.11 invariant: SANDBOX/Git-worktree transaction execution is removed.
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
    --patch|--all|--select|-a)
      filtered+=("$arg")
      force_inplace=1
      ;;
    -y|--yes|--zip-failed|--keep-failed-zip|--move)
      # Historical execution modifiers are commonly combined with -a/selection
      # and may also cause an old core to enter its execution path. Treat them
      # as PATCH-route evidence so the removed transaction/worktree default can
      # never be consulted even when the caller omits the primary selector flag.
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

if [ "$force_inplace" -eq 1 ]; then
  exec python3 "$RUNNER" "${filtered[@]}" --transaction off
fi

# Fail closed whenever obsolete transaction/SANDBOX flags were supplied but no
# PATCH execution route was positively identified.  Do not pass leftovers to
# the legacy core: an unknown positional/utility-shaped argument could be
# interpreted differently by an older core and consult a stale worktree default.
if [ "$stripped_legacy_transaction" -eq 1 ] && [ "$force_inplace" -eq 0 ]; then
  echo "ERROR: obsolete transaction/SANDBOX flags are not accepted without a recognized PATCH route." >&2
  echo "Use ./tools/run_python_patches.sh with no arguments for the normal queue." >&2
  exit 2
fi

# Non-execution utility commands (for example paths/help) are passed through
# without adding execution-only arguments.
exec python3 "$RUNNER" "${filtered[@]}"
