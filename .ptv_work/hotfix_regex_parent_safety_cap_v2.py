from pathlib import Path

ROOT=Path('.').resolve()
lib=ROOT/'_patch_lib'

# 1) Worker: compact the whole search payload, not only match_details/report.
p=lib/'python_patch_collect_regex_worker.py'
s=p.read_text(encoding='utf-8')
start=s.index('def _bounded_payload_json(')
end=s.index('\n\ndef main(', start)
replacement=r'''def _bounded_payload_json(payload: dict, limits: dict) -> str:
    """Keep worker IPC bounded while preserving useful PARTIAL evidence.

    Search payload growth can come from the rendered report, match details,
    coverage diagnostics, candidate filenames, skip lists, or combinations of
    them.  Build a compact contract when the raw payload crosses the IPC cap
    instead of trimming only one field and leaving a different large field for
    the parent to reject.
    """
    hard = max(int(limits.get("max_report_bytes", 0) or 0), 1024 * 1024) * 2

    def dump(value: dict) -> str:
        return json.dumps(value, ensure_ascii=False)

    def utf8_prefix(value: object, max_bytes: int) -> str:
        raw = str(value or "").encode("utf-8", errors="replace")
        if len(raw) <= max_bytes:
            return raw.decode("utf-8", errors="replace")
        return raw[:max_bytes].decode("utf-8", errors="ignore")

    def compact_rows(value, *, limit: int):
        if not isinstance(value, list):
            return []
        out=[]
        for row in value[:limit]:
            if isinstance(row, dict):
                clean={}
                for key in ("path", "reason", "name", "kind"):
                    if key in row:
                        clean[key]=utf8_prefix(row.get(key), 512)
                out.append(clean or {"summary": utf8_prefix(row, 512)})
            elif isinstance(row, (list, tuple)):
                out.append([utf8_prefix(x, 512) for x in row[:4]])
            else:
                out.append(utf8_prefix(row, 512))
        return out

    def compact_coverage(value) -> dict:
        if not isinstance(value, dict):
            return {"worker_result_truncated": True}
        out={"worker_result_truncated": True}
        for key in (
            "directories_visited", "files_considered", "limit_reached",
            "time_limit_reached", "skipped_dirs_count", "skipped_files_count",
        ):
            if key in value:
                out[key]=value[key]
        ext=value.get("extension_counts")
        if isinstance(ext, dict):
            out["extension_counts"]={str(k): v for k,v in list(ext.items())[:50]}
        modules=value.get("modules")
        if isinstance(modules, list):
            out["modules"]=[utf8_prefix(x, 512) for x in modules[:80]]
        candidates=value.get("candidate_filenames")
        if isinstance(candidates, list):
            out["candidate_filenames"]=[utf8_prefix(x, 512) for x in candidates[:100]]
            out["candidate_filenames_total_before_worker_cap"]=len(candidates)
        out["errors"]=compact_rows(value.get("errors"), limit=8)
        out["skipped_dirs"]=compact_rows(value.get("skipped_dirs"), limit=40)
        out["skipped_files"]=compact_rows(value.get("skipped_files"), limit=40)
        return out

    raw = dump(payload)
    original_bytes = len(raw.encode("utf-8"))
    if original_bytes <= hard:
        return raw

    details=list(payload.get("match_details") or [])
    cap_reason=(
        f"regex search worker result exceeded safety cap ({original_bytes} bytes); "
        "bounded partial evidence preserved"
    )
    reasons=[]
    for value in list(payload.get("reasons") or []) + [cap_reason]:
        text=utf8_prefix(value, 1200)
        if text and text not in reasons:
            reasons.append(text)
        if len(reasons) >= 32:
            break

    report_budget=min(512 * 1024, max(4096, hard // 4))
    report=utf8_prefix(payload.get("report"), report_budget)
    if report:
        report += "\n\n[PTV: report truncated/bounded to keep regex worker IPC within safety cap]\n"

    out={
        "report": report,
        "incomplete": True,
        "inconsistency": bool(payload.get("inconsistency")),
        "must_find_failed": bool(payload.get("must_find_failed")),
        "matches": payload.get("matches", len(details)),
        "match_details": [],
        "coverage": compact_coverage(payload.get("coverage")),
        "coverage_status": "INCONSISTENT" if payload.get("coverage_status") == "INCONSISTENT" else "PARTIAL",
        "execution_status": "INCONSISTENT" if payload.get("execution_status") == "INCONSISTENT" else "PARTIAL",
        "reasons": reasons,
        "worker_result_truncated": True,
        "worker_result_original_bytes": original_bytes,
        "match_details_total_before_worker_cap": len(details),
    }

    # Preserve the largest safe prefix of detailed matches.  Reserve headroom
    # for the bookkeeping fields added after the search.
    lo=0; hi=min(len(details), 512); best=0
    target=max(0, hard - 4096)
    while lo <= hi:
        mid=(lo + hi) // 2
        out["match_details"]=details[:mid]
        size=len(dump(out).encode("utf-8"))
        if size <= target:
            best=mid; lo=mid + 1
        else:
            hi=mid - 1
    out["match_details"]=details[:best]
    out["match_details_preserved_after_worker_cap"]=best
    raw=dump(out)

    # Emergency minimum is still a useful PARTIAL contract: total match count,
    # reason, and coverage counters survive even if one context row is huge.
    if len(raw.encode("utf-8")) > hard:
        out["match_details"]=[]
        out["match_details_preserved_after_worker_cap"]=0
        out["report"]=utf8_prefix(payload.get("report"), 4096) + "\n[PTV: report heavily truncated by worker safety cap]\n"
        out["coverage"]={"worker_result_truncated": True}
        raw=dump(out)
    if len(raw.encode("utf-8")) > hard:
        raise ValueError(
            f"cannot bound regex worker partial payload below safety cap "
            f"({len(raw.encode('utf-8'))} > {hard})"
        )
    return raw
'''
s=s[:start]+replacement+s[end:]
p.write_text(s, encoding='utf-8')

