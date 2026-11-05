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
let remaining=16;
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
  task_run:true,git_status:false,remote_control_available:false,
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
  ok:true,task_id:55,exit_code:7,
  output:'safe <script>alert(1)</script>',truncated:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw new Error('unexpected endpoint '+path);
};
const sandbox={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{confirm(){return true},addEventListener(){}},history:{replaceState(){}},
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
 const btn=checked.children[2];
 assert.equal(btn.textContent,'Chạy task');
 assert.equal(btn.disabled,false);
 btn.click();await flush();
 assert.deepEqual(sent.filter(x=>x.startsWith('/api/tasks-run/')),['/api/tasks-run/55']);
 assert.match(get('output').textContent,/mã thoát: 7/);
 assert.match(get('output').textContent,/safe <script>alert\(1\)<\/script>/);
 assert.equal(get('output').children.length,0,'output must not become executable HTML');
 get('disconnect').click();await flush();
 assert.equal(get('task-preview-labels').children.length,0,'logout retained task selection');
 console.log('PASS: trusted native GUI runs only host-issued E2EE task IDs and renders output safely');
})().catch(error=>{console.error(error);process.exitCode=1});
