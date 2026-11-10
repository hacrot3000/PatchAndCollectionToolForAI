'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const source=fs.readFileSync(
 'vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const match=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(match,'trusted GUI JavaScript missing');
assert.ok(match[1].includes('Một bước thất bại có thể được chạy lại đúng một lần'),
 'native GUI must disclose that host-approved retry can repeat side effects');
assert.ok(match[1].includes('bước hết giờ sẽ bị dừng, không chạy lại'),
 'native GUI must disclose host-authorized per-step timeout');
assert.ok(match[1].includes('workflow vẫn thất bại'),
 'native GUI must disclose the final cleanup cannot mask a failed workflow');

class Element{
 constructor(id){
  this.id=id;this.textContent='';this.value='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(name,fn){this.listeners[name]=fn}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
 replaceChildren(){this.children=[]}
 append(...nodes){this.children.push(...nodes)}
 click(){assert.ok(this.listeners.click,'missing click '+this.id);this.listeners.click()}
}
const nodes=new Map();
const get=id=>{if(!nodes.has(id))nodes.set(id,new Element(id));return nodes.get(id)};
const token='0123456789abcdef'.repeat(4);
const calls=[];let remaining=16;let workflowPollCount=0;
let available=true,confirmAccept=false,previewLabel='Build <b>safe</b>';
const fetch=async(path,opt)=>{
 calls.push({path,body:opt.body});
 assert.equal(opt.method,'POST');
 assert.equal(opt.headers['X-TaskDeck-Local-Capability'],token);
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:true,
  git_status:false,task_run:true,task_long:true,remote_control_available:false,
  remaining:--remaining,
 })};
 if(path==='/api/tasks-preview')return {ok:true,json:async()=>({
  ok:true,total:1,labels:[previewLabel],ids:[55],
  remaining:--remaining,
 })};
 if(path==='/api/tasks-dependencies/55')return {ok:true,json:async()=>({
  ok:true,task_id:55,available,run_available:available,
  order:available?[44,55]:[],edges:available?[[44,55]]:[],
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/tasks-workflow-start/55')return {ok:true,json:async()=>({
  ok:true,task_id:55,status:'started',exit_code:-1,
  more:false,output:'',truncated:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/tasks-poll/55'){
  workflowPollCount++;
  if(workflowPollCount===1)return {ok:true,json:async()=>({
   ok:true,task_id:55,status:'running',exit_code:-1,
   more:true,output:'authenticated-child-output',truncated:false,
   step:1,step_total:2,attempt:2,attempt_total:2,step_status:'running',
   step_history:'R-',step_details:'-|-',
   remote_control_available:false,remaining:--remaining,
  })};
  return {ok:true,json:async()=>({
   ok:true,task_id:55,status:'completed',exit_code:0,
   more:false,output:'SKIPPED',truncated:false,
   step:2,step_total:2,attempt:1,attempt_total:1,step_status:'skipped',
   step_history:'CS',step_details:'0:10|-',
   remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected endpoint '+path);
};
const scope={document:{getElementById:get,createElement:x=>new Element(x)},
 window:{addEventListener(){},confirm(){return confirmAccept}},history:{replaceState(){}},
 location:{hash:'#'+token,pathname:'/'},fetch,Date,Number,String,Promise,setInterval(){},setTimeout(){}};
vm.createContext(scope);
new vm.Script(match[1],{filename:'native-task-dependencies.js'}).runInContext(scope);
const flush=async()=>{for(let i=0;i<70;i++)await Promise.resolve()};

async function assertGraphValidation(graph,description,accepted=false){
 const localNodes=new Map();
 const getLocal=id=>{
  if(!localNodes.has(id))localNodes.set(id,new Element(id));
  return localNodes.get(id);
 };
 let left=16;
 const requests=[];
 const badFetch=async(path,opt)=>{
  requests.push(path);
  assert.equal(opt.headers['X-TaskDeck-Local-Capability'],token);
  if(path==='/api/session')return {ok:true,json:async()=>({
   ok:true,closed:false,active:false,remaining:left,expires_in_seconds:300,
   device_id:'a'.repeat(64),username:'alice'
  })};
  if(path==='/api/features')return {ok:true,json:async()=>({
   ok:true,session_ping:true,project_info:false,tasks_preview:true,
   git_status:false,task_run:true,task_long:true,remote_control_available:false,
   remaining:--left
  })};
  if(path==='/api/tasks-preview')return {ok:true,json:async()=>({
   ok:true,total:1,labels:['Workflow'],ids:[55],remaining:--left
  })};
  if(path==='/api/tasks-dependencies/55')return {ok:true,json:async()=>({
   ok:true,task_id:55,remote_control_available:false,remaining:--left,
   ...graph
  })};
  if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
  throw Error('malformed graph initiated an unexpected action '+path);
 };
 const localScope={
  document:{getElementById:getLocal,createElement:x=>new Element(x)},
  window:{addEventListener(){},confirm(){throw Error('unsafe workflow confirmation')}},
  history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
  fetch:badFetch,Date,Number,String,Promise,setInterval(){},setTimeout(){}
 };
 vm.createContext(localScope);
 new vm.Script(match[1],{filename:'native-task-dependency-invalid.js'})
  .runInContext(localScope);
 const drain=async()=>{for(let i=0;i<70;i++)await Promise.resolve()};
 await drain();
 getLocal('features').click();await drain();
 getLocal('tasks-preview').click();await drain();
 getLocal('task-deps-select').value='55';
 getLocal('task-deps-select').listeners.change();
 getLocal('task-deps-view').click();await drain();
 assert.equal(getLocal('task-workflow-start').disabled,!accepted,
  description+': wrong workflow start availability');
 assert.equal(getLocal('task-deps-output').hidden,!accepted,
  description+': wrong graph inspection visibility');
 if(accepted){
  assert.match(getLocal('task-deps-output').textContent,/Thứ tự topo/,
   description+': verified DAG not rendered');
 }
 assert.equal(requests.some(x=>x.includes('workflow-start')),false,
  description+': invalid graph started remote task');
}

(async()=>{
 await flush();
 assert.equal(get('task-deps-view').disabled,true,'unissued task ID could be inspected');
 get('features').click();await flush();
 get('tasks-preview').click();await flush();
 assert.equal(get('task-deps-select').children.length,2,'selector not bound to issued root ID');
 assert.equal(get('task-deps-select').children[1].value,'55');
 assert.match(get('task-deps-select').children[1].textContent,/Build <b>safe<\/b>/);
 get('task-deps-select').value='55';get('task-deps-select').listeners.change();
 assert.equal(get('task-deps-view').disabled,false,'issued task cannot inspect dependencies');
 get('task-deps-view').click();await flush();
 assert.deepEqual(calls.filter(x=>x.path.startsWith('/api/tasks-dependencies/')),
  [{path:'/api/tasks-dependencies/55',body:undefined}],
  'native GUI sent arbitrary args or did not use fixed Task.ID');
 assert.match(get('task-deps-output').textContent,/Thứ tự topo/);
 assert.match(get('task-deps-output').textContent,/Task #44/);
 assert.match(get('task-deps-output').textContent,/Build <b>safe<\/b>/);
 assert.equal(get('task-deps-output').children.length,0,'HTML from a task label executed');
 assert.equal(get('task-workflow-start').disabled,false,
  'reviewed linear workflow did not enable host-gated button');
 get('task-workflow-start').click();await flush();
 assert.equal(calls.filter(x=>x.path==='/api/tasks-workflow-start/55').length,0,
  'workflow launched without confirmation');
 confirmAccept=true;
 get('task-workflow-start').click();await flush();
 assert.deepEqual(calls.filter(x=>x.path==='/api/tasks-workflow-start/55'),
  [{path:'/api/tasks-workflow-start/55',body:undefined}],
  'workflow start must send only authenticated root ID');
 assert.equal(get('task-workflow-start').disabled,true,
  'second workflow run was allowed while one job owns the session');
 get('long-task-poll').click();await flush();
 assert.match(get('long-task-output').textContent,/Bước 1\/2/,
  'signed workflow step index not rendered');
 assert.match(get('long-task-output').textContent,/Lần 2\/2/,
  'signed workflow retry attempt not rendered');
 assert.match(get('long-task-output').textContent,/Bước 1: Đang chạy · Bước 2: Chưa chạy/,
  'authenticated per-step history not shown');
 assert.ok(!get('long-task-output').textContent.includes('exit=NaN'),
  'unfinished steps must not render a fabricated exit result');
 assert.match(get('long-task-output').textContent,/authenticated-child-output/,
  'workflow output was not preserved alongside typed progress');
 assert.match(get('long-task-output').textContent,/Bước 1 → Task #44/,
  'workflow progress did not map its first step to the real Task.ID');
 assert.match(get('long-task-output').textContent,
  /Bước 2 → Task #55 · Build <b>safe<\/b>/,
  'verified dependency labels did not appear in workflow progress');
 assert.equal(get('long-task-output').children.length,0,
  'task names must remain literal text, never executable HTML');
 get('long-task-poll').click();await flush();
 assert.match(get('long-task-output').textContent,
  /Bước 1: Hoàn tất \(exit=0, 10 ms\) · Bước 2: Bỏ qua/,
  'failure-only final step not rendered as skipped');
 assert.match(get('long-task-output').textContent,/Task 55 · completed · exit=0/,
  'skip after success incorrectly changed aggregate result');
 assert.ok(!/Bước 2: Bỏ qua \(exit=/.test(get('long-task-output').textContent),
  'skipped task was shown as an executed process');
 assert.match(get('long-task-output').textContent,/SKIPPED/,
  'completed workflow output was not appended');
 assert.equal(workflowPollCount,2,'workflow report needed additional arbitrary polling');
 available=false;
 get('task-deps-view').click();await flush();
 assert.match(get('task-deps-output').textContent,/Host chưa bật/);
 assert.equal(get('task-deps-output').children.length,0);
 // New task snapshots can rename or reorder their labels while a
 // terminal workflow is still present. Its per-run step map is immutable.
 previewLabel='RENAMED AFTER WORKFLOW';
 get('tasks-preview').click();await flush();
 assert.match(get('long-task-output').textContent,
  /Bước 2 → Task #55 · Build <b>safe<\/b>/,
  'task snapshot refresh replaced an already started workflow step label');
 assert.doesNotMatch(get('long-task-output').textContent,/RENAMED AFTER WORKFLOW/,
  'workflow map used unverified mutable task list labels');
 get('disconnect').click();await flush();
 assert.equal(get('task-deps-select').children.length,0,'issued IDs persisted after logout');
 assert.equal(get('task-deps-output').textContent,'','dependency graph persisted after logout');
 assert.equal(get('task-deps-output').hidden,true,'graph visible after logout');
 assert.equal(get('task-workflow-start').disabled,true,
  'workflow start button persisted after logout');
 assert.equal(get('long-task-output').textContent,'',
  'authenticated workflow progress not scrubbed on logout');
 await assertGraphValidation({
  available:true,run_available:true,
  order:[33,44,55],edges:[[33,55],[44,55]]
 },'explicit sequential fan-in',true);
 await assertGraphValidation({
  available:true,run_available:true,
  order:[22,33,44,55],edges:[[22,33],[22,44],[33,55],[44,55]]
 },'explicit sequential diamond',true);
 await assertGraphValidation({
  available:false,run_available:true,order:[],edges:[]
 },'false available with true run flag');
 await assertGraphValidation({
  available:true,run_available:true,
  order:[44,33,55],edges:[[44,33],[44,33]]
 },'duplicate edge replacing required dependency');
 await assertGraphValidation({
  available:true,run_available:true,
  order:[44,33,55],edges:[[44,33],[44,55]]
 },'nonlinear graph advertising executable chain');
 await assertGraphValidation({
  available:true,run_available:false,
  order:[44,33,55],edges:[[44,33],[44,33]]
 },'duplicate read-only graph edge');
 console.log('PASS: native E2EE workflow DAG preview, strict graph validation, scrubbed on logout');
})().catch(err=>{console.error(err);process.exitCode=1});
