#!/usr/bin/env python3
from pathlib import Path
import json
HERE=Path(__file__).resolve().parent; DOC=HERE/'docs'; ROOT=HERE.parent.parent
for name in ['NO_SILENT_REMOVAL_POLICY.md','CAPABILITY_LEDGER.md','HISTORICAL_FEATURE_BASELINE_V5_15.md','HISTORICAL_FEATURE_STATUS_V5_15.json','LAYOUT_AND_MIGRATION.md','OUTPUT_FILES_GUIDE.md']:
    assert (DOC/name).is_file(),name
base=(DOC/'HISTORICAL_FEATURE_BASELINE_V5_15.md').read_text(encoding='utf-8')
for i in range(1,108): assert f'| {i} |' in base,f'missing historical capability {i}'
ledger=(DOC/'CAPABILITY_LEDGER.md').read_text(encoding='utf-8')
for token in ['PRESERVED','COMPATIBILITY_RESTORED','SUPERSEDED','REMOVED_BY_REQUIREMENT','NOT_CURRENTLY_GUARANTEED','#13','#29–46','Repeated explicit `--patch`','Command-only package','Legacy v4 archive']:
    assert token in ledger,token
policy=(DOC/'NO_SILENT_REMOVAL_POLICY.md').read_text(encoding='utf-8')
for token in ['must not be deleted','behavioral regression','Do not remove code','Zero matches is a search result, not proof of absence']:
    assert token.lower() in policy.lower(),token
contract=(DOC/'AI_USAGE_CONTRACT.md').read_text(encoding='utf-8')
for name in ['NO_SILENT_REMOVAL_POLICY.md','CAPABILITY_LEDGER.md','HISTORICAL_FEATURE_BASELINE_V5_15.md','HISTORICAL_FEATURE_STATUS_V5_15.json']:
    assert name in contract,name
schema=json.loads((DOC/'PATCH_PACKAGE_SCHEMA.json').read_text())
post=schema['manifest']['fields']['post_patch']; assert {'no_change_reason','safety_profile'} <= set(post['allowed_fields'])
master=(HERE/'self_test_python_patch_tool_v6_18_6.py').read_text(); assert 'self_test_historical_compatibility_v6_18_6.py' in master and 'self_test_capability_ledger_v6_18_6.py' in master

collect=json.loads((DOC/'COLLECT_ACTION_SCHEMA.json').read_text())
protected={'pack','zip','overview','ls','tree','find','search','search_files','content','research','file','range','head','tail','symbol','references','callgraph','dependencies','directory','symbol_graph','decompile','ida','ghidra','git'}
assert protected <= set(collect['actions']), protected-set(collect['actions'])
assert 'max_decompile_file_bytes' in collect['limits']['allowed_fields']
for token in ['COMPATIBILITY_RESTORED v6.18.3','search_files','content','symbol_graph','decompile']:
    assert token in ledger,token
for token in ['COLLECT capability continuity','search_files','symbol_graph']:
    assert token.lower() in policy.lower(),token
assert 'self_test_collect_historical_actions_v6_18_6.py' in master

status=json.loads((DOC/'HISTORICAL_FEATURE_STATUS_V5_15.json').read_text())
complete=set(status['complete_ids']); partial={int(k) for k in status['partial']}; not_started=set(status['not_started_ids'])
assert len(complete)==95,(len(complete),sorted(complete))
assert complete|partial|not_started==set(range(1,108))
assert not (complete&partial or complete&not_started or partial&not_started)
assert partial=={47,48,49,50,62,64},partial
assert not_started=={56,60,61,63,65,66},not_started
for token in ['Optional controlled installer','user_not_selected','Selection-aware automatic identity adoption']:
    assert token in ledger,token
assert 'self_test_portable_installer_v6_18_6.py' in master

print('PASS: v6.18.6 cumulative capability ledger and no-silent-removal governance are packaged and release-gated')
