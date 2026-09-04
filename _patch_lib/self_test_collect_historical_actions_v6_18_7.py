#!/usr/bin/env python3
from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parent
COLLECTOR = ROOT / "python_patch_collect_compat.py"


def write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


with tempfile.TemporaryDirectory(prefix="ptv6183-historical-collect-") as td:
    project = Path(td)
    (project / "patchs").mkdir()
    write(
        project / "src/pkg/Foo.java",
        """package pkg;
import java.util.List;
import pkg.Bar;
public class Foo {
    public int alpha(int x) {
        return beta(x) + 1;
    }
    public int beta(int x) {
        return x * 2;
    }
}
""",
    )
    write(
        project / "src/pkg/Caller.java",
        """package pkg;
import pkg.Foo;
public class Caller {
    public int run() {
        Foo f = new Foo();
        return f.alpha(3);
    }
}
""",
    )
    write(project / "src/pkg/Bar.java", "package pkg;\npublic class Bar {}\n")
    write(project / "src/.env", "API_TOKEN=do-not-auto-collect\n")
    write(
        project / "docs/decomp.c",
        """//----- (0x1000) ----------------------------------------------------
int Alpha(int x)
{
    return Beta(x) + 1;
}

//----- (0x1010) ----------------------------------------------------
int Beta(int x)
{
    return x * 2;
}

//----- (0x1020) ----------------------------------------------------
int Caller(int x)
{
    return Alpha(x);
}
""",
    )
    request = {
        "id": "historical-actions-v6183",
        "actions": [
            {"type": "ls", "path": "src"},
            {"type": "tree", "path": "src", "max_depth": 4},
            {"type": "file", "path": "src/pkg/Foo.java", "start_line": 1, "end_line": 8},
            {"type": "range", "path": "src/pkg/Foo.java", "start_line": 4, "end_line": 8},
            {"type": "head", "path": "src/pkg/Foo.java", "lines": 4},
            {"type": "tail", "path": "src/pkg/Foo.java", "lines": 4},
            {"type": "symbol", "path": "src/pkg/Foo.java", "symbol": "alpha"},
            {"type": "references", "symbol": "alpha", "paths": ["src"]},
            {"type": "callgraph", "symbol": "alpha", "paths": ["src"]},
            {"type": "dependencies", "paths": ["src"]},
            {"type": "directory", "path": "src", "include": ["*", "**/*"], "exclude": ["Bar.java"]},
            {"type": "zip", "paths": ["src/pkg/Bar.java"]},
            {"type": "research", "query": "alpha", "paths": ["src"]},
            {"type": "search_files", "query": "alpha", "paths": ["src"]},
            {"type": "content", "query": "beta", "paths": ["src"]},
            {
                "type": "symbol_graph",
                "paths": ["src"],
                "symbols": ["alpha"],
                "context_lines": 8,
                "include_references": True,
                "include_callers": True,
                "include_callees": True,
                "include_dependencies": True,
                "dependency_depth": 1,
                "max_occurrences": 1200,
                "max_callers": 300,
                "max_callees": 50,
                "max_dependency_files": 400,
            },
            {"type": "decompile", "source": "docs/decomp.c", "name": "Alpha", "neighbors_after": 1, "include_references": True},
            {"type": "ida", "source": "docs/decomp.c", "address": "1010"},
            {"type": "ghidra", "source": "docs/decomp.c", "name": "Caller", "match": "exact"},
        ],
    }
    request_zip = project / "patchs/CODE_COLLECTION_REQUEST_historical.zip"
    with zipfile.ZipFile(request_zip, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("CODE_COLLECTION_REQUEST_historical.json", json.dumps(request))

    env = dict(os.environ)
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    env["PYTHONPATH"] = str(ROOT)
    proc = subprocess.run(
        [sys.executable, "-S", str(COLLECTOR), "--project-root", str(project), "request", "patchs/CODE_COLLECTION_REQUEST_historical.zip"],
        cwd=project,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        timeout=60,
    )
    assert proc.returncode == 0, proc.stdout
    results = list((project / "artifacts/patch_tool_code_collections").glob("CODE_COLLECTION_RESULT_*.zip"))
    assert len(results) == 1, results
    with zipfile.ZipFile(results[0]) as zf:
        manifest = json.loads(zf.read("COLLECTION_MANIFEST.json"))
        assert manifest["collection_status"] == "PASS", manifest
        assert manifest["action_count"] == 19, manifest["action_count"]
        by_type = {row["type"]: zf.read(row["archive_path"]).decode("utf-8", errors="replace") for row in manifest["reports"]}
        expected = {
            "ls", "tree", "file", "range", "head", "tail", "symbol", "references",
            "callgraph", "dependencies", "directory", "zip", "research", "search_files",
            "content", "symbol_graph", "decompile", "ida", "ghidra",
        }
        assert set(by_type) == expected, set(by_type)
        assert "pkg" in by_type["ls"]
        assert "Caller.java" in by_type["tree"]
        assert "return beta(x) + 1" in by_type["file"]
        assert "Symbol extraction" in by_type["symbol"] and "alpha" in by_type["symbol"]
        assert "Coverage status: VERIFIED" in by_type["references"]
        assert "Heuristic callees" in by_type["callgraph"] and "beta" in by_type["callgraph"]
        assert "java_import" in by_type["dependencies"]
        assert ".env" not in by_type["directory"]
        assert "Coverage status: VERIFIED" in by_type["search_files"]
        assert "Coverage status: VERIFIED" in by_type["content"]
        assert "Symbol graph compatibility report" in by_type["symbol_graph"]
        assert "Historical decompile extraction" in by_type["decompile"]
        assert "0x1000" in by_type["decompile"] and "0x1010" in by_type["decompile"]
        assert "0x1010" in by_type["ida"] and "Beta" in by_type["ida"]
        assert "0x1020" in by_type["ghidra"] and "Caller" in by_type["ghidra"]
        files = {row["path"] for row in manifest["files"]}
        assert "src/pkg/Foo.java" in files and "src/pkg/Caller.java" in files and "src/pkg/Bar.java" in files
        assert "src/.env" not in files

print("PASS: v6.18.7 historical COLLECT actions, M3 aliases and decompile compatibility")
