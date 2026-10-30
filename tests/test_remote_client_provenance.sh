#!/usr/bin/env bash
# Offline negative tests. A mocked verifier tests the local fail-closed policy,
# not actual Sigstore cryptography or availability of a GitHub attestation.
set -euo pipefail
repo="$(cd "$(dirname "$0")/.." && pwd)"
verify="$repo/deploy/remote/verify_client.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/contents"
printf '#!/bin/sh\nexit 0\n' > "$tmp/contents/taskdeck_remote_login"
printf 'bad\n' > "$tmp/contents/other"
tar -czf "$tmp/good.tar.gz" -C "$tmp/contents" taskdeck_remote_login
tar -czf "$tmp/bad.tar.gz" -C "$tmp/contents" other
cat > "$tmp/bin/gh" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
[ "$#" -eq 12 ] || exit 17
[ "$1" = attestation ] && [ "$2" = verify ] || exit 17
[ "$4" = --repo ] && [ "$5" = hacrot3000/PatchAndCollectionToolForAI ] || exit 17
[ "$6" = --signer-workflow ] && [ "$7" = hacrot3000/PatchAndCollectionToolForAI/.github/workflows/taskdeck_remote_client_provenance.yml ] || exit 17
[ "$8" = --source-digest ] && [ "$9" = aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ] || exit 17
[ "${10}" = --source-ref ] && [ "${11}" = refs/heads/main ] || exit 17
[ "${12}" = --deny-self-hosted-runners ] || exit 17
[ "${GH_MOCK_FAIL:-0}" = 0 ] || exit 18
MOCK
chmod +x "$tmp/bin/gh"
export PATH="$tmp/bin:$PATH"
sha="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
bash "$verify" "$tmp/good.tar.gz" "$sha" >/dev/null
if bash "$verify" "$tmp/bad.tar.gz" "$sha" >/dev/null 2>&1; then
  echo "FAIL: unexpected tar member was accepted" >&2; exit 1
fi
if bash "$verify" "$tmp/good.tar.gz" "not-a-full-hash" >/dev/null 2>&1; then
  echo "FAIL: unknown commit was accepted" >&2; exit 1
fi
if GH_MOCK_FAIL=1 bash "$verify" "$tmp/good.tar.gz" "$sha" >/dev/null 2>&1; then
  echo "FAIL: invalid provenance was accepted" >&2; exit 1
fi
ln -s "$tmp/good.tar.gz" "$tmp/symlink.tar.gz"
if bash "$verify" "$tmp/symlink.tar.gz" "$sha" >/dev/null 2>&1; then
  echo "FAIL: symlink archive was accepted" >&2; exit 1
fi
mkdir -p "$tmp/linkdir"
ln -s "$tmp/contents/taskdeck_remote_login" "$tmp/linkdir/taskdeck_remote_login"
tar -czf "$tmp/member-link.tar.gz" -C "$tmp/linkdir" taskdeck_remote_login
if bash "$verify" "$tmp/member-link.tar.gz" "$sha" >/dev/null 2>&1; then
  echo "FAIL: attested archive with symlink executable was accepted" >&2; exit 1
fi
tar -czf "$tmp/duplicate.tar.gz" -C "$tmp/contents" taskdeck_remote_login taskdeck_remote_login
if bash "$verify" "$tmp/duplicate.tar.gz" "$sha" >/dev/null 2>&1; then
  echo "FAIL: attested archive with duplicate executable was accepted" >&2; exit 1
fi
printf 'malformed gzip\n' > "$tmp/corrupt.tar.gz"
if bash "$verify" "$tmp/corrupt.tar.gz" "$sha" >/dev/null 2>&1; then
  echo "FAIL: corrupt archive was accepted" >&2; exit 1
fi
echo "PASS: independent native client verifier fail-closed policy (mock verifier)"
