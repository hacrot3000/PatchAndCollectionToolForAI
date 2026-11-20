#!/bin/sh
# TaskDeck Docker initialization. Execute as non-root (UID 1000 by default).
set -eu
workspace=${TASKDECK_WORKSPACE:-/workspace}
case "${1:-}" in
  --version|--help|-h) exec /usr/local/bin/taskdeck "$@" ;;
esac
if [ ! -d "$workspace" ]; then
  echo "TaskDeck: workspace not mounted: $workspace" >&2
  exit 64
fi
if [ ! -w "$workspace" ]; then
  echo "TaskDeck: workspace is not writable by container UID $(id -u): $workspace" >&2
  echo "Adjust the bind mount permissions or run with a matching UID/GID." >&2
  exit 73
fi
case "$workspace" in
  /) echo "TaskDeck: refusing to use the filesystem root as a workspace" >&2; exit 64 ;;
esac

tasks="$workspace/.vscode/tasks.json"
conf="$workspace/.vscode/vscode_tasks_menu.ini"
if [ ! -f "$tasks" ]; then
  if [ "${TASKDECK_INIT_WORKSPACE:-0}" != "1" ]; then
    echo "TaskDeck: $tasks is missing; set TASKDECK_INIT_WORKSPACE=1 to initialize this mount." >&2
    exit 66
  fi
  mkdir -p "$workspace/.vscode"
  if [ ! -e "$tasks" ]; then
    printf '{\n  "version": "2.0.0",\n  "tasks": []\n}\n' > "$tasks"
    echo "TaskDeck: created minimal .vscode/tasks.json (no task commands are added)."
  fi
fi

if [ ! -f "$conf" ]; then
  if [ "${TASKDECK_AUTO_CONFIG:-0}" != "1" ]; then
    echo "TaskDeck: $conf is missing; set TASKDECK_AUTO_CONFIG=1 for an authenticated HTTPS config." >&2
    exit 66
  fi
  # 24 random bytes from the OS CSPRNG -> 192-bit initial admin password.
  # Never pass credentials through process arguments or print them to logs.
  umask 077
  password="$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')"
  [ "${#password}" -eq 48 ] || { echo "Cannot generate secure password" >&2; exit 70; }
  tmp="$(mktemp "$workspace/.vscode/.taskdeck.ini.XXXXXXXX")"
  trap 'rm -f "$tmp"' EXIT HUP INT TERM
  cat > "$tmp" <<EOF
[server]
protocol = https
bind = 0.0.0.0
port = 42882
open_browser = false

[auth]
enabled = true
username = admin
password = $password
EOF
  chmod 600 "$tmp"
  if [ ! -e "$conf" ]; then
    mv "$tmp" "$conf"
    echo "TaskDeck: created authenticated HTTPS config at $conf."
    echo "Retrieve initial credentials locally via your mounted .vscode/vscode_tasks_menu.ini; never commit this file."
  else
    rm -f "$tmp"
  fi
  trap - EXIT HUP INT TERM
fi

exec /usr/local/bin/taskdeck --workspace "$workspace" "$@"
