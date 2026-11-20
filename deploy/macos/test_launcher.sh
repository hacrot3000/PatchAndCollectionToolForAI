#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin" "$TMP/project one" "$TMP/project two"
cat > "$TMP/bin/docker" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TASKDECK_MOCK_DOCKER_LOG"
exit 0
SH
chmod +x "$TMP/bin/docker"
export TASKDECK_MOCK_DOCKER_LOG="$TMP/docker.log"
export PATH="$TMP/bin:$PATH"
export TASKDECK_NO_BROWSER=1

doctor_a="$(bash "$ROOT/deploy/macos/taskdeck.sh" doctor "$TMP/project one")"
doctor_b="$(bash "$ROOT/deploy/macos/taskdeck.sh" doctor "$TMP/project two")"
echo "$doctor_a" | grep -F "Workspace: $TMP/project one" >/dev/null
id_a="$(printf '%s\n' "$doctor_a" | sed -n 's/^Compose project: //p')"
id_b="$(printf '%s\n' "$doctor_b" | sed -n 's/^Compose project: //p')"
[[ -n "$id_a" && -n "$id_b" && "$id_a" != "$id_b" ]] ||
  { echo "Two projects reused the same Compose identifier" >&2; exit 1; }

bash "$ROOT/deploy/macos/taskdeck.sh" start "$TMP/project one"
grep -F -- "-p $id_a" "$TMP/docker.log" | grep -F -- "up -d --build" >/dev/null

bash "$ROOT/deploy/macos/taskdeck.sh" stop "$TMP/project one"
grep -F -- "-p $id_a" "$TMP/docker.log" | grep -F -- "down" >/dev/null
if grep -Fq -- "down -v" "$TMP/docker.log"; then
  echo "Stop deleted persistent home volume" >&2
  exit 1
fi

if TASKDECK_PORT=70000 bash "$ROOT/deploy/macos/taskdeck.sh" doctor "$TMP/project one" >/dev/null 2>&1; then
  echo "Accepted invalid port" >&2
  exit 1
fi

echo "macOS Docker launcher smoke: PASS"
