const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const read=name=>fs.readFileSync(path.join(__dirname,'web/featuremods',name),'utf8');

function harness({line='$ cp Cli',entries=[{name:'Client',dir:true}],fetcher=null}={}){
 const listeners=new Map(),sent=[],requests=[],elements=[],timers=[];
 let keyHandler=null,nextTimer=0;
 const makeElement=tag=>{
  const names=new Set(),events=new Map();
  const element={tag,className:'',children:[],style:{},textContent:'',attributes:{},
   classList:{add:x=>names.add(x),remove:x=>names.delete(x),contains:x=>names.has(x)},
   append(...values){this.children.push(...values)},
   replaceChildren(...values){this.children=[...values]},
   setAttribute(name,value){this.attributes[name]=value},
   addEventListener(name,cb){events.set(name,cb)},
   getBoundingClientRect(){return {left:100,top:200,width:900,height:250}},
   fire(name,event){return events.get(name)?.(event)}
  };
  elements.push(element);return element;
 };
 let visibleLine=line;
 const screen={isWrapped:false,translateToString(){return visibleLine;}};
 const buffer={baseY:0,cursorY:0,cursorX:line.length,getLine(){return screen;}};
 const term={options:{fontSize:13},buffer:{active:buffer},focus(){},
  _core:{_renderService:{dimensions:{css:{cell:{width:8,height:16}}}}},
  element:{querySelector(){return {getBoundingClientRect(){return {left:100,top:200,width:900,height:250}}}}},
  attachCustomKeyEventHandler(callback){keyHandler=callback}};
 const view={meta:{id:'term1',status:'running',task_id:0,target_type:'local'},term,canControl:true,tabReadOnly:false,closed:false,
  ws:{readyState:1,send(value){sent.push(value)}}};
 const shell={seenPrompt:true,commandRow:0,commandCol:2,executing:false};
 const context={console,Map,Set,WeakSet,Date,Promise,URLSearchParams,WebSocket:{OPEN:1},
  TaskMenuApp:{active:'term1',browserLeaseLost:false,views:new Map([['term1',view]]),
   jsonFetch(url){requests.push(url);return fetcher?fetcher(url):Promise.resolve({entries});}},
  TaskDeckShellIntegration:{getState(){return shell;},getCommands(){return []}},
  window:{innerWidth:1200,innerHeight:800,addEventListener(name,cb){listeners.set(name,cb)}},
  document:{createElement:makeElement,head:{append(){}},body:{append(){}}},
  setTimeout(fn){const id=++nextTimer;timers.push({id,fn,cancelled:false});return id;},
  clearTimeout(id){const timer=timers.find(item=>item.id===id);if(timer)timer.cancelled=true;},
  requestAnimationFrame(fn){fn();}
 };
 context.globalThis=context;vm.createContext(context);
 vm.runInContext(read('smartterminal_engine.js'),context);
 vm.runInContext(read('smartterminal.js'),context);
 const menu=elements.find(node=>node.className==='smartterm-suggestions');
 const ghost=elements.find(node=>node.className==='smartterm-ghost');
 listeners.get('taskmenu:view-activated')({detail:{id:'term1'}});
 return {
  menu,ghost,requests,sent,view,shell,context,
  setLine(next){visibleLine=next;buffer.cursorX=next.length;listeners.get('taskmenu:output')({detail:{view}});},
  async flush(){
   for(const timer of timers.splice(0))if(!timer.cancelled)await timer.fn();
   await Promise.resolve();await Promise.resolve();
  },
  key(name){
   const event={type:'keydown',key:name,isComposing:false,shiftKey:false,ctrlKey:false,altKey:false,metaKey:false,
    preventDefault(){this.prevented=true},stopPropagation(){this.stopped=true}};
   const allowed=keyHandler(event);return {event,allowed};
  },
  labels(){return menu.children.map(button=>button.children[0].textContent)}
 };
}

test('cp Cli suggests Client directory, then Client/as suggests nested assets; Tab only sends missing suffix',async()=>{
 const h=harness({entries:[{name:'Client',dir:true},{name:'Clip.txt',dir:false}]});
 await h.flush();
 assert.ok(h.requests[0].includes('/api/sessions/term1/completion-paths?'));
 assert.equal(new URL('https://host.test'+h.requests[0]).searchParams.get('prefix'),'Cli');
 assert.equal(h.labels()[0],'cp Client/');
 assert.equal(h.ghost.textContent,'ent/');
 const tab=h.key('Tab');
 assert.equal(tab.allowed,false);
 assert.equal(tab.event.prevented,true);
 assert.deepEqual(h.sent,['ent/']);
 h.setLine('$ cp Client/as');
 // The second directory lookup reads only Client/, rather than scanning a tree.
 h.context.TaskMenuApp.jsonFetch=url=>{
  h.requests.push(url);
  return Promise.resolve({entries:[{name:'assets',dir:true},{name:'aside.txt',dir:false}]});
 };
 await h.flush();
 const second=new URL('https://host.test'+h.requests.at(-1));
 assert.equal(second.searchParams.get('dir'),'Client/');
 assert.equal(second.searchParams.get('prefix'),'as');
 assert.equal(h.labels()[0],'cp Client/assets/');
 assert.equal(h.ghost.textContent,'sets/');
});

