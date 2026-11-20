# PATCH provenance and signature trust — v6.21.0

This contract adds **local Ed25519 signature verification for PATCH packages**. It verifies provenance before payload/source mutation. It does not add private-key management, PKI, a remote trust registry, network key discovery, or reproducible raw ZIP bytes.

## Local trust policy

Trust is configured only by the operator in the project-local `.python_patch_tool.json`:

```json
{
  "provenance": {
    "require_signature": true,
    "trusted_ed25519_keys": {
      "release-key-1": "BASE64_OF_RAW_32_BYTE_ED25519_PUBLIC_KEY"
    }
  }
}
```

- `require_signature` defaults to `false` so existing unsigned PATCH packages remain compatible unless the operator opts in to mandatory signatures.
- `trusted_ed25519_keys` maps a bounded safe `key_id` to canonical Base64 of a raw 32-byte Ed25519 public key.
- A PATCH that contains `manifest.provenance` is always verified. A present signature from an untrusted key or an invalid/tampered signature fails closed even when `require_signature=false`.
- With `require_signature=true`, unsigned current packages and manifestless legacy PATCH archives are rejected before payload execution.

## Manifest

```json
{
  "schema_version": 1,
  "patch": {"id": "example"},
  "provenance": {
    "format": "ptv-patch-signature-v1",
    "algorithm": "ed25519",
    "key_id": "release-key-1",
    "signature": "BASE64_OF_RAW_64_BYTE_ED25519_SIGNATURE"
  }
}
```

Only `ed25519` and `ptv-patch-signature-v1` are accepted by this contract.

## Exactly what is signed

The Ed25519 message is canonical UTF-8 JSON with `sort_keys=true`, compact separators `,` / `:`, and `ensure_ascii=false` for this object:

```json
{
  "format": "ptv-patch-signature-v1",
  "manifest": "<parsed manifest object with only provenance.signature removed>",
  "files": [
    {"path": "...", "size": 123, "sha256": "..."}
  ]
}
```

`files` contains **every regular package file except `PATCH_TOOL_MANIFEST.json`**, sorted by POSIX relative path. The manifest is represented by its parsed canonical semantics, so formatting/key-order changes to the manifest file do not invalidate a signature, while semantic manifest changes do. Payload/resource byte changes, extra package files, removed files, renamed files, or size/hash changes invalidate the signature.

The raw ZIP container bytes are intentionally not part of this signature contract. Repacking the same signed semantic manifest and identical package files can therefore retain the same valid signature. Reproducible raw archive bytes remain outside this task.

## Author-side signing

Patch Tool only verifies signatures; it never reads or stores a private signing key. A PATCH author may build an extracted package directory with `provenance.signature` temporarily empty, then use the packaged helper to obtain the exact message:

```python
from pathlib import Path
import json
from python_patch_provenance import canonical_signed_message

package_dir = Path("staging_patch")
manifest = json.loads((package_dir / "PATCH_TOOL_MANIFEST.json").read_text())
message = canonical_signed_message(manifest, package_dir)
```

Sign `message` with the matching Ed25519 private key using an author-controlled signing tool/library, Base64-encode the raw 64-byte signature, then place it in `manifest.provenance.signature`. Private key lifecycle and signing-key generation are deliberately outside Patch Tool.

## Failure model

Before any payload/source mutation:

- missing signature while required → `signature_required`;
- unknown/untrusted `key_id` → `signer_untrusted`;
- malformed, non-canonical, tampered, or cryptographically invalid signature → `signature_invalid`;
- malformed local provenance policy → `provenance_invalid` / project-config failure.

A successful preflight records `key_id`, algorithm, signed-message SHA-256, file count, and whether local policy required the signature. No private material is written to reports.
