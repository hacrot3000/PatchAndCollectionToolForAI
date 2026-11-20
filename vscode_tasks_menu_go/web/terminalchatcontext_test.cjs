// Runtime tests for terminal→Agent Chat staging. Uses a fake xterm buffer and
// mocks only the public stageContext contract (never the production feature).
'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

class FakeElement{
 constructor(tag='div'){
  this.tagName=tag.toUpperCase();this.children=[];this.parentElement=null;this.className='';
  this.textContent='';this.title='';this.type='';this.disabled=false;this.hidden=false;this.dataset={};
  this.style={};this.attributes={};this.listeners={};this.isConnected=true;this.offsetWidth=10;this.offsetHeight=10;
 }
 append(...nodes){for(const node of nodes){node.parentElement=this;this.children.push(node);}}
 remove(){this.isConnected=false;if(this.parentElement)this.parentElement.children=this.parentElement.children.filter(x=>x!==this);}
 setAttribute(k,v){this.attributes[k]=String(v);}
 addEventListener(type,fn){(this.listeners[type] ||= []).push(fn);}
 removeEventListener(type,fn){this.listeners[type]=(this.listeners[type]||[]).filter(x=>x!==fn);}
 async dispatch(type,event={}){event.target??=this;event.preventDefault??=()=>{};event.stopPropagation??=()=>{};const results=[];for(const fn of this.listeners[type]||[])results.push(fn(event));const direct=this['on'+type]?.(event);if(direct&&typeof direct.then==='function')results.push(direct);await Promise.all(results.filter(x=>x&&typeof x.then==='function'));}
 querySelector(selector){
  if(selector===':scope > .taskmenu-menu-trigger')return this.trigger||null;
  if(selector===':scope > .taskmenu-menu-popover')return this.popover||null;
  if(selector===':scope > .terminal-chat-attach-console')return this.children.find(x=>x.className==='terminal-chat-attach-console')||null;
  if(selector==='button:not(:disabled)')return this.children.find(x=>x.tagName==='BUTTON'&&!x.disabled)||null;
  if(selector==='.console-search-btn')return this.searchButton||null;
  if(selector===':scope > .mobile-tab-actions')return null;
  return null;
 }
 querySelectorAll(selector){
  if(selector==='button:not(:disabled)')return this.children.filter(x=>x.tagName==='BUTTON'&&!x.disabled);
  if(selector==='.pane-action-menus .taskmenu-menu')return this.groups||[];
  return [];
 }
 focus(){}
}
const document={
 head:new FakeElement('head'),body:new FakeElement('body'),
 createElement:tag=>new FakeElement(tag),
 listeners:{},addEventListener(type,fn){(this.listeners[type] ||= []).push(fn);},
};
const window={innerWidth:1200,innerHeight:800,prompt:()=>null,listeners:{},addEventListener(type,fn){(this.listeners[type] ||= []).push(fn);}};
const staged=[];
const app={
 views:new Map(),sharedMode:false,
 consoleText:view=>view.console,
 showError:error=>{throw error;},
};
const context={TaskMenuApp:app,document,window,TextEncoder,TextDecoder,Date,Map,WeakMap,Set,Promise,console,setTimeout(fn){fn();return 1;},clearTimeout(){},processEnv:1,
 globalThis:null,navigator:{clipboard:{writeText:async()=>{},readText:async()=>''}},
};
context.globalThis=context;
context.TaskMenuAgentChat={stageContext:async(payload,options)=>{staged.push({payload,options});return {ok:true,session_id:'A'};}};

function makeView(id,consoleText){
 const popover=new FakeElement();
 const group=new FakeElement();group.trigger=new FakeElement('button');group.trigger.textContent='Console ▾';group.popover=popover;
 const pane=new FakeElement();pane.groups=[group];pane.popover=popover;
 const termElement=new FakeElement();
 const tab=new FakeElement('button');tab.textContent='Terminal '+id;
 const view={meta:{id,title:'Terminal '+id,cwd:'/project/'+id,kind:'terminal'},
  term:{element:termElement,selection:'',getSelection(){return this.selection;},getSelectionPosition(){return {start:{x:2,y:10},end:{x:8,y:11}};},options:{disableStdin:false},focus(){},paste(){},selectAll(){}},
  pane,tab,console:consoleText,closed:false};
 app.views.set(id,view);return view;
}

