'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const script=source.match(/const trustedGUIJavaScript = \`([\s\S]*?)\`/);
assert.ok(script,'trusted native GUI JS missing');
assert.match(script[1],/canFetchTaskNext&&budget>\(longTaskID!==0\?3:0\)/,
 'bulk task paging must reserve three E2EE rounds for a running host job');
assert.match(script[1],/if\(!closed&&!terminated\)scheduleLongTaskPoll\(\);/,
 'bulk task paging must rearm long-task auto polling after it releases the UI');
class Element{
 constructor(id){
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(name,fn){this.listeners[name]=fn}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
 append(...els){this.children.push(...els)}
 replaceChildren(){this.children=[]}
 click(){assert.ok(this.listeners.click,'missing click '+this.id);this.listeners.click()}
}
const elements=new Map();
const get=id=>{
 if(!elements.has(id))elements.set(id,new Element(id));
 return elements.get(id);
};
const token='0123456789abcdef'.repeat(4);
let remaining=16,pollCount=0,nextCount=0,releaseBulk;
let discardConfirmationCount=0;
const timers=[];
const sent=[];
const fetch=async(endpoint,opts)=>{
 sent.push(endpoint);
 assert.equal(opts.method,'POST');
 assert.equal(opts.mode,'same-origin');
 assert.equal(opts.headers['X-TaskDeck-Local-Capability'],token);
 if(endpoint==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(endpoint==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:true,
  task_run:true,task_long:true,git_status:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(endpoint==='/api/tasks-preview')return {ok:true,json:async()=>({
  ok:true,total:3,labels:['Long host task'],ids:[123],remaining:--remaining,
 })};
 if(endpoint==='/api/tasks-preview-next'){
  nextCount++;
  if(nextCount===1)await new Promise(resolve=>{releaseBulk=resolve});
  return {ok:true,json:async()=>({
   ok:true,total:3,labels:['Extra host task'],ids:[124],remaining:--remaining,
  })};
 }
 if(endpoint==='/api/tasks-inspect/123')return {ok:true,json:async()=>({
  ok:true,task_id:123,eligible:true,input_id:'',choices:[],
  input_type:'',input_id_2:'',choices_2:[],
  remote_control_available:false,remaining:--remaining,
 })};
 if(endpoint==='/api/tasks-start/123')return {ok:true,json:async()=>({
  ok:true,task_id:123,status:'started',exit_code:-1,output:'',
  truncated:false,more:false,remote_control_available:false,remaining:--remaining,
 })};
 if(endpoint==='/api/tasks-poll/123'){
  pollCount++;
  return {ok:true,json:async()=>({
   ok:true,task_id:123,status:'completed',exit_code:0,
   output:pollCount===1?'A'.repeat(128):'TAIL',
   more:pollCount===1,truncated:false,
   remote_control_available:false,remaining:--remaining,
  })};
 }
 if(endpoint==='/api/tasks-cancel/123')return {ok:true,json:async()=>({
  ok:true,task_id:123,status:'cancel_requested',exit_code:-1,output:'',
  truncated:false,more:false,remote_control_available:false,remaining:--remaining,
 })};
 if(endpoint==='/api/tasks-clear/123')return {ok:true,json:async()=>({
  ok:true,task_id:123,status:'cleared',exit_code:-1,output:'',
  truncated:false,more:false,remote_control_available:false,remaining:--remaining,
 })};
 if(endpoint==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected endpoint '+endpoint);
};
const sandbox={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{confirm(message){
  if(message.includes('Bỏ vĩnh viễn')){
   discardConfirmationCount++;
   return false; // Protect unread tail; test may still drain and clear later.
  }
  return true;
 },addEventListener(){}},history:{replaceState(){}},
 location:{hash:'#'+token,pathname:'/'},
 fetch,setInterval(){},setTimeout(fn){timers.push(fn);return timers.length;},
 Date,Number,String,Promise,
};
vm.createContext(sandbox);
new vm.Script(script[1],{filename:'native-long-task.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<45;i++)await Promise.resolve()};
(async()=>{
 await flush();
 get('features').click();await flush();
 get('tasks-preview').click();await flush();
 let row=get('task-preview-labels').children[0];
 row.children[1].click();await flush();
 row=get('task-preview-labels').children[0];
 row.children[3].click();await flush();
 assert.equal(get('long-task-controls').hidden,false);
 assert.equal(timers.length,1,'auto polling did not schedule after start');
 // Legacy single-process task responses have no workflow metadata, so
 // the Native GUI must remain backwards compatible with existing Agent.
 assert.ok(!get('long-task-output').textContent.includes('Bước 0/0'),
  'legacy task rendered a fake workflow step');
 // A bulk page fetch occupies the single E2EE action slot; the pending
 // timer must not poll concurrently. Once bulk is stopped, poll resumes.
 get('task-fetch-all').click();await flush();
 assert.equal(typeof releaseBulk,'function','bulk request did not start');
 timers.shift()();await flush();
 assert.equal(pollCount,0,'auto poll overlapped bulk page request');
 get('task-stop-all').click();
 releaseBulk();await flush();
 assert.equal(nextCount,1,'stop must halt the bulk fetch after one page');
 assert.equal(timers.length,1,'bulk completion failed to resume automatic polling');
 get('long-task-auto').click();
 assert.equal(get('long-task-auto').textContent,'Tiếp tục tự nhận');
 timers.shift()();await flush();
 assert.equal(pollCount,0,'paused auto poll executed an E2EE request');
 get('long-task-auto').click();
 assert.equal(get('long-task-auto').textContent,'Tạm dừng tự nhận');
 assert.equal(timers.length,1,'resume did not schedule one poll');
 remaining=4;
 timers.shift()();await flush();
 assert.equal(pollCount,1);
 assert.equal(get('long-task-clear').textContent,'Bỏ output còn lại và dọn job');
 assert.equal(get('long-task-clear').disabled,false);
 assert.match(get('long-task-output').textContent,/còn output chưa nhận/);
 const beforeDiscard=sent.filter(x=>x==='/api/tasks-clear/123').length;
 const budgetBeforeDiscard=remaining;
 get('long-task-clear').click();await flush();
 assert.equal(discardConfirmationCount,1,'unread tail had no confirmation');
 assert.equal(sent.filter(x=>x==='/api/tasks-clear/123').length,beforeDiscard,
  'declining output discard triggered a destructive clear');
 assert.equal(remaining,budgetBeforeDiscard,'declining discard spent an E2EE action');
 assert.equal(get('long-task-controls').hidden,false);
 assert.equal(timers.length,1,
  'completed task must keep draining unread output with Clear quota reserved');
 assert.doesNotMatch(get('output').textContent,/Đã tạm dừng tự nhận/,
  'finished job no longer needs a Cancel reservation');
 assert.equal(get('task-fetch-all').disabled,true,'bulk button must reserve cancel quota');
 assert.equal(get('task-fetch-next').disabled,true,'manual next must preserve cleanup rounds');
 assert.equal(get('features').disabled,true,'features must preserve cleanup rounds');
 assert.equal(get('tasks-preview').disabled,true,'new snapshot must preserve cleanup rounds');
 assert.equal(get('long-task-poll').disabled,false,'finished job must permit draining output');
 const beforeReserved=sent.length;
 get('task-fetch-all').click();
 get('task-fetch-next').click();
 get('features').click();
 get('tasks-preview').click();
 get('quick-command').value='ping';
 get('quick-run').click();
 await flush();
 assert.equal(sent.length,beforeReserved,'unrelated clicks consumed job cleanup rounds');
 assert.equal(nextCount,1,'manual and bulk page fetches must both preserve quota');
 assert.match(get('quick-hint').textContent,/giữ lượt E2EE/);
 timers.shift()();await flush();
 assert.equal(pollCount,2,'finished job did not automatically drain its output');
 assert.equal(timers.length,0,'auto poll did not stop when output was fully drained');
 assert.equal(get('long-task-clear').textContent,'Dọn job đã kết thúc');
 assert.equal(get('long-task-poll').disabled,true);
 assert.match(get('long-task-output').textContent,/TAIL/);
 get('long-task-clear').click();await flush();
 assert.equal(get('long-task-controls').hidden,true);
 get('disconnect').click();await flush();
 assert.equal(get('long-task-output').textContent,'');
 assert.equal(get('task-preview-labels').children.length,0);
 console.log('PASS: long task autopolled, paused, reserved E2EE quota and drained terminal backlog');
})().catch(error=>{console.error(error);process.exitCode=1});
