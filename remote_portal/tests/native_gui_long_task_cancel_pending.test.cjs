'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const source=fs.readFileSync(
 'vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const script=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(script,'native GUI JavaScript missing');
class Element {
 constructor(id) {
  this.id=id;this.textContent='';this.value='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(k,fn){this.listeners[k]=fn}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
 append(...items){this.children.push(...items)}
 replaceChildren(){this.children=[]}
 click(){assert.ok(this.listeners.click,'missing listener '+this.id);this.listeners.click()}
}
const nodes=new Map();
const get=id=>{
 if(!nodes.has(id))nodes.set(id,new Element(id));
 return nodes.get(id);
};
const capability='0123456789abcdef'.repeat(4);
let remaining=16,polls=0;
const calls=[],timers=[];
const respond=data=>({ok:true,json:async()=>data});
const progress=(status,output='')=>({
 ok:true,task_id:21,status,exit_code:status==='completed'?0:-1,
 output,truncated:false,more:false,remote_control_available:false,
 remaining:--remaining
});
const fetch=async (route,options)=>{
 calls.push(route);
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],capability);
 if(route==='/api/session')return respond({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'tester'
 });
 if(route==='/api/features')return respond({
  ok:true,session_ping:true,project_info:false,tasks_preview:true,
  task_run:true,task_long:true,git_status:false,
  remote_control_available:false,remaining:--remaining
 });
 if(route==='/api/tasks-preview')return respond({
  ok:true,total:1,labels:['Safe managed job'],ids:[21],remaining:--remaining
 });
 if(route==='/api/tasks-inspect/21')return respond({
  ok:true,task_id:21,eligible:true,input_id:'',choices:[],
  input_type:'',input_id_2:'',choices_2:[],
  remote_control_available:false,remaining:--remaining
 });
 if(route==='/api/tasks-start/21')return respond(progress('started'));
 if(route==='/api/tasks-cancel/21')return respond(progress('cancel_requested'));
 if(route==='/api/tasks-poll/21')return respond(
  polls++===0?progress('running','terminating...'):progress('completed')
 );
 if(route==='/api/tasks-clear/21')return respond(progress('cleared'));
 if(route==='/api/close')return respond({ok:true,closed:true});
 throw Error('unexpected localhost route '+route);
};
const sandbox={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{confirm(){return true},addEventListener(){}},
 history:{replaceState(){}},location:{hash:'#'+capability,pathname:'/'},
 fetch,setInterval(){},setTimeout(fn){timers.push(fn);return timers.length;},Date,Number,String,Promise
};
vm.createContext(sandbox);
const quotaHook="  globalThis.__checkCancelQuota=(left)=>{\n  const prior={id:longTaskID,state:longTaskState,pending:longTaskCancelPending,\n    budget,auto:longTaskAuto};\n  longTaskID=21;longTaskState='running';longTaskCancelPending=false;\n  longTaskAuto=false;budget=left;\n  render();\n  const permitted=taskMaintenanceAllowed('/api/tasks-cancel/21');\n  const disabled=longTaskCancel.disabled;\n  longTaskID=prior.id;longTaskState=prior.state;\n  longTaskCancelPending=prior.pending;longTaskAuto=prior.auto;\n  budget=prior.budget;render();\n  return {permitted,disabled};\n};\n";
const anchor='  function taskMaintenanceAllowed(endpoint){';
assert.ok(script[1].includes(anchor),'quota hook injection anchor missing');
const instrumented=script[1].replace(anchor,quotaHook+'\n'+anchor);
new vm.Script(instrumented,{filename:'cancel-pending-native.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<50;i++)await Promise.resolve()};
(async()=>{
 await flush();
 get('features').click();await flush();
 for(const [left,allowed] of [[1,false],[2,false],[3,true],[4,true]]){
  const result=sandbox.__checkCancelQuota(left);
  assert.equal(result.permitted,allowed,'Cancel quota guard at '+left);
  assert.equal(result.disabled,!allowed,'Cancel button quota at '+left);
 }
 get('tasks-preview').click();await flush();
 let row=get('task-preview-labels').children[0];
 row.children[1].click();await flush();
 row=get('task-preview-labels').children[0];
 row.children[3].click();await flush();
 assert.equal(timers.length,1,'running job must schedule an auto poll');
 assert.equal(get('long-task-cancel').disabled,false);
 // Reserve exactly four E2EE rounds before Cancel: Cancel, two status
 // polls (the first can still say "running"), and final Clear.
 remaining=4;
 get('long-task-cancel').click();await flush();
 assert.equal(remaining,3);
 assert.equal(get('long-task-poll').disabled,false,
  'cancel-pending must allow polling with one Clear round reserved');
 assert.equal(get('long-task-cancel').disabled,true,
  'acknowledged cancellation must disable duplicate cancel');
 assert.match(get('session-activity').textContent,/đã yêu cầu hủy/);
 assert.equal(timers.length,1,'Cancel must not queue a duplicate timer');
 timers.shift()();await flush();
 assert.match(get('long-task-output').textContent,/terminating/);
 assert.match(get('output').textContent,/Đã yêu cầu hủy/);
 assert.equal(get('long-task-cancel').disabled,true,
  'intermediate running poll must not re-arm the cancel button');
 assert.equal(remaining,2);
 assert.equal(timers.length,1,'pending cancellation must auto poll with two rounds remaining');
 assert.match(get('session-activity').textContent,/đã yêu cầu hủy/,
  'intermediate running reply must preserve pending-cancel indication');
 assert.equal(get('long-task-poll').disabled,false,
  'cancel-pending running response must allow one more poll and Clear');
 const count=calls.filter(x=>x==='/api/tasks-cancel/21').length;
 const quota=remaining;
 // Force invocation even while the button is disabled, to verify that
 // local action admission rejects duplicates as well as the visual state.
 get('long-task-cancel').click();await flush();
 assert.equal(calls.filter(x=>x==='/api/tasks-cancel/21').length,count);
 assert.equal(remaining,quota,'duplicate cancel must not consume AEAD quota');
 timers.shift()();await flush();
 assert.equal(timers.length,0,'terminal reply must reserve last round for Clear');
 assert.equal(get('long-task-clear').disabled,false);
 get('long-task-clear').click();await flush();
 assert.equal(remaining,0,'exactly four cleanup rounds were expected');
 assert.equal(get('long-task-controls').hidden,true);
 get('disconnect').click();await flush();
 assert.equal(get('long-task-output').textContent,'');
 console.log('PASS: cancel stays pending through running poll; no duplicate action');
})().catch(err=>{console.error(err);process.exitCode=1});
