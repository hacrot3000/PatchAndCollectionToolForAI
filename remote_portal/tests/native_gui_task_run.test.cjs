'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const source=fs.readFileSync(
 'vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const match=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(match,'trusted GUI must bundle its JS locally');

class Element{
 constructor(id){
  this.id=id;this.textContent='';this.value='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(event,cb){this.listeners[event]=cb}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
 append(...nodes){this.children.push(...nodes)}
 replaceChildren(){this.children=[]}
 click(){assert.ok(this.listeners.click,'missing click handler '+this.id);this.listeners.click()}
}
const elements=new Map();
const get=id=>{
 if(!elements.has(id))elements.set(id,new Element(id));
 return elements.get(id);
};
const token='0123456789abcdef'.repeat(4);
const sent=[];
const confirmationMessages=[];
let approveTask=false;
let remaining=16;
let taskRunEnabled=true;
let nextExitCode=7;
const fetch=async(path,options)=>{
 sent.push(path);
 assert.equal(options.method,'POST');
 assert.equal(options.mode,'same-origin');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'b'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:true,
  task_run:taskRunEnabled,git_status:false,remote_control_available:false,
  remaining:--remaining,
 })};
 if(path==='/api/tasks-preview')return {ok:true,json:async()=>({
  ok:true,total:1,labels:['Short <script>'],ids:[55],
  remaining:--remaining,
 })};
 if(path==='/api/tasks-inspect/55')return {ok:true,json:async()=>({
  ok:true,task_id:55,eligible:true,input_id:'',choices:[],
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/tasks-run/55')return {ok:true,json:async()=>({
  ok:true,task_id:55,exit_code:nextExitCode,
  output:'safe <script>alert(1)</script>',truncated:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw new Error('unexpected endpoint '+path);
};
const sandbox={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{confirm(message){confirmationMessages.push(message);return approveTask},addEventListener(){}},history:{replaceState(){}}, 
 location:{hash:'#'+token,pathname:'/'},
 fetch,setInterval(){},Date,Number,String,Promise,
};
vm.createContext(sandbox);
new vm.Script(match[1],{filename:'trusted-task-run.js'}).runInContext(sandbox);
const flush=async()=>{for(let n=0;n<30;n++)await Promise.resolve()};
(async()=>{
 await flush();
 get('features').click();
 await flush();
 assert.match(get('feature-summary').textContent,/Chạy task: Được phép/);
 get('tasks-preview').click();
 await flush();
 const entries=get('task-preview-labels').children;
 assert.equal(entries.length,1);
 assert.equal(entries[0].textContent,'Short <script>');
 assert.equal(entries[0].children.length,3,'task needs inspect+run controls');
 assert.equal(entries[0].children[2].disabled,true,
  'task must be inspected before execution');
 entries[0].children[1].click();await flush();
 const checked=get('task-preview-labels').children[0];
 taskRunEnabled=false;
 get('features').click();await flush();
 const revoked=get('task-preview-labels').children[0];
 assert.notEqual(revoked,checked,'task_run flag change did not rebuild the row');
 assert.equal(revoked.children.length,2,
  'revoked task permission still displayed an execution button');
 taskRunEnabled=true;
 get('features').click();await flush();
 const restored=get('task-preview-labels').children[0];
 assert.equal(restored.children.length,3,
  'restored task permission did not restore its execution button');
 const btn=restored.children[2];
 assert.equal(btn.textContent,'Chạy task');
 assert.equal(btn.disabled,false);
 const previousBudget=remaining;
 btn.click();await flush();
 assert.equal(remaining,previousBudget,'declining task used E2EE quota');
 assert.equal(sent.filter(x=>x.startsWith('/api/tasks-run/')).length,0,
  'declining confirmation must not execute the host task');
 assert.match(confirmationMessages.at(-1),/Short <script> \(ID 55\)/);
 assert.match(confirmationMessages.at(-1),/thay đổi dữ liệu/);
 approveTask=true;
 btn.click();await flush();
 assert.deepEqual(sent.filter(x=>x.startsWith('/api/tasks-run/')),['/api/tasks-run/55']);
 assert.match(get('output').textContent,/mã thoát: 7/);
 assert.match(get('output').textContent,/safe <script>alert\(1\)<\/script>/);
 assert.equal(get('output').children.length,0,'output must not become executable HTML');
 // A task result must remain visible after the generic output area is
 // replaced by other actions, without writing to storage or the Relay.
 assert.equal(get('task-run-history').hidden,false);
 assert.equal(get('task-run-history-list').children.length,1);
 let entry=get('task-run-history-list').children[0];
 assert.match(entry.children[0].textContent,/Task 55 .* ngắn .* failed .* exit=7/);
 assert.match(entry.children[0].textContent,/Short <script>/);
 assert.equal(entry.children[0].children.length,0,'task label was interpreted as HTML');
 assert.equal(entry.children[1].textContent,'safe <script>alert(1)</script>');
 assert.equal(entry.children[1].children.length,0,'history output was interpreted as HTML');
 for(let i=0;i<7;i++){btn.click();await flush();}
 assert.equal(get('task-run-history-list').children.length,6,
  'browser retained more than six recent task executions');
 get('features').click();await flush();
 assert.match(get('output').textContent,/Đã đọc các quyền GUI/);
 assert.equal(get('task-run-history-list').children.length,6,
  'unrelated E2EE read erased last task results');
 const requestsBeforeClear=sent.length;
 get('task-run-history-clear').click();
 assert.equal(sent.length,requestsBeforeClear,'clearing local task history sent a network request');
 assert.equal(get('task-run-history-list').children.length,0);
 assert.equal(get('task-run-history').hidden,true);
 btn.click();await flush();
 assert.equal(get('task-run-history-list').children.length,1,
  'cleared history could not record a fresh task');
 // Exit zero is an actual success, unlike a completed invocation whose
 // host process returned a nonzero failure code.
 nextExitCode=0;
 btn.click();await flush();
 assert.match(get('task-run-history-list').children[0].children[0].textContent,
  /Task 55 .* ngắn .* completed .* exit=0/);
 get('disconnect').click();await flush();
 assert.equal(get('task-run-history-list').children.length,0,
  'logout retained sensitive host task output');
 assert.equal(get('task-run-history').hidden,true);
 assert.equal(get('task-preview-labels').children.length,0,'logout retained task selection');
 console.log('PASS: trusted native GUI runs only host-issued E2EE task IDs and renders output safely');
})().catch(error=>{console.error(error);process.exitCode=1});
