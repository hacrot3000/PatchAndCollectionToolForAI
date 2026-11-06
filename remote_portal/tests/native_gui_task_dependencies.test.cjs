'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const source=fs.readFileSync(
 'vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
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
 click(){assert.ok(this.listeners.click,'missing click '+this.id);this.listeners.click()}
}
const nodes=new Map();
const get=id=>{if(!nodes.has(id))nodes.set(id,new Element(id));return nodes.get(id)};
const token='0123456789abcdef'.repeat(4);
const calls=[];let remaining=16;
let available=true,confirmAccept=false;
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
  ok:true,total:1,labels:['Build <b>safe</b>'],ids:[55],
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
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected endpoint '+path);
};
const scope={document:{getElementById:get,createElement:x=>new Element(x)},
 window:{addEventListener(){},confirm(){return confirmAccept}},history:{replaceState(){}},
 location:{hash:'#'+token,pathname:'/'},fetch,Date,Number,String,Promise,setInterval(){}};
vm.createContext(scope);
new vm.Script(match[1],{filename:'native-task-dependencies.js'}).runInContext(scope);
const flush=async()=>{for(let i=0;i<70;i++)await Promise.resolve()};
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
 available=false;
 get('task-deps-view').click();await flush();
 assert.match(get('task-deps-output').textContent,/Host chưa bật/);
 assert.equal(get('task-deps-output').children.length,0);
 get('disconnect').click();await flush();
 assert.equal(get('task-deps-select').children.length,0,'issued IDs persisted after logout');
 assert.equal(get('task-deps-output').textContent,'','dependency graph persisted after logout');
 assert.equal(get('task-deps-output').hidden,true,'graph visible after logout');
 assert.equal(get('task-workflow-start').disabled,true,
  'workflow start button persisted after logout');
 console.log('PASS: native E2EE workflow DAG preview and content-free opt-out, scrubbed on logout');
})().catch(err=>{console.error(err);process.exitCode=1});
