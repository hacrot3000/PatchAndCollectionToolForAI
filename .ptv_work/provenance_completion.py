#!/usr/bin/env python3
from pathlib import Path
import json


def insert_after(path: str, needle: str, addition: str) -> None:
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    if addition.strip() in text:
        return
    if needle not in text:
        raise SystemExit(f"missing insertion anchor in {path}: {needle!r}")
    p.write_text(text.replace(needle, needle + addition, 1), encoding="utf-8")


def prepend_section(path: str, marker: str, section: str) -> None:
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    if marker in text:
        return
    lines = text.splitlines(True)
    if not lines:
        raise SystemExit(f"empty doc: {path}")
    p.write_text(lines[0] + "\n" + section.strip() + "\n\n" + "".join(lines[1:]), encoding="utf-8")


def append_section(path: str, marker: str, section: str) -> None:
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    if marker in text:
        return
    p.write_text(text.rstrip() + "\n\n" + section.strip() + "\n", encoding="utf-8")


insert_after(
    "_patch_lib/python_patch_health.py",
    '    "tools/_patch_lib/python_patch_package_schema.py",\n',
    '    "tools/_patch_lib/python_patch_provenance.py",\n',
)
insert_after(
    "_patch_lib/python_patch_health.py",
    '    "tools/_patch_lib/docs/MANUAL_EXECUTION_WORKFLOW.md",\n',
    '    "tools/_patch_lib/docs/PROVENANCE_SIGNATURE_TRUST.md",\n',
)
insert_after(
    "_patch_lib/self_test_python_patch_tool_v6_20_0.py",
    " 'self_test_patch_preflight_v6_20_0.py',\n",
    " 'self_test_provenance_signature_v6_21_0.py',\n",
)
insert_after(
    "_patch_lib/python_patch_ai_sync.py",
    '    "tools/_patch_lib/docs/MANUAL_EXECUTION_WORKFLOW.md",\n',
    '    "tools/_patch_lib/docs/PROVENANCE_SIGNATURE_TRUST.md",\n',
)
insert_after(
    "_patch_lib/self_test_ai_sync_v6_20_0.py",
    "assert 'tools/_patch_lib/docs/MANUAL_EXECUTION_WORKFLOW.md' in sync.SYNC_DOCS\n",
    "assert 'tools/_patch_lib/docs/PROVENANCE_SIGNATURE_TRUST.md' in sync.SYNC_DOCS\n",
)

checklist = Path("_patch_lib/docs/PATCH_PACKAGE_CHECKLIST.json")
data = json.loads(checklist.read_text(encoding="utf-8"))
check = (
    "if manifest.provenance is present, use exactly ptv-patch-signature-v1 + ed25519 + a locally trusted key_id; "
    "never invent a signature or trust key; local require_signature policy may reject unsigned PATCHes"
)
if check not in data["checks"]:
    data["checks"].append(check)
checklist.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