(async()=>{
 const a=makeView('A','console A\nBUILD FAILED\n');
 const b=makeView('B','console B\n');
 // Exercise late module load: both terminal views pre-exist in app.views.
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'featuremods/terminalchatcontext.js'),'utf8'),context,{filename:'terminalchatcontext.js',timeout:2000});
 app.active='B'; // the tab clicked below is A; active view must be irrelevant.
 const module=context.TaskMenuTerminalChatContext;
 assert.ok(module,'terminal chat module public test surface missing');
 const captured=module.captureSelectionContext(a);
 assert.equal(captured.metadata.terminal_id,'A');
 assert.equal(captured.position.selection_start.buffer_row,10);
 assert.equal(captured.position.selection_end.column,8);
 module.installAll();module.installView(a);module.installView(a);
 const buttonsA=a.pane.popover.children.filter(x=>x.className==='terminal-chat-attach-console');
 assert.equal(buttonsA.length,1,'repeated lifecycle events must not duplicate action');
 const buttonB=b.pane.popover.children.find(x=>x.className==='terminal-chat-attach-console');
 assert.ok(buttonB,'terminal tab console action missing');
 // The action must capture the terminal it belongs to, not the active view.
 const buttonA=buttonsA[0];
 await buttonA.dispatch('click');
 assert.equal(staged.length,1);
 assert.equal(staged[0].payload.kind,'terminal_console');
 assert.match(staged[0].payload.text,/console A/);
 assert.doesNotMatch(staged[0].payload.text,/console B/);
 assert.equal(staged[0].payload.source.terminal_id,'A');
 assert.equal(staged[0].options.open,true);
 assert.equal(staged[0].options.focusComposer,true);
 // Each terminal keeps its own action instance bound to its own view.
 const buttonAAgain=a.pane.popover.children.find(x=>x.className==='terminal-chat-attach-console');
 assert.equal(buttonAAgain,buttonA,'console action must be stable across calls');
 await buttonAAgain.dispatch('click');
 assert.equal(staged.length,2);
 assert.equal(staged[1].payload.source.terminal_id,'A');
 assert.doesNotMatch(staged[1].payload.text,/console B/);

 // Retarget/reconnect preserves one action/listener set while using the new
 // terminal identity and contents.
 a.meta.id='A-retargeted';a.meta.title='Retargeted build';a.console='new target output\\n';
 app.views.delete('A');app.views.set(a.meta.id,a);
 for(const handler of window.listeners['taskmenu:session-retargeted']||[])handler({detail:{view:a,newID:a.meta.id}});
 module.installView(a);module.installView(a);
 assert.equal(a.pane.popover.children.filter(x=>x.className==='terminal-chat-attach-console').length,1,'retarget duplicated the action');
 const retargetButton=a.pane.popover.children.find(x=>x.className==='terminal-chat-attach-console');
 await retargetButton.dispatch('click');
 assert.equal(staged.at(-1).payload.source.terminal_id,'A-retargeted');
 assert.match(staged.at(-1).payload.text,/new target output/);
 // Right-click snapshots the selection synchronously. Changing xterm selection
 // after the menu appears must not mutate the staged attachment.
 a.term.selection='error: exact selected text\n    trace';
 const contextEvent={clientX:20,clientY:30,preventDefault(){this.prevented=true;},stopPropagation(){this.stopped=true;}};
 a.term.element.dispatch('contextmenu',contextEvent);
 assert.equal(contextEvent.prevented,true);
 assert.equal(module.hasSelectionMenu,true);
 a.term.selection='different later selection';
 const menu=document.body.children.find(x=>x.className==='terminal-chat-context-menu');
 const attachSelection=menu.children.find(x=>x.textContent==='Attach selection to Chat');
 assert.ok(attachSelection);
 await attachSelection.dispatch('click');
 assert.equal(staged.length,4);
 assert.equal(staged[3].payload.kind,'terminal_selection');
 assert.match(staged[3].payload.text,/error: exact selected text/);
 assert.match(staged[3].payload.text,/SOURCE_TYPE: terminal_selection/);
 assert.match(staged[3].payload.text,/TERMINAL_ID: A-retargeted/);
 assert.match(staged[3].payload.text,/SELECTION_START_BUFFER_ROW: 10/);
 assert.doesNotMatch(staged[3].payload.text,/different later selection/);
 assert.equal(staged[3].payload.source.selection_start.buffer_row,10);
 assert.equal(module.hasSelectionMenu,false);

 // ANSI/control data from a terminal buffer is removed while Unicode and
 // line boundaries remain intact in the staged immutable snapshot.
 a.console='\u001b[31mERROR\u001b[0m — ошибка\r\nnext\u0007';
 await module.attachConsole(a);
 assert.match(staged.at(-1).payload.text,/ERROR — ошибка\nnext/);
 assert.doesNotMatch(staged.at(-1).payload.text,/\u001b|\u0007|\[31m/);

 // Missing Coder 1 API is a visible hard failure, never a fake production
 // fallback or alternate attachment route.
 const originalStage=context.TaskMenuAgentChat.stageContext;
 context.TaskMenuAgentChat.stageContext=undefined;
 await assert.rejects(()=>module.attachConsole(a),/staging is not available/);
 context.TaskMenuAgentChat.stageContext=originalStage;

 // No selection leaves native/browser/xterm context menu untouched.
 a.term.selection='';
 const noSelection={clientX:5,clientY:5,preventDefault(){this.prevented=true;},stopPropagation(){this.stopped=true;}};
 a.term.element.dispatch('contextmenu',noSelection);
 assert.equal(noSelection.prevented,undefined);
 // Clone/restore lifecycle notifications remain idempotent.
 for(const eventName of ['taskmenu:terminal-cloned','taskmenu:terminal-restored','taskmenu:tasks']){
  for(const handler of window.listeners[eventName]||[])handler({detail:{view:a}});
  module.installView(a);
  assert.equal(a.pane.popover.children.filter(x=>x.className==='terminal-chat-attach-console').length,1,'lifecycle duplicated Console action: '+eventName);
 }

 // Long console requires an explicit user choice; chosen tail is line-bounded
 // and truncation metadata is present. Cancel produces no attachment.
 let prompts=0;window.prompt=(message,initial)=>{prompts++;return initial;};
 a.console='row\n'.repeat(18000);
 const before=staged.length;
 await module.attachConsole(a);
 assert.equal(prompts,1,'oversized console must require user input');
 assert.equal(staged.length,before+1);
 assert.match(staged.at(-1).payload.text,/TRUNCATED: true/);
 assert.match(staged.at(-1).payload.text,/CAPTURE_MODE: recent_tail/);
 assert.match(staged.at(-1).payload.text,/ORIGINAL_BYTES: \d+/);
 assert.match(staged.at(-1).payload.text,/ATTACHED_BYTES: \d+/);
 assert.match(staged.at(-1).payload.text,/OMITTED_PREFIX_BYTES: \d+/);
 assert.equal(staged.at(-1).payload.source.truncated,true);
 assert.equal(staged.at(-1).payload.source.capture_mode,'recent_tail');
 assert.ok(new TextEncoder().encode(staged.at(-1).payload.text).length<=32*1024);
 // Empty console and non-text/invalid UTF-8 selection fail before staging.
 a.console='';
 await assert.rejects(()=>module.attachConsole(a),/no console output/);
 a.console='safe';
 const beforeSanitize=staged.length;
 await module.attachSelection(a,'\\u0000bad',{});
 assert.equal(staged.length,beforeSanitize+1);
 assert.doesNotMatch(staged.at(-1).payload.text,/\u0000/);
 // Restore the long console snapshot before checking cancel behavior.
 a.console='row\\n'.repeat(18000);
 window.prompt=()=>null;
 const beforeCancel=staged.length;
 await module.attachConsole(a);
 assert.equal(staged.length,beforeCancel,'canceling oversized selection must not stage');

 // Closed/removed view is rejected and its action is removed.
 a.closed=true;module.installView(a);
 assert.throws(()=>module.captureConsole(a),/closed or not readable/);
 // MIME/UTF-8 and target API failure paths are explicit, not silent.
 const originalStageAfterClosed=context.TaskMenuAgentChat.stageContext;
 context.TaskMenuAgentChat.stageContext=async()=>({ok:false,error:'staging unavailable'});
 const failed=makeView('stage-fail','output\n');
 await assert.rejects(()=>module.attachConsole(failed),/staging unavailable/);
 context.TaskMenuAgentChat.stageContext=originalStageAfterClosed;
 const unicode=makeView('unicode','e\u0301 😀\r\nline\tindent\n');
 unicode.term.selection='  α\r\n\tβ  ';
 await module.attachSelection(unicode,unicode.term.selection,{});
 assert.equal(staged.at(-1).payload.kind,'terminal_selection');
 assert.match(staged.at(-1).payload.text,/  α\r\n\tβ  /,'selection whitespace/newline must be preserved');
 console.log('PASS: terminal console/selection snapshots, bounds, Unicode and staging errors');
})().catch(error=>{console.error(error);process.exitCode=1;});
