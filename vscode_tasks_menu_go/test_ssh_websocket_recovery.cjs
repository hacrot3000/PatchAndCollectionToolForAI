// Regression coverage for recovering the terminal WebSocket when both the
// socket and the session metadata endpoint briefly become unavailable.
// SSH process reconnection itself remains subject to sshreconnect_policy.js.
const fs=require('node:fs');
const path=require('node:path');
const assert=require('node:assert/strict');
const test=require('node:test');

const source=fs.readFileSync(path.join(__dirname,'internal/server/ui.go'),'utf8');
const start=source.indexOf('function connect(view,replay)');
const end=source.indexOf('\nfunction focusView(',start);
assert.ok(start>=0&&end>start,'embedded terminal connect function exists');
const snippet=source.slice(start,end);

function harness({metaStatus='running',failMetadata=false,browserLeaseLost=false}={}){
  const sockets=[],timers=[],updates=[],log=[];
  class FakeWebSocket{
    constructor(url){this.url=url;this.readyState=1;sockets.push(this);}
  }
  let failures=failMetadata, status=metaStatus;
  const create=new Function(
    'WebSocket','wsURL','jsonFetch','updateMeta','browserLeaseLost',
    'setTimeout','clearTimeout','console','writeOutput','filterOutput','window','CustomEvent',
    snippet+'\nreturn connect;'
  );
  const connect=create(
    FakeWebSocket,id=>'/ws/'+id,
    async url=>{
      log.push(url);
      if(failures)throw Error('proxy/network unavailable');
      return {id:'ssh-1',status,exit_code:status==='exited'?255:null};
    },
    meta=>updates.push(meta),
    browserLeaseLost,
    (fn,delay)=>{const timer={fn,delay,cancelled:false};timers.push(timer);return timer;},
    timer=>{if(timer)timer.cancelled=true;},
    {warn:()=>{}},
    ()=>{},data=>data,{dispatchEvent:()=>{}},function(){}
  );
  const view={meta:{id:'ssh-1'},closed:false,ws:null,
    socketReconnectAttempt:0,resets:0,
    term:{reset(){view.resets++;}},fit:{fit(){}}};
  const nextTimer=()=>timers.find(t=>!t.cancelled);
  const runTimer=()=>{const timer=nextTimer();assert.ok(timer,'pending retry');timer.cancelled=true;timer.fn();};
  return {connect,view,sockets,timers,updates,log,nextTimer,runTimer,
    setFailure(value){failures=value;},setStatus(value){status=value;}};
}

test('metadata outage after WebSocket disconnect retries attaching the same session',async()=>{
  const h=harness({failMetadata:true});
  h.connect(h.view,false);
  await h.sockets[0].onclose();
  assert.equal(h.nextTimer()?.delay,600);
  h.runTimer();
  assert.equal(h.sockets.length,2);
  assert.equal(h.view.resets,1);
  await h.sockets[1].onclose();
  assert.equal(h.nextTimer()?.delay,1200);
  h.sockets[1].onopen();
  assert.equal(h.view.socketReconnectAttempt,0);
});

test('SSH exit code 255 dispatches session metadata; WebSocket never creates a new SSH login',async()=>{
  const h=harness({metaStatus:'exited'});
  h.connect(h.view,false);
  await h.sockets[0].onclose();
  assert.equal(h.updates.length,1);
  assert.equal(h.updates[0].exit_code,255);
  assert.equal(h.nextTimer(),undefined);
  assert.equal(h.sockets.length,1);
  assert.deepEqual(h.log,['/api/sessions/ssh-1']);
});

test('normal session exit and user-closed tabs do not reconnect WebSocket',async()=>{
  const h=harness({metaStatus:'exited'});
  h.connect(h.view,false);
  h.view.closed=true;
  await h.sockets[0].onclose();
  assert.equal(h.nextTimer(),undefined);
});

test('successful metadata check still reattaches running sessions',async()=>{
  const h=harness({metaStatus:'running'});
  h.connect(h.view,false);
  await h.sockets[0].onclose();
  assert.equal(h.nextTimer()?.delay,600);
  h.runTimer();
  assert.equal(h.sockets.length,2);
  assert.equal(h.view.resets,1);
  // An old socket's close event cannot schedule a second live reconnect.
  await h.sockets[0].onclose();
  assert.equal(h.nextTimer(),undefined);
});

test('browser lease revocation prevents socket retries',async()=>{
  const h=harness({failMetadata:true,browserLeaseLost:true});
  h.connect(h.view,false);
  assert.equal(h.sockets.length,0);
});
