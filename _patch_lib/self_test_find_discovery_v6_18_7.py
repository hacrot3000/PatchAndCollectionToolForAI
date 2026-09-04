#!/usr/bin/env python3
from pathlib import Path
import tempfile

from python_patch_collect_compat import _directory_action, _find_action_result
from python_patch_collect_schema import validate_request_data


def limits(**overrides):
    req = validate_request_data({
        "id": "find-glob-coverage",
        "actions": [{"type": "find", "paths": ["."], "patterns": ["*.java"]}],
        "limits": overrides,
    })
    return req["limits"]


with tempfile.TemporaryDirectory(prefix="ptv-find-coverage-") as tmp:
    root = Path(tmp).resolve()
    module = root / "projects/m3-server/trunk/jdqs_server"
    # Put noise lexically before src so the historical max_files traversal cap
    # would exhaust before reaching the target subtree.
    for i in range(8):
        p = module / f"a_noise_{i:02d}" / f"Noise{i}.txt"
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text("noise\n", encoding="utf-8")

    handler = module / "src/main/java/com/xkhy/jdqs/handler/mine/MineInfoCSHandler.java"
    handler.parent.mkdir(parents=True, exist_ok=True)
    handler.write_text("class MineInfoCSHandler {}\n", encoding="utf-8")
    service = module / "src/main/java/com/xkhy/jdqs/service/game/mine/MineService.java"
    service.parent.mkdir(parents=True, exist_ok=True)
    service.write_text("class MineService {}\n", encoding="utf-8")
    nested = handler.parent / "nested/DeepMine.java"
    nested.parent.mkdir(parents=True, exist_ok=True)
    nested.write_text("class DeepMine {}\n", encoding="utf-8")
    proto = module / "proto/proto/CmdMineMsg.proto"
    proto.parent.mkdir(parents=True, exist_ok=True)
    proto.write_text("message CmdMineChallengeCSReqMsg {}\n", encoding="utf-8")

    l = limits(max_files=2, max_search_files=100)

    # Regression 1: path-bearing find patterns are relative to each requested
    # scope, not only to the project root.
    action = {
        "type": "find",
        "paths": ["projects/m3-server/trunk/jdqs_server"],
        "patterns": [
            "src/main/java/com/xkhy/jdqs/handler/mine/*.java",
            "src/main/java/com/xkhy/jdqs/service/game/mine/*.java",
            "proto/proto/CmdMine*.proto",
        ],
        "collect": False,
        "max_results": 20,
    }
    result = _find_action_result(root, action, l)
    rels = {rel for rel, _ in result["matches"]}
    assert "projects/m3-server/trunk/jdqs_server/src/main/java/com/xkhy/jdqs/handler/mine/MineInfoCSHandler.java" in rels, rels
    assert "projects/m3-server/trunk/jdqs_server/src/main/java/com/xkhy/jdqs/service/game/mine/MineService.java" in rels, rels
    assert "projects/m3-server/trunk/jdqs_server/proto/proto/CmdMineMsg.proto" in rels, rels
    assert result["coverage_status"] == "VERIFIED", result
    assert result["files_considered"] > l["max_files"], result
    assert "scope-relative path" in result["report"]

    # Regression 2: directory globstar means zero-or-more directories, so a
    # direct child must match **/*.java as well as nested children.
    d_action = {
        "type": "directory",
        "path": "projects/m3-server/trunk/jdqs_server/src/main/java/com/xkhy/jdqs/handler/mine",
        "include": ["**/*.java"],
        "exclude": [],
        "max_results": 20,
    }
    d_report, d_matches = _directory_action(root, d_action, l)
    d_rels = {rel for rel, _ in d_matches}
    assert handler.relative_to(root).as_posix() in d_rels, d_rels
    assert nested.relative_to(root).as_posix() in d_rels, d_rels
    assert "**/ may match zero or more directories" in d_report

    # Regression 3: preserve historical basename matching semantics.
    basename_result = _find_action_result(root, {
        "type": "find",
        "paths": ["projects/m3-server/trunk/jdqs_server/src/main/java/com/xkhy/jdqs"],
        "patterns": ["*Mine*.java"],
        "collect": False,
        "max_results": 20,
    }, l)
    assert any(rel.endswith("MineInfoCSHandler.java") for rel, _ in basename_result["matches"])

    # Regression 4: a genuinely truncated discovery scan must never claim
    # verified coverage.
    tiny = limits(max_files=2, max_search_files=2)
    partial = _find_action_result(root, action, tiny)
    assert partial["incomplete"] is True, partial
    assert partial["coverage_status"] == "PARTIAL", partial
    assert "INCOMPLETE" in partial["report"]

print("PASS: v6.18.7 scope-relative find glob, globstar and discovery-budget regression")
