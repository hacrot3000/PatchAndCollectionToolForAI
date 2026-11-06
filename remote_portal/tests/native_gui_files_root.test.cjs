'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const tick=String.fromCharCode(96);
const split=raw.split('const trustedGUIJavaScript = '+tick);
assert.equal(split.length,2);
const script=split[1].split(tick)[0];
class Element{
 constructor(id){
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(kind,cb){this.listeners[kind]=cb}
 click(){assert.ok(this.listeners.click,'handler missing: '+this.id);this.listeners.click()}
 append(...x){this.children.push(...x)}
 replaceChildren(){this.children=[]}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
}
const elements=new Map();
const get=id=>{if(!elements.has(id))elements.set(id,new Element(id));return elements.get(id)};
const token='0123456789abcdef'.repeat(4);
let remaining=16,starts=0,nexts=0,textStarts=0,textNexts=0;
const fetch=async(url,opt)=>{
 assert.equal(opt.method,'POST');
 assert.equal(opt.headers['X-TaskDeck-Local-Capability'],token);
 assert.equal(opt.body,undefined,'Files root pagination cannot transmit path or offset');
 if(url==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(url==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:false,project_info:false,files_root:true,tasks_preview:false,
  task_run:false,git_status:false,git_pull:false,git_commit:false,git_push:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(url==='/api/files-root'){
  starts++;
  return {ok:true,json:async()=>({
   ok:true,items:['D:docs','F:one.txt'],more:true,
   remote_control_available:false,remaining:--remaining,
  })};
 }
 if(url==='/api/files-root-next'){
  nexts++;
  return {ok:true,json:async()=>({
   ok:true,items:['F:two.txt'],more:false,
   remote_control_available:false,remaining:--remaining,
  })};
 }
 if(url==='/api/files-read-start'){
  textStarts++;
  return {ok:true,json:async()=>({
   ok:true,name:'notes.txt',page:1,text:'Host-approved text, ',more:true,
   remote_control_available:false,remaining:--remaining,
  })};
 }
 if(url==='/api/files-read-next'){
  textNexts++;
  return {ok:true,json:async()=>({
   ok:true,name:'notes.txt',page:2,text:'not a file chosen by PHP.',
   more:false,remote_control_available:false,remaining:--remaining,
  })};
 }
 if(url==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('Unexpected local route '+url);
};
const sandbox={
 document:{getElementById:get,createElement:n=>new Element(n)},
 window:{addEventListener(){},confirm(){return true}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,Promise,Date,Number,String,encodeURIComponent,
 setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
new vm.Script(script,{filename:'native-files-root.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<100;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('files-root').disabled,true,'Files read enabled before feature gate');
 get('features').click();await flush();
 assert.equal(get('files-root').disabled,false,'files.read opt-in hidden');
 assert.equal(get('files-root-next').disabled,true,'next accepted before first page');
 assert.equal(get('files-read-start').disabled,false,'file preview not available on read-only Files screen');
 assert.equal(get('files-read-next').disabled,true,'file continuation accepted before first page');
 get('nav-files').click();
 assert.equal(get('panel-files').hidden,false,'Files page not integrated in navigation');
 assert.equal(get('panel-project').hidden,true,'switching Files tab kept Project visible');
 get('files-root').click();await flush();
 assert.equal(starts,1);
 assert.equal(get('files-root-next').disabled,false,'next disabled while host has more');
 assert.match(get('files-root-output').textContent,/docs/);
 get('files-root-next').click();await flush();
 assert.equal(nexts,1);
 assert.equal(get('files-root-next').disabled,true,'terminal page can be replayed');
 assert.ok(get('files-root-output').textContent.includes('one.txt'));
 assert.ok(get('files-root-output').textContent.includes('two.txt'));
 get('files-read-start').click();await flush();
 assert.equal(textStarts,1,'fixed host file-read start not called');
 assert.equal(get('files-read-next').disabled,false,'next file page should be available');
 assert.ok(get('files-read-output').textContent.includes('Host-approved text, '));
 get('files-read-next').click();await flush();
 assert.equal(textNexts,1,'fixed host file-read next not called');
 assert.equal(get('files-read-next').disabled,true,'terminal read page is reusable');
 assert.ok(get('files-read-output').textContent.includes('not a file chosen by PHP.'));
 assert.ok(get('files-read-output').textContent.includes('notes.txt'));
 get('disconnect').click();await flush();
 assert.equal(get('files-root-output').textContent,'','Files names survived logout');
 assert.equal(get('files-root-output').hidden,true);
 assert.equal(get('files-read-output').textContent,'','Files contents survived logout');
 assert.equal(get('files-read-output').hidden,true,'File contents remained visible after logout');
 console.log('PASS: fixed E2EE Files root and text pages, no path, no write, logout scrub');
})().catch(err=>{console.error(err);process.exitCode=1});
