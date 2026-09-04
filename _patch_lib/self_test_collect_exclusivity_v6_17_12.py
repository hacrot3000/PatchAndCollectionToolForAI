#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, io, sys, tempfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
MOD=HERE/'python_patch_queue_dispatcher.py'
spec=importlib.util.spec_from_file_location('ptv610_collect_exclusive',MOD)
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; spec.loader.exec_module(m)
assert m.VERSION=='6.17.12'

p1=m.QueueItem('patch_1.zip','PATCH')
p2=m.QueueItem('patch_2.zip','PATCH')
c1=m.QueueItem('CODE_COLLECTION_REQUEST_one.zip','COLLECT')
c2=m.QueueItem('CODE_COLLECTION_REQUEST_two.zip','COLLECT')
assert m._selection_contract_error([p1,p2]) is None
assert m._selection_contract_error([c1]) is None
assert 'chạy riêng' in m._selection_contract_error([c1,p1])
assert 'đúng 1' in m._selection_contract_error([c1,c2])

# initial_selection=all chooses PATCHes only in a mixed queue.
assert m._initial_selected([c1,p1,p2,c2],'all')=={1,2}

# Line mode rejects COLLECT+PATCH and multiple COLLECT, then accepts one COLLECT.
with tempfile.TemporaryDirectory(prefix='ptv610_line_collect_') as td:
    root=Path(td); (root/'patchs').mkdir()
    items=[c1,p1,c2]
    old_in,old_out=m.sys.stdin,m.sys.stdout
    capture=io.StringIO()
    try:
        m.sys.stdin=io.StringIO('1,2\n1,3\n3\n')
        m.sys.stdout=capture
        chosen=m._select_items_line(root,list(items),'none')
    finally:
        m.sys.stdin,m.sys.stdout=old_in,old_out
    assert [x.name for x in chosen]==[c2.name],chosen
    text=capture.getvalue()
    assert 'không thể chọn COLLECT cùng với PATCH' in text,text
    assert 'không thể chạy nhiều COLLECT cùng lúc' in text,text

# "a" is all PATCH, never all COLLECT.
with tempfile.TemporaryDirectory(prefix='ptv610_line_all_') as td:
    root=Path(td); (root/'patchs').mkdir()
    old_in,old_out=m.sys.stdin,m.sys.stdout
    try:
        m.sys.stdin=io.StringIO('a\n'); m.sys.stdout=io.StringIO()
        chosen=m._select_items_line(root,[c1,p1,p2,c2],'none')
    finally:
        m.sys.stdin,m.sys.stdout=old_in,old_out
    assert [x.name for x in chosen]==[p1.name,p2.name],chosen

# Line-mode "a/all" is PATCH-only. With COLLECT-only queue it must not
# auto-run the request; the operator must explicitly choose the COLLECT index.
with tempfile.TemporaryDirectory(prefix='ptv6101_line_all_collect_only_') as td:
    root=Path(td); (root/'patchs').mkdir()
    old_in,old_out=m.sys.stdin,m.sys.stdout
    capture=io.StringIO()
    try:
        m.sys.stdin=io.StringIO('a\n1\n'); m.sys.stdout=capture
        chosen=m._select_items_line(root,[c1],'none')
    finally:
        m.sys.stdin,m.sys.stdout=old_in,old_out
    assert [x.name for x in chosen]==[c1.name],chosen
    text=capture.getvalue()
    assert 'Không có PATCH để chọn tất cả' in text,text

# TTY: selecting a COLLECT clears other selections; selecting PATCH afterwards
# switches back to PATCH mode. Two COLLECT Space selections leave only the last.
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

def tty_choose(items,keys):
    old_in,old_out=m.sys.stdin,m.sys.stdout
    old_termios,old_tty,old_read=m.termios,m.tty,m._read_key
    it=iter(keys)
    try:
        m.sys.stdin=_FakeTTYIn(); m.sys.stdout=_FakeTTYOut()
        m.termios=_DummyTermios; m.tty=_DummyTty; m._read_key=lambda fd: next(it)
        return m.select_items(Path('.'),list(items),initial_selection='none',selector_ui='auto')
    finally:
        m.sys.stdin,m.sys.stdout=old_in,old_out
        m.termios,m.tty,m._read_key=old_termios,old_tty,old_read

chosen=tty_choose([c1,p1],['SPACE','DOWN','SPACE','ENTER'])
assert [x.name for x in chosen]==[p1.name],chosen
chosen=tty_choose([c1,c2],['SPACE','DOWN','SPACE','ENTER'])
assert [x.name for x in chosen]==[c2.name],chosen

# Execution boundary rejects bypass callers before any child starts.
with tempfile.TemporaryDirectory(prefix='ptv610_exec_guard_') as td:
    root=Path(td); (root/'patchs').mkdir(); (root/'tools').mkdir()
    rc,executed,remaining,dups,warnings=m.execute_items(root,[c1,p1])
    assert rc==2 and executed==[] and len(remaining)==2,(rc,executed,remaining)
    rc,executed,remaining,dups,warnings=m.execute_items(root,[c1,c2])
    assert rc==2 and executed==[] and len(remaining)==2,(rc,executed,remaining)

print('PASS: v6.17.12 exactly-one-COLLECT per invocation and no mixed PATCH/COLLECT selection')
