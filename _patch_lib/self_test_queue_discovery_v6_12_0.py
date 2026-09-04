#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, io, json, sys, tarfile, tempfile, zipfile
from pathlib import Path
HERE=Path(__file__).resolve(); MOD=HERE.parent/'python_patch_queue_dispatcher.py'
spec=importlib.util.spec_from_file_location('ptv_queue_v678',MOD); m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; spec.loader.exec_module(m)

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
    with tarfile.open(p/'handoff_archive.tgz','w:gz') as tf:
        for name,data in [('HANDOFF_README.md',b'handoff'),('CURRENT_STATE.md',b'state'),('sample.py',b'PATCH_NAME="x"')]:
            info=tarfile.TarInfo(name); info.size=len(data); tf.addfile(info,io.BytesIO(data))
    with tarfile.open(p/'wrapped_handoff_archive.tgz','w:gz') as tf:
        for name,data in [('bundle/HANDOFF_README.md',b'handoff'),('bundle/CURRENT_STATE.md',b'state'),('bundle/sample.py',b'PATCH_NAME="x"')]:
            info=tarfile.TarInfo(name); info.size=len(data); tf.addfile(info,io.BytesIO(data))

    # Non-patch artifacts must NOT pollute the runnable queue.
    with zipfile.ZipFile(p/'PTV_PASS_HANDOFF.zip','w') as z:
        z.writestr('CURRENT_STATE.md','handoff only')
    with zipfile.ZipFile(p/'python_patch_tool_v6.12.0.zip','w') as z:
        z.writestr('tools/run_python_patches.sh','#!/bin/sh\n')
        z.writestr('tools/_patch_lib/self_test.py','PATCH_NAME=\"test literal only\"\n')
    with zipfile.ZipFile(p/'wrapped_python_patch_tool.zip','w') as z:
        z.writestr('release/tools/run_python_patches.sh','#!/bin/sh\n')
        z.writestr('release/tools/_patch_lib/self_test.py','PATCH_NAME=\"test literal only\"\n')
        z.writestr('release/evidence/CODE_COLLECTION_REQUEST_old.json',json.dumps({'actions':[{'type':'overview'}]}))
    with zipfile.ZipFile(p/'PTV_REALISTIC_HANDOFF.zip','w') as z:
        z.writestr('HANDOFF_README.md','handoff')
        z.writestr('CURRENT_STATE.md','state')
        z.writestr('samples/example.py','PATCH_NAME=\"example only\"\n')
    # A handoff may preserve the original request JSON as evidence. Handoff
    # identity must win over COLLECT discovery or an old request can rerun.
    with zipfile.ZipFile(p/'PTV_HANDOFF_WITH_COLLECT_REQUEST.zip','w') as z:
        z.writestr('bundle/HANDOFF_README.md','handoff')
        z.writestr('bundle/CURRENT_STATE.md','state')
        z.writestr('bundle/evidence/CODE_COLLECTION_REQUEST_old.json',json.dumps({'id':'old','actions':[{'type':'overview'}]}))

    # A readonly collection RESULT can contain collected Python whose filename
    # or source text looks exactly like a legacy PATCH. COLLECTION_MANIFEST is
    # authoritative non-runnable structure and must win before legacy marker
    # fallback, otherwise uploading a result ZIP into patchs/ could execute it.
    with zipfile.ZipFile(p/'m3_collection_result.zip','w') as z:
        z.writestr('COLLECTION_MANIFEST.json','{}')
        z.writestr('collected/patch_example.py','PATCH_NAME="evidence only"\n')
    with zipfile.ZipFile(p/'wrapped_collection_result.zip','w') as z:
        z.writestr('bundle/COLLECTION_MANIFEST.json','{}')
        z.writestr('bundle/collected/patch_nested.py','PATCH_NAME="evidence only"\n')
    # Re-zipping the collection folder on macOS can add __MACOSX/.DS_Store
    # entries outside the wrapper. Metadata must not break result identity and
    # expose collected patch-looking evidence as a runnable legacy PATCH.
    with zipfile.ZipFile(p/'wrapped_collection_result_macos.zip','w') as z:
        z.writestr('bundle/COLLECTION_MANIFEST.json','{}')
        z.writestr('bundle/collected/patch_nested.py','PATCH_NAME="evidence only"\n')
        z.writestr('__MACOSX/._COLLECTION_MANIFEST.json','appledouble')
        z.writestr('bundle/.DS_Store','metadata')
    with zipfile.ZipFile(p/'ambiguous_collection_with_patch_manifest.zip','w') as z:
        z.writestr('COLLECTION_MANIFEST.json','{}')
        z.writestr('PATCH_TOOL_MANIFEST.json','{}')
        z.writestr('patch_payload.py','PATCH_NAME="must not execute"\n')
    # A real v5 PATCH root manifest remains stronger even if the package also
    # carries a collection manifest as a resource/evidence file.
    with zipfile.ZipFile(p/'patch_with_collection_manifest_resource.zip','w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json','{}')
        z.writestr('resources/COLLECTION_MANIFEST.json','{}')
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
        'legacy_bundle.zip','patch_standalone.py','marker_standalone.py','legacy_tar.tgz','patch_with_collect_resource.zip','patch_with_collection_manifest_resource.zip'
    }
    assert expected_patch <= set(by_name), (expected_patch-set(by_name),items,w)
    assert all(by_name[n].kind=='PATCH' for n in expected_patch)
    assert by_name['collect_good.zip'].kind=='COLLECT'
    assert by_name['CODE_COLLECTION_REQUEST_bad.zip'].kind=='COLLECT INVALID'
    for rejected in ['PTV_PASS_HANDOFF.zip','PTV_REALISTIC_HANDOFF.zip','PTV_HANDOFF_WITH_COLLECT_REQUEST.zip','python_patch_tool_v6.12.0.zip','wrapped_python_patch_tool.zip','notes.py','broken.zip','linked_patch.py','tool_distribution.tgz','handoff_archive.tgz','wrapped_handoff_archive.tgz','m3_collection_result.zip','wrapped_collection_result.zip','wrapped_collection_result_macos.zip','ambiguous_collection_with_patch_manifest.zip','CODE_COLLECTION_REQUEST_loose.json']:
        assert rejected not in by_name, (rejected,items,w)
    joined='\n'.join(w)
    assert 'RAW JSON REJECTED' in joined
    assert 'PTV_PASS_HANDOFF.zip' in joined
    assert 'PTV_HANDOFF_WITH_COLLECT_REQUEST.zip' in joined
    assert 'python_patch_tool_v6.12.0.zip' in joined
    assert 'broken.zip' in joined
    assert 'm3_collection_result.zip' in joined and 'collection_result_archive' in joined
    assert 'wrapped_collection_result.zip' in joined
    assert 'wrapped_collection_result_macos.zip' in joined
    assert 'ambiguous_collection_with_patch_manifest.zip' in joined
    assert 'SKIPPED symlink queue entry: patchs/linked_patch.py' in joined

print('PASS: v6.12.0 queue recognizes PATCH/COLLECT structurally and skips non-patch artifacts')
