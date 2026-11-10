'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const script=source.match(/const trustedGUIJavaScript = \`([\s\S]*?)\`/);
assert.ok(script,'trusted native GUI JS missing');
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
let remaining=16,pollCount=0;
const sent=[],timers=[],copied=[];
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
  ok:true,total:1,labels:['Long host task'],ids:[123],remaining:--remaining,
 })};
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
  const status=pollCount===1?'running':'cancelled';
  return {ok:true,json:async()=>({
   ok:true,task_id:123,status,exit_code:-1,
   output:pollCount===1?'safe <script>alert(1)</script>':'',
   truncated:false,more:false,remote_control_available:false,remaining:--remaining,
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
 window:{confirm(){return true},addEventListener(){}},history:{replaceState(){}},
 location:{hash:'#'+token,pathname:'/'},
 navigator:{clipboard:{writeText(value){copied.push(value);return Promise.resolve()}}},
 fetch,setInterval(){},setTimeout(fn){timers.push(fn);return timers.length;},Date,Number,String,Promise,
};
vm.createContext(sandbox);
new vm.Script(script[1],{filename:'native-long-task.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<45;i++)await Promise.resolve()};
(async()=>{
 await flush();
 get('features').click();await flush();
 assert.match(get('feature-summary').textContent,/Job dài: Được phép/);
 get('tasks-preview').click();await flush();
 let row=get('task-preview-labels').children[0];
 assert.equal(row.children.length,4);
 assert.equal(row.children[3].disabled,true,'uninspected task could start a long job');
 row.children[1].click();await flush();
 row=get('task-preview-labels').children[0];
 assert.equal(row.children[3].textContent,'Chạy task dài');
 assert.equal(row.children[3].disabled,false);
 row.children[3].click();await flush();
 assert.equal(timers.length,1,'first task run did not schedule its poll');
 assert.deepEqual(sent.filter(x=>x.startsWith('/api/tasks-start/')),['/api/tasks-start/123']);
 assert.equal(get('long-task-controls').hidden,false);
 assert.equal(get('long-task-cancel').disabled,false);
 assert.match(get('session-activity').textContent,/Task 123 đang chạy/);
 assert.equal(get('task-run-history-list').children.length,1);
 assert.match(get('task-run-history-list').children[0].children[0].textContent,
  /Task 123 .* dài .* started/);
 get('long-task-poll').click();await flush();
 assert.match(get('long-task-output').textContent,/safe <script>alert\(1\)<\/script>/);
 assert.equal(get('long-task-output').children.length,0,
  'output must be textContent only, never HTML');
 get('long-task-copy').click();await flush();
 assert.deepEqual(copied,['safe <script>alert(1)</script>'],
  'explicit active-job copy must use authenticated output only');
 assert.equal(get('long-task-copy').textContent,'Đã sao chép');
 get('long-task-cancel').click();await flush();
 assert.match(get('session-activity').textContent,/Task 123 đã yêu cầu hủy/);
 assert.equal(get('long-task-controls').hidden,false);
 get('long-task-poll').click();await flush();
 assert.match(get('long-task-output').textContent,/cancelled/);
 assert.match(get('session-activity').textContent,/Task 123 đã kết thúc \(cancelled\) — chờ dọn job/);
 assert.match(get('task-run-history-list').children[0].children[0].textContent,
  /Task 123 .* dài .* cancelled/);
 const historyEntry=get('task-run-history-list').children[0];
 assert.equal(historyEntry.children.length,2,
  'long job history must retain a single bounded output preview');
 assert.equal(historyEntry.children[1].textContent,'safe <script>alert(1)</script>',
  'history must capture exactly the authenticated output once');
 assert.equal(historyEntry.children[1].children.length,0,
  'history output must be textContent, never injected as HTML');
 assert.equal(historyEntry.children[0].children.length,1,
  'history copy action should not replace the literal output pre');
 const historyCopy=historyEntry.children[0].children[0];
 historyCopy.click();await flush();
 assert.deepEqual(copied,['safe <script>alert(1)</script>',
  'safe <script>alert(1)</script>'],
  'history copy must use the bounded decrypted local output only');
 assert.equal(get('long-task-clear').disabled,false);
 get('long-task-clear').click();await flush();
 assert.equal(get('session-activity').textContent,'Không có tác vụ đang chạy');
 assert.equal(get('long-task-controls').hidden,true);
 assert.equal(get('task-run-history-list').children.length,1,
  'clear may remove host job but keep locally verified final outcome');
 assert.equal(get('long-task-copy').disabled,true,
  'cleared job still exposed an active-output clipboard action');
 // The first execution still has a queued callback. Reusing the same
 // published task ID must schedule a fresh callback immediately; the
 // stale callback cannot poll the new execution or reset its timer flag.
 row.children[3].click();await flush();
 assert.equal(sent.filter(x=>x==='/api/tasks-start/123').length,2,
  'same verified task ID could not be run again after Clear');
 assert.equal(timers.length,2,
  'a stale timer incorrectly prevented scheduling the new execution');
 const oldTimer=timers.shift(),currentTimer=timers.shift();
 const pollsBefore=sent.filter(x=>x==='/api/tasks-poll/123').length;
 oldTimer();await flush();
 assert.equal(sent.filter(x=>x==='/api/tasks-poll/123').length,pollsBefore,
  'stale timer polled a new execution sharing the previous task ID');
 currentTimer();await flush();
 assert.equal(sent.filter(x=>x==='/api/tasks-poll/123').length,pollsBefore+1,
  'new execution timer was cleared by the previous execution');
 get('disconnect').click();await flush();
 const beforeStaleCopy=copied.length;
 historyCopy.click();await flush();
 assert.equal(copied.length,beforeStaleCopy,
  'detached history button copied sensitive output after logout');
 assert.equal(get('task-run-history-list').children.length,0);
 assert.equal(get('task-run-history').hidden,true);
 assert.equal(get('long-task-output').textContent,'');
 assert.equal(get('task-preview-labels').children.length,0);
 console.log('PASS: native GUI long task stream, cancel without session loss and clear');
})().catch(error=>{console.error(error);process.exitCode=1});
