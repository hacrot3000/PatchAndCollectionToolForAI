#!/usr/bin/env python3
from __future__ import annotations
import json
from pathlib import Path
import subprocess
import tempfile

HERE=Path(__file__).resolve().parent
import sys
sys.path.insert(0,str(HERE))
import python_patch_collect_compat as compat
import python_patch_collect_regex_worker as worker

limits={"max_report_bytes":1024*1024,"max_search_files":1000}
hard=max(limits["max_report_bytes"],1024*1024)*2

# Worker must bound growth from ALL major payload containers, preserve total
# match evidence, and retain as many concrete match details as fit.
context="x"*6000
details=[{"path":f"src/f{i}.py","line":i+1,"context":[[i+1,context,True]]} for i in range(220)]
payload={
    "report":"R"*(5*1024*1024),
    "incomplete":False,
    "inconsistency":False,
    "must_find_failed":False,
    "matches":98765,
    "match_details":details,
    "coverage":{
        "directories_visited":123,
        "files_considered":456,
        "candidate_filenames":[("candidate_"+str(i)+"_"+("z"*160)) for i in range(60000)],
        "modules":["module/"+("m"*300)]*1000,
        "skipped_dirs":[{"path":"skip/"+("s"*300),"reason":"excluded"}]*1000,
        "skipped_files":[{"path":"file/"+("f"*300),"reason":"oversize"}]*1000,
    },
    "coverage_status":"VERIFIED",
    "execution_status":"COMPLETED",
    "reasons":[],
}
text=worker._bounded_payload_json(payload,limits)
raw=text.encode("utf-8")
assert len(raw)<=hard,(len(raw),hard)
data=json.loads(text)
assert data["incomplete"] is True,data
assert data["coverage_status"]=="PARTIAL",data
assert data["execution_status"]=="PARTIAL",data
assert data["matches"]==98765,data
assert data["worker_result_truncated"] is True,data
assert data["worker_result_original_bytes"]>hard,data
assert data["match_details_total_before_worker_cap"]==len(details),data
assert data["match_details_preserved_after_worker_cap"]>0,data
assert any("safety cap" in x for x in data["reasons"]),data
assert data["coverage"].get("candidate_filenames_total_before_worker_cap")==60000,data

# Parent safety net must NEVER convert an oversized worker result to rc=2. It
# returns a generic PARTIAL result so subsequent COLLECT actions may continue
# and preserve already/later collected files.
with tempfile.TemporaryDirectory(prefix="ptv-regex-parent-cap-") as td:
    root=Path(td)
    (root/"a.txt").write_text("needle\n",encoding="utf-8")
    action={"type":"search","query":"needle","regex":True,"paths":["."],"backend":"python","max_matches":20}
    real_run=compat.subprocess.run
    def fake_run(cmd,**kwargs):
        result_path=Path(cmd[cmd.index("--result")+1])
        result_path.write_bytes(b"x"*(hard+8192))
        return subprocess.CompletedProcess(cmd,0,stdout="")
    compat.subprocess.run=fake_run
    try:
        result=compat._search_action(root,action,limits)
    finally:
        compat.subprocess.run=real_run
    assert result["incomplete"] is True,result
    assert result["coverage_status"]=="PARTIAL",result
    assert result["worker_result_truncated"] is True,result
    assert result["worker_result_original_bytes"]>hard,result
    assert any("safety cap" in x for x in result.get("reasons",[])),result

source=(HERE/"python_patch_collect_compat.py").read_text(encoding="utf-8")
assert 'raise ValueError(f"regex search worker result exceeded safety cap' not in source
print("PASS: regex worker and parent preserve bounded PARTIAL evidence across safety-cap overflow")
