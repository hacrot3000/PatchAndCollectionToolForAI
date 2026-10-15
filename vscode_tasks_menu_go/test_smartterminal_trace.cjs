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
 const context={TaskMenuApp:app,TaskDeckSmartTerminal:smart,document:doc,
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
 const controls=overlay.children[0].children[2];
 const buttons=controls.children.map(button=>button.textContent);
 for(const label of ['Start trace','Stop','Snapshot','Probe now (read-only)','Copy JSON','Download JSON','Clear'])
  assert.ok(buttons.includes(label),label);
 h.api.open();assert.equal(overlay.classList.contains('visible'),true);
});
