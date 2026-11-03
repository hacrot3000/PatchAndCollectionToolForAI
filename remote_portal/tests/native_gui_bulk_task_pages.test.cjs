'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');

const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const tick=String.fromCharCode(96);
const anchor='const trustedGUIJavaScript = '+tick;
const start=raw.indexOf(anchor);
assert.notEqual(start,-1,'native client must bundle trusted GUI JavaScript');
const codeStart=start+anchor.length;
const codeEnd=raw.indexOf(tick,codeStart);
assert.ok(codeEnd>codeStart);
const script=raw.slice(codeStart,codeEnd);
const token='abcdef0123456789'.repeat(4);
const flush=async()=>{for(let i=0;i<60;i++)await Promise.resolve();};

class Element {
 constructor(id){
  this.id=id;this.textContent='';this.value='';this.hidden=false;
  this.disabled=false;this.children=[];this.events={};
  this.classList={toggle(){}};
 }
 addEventListener(name,handler){this.events[name]=handler;}
 setAttribute(){}
 click(){if(!this.events.click)throw Error('no handler: '+this.id);this.events.click();}
 append(...nodes){this.children.push(...nodes);}
 replaceChildren(){this.children=[];}
}

function makeGUI({total=56,delayFirstNext=false}={}){
 const nodes=new Map(),calls=[];
 const get=id=>{
  if(!nodes.has(id))nodes.set(id,new Element(id));
  return nodes.get(id);
 };
 let rounds=0,nextCount=0,closed=false,pending=null;
 const makePage=(first)=>{
  const start=first?0:8*nextCount;
  const count=Math.min(8,total-start);
  return {ok:true,total,
   labels:Array.from({length:count},(_,i)=>'Task '+String(start+i).padStart(3,'0')),
   remaining:16-rounds};
 };
 const json=response=>({ok:true,json:async()=>response});
 const fetch=async(path,options)=>{
  assert.equal(options.method,'POST');
  assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
  calls.push(path);
  if(path==='/api/session'){
   return json({ok:true,closed:false,remaining:16,expires_in_seconds:300,
    device_id:'a'.repeat(64),username:'operator'});
  }
  if(path==='/api/features'){
   rounds++;
   return json({ok:true,remote_control_available:false,session_ping:true,
    project_info:true,tasks_preview:true,git_status:true,remaining:16-rounds});
  }
  if(path==='/api/tasks-preview'){
   rounds++;
   return json(makePage(true));
  }
  if(path==='/api/tasks-preview-next'){
   rounds++;nextCount++;
   const response=json(makePage(false));
   if(delayFirstNext&&nextCount===1){
    return await new Promise(resolve=>{pending=()=>resolve(response);});
   }
   return response;
  }
  if(path==='/api/close'){closed=true;return json({ok:true,closed:true});}
  throw Error('unreviewed GUI or PHP operation: '+path);
 };
 const pageEvents={};
 const context={
  document:{getElementById:get,createElement:id=>new Element(id)},
  window:{addEventListener(name,handler){pageEvents[name]=handler;}},
  fetch,location:{hash:'#'+token,pathname:'/'},
  history:{replaceState(){}},setInterval(){},Date,Number,String,Promise,
 };
 vm.createContext(context);
 new vm.Script(script,{filename:'native-gui-bulk-pages.js'}).runInContext(context);
 return {get,calls,pageEvents,rounds:()=>rounds,nextCount:()=>nextCount,
  releaseNext(){assert.equal(typeof pending,'function');const cb=pending;pending=null;cb();},
  closed:()=>closed};
}

