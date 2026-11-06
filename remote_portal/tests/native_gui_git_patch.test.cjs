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
 click(){assert.ok(this.listeners.click,'missing handler '+this.id);this.listeners.click()}
 append(...elements){this.children.push(...elements)}
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
let remaining=16,starts=0,nexts=0,cachedStarts=0,cachedNexts=0;
const fetch=async(path,options)=>{
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 assert.equal(options.body,undefined,'diff page must never accept a selected file or offset');
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:false,
  task_run:false,task_long:false,git_status:true,git_diff:false,
  git_patch:true,git_log:false,git_branches:false,git_pull:false,
  git_commit:false,git_push:false,git_fetch:false,terminal:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/git-patch-start'){
  starts++;
  return {ok:true,json:async()=>({
   ok:true,page:1,patch:'diff --git a/a.txt b/a.txt\n',
   more:true,remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/git-patch-next'){
  nexts++;
  return {ok:true,json:async()=>({
   ok:true,page:2,patch:'@@ -1 +1 @@\n-old\n+new\n',
   more:false,remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/git-patch-cached-start'){
  cachedStarts++;
  return {ok:true,json:async()=>({
   ok:true,page:1,patch:'diff --git a/staged.txt b/staged.txt\\n',
   more:true,remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/git-patch-cached-next'){
  cachedNexts++;
  return {ok:true,json:async()=>({
   ok:true,page:2,patch:'@@ -1 +1 @@\\n-old\\n+staged\\n',
   more:false,remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected endpoint '+path);
};
const sandbox={
 document:{getElementById:get,createElement:name=>new Element(name)},
 window:{addEventListener(){},confirm(){return true}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,Promise,Date,Number,String,encodeURIComponent,
 setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
new vm.Script(script,{filename:'native-git-patch.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<100;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('git-patch-start').disabled,true,'read enabled before feature preflight');
 get('features').click();await flush();
 assert.equal(get('git-patch-start').disabled,false,'independent read flag hidden');
 assert.equal(get('git-patch-next').disabled,true,'next enabled without first page');
 assert.equal(get('git-pull').disabled,true,'read access escalated to mutation');
 get('git-patch-start').click();await flush();
 assert.equal(starts,1);
 assert.equal(get('git-patch-next').disabled,false,'page continuation disabled');
 assert.equal(get('git-patch-output').hidden,false);
 assert.equal(get('git-patch-output').textContent,'diff --git a/a.txt b/a.txt\n');
 get('git-patch-next').click();await flush();
 assert.equal(nexts,1);
 assert.equal(get('git-patch-next').disabled,true,'next enabled after final page');
 assert.match(get('git-patch-output').textContent,/\\+new/);
 assert.equal(get('git-patch-cached-start').disabled,false,
  'cached review should share read-only git.patch permission');
 get('git-patch-cached-start').click();await flush();
 assert.equal(cachedStarts,1);
 assert.equal(get('git-patch-output').textContent,'diff --git a/staged.txt b/staged.txt\\n',
  'switching mode must replace, not concatenate, prior HEAD diff');
 get('git-patch-next').click();await flush();
 assert.equal(cachedNexts,1,'next page must use staged-only route');
 assert.match(get('git-patch-output').textContent,/\\+staged/);
 assert.equal(get('git-patch-next').disabled,true);
 get('disconnect').click();await flush();
 assert.equal(get('git-patch-start').disabled,true,'read enabled after logout');
 assert.equal(get('git-patch-output').textContent,'','private patch retained after logout');
 assert.equal(get('git-patch-output').hidden,true,'private patch visible after logout');
 console.log('PASS: native Git patch page1/page2, opt-in, no args and logout cleanup');
})().catch(err=>{console.error(err);process.exitCode=1});
