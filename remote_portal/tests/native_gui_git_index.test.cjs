'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const tick=String.fromCharCode(96);
const sections=raw.split('const trustedGUIJavaScript = '+tick);
assert.equal(sections.length,2);
const script=sections[1].split(tick)[0];
assert.ok(script.length>1000);
class Element{
 constructor(id){
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(type,fn){this.listeners[type]=fn}
 click(){assert.ok(this.listeners.click,'missing '+this.id);this.listeners.click()}
 append(...items){this.children.push(...items)}
 replaceChildren(){this.children=[]}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
}
const nodes=new Map();
const get=id=>{
 if(!nodes.has(id))nodes.set(id,new Element(id));
 return nodes.get(id);
};
let remaining=16,confirmed=false,stageCalls=0,unstageCalls=0;
const token='0123456789abcdef'.repeat(4);
const fetch=async(path,options)=>{
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 assert.equal(options.body,undefined,'Git index mutation must not accept a request body');
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:false,
  task_run:false,task_long:false,git_status:true,git_pull:false,
  git_fetch:false,git_commit:false,git_push:false,
  git_stage_tracked:true,git_unstage_all:true,git_log:false,
  git_diff:false,terminal:false,remote_control_available:false,
  remaining:--remaining,
 })};
 if(path==='/api/git-stage-tracked'||path==='/api/git-unstage-all'){
  if(path==='/api/git-stage-tracked')stageCalls++;else unstageCalls++;
  return {ok:true,json:async()=>({
   ok:true,operation:path==='/api/git-stage-tracked'?'git.stage_tracked':'git.unstage_all',
   head:'a'.repeat(40),remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected endpoint '+path);
};
const sandbox={
 document:{getElementById:get,createElement:name=>new Element(name)},
 window:{addEventListener(){},confirm(){return confirmed}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,Promise,Date,Number,String,encodeURIComponent,setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
new vm.Script(script,{filename:'native-git-index.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<90;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('git-stage-tracked').disabled,true,'stage enabled before feature preflight');
 assert.equal(get('git-unstage-all').disabled,true,'unstage enabled before preflight');
 get('features').click();await flush();
 assert.equal(get('git-stage-tracked').disabled,false);
 assert.equal(get('git-unstage-all').disabled,false);
 assert.equal(get('git-fetch').disabled,true,'index opt-ins escalated Git fetch');
 assert.equal(get('git-pull').disabled,true,'index opt-ins escalated Git pull');
 get('git-stage-tracked').click();await flush();
 get('git-unstage-all').click();await flush();
 assert.equal(stageCalls,0,'stage bypassed user confirmation');
 assert.equal(unstageCalls,0,'unstage bypassed user confirmation');
 confirmed=true;
 get('git-stage-tracked').click();await flush();
 assert.equal(stageCalls,1,'stage request not sent exactly once');
 assert.match(get('output').textContent,/Đã stage thay đổi/);
 assert.equal(get('git-summary').hidden,true,'stale Git Status after stage');
 get('git-unstage-all').click();await flush();
 assert.equal(unstageCalls,1,'unstage request not sent exactly once');
 assert.match(get('output').textContent,/Đã unstage toàn bộ index/);
 get('disconnect').click();await flush();
 assert.equal(get('git-stage-tracked').disabled,true,'stage active after logout');
 assert.equal(get('git-unstage-all').disabled,true,'unstage active after logout');
 console.log('PASS: stage-tracked and unstage-all opt-in, confirmation, no args, logout');
})().catch(err=>{console.error(err);process.exitCode=1});
