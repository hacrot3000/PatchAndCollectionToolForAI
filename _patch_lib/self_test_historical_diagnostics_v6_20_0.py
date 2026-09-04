#!/usr/bin/env python3
from __future__ import annotations
import json, tempfile, zipfile
from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
import python_patch_diagnostics_compat as d
import python_patch_queue_dispatcher as q
assert d.VERSION==q.VERSION=='6.20.2'
log='''noise\nAuthorization: Bearer SECRET_BEARER\napi_key=SECRET_KEY\nsrc/main.c:12:5: error: expected ; before }\nsrc/main.c:12:5: error: expected ; before }\nTraceback (most recent call last):\n  File "tool.py", line 7, in <module>\nValueError: bad value 123\n'''
red,counts=d.redact_text(log)
assert 'SECRET_BEARER' not in red and 'SECRET_KEY' not in red and sum(counts.values())>=2
items=d.normalize_diagnostics(red); assert len(items)>=3,items
clusters=d.cluster_diagnostics(items); assert clusters['unique_clusters']>=2 and clusters['suppressed_cascade_count']>=1,clusters
smart=d.smart_filter(red); assert 'src/main.c' in smart and 'ValueError' in smart
with tempfile.TemporaryDirectory(prefix='ptv_diag_') as td:
    root=Path(td); (root/'patchs').mkdir(); (root/'src').mkdir(); (root/'src/main.c').write_text('int x;\n')
    pr={'diagnosis':{'kind':'validation_profile_failed','message':'compile failed','affected_paths':['src/main.c']},'partial_modification':{'detected':True,'changed_paths':['src/main.c']}}
    out=q._create_fail_handoff(root,q.QueueItem('bad.zip','PATCH'),2,log,pr,None)
    assert out and out.is_file()
    with zipfile.ZipFile(out) as zf:
        names=set(zf.namelist())
        required={
          'compat_diagnostics/AI_SUMMARY.md','compat_diagnostics/REDACTED_DETAIL.log','compat_diagnostics/SMART_LOG.txt',
          'compat_diagnostics/DIAGNOSTICS.json','compat_diagnostics/ROOT_CAUSE_CLUSTERS.json',
          'compat_diagnostics/ENVIRONMENT_FINGERPRINT.json','compat_diagnostics/DIAGNOSTIC_QUALITY.json',
          'compat_diagnostics/FAILURE_DELTA.json','console.log','FAIL_SUMMARY.json'
        }
        assert required<=names,required-names
        exact=zf.read('console.log').decode(); safe=zf.read('compat_diagnostics/REDACTED_DETAIL.log').decode()
        assert 'SECRET_BEARER' in exact and 'SECRET_KEY' in exact
        assert 'SECRET_BEARER' not in safe and 'SECRET_KEY' not in safe
        quality=json.loads(zf.read('compat_diagnostics/DIAGNOSTIC_QUALITY.json'))
        assert quality['redaction_total']>=2 and quality['normalized_diagnostics']>=3
        summary=json.loads(zf.read('FAIL_SUMMARY.json'))
        assert summary['compat_diagnostics']['status']=='AVAILABLE' and summary['compat_diagnostics']['exact_v6_evidence_preserved'] is True
print('PASS: v6.20.2 historical diagnostics #18-28 additive compatibility evidence')
