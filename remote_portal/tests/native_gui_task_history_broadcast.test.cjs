'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const tick=String.fromCharCode(96);
const parts=raw.split('const trustedGUIJavaScript = '+tick);
assert.equal(parts.length,2);
const anchor='  let inspectButtons=[];';
const instrument=[
 "  globalThis.__add=(id,kind,status,exit,output,truncated,report,label)=>{",
 "   const serial=rememberTaskRun(id,kind,status,exit,output,truncated);",
 "   const entry=taskRunHistory.find(x=>x.serial===serial);",
 "   entry.workflowReport=report;entry.label=label;renderTaskRunHistory();",
 "   return serial;",
 "  };",
 "  globalThis.__change=(serial,value)=>{",
 "   const entry=taskRunHistory.find(x=>x.serial===serial);",
 "   if(entry){entry.output=value;renderTaskRunHistory();}",
 "  };",""].join('\n');
const script=parts[1].split(tick)[0];
assert.ok(script.includes(anchor));
class Element{
 constructor(id){this.id=id;this.textContent='';this.value='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(type,fn){this.listeners[type]=fn}
 click(){assert.ok(this.listeners.click,'missing click '+this.id);this.listeners.click()}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
 append(...items){this.children.push(...items)}
 replaceChildren(...items){this.children=items}
}
const nodes=new Map();
const get=id=>{if(!nodes.has(id))nodes.set(id,new Element(id));return nodes.get(id)};
// Minimal test DOM must honor the static disabled attribute. The real
// browser parses this from the trusted HTML before the script starts.
assert.match(raw,/id="task-run-history-copy" disabled/);
get('task-run-history-copy').disabled=true;
const capability='0123456789abcdef'.repeat(4),copied=[],network=[];
const context={
 document:{getElementById:get,createElement:name=>new Element(name)},
 window:{confirm(){return true},addEventListener(){}},
 history:{replaceState(){}},location:{hash:'#'+capability,pathname:'/'},
 navigator:{clipboard:{writeText(value){copied.push(value);return Promise.resolve()}}},
 fetch:async endpoint=>{
  network.push(endpoint);
  if(endpoint==='/api/session')return {ok:true,json:async()=>({
   ok:true,closed:false,active:false,remaining:16,expires_in_seconds:300,
   device_id:'a'.repeat(64),username:'alice'
  })};
  if(endpoint==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
  throw Error('unexpected endpoint '+endpoint);
 },
 Date,Number,String,Promise,setInterval(){},setTimeout(){}
};
vm.createContext(context);
new vm.Script(script.replace(anchor,instrument+anchor),{filename:'task-history-broadcast.js'}).runInContext(context);
const flush=async()=>{for(let n=0;n<90;n++)await Promise.resolve()};
(async()=>{
 await flush();
 const button=get('task-run-history-copy');
 assert.equal(button.disabled,true);
 const first=context.__add(11,'ngắn','completed',0,'first: <script>',false,'','Build');
 context.__add(22,'dài','failed',9,'lỗi cuối: ế 😀',true,
  'Workflow #22\nBước 2: failed','Deploy');
 assert.equal(button.disabled,false);
 const before=network.length;
 button.click();await flush();
 assert.equal(network.length,before,'Copy all used an E2EE request');
 const body=copied.at(-1);
 assert.ok(body.indexOf('Task #11')<body.indexOf('Task #22'));
 assert.match(body,/Task #11 · Build · ngắn · completed · exit=0/);
 assert.match(body,/Task #22 · Deploy · dài · failed · exit=9/);
 assert.match(body,/\[Báo cáo workflow đã xác minh\]/);
 assert.match(body,/\[Console đã nhận\]\nlỗi cuối: ế 😀/);
 assert.match(body,/Output đã cắt/);
 assert.match(body,/<script>/);
 assert.equal(get('task-run-history-list').children.length,2);
 context.__change(first,'updated console');
 assert.equal(button.textContent,'Sao chép toàn bộ kết quả task');
 button.click();await flush();
 assert.match(copied.at(-1),/updated console/);
 assert.doesNotMatch(copied.at(-1),/<script>/);
 for(let id=30;id<37;id++)context.__add(id,'ngắn','completed',0,'log',false,'','');
 button.click();await flush();
 assert.equal((copied.at(-1).match(/Task #/g)||[]).length,6);
 get('task-run-history-clear').click();await flush();
 assert.equal(button.disabled,true);
 let n=copied.length;button.click();await flush();assert.equal(copied.length,n);
 context.__add(99,'ngắn','failed',4,'latest',false,'','');
 get('disconnect').click();await flush();
 assert.equal(button.disabled,true);
 n=copied.length;button.click();await flush();assert.equal(copied.length,n);
 assert.equal(get('task-run-history').hidden,true);
 console.log('PASS: bounded Task GUI session broadcast copy and logout');
})().catch(e=>{console.error(e);process.exitCode=1});
