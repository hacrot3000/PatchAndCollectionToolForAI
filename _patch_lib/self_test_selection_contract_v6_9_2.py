#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, io, json, os, stat, sys, tempfile, subprocess, signal, time, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
MOD=HERE/'python_patch_queue_dispatcher.py'
spec=importlib.util.spec_from_file_location('ptv_queue_v677_selection',MOD)
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; spec.loader.exec_module(m)
assert m.VERSION=='6.9.2'

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
        try:
            m._select_items_line(root,[m.QueueItem('patch_1.zip','PATCH')],'none')
        except KeyboardInterrupt:
            pass
        else:
            raise AssertionError('line Ctrl+C must propagate to main for rc=130')
    finally:
        m.sys.stdin,m.sys.stdout=old_in,old_out
    assert 'Cancelled by Ctrl+C.' in capture.getvalue(),capture.getvalue()

# End-to-end non-TTY Ctrl+C must be visible to task runners as shell rc=130.
with tempfile.TemporaryDirectory(prefix='ptv691_line_sigint_') as td:
    root=Path(td); pdir=root/'patchs'; pdir.mkdir()
    with zipfile.ZipFile(pdir/'patch_1.zip','w') as zf:
        zf.writestr('PATCH_TOOL_MANIFEST.json','{}')
    proc=subprocess.Popen(
        [sys.executable,'-S',str(MOD),'--project-root',str(root)],
        stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,
    )
    time.sleep(0.25)
    os.kill(proc.pid, signal.SIGINT)
    out,err=proc.communicate(timeout=5)
    assert proc.returncode==130,(proc.returncode,out,err)
    assert 'Traceback' not in out+err,(out,err)
    assert 'Cancelled by Ctrl+C.' in out,(out,err)


# TTY priority contract: digit 0..9 assigns an explicit execution priority to
# the cursor row. Lower numbers run first; equal numbers preserve the queue
# order already shown by the tool. Plain [x] selections run after numbered
# priorities and retain natural queue order.
class _FakeTTYIn:
    def isatty(self): return True
    def fileno(self): return 123
class _FakeTTYOut(io.StringIO):
    def isatty(self): return True
class _DummyTermios:
    TCSADRAIN=0
    @staticmethod
    def tcgetattr(fd): return ['old']
    @staticmethod
    def tcsetattr(fd, when, old): return None
class _DummyTty:
    @staticmethod
    def setcbreak(fd): return None

priority_items=[m.QueueItem(f'patch_{i}.zip','PATCH') for i in range(1,6)]
keys=iter(['0','DOWN','1','DOWN','3','DOWN','0','DOWN','2','ENTER'])
old_stdin,old_stdout=m.sys.stdin,m.sys.stdout
old_termios,old_tty,old_read=m.termios,m.tty,m._read_key
try:
    m.sys.stdin=_FakeTTYIn(); m.sys.stdout=_FakeTTYOut()
    m.termios=_DummyTermios; m.tty=_DummyTty
    m._read_key=lambda fd: next(keys)
    chosen=m.select_items(Path('.'),list(priority_items),initial_selection='none',selector_ui='auto')
finally:
    m.sys.stdin,m.sys.stdout=old_stdin,old_stdout
    m.termios,m.tty,m._read_key=old_termios,old_tty,old_read
assert [x.name for x in chosen]==['patch_1.zip','patch_4.zip','patch_2.zip','patch_5.zip','patch_3.zip'],chosen

# Space on a numbered row returns it to ordinary [x] ordering; the next Space
# would deselect it.
keys=iter(['2','SPACE','ENTER'])
old_stdin,old_stdout=m.sys.stdin,m.sys.stdout
old_termios,old_tty,old_read=m.termios,m.tty,m._read_key
try:
    m.sys.stdin=_FakeTTYIn(); m.sys.stdout=_FakeTTYOut()
    m.termios=_DummyTermios; m.tty=_DummyTty
    m._read_key=lambda fd: next(keys)
    chosen=m.select_items(Path('.'),list(priority_items[:2]),initial_selection='none',selector_ui='auto')
finally:
    m.sys.stdin,m.sys.stdout=old_stdin,old_stdout
    m.termios,m.tty,m._read_key=old_termios,old_tty,old_read
assert [x.name for x in chosen]==['patch_1.zip'],chosen

