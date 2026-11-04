'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const script=source.match(/const trustedGUIJavaScript = \`([\s\S]*?)\`/);
assert.ok(script,'missing native-bundled JavaScript');
class Element{
 constructor(id){
  this.id=id;this.textContent='';this.value='';
  this.disabled=false;this.hidden=false;this.children=[];
  this.listeners={};this.attributes={};this.classList={toggle(){}};
 }
 addEventListener(name,fn){this.listeners[name]=fn}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
 append(...nodes){this.children.push(...nodes)}
 replaceChildren(){this.children=[]}
 click(){assert.ok(this.listeners.click,'click missing '+this.id);this.listeners.click()}
}
const nodes=new Map(),get=id=>{
 if(!nodes.has(id))nodes.set(id,new Element(id));
 return nodes.get(id);
};
const token='0123456789abcdef'.repeat(4),sent=[];
let remaining=16;
const fetch=async(endpoint,options)=>{
 sent.push(endpoint);
 assert.equal(options.method,'POST');
 assert.equal(options.mode,'same-origin');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 if(endpoint==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(endpoint==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:true,
  task_run:true,git_status:false,remote_control_available:false,
  remaining:--remaining,
 })};
 if(endpoint==='/api/tasks-preview')return {ok:true,json:async()=>({
  ok:true,total:1,labels:['Deploy'],ids:[123],remaining:--remaining,
 })};
 if(endpoint==='/api/tasks-inspect/123')return {ok:true,json:async()=>({
  ok:true,task_id:123,eligible:true,input_id:'target',
  choices:['gateway','main'],remote_control_available:false,
  remaining:--remaining,
 })};
 if(endpoint==='/api/tasks-run/123/2')return {ok:true,json:async()=>({
  ok:true,task_id:123,exit_code:0,output:'deployed main\n',
  truncated:false,remote_control_available:false,remaining:--remaining,
 })};
 if(endpoint==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected native route '+endpoint);
};
const sandbox={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{addEventListener(){}},history:{replaceState(){}},
 location:{hash:'#'+token,pathname:'/'},fetch,
 setInterval(){},Date,Number,String,Promise,
};
vm.createContext(sandbox);
new vm.Script(script[1],{filename:'trusted-native-pick.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<35;i++)await Promise.resolve()};
(async()=>{
 await flush();
 get('features').click();await flush();
 get('tasks-preview').click();await flush();
 let row=get('task-preview-labels').children[0];
 assert.equal(row.children.length,3);
 assert.equal(row.children[2].disabled,true,
  'uninspected task must not start any process');
 row.children[1].click();await flush();
 row=get('task-preview-labels').children[0];
 assert.equal(row.children.length,5,'pickString should show fixed options');
 const select=row.children[3],button=row.children[4];
 assert.equal(select.children.length,2);
 assert.equal(select.children[0].textContent,'gateway');
 assert.equal(select.children[1].textContent,'main');
 assert.equal(button.disabled,false);
 select.value='2';
 button.click();await flush();
 assert.deepEqual(sent.filter(path=>path.startsWith('/api/tasks-run/')),
  ['/api/tasks-run/123/2'],'client must send only authenticated index');
 assert.match(get('output').textContent,/deployed main/);
 get('disconnect').click();await flush();
 assert.equal(get('task-preview-labels').children.length,0);
 assert.equal(get('task-preview').hidden,true);
 console.log('PASS: one fixed host-owned pickString runs via index-only E2EE native GUI');
})().catch(error=>{console.error(error);process.exitCode=1});
