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
const elements=new Map();
const get=id=>{
 if(!elements.has(id))elements.set(id,new Element(id));
 return elements.get(id);
};
const token='0123456789abcdef'.repeat(4);
let remaining=16,starts=0,nexts=0;
const oid='a'.repeat(40);
const part1='GIT_TRACKING_V1\norigin/a '+oid+'\norigin/b '+oid+'\n';
const part2='origin/c '+oid+'\norigin/d '+oid+'\n';
const fetch=async(path,options)=>{
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 assert.equal(options.body,undefined,'tracking request carried client-selected ref/cursor');
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:false,
  task_run:false,task_long:false,git_status:true,
  git_pull:false,git_commit:false,git_push:false,git_fetch:false,
  git_stage_tracked:false,git_unstage_all:false,
  git_stage_file:false,git_unstage_file:false,
  git_log:false,git_diff:false,terminal:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/git-tracking-start')return {ok:true,json:async()=>{
  starts++;return {ok:true,page:1,refs:part1,more:true,remote_control_available:false,remaining:--remaining};
 }};
 if(path==='/api/git-tracking-next')return {ok:true,json:async()=>{
  nexts++;return {ok:true,page:2,refs:part2,more:false,remote_control_available:false,remaining:--remaining};
 }};
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected route '+path);
};
const sandbox={
 document:{getElementById:get,createElement:name=>new Element(name)},
 window:{addEventListener(){},confirm(){return true}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,Promise,Date,Number,String,encodeURIComponent,
 setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
new vm.Script(script,{filename:'native-git-tracking.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<90;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('git-tracking-start').disabled,true,'tracking read before features');
 assert.equal(get('git-tracking-next').disabled,true,'next available without initial snapshot');
 get('features').click();await flush();
 assert.equal(get('git-tracking-start').disabled,false,'git.status permission did not admit tracking');
 assert.equal(get('git-push').disabled,true,'tracking read escalated to git push');
 get('git-tracking-start').click();await flush();
 assert.equal(starts,1);
 assert.equal(get('git-tracking-next').disabled,false,'more page not enabled');
 assert.ok(get('output').textContent.startsWith(part1));
 get('git-tracking-next').click();await flush();
 assert.equal(nexts,1);
 assert.equal(get('output').textContent,part1+part2,'cross-page tracking refs corrupted');
 assert.equal(get('git-tracking-next').disabled,true,'exhausted page remained enabled');
 get('disconnect').click();await flush();
 assert.equal(get('git-tracking-start').disabled,true,'tracking read after logout');
 console.log('PASS: trusted Git tracking E2EE pages, no args, quota, logout');
})().catch(err=>{console.error(err);process.exitCode=1});
