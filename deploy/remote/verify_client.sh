#!/usr/bin/env bash
# Verify the independently downloaded native login-only client BEFORE unpacking.
# No PHP Portal / Relay URL is consulted, and this script never executes the artifact.
set -euo pipefail

REPOSITORY="hacrot3000/PatchAndCollectionToolForAI"
SIGNER="$REPOSITORY/.github/workflows/taskdeck_remote_client_provenance.yml"
TRUSTED_REF="refs/heads/main"

if [ "$#" -ne 2 ]; then
  echo "Usage: bash verify_client.sh ARCHIVE.tar.gz TRUSTED_40_HEX_GIT_SHA" >&2
  exit 2
fi
archive="$1"
expected_sha="$2"
if [[ ! "$expected_sha" =~ ^[0-9a-f]{40}$ ]]; then
  echo "DENY: source SHA must be an independently verified full lowercase commit hash" >&2
  exit 2
fi
if [ ! -f "$archive" ] || [ -L "$archive" ] || [[ "$archive" != *.tar.gz ]]; then
  echo "DENY: expected an existing non-symlink .tar.gz archive" >&2
  exit 2
fi
if ! command -v gh >/dev/null 2>&1; then
  echo "DENY: trusted GitHub CLI (gh) with attestation support is required" >&2
  exit 2
fi
if ! command -v tar >/dev/null 2>&1; then
  echo "DENY: tar is required for archive inspection" >&2
  exit 2
fi
# Exact repository, GitHub Actions signing workflow, source digest, branch,
# and hosted runner are all independently enforced by GitHub's verifier.
# A SHA256SUMS file from the same download alone is NOT an authentication.
if ! gh attestation verify "$archive" \
  --repo "$REPOSITORY" \
  --signer-workflow "$SIGNER" \
  --source-digest "$expected_sha" \
  --source-ref "$TRUSTED_REF" \
  --deny-self-hosted-runners >/dev/null; then
  echo "DENY: attestation, source identity, workflow or archive digest did not verify" >&2
  exit 1
fi
# The approved workflow emits exactly one regular file per archive. Reject
# paths, duplicate names and links before a user attempts manual extraction.
members="$(tar -tzf "$archive")" || {
  echo "DENY: malformed archive" >&2
  exit 1
}
if [ "$members" != "taskdeck_remote_login" ]; then
  echo "DENY: unexpected archive members or path traversal" >&2
  exit 1
fi
details="$(tar -tvzf "$archive")" || exit 1
if [[ "$details" != -* ]]; then
  echo "DENY: archive member is not a regular file" >&2
  exit 1
fi
echo "PASS: attested native LOGIN-ONLY client; independently pinned source $expected_sha"
echo "Archive verified. Extract manually ONLY in a private directory. Remote Control is still disabled."
