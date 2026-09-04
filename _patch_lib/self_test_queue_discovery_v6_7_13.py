#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, io, json, sys, tarfile, tempfile, zipfile
from pathlib import Path
HERE=Path(__file__).resolve(); MOD=HERE.parent/'python_patch_queue_dispatcher.py'
spec=importlib.util.spec_from_file_location('ptv_queue_v678',MOD); m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; spec.loader.exec_module(m)
assert m.VERSION=='6.7.13'
bidi_display=m._safe_display('safe\u202eevil')
assert '\u202e' not in bidi_display, repr(bidi_display)

with tempfile.TemporaryDirectory() as td:
    root=Path(td); p=root/'patchs'; p.mkdir()

    # Filename-independent v5 packages, including the user's real naming shapes.
    for name in ['NFC_implement_201_example.zip','OTA_FIX_example.zip','patch_example.zip','other_valid_name.zip']:
        with zipfile.ZipFile(p/name,'w') as z:
            z.writestr('PATCH_TOOL_MANIFEST.json','{}')

    # Root PATCH manifest has precedence even if the package carries a
    # collection request as a nested resource.
    with zipfile.ZipFile(p/'patch_with_collect_resource.zip','w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json','{}')
        z.writestr('resources/CODE_COLLECTION_REQUEST_helper.json',json.dumps({'id':'nested','actions':[{'type':'overview'}]}))

    # Legacy v4 shapes remain recognized.
    with zipfile.ZipFile(p/'legacy_bundle.zip','w') as z:
        z.writestr('nested/patch_old.py','print("ok")')
    (p/'patch_standalone.py').write_text('print("ok")',encoding='utf-8')
    (p/'marker_standalone.py').write_text('PATCH_NAME="x"\n',encoding='utf-8')
    with tarfile.open(p/'legacy_tar.tgz','w:gz') as tf:
        data=b'print("ok")\n'; info=tarfile.TarInfo('patch_tar.py'); info.size=len(data); tf.addfile(info,io.BytesIO(data))

    with tarfile.open(p/'tool_distribution.tgz','w:gz') as tf:
        for name,data in [
            ('tools/run_python_patches.sh',b'#!/bin/sh\n'),
            ('tools/_patch_lib/self_test.py',b'PATCH_NAME="test only"\n'),
        ]:
            info=tarfile.TarInfo(name); info.size=len(data); tf.addfile(info,io.BytesIO(data))
    with tarfile.open(p/'dot_wrapped_tool_distribution.tgz','w:gz') as tf:
        for name,data in [
            ('./tools/run_python_patches.sh',b'#!/bin/sh\n'),
            ('./tools/_patch_lib/self_test.py',b'PATCH_NAME="test only"\n'),
        ]:
            info=tarfile.TarInfo(name); info.size=len(data); tf.addfile(info,io.BytesIO(data))
    with tarfile.open(p/'handoff_archive.tgz','w:gz') as tf:
        for name,data in [('HANDOFF_README.md',b'handoff'),('CURRENT_STATE.md',b'state'),('sample.py',b'PATCH_NAME="x"')]:
            info=tarfile.TarInfo(name); info.size=len(data); tf.addfile(info,io.BytesIO(data))

    # Non-patch artifacts must NOT pollute the runnable queue.
    with zipfile.ZipFile(p/'PTV_PASS_HANDOFF.zip','w') as z:
        z.writestr('CURRENT_STATE.md','handoff only')
    with zipfile.ZipFile(p/'PTV_260808_120000_deadbeef_PASS_DETAIL.zip','w') as z:
        z.writestr('raw/source.py','PATCH_NAME="embedded evidence only"\n')
        z.writestr('raw/CODE_COLLECTION_REQUEST_prior.json',json.dumps({'id':'prior','actions':[{'type':'overview'}]}))
    with zipfile.ZipFile(p/'python_patch_tool_v6.7.13.zip','w') as z:
        z.writestr('tools/run_python_patches.sh','#!/bin/sh\n')
        z.writestr('tools/_patch_lib/self_test.py','PATCH_NAME=\"test literal only\"\n')
    with zipfile.ZipFile(p/'PTV_REALISTIC_HANDOFF.zip','w') as z:
        z.writestr('HANDOFF_README.md','handoff')
        z.writestr('CURRENT_STATE.md','state')
        z.writestr('samples/example.py','PATCH_NAME=\"example only\"\n')
    with zipfile.ZipFile(p/'DATBIKE_BLE_OTA_HANDOFF_20260808.zip','w') as z:
        z.writestr('evidence/patch_embedded.py','PATCH_NAME="embedded evidence only"\n')
        z.writestr('evidence/CODE_COLLECTION_REQUEST_prior.json',json.dumps({'id':'prior','actions':[{'type':'overview'}]}))
    with zipfile.ZipFile(p/'DATBIKE_BLE_OTA_HANDOFF(1).zip','w') as z:
        z.writestr('evidence/patch_embedded.py','PATCH_NAME="embedded evidence only"\n')
    # The word handoff may legitimately appear in a legacy patch description.
    # The historical patch_* namespace remains authoritative for that case.
    with zipfile.ZipFile(p/'patch_fix_handoff_cleanup.zip','w') as z:
        z.writestr('patch_fix_handoff_cleanup.py','PATCH_NAME="legacy real patch"\n')
    with zipfile.ZipFile(p/'WRAPPED_TOOL_DISTRIBUTION.zip','w') as z:
        z.writestr('python_patch_tool/tools/run_python_patches.sh','#!/bin/sh\n')
        z.writestr('python_patch_tool/tools/_patch_lib/self_test.py','PATCH_NAME="test literal only"\n')
    with zipfile.ZipFile(p/'WRAPPED_HANDOFF.zip','w') as z:
        z.writestr('handoff/HANDOFF_README.md','handoff')
        z.writestr('handoff/CURRENT_STATE.md','state')
        z.writestr('handoff/samples/example.py','PATCH_NAME="example only"\n')
    # A HANDOFF may contain a previous valid COLLECT request as evidence. The
    # support-bundle identity must win; it must never become a runnable COLLECT.
    with zipfile.ZipFile(p/'WRAPPED_HANDOFF_WITH_COLLECT.zip','w') as z:
        z.writestr('handoff/HANDOFF_README.md','handoff')
        z.writestr('handoff/CURRENT_STATE.md','state')
        z.writestr('handoff/evidence/CODE_COLLECTION_REQUEST_prior.json',json.dumps({'id':'prior','actions':[{'type':'overview'}]}))
    (p/'notes.py').write_text('print("not a patch")\n',encoding='utf-8')
    (p/'broken.zip').write_bytes(b'not-a-zip')
    outside=root/'outside_patch.py'; outside.write_text('PATCH_NAME=\"outside\"\n',encoding='utf-8')
    (p/'linked_patch.py').symlink_to(outside)

    # Loose collect JSON is rejected; a malformed collect ZIP stays visibly invalid.
    (p/'CODE_COLLECTION_REQUEST_loose.json').write_text('{"actions":[]}',encoding='utf-8')
    with zipfile.ZipFile(p/'CODE_COLLECTION_REQUEST_bad.zip','w') as z:
        z.writestr('CODE_COLLECTION_REQUEST_bad.json',json.dumps({'actions':[]}))

    # Valid collect remains available.
    with zipfile.ZipFile(p/'collect_good.zip','w') as z:
        z.writestr('CODE_COLLECTION_REQUEST_good.json',json.dumps({'id':'good','actions':[{'type':'overview'}]}))

    items,w=m.discover_queue(root)
    by_name={i.name:i for i in items}
    expected_patch={
        'NFC_implement_201_example.zip','OTA_FIX_example.zip','patch_example.zip','other_valid_name.zip',
        'legacy_bundle.zip','patch_standalone.py','marker_standalone.py','legacy_tar.tgz','patch_with_collect_resource.zip','patch_fix_handoff_cleanup.zip'
    }
    assert expected_patch <= set(by_name), (expected_patch-set(by_name),items,w)
    assert all(by_name[n].kind=='PATCH' for n in expected_patch)
    assert by_name['collect_good.zip'].kind=='COLLECT'
    assert by_name['CODE_COLLECTION_REQUEST_bad.zip'].kind=='COLLECT INVALID'
    for rejected in ['PTV_PASS_HANDOFF.zip','PTV_260808_120000_deadbeef_PASS_DETAIL.zip','PTV_REALISTIC_HANDOFF.zip','DATBIKE_BLE_OTA_HANDOFF_20260808.zip','DATBIKE_BLE_OTA_HANDOFF(1).zip','WRAPPED_TOOL_DISTRIBUTION.zip','WRAPPED_HANDOFF.zip','WRAPPED_HANDOFF_WITH_COLLECT.zip','python_patch_tool_v6.7.13.zip','notes.py','broken.zip','linked_patch.py','tool_distribution.tgz','dot_wrapped_tool_distribution.tgz','handoff_archive.tgz','CODE_COLLECTION_REQUEST_loose.json']:
        assert rejected not in by_name, (rejected,items,w)
    joined='\n'.join(w)
    assert 'RAW JSON REJECTED' in joined
    assert 'PTV_PASS_HANDOFF.zip' in joined
    assert 'python_patch_tool_v6.7.13.zip' in joined
    assert 'broken.zip' in joined
    assert 'SKIPPED symlink queue entry: patchs/linked_patch.py' in joined

    # Revalidation must use the same support-bundle precedence. A selected
    # COLLECT replaced after selection by a HANDOFF containing a valid request
    # must fail closed rather than execute the embedded historical request.
    selected_collect = by_name['collect_good.zip']
    with zipfile.ZipFile(p/'collect_good.zip','w') as z:
        z.writestr('handoff/HANDOFF_README.md','handoff')
        z.writestr('handoff/CURRENT_STATE.md','state')
        z.writestr('handoff/CODE_COLLECTION_REQUEST_prior.json',json.dumps({'id':'prior','actions':[{'type':'overview'}]}))
    valid, reason = m._revalidate_selected_item(root, selected_collect)
    assert not valid and ('support bundle' in reason or 'replaced or modified' in reason), (valid, reason)

print('PASS: v6.7.13 queue recognizes PATCH/COLLECT structurally and skips non-patch artifacts')
