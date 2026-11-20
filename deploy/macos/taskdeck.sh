#!/usr/bin/env bash
set -euo pipefail

# TaskDeck macOS Docker Desktop launcher. Project and home are isolated by
# a stable Compose project key; only the host loopback port is published.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/deploy/docker/compose.yaml"

usage() {
  cat <<'USAGE'
Usage: ./deploy/macos/taskdeck.sh <doctor|start|stop|restart|update|status|logs|open> [project-path]
Default project-path: current directory (or TASKDECK_WORKSPACE).
Optional env: TASKDECK_PORT=42882 TASKDECK_PROFILE=runtime|dev TASKDECK_IMAGE=taskdeck:local
Starts a Linux backend in Docker Desktop; terminal commands run inside Linux.
USAGE
}

die() { echo "ERROR: $*" >&2; exit 1; }

ACTION="${1:-start}"
if [[ "$ACTION" == "-h" || "$ACTION" == "--help" || "$ACTION" == "help" ]]; then
  usage
  exit 0
fi

case "$ACTION" in
  doctor|start|stop|restart|update|status|logs|open) ;;
  *) usage >&2; die "Unknown action: $ACTION" ;;
esac

PROJECT_INPUT="${2:-${TASKDECK_WORKSPACE:-$PWD}}"
[[ -d "$PROJECT_INPUT" ]] || die "Project folder does not exist: $PROJECT_INPUT"
PROJECT_PATH="$(cd "$PROJECT_INPUT" && pwd -P)"
[[ -n "$PROJECT_PATH" ]] || die "Cannot resolve project path"

command -v docker >/dev/null 2>&1 || die "Install Docker Desktop for macOS first"
docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is required"
docker info >/dev/null 2>&1 || die "Docker Desktop is not running"

export TASKDECK_WORKSPACE="$PROJECT_PATH"
export TASKDECK_PORT="${TASKDECK_PORT:-42882}"
export TASKDECK_PROFILE="${TASKDECK_PROFILE:-runtime}"
export TASKDECK_IMAGE="${TASKDECK_IMAGE:-taskdeck:local}"

[[ "$TASKDECK_PORT" =~ ^[0-9]+$ ]] && ((TASKDECK_PORT>=1 && TASKDECK_PORT<=65535)) ||
  die "TASKDECK_PORT must be between 1 and 65535"
case "$TASKDECK_PROFILE" in runtime|dev) ;; *) die "TASKDECK_PROFILE must be runtime or dev" ;; esac

# macOS provides shasum. Docker Compose names stay deterministic per project.
command -v shasum >/dev/null 2>&1 || die "macOS shasum is required"
project_id="$(printf '%s' "$PROJECT_PATH" | shasum -a 256 | cut -c1-12)"
COMPOSE_PROJECT="taskdeckmac_${project_id}"
compose() { docker compose -p "$COMPOSE_PROJECT" -f "$COMPOSE_FILE" "$@"; }

case "$ACTION" in
  doctor)
    echo "Host: $(uname -sm)"
    echo "Workspace: $PROJECT_PATH"
    echo "Compose project: $COMPOSE_PROJECT"
    echo "Listen: https://localhost:$TASKDECK_PORT"
    echo "Profile: $TASKDECK_PROFILE"
    echo "Docker/Compose: available"
    echo "Credentials: $PROJECT_PATH/.vscode/vscode_tasks_menu.ini (never commit)"
    ;;
  start|update)
    compose up -d --build
    echo "TaskDeck: https://localhost:$TASKDECK_PORT"
    echo "Credentials: $PROJECT_PATH/.vscode/vscode_tasks_menu.ini"
    if [[ "${TASKDECK_NO_BROWSER:-0}" != "1" ]] && command -v open >/dev/null 2>&1; then
      open "https://localhost:$TASKDECK_PORT" || true
    fi
    ;;
  restart)
    compose restart
    ;;
  stop)
    compose down # Preserve named volumes.
    ;;
  status)
    compose ps
    ;;
  logs)
    compose logs -f
    ;;
  open)
    open "https://localhost:$TASKDECK_PORT"
    ;;
esac
