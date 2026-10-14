const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const test=require('node:test');

const base=path.join(__dirname,'web/featuremods/');
const policySource=fs.readFileSync(path.join(base,'sshreconnect_policy.js'),'utf8');
const uiSource=fs.readFileSync(path.join(base,'sshreconnect.js'),'utf8');

function mount({disabled=false,initiallyExited=false}={}){
 const listeners=new Map(),timers=new Map(),intervals=new Map(),views=new Map();
 let seq=0;
 const createElement=tag=>({
  tagName:tag.toUpperCase(),children:[],textContent:'',hidden:false,
  disabled:false,checked:false,isConnected:true,
  append(...nodes){this.children.push(...nodes);},
  insertBefore(node){this.children.push(node);},
  querySelector(tag){return this.children.flatMap(n=>n.children||[]).find(n=>n.tagName===tag.toUpperCase())||null;}
 });
 const element=createElement('div');
 const session={id:'ssh-1',kind:'terminal',task_id:-1,label:'SSH · m3-prod',
  target_type:'ssh',target_profile_id:'profile-1',
  status:initiallyExited?'exited':'running',exit_code:initiallyExited?255:null};
 let onData=null;
 const view={
  meta:session,closed:false,tab:createElement('button'),pane:createElement('div'),
  term:{onData(fn){onData=fn;}}
 };
 view.pane.children=[createElement('div'),createElement('div')];views.set(session.id,view);
 const app={
  taskData:{workspace:'test'},views,active:session.id,browserLeaseLost:false,sharedMode:false,
  async jsonFetch(){throw Error('No network calls should occur before retry timer fires');},
  materializeSession(){throw Error('No reconnect attempt expected in classification test');}
 };
 const context={
  TaskMenuApp:app,
  localStorage:{getItem(){return disabled?'off':null;},setItem(){}},
  document:{head:element,createElement},window:{addEventListener(name,fn){
   if(!listeners.has(name))listeners.set(name,[]);listeners.get(name).push(fn);
  }},
  setTimeout(fn,delay){const id=++seq;timers.set(id,{fn,delay});return id;},
  clearTimeout(id){timers.delete(id);},
  setInterval(fn,delay){const id=++seq;intervals.set(id,{fn,delay});return id;},
  clearInterval(id){intervals.delete(id);},
  console:{warn(){}},TextDecoder,Map,Set,Date
 };
 context.globalThis=context;
 vm.runInNewContext(policySource,context,{timeout:1000});
 vm.runInNewContext(uiSource,context,{timeout:1000});
 const dispatch=(name,detail)=>{for(const fn of listeners.get(name)||[])fn({detail});};
 dispatch('taskmenu:session',{view,meta:session});
 const exit=(exit_code,status='exited')=>{
  view.meta={...view.meta,status,exit_code};
  dispatch('taskmenu:session',{view,meta:view.meta});
 };
 const retryDelays=()=>[...timers.values()].map(x=>x.delay).filter(ms=>ms<=30000);
 return {context,session,view,dispatch,exit,retryDelays,input(text){onData(text);},
  status(){return context.TaskDeckSSHReconnect.getState(session.id);}};
}
test('runtime: normal exit code 0 does not schedule reconnection',()=>{
 const a=mount();a.input('exit\r');a.exit(0);
 assert.deepEqual(a.retryDelays(),[]);
 assert.match(a.status().message,/auto reconnect skipped/);
});
test('runtime: user explicitly exit 255 is never mistaken for a transport error',()=>{
 const a=mount();a.input('exit 255');a.input('\r');a.exit(255);
 assert.deepEqual(a.retryDelays(),[]);
 assert.equal(a.status().explicitLogout,true);
});
test('runtime: Ctrl+D at empty prompt disables reconnect even with exit 255',()=>{
 const a=mount();a.input('\x04');a.exit(255);
 assert.deepEqual(a.retryDelays(),[]);
 assert.equal(a.status().explicitLogout,true);
});
test('runtime: connection timeout 255 schedules first 2-second retry',()=>{
 const a=mount();
 a.dispatch('taskmenu:output',{view:a.view,data:'ssh: connect to host m3-prod port 22: Connection timed out'});
 a.exit(255);
 assert.deepEqual(a.retryDelays(),[2000]);
 assert.match(a.status().message,/Reconnecting in 2s/);
});
test('runtime: host-key failure 255 stops automatic retries',()=>{
 const a=mount();
 a.dispatch('taskmenu:output',{view:a.view,data:'Host key verification failed.'});
 a.exit(255);
 assert.deepEqual(a.retryDelays(),[]);
 assert.match(a.status().message,/authentication\/host-key/);
});
test('runtime: opted-out settings never schedule automatic retries',()=>{
 const a=mount({disabled:true});
 a.exit(255);
 assert.deepEqual(a.retryDelays(),[]);
 assert.match(a.status().message,/disabled/);
});
test('runtime: browser reload showing an old exited SSH tab does not reconnect',()=>{
 const a=mount({initiallyExited:true});
 assert.deepEqual(a.retryDelays(),[]);
 assert.match(a.status().message,/manual reconnect available/);
});
