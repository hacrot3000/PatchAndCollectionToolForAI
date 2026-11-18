#!/usr/bin/env python3
"""Regression tests for legacy_strict local read-only file/content whitelist."""
from __future__ import annotations

from pathlib import Path
import sys
import tempfile

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
from python_patch_package_schema import (
    PatchSchemaError, _check_legacy_strict_command,
)


def check(root: Path, argv: list[str]) -> None:
    _check_legacy_strict_command(root, {"argv": argv, "cwd": "."}, 0)


with tempfile.TemporaryDirectory(prefix="ptv_local_readonly_whitelist_") as td:
    root = Path(td)
    (root / "src").mkdir()
    (root / "src" / "file with spaces.txt").write_text(
        "first line\nsome text with spaces\nTODO\n", encoding="utf-8"
    )
    (root / "src" / "main.go").write_text("func main() {}\n", encoding="utf-8")
    allowed = [
        ["pwd"],
        ["ls", "src"],
        ["find", ".", "-name", "*.go", "-print"],
        ["find", ".", "-type", "f"],
        ["grep", "-nriF", "--include=*.go", "--", "func main", "."],
        ["grep", "-n", "--", "-error", "src/file with spaces.txt"],
        ["grep", "-nF", "$(id); echo never execute", "src/main.go"],
        ["rg", "-n", "-i", "-g", "*.go", "--", "func main", "src"],
        ["rg", "-n", "--hidden", "--", "TODO", "."],
        ["cat", "src/file with spaces.txt"],
        ["head", "-n", "20", "src/main.go"],
        ["tail", "-n", "20", "src/main.go"],
        ["wc", "-l", "src/main.go"],
        ["nl", "-ba", "src/main.go"],
        ["sed", "-n", "1,2p", "src/main.go"],
    ]
    for argv in allowed:
        check(root, argv)

    forbidden = [
        ["find", ".", "-exec", "echo", "BAD", ";"],
        ["find", ".", "-execdir", "bash", "BAD", ";"],
        ["find", ".", "-okdir", "bash", "BAD", ";"],
        ["find", ".", "-delete"],
        ["find", ".", "-fprint0", "results.txt"],
        ["find", ".", "-fprintf", "results.txt", "%p"],
        ["grep", "-f", "src/main.go", "src/main.go"],
        ["/bin/grep", "-n", "TODO", "src/main.go"],
        ["./grep", "TODO", "src/main.go"],
        ["grep", "--exclude-dir=.", "TODO", "src"],
        ["grep", "--include=../../etc", "TODO", "."],
        ["rg", "--pre", "src/main.go", "TODO", "."],
        ["rg", "-e", "TODO", "."],
        ["rg", "-g", "-unsafe", "TODO", "."],
        ["cat", "-"],
        ["cat", "../../secrets.txt"],
        ["head", "-c", "100", "src/main.go"],
        ["head", "-n", "100000", "src/main.go"],
        ["tail", "-f", "src/main.go"],
        ["wc", "--files0-from=src/main.go"],
        ["nl", "-ba", "-"],
        ["sed", "-i", "1p", "src/main.go"],
        ["sed", "-n", "1;d", "src/main.go"],
        ["sed", "-n", "20,10p", "src/main.go"],
    ]
    for argv in forbidden:
        try:
            check(root, argv)
        except PatchSchemaError as exc:
            assert exc.kind == "command_not_allowed", (argv, exc.kind, str(exc))
        else:
            raise AssertionError(f"unsafe local strict command was allowed: {argv}")

print("PASS: local legacy_strict safe content search and find no -exec/-delete")
