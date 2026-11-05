'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const tick=String.fromCharCode(96);
const chunks=raw.split('const trustedGUIJavaScript = '+tick);
assert.equal(chunks.length,2,'trusted native GUI JavaScript absent');
const js=chunks[1].split(tick)[0];
assert.ok(js.length>1000,'native bundled GUI code truncated');
class Element{
 constructor(id){
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(type,cb){this.listeners[type]=cb}
 click(){assert.ok(this.listeners.click,'no click handler for '+this.id);this.listeners.click()}
 append(...children){this.children.push(...children)}
 replaceChildren(){this.children=[]}
 setAttribute(key,value){this.attributes[key]=value}
 getAttribute(key){return this.attributes[key]||null}
}
const elems=new Map();
const get=id=>{
 if(!elems.has(id))elems.set(id,new Element(id));
 return elems.get(id);
};
let remaining=16,confirmed=false,pushes=0;
const token='0123456789abcdef'.repeat(4);
const requests=[];
const fetch=async (path,options)=>{
 requests.push(path);
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 assert.equal(options.body,undefined,'git.push must not carry arguments');
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:false,
  task_run:false,task_long:false,git_status:true,
  git_pull:false,git_commit:false,git_push:true,
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/git-push'){
  pushes++;
  return {ok:true,json:async()=>({
   ok:true,head:'b'.repeat(40),
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
 fetch,Promise,Date,Number,String,encodeURIComponent,setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
new vm.Script(js,{filename:'native-git-push.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<80;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('git-push').disabled,true,'push enabled before host opt-in');
 get('features').click();await flush();
 assert.equal(get('git-push').disabled,false,'host-enabled GitPush unavailable');
 assert.equal(get('git-pull').disabled,true,'feature flag unintentionally enabled pull');
 assert.equal(get('git-commit').disabled,true,'feature flag unintentionally enabled commit');
 get('git-push').click();await flush();
 assert.equal(pushes,0,'push executed despite rejected confirmation');
 confirmed=true;
 get('git-push').click();await flush();
 assert.equal(pushes,1,'push was not sent exactly once');
 assert.equal(get('git-summary').hidden,true,'stale status retained after push');
 assert.match(get('output').textContent,/Git push đã thành công trên upstream/);
 get('disconnect').click();await flush();
 assert.equal(get('git-push').disabled,true,'push enabled after logout');
 console.log('PASS: explicit no-force Git push confirmation, host feature gate and status invalidation');
})().catch(err=>{console.error(err);process.exitCode=1});
