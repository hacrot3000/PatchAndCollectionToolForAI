const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const test=require('node:test');
const source=fs.readFileSync(path.join(__dirname,'internal/server/ui.go'),'utf8');
function part(from,to){
  const start=source.indexOf(from),end=source.indexOf(to,start+from.length);
  assert.ok(start>=0&&end>start,'missing JS source: '+from);
  return source.slice(start,end);
}
const usage=part('function terminalUsageKey(id){','function wsURL(id){');
const lifecycle=part('async function closeView(id){','function teardownView(id){');
function rig({kind='terminal',task_id=0,target_type='local',status='running',stopError=false}={}){
  const calls=[],storage=new Map(),handlers=new Map(),hidden=new Set();
  const view={meta:{id:'t1',kind,task_id,target_type,status},canControl:true};
  const views=new Map([['t1',view]]);
  const globals={TaskMenuTerminalRestore:{persistSnapshot(){calls.push('snapshot');}}};
  const ctx={
    globalThis:globals,taskData:{workspace:'workspace'},views,hidden,
    window:{addEventListener(type,fn){handlers.set(type,fn);}},
    localStorage:{
      setItem(k,v){storage.set(k,v);},
      getItem(k){return storage.get(k)??null;},
      removeItem(k){storage.delete(k);}
    },
    jsonFetch:async(url,opts={})=>{
      calls.push((opts.method||'GET')+' '+url);
      if(opts.method==='POST'){
        if(stopError)throw Error('stop unavailable');
        view.meta.status='stopped';
      }
      return view.meta;
    },
    teardownView(id){calls.push('teardown');views.delete(id);},
    attach(meta){calls.push('attach');views.set(meta.id,view);},
    setTimeout,Promise,encodeURIComponent
  };
  const code=usage+'\n'+lifecycle+'\nglobalThis.testing={markFreshTerminal,markTerminalUsed,isPristineLocalTerminal,closeView,reopenHiddenSession};';
  vm.runInNewContext(code,ctx,{filename:'taskdeck-ui-js',timeout:1000});
  return {api:globals.testing,ctx,calls,storage,handlers,hidden,view,views};
}
test('fresh terminal close stops, removes and tears down PTY',async()=>{
  const h=rig();h.api.markFreshTerminal(h.view.meta);
  assert.equal(h.api.isPristineLocalTerminal(h.view),true);
  await h.api.closeView('t1');
  assert.deepEqual(h.calls,[
    'POST /api/sessions/t1/stop',
    'GET /api/sessions/t1',
    'DELETE /api/sessions/t1','teardown','snapshot'
  ]);
  assert.equal(h.views.size,0);
  assert.equal(h.hidden.has('t1'),true);
});
test('terminal that executed a command remains alive after tab closes',async()=>{
  const h=rig();h.api.markFreshTerminal(h.view.meta);
  h.handlers.get('taskmenu:shell-integration')({detail:{type:'execution-started',view:h.view}});
  assert.equal(h.api.isPristineLocalTerminal(h.view),false);
  await h.api.closeView('t1');
  assert.deepEqual(h.calls,['teardown','snapshot']);
});
test('old/restored terminal does not get killed without fresh evidence',async()=>{
  const h=rig();await h.api.closeView('t1');
  assert.deepEqual(h.calls,['teardown','snapshot']);
});
test('SSH or task sessions cannot qualify for fresh terminal cleanup',async()=>{
  for(const options of [{target_type:'ssh'},{kind:'task',task_id:17}]){
    const h=rig(options);h.api.markFreshTerminal(h.view.meta);
    await h.api.closeView('t1');
    assert.deepEqual(h.calls,['teardown','snapshot']);
  }
});
test('stop failure leaves terminal tab attached',async()=>{
  const h=rig({stopError:true});h.api.markFreshTerminal(h.view.meta);
  await assert.rejects(h.api.closeView('t1'),/stop unavailable/);
  assert.equal(h.views.has('t1'),true);
  assert.equal(h.hidden.has('t1'),false);
  assert.equal(h.view.closing,false);
});
test('reopening a used terminal reattaches live session without starting new process',async()=>{
  const h=rig();h.hidden.add('t1');
  assert.equal(await h.api.reopenHiddenSession('t1'),true);
  assert.deepEqual(h.calls,['GET /api/sessions/t1','attach']);
  assert.equal(h.hidden.has('t1'),false);
});
