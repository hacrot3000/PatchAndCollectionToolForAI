const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const source=fs.readFileSync(path.join(__dirname,'web/featuremods/smartterminal_trace.js'),'utf8');
const terminalSource=fs.readFileSync(path.join(__dirname,'web/featuremods/smartterminal.js'),'utf8');
const next=fs.readFileSync(path.join(__dirname,'web/featuremods/next.js'),'utf8');
function node(tag){
 const listeners=new Map();
 const x={tag,children:[],className:'',style:{},dataset:{},isConnected:true,textContent:'',listeners,
  append(...items){this.children.push(...items);},
  replaceChildren(...items){this.children=items;},
  setAttribute(name,value){this[name]=String(value);},
  addEventListener(type,fn){listeners.set(type,fn);},
  before(item){this.beforeItems.push(item);},
  beforeItems:[],
  focus(){},
  remove(){}
 };
 x.classList={
  contains(name){return x.className.split(' ').includes(name);},
  add(name){if(!this.contains(name))x.className+=' '+name;},
  remove(name){x.className=x.className.split(' ').filter(k=>k!==name).join(' ');}
 };
 return x;
}
function fixture({networkError=false}={}){
 const view={meta:{id:'local-1',kind:'terminal',task_id:0,status:'running',target_type:'local'},
  closed:false,canControl:true,tabReadOnly:false};
 const output=[],requests=[],listeners=new Map(),reload=node('button');
 const app={active:'local-1',views:new Map([['local-1',view]]),browserLeaseLost:false,
  jsonFetch:async url=>{
   requests.push(String(url));
   if(networkError)throw Error('HTTP 403 forbidden');
   return {cwd:'/workspace/private',entries:[{name:'a_python_patch',dir:true},{name:'a_python_patch_ok',dir:true}]};
  }};
 const doc={body:node('body'),head:node('head'),activeElement:node('input'),
  createElement:node,querySelector:selector=>selector==='#reload'?reload:null};
 const smart={diagnose:()=>({analysis:{line:'cp a_py',suggestionReady:false,
  pathContext:{kind:'paths',dir:'',prefix:'a_py'},popupVisible:false},
  shell:{seenPrompt:false,executing:false},buffer:{cursorX:62,rowWrapped:false}})};
 const context={TaskMenuApp:app,TaskDeckSmartTerminal:smart,
  TaskDeckCompletionEngine:{pathContext:text=>/^cp [a-z_]+$/.test(text)?{kind:'paths',dir:'',prefix:text.slice(3)}:null},document:doc,
  navigator:{clipboard:{writeText:async text=>output.push(text)}},
  window:{addEventListener:(name,fn)=>listeners.set(name,fn)},
  URLSearchParams,URL,Blob,Date,console,setTimeout,clearTimeout};
 context.globalThis=context;vm.createContext(context);
 vm.runInContext(source,context,{filename:'smartterminal_trace.js'});
 return {api:context.TaskDeckCompletionTrace,context,view,app,doc,requests,listeners,output};
}
test('module is loaded before smartterminal and exports diagnostic probe',()=>{
 assert.ok(next.indexOf('smartterminal_trace.js')>0);
 assert.ok(next.indexOf('smartterminal_trace.js')<next.indexOf("smartterminal.js"));
 assert.match(terminalSource,/function diagnose\(view\)/);
 assert.match(terminalSource,/onInput,hide,diagnose/);
 const h=fixture();assert.ok(h.api.open);assert.ok(h.api.probe);assert.ok(h.api.report);
 assert.equal(h.api.enabled,false);
});
test('legacy single-user kind-less local terminal is eligible for trace',()=>{
 const h=fixture();h.view.meta.kind='';
 assert.equal(h.api.localTerminal(h.view),true);
 assert.equal(h.api.start(),true);
 assert.equal(h.api.target,'local-1');
});
test('header and editor activation do not prevent trace of an existing local terminal',()=>{
 const h=fixture();h.view.meta.kind='';
 h.app.active='external:editor-tab';
 h.api.open();
 assert.equal(h.api.activeTerminal(),h.view);
 assert.equal(h.api.start(),true);
 const picker=h.doc.body.children[0].children[0].children[3].children[0];
 assert.equal(picker.value,'local-1');
});
test('terminal selector can target a second local shell, but excludes SSH, task and Patch Tool',()=>{
 const h=fixture();
 h.app.views.set('local-2',{meta:{id:'local-2',kind:'',task_id:0,status:'running',label:'Another local shell'},closed:false});
 h.app.views.set('ssh',{meta:{id:'ssh',kind:'terminal',task_id:0,target_type:'ssh'},closed:false});
 h.app.views.set('task',{meta:{id:'task',kind:'task',task_id:7},closed:false});
 h.app.views.set('patch',{meta:{id:'patch',kind:'patch',task_id:0},closed:false});
 h.api.open();
 const picker=h.doc.body.children[0].children[0].children[3].children[0];
 assert.deepEqual(picker.children.map(option=>option.value),['local-1','local-2']);
 picker.value='local-2';
 assert.equal(h.api.start(),true);
 assert.equal(h.api.target,'local-2');
});
test('legacy unowned compatibility is disabled in shared-server mode',()=>{
 const h=fixture();h.view.meta.kind='';h.app.sharedMode=true;
 assert.equal(h.api.localTerminal(h.view),false);
 assert.equal(h.api.start(),false);
});
test('trace is opt-in and records a bounded, redacted browser-local snapshot',()=>{
 const h=fixture();
 h.api.capture(h.view,'unwanted',{line:'cp a_py'});assert.equal(h.api.count,0);
 assert.equal(h.api.start(),true);assert.equal(h.api.enabled,true);
 h.api.capture(h.view,'render-context',{line:'cp a_py',completion:{kind:'paths',prefix:'a_py'}});
 h.api.capture(h.view,'secret-input',{line:'export TOKEN=secret'});
 const report=h.api.report();assert.equal(report.schema,'taskdeck.autocomplete.trace.v1');
 assert.equal(report.events[report.events.length-2].line,'cp a_py');
 assert.match(report.events[report.events.length-1].line,/hidden/);
 assert.ok(!JSON.stringify(report).includes('TOKEN=secret'));
 h.api.stop();assert.equal(h.api.enabled,false);
 h.api.capture(h.view,'after-stop',{line:'cp a_py'});
 assert.equal(h.api.report().events.some(row=>row.stage==='after-stop'),false);
});
test('manual read-only probe reveals API results independently of popup gating',async()=>{
 const h=fixture();h.api.start();
 await h.api.probe();
 const events=h.api.report().events;
 assert.ok(events.some(row=>row.stage==='manual-snapshot'&&row.analysis?.suggestionReady===false));
 assert.ok(events.some(row=>row.stage==='manual-probe-request'&&row.prefix==='a_py'));
 assert.ok(events.some(row=>row.stage==='manual-probe-response'&&row.count===2));
 assert.ok(h.requests.every(url=>url.includes('completion-paths?kind=paths')));
 assert.ok(h.requests.every(url=>url.includes('prefix=a_py')));
});
test('failed API request is visible as a trace event, not swallowed',async()=>{
 const h=fixture({networkError:true});h.api.start();await h.api.probe();
 assert.ok(h.api.report().events.some(row=>row.stage==='manual-probe-error'&&row.error.includes('403')));
});
test('manual probe accepts safe typed path context even when prompt detection failed',async()=>{
 const h=fixture();h.api.start();
 h.context.TaskDeckSmartTerminal.diagnose=()=>({analysis:{line:'',suggestionReady:false,pathContext:null}});
 const probeInput=h.doc.body.children[0].children[0].children[2];
 probeInput.value='cp a_py';
 await h.api.probe();
 const events=h.api.report().events;
 assert.ok(events.some(x=>x.stage==='manual-probe-request'&&x.prefix==='a_py'));
 assert.ok(events.some(x=>x.stage==='manual-probe-response'&&x.count===2));
});
test('trace is session scoped and has a 400-event ring buffer',()=>{
 const h=fixture();h.api.start();const another={meta:{id:'other'}};
 h.api.capture(another,'should-be-ignored',{});
 for(let i=0;i<430;i++)h.api.capture(h.view,'output-'+i,{i});
 const result=h.api.report();
 assert.equal(result.events.length,400);
 assert.ok(result.dropped>0);
 assert.equal(result.events.some(row=>row.stage==='should-be-ignored'),false);
});
test('headless trace UI offers Start, Stop, Snapshot, Probe, Copy and Download',()=>{
 const h=fixture();const overlay=h.doc.body.children[0];
 const controls=overlay.children[0].children[3];
 const buttons=controls.children.map(button=>button.textContent);
 for(const label of ['Start trace','Stop','Snapshot','Probe now (read-only)','Copy JSON','Download JSON','Clear'])
  assert.ok(buttons.includes(label),label);
 h.api.open();assert.equal(overlay.classList.contains('visible'),true);
});
