'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const js=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(js);
class Element{
 constructor(id){this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(name,fn){this.listeners[name]=fn}
 click(){assert.ok(this.listeners.click,'missing '+this.id);this.listeners.click()}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
 append(...nodes){this.children.push(...nodes)}
 replaceChildren(...nodes){this.children=nodes}
}
const elements=new Map();
const get=id=>{if(!elements.has(id))elements.set(id,new Element(id));return elements.get(id)};
const token='0123456789abcdef'.repeat(4),timers=[],requests=[];
let remaining=16,polls=0;
const responses=[
 ['running','',false],['running','',false],
 ['running','',false],['running','NEW-COMPILER-OUTPUT',false],
 ['running','',false],['running','',false],
 ['running','',false],['completed','FINAL-DIAGNOSTIC',false],
];
const fetch=async(endpoint,opts)=>{
 requests.push(endpoint);
 assert.equal(opts.headers['X-TaskDeck-Local-Capability'],token);
 if(endpoint==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice'
 })};
 if(endpoint==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:true,
  task_run:true,task_long:true,git_status:false,
  remote_control_available:false,remaining:--remaining
 })};
 if(endpoint==='/api/tasks-preview')return {ok:true,json:async()=>({
  ok:true,total:1,labels:['Quiet build'],ids:[71],remaining:--remaining
 })};
 if(endpoint==='/api/tasks-inspect/71')return {ok:true,json:async()=>({
  ok:true,task_id:71,eligible:true,input_id:'',choices:[],
  input_type:'',input_id_2:'',choices_2:[],
  remote_control_available:false,remaining:--remaining
 })};
 if(endpoint==='/api/tasks-start/71')return {ok:true,json:async()=>({
  ok:true,task_id:71,status:'started',exit_code:-1,output:'',
  truncated:false,more:false,remote_control_available:false,remaining:--remaining
 })};
 if(endpoint==='/api/tasks-poll/71'){
  const [status,output,more]=responses[polls++];
  if(!status)throw Error('auto poll attempted too many times');
  return {ok:true,json:async()=>({
   ok:true,task_id:71,status,exit_code:status==='completed'?0:-1,
   output,more,truncated:false,remote_control_available:false,
   remaining:--remaining
  })};
 }
 if(endpoint==='/api/tasks-clear/71')return {ok:true,json:async()=>({
  ok:true,task_id:71,status:'cleared',exit_code:-1,output:'',
  more:false,truncated:false,remote_control_available:false,remaining:--remaining
 })};
 if(endpoint==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected endpoint '+endpoint);
};
const sandbox={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{confirm(){return true},addEventListener(){}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,setInterval(){},setTimeout(fn,delay){timers.push({fn,delay});return timers.length;},
 Date,Number,String,Promise
};
vm.createContext(sandbox);
new vm.Script(js[1],{filename:'native-task-idle-backoff.js'}).runInContext(sandbox);
const flush=async()=>{for(let n=0;n<65;n++)await Promise.resolve()};
(async()=>{
 await flush();
 get('features').click();await flush();
 get('tasks-preview').click();await flush();
 let row=get('task-preview-labels').children[0];
 row.children[1].click();await flush();
 row=get('task-preview-labels').children[0];
 row.children[3].click();await flush();
 assert.equal(timers.length,1);
 const delays=[];
 for(let i=0;i<responses.length;i++){
  assert.equal(timers.length,1,'silent poll failed to rearm');
  const timer=timers.shift();
  delays.push(timer.delay);
  timer.fn();await flush();
  if(i<responses.length-1){
   assert.equal(get('long-task-controls').hidden,false,
    'running job was detached before terminal state');
  }
 }
 assert.deepEqual(delays,[450,450,900,1800,450,900,1800,3600],
  'backoff should reset on new output, not spend quota at 450ms forever');
 assert.equal(polls,8);
 assert.equal(timers.length,0,'terminal reply must stop automatic Poll');
 assert.match(get('long-task-output').textContent,/FINAL-DIAGNOSTIC/);
 assert.equal(get('long-task-clear').disabled,false);
 const pollsBefore=requests.filter(x=>x.startsWith('/api/tasks-poll/')).length;
 get('long-task-clear').click();await flush();
 assert.equal(requests.filter(x=>x.startsWith('/api/tasks-poll/')).length,pollsBefore);
 get('disconnect').click();await flush();
 console.log('PASS: quiet native job auto-poll backs off and resets on progress');
})().catch(e=>{console.error(e);process.exitCode=1});
