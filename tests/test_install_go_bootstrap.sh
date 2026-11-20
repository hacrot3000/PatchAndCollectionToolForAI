#!/usr/bin/env bash
# Offline smoke of single-file installer Go bootstrap; no network or sudo.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TEMP="$(mktemp -d "${TMPDIR:-/tmp}/taskdeck-go-bootstrap-test.XXXXXX")"
trap 'rm -rf "$TEMP"' EXIT
sed '/^ensure_go$/,$d' "$ROOT/install.sh" > "$TEMP/functions.sh"
bash -n "$TEMP/functions.sh"

mkdir -p "$TEMP/oldbin"
cat > "$TEMP/oldbin/go" <<'SH'
#!/bin/sh
echo "go version go1.20.0 darwin/arm64"
SH
chmod +x "$TEMP/oldbin/go"
SYSTEM_PATH="$PATH"

(
    export TASKDECK_APP_DIR="$TEMP/app" TASKDECK_INSTALL_DIR="$TEMP/bin"
    export PATH="$TEMP/oldbin:$SYSTEM_PATH"
    source "$TEMP/functions.sh"
    uname() { if [[ "$1" == "-s" ]]; then echo Darwin; else echo arm64; fi; }
    http_download() {
        echo "$1" > "$TEMP/url"
        dd if=/dev/zero of="$2" bs=1024 count=1025 2>/dev/null
    }
    sha256_file() { echo "dd3908cf20959d798c1e06ebe5b72213f0505ce01373b40ce3110975a6cd99c2"; }
    tar() {
        if [[ "$1" == "-tzf" ]]; then
            printf '%s\n' 'go/bin/go'
        elif [[ "$1" == "-xzf" ]]; then
            mkdir -p "$4/go/bin"
            printf '#!/bin/sh\necho "go version go1.27.3 darwin/arm64"\n' > "$4/go/bin/go"
            chmod +x "$4/go/bin/go"
        else
            return 1
        fi
    }
    if go_version_supported; then echo 'Old Go was incorrectly accepted' >&2; exit 1; fi
    ensure_go
    [[ "$(go version)" == "go version go1.27.3 darwin/arm64" ]]
    [[ -x "$APP_ROOT/toolchains/go1.27.3-darwin-arm64/bin/go" ]]
    [[ "$(cat "$TEMP/url")" == "https://go.dev/dl/go1.27.3.darwin-arm64.tar.gz" ]]
    http_download() { echo 'unexpected second download' >&2; return 1; }
    export PATH="$TEMP/oldbin:$SYSTEM_PATH"
    hash -r
    ensure_go
    [[ "$(go version)" == "go version go1.27.3 darwin/arm64" ]]
)
if (
    export TASKDECK_APP_DIR="$TEMP/rejected" TASKDECK_INSTALL_DIR="$TEMP/unused"
    export PATH="$TEMP/oldbin:$SYSTEM_PATH"
    source "$TEMP/functions.sh"
    uname() { if [[ "$1" == "-s" ]]; then echo Darwin; else echo arm64; fi; }
    http_download() { dd if=/dev/zero of="$2" bs=1024 count=1025 2>/dev/null; }
    sha256_file() { echo badsha256; }
    ensure_go
) >"$TEMP/rejected.log" 2>&1; then
    echo 'Installer accepted a mismatched checksum' >&2
    exit 1
fi
[[ ! -e "$TEMP/rejected/toolchains/go1.27.3-darwin-arm64" ]]
grep -Fq 'SHA256 của Go không khớp' "$TEMP/rejected.log"
echo 'TaskDeck Go bootstrap offline smoke PASS'
