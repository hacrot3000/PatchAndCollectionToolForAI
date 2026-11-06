'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const tick=String.fromCharCode(96);
const parts=raw.split('const trustedGUIJavaScript = '+tick);
assert.equal(parts.length,2);
const script=parts[1].split(tick)[0];
class Element{
 constructor(id){
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(kind,fn){this.listeners[kind]=fn}
 click(){assert.ok(this.listeners.click,'missing handler '+this.id);this.listeners.click()}
 append(...items){this.children.push(...items)}
 replaceChildren(){this.children=[]}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
}
const elements=new Map();
const get=id=>{if(!elements.has(id))elements.set(id,new Element(id));return elements.get(id)};
const token='0123456789abcdef'.repeat(4);
let remaining=16,approved=false,calls=0;
const fetch=async(path,options)=>{
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 assert.equal(options.body,undefined,'switch must not accept client-selected ref, path or args');
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:false,
  git_status:true,git_branches:true,git_branch_create:false,
  git_pull:false,git_push:false,git_commit:false,git_fetch:false,
  task_run:false,task_long:false,terminal:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/git-switch-same-head'){
  calls++;
  return {ok:true,json:async()=>({
   ok:true,from:'feature/old',branch:'review',head:'a'.repeat(40),
   remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected route '+path);
};
const sandbox={
 document:{getElementById:get,createElement:name=>new Element(name)},
 window:{addEventListener(){},confirm(){return approved}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,Promise,Date,Number,String,encodeURIComponent,
 setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
new vm.Script(script,{filename:'native-git-switch.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<100;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('git-switch-same-head').disabled,true,
  'switch enabled before verified feature check');
 get('features').click();await flush();
 assert.equal(get('git-switch-same-head').disabled,false,
  'fixed switch visibility should use read-only branch capability');
 assert.equal(get('git-pull').disabled,true,'switch UX escalated Git Pull');
 assert.equal(get('git-push').disabled,true,'switch UX escalated Git Push');
 get('git-switch-same-head').click();await flush();
 assert.equal(calls,0,'branch switch bypassed user confirmation');
 approved=true;
 get('git-switch-same-head').click();await flush();
 assert.equal(calls,1,'the fixed branch switch route was not used once');
 assert.match(get('output').textContent,/feature\/old/);
 assert.match(get('output').textContent,/review/);
 assert.match(get('output').textContent,/Không fetch, reset/);
 get('disconnect').click();await flush();
 assert.equal(get('git-switch-same-head').disabled,true,'switch remained enabled after logout');
 console.log('PASS: same-HEAD switch host-fixed, user confirmed, no arguments, no Git escalation');
})().catch(err=>{console.error(err);process.exitCode=1});
