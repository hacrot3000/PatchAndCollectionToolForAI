'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const match=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(match,'trusted GUI JavaScript missing');
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
 click(){assert.ok(this.listeners.click,'missing '+this.id);this.listeners.click()}
}
const nodes=new Map();
const get=id=>{if(!nodes.has(id))nodes.set(id,new Element(id));return nodes.get(id)};
const token='0123456789abcdef'.repeat(4);
const sent=[];
let remaining=16;
const epoch=Math.floor(Date.now()/1000);
const fetch=async(path,opt)=>{
 sent.push(path);
 assert.equal(opt.method,'POST');
 assert.equal(opt.headers['X-TaskDeck-Local-Capability'],token);
 assert.equal(opt.body,undefined,'task report must have no user-selected filters');
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:true,task_run:false,git_status:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/tasks-report')return {ok:true,json:async()=>({
  ok:true,items:[
   {task_id:42,status:'failed',exit_code:7,duration_ms:120,timestamp:epoch},
   {task_id:12,status:'completed',exit_code:0,duration_ms:10,timestamp:epoch-1},
  ],remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/tasks-workflow-audit')return {ok:true,json:async()=>({
  ok:true,items:[
   {root_task_id:80,task_id:70,step:2,total:3,attempt:2,
    status:'failed',exit_code:7,timestamp:epoch,run_id:'0123456789abcdef'},
   {root_task_id:80,task_id:60,step:1,total:3,attempt:1,
    status:'completed',exit_code:0,timestamp:epoch-1,run_id:'0123456789abcdef'},
  ],more:true,remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/tasks-workflow-audit-next')return {ok:true,json:async()=>({
  ok:true,items:[
   {root_task_id:80,task_id:50,step:1,total:3,attempt:1,
    status:'completed',exit_code:0,timestamp:epoch-2},
  ],more:false,remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected local endpoint '+path);
};
const context={document:{getElementById:get,createElement:x=>new Element(x)},
 window:{addEventListener(){},confirm(){return false}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,Date,Number,String,Promise,setInterval(){},setTimeout(){}};
vm.createContext(context);
new vm.Script(match[1],{filename:'native-task-report.js'}).runInContext(context);
const flush=async()=>{for(let i=0;i<80;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('tasks-report').disabled,true,'report enabled before features');
 get('features').click();await flush();
 assert.equal(get('tasks-report').disabled,false,'tasks.view report not shown');
 get('tasks-report').click();await flush();
 assert.deepEqual(sent.filter(p=>p==='/api/tasks-report'),['/api/tasks-report']);
 assert.equal(get('tasks-report-output').hidden,false);
 assert.match(get('tasks-report-output').textContent,/Task #42 · failed · exit 7 · 120 ms/);
 assert.match(get('tasks-report-output').textContent,/Task #12 · completed · exit 0/);
 assert.equal(get('tasks-report-output').children.length,0,'audit metadata became HTML');
 assert.equal(get('tasks-workflow-audit').disabled,false,
  'principal-only audit button not shown after features');
 get('tasks-workflow-audit').click();await flush();
 assert.deepEqual(sent.filter(p=>p==='/api/tasks-workflow-audit'),
  ['/api/tasks-workflow-audit'],
  'native GUI must read step audit without client arguments');
 assert.match(get('tasks-report-output').textContent,/Workflow #80/);
 assert.match(get('tasks-report-output').textContent,/Bước 2\/3 \(Task #70\)/);
 assert.match(get('tasks-report-output').textContent,/Lần 2 · failed · exit 7/);
 assert.equal((get('tasks-report-output').textContent.match(/Lượt 0123456789abcdef/g)||[]).length,2,
  'two completed steps must retain their same host-issued execution label');
 assert.match(get('tasks-report-output').textContent,/Còn trang; bấm Xem tiếp audit/);
 assert.equal(get('tasks-workflow-audit-next').disabled,false,
  'additional host-owned workflow audit page not offered');
 get('tasks-workflow-audit-next').click();await flush();
 assert.deepEqual(sent.filter(p=>p==='/api/tasks-workflow-audit-next'),
  ['/api/tasks-workflow-audit-next'],
  'next page must use a body-free native endpoint');
 assert.match(get('tasks-report-output').textContent,/Task #70/);
 assert.match(get('tasks-report-output').textContent,/Task #50/);
 assert.match(get('tasks-report-output').textContent,/Lượt cũ/,
  'historical step records without execution correlation remain readable');
 assert.equal(get('tasks-workflow-audit-next').disabled,true,
  'next button remained active after final audit page');
 assert.equal(get('tasks-report-output').children.length,0,
  'host-owned audit content was inserted as HTML');
 get('disconnect').click();await flush();
 assert.equal(get('tasks-report-output').hidden,true,'audit report visible after logout');
 assert.equal(get('tasks-report-output').textContent,'','audit persisted after logout');
 assert.equal(get('tasks-report').disabled,true,'report enabled after logout');
 assert.equal(get('tasks-workflow-audit').disabled,true,
  'workflow audit button enabled after logout');
 assert.equal(get('tasks-workflow-audit-next').disabled,true,
  'workflow audit next remained enabled after logout');
 console.log('PASS: principal-owned E2EE task outcome report, zero caller params, scrub on logout');
})().catch(e=>{console.error(e);process.exitCode=1});
