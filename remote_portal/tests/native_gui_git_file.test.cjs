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
 click(){assert.ok(this.listeners.click,'no handler '+this.id);this.listeners.click()}
 append(...items){this.children.push(...items)}
 replaceChildren(){this.children=[]}
 setAttribute(key,value){this.attributes[key]=value}
 getAttribute(key){return this.attributes[key]||null}
}
const elements=new Map();
const get=id=>{
 if(!elements.has(id))elements.set(id,new Element(id));
 return elements.get(id);
};
const first='## main\n M a.txt\n M b.txt\n';
const second='## main\nM  a.txt\n M b.txt\n';
const token='0123456789abcdef'.repeat(4);
let remaining=16,confirm=false,stage=0,unstage=0,reads=0;
const fetch=async(path,options)=>{
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:false,
  task_run:false,task_long:false,git_status:true,
  git_stage_file:true,git_unstage_file:true,
  git_stage_tracked:false,git_unstage_all:false,terminal:false,
  remote_control_available:false,remaining:--remaining,
 })};
 if(path==='/api/git-status'){
  reads++;
  return {ok:true,json:async()=>({
   ok:true,status:stage===0?first:second,remaining:--remaining,
  })};
 }
 if(path==='/api/git-stage-file'||path==='/api/git-unstage-file'){
  assert.equal(options.headers['Content-Type'],'application/json');
  const payload=JSON.parse(options.body);
  assert.deepEqual(Object.keys(payload),['index','snapshot']);
  assert.equal(payload.index,1);
  assert.equal(payload.snapshot,path==='/api/git-stage-file'?first:second);
  if(path==='/api/git-stage-file')stage++;else unstage++;
  return {ok:true,json:async()=>({
   ok:true,operation:path==='/api/git-stage-file'?'git.stage_file':'git.unstage_file',
   head:'a'.repeat(40),remote_control_available:false,remaining:--remaining,
  })};
 }
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected endpoint '+path);
};
const sandbox={
 document:{getElementById:get,createElement:name=>new Element(name)},
 window:{addEventListener(){},confirm(){return confirm}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,Promise,Date,Number,String,encodeURIComponent,
 setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
new vm.Script(script,{filename:'native-git-file.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<90;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('git-stage-file').disabled,true);
 get('features').click();await flush();
 assert.equal(get('git-stage-file').disabled,true,'per-file stage enabled without status snapshot');
 get('git').click();await flush();
 assert.equal(reads,1);
 assert.equal(get('git-file-select').children.length,3);
 assert.equal(get('git-stage-file').disabled,true,'per-file stage enabled without row selection');
 get('git-file-select').value='1';
 get('git-file-select').listeners.change();await flush();
 assert.equal(get('git-stage-file').disabled,false);
 assert.equal(get('git-unstage-file').disabled,true,'unstage enabled for an unstaged row');
 get('git-stage-file').click();await flush();
 assert.equal(stage,0,'stage ignored user confirmation');
 confirm=true;
 get('git-stage-file').click();await flush();
 assert.equal(stage,1);
 assert.equal(get('git-file-select').value,'','stale file index retained after stage');
 assert.equal(get('git-stage-file').disabled,true);
 assert.equal(get('git-summary').hidden,true,'stale Git status remained visible');
 get('git').click();await flush();
 assert.equal(reads,2);
 get('git-file-select').value='1';
 get('git-file-select').listeners.change();await flush();
 assert.equal(get('git-unstage-file').disabled,false);
 get('git-unstage-file').click();await flush();
 assert.equal(unstage,1);
 assert.match(get('output').textContent,/Đã unstage đúng file/);
 get('disconnect').click();await flush();
 assert.equal(get('git-stage-file').disabled,true);
 assert.equal(get('git-unstage-file').disabled,true);
 console.log('PASS: Git file snapshot-bound stage, unstage, confirmation and logout');
})().catch(err=>{console.error(err);process.exitCode=1});
