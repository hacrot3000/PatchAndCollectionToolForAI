#!/usr/bin/env bash
set -euo pipefail
image="${1:-taskdeck:local}"
command -v docker >/dev/null || { echo "Docker not available" >&2; exit 2; }
echo "Checking TaskDeck runtime commands in $image"
docker run --rm --entrypoint bash "$image" -ec '
  for tool in bash sh git ssh ssh-keygen scp sftp curl wget jq rsync find grep sed awk \
    tar zip unzip gzip xz python3 make ps ip ping ls cat chmod ssh-add; do
    command -v "$tool" >/dev/null || { echo "Missing runtime tool: $tool" >&2; exit 1; }
  done
  git lfs version >/dev/null
  ssh -V 2>&1 | grep -q OpenSSH
  python3 -c "import sys; assert sys.version_info >= (3,10)"
  test -f "$TASKDECK_PATCH_RUNTIME"
  /usr/local/bin/taskdeck --version
'
workspace=$(mktemp -d "${TMPDIR:-/tmp}/taskdeck Windows smoke.XXXXXXXX")
name="taskdeck-smoke-$$"
home="taskdeck-smoke-home-$$"
cleanup(){
  docker rm -f "$name" >/dev/null 2>&1 || true
  docker volume rm "$home" >/dev/null 2>&1 || true
  rm -rf -- "$workspace"
}
trap cleanup EXIT
chmod 0777 "$workspace" # GitHub runner UID differs from container UID 1000.
echo "Checking read-write bind mount (including spaces) and secure HTTPS startup"
docker run --detach --init --name "$name" \
  --security-opt no-new-privileges \
  --mount "type=bind,source=$workspace,target=/workspace" \
  --mount "type=volume,source=$home,target=/home/taskdeck" \
  --publish "127.0.0.1:42882:42882" \
  --env TASKDECK_INIT_WORKSPACE=1 \
  --env TASKDECK_AUTO_CONFIG=1 \
  "$image" >/dev/null
ready=0
for i in $(seq 1 35); do
  code=$(curl --silent --insecure --output /dev/null --write-out '%{http_code}' \
    --connect-timeout 2 https://127.0.0.1:42882/ || true)
  if [ "$code" != "000" ] && [ -n "$code" ]; then ready=1; break; fi
  sleep 1
done
if [ "$ready" != "1" ]; then docker logs "$name" >&2 || true; exit 1; fi
docker exec "$name" sh -ec '
  test -f /workspace/.vscode/tasks.json
  test -f /workspace/.vscode/vscode_tasks_menu.ini
  grep -qx "protocol = https" /workspace/.vscode/vscode_tasks_menu.ini
  grep -qx "bind = 0.0.0.0" /workspace/.vscode/vscode_tasks_menu.ini
  grep -qx "enabled = true" /workspace/.vscode/vscode_tasks_menu.ini
  grep -Eq "^password = [0-9a-f]{48}$" /workspace/.vscode/vscode_tasks_menu.ini
  test "$(stat -c %a /workspace/.vscode/vscode_tasks_menu.ini)" = "600"
'
test -f "$workspace/.vscode/tasks.json"
echo "PASS: TaskDeck image has Git/OpenSSH/Python/terminal tools, secure HTTPS and Windows-style mounted workspace"
