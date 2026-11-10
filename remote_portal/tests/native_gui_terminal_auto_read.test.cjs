'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const js=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(js,'trusted GUI JavaScript is required');
class Element{
 constructor(id){this.id=id;this.value='';this.textContent='';this.hidden=false;
 this.disabled=false;this.children=[];this.listeners={};this.attributes={};this.classList={toggle(){}};}
 addEventListener(k,fn){this.listeners[k]=fn}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
 append(...items){this.children.push(...items)}
 replaceChildren(){this.children=[]}
 click(){assert.ok(this.listeners.click,'missing handler: '+this.id);this.listeners.click()}
}
const elements=new Map();
const get=id=>{if(!elements.has(id))elements.set(id,new Element(id));return elements.get(id)};
const token='0123456789abcdef'.repeat(4);
let remaining=16,polls=0;
const timers=[],sent=[];
const reply=data=>({ok:true,json:async()=>data});
const terminal=(state,output='',more=false)=>({
 ok:true,state,output,more,truncated:false,
 remote_control_available:false,remaining:--remaining
});
const fetch=async(endpoint,options)=>{
 sent.push(endpoint);
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 if(endpoint==='/api/session')return reply({ok:true,closed:false,active:false,
  remaining,expires_in_seconds:300,device_id:'a'.repeat(64),username:'alice'});
 if(endpoint==='/api/features')return reply({ok:true,
  session_ping:true,project_info:false,tasks_preview:false,task_run:false,
  task_long:false,git_status:false,terminal:true,
  remote_control_available:false,remaining:--remaining});
 if(endpoint==='/api/terminal-open')return reply(terminal('running'));
 if(endpoint==='/api/terminal-write')return reply(terminal('running'));
 if(endpoint==='/api/terminal-poll'){
  polls++;
  return reply(terminal('running',polls===1?'first':polls===2?'tail':'last',
    polls===1||polls===3));
 }
 if(endpoint==='/api/terminal-close')return reply(terminal('closed'));
 if(endpoint==='/api/close')return reply({ok:true,closed:true});
 throw Error('unrecognized route '+endpoint);
};
const sandbox={document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{confirm(){return true},addEventListener(){}},history:{replaceState(){}},
 location:{hash:'#'+token,pathname:'/'},fetch,setInterval(){},
 setTimeout(fn){timers.push(fn);return timers.length},Date,Number,String,Promise};
vm.createContext(sandbox);
new vm.Script(js[1],{filename:'trusted-terminal-auto.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<80;i++)await Promise.resolve()};
(async()=>{
 await flush();
 get('features').click();await flush();
 assert.equal(get('terminal-auto').textContent,'Bật tự nhận output');
 get('terminal-open').click();await flush();
 assert.equal(timers.length,0,'default manual mode silently consumed E2EE quota');
 assert.equal(get('terminal-poll').disabled,false);
 // Enabling later must consume the buffered one-shot Open hint.
 get('terminal-auto').click();
 assert.equal(get('terminal-auto').textContent,'Tắt tự nhận output');
 assert.equal(timers.length,1,'enabling did not schedule an initial output read');
 const oldTimer=timers.shift();
 get('terminal-auto').click(); // pause invalidates previously queued timer
 oldTimer();await flush();
 assert.equal(polls,0,'paused stale callback sent a Poll request');
 get('terminal-auto').click();
 assert.equal(timers.length,1);
 timers.shift()();await flush();
 assert.equal(polls,1);
 assert.equal(get('terminal-output').textContent,'first');
 assert.equal(timers.length,1,'more=true did not request next bounded output chunk');
 // Manual read wins over queued auto timer; no duplicate response.
 const duplicateTimer=timers.shift();
 get('terminal-poll').click();await flush();
 assert.equal(polls,2);
 duplicateTimer();await flush();
 assert.equal(polls,2,'stale auto timer duplicated manual Poll');
 assert.equal(timers.length,0,'drained output caused an unbounded idle loop');
 // A write triggers a new one-shot read; without another more=true
 // indication, auto-drain does not burn the remaining budget at idle.
 get('terminal-input').value='pwd';
 get('terminal-send').click();await flush();
 assert.equal(timers.length,1,'write did not re-arm opt-in drain');
 remaining=2; // reserve the last authenticated E2EE round for Close
 timers.shift()();await flush();
 assert.equal(polls,3);
 assert.equal(remaining,1);
 assert.equal(timers.length,0,'auto poll exhausted the mandatory Close reserve');
 assert.equal(get('terminal-poll').disabled,true);
 assert.equal(get('terminal-close').disabled,false);
 get('terminal-close').click();await flush();
 assert.equal(get('terminal-auto').textContent,'Tắt tự nhận output');
 assert.equal(get('terminal-poll').disabled,true);
 const beforeLogout=sent.length;
 get('disconnect').click();await flush();
 assert.equal(get('terminal-output').textContent,'');
 assert.ok(sent.length===beforeLogout+1,'logout did not revoke the session');
 console.log('PASS: opt-in Terminal auto drain, pause, stale timers, idle and Close reserve');
})().catch(err=>{console.error(err);process.exitCode=1});
