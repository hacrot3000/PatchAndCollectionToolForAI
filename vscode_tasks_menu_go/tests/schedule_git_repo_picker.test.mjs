import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const ui=readFileSync(new URL('../web/featuremods/schedulespanel.js',import.meta.url),'utf8');
const from=ui.indexOf('function schedulePreferredGitRepo(){');
const to=ui.indexOf('function closeTab(){',from);
assert.ok(from>0&&to>from,'Schedule repository picker helpers must exist');
const helpers=ui.slice(from,to);

function fixture({repositories=[],defaultID='',activeID=''}={}){
 const input={value:'',dataset:{}},type={value:'git'};
 const form={dataset:{id:''}};
 const box=()=>({
  value:'',disabled:false,children:[],replaceChildren(){this.children=[];this.rebuilds=(this.rebuilds||0)+1;},
  append(child){this.children.push(child);}
 });
 const picker=box(),list=box(),status={textContent:''};
 const responses=[];
 const ctx=vm.createContext({
  scheduleGitRepos:repositories,scheduleDefaultRepoID:defaultID,
  scheduleRepoLoading:null,scheduleRepoLoadSequence:0,scheduleRepoDiscoveryLoaded:repositories.length>0,
  scheduleRepoSelect:picker,scheduleRepoList:list,scheduleRepoStatus:status,
  field:name=>name==='git_path'?input:name==='type'?type:null,
  data:{form},
  globalThis:{TaskMenuGitFiles:{snapshotState:()=>({repository_id:activeID})}},
  options:(element,rows)=>{element.replaceChildren();for(const [value,label] of rows)element.append({value,textContent:label});},
  el:()=>({value:'',label:''}),
  app:{jsonFetch:async url=>{responses.push(url);return {repositories,default_repository:defaultID};}},
 });
 vm.runInContext(helpers,ctx);
 return {ctx,input,type,form,picker,list,status,responses};
}
const repos=[
 {id:'.',path:'.',name:'Workspace'},
 {id:'projects/client',path:'projects/client',name:'Client'},
 {id:'projects/server',path:'projects/server',name:'Server'}
];

test('pre-fills Git schedule from active git-repo-select, not the first repo',()=>{
 const f=fixture({repositories:repos,defaultID:'.',activeID:'projects/server'});
 f.ctx.schedulePrefillGitPath();
 assert.equal(f.input.value,'projects/server');
 assert.equal(f.picker.value,'projects/server');
});

test('falls back to Git bar default, then first repo, when active selection absent',()=>{
 const f=fixture({repositories:repos,defaultID:'projects/client'});
 f.ctx.schedulePrefillGitPath();
 assert.equal(f.input.value,'projects/client');
 const g=fixture({repositories:repos,defaultID:'missing'});
 g.ctx.schedulePrefillGitPath();
 assert.equal(g.input.value,'.');
});

test('never replaces a manually entered path or an existing schedule',()=>{
 const f=fixture({repositories:repos,activeID:'projects/server'});
 f.input.value='hand-typed/subrepo';
 f.input.dataset.userEdited='1';
 f.ctx.schedulePrefillGitPath();
 assert.equal(f.input.value,'hand-typed/subrepo');
 f.input.value='';f.form.dataset.id='existing-schedule';
 f.ctx.schedulePrefillGitPath();
 assert.equal(f.input.value,'');
});

test('datalist choices rebuild only when repos reload, not during typing',()=>{
 const f=fixture({repositories:repos});
 f.ctx.scheduleSyncRepoSelect(true);
 assert.equal(f.list.children.length,3);
 const before=f.list.rebuilds;
 f.input.value='custom';
 f.ctx.scheduleSyncRepoSelect();
 assert.equal(f.list.rebuilds,before,'rebuilding options per keypress closes the native autocomplete');
 assert.equal(f.picker.value,'','custom path must remain intact');
});

test('first discovery fills a new Git schedule while preserving edits typed during the request',async()=>{
 const f=fixture({repositories:repos,defaultID:'projects/client',activeID:'projects/server'});
 let resolve;
 const deferred=new Promise(done=>{resolve=done;});
 f.ctx.app.jsonFetch=()=>deferred;
 const fetching=f.ctx.discoverScheduleRepos();
 f.input.value='manual/path';
 f.input.dataset.userEdited='1';
 resolve({repositories:repos,default_repository:'projects/client'});
 await fetching;
 assert.equal(f.input.value,'manual/path');
 assert.equal(f.list.children.length,3);
 assert.equal(f.ctx.scheduleRepoDiscoveryLoaded,true);
});

test('forced rescan uses the Git repository discovery refresh endpoint',async()=>{
 const f=fixture({repositories:repos,defaultID:'projects/client'});
 f.input.value='custom/root';
 await f.ctx.discoverScheduleRepos({force:true});
 assert.deepEqual(f.responses,['/api/schedules/repositories?refresh=1']);
 assert.equal(f.input.value,'custom/root');
});

test('Schedule UI provides editable input, dropdown, datalist and rescan',()=>{
 for(const clause of [
  "gitInput.setAttribute('list',scheduleRepoList.id)",
  "scheduleRepoSelect.addEventListener('change'",
  "gitInput.addEventListener('input'",
  'scheduleSyncRepoSelect(rebuild=false)',
  'schedule-repo-combo',
  'discoverScheduleRepos({force:true})'
 ])assert.ok(ui.includes(clause),clause);
});
