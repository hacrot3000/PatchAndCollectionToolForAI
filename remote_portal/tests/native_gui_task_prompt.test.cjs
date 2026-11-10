'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const source=fs.readFileSync(
 'vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const script=source.match(/const trustedGUIJavaScript = \`([\s\S]*?)\`/);
assert.ok(script,'trusted UI must bundle native JavaScript');
class Element{
 constructor(id){
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};
  this.attributes={};this.classList={toggle(){}};
 }
 addEventListener(event,callback){this.listeners[event]=callback}
 setAttribute(key,val){this.attributes[key]=val}
 getAttribute(key){return this.attributes[key]||null}
 append(...children){this.children.push(...children)}
 replaceChildren(){this.children=[]}
 click(){assert.ok(this.listeners.click,'missing click '+this.id);this.listeners.click()}
}
const elements=new Map(),get=id=>{
 if(!elements.has(id))elements.set(id,new Element(id));
 return elements.get(id);
};
const token='0123456789abcdef'.repeat(4),sent=[];
let remaining=16,confirmNext=true;
const fetch=async(endpoint,options)=>{
 sent.push({endpoint,options});
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
  ok:true,task_id:123,eligible:true,input_id:'version',
  input_type:'promptString',choices:[],remote_control_available:false,
  remaining:--remaining,
 })};
 if(endpoint==='/api/tasks-run-input/123'){
  assert.equal(options.headers['Content-Type'],'application/json');
  assert.equal(options.body,JSON.stringify({value:'release-1.2.3'}));
  assert.ok(!endpoint.includes('release-1.2.3'),
   'raw input value leaked into localhost URL');
  return {ok:true,json:async()=>({
   ok:true,task_id:123,exit_code:0,output:'completed',
   truncated:false,remote_control_available:false,remaining:--remaining,
  })};
 }
 if(endpoint==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected native-only endpoint: '+endpoint);
};
const sandbox={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{confirm(){return confirmNext},addEventListener(){}},history:{replaceState(){}},
 location:{hash:'#'+token,pathname:'/'},fetch,
 setInterval(){},Date,Number,String,Promise,
};
vm.createContext(sandbox);
new vm.Script(script[1],{filename:'trusted-native-prompt.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<40;i++)await Promise.resolve()};
(async()=>{
 await flush();
 get('features').click();await flush();
 get('tasks-preview').click();await flush();
 let row=get('task-preview-labels').children[0];
 assert.equal(row.children[2].disabled,true,
  'task must be inspected before any process can start');
 row.children[1].click();await flush();
 row=get('task-preview-labels').children[0];
 assert.equal(row.children.length,5);
 const input=row.children[3],run=row.children[4];
 assert.equal(input.type,'text');
 assert.equal(input.maxLength,64);
 assert.equal(run.disabled,false);
 input.value='bad;command';
 run.click();await flush();
 assert.equal(sent.filter(r=>r.endpoint.startsWith('/api/tasks-run-input/')).length,0,
  'shell metacharacters must fail in the native GUI before Relay');
 input.value='release-1.2.3';
 input.listeners.input();
 confirmNext=false;
 run.click();await flush();
 assert.equal(sent.filter(r=>r.endpoint.startsWith('/api/tasks-run-input/')).length,0,
  'declined prompt confirmation must not execute any task');
 assert.equal(input.value,'release-1.2.3',
  'declined confirmation must preserve editable prompt');
 confirmNext=true;
 run.click();
 assert.equal(input.value,'',
  'confirmed prompt was retained in the active input element');
 await flush();
 assert.equal(sent.filter(r=>r.endpoint.startsWith('/api/tasks-run-input/')).length,1);
 assert.match(get('output').textContent,/completed/);
 // Rebuilding the task page must not restore a submitted prompt from
 // cached drafts. The chosen host-owned task ID and inspection persist.
 get('task-filter').value='deploy';
 get('task-filter').listeners.input();
 const refreshed=get('task-preview-labels').children[0];
 assert.equal(refreshed.children[3].value,'',
  'confirmed prompt was resurrected from local draft after rerender');
 assert.equal(refreshed.children[4].disabled,false,
  'scrubbing submitted token incorrectly removed authenticated Task.ID');
 get('disconnect').click();await flush();
 assert.equal(get('task-preview-labels').children.length,0);
 assert.equal(get('task-preview').hidden,true);
 console.log('PASS: bounded native promptString uses POST body and authenticated E2EE only');
})().catch(error=>{console.error(error);process.exitCode=1});
