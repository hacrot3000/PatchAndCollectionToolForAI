const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const read=name=>fs.readFileSync(path.join(__dirname,'web/featuremods',name),'utf8');

function harness({line='$ cp Cli',entries=[{name:'Client',dir:true}],fetcher=null,oscMarkers=true,fallbackAliases=[],screenTop=200,screenLeft=100,viewportHeight=800}={}){
 const listeners=new Map(),sent=[],requests=[],aliasRequests=[],elements=[],timers=[];
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
  element:{querySelector(){return {getBoundingClientRect(){return {left:screenLeft,top:screenTop,width:900,height:250}}}}},
  attachCustomKeyEventHandler(callback){keyHandler=callback}};
 const view={meta:{id:'term1',status:'running',task_id:0,target_type:'local'},term,canControl:true,tabReadOnly:false,closed:false,
  ws:{readyState:1,send(value){sent.push(value)}}};
 const shell={seenPrompt:oscMarkers,commandRow:oscMarkers?0:null,commandCol:2,executing:false};
 const context={console,Map,Set,WeakSet,Date,Promise,URLSearchParams,WebSocket:{OPEN:1},
  TaskMenuApp:{active:'term1',browserLeaseLost:false,views:new Map([['term1',view]]),
   jsonFetch(url){if(url.includes('kind=aliases')){aliasRequests.push(url);return Promise.resolve({aliases:fallbackAliases});}requests.push(url);return fetcher?fetcher(url):Promise.resolve({entries});}},
  TaskDeckShellIntegration:{getState(){return shell;},getCommands(){return []}},
  window:{innerWidth:1200,innerHeight:viewportHeight,addEventListener(name,cb){listeners.set(name,cb)}},
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
  menu,ghost,requests,aliasRequests,sent,view,shell,context,
  activate(){listeners.get('taskmenu:view-activated')({detail:{id:'term1'}})},
  setLine(next){visibleLine=next;buffer.cursorX=next.length;listeners.get('taskmenu:output')({detail:{view}});},
  async flush(){
   for(const timer of timers.splice(0))if(!timer.cancelled)timer.fn(); // do not await intentionally delayed network responses
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


test('tel comes from live executable PATH lookup without shell index or OSC 133',async()=>{
 const h=harness({
  line:'duongtc@duongpc:~/dubike$ tel',
  oscMarkers:false,
  fetcher:url=>{
   const query=new URL('https://host.test'+url).searchParams;
   assert.equal(query.get('kind'),'commands');
   assert.equal(query.get('prefix'),'tel');
   return Promise.resolve({commands:['telnet']});
  }
 });
 await h.flush();
 assert.equal(h.requests.length,1,'first command word queries the real executable inventory');
 assert.equal(h.labels()[0],'telnet','PATH response must be offered above static history');
 assert.equal(h.ghost.textContent,'net');
 const accepted=h.key('Tab');
 assert.equal(accepted.allowed,false);
 assert.equal(accepted.event.prevented,true);
 assert.deepEqual(h.sent,['net'],'Tab must never execute Telnet or add Enter');
});
test('command lookup is not made for completed command arguments or TUI output',async()=>{
 const h=harness({line:'duongtc@duongpc:~/dubike$ telnet 10.9.1.20',oscMarkers:false});
 await h.flush();
 assert.equal(h.requests.length,0,'no command search while typing arguments');
 h.setLine('telnet> q');
 await h.flush();
 assert.equal(h.requests.length,0,'Telnet interactive input must not query PATH');
 assert.equal(h.key('Tab').allowed,true,'Telnet owns Tab in its command interpreter');
 assert.deepEqual(h.sent,[]);
});
test('single-word PATH suggestions complement cached shell command index',async()=>{
 const h=harness({line:'$ tel',fetcher:()=>Promise.resolve({commands:['telnet']})});
 h.context.TaskDeckSmartTerminal.setOSIndex(h.view,'commands',['telnet']);
 await h.flush();
 assert.equal(h.labels()[0],'telnet');
 assert.equal(h.labels().filter(x=>x==='telnet').length,1,'cache and PATH results deduplicated');
});


test('bare gb and git branch expose real branches safely plus inspection options',async()=>{
 const branches=['dev','develop/new_SNV','develop/new_SNV_only','master','new-cs-state','ss20-dev','ss20-dev-external','ss20-dev-fixarena_deep','ss20-dev-fixarena_deep2','ss20-dev-hp64','ss20-dev-multi-wave','ss20-test','ss20-test_tmp1','stable-release','testing'];
 const h=harness({line:'duongtc@duongpc:~/568E/Naruto/server-patching$ gb',
  oscMarkers:false,
  fallbackAliases:["alias gb='git branch'","alias gc='gc-safe'","alias ll='ls -alF'"],
  fetcher:()=>Promise.resolve({branches})});
 await h.flush();await h.flush();
 assert.equal(h.aliasRequests.length,1,'fallback aliases should be read once, not injected into the PTY');
 assert.ok(h.requests.some(url=>new URL('https://host.test'+url).searchParams.get('kind')==='branches'),'alias hydration must trigger a real Git branch lookup');
 assert.ok(h.labels().includes('gb --list testing'),'gb must never suggest an accidental branch-creation command');
 assert.ok(h.labels().length>10,'branch list must scroll instead of dropping later refs');
 assert.ok(h.labels().includes('gb --list develop/new_SNV'),'slash-delimited branch refs must be preserved');
 assert.ok(h.labels().includes('gb -a'),'alias should offer git branch flags');
 h.setLine('duongtc@duongpc:~/568E/Naruto/server-patching$ git branch');
 await h.flush();
 assert.ok(h.labels().includes('git branch --list testing'),'bare git branch lists actual branch safely');
 h.setLine('duongtc@duongpc:~/568E/Naruto/server-patching$ gc');
 await h.flush();
 assert.ok(h.labels().includes('gc testing'),'gc-safe alias should offer existing branches');
 h.setLine('duongtc@duongpc:~/568E/Naruto/server-patching$ git checkout');
 await h.flush();
 assert.ok(h.labels().includes('git checkout testing'),'checkout should offer actual branch before typing space');
 assert.deepEqual(h.sent,[],'completion never runs a checkout');
});
test('live CWD files autocomplete cp ar, bare cp, ls and ll without OSC',async()=>{
 const h=harness({
  line:'duongtc@duongpc:~/568E/M3$ cp ar',oscMarkers:false,
  fallbackAliases:["alias ll='ls -alF'"],
  entries:[{name:'artifacts',dir:true},{name:'auto_build_file_change.txt',dir:false}]
 });
 await h.flush();await h.flush();
 assert.equal(h.labels()[0],'cp artifacts/');
 assert.ok(h.requests.some(url=>new URL('https://host.test'+url).searchParams.get('prefix')==='ar'));
 h.setLine('duongtc@duongpc:~/568E/M3$ cp');
 await h.flush();
 assert.equal(h.labels()[0],'cp artifacts/');
 h.setLine('duongtc@duongpc:~/568E/M3$ ls');
 await h.flush();
 assert.equal(h.labels()[0],'ls artifacts/');
 h.setLine('duongtc@duongpc:~/568E/M3$ ll');
 await h.flush();
 assert.equal(h.labels()[0],'ll artifacts/');
 const tab=h.key('Tab');
 assert.equal(tab.allowed,false);
 assert.equal(tab.event.prevented,true);
 assert.deepEqual(h.sent,[' artifacts/'],'accepting a suggestion sends only the suffix');
});


test('opening and activating a terminal silently prefetches aliases without PTY writes',async()=>{
 const h=harness({
  line:'duongtc@duongpc:~/568E/Naruto/server-patching$ gb',
  oscMarkers:false,
  fallbackAliases:["alias gb='git branch'","alias gc='gc-safe'"],
  fetcher:()=>Promise.resolve({branches:['testing','master']})
 });
 await h.flush();await h.flush();
 assert.equal(h.aliasRequests.length,1,'read-only alias endpoint queried once');
 assert.deepEqual(h.sent,[],'opening tab must not inject alias -p into Bash');
 h.activate();h.activate();h.activate();
 await h.flush();await h.flush();
 assert.equal(h.aliasRequests.length,1,'repeated tab activation reuses cached aliases');
 assert.deepEqual(h.sent,[],'activating a tab cannot echo TDIDX scripts');
 assert.ok(h.labels().includes('gb --list testing'),'read-only aliases still drive branch suggestions');
});


test('suggestion menu prefers above cursor when full list fits',async()=>{
 const h=harness({screenTop:450});
 await h.flush();
 assert.equal(h.menu.classList.contains('visible'),true);
 const y=Number.parseFloat(h.menu.style.top),height=Number.parseFloat(h.menu.style.maxHeight);
 assert.ok(y+height<=450-6,'popup must end above editable terminal row');
 assert.equal(height,250);
 assert.equal(h.ghost.style.top,'450px','inline ghost still belongs on the editable row');
});
test('suggestion menu flips below cursor when top of viewport lacks space',async()=>{
 const h=harness({screenTop:40});
 await h.flush();
 assert.equal(h.menu.style.top,'62px','popup must begin after command row (16px) and 6px gap');
 assert.equal(h.menu.style.maxHeight,'250px');
 assert.equal(h.menu.classList.contains('visible'),true);
});
test('short Firefox viewport shrinks and scrolls suggestions above cursor, without covering input',async()=>{
 const h=harness({screenTop:258,viewportHeight:311});
 await h.flush();
 const y=Number.parseFloat(h.menu.style.top),height=Number.parseFloat(h.menu.style.maxHeight);
 assert.equal(y,6);
 assert.equal(height,246);
 assert.ok(y+height+6<=258,'popup must not cover the current typed command');
 // When the window grows again, the menu must regain its full height.
 h.context.window.innerHeight=800;
 h.activate();
 await h.flush();
 assert.equal(h.menu.style.maxHeight,'250px','old shortened max-height must not stick after resize');
});
test('suggestion menu remains within the right edge of a narrow window',async()=>{
 const h=harness({screenLeft:1100,screenTop:450});
 await h.flush();
 const left=Number.parseFloat(h.menu.style.left);
 assert.ok(left>=6);
 assert.ok(left+900<=1200-6,'simulated menu width must not exceed viewport right margin');
});


test('gc multi displays an inline dimmed infix branch preview without changing Bash input',async()=>{
 const branches=['ss20-dev-multi-wave','testing','ss20-dev-hp64'];
 const h=harness({line:'$ gc multi',fallbackAliases:["alias gc='gc-safe'"],
  fetcher:()=>Promise.resolve({branches})});
 await h.flush();await h.flush();
 assert.ok(h.requests.some(url=>new URL('https://host.test'+url).searchParams.get('prefix')==='multi'));
 assert.equal(h.menu.classList.contains('visible'),true);
 const first=h.menu.children[0].children[0];
 assert.equal(first.children.map(part=>part.textContent).join(''),'gc ss20-dev-multi-wave');
 assert.equal(h.ghost.classList.contains('inline-replace'),true);
 assert.deepEqual(h.ghost.children.map(part=>[part.className,part.textContent]),[
  ['suggestion-muted','ss20-dev-'],['suggestion-typed','multi'],['suggestion-muted','-wave']
 ]);
 assert.deepEqual(h.sent,[],'visual suggestion must NOT overwrite or execute actual gc multi');
 // The next typed character hides the stale ghost even before the PTY echoes.
 h.context.TaskDeckSmartTerminal.onInput(h.view,'a');
 assert.equal(h.ghost.style.display,'none');
 h.setLine('$ gc multia');
 await h.flush();
 assert.equal(h.menu.classList.contains('visible'),false);
 assert.equal(h.ghost.style.display,'none');
 assert.deepEqual(h.sent,[]);
});

test('explicit Tab accepts a fuzzy branch as a full-line replacement, without Enter',async()=>{
 const h=harness({line:'$ gc multi',fallbackAliases:["alias gc='gc-safe'"],
  fetcher:()=>Promise.resolve({branches:['ss20-dev-multi-wave']})});
 await h.flush();await h.flush();
 assert.equal(h.ghost.classList.contains('inline-replace'),true);
 const tab=h.key('Tab');
 assert.equal(tab.allowed,false);
 assert.equal(tab.event.prevented,true);
 assert.deepEqual(h.sent,['\x15gc ss20-dev-multi-wave']);
 assert.ok(!h.sent[0].includes('\r')&&!h.sent[0].includes('\n'));
});

test('fuzzy file names use the same dimmed fragment model and cancel on mismatch',async()=>{
 const entries=[{name:'a_python_patch',dir:true},{name:'artifact',dir:true}];
 const h=harness({line:'$ cp python',entries});
 await h.flush();
 assert.equal(h.ghost.classList.contains('inline-replace'),true);
 assert.deepEqual(h.ghost.children.map(part=>[part.className,part.textContent]),[
  ['suggestion-muted','a_'],['suggestion-typed','python'],['suggestion-muted','_patch/']
 ]);
 assert.deepEqual(h.sent,[]);
 h.setLine('$ cp pythona');await h.flush();
 assert.equal(h.menu.classList.contains('visible'),false);
 assert.equal(h.ghost.style.display,'none');
});

test('PATH executable fuzzy names share the same preview and do not execute',async()=>{
 const h=harness({line:'$ grep',fetcher:()=>Promise.resolve({commands:['egrep','pgrep']})});
 await h.flush();
 assert.equal(h.ghost.classList.contains('inline-replace'),true);
 assert.deepEqual(h.ghost.children.map(part=>[part.className,part.textContent]),[
  ['suggestion-muted','e'],['suggestion-typed','grep'],['suggestion-muted','']
 ]);
 assert.deepEqual(h.sent,[]);
});
