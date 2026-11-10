'use strict';
// Run the EXACT shipped native GUI JS. Git pull changes the local HEAD,
// so a previously authenticated Task.ID and prompt draft must not be used
// for a subsequent run unless the host issues a NEW tasks preview.
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const script=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(script);
class Element{
 constructor(id){this.id=id;this.value='';this.textContent='';this.hidden=false;this.disabled=false;this.children=[];this.listeners={};this.attributes={};this.classList={toggle(){}};}
 addEventListener(k,fn){this.listeners[k]=fn}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
 append(...items){this.children.push(...items)}
 replaceChildren(){this.children=[]}
 click(){assert.ok(this.listeners.click,'missing click '+this.id);this.listeners.click()}
}
const elements=new Map();
const get=id=>{if(!elements.has(id))elements.set(id,new Element(id));return elements.get(id)};
const token='0123456789abcdef'.repeat(4);
let remaining=16,previewVersion=0,pullUpdated=true;
const calls=[];
const reply=data=>({ok:true,json:async()=>data});
const sha='a'.repeat(40),shaAfter='b'.repeat(40);
const fetch=async(path,options)=>{
 calls.push(path);
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 if(path==='/api/session')return reply({ok:true,closed:false,active:false,
  remaining,expires_in_seconds:300,device_id:'a'.repeat(64),username:'alice'});
 if(path==='/api/features')return reply({ok:true,session_ping:true,project_info:false,
  tasks_preview:true,task_run:true,task_long:false,git_status:true,git_pull:true,
  remote_control_available:false,remaining:--remaining});
 if(path==='/api/tasks-preview')return reply({ok:true,total:1,
  labels:previewVersion++===0?['OLD task']:['NEW task'],
  ids:previewVersion===1?[55]:[66],remaining:--remaining});
 if(path==='/api/tasks-inspect/55'||path==='/api/tasks-inspect/66')return reply({
  ok:true,task_id:Number(path.slice('/api/tasks-inspect/'.length)),
  eligible:true,input_id:'tag',input_type:'promptString',choices:[],
  remote_control_available:false,remaining:--remaining});
 if(path==='/api/git-pull')return reply({ok:true,before:sha,
  after:pullUpdated?shaAfter:sha,updated:pullUpdated,
  remote_control_available:false,remaining:--remaining});
 if(path.startsWith('/api/tasks-run'))throw Error('stale task was executed: '+path);
 if(path==='/api/close')return reply({ok:true,closed:true});
 throw Error('unexpected route '+path);
};
const scope={document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{confirm(){return true},addEventListener(){}},history:{replaceState(){}},
 location:{hash:'#'+token,pathname:'/'},fetch,Date,Number,String,Promise,
 setInterval(){},setTimeout(){},encodeURIComponent};
vm.createContext(scope);
new vm.Script(script[1],{filename:'native-git-pull-task-snapshot.js'}).runInContext(scope);
const flush=async()=>{for(let i=0;i<90;i++)await Promise.resolve()};
(async()=>{
 await flush();get('features').click();await flush();
 get('tasks-preview').click();await flush();
 let oldRow=get('task-preview-labels').children[0];
 assert.match(oldRow.textContent,/OLD task/);
 oldRow.children[1].click();await flush();
 oldRow=get('task-preview-labels').children[0];
 assert.equal(oldRow.children[4].disabled,false,'host-inspected old task was not runnable');
 oldRow.children[3].value='release-1';
 oldRow.children[3].listeners.input();
 const callsBefore=calls.length;
 get('git-pull').click();await flush();
 assert.equal(calls.length,callsBefore+1,'fixed Git Pull endpoint not invoked');
 assert.equal(get('task-preview').hidden,true,'old task list still visible after Pull');
 assert.equal(get('task-preview-labels').children.length,0,'old Task.ID remained in DOM');
 assert.equal(get('task-deps-select').children.length,0,'old dependency list survived Pull');
 assert.equal(get('task-run-history').hidden,true,'an empty history was unexpectedly changed');
 const afterPull=calls.length;
 oldRow.children[4].click();
 oldRow.children[1].click();
 await flush();
 assert.equal(calls.length,afterPull,
  'detached old task controls sent requests after Git pull');
 get('tasks-preview').click();await flush();
 const freshRow=get('task-preview-labels').children[0];
 assert.match(freshRow.textContent,/NEW task/);
 assert.equal(freshRow.children[2].disabled,true,'fresh host Task.ID was runnable before inspect');
 // The previous row remains detached; its callback cannot run just
 // because the user has now loaded a different Task.ID from the Agent.
 oldRow.children[4].click();oldRow.children[1].click();await flush();
 assert.equal(calls.filter(x=>x.startsWith('/api/tasks-run')).length,0);
 freshRow.children[1].click();await flush();
 const freshInput=get('task-preview-labels').children[0].children[3];
 assert.equal(freshInput.value,'','old prompt draft was restored for new Task.ID');
 assert.equal(get('task-preview-labels').children[0].children[4].disabled,false);
 assert.equal(calls.filter(x=>x.startsWith('/api/tasks-run')).length,0);
 console.log('PASS: Git pull invalidates stale E2EE tasks without executing or persisting inputs');
})().catch(err=>{console.error(err);process.exitCode=1});
