#!/usr/bin/env python3
from pathlib import Path
import json
HERE=Path(__file__).resolve().parent; DOC=HERE/'docs'; ROOT=HERE.parent.parent
for name in ['NO_SILENT_REMOVAL_POLICY.md','CAPABILITY_LEDGER.md','HISTORICAL_FEATURE_BASELINE_V5_15.md']:
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
for name in ['NO_SILENT_REMOVAL_POLICY.md','CAPABILITY_LEDGER.md','HISTORICAL_FEATURE_BASELINE_V5_15.md']:
    assert name in contract,name
schema=json.loads((DOC/'PATCH_PACKAGE_SCHEMA.json').read_text())
post=schema['manifest']['fields']['post_patch']; assert {'no_change_reason','safety_profile'} <= set(post['allowed_fields'])
master=(HERE/'self_test_python_patch_tool_v6_18_2.py').read_text(); assert 'self_test_historical_compatibility_v6_18_2.py' in master and 'self_test_capability_ledger_v6_18_2.py' in master
print('PASS: v6.18.2 cumulative capability ledger and no-silent-removal governance are packaged and release-gated')
