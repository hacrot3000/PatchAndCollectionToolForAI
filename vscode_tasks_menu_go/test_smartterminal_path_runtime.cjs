// PTY -> xterm write callback -> context-aware path popup regression.
// This exercises the UI in addition to the pure completion engine.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

const engineJS=fs.readFileSync(path.join(__dirname,'web/featuremods/smartterminal_engine.js'),'utf8');
const uiJS=fs.readFileSync(path.join(__dirname,'web/featuremods/smartterminal.js'),'utf8');
const core=(fs.readFileSync(path.join(__dirname,'internal/server/ui.go'),'utf8'));
function node(tag='div'){
 const listeners=new Map();
 const element={
  tag,children:[],style:{},className:'',dataset:{},textContent:'',listeners,
  append(...children){this.children.push(...children);},
  replaceChildren(...children){this.children=children;},
  setAttribute(name,value){this[name]=String(value);},
  addEventListener(type,callback){listeners.set(type,callback);},
  getBoundingClientRect(){return {left:20,top:200};},
  focus(){}
 };
 element.classList={
  contains(name){return element.className.split(' ').includes(name);},
  add(name){if(!this.contains(name))element.className+=(element.className?' ':'')+name;},
  remove(name){element.className=element.className.split(' ').filter(x=>x!==name).join(' ');}
 };
 return element;
}
function harness(){
 const handlers=new Map(),requests=[],sent=[];
 const doc={head:node('head'),body:node('body'),createElement:node};
 let typed='';
 const prompt='duongtc@duongpc:~/568E/Naruto/server-patching$ ';
 const buffer={
  baseY:0,cursorY:0,get cursorX(){return prompt.length+typed.length;},
  getLine(row){
   return row===0?{isWrapped:false,translateToString(){return prompt+typed;}}:null;
  }
 };
 const ws={readyState:1,send:data=>sent.push(data)};
 const shell={seenPrompt:true,executing:false,commandRow:0,commandCol:prompt.length};
 const view={meta:{id:'terminal-1',kind:'terminal',task_id:0,status:'running',target_type:'local'},
  canControl:true,tabReadOnly:false,closed:false,ws,term:{
   buffer:{active:buffer},options:{fontSize:13},
   element:{querySelector(){return node('screen');}},
   attachCustomKeyEventHandler(){},focus(){}}
 };
 const app={active:'terminal-1',browserLeaseLost:false,views:new Map([['terminal-1',view]]),
  jsonFetch:async url=>{
   const request=new URL(url,'https://localhost');
   const kind=request.searchParams.get('kind')||'paths';
   const dir=request.searchParams.get('dir')||'',prefix=request.searchParams.get('prefix')||'';
   requests.push({kind,dir,prefix});
   if(kind==='aliases')return {aliases:[]};
   if(kind==='paths'){
    const entries=dir==='artifacts/'?[{name:'log.txt',dir:false},{name:'src',dir:true}]:
     [{name:'arenaserver',dir:true},{name:'artifacts',dir:true},{name:'build.sh',dir:false}];
    return {entries:entries.filter(item=>item.name.toLowerCase().startsWith(prefix.toLowerCase()))};
   }
   return {entries:[]};
  }
 };
 const context={TaskMenuApp:app,TaskDeckShellIntegration:{getState:()=>shell,getCommands:()=>[]},
  WebSocket:{OPEN:1},TaskDeckTerminalAgents:null,
  window:{addEventListener:(name,callback)=>handlers.set(name,callback),
   innerWidth:1280,innerHeight:800},
  document:doc,URLSearchParams,URL,console,
  setTimeout,clearTimeout,requestAnimationFrame:callback=>setTimeout(callback,0)};
 context.globalThis=context;
 vm.createContext(context);
 vm.runInContext(engineJS,context,{filename:'smartterminal_engine.js'});
 vm.runInContext(uiJS,context,{filename:'smartterminal.js'});
 const menu=doc.body.children[0],ghost=doc.body.children[1];
 function emit(name){const listener=handlers.get(name);assert.equal(typeof listener,'function');listener({detail:{view}});}
 function rows(){return menu.children.map(item=>item.children[0].textContent);}
 function setLine(text){typed=text;}
 async function flush(){await new Promise(resolve=>setTimeout(resolve,175));}
 return {app,view,menu,ghost,requests,sent,emit,rows,setLine,flush};
}
test('xterm completion is recalculated after buffered command echo',async()=>{
 const h=harness();
 // Old behavior: notification fires BEFORE the buffered PTY write is visible.
 h.emit('taskmenu:output');
 await h.flush();
 assert.deepEqual(h.rows(),[]);
 // The xterm.write callback fires after its internal command buffer advances.
 h.setLine('cp ar');
 h.emit('taskmenu:terminal-rendered');
 await h.flush();
 assert.deepEqual(h.rows().slice(0,2),['cp arenaserver/','cp artifacts/']);
 assert.ok(h.requests.some(x=>x.kind==='paths'&&x.dir===''&&x.prefix==='ar'));
 assert.equal(h.menu.classList.contains('visible'),true);
 assert.equal(h.ghost.textContent,'enaserver/');
});
test('ls without arguments immediately suggests children of live terminal CWD',async()=>{
 const h=harness();
 h.setLine('ls');h.emit('taskmenu:terminal-rendered');await h.flush();
 assert.ok(h.rows().includes('ls arenaserver/'));
 assert.ok(h.rows().includes('ls artifacts/'));
 assert.ok(h.requests.some(x=>x.kind==='paths'&&x.dir===''&&x.prefix===''));
});
test('cp artifacts/ suggests only immediate children inside the nested folder',async()=>{
 const h=harness();
 h.setLine('cp artifacts/');h.emit('taskmenu:terminal-rendered');await h.flush();
 assert.ok(h.rows().includes('cp artifacts/src/'));
 assert.ok(h.rows().includes('cp artifacts/log.txt'));
 assert.ok(h.requests.some(x=>x.kind==='paths'&&x.dir==='artifacts/'&&x.prefix===''));
});
test('unverified running commands and non-local sessions never request path lookup',async()=>{
 const h=harness();
 h.setLine('cp ar');h.view.meta.target_type='ssh';
 h.emit('taskmenu:terminal-rendered');await h.flush();
 assert.equal(h.requests.some(x=>x.kind==='paths'),false);
 assert.deepEqual(h.rows().filter(s=>s.includes('arenaserver')),[]);
});
test('WebSocket PTY writer emits render-complete event via xterm callback',()=>{
 assert.match(core,/function writeOutput\(view,data\)/);
 assert.match(core,/view\.term\.write\(data,afterWrite\)/);
 assert.match(core,/taskmenu:terminal-rendered/);
 assert.match(uiJS,/window\.addEventListener\('taskmenu:terminal-rendered'/);
});
