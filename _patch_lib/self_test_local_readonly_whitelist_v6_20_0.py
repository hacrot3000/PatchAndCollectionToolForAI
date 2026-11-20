#!/usr/bin/env python3
"""Regression tests for legacy_strict local read-only file/content whitelist."""
from __future__ import annotations

from pathlib import Path
import sys
import tempfile

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
from python_patch_package_schema import (
    PatchSchemaError, _check_legacy_strict_command, _check_command,
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
    # The default source-changing PATCH profile is broader than the strict
    # command-only lane, but it must never allow direct mutating find actions.
    for argv in (
        ["find", ".", "-exec", "echo", "oops", ";"],
        ["find", ".", "-delete"],
        ["find", ".", "-execdir", "sh", "-c", "true", ";"],
        ["/usr/bin/find", ".", "-fprint", "bad-result.txt"],
    ):
        for field in ("post_patch.commands", "on_failure.commands"):
            try:
                _check_command(root, {"argv": argv, "cwd": "."}, 0, field=field)
            except PatchSchemaError as exc:
                assert exc.kind == "command_not_allowed", (argv, field, exc.kind)
            else:
                raise AssertionError(f"current PATCH profile accepted mutating find: {field} {argv}")

    for argv in forbidden:
        try:
            check(root, argv)
        except PatchSchemaError as exc:
            assert exc.kind == "command_not_allowed", (argv, exc.kind, str(exc))
        else:
            raise AssertionError(f"unsafe local strict command was allowed: {argv}")


# End-to-end command-only PATCH acceptance/rejection through the actual runner,
# not merely the schema helper. Skip the POSIX launcher on Windows.
import json
import os
import shutil
import subprocess
import zipfile

if os.name != "nt" and shutil.which("grep"):
    with tempfile.TemporaryDirectory(prefix="ptv_local_readonly_e2e_") as td:
        root = Path(td)
        shutil.copytree(HERE.parent, root / "tools")
        (root / "tools" / "run_python_patches.sh").chmod(0o755)
        (root / "patchs").mkdir()
        (root / "src.txt").write_text("alpha\nsearch phrase\nomega\n", encoding="utf-8")
        env = dict(os.environ)
        env["PYTHONDONTWRITEBYTECODE"] = "1"
        env["PTV_DISABLE_LIVE_STATUS"] = "1"

        def run_command_only(name: str, argv: list[str]) -> subprocess.CompletedProcess:
            manifest = {
                "schema_version": 1,
                "patch": {"id": name},
                "post_patch": {
                    "commands": [{"name": "readonly", "argv": argv, "cwd": ".",
                                  "timeout_seconds": 10}],
                    "run_when_no_changes": True,
                    "no_change_reason": "Search pre-existing project files with a read-only command.",
                },
            }
            archive = root / "patchs" / (name + ".zip")
            with zipfile.ZipFile(archive, "w") as z:
                z.writestr("PATCH_TOOL_MANIFEST.json", json.dumps(manifest))
            return subprocess.run(
                [str(root / "tools" / "run_python_patches.sh"),
                 "--patch", "patchs/" + name + ".zip"],
                cwd=root, capture_output=True, text=True, timeout=65, env=env,
            )

        result = run_command_only(
            "allowed-grep", ["grep", "-nF", "--", "search phrase", "src.txt"]
        )
        assert result.returncode == 0, (result.stdout, result.stderr)
        assert "search phrase" in result.stdout

        for name, argv in [
            ("forbidden-find-exec", ["find", ".", "-exec", "touch", "BAD", ";"]),
            ("forbidden-find-delete", ["find", ".", "-delete"]),
        ]:
            result = run_command_only(name, argv)
            assert result.returncode != 0, (name, result.stdout, result.stderr)
            assert "command_not_allowed" in result.stdout + result.stderr
            assert not (root / "BAD").exists()
        assert (root / "src.txt").is_file()

print("PASS: local legacy_strict safe content search and find no -exec/-delete")
