#!/usr/bin/env python3
from __future__ import annotations
import json
from pathlib import Path
HERE=Path(__file__).resolve().parent; docs=HERE/'docs'
historical=json.loads((docs/'HISTORICAL_FEATURE_STATUS_V5_15.json').read_text())
current=json.loads((docs/'CURRENT_CAPABILITY_DISPOSITION.json').read_text())
assert current['current_version']=='6.20.1'
complete=set(historical['complete_ids']); rows=current['entries']; ids=[int(x['id']) for x in rows]
assert len(ids)==len(set(ids))==95 and set(ids)==complete,(len(ids),sorted(complete-set(ids)),sorted(set(ids)-complete))
allowed={'PRESERVED','COMPATIBILITY_RESTORED','SUPERSEDED','REMOVED_BY_REQUIREMENT'}
surface_only={'self_test_upgrade_continuity_v6_20_0.py','self_test_capability_ledger_v6_20_0.py','self_test_capability_disposition_v6_20_0.py'}
for row in rows:
    assert row['disposition'] in allowed,(row['id'],row['disposition'])
    assert row.get('note'),row['id']
    evidence=row.get('evidence') or []
    assert evidence,row['id']
    test_refs=[x for x in evidence if isinstance(x,str) and x.startswith('self_test_') and x.endswith('.py')]
    assert test_refs,(row['id'],'missing behavioral/release test evidence',evidence)
    assert all((HERE/x).is_file() for x in test_refs),(row['id'],test_refs)
    # A historical COMPLETE capability cannot be protected only by a string/surface continuity gate.
    assert any(x not in surface_only for x in test_refs),(row['id'],'surface-only evidence is insufficient',test_refs)
# COMPLETE capabilities may never disappear into an unexplained/unguaranteed bucket.
assert not any(x['disposition']=='NOT_CURRENTLY_GUARANTEED' for x in rows)
contract=(docs/'AI_USAGE_CONTRACT.md').read_text(); policy=(docs/'NO_SILENT_REMOVAL_POLICY.md').read_text(); ledger=(docs/'CAPABILITY_LEDGER.md').read_text()
for text in (contract,policy): assert 'CURRENT_CAPABILITY_DISPOSITION.json' in text
assert '95 historical COMPLETE' in ledger or '95/95' in ledger
print('PASS: v6.20.1 95/95 historical COMPLETE capability disposition coverage gate')
