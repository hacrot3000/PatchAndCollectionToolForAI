#!/usr/bin/env python3
from __future__ import annotations

import base64
import binascii
import copy
import hashlib
import json
import re
from pathlib import Path
from typing import Any

SIGNED_FORMAT = "ptv-patch-signature-v1"
ALGORITHM = "ed25519"
_MANIFEST_NAME = "PATCH_TOOL_MANIFEST.json"
_KEY_ID_RE = re.compile(r"^[A-Za-z0-9_.:@+-]{1,96}$")
_MAX_TRUSTED_KEYS = 128


class ProvenanceError(ValueError):
    def __init__(self, message: str, *, kind: str = "provenance_invalid"):
        super().__init__(message)
        self.kind = kind


# Minimal strict Ed25519 verification (RFC 8032). Runtime verification is
# self-contained so Patch Tool does not gain a dependency on OpenSSL or a
# third-party Python crypto package. Private-key creation/signing is
# deliberately out of scope: this module verifies public signatures only.
_Q = 2**255 - 19
_L = 2**252 + 27742317777372353535851937790883648493
_D = (-121665 * pow(121666, _Q - 2, _Q)) % _Q
_I = pow(2, (_Q - 1) // 4, _Q)
_IDENTITY = (0, 1)


def _xrecover(y: int) -> int:
    xx = (y * y - 1) * pow(_D * y * y + 1, _Q - 2, _Q) % _Q
    x = pow(xx, (_Q + 3) // 8, _Q)
    if (x * x - xx) % _Q != 0:
        x = (x * _I) % _Q
    if x & 1:
        x = _Q - x
    return x


_BY = (4 * pow(5, _Q - 2, _Q)) % _Q
_BX = _xrecover(_BY)
_B = (_BX, _BY)


def _isoncurve(p: tuple[int, int]) -> bool:
    x, y = p
    return (-x * x + y * y - 1 - _D * x * x * y * y) % _Q == 0


def _edwards(p: tuple[int, int], q: tuple[int, int]) -> tuple[int, int]:
    x1, y1 = p
    x2, y2 = q
    den_x = pow((1 + _D * x1 * x2 * y1 * y2) % _Q, _Q - 2, _Q)
    den_y = pow((1 - _D * x1 * x2 * y1 * y2) % _Q, _Q - 2, _Q)
    return (
        (x1 * y2 + x2 * y1) * den_x % _Q,
        (y1 * y2 + x1 * x2) * den_y % _Q,
    )


def _scalarmult(p: tuple[int, int], scalar: int) -> tuple[int, int]:
    result = _IDENTITY
    addend = p
    n = int(scalar)
    while n:
        if n & 1:
            result = _edwards(result, addend)
        addend = _edwards(addend, addend)
        n >>= 1
    return result


def _encodepoint(p: tuple[int, int]) -> bytes:
    x, y = p
    value = y | ((x & 1) << 255)
    return value.to_bytes(32, "little")


def _decodepoint(raw: bytes) -> tuple[int, int]:
    if len(raw) != 32:
        raise ProvenanceError("Ed25519 point must be exactly 32 bytes", kind="signature_invalid")
    value = int.from_bytes(raw, "little")
    sign = (value >> 255) & 1
    y = value & ((1 << 255) - 1)
    if y >= _Q:
        raise ProvenanceError("non-canonical Ed25519 point encoding", kind="signature_invalid")
    x = _xrecover(y)
    if (x & 1) != sign:
        x = _Q - x
    p = (x, y)
    if not _isoncurve(p) or _encodepoint(p) != raw:
        raise ProvenanceError("invalid Ed25519 point", kind="signature_invalid")
    if p == _IDENTITY or _scalarmult(p, 8) == _IDENTITY or _scalarmult(p, _L) != _IDENTITY:
        raise ProvenanceError("Ed25519 point is outside the prime-order subgroup", kind="signature_invalid")
    return p


def verify_ed25519(public_key: bytes, message: bytes, signature: bytes) -> bool:
    if len(public_key) != 32 or len(signature) != 64:
        return False
    try:
        a = _decodepoint(public_key)
        r_raw = signature[:32]
        r = _decodepoint(r_raw)
        s = int.from_bytes(signature[32:], "little")
        if s >= _L:
            return False
        h = int.from_bytes(hashlib.sha512(r_raw + public_key + message).digest(), "little") % _L
        return _scalarmult(_B, s) == _edwards(r, _scalarmult(a, h))
    except ProvenanceError:
        return False


def _decode_b64_exact(text: Any, *, field: str, size: int, kind: str) -> bytes:
    if not isinstance(text, str) or not text:
        raise ProvenanceError(f"{field} must be non-empty base64", kind=kind)
    try:
        raw = base64.b64decode(text, validate=True)
    except (binascii.Error, ValueError) as exc:
        raise ProvenanceError(f"{field} must be canonical base64", kind=kind) from exc
    if len(raw) != size or base64.b64encode(raw).decode("ascii") != text:
        raise ProvenanceError(f"{field} must encode exactly {size} bytes using canonical base64", kind=kind)
    return raw


def _load_policy(root: Path) -> tuple[bool, dict[str, bytes]]:
    try:
        from python_patch_project_state import load_project_config
        cfg = load_project_config(root)
    except Exception as exc:
        if getattr(exc, "kind", None):
            raise ProvenanceError(str(exc), kind=str(getattr(exc, "kind"))) from exc
        raise ProvenanceError(f"cannot read local provenance policy: {type(exc).__name__}: {exc}") from exc
    raw = cfg.get("provenance", {}) if isinstance(cfg, dict) else {}
    if raw is None:
        raw = {}
    if not isinstance(raw, dict):
        raise ProvenanceError("local provenance config must be an object")
    extra = sorted(set(raw) - {"require_signature", "trusted_ed25519_keys"})
    if extra:
        raise ProvenanceError("local provenance config contains unsupported field(s): " + ", ".join(extra))
    required = raw.get("require_signature", False)
    if not isinstance(required, bool):
        raise ProvenanceError("provenance.require_signature must be boolean")
    table = raw.get("trusted_ed25519_keys", {})
    if table is None:
        table = {}
    if not isinstance(table, dict) or len(table) > _MAX_TRUSTED_KEYS:
        raise ProvenanceError(f"provenance.trusted_ed25519_keys must be an object with at most {_MAX_TRUSTED_KEYS} entries")
    keys: dict[str, bytes] = {}
    for key_id, encoded in table.items():
        if not isinstance(key_id, str) or not _KEY_ID_RE.fullmatch(key_id):
            raise ProvenanceError("trusted Ed25519 key id must match [A-Za-z0-9_.:@+-]{1,96}")
        keys[key_id] = _decode_b64_exact(encoded, field=f"trusted_ed25519_keys[{key_id!r}]", size=32, kind="provenance_invalid")
    return required, keys


def _regular_package_files(extracted: Path) -> list[dict[str, Any]]:
    root = extracted.resolve(strict=True)
    rows: list[dict[str, Any]] = []
    for path in sorted(extracted.rglob("*"), key=lambda p: p.relative_to(extracted).as_posix()):
        if path.is_symlink():
            raise ProvenanceError(f"signed package contains symlink: {path.relative_to(extracted).as_posix()}")
        if path.is_dir():
            continue
        if not path.is_file():
            raise ProvenanceError(f"signed package contains non-regular entry: {path.relative_to(extracted).as_posix()}")
        try:
            path.resolve(strict=True).relative_to(root)
        except Exception as exc:
            raise ProvenanceError(f"signed package path escapes extraction root: {path}") from exc
        rel = path.relative_to(extracted).as_posix()
        if rel == _MANIFEST_NAME:
            continue
        h = hashlib.sha256()
        size = 0
        with path.open("rb") as fh:
            for chunk in iter(lambda: fh.read(1024 * 1024), b""):
                h.update(chunk)
                size += len(chunk)
        rows.append({"path": rel, "size": size, "sha256": h.hexdigest()})
    return rows


def canonical_signed_message(manifest: dict[str, Any], extracted: Path) -> bytes:
    unsigned = copy.deepcopy(manifest)
    provenance = unsigned.get("provenance")
    if not isinstance(provenance, dict):
        raise ProvenanceError("manifest.provenance must be an object", kind="signature_invalid")
    provenance.pop("signature", None)
    obj = {
        "format": SIGNED_FORMAT,
        "manifest": unsigned,
        "files": _regular_package_files(extracted),
    }
    return json.dumps(obj, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def signed_message_sha256(manifest: dict[str, Any], extracted: Path) -> str:
    return hashlib.sha256(canonical_signed_message(manifest, extracted)).hexdigest()


def verify_patch_provenance(root: Path, manifest: dict[str, Any], extracted: Path | None) -> dict[str, Any]:
    required, trusted = _load_policy(root)
    provenance = manifest.get("provenance")
    if provenance is None:
        if required:
            raise ProvenanceError("local policy requires a signed PATCH, but manifest.provenance is missing", kind="signature_required")
        return {"kind": "provenance", "status": "UNSIGNED", "policy": "optional"}
    if not isinstance(provenance, dict):
        raise ProvenanceError("manifest.provenance must be an object", kind="signature_invalid")
    if extracted is None:
        raise ProvenanceError("signed PATCH requires an archive package", kind="signature_invalid")
    fmt = provenance.get("format")
    algorithm = provenance.get("algorithm")
    key_id = provenance.get("key_id")
    if fmt != SIGNED_FORMAT or algorithm != ALGORITHM:
        raise ProvenanceError(f"unsupported PATCH signature contract: format={fmt!r} algorithm={algorithm!r}", kind="signature_invalid")
    if not isinstance(key_id, str) or not _KEY_ID_RE.fullmatch(key_id):
        raise ProvenanceError("manifest.provenance.key_id is invalid", kind="signature_invalid")
    public_key = trusted.get(key_id)
    if public_key is None:
        raise ProvenanceError(f"PATCH signer is not trusted locally: {key_id}", kind="signer_untrusted")
    signature = _decode_b64_exact(provenance.get("signature"), field="manifest.provenance.signature", size=64, kind="signature_invalid")
    message = canonical_signed_message(manifest, extracted)
    if not verify_ed25519(public_key, message, signature):
        raise ProvenanceError("PATCH Ed25519 signature verification failed", kind="signature_invalid")
    return {
        "kind": "provenance",
        "status": "PASS",
        "format": SIGNED_FORMAT,
        "algorithm": ALGORITHM,
        "key_id": key_id,
        "signed_message_sha256": hashlib.sha256(message).hexdigest(),
        "package_files": len(_regular_package_files(extracted)),
        "policy": "required" if required else "optional",
    }


def enforce_legacy_unsigned_policy(root: Path, *, package_label: str) -> None:
    required, _ = _load_policy(root)
    if required:
        raise ProvenanceError(f"local policy requires a signed PATCH; unsigned legacy package is not allowed: {package_label}", kind="signature_required")