# 2) Parent: an unexpected oversized worker file is fail-partial, never rc=2.
p=lib/'python_patch_collect_compat.py'
s=p.read_text(encoding='utf-8')
old='''        size=result_path.stat().st_size; hard=max(int(limits.get('max_report_bytes',0)),1024*1024)*2
        if size>hard: raise ValueError(f"regex search worker result exceeded safety cap ({size} bytes)")
        data=json.loads(result_path.read_text(encoding='utf-8'))
'''
new='''        size=result_path.stat().st_size; hard=max(int(limits.get('max_report_bytes',0)),1024*1024)*2
        if size>hard:
            reason=(
                f"regex search worker result exceeded safety cap ({size} bytes); "
                "oversized worker payload was not trusted, bounded partial coverage returned"
            )
            data=_generic_timeout_partial_payload(root,action,limits,reason)
            data["worker_result_truncated"]=True
            data["worker_result_original_bytes"]=size
            return data
        data=json.loads(result_path.read_text(encoding='utf-8'))
'''
if old not in s:
    raise SystemExit('parent safety-cap block not found')
s=s.replace(old,new,1)
p.write_text(s, encoding='utf-8')

# 3) Permanent regression.
test=lib/'self_test_regex_worker_safety_cap_v6_20_2.py'
test.write_text(r'''#!/usr/bin/env python3
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
''',encoding='utf-8')

# 4) Register regression in the master suite.
p=lib/'self_test_python_patch_tool_v6_20_0.py'
s=p.read_text(encoding='utf-8')
needle=" 'self_test_search_partial_timeout_v6_20_0.py',\n"
entry=" 'self_test_regex_worker_safety_cap_v6_20_2.py',\n"
if entry not in s:
    if needle not in s:
        raise SystemExit('master search test insertion point not found')
    s=s.replace(needle,needle+entry,1)
p.write_text(s,encoding='utf-8')
