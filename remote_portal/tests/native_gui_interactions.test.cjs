'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');

const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const match=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(match,'trusted GUI JavaScript must be compiled into native client');

class MockElement{
  constructor(id){
    this.id=id;
    this.textContent='';
    this.hidden=false;
    this.disabled=false;
    this.listeners={};
    this.classList={toggle(){}};
    this.children=[];
  }
  addEventListener(event,cb){this.listeners[event]=cb;}
  click(){const cb=this.listeners.click;if(!cb)throw Error('click missing '+this.id);cb();}
  replaceChildren(){this.children=[];}
  append(...nodes){this.children.push(...nodes);}
}
const nodes=new Map();
function get(id){
  if(!nodes.has(id))nodes.set(id,new MockElement(id));
  return nodes.get(id);
}
const token='0123456789abcdef'.repeat(4);
let resolvePing;
let pingStarted=false,closeStarted=false,sessionStarted=false;
const fetch=async(path,options)=>{
  assert.equal(options.method,'POST');
  assert.equal(options.mode,'same-origin');
  assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
  if(path==='/api/session'){
    sessionStarted=true;
    return {ok:true,json:async()=>({
      ok:true,closed:false,active:false,remaining:4,expires_in_seconds:300,
      device_id:'a'.repeat(64),username:'alice',
    })};
  }
  if(path==='/api/ping'){
    pingStarted=true;
    return await new Promise(resolve=>{resolvePing=resolve;});
  }
  if(path==='/api/close'){
    closeStarted=true;
    return {ok:true,json:async()=>({ok:true,closed:true})};
  }
  throw Error('unexpected native UI endpoint '+path);
};
const history={replaceState(_state,_title,path){assert.equal(path,'/');}};
const location={hash:'#'+token,pathname:'/'};
const pageEvents=new Map();
const sandbox={
  document:{getElementById:get,createElement:(tag)=>new MockElement(tag)},
  window:{addEventListener(event,cb){pageEvents.set(event,cb);}},
  history,location,fetch,setInterval(){},Date,Number,String,Promise,
};
vm.createContext(sandbox);
new vm.Script(match[1],{filename:'trusted-remote-gui.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<15;i++)await Promise.resolve();};
(async()=>{
  await flush();
  assert.ok(sessionStarted,'GUI failed to load local-only session metadata');
  assert.equal(get('target-device').textContent,'a'.repeat(64));
  assert.equal(get('target-user').textContent,'alice');
  assert.equal(get('ping').disabled,false);
  get('ping').click();
  await flush();
  assert.ok(pingStarted,'E2EE ping not started');
  assert.equal(get('ping').disabled,true,'parallel read must be disabled');
  assert.equal(get('disconnect').disabled,false,'user must be able to disconnect while E2EE request is pending');
  get('disconnect').click();
  await flush();
  assert.ok(closeStarted,'pending E2EE blocked logout request');
  assert.equal(get('output').textContent,'Đã ngắt phiên.');
  // A slow/malicious Relay reply must never replace the local logout state.
  resolvePing({ok:true,json:async()=>({
    ok:true,message:'stale success must not show',remaining:2,round_trip_ms:100,
  })});
  await flush();
  assert.equal(get('output').textContent,'Đã ngắt phiên.');
  assert.equal(get('ping').disabled,true);
  assert.equal(get('disconnect').disabled,true);
  assert.ok(pageEvents.has('pagehide'),'browser page close handler missing');
  const countBefore=closeStarted;
  pageEvents.get('pagehide')();
  await flush();
  assert.equal(closeStarted,countBefore,'pagehide attempted to reopen an already closed session');
  console.log('PASS: native GUI renders bound Agent identity and logout beats pending E2EE reply');
})().catch(error=>{console.error(error);process.exitCode=1;});
