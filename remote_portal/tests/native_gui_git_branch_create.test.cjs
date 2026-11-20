'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const tick=String.fromCharCode(96);
const sections=raw.split('const trustedGUIJavaScript = '+tick);
assert.equal(sections.length,2);
const script=sections[1].split(tick)[0];
class Element{
 constructor(id){
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(kind,fn){this.listeners[kind]=fn}
 click(){assert.ok(this.listeners.click,'missing '+this.id);this.listeners.click()}
 append(...items){this.children.push(...items)}
 replaceChildren(){this.children=[]}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
}
const nodes=new Map();
const get=id=>{if(!nodes.has(id))nodes.set(id,new Element(id));return nodes.get(id)};
const token='0123456789abcdef'.repeat(4);
let remaining=16,confirmed=false,creates=0;
const fetch=async(path,options)=>{
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:false,
  task_run:false,task_long:false,git_status:true,git_branches:false,
  git_branch_create:true,git_diff:false,git_patch:false,
  git_pull:false,git_commit:false,git_push:false,git_fetch:false,
  git_stage_tracked:false,git_unstage_all:false,git_stage_file:false,
  git_unstage_file:false,terminal:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/git-branch-create'){
  assert.equal(options.body,undefined,'client selected a branch name or args');
  creates++;
  return {ok:true,json:async()=>({
   ok:true,branch:'remote-topic',head:'a'.repeat(40),
   remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected route '+path);
};
const sandbox={
 document:{getElementById:get,createElement:name=>new Element(name)},
 window:{addEventListener(){},confirm(){return confirmed}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,Promise,Date,Number,String,encodeURIComponent,
 setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
new vm.Script(script,{filename:'native-git-branch-create.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<100;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('git-branch-create').disabled,true,'mutation enabled before features');
 get('features').click();await flush();
 assert.equal(get('git-branch-create').disabled,false);
 assert.equal(get('git-push').disabled,true,'branch-create permission escalated push');
 assert.equal(get('git-pull').disabled,true,'branch-create permission escalated pull');
 get('git-branch-create').click();await flush();
 assert.equal(creates,0,'create bypassed user confirmation');
 confirmed=true;
 get('git-branch-create').click();await flush();
 assert.equal(creates,1);
 assert.match(get('output').textContent,/remote-topic/);
 assert.match(get('output').textContent,/HEAD hiện tại không đổi/);
 get('disconnect').click();await flush();
 assert.equal(get('git-branch-create').disabled,true);
 console.log('PASS: host-fixed branch creation confirmation, opt-in, no args and logout');
})().catch(err=>{console.error(err);process.exitCode=1});
