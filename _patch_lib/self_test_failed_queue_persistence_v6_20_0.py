#!/usr/bin/env python3
from pathlib import Path
import json,tempfile
import python_patch_queue_dispatcher as m
assert m.VERSION=='6.20.0'
with tempfile.TemporaryDirectory(prefix='ptv-persist-failed-') as td:
 root=Path(td); (root/'patchs').mkdir(parents=True); hist=root/'artifacts'/'patch_tool'/'history'; hist.mkdir(parents=True)
 fc='CODE_COLLECTION_REQUEST_failed_v2.zip'; nc='CODE_COLLECTION_REQUEST_new_v3.zip'; fp='patch_failed.zip'
 for name,data in [(fc,b'collect-v2'),(nc,b'collect-v3'),(fp,b'patch-v1')]: (root/'patchs'/name).write_bytes(data)
 psha=m._queue_item_sha256(root,m.QueueItem(fp,'PATCH'))
 r1={'format':'python-patch-tool-last-run','run_id':'r1','status':'FAIL','selected':[fc,fp],'results':[{'name':fc,'kind':'COLLECT','status':'FAIL','rc':2},{'name':fp,'kind':'PATCH','status':'FAIL','rc':2,'patch_result':{'patch_sha256':psha}}]}
 (hist/'r1.json').write_text(json.dumps(r1)); r2={'format':'python-patch-tool-last-run','run_id':'r2','status':'PASS','selected':[nc],'results':[{'name':nc,'kind':'COLLECT','status':'PASS','rc':0,'request_sha256':'a'*64}]}
 (hist/'r2.json').write_text(json.dumps(r2)); (root/'artifacts'/'patch_tool'/'LAST_RUN.json').write_text(json.dumps(r2))
 items=[m.QueueItem(nc,'COLLECT'),m.QueueItem(fc,'COLLECT'),m.QueueItem(fp,'PATCH')]
 assert m._last_failed_queue_names(root,items,r2)=={fc,fp}
 regp=root/'artifacts'/'patch_tool'/'UNRESOLVED_FAILURES.json'; reg=json.loads(regp.read_text()); unresolved=[e for e in reg['entries'] if e.get('resolved') is not True]
 assert {e['row']['name'] for e in unresolved}=={fc,fp}; crow=next(e['row'] for e in unresolved if e['row']['name']==fc); assert len(crow.get('request_sha256',''))==64
 m._update_unresolved_registry(root,{'run_id':'r3','status':'PASS','results':[{'name':'other.zip','kind':'COLLECT','status':'PASS','request_sha256':'b'*64}]})
 assert m._last_failed_queue_names(root,items,None)=={fc,fp}
 extra='CODE_COLLECTION_REQUEST_extra_fail.zip'; (root/'patchs'/extra).write_bytes(b'extra'); ei=m.QueueItem(extra,'COLLECT'); esha=m._queue_item_sha256(root,ei); items.append(ei)
 m._update_unresolved_registry(root,{'run_id':'r4','status':'FAIL','results':[{'name':extra,'kind':'COLLECT','status':'FAIL','rc':2,'request_sha256':esha}]}); assert extra in m._last_failed_queue_names(root,items,None)
 # unrelated/same-name wrong bytes PASS does not resolve
 m._update_unresolved_registry(root,{'run_id':'r5','status':'PASS','results':[{'name':extra,'kind':'COLLECT','status':'PASS','request_sha256':'c'*64}]}); assert extra in m._last_failed_queue_names(root,items,None)
 # normal selector delete resolves exact persistent item
 mutable=[m.QueueItem(fc,'COLLECT')]; _sel,_prio,deleted,failures=m._delete_indexes(root,mutable,set(),{0},{})
 assert deleted==[fc] and not failures; reg=json.loads(regp.read_text()); assert all(e.get('resolved') is True for e in reg['entries'] if (e.get('row') or {}).get('name')==fc)
print('PASS: v6.20.0 persistent failed PATCH/COLLECT grouping survives unrelated runs and migrates v6.19.4 history')