test('context-aware shell escaping, directory prioritization and no auto-execution',async()=>{
 const h=harness({line:'$ cp My',entries:[{name:'My File.txt',dir:false},{name:'My Folder',dir:true},{name:'My$special',dir:false}]});
 await h.flush();
 assert.equal(h.labels()[0],'cp My\\ Folder/');
 assert.ok(h.labels().includes('cp My\\$special'));
 assert.deepEqual(h.sent,[],'ghost and popup must not type or run commands');
 h.key('Tab');
 assert.deepEqual(h.sent,['\\ Folder/']);
});

test('stale filesystem response never replaces suggestions for a later command',async()=>{
 let releaseFirst;const first=new Promise(resolve=>{releaseFirst=resolve;});
 const h=harness({fetcher:()=>first});
 await h.flush();
 assert.equal(h.requests.length,1);
 h.setLine('$ cp Other');
 await h.flush();
 assert.equal(h.requests.length,2);
 releaseFirst({entries:[{name:'Client',dir:true}]});
 await Promise.resolve();await Promise.resolve();
 assert.ok(!h.labels().includes('cp Client/'),'stale result must be discarded');
});

test('no filesystem scan for an unsupported command, unsafe expansion or running shell',async()=>{
 const h=harness({line:'$ echo Cli'});
 await h.flush();assert.equal(h.requests.length,0);
 h.setLine('$ cp $HOME');await h.flush();assert.equal(h.requests.length,0);
 h.setLine('$ git status Cli');await h.flush();assert.equal(h.requests.length,0);
 h.setLine('$ cp Cli');h.shell.executing=true;await h.flush();assert.equal(h.requests.length,0);
});

test('aliases change operand suggestions without rewriting the command user typed',async()=>{
 const h=harness({line:'$ gc tes',entries:[]});
 h.context.TaskDeckSmartTerminal.setOSIndex(h.view,'aliases',["alias gb='git branch'","alias gc='gc-safe'","alias ga='git add'","alias ll='ls -alF'","alias gis='git status'"]);
 h.context.TaskMenuApp.jsonFetch=url=>{
  h.requests.push(url);
  return Promise.resolve({branches:['testing','ss20-dev','ss20-test']});
 };
 await h.flush();
 const requested=new URL('https://host.test'+h.requests.at(-1));
 assert.equal(requested.searchParams.get('kind'),'branches');
 assert.equal(requested.searchParams.get('prefix'),'tes');
 assert.equal(h.labels()[0],'gc testing');
 assert.equal(h.ghost.textContent,'ting');
 h.key('Tab');
 assert.deepEqual(h.sent,['ting'],'append branch suffix; do not turn gc-safe into a Git command');
});

test('alias for git add and ls enables filesystem suggestions; unrelated aliases cannot trigger Git scans',async()=>{
 const h=harness({line:'$ ga Cli',entries:[{name:'Client',dir:true}]});
 h.context.TaskDeckSmartTerminal.setOSIndex(h.view,'aliases',["alias ga='git add'","alias ll='ls -alF'","alias gí='git status'"]);
 await h.flush();
 assert.equal(h.labels()[0],'ga Client/');
 assert.equal(new URL('https://host.test'+h.requests[0]).searchParams.get('kind'),'paths');
 h.setLine('$ ll Cli');
 await h.flush();
 assert.equal(h.labels()[0],'ll Client/');
 h.setLine('$ gí test');
 await h.flush();
 assert.ok(!h.requests.some(x=>new URL('https://host.test'+x).searchParams.get('prefix')==='test'));
});
test('git merge and branch-delete subcommands suggest matching refs instead of files',async()=>{
 const h=harness({line:'$ git merge ss20',entries:[]});
 h.context.TaskMenuApp.jsonFetch=url=>{
  h.requests.push(url);
  return Promise.resolve({branches:['ss20-dev-hp64','ss20-test','testing']});
 };
 await h.flush();
 assert.equal(h.labels()[0],'git merge ss20-dev-hp64');
 h.setLine('$ gb -d ss20');
 h.context.TaskDeckSmartTerminal.setOSIndex(h.view,'aliases',["alias gb='git branch'"]);
 await h.flush();
 assert.equal(h.labels()[0],'gb -d ss20-dev-hp64');
 assert.ok(h.requests.every(x=>new URL('https://host.test'+x).searchParams.get('kind')==='branches'));
});
