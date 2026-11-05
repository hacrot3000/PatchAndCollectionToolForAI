#!/usr/bin/env python3
"""Native end-to-end integration using a fake mysql executable, no real DB."""
from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

def run():
    if len(sys.argv) != 2:
        raise SystemExit("usage: self_test_global_taskdeck_db_collect_native.py /path/to/taskdeck")
    exe = Path(sys.argv[1]).resolve(strict=True)
    with tempfile.TemporaryDirectory(prefix="taskdeck-collect-native-") as td:
        root = Path(td)
        home = root / "home"
        home.mkdir()
        binpath = root / "bin"
        binpath.mkdir()
        workspace = root / "project"
        workspace.mkdir()
        (workspace / ".vscode").mkdir()
        (workspace / ".vscode" / "vscode_tasks_menu.ini").write_text(
            "[shared_server]\nenabled = false\n", encoding="utf-8"
        )
        (workspace / ".python_patch_tool.json").write_text(json.dumps({
            "project": {"key": "ss_tool", "id": "2a94ed57-5587-480b-adae-7c5c720f84f1"}
        }), encoding="utf-8")
        for config_dir in (home / ".config" / "taskdeck", home / "Library" / "Application Support" / "taskdeck"):
            config_dir.mkdir(parents=True)
            (config_dir / "db_profiles.json").write_text(json.dumps({
                "version": 1,
                "profiles": [{
                    "id": "abc123", "name": "HaiTacLocal", "adapter_id": "mysql-cli",
                    "transport": "direct", "host": "127.0.0.1",
                    "port": 9000, "username": "app", "database": "",
                    "read_only": False, "options": {"charset": "utf8"}
                }]
            }), encoding="utf-8")
        args_log = root / "mysql_args.txt"
        mysql = binpath / "mysql"
        mysql.write_text(
            '#!/bin/sh\n'
            'if [ "$1" = "--version" ]; then echo "mysql fake 5.5"; exit 0; fi\n'
            'printf "%s\\n" "$@" >> "$TASKDECK_NATIVE_COLLECT_ARGS_LOG"\n'
            'if [ "$TASKDECK_NATIVE_COLLECT_LEGACY_SERVER" = "1" ]; then\n'
            '  for arg in "$@"; do\n'
            '    case "$arg" in\n'
            '      "--init-command=SET SESSION TRANSACTION READ ONLY")\n'
            '        echo "ERROR 1193 (HY000): Unknown system variable \'TRANSACTION\'" >&2; exit 1 ;;\n'
            '    esac\n'
            '  done\n'
            'fi\n'
            'sql=$(cat)\n'
            'has_database=0\n'
            'for arg in "$@"; do\n'
            '  case "$arg" in "--database=haitac") has_database=1 ;; esac\n'
            'done\n'
            'case "$sql" in *utf8mb4*) echo "ERROR 1115 Unknown character set utf8mb4" >&2; exit 1 ;; esac\n'
            'case "$sql" in\n'
            '  *taskdeck_connect*)\n'
            '    printf "%s\\n" \'<resultset><row><field name="taskdeck_connect">1</field></row></resultset>\' ;;\n'
            '  *"FROM information_schema.TABLES"*)\n'
            '    if [ "$TASKDECK_NATIVE_COLLECT_AMBIGUOUS" = "1" ]; then\n'
            '      printf "%s\\n" \'<resultset><row><field name="TABLE_SCHEMA">haitac</field><field name="TABLE_NAME">ep00_buff_columns</field></row><row><field name="TABLE_SCHEMA">archive</field><field name="TABLE_NAME">ep00_buff_columns</field></row></resultset>\'\n'
            '    else\n'
            '      printf "%s\\n" \'<resultset><row><field name="TABLE_SCHEMA">haitac</field><field name="TABLE_NAME">ep00_buff_columns</field></row></resultset>\'\n'
            '    fi ;;\n'
            '  *information_schema*)\n'
            '    printf "%s\\n" \'<resultset></resultset>\' ;;\n'
            '  *ep00_buff_columns*)\n'
            '    if [ "$has_database" != "1" ]; then\n'
            '      echo "ERROR 1046 (3D000) at line 1: No database selected" >&2; exit 1\n'
            '    fi\n'
            '    printf "%s\\n" \'<resultset><row><field name="id">1</field><field name="csv_index">0</field></row></resultset>\' ;;\n'
            '  *) echo "Unexpected query" >&2; exit 1 ;;\n'
            'esac\n', encoding="utf-8"
        )
        mysql.chmod(0o700)
        env = dict(os.environ)
        env["HOME"] = str(home)
        env["XDG_CONFIG_HOME"] = str(home / ".config")
        env["PATH"] = str(binpath) + os.pathsep + env.get("PATH", "")
        env["TASKDECK_NATIVE_COLLECT_ARGS_LOG"] = str(args_log)
        req = {
            "profile": "HaiTacLocal", "sql": "SELECT id, csv_index FROM ep00_buff_columns LIMIT 200",
            "max_rows": 200, "timeout_sec": 30,
            "source_tables": ["ep00_buff_columns"],
        }
        cmd = [str(exe), "collect-db-select", "--workspace", str(workspace)]
        completed = subprocess.run(cmd, input=json.dumps(req), text=True,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   env=env, timeout=60, check=False)
        if completed.returncode != 0:
            raise AssertionError(f"native helper failed: {completed.stderr[-2000:]}")
        result = json.loads(completed.stdout)
        assert result["engine"] == "mysql" and result["transport"] == "direct", result
        assert result["columns"] == ["id", "csv_index"], result
        assert result["rows"] == [["1", "0"]], result
        assert result["read_only_enforcement"] == "server_transaction", result
        assert result["resolved_database"] == "haitac", result
        assert result["database_source"] == "information_schema_unique", result

        # Simulated MySQL 5.5 rejects the normal session readonly setting.
        # The bridge must retry with SQL-level gates, *never* as read/write UI.
        legacy_env = dict(env)
        legacy_env["TASKDECK_NATIVE_COLLECT_LEGACY_SERVER"] = "1"
        args_before_legacy = args_log.read_text()
        legacy = subprocess.run(cmd, input=json.dumps(req), text=True,
                                capture_output=True, env=legacy_env,
                                timeout=60, check=False)
        if legacy.returncode != 0:
            raise AssertionError(f"legacy MySQL 1193 fallback failed: {legacy.stderr[-1500:]}")
        legacy_result = json.loads(legacy.stdout)
        assert legacy_result["rows"] == [["1", "0"]], legacy_result
        assert legacy_result["read_only_enforcement"] == "collect_sql_only", legacy_result
        assert legacy_result["resolved_database"] == "haitac", legacy_result
        assert legacy_result["database_source"] == "information_schema_unique", legacy_result
        additional_args = args_log.read_text()[len(args_before_legacy):]
        assert "--init-command=SET SESSION TRANSACTION READ ONLY" in additional_args
        assert "--default-character-set=utf8" in additional_args
        # The Python AST compiler emits UTF-8 hex values using utf8mb4;
        # the native profile bridge must adapt only that conversion for old DBs.
        bound = dict(req)
        bound["sql"] = "SELECT id, csv_index FROM ep00_buff_columns WHERE `name` = CONVERT(0x4142 USING utf8mb4) LIMIT 200"
        with_value = subprocess.run(cmd, input=json.dumps(bound), text=True, capture_output=True,
                                    env=env, timeout=60, check=False)
        if with_value.returncode != 0:
            raise AssertionError(f"utf8-conversion regression: {with_value.stderr[-1000:]}")
        assert json.loads(with_value.stdout)["rows"] == [["1", "0"]]
        argv = args_log.read_text()
        assert "--default-character-set=utf8" in argv, argv
        assert "--database=haitac" in argv, argv
        assert "--init-command=SET SESSION TRANSACTION READ ONLY" in argv, argv
        assert "password" not in argv.lower(), argv

        # Ambiguous source names across databases must NOT be guessed.
        ambiguous_env = dict(env)
        ambiguous_env["TASKDECK_NATIVE_COLLECT_AMBIGUOUS"] = "1"
        ambiguous = subprocess.run(cmd, input=json.dumps(req), text=True,
                                   capture_output=True, env=ambiguous_env,
                                   timeout=60, check=False)
        assert ambiguous.returncode != 0, ambiguous
        assert not ambiguous.stdout.strip(), ambiguous.stdout
        assert "ambiguous" in ambiguous.stderr.lower(), ambiguous.stderr
        assert "archive" in ambiguous.stderr and "haitac" in ambiguous.stderr, ambiguous.stderr

        # Fail closed on an invalid original project identity.
        (workspace / ".python_patch_tool.json").write_text("{}", encoding="utf-8")
        failed = subprocess.run(cmd, input=json.dumps(req), text=True, capture_output=True,
                                env=env, timeout=10, check=False)
        assert failed.returncode != 0 and "project.key and project.id" in failed.stderr, failed
        print("PASS: native TaskDeck globally saved MySQL profile, read-only adapter, utf8, workspace gate")

if __name__ == "__main__":
    run()
