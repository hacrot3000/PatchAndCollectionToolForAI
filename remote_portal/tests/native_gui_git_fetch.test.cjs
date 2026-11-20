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
 addEventListener(type,callback){this.listeners[type]=callback}
 click(){assert.ok(this.listeners.click,'no handler for '+this.id);this.listeners.click()}
 append(...children){this.children.push(...children)}
 replaceChildren(){this.children=[]}
 setAttribute(key,val){this.attributes[key]=val}
 getAttribute(key){return this.attributes[key]||null}
}
const elems=new Map();
const get=id=>{
 if(!elems.has(id))elems.set(id,new Element(id));
 return elems.get(id);
};
let remaining=16,confirmed=false,fetches=0,trackingStarts=0,trackingNexts=0;
const token='0123456789abcdef'.repeat(4);
const request=async(path,options)=>{
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 assert.equal(options.body,undefined,'git.fetch must have no request body');
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:false,
  task_run:false,task_long:false,git_status:true,git_pull:false,
  git_commit:false,git_push:false,git_fetch:true,
  git_log:false,git_diff:false,terminal:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/git-tracking-start'){
  trackingStarts++;
  return {ok:true,json:async()=>({
   ok:true,page:1,refs:'GIT_TRACKING_V1\norigin/main '+('a'.repeat(40))+'\n',
   more:true,remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/git-tracking-next'){
  trackingNexts++;
  throw Error('stale tracking continuation was submitted after Fetch');
 }
 if(path==='/api/git-fetch'){
  fetches++;
  return {ok:true,json:async()=>({
   ok:true,head:'a'.repeat(40),before:'a'.repeat(40),
   after:'b'.repeat(40),updated:true,
   remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected authenticated endpoint '+path);
};
const sandbox={
 document:{getElementById:get,createElement:name=>new Element(name)},
 window:{addEventListener(){},confirm(){return confirmed}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch:request,Promise,Date,Number,String,encodeURIComponent,
 setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
new vm.Script(script,{filename:'native-git-fetch.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<80;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('git-fetch').disabled,true,'fetch enabled before feature read');
 get('features').click();await flush();
 assert.equal(get('git-fetch').disabled,false,'host allowed GitFetch missing');
 assert.equal(get('git-pull').disabled,true,'fetch opt-in implicitly enabled pull');
 assert.equal(get('git-push').disabled,true,'fetch opt-in implicitly enabled push');
 get('git-tracking-start').click();await flush();
 assert.equal(trackingStarts,1);
 assert.equal(get('git-tracking-next').disabled,false,'host tracking page not available');
 get('git-fetch').click();await flush();
 assert.equal(fetches,0,'fetch ignored explicit confirmation');
 confirmed=true;
 get('git-fetch').click();await flush();
 assert.equal(fetches,1,'fetch request did not execute once');
 assert.match(get('output').textContent,/Đã fetch upstream/);
 assert.match(get('output').textContent,/HEAD cục bộ vẫn là/);
 assert.equal(get('git-summary').hidden,true,'stale Git Status retained');
 assert.equal(get('git-tracking-next').disabled,true,
  'Fetch retained a stale remote tracking cursor');
 get('git-tracking-next').click();await flush();
 assert.equal(trackingNexts,0,'retired tracking callback consumed E2EE quota');
 get('git-tracking-start').click();await flush();
 assert.equal(trackingStarts,2,'a fresh host tracking snapshot was not available');
 get('disconnect').click();await flush();
 assert.equal(get('git-fetch').disabled,true,'fetch remained enabled after logout');
 console.log('PASS: bounded GitFetch GUI confirmation, flag isolation, no args and logout');
})().catch(error=>{console.error(error);process.exitCode=1});
