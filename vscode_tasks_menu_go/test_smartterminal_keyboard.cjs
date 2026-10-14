// Regression: xterm's custom key handler alone does not suppress browser Tab focus.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

const read=name=>fs.readFileSync(path.join(__dirname,'web/featuremods',name),'utf8');

function harness({broadcast=null,forcedSuggestion=null,terminalLine='$ ls',oscMarkers=true,expectPopup=true}={}){
 const listeners=new Map(), sent=[],focuses=[],elements=[];
 let keyHandler=null;
 const makeElement=tag=>{
  const handlers=new Map(),classes=new Set();
  const element={tag,style:{},children:[],attributes:{},className:'',textContent:'',
   classList:{add:v=>classes.add(v),remove:v=>classes.delete(v),contains:v=>classes.has(v)},
   append(...nodes){this.children.push(...nodes)},
   replaceChildren(...nodes){this.children=[...nodes]},
   setAttribute(key,value){this.attributes[key]=value},
   addEventListener(name,handler){handlers.set(name,handler)},
   fire(name,event){return handlers.get(name)?.(event)},
   getBoundingClientRect(){return {left:100,top:200,width:900,height:250}}
  };
  elements.push(element);
  return element;
 };
 const shell={seenPrompt:oscMarkers,commandRow:oscMarkers?0:null,commandCol:2,executing:false};
 const line={isWrapped:false,translateToString:()=>terminalLine};
 const buffer={baseY:0,cursorY:0,cursorX:terminalLine.length,length:1,getLine:()=>line};
 const term={
  options:{fontSize:13},buffer:{active:buffer},
  _core:{_renderService:{dimensions:{css:{cell:{width:8,height:16}}}}},
  element:{querySelector:()=>({getBoundingClientRect:()=>({left:100,top:200,width:900,height:250})})},
  attachCustomKeyEventHandler(fn){keyHandler=fn},
  focus(){focuses.push('terminal')}
 };
 const view={meta:{id:'tab1',status:'running'},term,
  canControl:true,tabReadOnly:false,closed:false,
  ws:{readyState:1,send(value){sent.push(value)}}};
 const app={active:'tab1',browserLeaseLost:false,views:new Map([['tab1',view]])};
 const context={
  console,Map,Set,WeakSet,Date,WebSocket:{OPEN:1},
  TaskMenuApp:app,
  TaskMenuBroadcast:broadcast,
  TaskDeckShellIntegration:{
   getState:()=>shell,
   getCommands:()=>[
    {command:'ls -la',finishedAt:100},
    {command:'ls -lh',finishedAt:200}
   ]
  },
  window:{innerWidth:1200,innerHeight:800,
   addEventListener(name,cb){listeners.set(name,cb)}},
  document:{
   createElement:makeElement,
   head:{append(){}},
   body:{append(){}}
  },
  requestAnimationFrame(fn){fn()}
 };
 context.globalThis=context;
 vm.createContext(context);
 vm.runInContext(read('smartterminal_engine.js'),context);
 if(forcedSuggestion)context.TaskDeckCompletionEngine.rank=()=>[{command:forcedSuggestion,source:'terminal',group:0,score:6}];
 vm.runInContext(read('smartterminal.js'),context);
 listeners.get('taskmenu:view-activated')({detail:{id:'tab1'}});
 const menu=elements.find(el=>el.className==='smartterm-suggestions');
 assert.equal(menu?.classList.contains('visible'),expectPopup,'completion popup visibility');
 assert.equal(typeof keyHandler,'function');
 let browserFocus='terminal';
 function key(value,overrides={}){
  const event={
   type:'keydown',key:value,isComposing:false,
   shiftKey:false,ctrlKey:false,altKey:false,metaKey:false,
   defaultPrevented:false,stopped:false,
   preventDefault(){this.defaultPrevented=true},
   stopPropagation(){this.stopped=true},
   ...overrides
  };
  const result=keyHandler(event);
  // Model the native browser behavior if preventDefault was omitted.
  if(value==='Tab'&&!event.defaultPrevented)browserFocus='browser-back';
  return {event,result};
 }
 return {key,view,sent,focuses,menu,shell,get browserFocus(){return browserFocus}};
}
test('ArrowDown twice chooses the second suggestion, Tab inserts it and keeps terminal focus',()=>{
 const h=harness();
 const first=h.key('ArrowDown');
 assert.equal(first.result,false);
 assert.equal(first.event.defaultPrevented,true);
 assert.equal(first.event.stopped,true);
 const second=h.key('ArrowDown');
 assert.equal(second.event.defaultPrevented,true);
 const tab=h.key('Tab');
 assert.equal(tab.result,false);
 assert.equal(tab.event.defaultPrevented,true);
 assert.equal(tab.event.stopped,true);
 assert.deepEqual(h.sent,[' -la']);
 assert.equal(h.browserFocus,'terminal');
 assert.ok(h.focuses.length>=1,'xterm must retain keyboard focus');
 assert.equal(h.menu.classList.contains('visible'),false);
});
test('Tab on stale prompt dismisses suggestion without leaking focus or sending shell input',()=>{
 const h=harness();
 h.key('ArrowDown');
 h.shell.executing=true;
 const tab=h.key('Tab');
 assert.equal(tab.result,false);
 assert.equal(tab.event.defaultPrevented,true);
 assert.equal(h.browserFocus,'terminal');
 assert.deepEqual(h.sent,[]);
 assert.equal(h.menu.classList.contains('visible'),false);
});
test('Escape cancels the menu without navigating browser or sending input',()=>{
 const h=harness();
 const escape=h.key('Escape');
 assert.equal(escape.result,false);
 assert.equal(escape.event.defaultPrevented,true);
 assert.equal(escape.event.stopped,true);
 assert.deepEqual(h.sent,[]);
 assert.equal(h.menu.classList.contains('visible'),false);
});
test('Tab outside an active suggestion menu is not intercepted',()=>{
 const h=harness();
 h.key('Escape');
 const tab=h.key('Tab');
 assert.equal(tab.result,true);
 assert.equal(tab.event.defaultPrevented,false);
 assert.deepEqual(h.sent,[]);
});