append_section(
    "_patch_lib/docs/PATCH_PACKAGE_GUIDE.md",
    "## Cryptographic provenance / signed PATCH trust",
    """## Cryptographic provenance / signed PATCH trust

PATCH packages may optionally declare `manifest.provenance` using the documented `ptv-patch-signature-v1` / `ed25519` contract. Verification covers canonical manifest semantics plus every regular package file and runs during preflight before payload/source mutation.

Trust is operator-local only. Configure `provenance.trusted_ed25519_keys` in `.python_patch_tool.json`; set `provenance.require_signature=true` only when the project must reject every unsigned PATCH, including recognized legacy PATCHes. A present but invalid signature, an unknown signer, malformed trust data, or a required-but-missing signature fails closed.

The tool verifies signatures only. Private-key generation/storage, signing services, PKI, remote key lookup/registry, COLLECT signing, and reproducible raw ZIP-byte signing are outside this contract. See `PROVENANCE_SIGNATURE_TRUST.md`.""",
)
append_section(
    "_patch_lib/docs/AI_USAGE_CONTRACT.md",
    "## PATCH provenance / signature trust",
    """## PATCH provenance / signature trust

When a client requests a signed PATCH, follow `PROVENANCE_SIGNATURE_TRUST.md` exactly. Do not invent trust roots, key IDs, public keys, signatures, or signing commands. `manifest.provenance` is optional unless the operator's local project policy requires it; if present it is verified before payload/source mutation and any invalid/untrusted signature is a hard preflight failure. The AI must not assume access to private signing keys or remote trust services.""",
)
prepend_section(
    "_patch_lib/docs/CAPABILITY_LEDGER.md",
    "## Current additive capability — cryptographic provenance / signature trust",
    """## Current additive capability — cryptographic provenance / signature trust

- **COMPLETE (current capability; historical #65 was NOT STARTED):** PATCH manifests may carry a strict Ed25519 provenance signature (`ptv-patch-signature-v1`).
- **FAIL-CLOSED:** a present signature is verified against operator-local trusted public keys before payload/source mutation; tampered manifest/package content, malformed signature data, or an untrusted signer is rejected.
- **LOCAL POLICY:** `.python_patch_tool.json` may set `provenance.require_signature=true`, which also rejects unsigned recognized legacy PATCHes. Default remains compatible with existing unsigned PATCHes.
- **SCOPE BOUNDARY:** verifier/trust policy only. Private-key generation/management, PKI, remote trust registry/network lookup, COLLECT signing and reproducible raw ZIP-byte signing remain out of scope.
- Behavioral gate: `self_test_provenance_signature_v6_21_0.py`. Contract: `PROVENANCE_SIGNATURE_TRUST.md`.
- Historical `CURRENT_CAPABILITY_DISPOSITION.json` remains 95/95 COMPLETE coverage; historical #65 is not rewritten as if it had been COMPLETE in v5.""",
)
prepend_section(
    "_patch_lib/docs/PYTHON_PATCH_TOOL_FEATURE_STATUS.md",
    "## COMPLETE — local Ed25519 PATCH provenance/signature trust",
    """## COMPLETE — local Ed25519 PATCH provenance/signature trust

- Optional `manifest.provenance` contract: `ptv-patch-signature-v1`, `ed25519`, `key_id`, `signature`.
- Canonical signature input binds manifest semantics (excluding the signature value itself) plus path/size/SHA-256 for every regular package file.
- Trust comes only from local `.python_patch_tool.json`; optional `require_signature` can reject all unsigned PATCHes, including legacy PATCHes.
- Invalid/tampered/untrusted signatures fail before payload/source mutation. Existing unsigned PATCH compatibility remains the default.
- No signing/private-key/PKI/remote registry/COLLECT-signing/reproducible-ZIP feature is added.
- Gate: `self_test_provenance_signature_v6_21_0.py`.""",
)
prepend_section(
    "implementing.md",
    "## COMPLETE — Cryptographic provenance / signature trust",
    """## COMPLETE — Cryptographic provenance / signature trust

The accepted provenance/signature task is complete: self-contained strict Ed25519 verification, canonical manifest+package-file binding, operator-local trust store, optional fail-closed `require_signature` policy (including legacy PATCH), pre-payload enforcement, AI-sync/documentation coverage, Tool Health/package-required coverage, and permanent master regression. Scope intentionally stops at verification/trust; no private-key manager, signer, PKI, remote registry, COLLECT signing, or reproducible ZIP-byte feature was introduced.""",
)
prepend_section(
    "PYTHON_PATCH_TOOL_FEATURES_VI.md",
    "## HOÀN THÀNH — Xác minh provenance/chữ ký PATCH Ed25519",
    """## HOÀN THÀNH — Xác minh provenance/chữ ký PATCH Ed25519

- PATCH có thể khai báo `manifest.provenance` theo contract `ptv-patch-signature-v1` / `ed25519`.
- Chữ ký ràng buộc manifest canonical và toàn bộ file thường trong package bằng path/size/SHA-256; kiểm tra diễn ra trước khi payload sửa source.
- Trust store chỉ lấy từ cấu hình local `.python_patch_tool.json`. Có thể bật `provenance.require_signature=true` để từ chối cả PATCH legacy không ký.
- Chữ ký sai, package/manifest bị sửa hoặc signer không được trust đều fail-closed. Mặc định vẫn tương thích PATCH cũ không ký.
- Phạm vi chỉ là verify/trust; không thêm quản lý private key, signer, PKI, registry/network lookup, ký COLLECT hay reproducible ZIP bytes.""",
)