async function testStoppedBulkDoesNotQueueMoreRequests(){
 const gui=makeGUI({total:56,delayFirstNext:true});
 const {get,calls}=gui;
 await flush();
 get('features').click();await flush();
 get('tasks-preview').click();await flush();
 assert.equal(get('task-total').textContent,'8 / 56 task');
 assert.equal(get('task-fetch-all').hidden,false);
 const callsBefore=calls.length;
 get('task-fetch-all').click();await flush();
 assert.equal(gui.nextCount(),1,'bulk must not queue multiple E2EE requests');
 assert.equal(get('task-stop-all').hidden,false,'stop control missing during in-flight request');
 assert.equal(get('task-fetch-all').hidden,true,'must not start a second bulk loader');
 assert.equal(get('task-fetch-next').disabled,true,'manual fetch must not race with bulk');
 get('task-stop-all').click();await flush();
 gui.releaseNext();await flush();
 assert.equal(gui.nextCount(),1,'stop queued another E2EE request');
 assert.equal(calls.length,callsBefore+1,'stop unexpectedly called PHP or Relay');
 assert.equal(get('task-total').textContent,'16 / 56 task');
 assert.match(get('task-load-status').textContent,/Đã dừng/);
 assert.equal(get('task-stop-all').hidden,true);
 // Explicit resume starts at the host-owned next page, never from offset.
 get('task-fetch-all').click();await flush();
 assert.equal(gui.nextCount(),6);
 assert.equal(get('task-total').textContent,'56 / 56 task');
 assert.equal(get('task-fetch-all').hidden,true);
 assert.match(get('task-load-status').textContent,/Đã tải đủ/);
 assert.equal(gui.rounds(),8,'unexpected number of authenticated application rounds');
 assert.equal(get('task-preview-labels').children[0].textContent,'Task 000');
 assert.equal(get('task-coverage').textContent,'Đã tải 56 / 56 nhãn task');
 get('disconnect').click();await flush();
 assert.equal(get('task-coverage').textContent,'','logout retained private task progress');
 assert.equal(get('task-preview-labels').children.length,0,'logout retained labels');
}

async function testBulkQuotaIsBoundedToSixteenRounds(){
 const gui=makeGUI({total:500});
 const {get,calls}=gui;
 await flush();
 get('features').click();await flush();
 get('tasks-preview').click();await flush();
 get('task-fetch-all').click();await flush();
 assert.equal(gui.rounds(),16,'bulk exceeded or failed to utilize 16 E2EE rounds');
 assert.equal(gui.nextCount(),14,'bulk did not stop at 14 next-page requests');
 assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 0');
 assert.equal(get('task-total').textContent,'120 / 500 task');
 assert.match(get('task-load-status').textContent,/Đã hết lượt E2EE/);
 assert.equal(get('task-fetch-all').disabled,true);
 assert.equal(get('task-stop-all').hidden,true);
 const atQuota=calls.length;
 get('task-fetch-all').click();get('task-fetch-next').click();await flush();
 assert.equal(calls.length,atQuota,'exhausted GUI sent a 17th E2EE request');
 gui.pageEvents.pagehide();await flush();
 assert.equal(get('task-total').textContent,'');
 assert.equal(get('task-preview-labels').children.length,0);
}

async function testDisconnectWhileBulkPending(){
 const gui=makeGUI({total:100,delayFirstNext:true});
 const {get}=gui;
 await flush();
 get('features').click();await flush();
 get('tasks-preview').click();await flush();
 get('task-fetch-all').click();await flush();
 assert.equal(gui.nextCount(),1);
 get('disconnect').click();await flush();
 assert.equal(gui.closed(),true,'disconnect was blocked behind pending bulk E2EE');
 assert.equal(get('task-preview-labels').children.length,0,
  'disconnect failed to erase cached labels synchronously');
 gui.releaseNext();await flush();
 assert.equal(gui.nextCount(),1,'stale response restarted bulk after logout');
 assert.equal(get('task-fetch-all').disabled,true);
 assert.equal(get('task-load-status').textContent,'');
}

(async()=>{
 await testStoppedBulkDoesNotQueueMoreRequests();
 await testBulkQuotaIsBoundedToSixteenRounds();
 await testDisconnectWhileBulkPending();
 console.log('PASS: explicit native-only bulk task load is serialized, stoppable, bounded, and revoked on logout');
})().catch(err=>{console.error(err);process.exitCode=1;});