test('Broadcast completion sends only accepted suffix and preserves the local source',()=>{
 const routed=[];
 const h=harness({broadcast:{forwardCompletion(view,data,options){
  routed.push({source:view.meta.id,data,replacesLine:options.replacesLine});
  return true;
 }}});
 h.key('ArrowDown');h.key('Tab');
 assert.deepEqual(routed,[{source:'tab1',data:' -lh',replacesLine:false}]);
 assert.deepEqual(h.sent,[' -lh']);
 assert.equal(h.browserFocus,'terminal');
});
test('Broadcast confirmation rejection cancels the local completion too',()=>{
 const routed=[];
 const h=harness({broadcast:{forwardCompletion(view,data,options){routed.push(data);return false;}}});
 h.key('Tab');
 assert.deepEqual(routed,[' -lh']);
 assert.deepEqual(h.sent,[]);
 assert.equal(h.browserFocus,'terminal');
});
test('non-prefix suggestion routes one complete Ctrl-U replacement to Broadcast',()=>{
 const routed=[];
 const h=harness({forcedSuggestion:'echo ls -la',broadcast:{forwardCompletion(view,data,options){
  routed.push({data,replacesLine:options.replacesLine});return true;
 }}});
 h.key('Tab');
 assert.deepEqual(routed,[{data:'\\x15echo ls -la'.replace('\\x15','\x15'),replacesLine:true}]);
 assert.deepEqual(h.sent,['\x15echo ls -la']);
});

test('tel completes to telnet at a verified Bash prompt even without OSC 133',()=>{
 const h=harness({
  oscMarkers:false,
  terminalLine:'duongtc@duongpc:~/568E/Naruto/server-patching$ tel'
 });
 assert.equal(h.menu.children[0]?.children[0]?.textContent,'telnet');
 assert.equal(h.menu.children[0]?.children[1]?.textContent,'builtin');
 const tab=h.key('Tab');
 assert.equal(tab.result,false);
 assert.equal(tab.event.defaultPrevented,true);
 assert.deepEqual(h.sent,['net'],'only the missing suffix must be sent, not Enter');
 assert.equal(h.browserFocus,'terminal');
});
test('telnet interactive prompt is never mistaken for a Bash prompt or intercepted',()=>{
 const h=harness({
  oscMarkers:false,terminalLine:'telnet> qui',expectPopup:false
 });
 const tab=h.key('Tab');
 assert.equal(tab.result,true,'let Telnet handle its own Tab');
 assert.equal(tab.event.defaultPrevented,false);
 assert.deepEqual(h.sent,[]);
});
test('arbitrary user-like output without Bash prompt does not enable completion',()=>{
 const h=harness({oscMarkers:false,terminalLine:'Connected to 10.9.1.20.',expectPopup:false});
 assert.equal(h.key('Tab').result,true);
 assert.deepEqual(h.sent,[]);
});
