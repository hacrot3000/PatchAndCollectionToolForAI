#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, io, json, os, stat, sys, tempfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
MOD=HERE/'python_patch_queue_dispatcher.py'
spec=importlib.util.spec_from_file_location('ptv_queue_v677_selection',MOD)
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; spec.loader.exec_module(m)
assert m.VERSION=='6.8.0'

# Historical line grammar: lists, ranges (including reversed), bounds failure.
assert m._parse_index_spec('1,3-5',6)=={0,2,3,4}
assert m._parse_index_spec('5-3 1',6)=={0,2,3,4}
for bad in ['0','7','1-x','1,8']:
    try: m._parse_index_spec(bad,6)
    except ValueError: pass
    else: raise AssertionError(bad)

with tempfile.TemporaryDirectory(prefix='ptv678cfg_') as td:
    root=Path(td)
    # Documented confirmed automatic mode is restored.
    (root/'.python_patch_tool.json').write_text(json.dumps({'automation':{'zero_argument':{
        'selection':'first','non_interactive_confirmed':True,
        'initial_selection':'all','selector_ui':'line'}}}),encoding='utf-8')
    cfg,w=m._load_zero_argument_config(root)
    assert not w,w
    assert cfg=={'selection':'first','non_interactive_confirmed':True,'initial_selection':'all','selector_ui':'line'},cfg
    items=[m.QueueItem('patch_2.zip','PATCH'),m.QueueItem('patch_10.zip','PATCH')]
    assert [x.name for x in m._configured_auto_selection(root,items,cfg)]==['patch_2.zip']
    # Mixed PATCH/COLLECT never inherits old PATCH auto-selection implicitly.
    mixed=items+[m.QueueItem('collect.zip','COLLECT')]
    assert m._configured_auto_selection(root,mixed,cfg) is None

with tempfile.TemporaryDirectory(prefix='ptv678line_') as td:
    root=Path(td); p=root/'patchs'; p.mkdir()
    items=[]
    for n in ['patch_1.zip','patch_2.zip','patch_3.zip']:
        (p/n).write_text('x',encoding='utf-8'); items.append(m.QueueItem(n,'PATCH'))
    # Concrete range line confirms selection exactly as documented.
    old_in,old_out=m.sys.stdin,m.sys.stdout
    try:
        m.sys.stdin=io.StringIO('1,3\n')
        m.sys.stdout=io.StringIO()
        chosen=m._select_items_line(root,list(items),'none')
    finally:
        m.sys.stdin,m.sys.stdout=old_in,old_out
    assert [x.name for x in chosen]==['patch_1.zip','patch_3.zip'],chosen

    old_in,old_out=m.sys.stdin,m.sys.stdout
    try:
        m.sys.stdin=io.StringIO('a\n')
        m.sys.stdout=io.StringIO()
        chosen=m._select_items_line(root,list(items),'none')
    finally:
        m.sys.stdin,m.sys.stdout=old_in,old_out
    assert [x.name for x in chosen]==['patch_1.zip','patch_2.zip','patch_3.zip'],chosen

with tempfile.TemporaryDirectory(prefix='ptv678delete_') as td:
    root=Path(td); p=root/'patchs'; p.mkdir()
    items=[]
    for n in ['patch_1.zip','patch_2.zip','patch_3.zip']:
        (p/n).write_text('x',encoding='utf-8'); items.append(m.QueueItem(n,'PATCH'))
    old_in,old_out=m.sys.stdin,m.sys.stdout
    try:
        # delete range 1-2, confirm, then blank Enter confirms sole remaining item
        m.sys.stdin=io.StringIO('d 1-2\ny\n\n')
        m.sys.stdout=io.StringIO()
        chosen=m._select_items_line(root,items,'none')
    finally:
        m.sys.stdin,m.sys.stdout=old_in,old_out
    assert [x.name for x in chosen]==['patch_3.zip'],chosen
    assert not (p/'patch_1.zip').exists() and not (p/'patch_2.zip').exists()


# Ctrl+C in line mode is an operator cancellation, not a traceback.
class InterruptingInput:
    def readline(self):
        raise KeyboardInterrupt
with tempfile.TemporaryDirectory(prefix='ptv6711ctrlc_') as td:
    root=Path(td); pdir=root/'patchs'; pdir.mkdir(); (pdir/'patch_1.zip').write_text('x')
    old_in,old_out=m.sys.stdin,m.sys.stdout
    try:
        m.sys.stdin=InterruptingInput(); capture=io.StringIO(); m.sys.stdout=capture
        chosen=m._select_items_line(root,[m.QueueItem('patch_1.zip','PATCH')],'none')
    finally:
        m.sys.stdin,m.sys.stdout=old_in,old_out
    assert chosen is None,chosen
    assert 'Cancelled by Ctrl+C.' in capture.getvalue(),capture.getvalue()

print('PASS: v6.8.0 selector/config contracts and clean Ctrl+C handling')
