'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const tick=String.fromCharCode(96);
const scripts=raw.split('const trustedGUIJavaScript = '+tick);
assert.equal(scripts.length,2);
const script=scripts[1].split(tick)[0];
class Element{
 constructor(id){
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(kind,fn){this.listeners[kind]=fn}
 click(){assert.ok(this.listeners.click,'missing click handler: '+this.id);this.listeners.click()}
 append(...items){this.children.push(...items)}
 replaceChildren(){this.children=[]}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
}
const nodes=new Map();
const get=id=>{if(!nodes.has(id))nodes.set(id,new Element(id));return nodes.get(id)};
const token='0123456789abcdef'.repeat(4);
let remaining=16,confirmed=false,previews=0,deletes=0;
const fetch=async(path,opts)=>{
 assert.equal(opts.method,'POST');
 assert.equal(opts.headers['X-TaskDeck-Local-Capability'],token);
 assert.equal(opts.body,undefined,'client selected a Git ref or branch deletion arguments');
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,files_root:false,
  tasks_preview:false,task_run:false,task_long:false,
  git_status:true,git_branches:true,git_branch_create:false,
  git_pull:false,git_commit:false,git_push:false,git_fetch:false,
  terminal:false,remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/git-branch-delete-review'){
  previews++;
  return {ok:true,json:async()=>({
   ok:true,current:'feature/old',branch:'taskdeck/safe-target',
   head:'a'.repeat(40),remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/git-branch-delete'){
  deletes++;
  return {ok:true,json:async()=>({
   ok:true,current:'feature/old',branch:'taskdeck/safe-target',
   head:'a'.repeat(40),remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected local native route '+path);
};
const context={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{addEventListener(){},confirm(){return confirmed}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,Promise,Date,Number,String,encodeURIComponent,
 setInterval(){},setTimeout(){},
};
vm.createContext(context);
new vm.Script(script,{filename:'remote-git-delete-gui.js'}).runInContext(context);
const flush=async()=>{for(let i=0;i<100;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('git-branch-delete-review').disabled,true,'review available before feature authorization');
 get('features').click();await flush();
 assert.equal(get('git-branch-delete-review').disabled,false,
  'read-only branch inventory feature did not expose preview');
 assert.equal(get('git-branch-delete').disabled,true,
  'mutation enabled without authenticated and inspected preview');
 assert.equal(get('git-pull').disabled,true,'branch preview escalated Pull permission');
 assert.equal(get('git-push').disabled,true,'branch preview escalated Push permission');
 get('git-branch-delete-review').click();await flush();
 assert.equal(previews,1,'review action missing');
 assert.match(get('output').textContent,/taskdeck\/safe-target/);
 assert.match(get('output').textContent,/feature\/old/);
 assert.equal(get('git-branch-delete').disabled,false,'confirmed preview not displayed');
 get('git-branch-delete').click();await flush();
 assert.equal(deletes,0,'destructive action skipped native user confirmation');
 confirmed=true;
 get('git-branch-delete').click();await flush();
 assert.equal(deletes,1,'explicit confirmed deletion not submitted');
 assert.match(get('output').textContent,/Đã xoá nhánh/);
 assert.equal(get('git-branch-delete').disabled,true,'deleted plan could be replayed');
 get('disconnect').click();await flush();
 assert.equal(get('git-branch-delete-review').disabled,true);
 assert.equal(get('git-branch-delete').disabled,true);
 console.log('PASS: audited TaskDeck branch delete preview, named confirmation, no arguments, no escalation');
})().catch(e=>{console.error(e);process.exitCode=1});
