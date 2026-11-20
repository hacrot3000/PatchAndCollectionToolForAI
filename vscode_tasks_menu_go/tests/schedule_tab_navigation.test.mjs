import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const ui=readFileSync(new URL('../web/featuremods/schedulespanel.js',import.meta.url),'utf8');

const section=(start,end)=>{
  const a=ui.indexOf(start),b=ui.indexOf(end,a+start.length);
  assert.ok(a!==-1&&b>a,'Cannot extract '+start);
  return ui.slice(a,b);
};
const navigation=section('function syncScheduleActiveTab(){','function schedulePreferredGitRepo(){');
const closing=section('function closeTab(){','function install(){');

function classSet(initial=[]){
 const values=new Set(initial);
 return {values,contains:key=>values.has(key),add:key=>values.add(key),
  remove:key=>values.delete(key),toggle:(key,on)=>{
   if(on===undefined)on=!values.has(key);
   if(on)values.add(key);else values.delete(key);
   return Boolean(on);
  }};
}
function fixture({sidebarVisible=true,active='terminal-123',returnView='',lock=''}={}){
 const events=[],refreshed=[],loads=[],calls=[];
 const tab={hidden:true,dataset:{id:'schedules'},classList:classSet(),
  attrs:{},setAttribute(k,v){this.attrs[k]=v}};
 const workspace={classList:classSet()};
 const panel={classList:classSet(sidebarVisible?['visible']:[])};
 const terminal={};
 const app={active,views:new Map([['terminal-123',terminal]]),locked:lock,
  activateExternalView(id){
   this.active='external:'+id;
   calls.push('activateExternal:'+id);
   ctx.syncScheduleActiveTab();
   return true;
  },
  activateView(id){this.active=id;calls.push('activateTerminal:'+id);ctx.syncScheduleActiveTab();return true;},
  claimForegroundView(target){this.locked=target;},
  releaseForegroundView(target){if(this.locked===target)this.locked='';},
  showError(){},
 };
 const data={tab,workspace,panel};
 const listeners=[];
 const window={dispatchEvent(event){events.push(event.type)},addEventListener(...args){listeners.push(args)}};
 const document={querySelectorAll(){return []}};
 const ctx=vm.createContext({app,data,window,document,Event:class Event{constructor(type){this.type=type}},
  scheduleRepoDiscoveryLoaded:true,scheduleReturnView:returnView,
  load:id=>loads.push(id),
  discoverScheduleRepos:()=>Promise.resolve(),
  refresh:()=>{refreshed.push(true);return Promise.resolve()},
  closePanel:()=>{panel.classList.remove('visible');events.push('sidebar-close')},
 });
 vm.runInContext(navigation+closing,ctx);
 return {ctx,data,app,events,refreshed,loads,calls};
}

test('opening a schedule activates and marks Schedule tab, deactivating terminal',()=>{
 const f=fixture();
 f.ctx.openTab('scheduled-job');
 assert.equal(f.app.active,'external:schedules');
 assert.equal(f.data.tab.hidden,false);
 assert.equal(f.data.tab.classList.contains('active'),true);
 assert.equal(f.data.tab.attrs['aria-selected'],'true');
 assert.equal(f.data.workspace.classList.contains('visible'),true);
 assert.deepEqual(f.loads,['scheduled-job']);
});

test('Schedule sidebar stays open when clicking a schedule from its list',()=>{
 const f=fixture();
 f.ctx.openTab('scheduled-job');
 assert.equal(f.data.panel.classList.contains('visible'),true);
 assert.equal(f.events.includes('sidebar-close'),false);
});

test('Schedule sidebar stays open when clicking a run history item',()=>{
 // Recent-run item resolves its parent schedule then loads its console.
 const f=fixture();
 f.ctx.openTab('scheduled-job');
 assert.equal(f.data.panel.classList.contains('visible'),true);
 assert.ok(ui.includes('button.onclick=()=>{openTab(run.schedule_id);data.run=run.id;void consoleLog(run.id);};'));
 assert.equal(f.events.includes('sidebar-close'),false);
});

test('external tab switching removes active class and hides Schedule workspace',()=>{
 const f=fixture();
 f.ctx.openTab('scheduled-job');
 f.app.active='terminal-123';
 f.ctx.syncScheduleActiveTab();
 assert.equal(f.data.tab.classList.contains('active'),false);
 assert.equal(f.data.tab.attrs['aria-selected'],'false');
 assert.equal(f.data.workspace.classList.contains('visible'),false);
 f.app.active='external:schedules';
 f.ctx.syncScheduleActiveTab();
 assert.equal(f.data.tab.classList.contains('active'),true);
 assert.equal(f.data.workspace.classList.contains('visible'),true);
});

test('closing Schedule restores prior terminal instead of fake external:empty',()=>{
 const f=fixture();
 f.ctx.openTab('scheduled-job');
 f.ctx.closeTab();
 assert.equal(f.data.tab.hidden,true);
 assert.equal(f.data.tab.classList.contains('active'),false);
 assert.equal(f.data.workspace.classList.contains('visible'),false);
 assert.equal(f.app.active,'terminal-123');
 assert.equal(f.app.locked,'');
 assert.equal(f.calls.includes('activateExternal:empty'),false);
});

test('closing an inactive Schedule tab does not hijack the active workspace view',()=>{
 const f=fixture();
 f.ctx.openTab('scheduled-job');
 f.app.active='other-terminal';
 f.ctx.closeTab();
 assert.equal(f.app.active,'other-terminal');
 assert.equal(f.data.tab.hidden,true);
});

test('Schedule tab registration listens to core view activation, not terminal only',()=>{
 assert.ok(ui.includes("window.addEventListener('taskmenu:view-activated',syncScheduleActiveTab)"));
 assert.ok(ui.includes("data.tab.classList.toggle('active',active)"));
 assert.ok(ui.includes("data.tab.setAttribute('aria-selected',active?'true':'false')"));
});
