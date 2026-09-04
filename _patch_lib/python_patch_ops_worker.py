#!/usr/bin/env python3
from __future__ import annotations
import argparse, json, traceback
from pathlib import Path
from python_patch_utils import PatchFailure, diagnose_ops, run_ops

def write_result(path: Path, data: dict) -> None:
    tmp=path.with_name(path.name+".tmp")
    tmp.write_text(json.dumps(data,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
    tmp.replace(path)

def main() -> int:
    ap=argparse.ArgumentParser()
    ap.add_argument("--project-root",required=True)
    ap.add_argument("--ops-json",required=True)
    ap.add_argument("--patch-name",default="patch")
    ap.add_argument("--mode",choices=["run","diagnose"],default="run")
    ap.add_argument("--result",required=True)
    ns=ap.parse_args()
    result=Path(ns.result)
    try:
        data=json.loads(Path(ns.ops_json).read_text(encoding="utf-8"))
        if ns.mode == "diagnose":
            report=diagnose_ops(Path(ns.project_root),data)
            write_result(result,report if isinstance(report,dict) else {"status":"FAIL","kind":"tool_error","message":"invalid diagnose result"})
            return 0 if isinstance(report,dict) and report.get("status")=="PASS" else 2
        state=run_ops(Path(ns.project_root),data,patch_name=ns.patch_name)
        write_result(result,{"status":"PASS","stats":{"patched":state.stats.patched,"created":state.stats.created,"unchanged":state.stats.unchanged}})
        return 0
    except PatchFailure as exc:
        write_result(result,{"status":"FAIL","kind":"patch_failure","message":str(exc),"path":exc.rel_path,"expected":exc.expected,"anchor":exc.anchor,"operation":exc.op_id,"strategy":exc.strategy})
        return 2
    except Exception as exc:
        write_result(result,{"status":"FAIL","kind":"internal_error","message":f"{type(exc).__name__}: {exc}","traceback":traceback.format_exc(limit=8)})
        return 2

if __name__=="__main__": raise SystemExit(main())
