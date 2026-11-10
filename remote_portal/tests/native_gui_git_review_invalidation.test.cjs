'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const match=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(match,'trusted native GUI JS missing');
class Element{
 constructor(id){this.id=id;this.textContent='';this.value='';this.hidden=false;this.disabled=false;
 this.children=[];this.listeners={};this.attributes={};this.classList={toggle(){}};}
 addEventListener(kind,fn){this.listeners[kind]=fn}
 click(){assert.ok(this.listeners.click,'missing click listener '+this.id);this.listeners.click()}
 append(...nodes){this.children.push(...nodes)}
 replaceChildren(){this.children=[]}
 setAttribute(name,value){this.attributes[name]=value}
 getAttribute(name){return this.attributes[name]||null}
}
const elements=new Map();
const get=id=>{if(!elements.has(id))elements.set(id,new Element(id));return elements.get(id)};
const token='0123456789abcdef'.repeat(4);
const shaA='a'.repeat(40),shaB='b'.repeat(40),shaC='c'.repeat(40);
let remaining=16,patchStarts=0,patchNexts=0;
const endpoints=[];
const reply=data=>({ok:true,json:async()=>data});
const fetch=async(path,options)=>{
 endpoints.push(path);
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 if(path==='/api/session')return reply({ok:true,closed:false,active:false,
  remaining,expires_in_seconds:300,device_id:'a'.repeat(64),username:'alice'});
 if(path==='/api/features')return reply({ok:true,
  session_ping:true,project_info:false,tasks_preview:false,task_run:false,
  task_long:false,git_status:true,git_branches:true,git_patch:true,
  git_stage_tracked:true,git_commit:true,git_pull:true,
  remote_control_available:false,remaining:--remaining});
 if(path==='/api/git-patch-start'){
  patchStarts++;
  return reply({ok:true,page:1,patch:'diff --git a/old b/old\\n',
   more:true,remote_control_available:false,remaining:--remaining});
 }
 if(path==='/api/git-patch-next'){
  patchNexts++;
  return reply({ok:true,page:2,patch:'@@ stale\\n',more:false,
   remote_control_available:false,remaining:--remaining});
 }
 if(path==='/api/git-stage-tracked')return reply({
  ok:true,operation:'git.stage_tracked',head:shaA,
  remote_control_available:false,remaining:--remaining});
 if(path==='/api/git-switch-review')return reply({
  ok:true,from:'main',branch:'review',head:shaA,
  remote_control_available:false,remaining:--remaining});
 if(path==='/api/git-branch-delete-review')return reply({
  ok:true,current:'main',branch:'taskdeck/old',head:shaA,
  remote_control_available:false,remaining:--remaining});
 if(path==='/api/git-commit'){
  assert.equal(options.headers['Content-Type'],'application/json');
  assert.equal(options.body,JSON.stringify({message:'Reviewed mutation'}));
  return reply({ok:true,before:shaA,after:shaB,
   remote_control_available:false,remaining:--remaining});
 }
 if(path==='/api/git-pull')return reply({
  ok:true,before:shaB,after:shaC,updated:true,
  remote_control_available:false,remaining:--remaining});
 if(path==='/api/close')return reply({ok:true,closed:true});
 throw Error('unexpected route '+path);
};
const scope={document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{confirm(){return true},addEventListener(){}},history:{replaceState(){}},
 location:{hash:'#'+token,pathname:'/'},fetch,
 setInterval(){},setTimeout(){},Date,Number,String,Promise,encodeURIComponent};
vm.createContext(scope);
new vm.Script(match[1],{filename:'native-review-invalidation.js'}).runInContext(scope);
const flush=async()=>{for(let i=0;i<80;i++)await Promise.resolve()};
const assertPatchCleared=()=>{
 assert.equal(get('git-patch-next').disabled,true,'stale diff Next still enabled');
 assert.equal(get('git-patch-output').hidden,true,'stale diff remains visible');
 assert.equal(get('git-patch-output').textContent,'','old HEAD/index patch retained');
 const before=endpoints.length;
 get('git-patch-next').click();
 assert.equal(endpoints.length,before,'synthetic click read stale diff continuation');
};
(async()=>{
 await flush();
 get('features').click();await flush();
 get('git-patch-start').click();await flush();
 assert.equal(get('git-patch-next').disabled,false);
 assert.equal(patchStarts,1);
 get('git-stage-tracked').click();await flush();
 assertPatchCleared();
 get('git-patch-start').click();await flush();
 assert.equal(get('git-patch-next').disabled,false);
 get('git-switch-review').click();await flush();
 get('git-branch-delete-review').click();await flush();
 assert.equal(get('git-switch-same-head').disabled,false);
 assert.equal(get('git-branch-delete').disabled,false);
 get('git-commit-message').value='Reviewed mutation';
 get('git-commit').click();await flush();
 assertPatchCleared();
 assert.equal(get('git-switch-same-head').disabled,true,'stale branch switch preview survived commit');
 assert.equal(get('git-branch-delete').disabled,true,'stale branch deletion preview survived commit');
 const beforeReview=endpoints.length;
 get('git-switch-same-head').click();
 get('git-branch-delete').click();
 assert.equal(endpoints.length,beforeReview,'synthetic old branch preview performed a write');
 get('git-patch-start').click();await flush();
 assert.equal(get('git-patch-next').disabled,false);
 get('git-pull').click();await flush();
 assertPatchCleared();
 assert.equal(patchNexts,0,'old HEAD diff was continued after mutation');
 assert.equal(patchStarts,3);
 get('disconnect').click();await flush();
 console.log('PASS: stage/commit/pull invalidate old patch and branch previews without extra E2EE calls');
})().catch(err=>{console.error(err);process.exitCode=1});
