#!/usr/bin/env python3
from __future__ import annotations
import hashlib, json, os, sqlite3, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
import sys
sys.path.insert(0,str(HERE))
from python_patch_collect_schema import validate_request_data, CollectSchemaError
from python_patch_collect_compat import _run_request
import python_patch_queue_dispatcher as dispatcher
from python_patch_database_select import (
    DatabaseSelectError, compile_database_select, execute_database_select,
    validate_database_select_action,
)

def sha(p:Path): return hashlib.sha256(p.read_bytes()).hexdigest()

def assert_rejected(action, needle=''):
    try: validate_request_data({'actions':[action]})
    except Exception as exc:
        if needle: assert needle.lower() in str(exc).lower(),str(exc)
        return
    raise AssertionError('expected schema rejection')

with tempfile.TemporaryDirectory(prefix='ptv-db-select-test-') as td:
    root=Path(td).resolve(); (root/'patchs').mkdir(); (root/'tools').mkdir()
    db=root/'fixture.sqlite'
    con=sqlite3.connect(db)
    con.executescript('''
      CREATE TABLE teams(id INTEGER PRIMARY KEY, name TEXT);
      CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT, level INTEGER, team_id INTEGER);
      CREATE TABLE orders(id INTEGER PRIMARY KEY, user_id INTEGER, amount REAL, status TEXT);
      INSERT INTO teams VALUES(1,'Alpha'),(2,'Beta');
      INSERT INTO users VALUES(1,'Alice',120,1),(2,'Bob',80,1),(3,'Carol',150,2),(4,'Dave',110,2);
      INSERT INTO orders VALUES(1,1,10.5,'paid'),(2,1,20,'paid'),(3,2,5,'cancel'),(4,3,100,'paid'),(5,4,1,'paid');
    '''); con.commit(); con.close()
    profiles={
      'version':1,
      'profiles':{
        'sqlite_ro':{'engine':'sqlite','path':'fixture.sqlite'},
        'mysql_local':{'engine':'mysql','transport':'local','host':'127.0.0.1','port':3306,'database':'m3','auth':{'type':'login_path','login_path':'m3_ro'}},
        'mysql_ssh':{'engine':'mysql','transport':'ssh_tunnel','database':'m3','auth':{'type':'login_path','login_path':'m3_ro'},'ssh':{'target':'m3-local1','remote_host':'127.0.0.1','remote_port':3306,'connect_timeout_sec':3}},
      }
    }
    (root/'tools'/'db_profiles.local.json').write_text(json.dumps(profiles),encoding='utf-8')
    before=sha(db)

    paid_subquery={
      'select':[{'expr':{'column':'so.user_id'}}],
      'from':{'table':'orders','alias':'so'},
      'where':{'compare':{'left':{'column':'so.status'},'op':'=','right':{'value':'paid'}}}
    }
    complex_action={
      'type':'database_select','id':'complex','profile':'sqlite_ro','format':'csv','chunk_rows':1,
      'select':[
        {'expr':{'column':'u.id'},'alias':'user_id'},
        {'expr':{'column':'u.name'},'alias':'user_name'},
        {'expr':{'case':{'when':[{'if':{'compare':{'left':{'column':'u.level'},'op':'>=','right':{'value':120}}},'then':{'value':'high'}}],'else':{'value':'normal'}}},'alias':'tier'},
        {'expr':{'function':{'name':'COUNT','args':[{'column':'o.id'}]}},'alias':'paid_count'},
        {'expr':{'function':{'name':'SUM','args':[{'column':'o.amount'}]}},'alias':'paid_total'},
      ],
      'from':{'table':'users','alias':'u'},
      'joins':[{
        'type':'left','source':{'table':'orders','alias':'o'},
        'on':{'and':[
          {'compare':{'left':{'column':'o.user_id'},'op':'=','right':{'column':'u.id'}}},
          {'compare':{'left':{'column':'o.status'},'op':'=','right':{'value':'paid'}}},
        ]}
      }],
      'where':{'and':[
        {'compare':{'left':{'column':'u.id'},'op':'IN','right':{'subquery':paid_subquery}}},
        {'or':[
          {'compare':{'left':{'column':'u.team_id'},'op':'=','right':{'value':1}}},
          {'compare':{'left':{'column':'u.level'},'op':'>','right':{'value':100}}},
        ]}
      ]},
      'group_by':[{'column':'u.id'},{'column':'u.name'},{'column':'u.level'}],
      'having':{'compare':{'left':{'function':{'name':'COUNT','args':[{'column':'o.id'}]}},'op':'>','right':{'value':0}}},
      'order_by':[{'expr':{'function':{'name':'SUM','args':[{'column':'o.amount'}]}},'direction':'DESC'}],
      'max_rows':1000,'max_bytes':1024*1024,'timeout_sec':10,
    }
    req={'id':'db-complex','actions':[complex_action]}
    z=root/'patchs'/'CODE_COLLECTION_REQUEST_db-complex.zip'
    with zipfile.ZipFile(z,'w') as f:f.writestr('CODE_COLLECTION_REQUEST_db-complex.json',json.dumps(req))
    result,archived,count,lifecycle,status=_run_request(root,z)
    assert status=='PASS',(status,result)
    assert sha(db)==before,'SQLite file bytes changed during SELECT-only collection'
    with zipfile.ZipFile(result) as f:
        names=f.namelist(); assert 'reports/001_database_select.md' in names
        csvs=[n for n in names if n.startswith('database_queries/001_complex/result/') and n.endswith('.csv')]
        assert len(csvs)==3,csvs
        rows=''.join(f.read(n).decode() for n in csvs)
        assert 'Carol' in rows and 'Alice' in rows and 'Dave' in rows and 'Bob' not in rows,rows
        report=f.read('reports/001_database_select.md').decode()
        assert 'SELECT only' in report and 'Coverage status: **VERIFIED**' in report
        structure=f.read('database_queries/001_complex/ACTIVE_QUERY_STRUCTURE.json').decode()
        assert '"value": "paid"' not in structure and 'bound_type' in structure
        meta=json.loads(f.read('database_queries/001_complex/META.json'))
        assert meta['status']=='COMPLETED' and meta['rows_collected']==3
        assert 'profile_file' not in meta
    print('PASS: SQLite active builder JOIN/subquery/group/having/AND-OR/CASE + immutable DB')

    # Window function is represented structurally and executes through the same builder.
    win={
      'type':'database_select','profile':'sqlite_ro','format':'jsonl',
      'select':[
        {'expr':{'column':'u.name'},'alias':'name'},
        {'expr':{'function':{'name':'ROW_NUMBER','args':[],'over':{'order_by':[{'expr':{'column':'u.level'},'direction':'DESC'}]}}},'alias':'rank_no'}
      ],
      'from':{'table':'users','alias':'u'},'limit':3
    }
    work=root/'winwork'; r=execute_database_select(root,validate_database_select_action(win),{'max_total_bytes':1024*1024,'max_file_bytes':1024*1024},work)
    assert not r['incomplete'] and r['meta']['rows_collected']==3
    txt=(work/'database_select/result/result_0001.jsonl').read_text(); assert 'rank_no' in txt
    print('PASS: active builder window function OVER/ORDER BY')

    # max_rows is fail-partial, not fail-destructive.
    partial={
      'type':'database_select','profile':'sqlite_ro','select':[{'expr':{'column':'id'}}],
      'from':{'table':'users'},'order_by':[{'expr':{'column':'id'}}],
      'max_rows':1,'max_bytes':1024*1024
    }
    zp=root/'patchs'/'CODE_COLLECTION_REQUEST_db-partial.zip'
    with zipfile.ZipFile(zp,'w') as f:f.writestr('CODE_COLLECTION_REQUEST_db-partial.json',json.dumps({'id':'db-partial','actions':[partial]}))
    pres,*rest=_run_request(root,zp); pstatus=rest[-1]
    assert pstatus=='INCOMPLETE',pstatus
    with zipfile.ZipFile(pres) as f:
        manifest=json.loads(f.read('COLLECTION_MANIFEST.json')); assert manifest['collection_status']=='INCOMPLETE'
        data=f.read('database_queries/001_select/result/result_0001.csv').decode(); assert data.strip().splitlines()==['id','1']
    print('PASS: database_select max_rows preserves partial result + INCOMPLETE')

    # Schema has no raw SQL escape hatch and function identifiers are allowlisted.
    base={'type':'database_select','profile':'sqlite_ro','select':[{'expr':{'column':'id'}}],'from':{'table':'users'}}
    bad=dict(base); bad['query']='SELECT * FROM users'; assert_rejected(bad,'unsupported field')
    bad=dict(base); bad['raw_sql']='SELECT * FROM users'; assert_rejected(bad,'unsupported field')
    bad=dict(base); bad['select']=[{'expr':{'function':{'name':'SLEEP','args':[{'value':10}]}}}]; assert_rejected(bad,'allowlisted')
    bad=dict(base); bad['from']={'table':'users;DROP_TABLE'}; assert_rejected(bad,'unsafe identifier')
    print('PASS: raw SQL/mutation escape hatches and unsafe function/identifier are rejected')

    # Fake MySQL binaries validate local and SSH-tunnel execution without requiring a server.
    bindir=root/'fakebin'; bindir.mkdir(); capture=root/'mysql_capture.sql'
    mysql=bindir/'mysql'
    mysql.write_text('''#!/usr/bin/env python3\nimport os,sys,time\nsql=sys.stdin.read(); open(os.environ['PTV_FAKE_MYSQL_CAPTURE'],'w').write(sql)\nprint('id\\tname',flush=True)\nprint('1\\tAlice',flush=True)\nif os.environ.get('PTV_FAKE_MYSQL_SLEEP')=='1': time.sleep(2)\nelse: print('2\\tBob\\\\nTwo',flush=True)\n''',encoding='utf-8'); mysql.chmod(0o755)
    ssh=bindir/'ssh'
    ssh.write_text('''#!/usr/bin/env python3\nimport socket,sys\na=sys.argv; f=a[a.index('-L')+1]; port=int(f.split(':')[1]); s=socket.socket(); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1); s.bind(('127.0.0.1',port)); s.listen(5)\nwhile True:\n c,_=s.accept(); c.close()\n''',encoding='utf-8'); ssh.chmod(0o755)
    oldpath=os.environ.get('PATH',''); os.environ['PATH']=str(bindir)+os.pathsep+oldpath; os.environ['PTV_FAKE_MYSQL_CAPTURE']=str(capture)
    attack="O'Reilly'; DROP TABLE users; --"
    mbase={
      'type':'database_select','profile':'mysql_local','format':'jsonl',
      'select':[{'expr':{'column':'u.id'}},{'expr':{'column':'u.name'}}],
      'from':{'table':'users','alias':'u'},
      'where':{'compare':{'left':{'column':'u.name'},'op':'=','right':{'value':attack}}},
      'timeout_sec':3,'max_bytes':1024*1024,
    }
    try:
        mr=execute_database_select(root,validate_database_select_action(mbase),{'max_total_bytes':1024*1024,'max_file_bytes':1024*1024},root/'mysqlwork')
        sent=capture.read_text(); assert sent.strip().upper().startswith('SELECT '),sent
        assert sent.count(';')==1 and sent.rstrip().endswith(';'),sent # delimiter appended by mysql client wrapper only
        assert 'DROP TABLE' not in sent and '--' not in sent and 'CONVERT(0x' in sent,sent
        assert mr['meta']['rows_collected']==2 and not mr['incomplete']
        remote=dict(mbase); remote['profile']='mysql_ssh'; capture.write_text('')
        rr=execute_database_select(root,validate_database_select_action(remote),{'max_total_bytes':1024*1024,'max_file_bytes':1024*1024},root/'sshwork')
        assert rr['meta']['transport']=='ssh_tunnel' and rr['meta']['rows_collected']==2
        # timeout after a checkpointed row => partial result retained.
        os.environ['PTV_FAKE_MYSQL_SLEEP']='1'; timeout_action=dict(mbase); timeout_action['timeout_sec']=1
        tr=execute_database_select(root,validate_database_select_action(timeout_action),{'max_total_bytes':1024*1024,'max_file_bytes':1024*1024},root/'mysqltimeout')
        assert tr['incomplete'] and tr['meta']['rows_collected']==1 and any('timeout' in x for x in tr['reasons'])
        print('PASS: MySQL local + SSH tunnel + SELECT-only generated SQL + partial timeout')
    finally:
        os.environ['PATH']=oldpath; os.environ.pop('PTV_FAKE_MYSQL_SLEEP',None); os.environ.pop('PTV_FAKE_MYSQL_CAPTURE',None)

    # Credential-bearing fields are not accepted by local profiles either.
    (root/'tools'/'db_profiles.local.json').write_text(json.dumps({'version':1,'profiles':{'bad':{'engine':'mysql','transport':'local','database':'m3','auth':{'type':'login_path','login_path':'x'},'password':'secret'}}}),encoding='utf-8')
    bad_action=dict(mbase); bad_action['profile']='bad'
    try: execute_database_select(root,validate_database_select_action(bad_action),{'max_total_bytes':1024*1024,'max_file_bytes':1024*1024},root/'badwork')
    except DatabaseSelectError as exc: assert 'unsupported field' in str(exc),exc
    else: raise AssertionError('profile password field must be rejected')
    print('PASS: DB profile rejects embedded password fields')

    # Local database profiles are a hard evidence boundary, not merely a
    # generic sensitive-file warning.  Even a malicious/mistaken COLLECT
    # request must not copy or content-search the profile, and FAIL_HANDOFF
    # source discovery must refuse it as an attachment.
    marker='PTV_DB_PROFILE_NEVER_EVIDENCE_6A9F42'
    profile_path=root/'tools'/'db_profiles.local.json'
    profile_path.write_text(json.dumps({'version':1,'profiles':{'local_only':{'engine':'sqlite','path':'fixture.sqlite'},'note':marker}}),encoding='utf-8')
    assert dispatcher._safe_handoff_source(root,'tools/db_profiles.local.json') is None
    qitem=dispatcher.QueueItem('patch_dummy.zip','patch')
    attachments,discovery=dispatcher._discover_fail_handoff_sources(
        root,qitem,{'diagnosis':{'kind':'source_drift','affected_paths':['tools/db_profiles.local.json']}},
        'tools/db_profiles.local.json:1 test diagnostic'
    )
    assert all(rel!='tools/db_profiles.local.json' for rel,_ in attachments),attachments

    zpack=root/'patchs'/'CODE_COLLECTION_REQUEST_db-profile-pack.zip'
    with zipfile.ZipFile(zpack,'w') as f:
        f.writestr('CODE_COLLECTION_REQUEST_db-profile-pack.json',json.dumps({'id':'db-profile-pack','actions':[{'type':'pack','paths':['tools/db_profiles.local.json']}]}))
    try:
        _run_request(root,zpack)
    except Exception as exc:
        assert 'local database profile' in str(exc).lower(),exc
    else:
        raise AssertionError('exact COLLECT must refuse db_profiles.local.json')

    zsearch=root/'patchs'/'CODE_COLLECTION_REQUEST_db-profile-search.zip'
    search_req={'id':'db-profile-search','actions':[{'type':'search','query':marker,'paths':['.'],'regex':False,'must_find':False,'diagnose_on_zero':True,'fallback_search':True}]}
    with zipfile.ZipFile(zsearch,'w') as f:f.writestr('CODE_COLLECTION_REQUEST_db-profile-search.json',json.dumps(search_req))
    sresult,*srest=_run_request(root,zsearch); sstatus=srest[-1]
    assert sstatus=='PASS',sstatus
    with zipfile.ZipFile(sresult) as f:
        report=f.read('reports/001_search.md').decode('utf-8',errors='replace')
        assert 'Matches: 0' in report,report
        assert 'local_database_profile' in report,report
        assert not any(n == 'files/tools/db_profiles.local.json' or n.endswith('/db_profiles.local.json') for n in f.namelist()),f.namelist()
    print('PASS: local DB profile is hard-excluded from COLLECT/search/FAIL_HANDOFF evidence')

print('PASS: v6.20.2 database_select active-builder semantic contract')