# Mixed explicit priority and normal [x]: explicit priorities are intentional
# ordering overrides, while [x] items remain stable afterwards.
ordered=m._ordered_selection(
    priority_items,
    {0,1,2,4},
    {1:1,2:0},
)
assert [x.name for x in ordered]==['patch_3.zip','patch_2.zip','patch_1.zip','patch_5.zip'],ordered

# End-to-end execution order: the ordered selection is passed to the launcher
# exactly in priority order, not merely rendered that way.
with tempfile.TemporaryDirectory(prefix='ptv690priority_exec_') as td:
    root=Path(td); tools=root/'tools'; tools.mkdir(); (root/'patchs').mkdir()
    calls=root/'calls.txt'
    launcher=tools/'run_python_patches.sh'
    launcher.write_text(
        '#!/usr/bin/env bash\n'
        f'printf "%s\n" "$2" >> {str(calls)!r}\n'
        'exit 0\n',
        encoding='utf-8',
    )
    launcher.chmod(0o755)
    for item in priority_items:
        (root/'patchs'/item.name).write_bytes(b'not-identical-'+item.name.encode())
    priority_order=m._ordered_selection(priority_items,set(range(5)),{0:0,1:1,2:3,3:0,4:2})
    rc,executed,remaining,late_duplicates,warnings=m.execute_items(root,priority_order)
    assert rc==0,(rc,executed,remaining,warnings)
    assert remaining==[] and late_duplicates==[],(remaining,late_duplicates)
    assert [name for name,_ in executed]==['patch_1.zip','patch_4.zip','patch_2.zip','patch_5.zip','patch_3.zip'],executed
    assert calls.read_text(encoding='utf-8').splitlines()==[
        'patchs/patch_1.zip','patchs/patch_4.zip','patchs/patch_2.zip','patchs/patch_5.zip','patchs/patch_3.zip'
    ]
assert m._selection_mark(2,{0,1,2,4},{1:1,2:0})=='0'
assert m._selection_mark(0,{0,1,2,4},{1:1,2:0})=='x'

# Deletion reindexes priorities together with their PATCH row; a priority must
# never slide onto a different item after deleting an earlier row.
with tempfile.TemporaryDirectory(prefix='ptv690priority_delete_') as td:
    root=Path(td); pdir=root/'patchs'; pdir.mkdir()
    delete_items=[m.QueueItem(f'patch_{i}.zip','PATCH') for i in range(1,4)]
    for item in delete_items: (pdir/item.name).write_text('x',encoding='utf-8')
    selected={0,1,2}; priorities={1:4,2:1}
    selected,priorities,deleted,failures=m._delete_indexes(
        root,delete_items,selected,{0},priorities
    )
    assert failures==[],failures
    assert deleted==['patch_1.zip'],deleted
    assert [x.name for x in delete_items]==['patch_2.zip','patch_3.zip'],delete_items
    assert selected=={0,1},selected
    assert priorities=={0:4,1:1},priorities
    assert [x.name for x in m._ordered_selection(delete_items,selected,priorities)]==['patch_3.zip','patch_2.zip']


# Fullscreen rows must never wrap. Long OTA/NFC filenames are common and a
# wrapped physical row breaks the cursor-up accounting used by the selector,
# causing duplicated/overwritten rows after the next keypress. Cell width (not
# Python len) matters because CJK/full-width glyphs occupy two terminal cells.
assert m._clip_selector_line('x' * 200, 40).endswith('…')
assert m._display_cell_width(m._clip_selector_line('x' * 200, 40)) <= 38
assert m._display_cell_width(m._clip_selector_line('补丁' * 80, 40)) <= 38
long_item=m.QueueItem(
    'OTA_FIX_bletonfc_ble_ota_v10b_26_999_extremely_long_selector_filename_for_wrap_regression.zip',
    'PATCH',
    'manifest',
)
old_out=m.sys.stdout
old_width=m._selector_term_width
try:
    capture=_FakeTTYOut(); m.sys.stdout=capture
    m._selector_term_width=lambda: 42
    rendered=m._render([long_item],0,{0},{0:3},'Ưu tiên 3: '+'补丁'*40,0)
finally:
    m.sys.stdout=old_out; m._selector_term_width=old_width
assert rendered > 0
for physical in capture.getvalue().splitlines():
    clean=m._ANSI_RE.sub('',physical).replace('\r','')
    assert m._display_cell_width(clean) <= 40,(m._display_cell_width(clean),clean)

print('PASS: v6.9.2 selector/config/priority contracts and clean Ctrl+C handling')
