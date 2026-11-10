'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const source=fs.readFileSync(
 'vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const match=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(match,'native GUI script is missing');

class Element {
 constructor(id){
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(name,fn){this.listeners[name]=fn}
 setAttribute(key,value){this.attributes[key]=value}
 getAttribute(key){return this.attributes[key]||null}
 append(...nodes){this.children.push(...nodes)}
 replaceChildren(){this.children=[]}
 click(){
  assert.ok(this.listeners.click,'missing click handler: '+this.id);
  this.listeners.click();
 }
}

async function scenario(startStatus,pollStates,expectStartRejected=false){
 const elements=new Map();
 const get=id=>{
  if(!elements.has(id))elements.set(id,new Element(id));
  return elements.get(id);
 };
 const token='0123456789abcdef'.repeat(4);
 let budget=16;
 const calls=[];
 const reply=value=>({ok:true,json:async()=>value});
 const progress=(status,more=false)=>({
  ok:true,task_id:123,status,
  exit_code:status==='completed'?0:-1,
  output:'',truncated:false,more,
  remote_control_available:false,remaining:--budget,
 });
 const fetch=async(endpoint,opts)=>{
  calls.push(endpoint);
  assert.equal(opts.headers['X-TaskDeck-Local-Capability'],token);
  if(endpoint==='/api/session')return reply({
   ok:true,closed:false,active:false,remaining:budget,
   expires_in_seconds:300,device_id:'a'.repeat(64),username:'alice'
  });
  if(endpoint==='/api/features')return reply({
   ok:true,session_ping:true,project_info:false,tasks_preview:true,
   task_run:true,task_long:true,git_status:false,
   remote_control_available:false,remaining:--budget
  });
  if(endpoint==='/api/tasks-preview')return reply({
   ok:true,total:1,labels:['Long host task'],ids:[123],remaining:--budget
  });
  if(endpoint==='/api/tasks-inspect/123')return reply({
   ok:true,task_id:123,eligible:true,input_id:'',choices:[],
   input_type:'',input_id_2:'',choices_2:[],
   remote_control_available:false,remaining:--budget
  });
  if(endpoint==='/api/tasks-start/123'){
   const initial=typeof startStatus==='string'?startStatus:startStatus.status;
   return reply(Object.assign(progress(initial,false),
     typeof startStatus==='object'?startStatus:{}));
  }
  if(endpoint==='/api/tasks-poll/123'){
   assert.ok(pollStates.length,'unexpected extra poll');
   const next=pollStates.shift();
   const status=typeof next==='string'?next:next.status;
   return reply(Object.assign(progress(status,pollStates.length>0),
     typeof next==='object'?next:{}));
  }
  throw Error('unexpected native route '+endpoint);
 };
 const sandbox={
  document:{getElementById:get,createElement:tag=>new Element(tag)},
  window:{confirm(){return true},addEventListener(){}},
  history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
  fetch,setInterval(){},setTimeout(){},Date,Number,String,Promise
 };
 vm.createContext(sandbox);
 new vm.Script(match[1],{filename:'native-gui-lifecycle.js'}).runInContext(sandbox);
 const flush=async()=>{for(let n=0;n<50;n++)await Promise.resolve()};
 await flush();
 get('features').click();await flush();
 get('tasks-preview').click();await flush();
 let row=get('task-preview-labels').children[0];
 assert.ok(row,'missing task preview row');
 row.children[1].click();await flush();
 row=get('task-preview-labels').children[0];
 row.children[3].click();await flush();
 if(expectStartRejected){
  assert.equal(get('long-task-controls').hidden,true,
   'invalid start ACK did not lock and scrub GUI');
 }else{
  assert.equal(get('long-task-controls').hidden,false,'valid start rejected');
 }
 return {get,flush,calls};
}

(async()=>{
 // A host may complete a short-lived managed job before the start
 // acknowledgement is observed. The GUI must still own that job.
 const immediate=await scenario('completed',[]);
 assert.equal(immediate.get('long-task-clear').disabled,false,
  'immediate successful completion was not adopted');
 assert.equal(immediate.get('task-run-history-list').children.length,1);

 // Start/Cancel/Clear acknowledge only a state transition. The native
 // browser must also reject impossible nonempty output and truncation
 // on an ACK, even when a malicious localhost bridge returns HTTP 200.
 for(const forged of [
  {status:'started',output:'injected-host-output'},
  {status:'started',truncated:true},
 ]){
  const bad=await scenario(forged,[],true);
  assert.match(bad.get('output').textContent,/Trạng thái\/exit code/,
   'forged ACK was not rejected by typed browser validation');
  assert.equal(bad.get('long-task-output').textContent,'',
   'forged ACK leaked task output into the UI');
 }
 // A second "started" cannot overwrite the existing job/output/history.
 const restarted=await scenario('started',['started']);
 restarted.get('long-task-poll').click();await restarted.flush();
 assert.match(restarted.get('output').textContent,/vòng đời|Trạng thái\/exit code/);
 assert.equal(restarted.get('long-task-output').textContent,'');
 assert.equal(restarted.get('long-task-controls').hidden,true,
  'invalid status must close and scrub native GUI');

 // Only an explicit clear action may mark a host job cleared.
 const earlyClear=await scenario('started',['cleared']);
 earlyClear.get('long-task-poll').click();await earlyClear.flush();
 assert.match(earlyClear.get('output').textContent,/vòng đời|Trạng thái\/exit code/);

 // A terminal success cannot later turn into an active job.
 const regression=await scenario('started',['completed','running']);
 regression.get('long-task-poll').click();await regression.flush();
 assert.equal(regression.get('long-task-controls').hidden,false);
 regression.get('long-task-poll').click();await regression.flush();
 assert.match(regression.get('output').textContent,/vòng đời/);
 assert.equal(regression.get('long-task-controls').hidden,true);

 // Native Go E2EE validates exact UTF-8 byte limits and status/exit-code
 // relationships. The local browser must not accept an impossible
 // forged bridge reply simply because JS string.length is shorter.
 const impossible=[
  {status:'running',exit_code:0},
  {status:'running',output:'🙂'.repeat(40)}, // 80 UTF-16 units, 160 UTF-8 bytes
  {status:'running',output:'\\x1b[31m'}, // checked below as ESC escape
  {status:'cancel_requested',more:true},
  {status:'running',step:1,step_total:2,attempt:1,attempt_total:1,
   step_status:'running',output:'é'.repeat(40)}, // 80 bytes > 64 for workflow
 ];
 impossible[2].output='\x1b[31m';
 for(const response of impossible){
  const attempt=await scenario('started',[response]);
  attempt.get('long-task-poll').click();await attempt.flush();
  assert.equal(attempt.get('long-task-controls').hidden,true,
   'invalid E2EE task response did not fail closed: '+response.status);
  assert.equal(attempt.get('long-task-output').textContent,'',
   'invalid response left private output visible');
 }
 console.log('PASS: native long task lifecycle, strict UTF-8 quota and status codec');
})().catch(error=>{console.error(error);process.exitCode=1});
