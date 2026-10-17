const app=globalThis.TaskMenuApp;
const style=document.createElement('style');
style.textContent=[
'.schedule-nav{display:none;position:fixed;z-index:1810;top:calc(var(--taskmenu-header-height,30px) + 46px);bottom:0;left:0;width:min(var(--taskmenu-sidebar-inline-width,310px),100vw);background:#111820;border-right:1px solid #3b4855;flex-direction:column}',
'.schedule-nav.visible{display:flex}',
'body.task-sidebar-auto-hide .schedule-nav{left:48px;top:var(--taskmenu-header-height,30px);width:min(var(--taskmenu-sidebar-panel-width,310px),calc(100vw - 48px))}',
'.schedule-nav .head{display:flex;align-items:center;gap:5px;padding:8px;border-bottom:1px solid #35404d}.schedule-nav .head strong{flex:1}.schedule-nav .head button{padding:3px 6px;font-size:11px}',
'.schedule-nav .groups{overflow:auto;padding:6px;min-height:0}.schedule-nav details{border:1px solid #374451;border-radius:5px;margin-bottom:6px}.schedule-nav summary{cursor:pointer;padding:7px;font-weight:600;font-size:12px}',
'.schedule-nav .entry{display:block;width:100%;min-width:0;text-overflow:ellipsis;overflow:hidden;white-space:nowrap;text-align:left;border:0;padding:6px 8px;background:transparent;font-size:11px}.schedule-nav .entry:hover{background:#293648}',
'.schedule-workspace[hidden],.schedule-tab[hidden]{display:none!important}.schedule-workspace{display:none;position:absolute;inset:0;z-index:22;flex-direction:column;background:#111820}.schedule-workspace.visible{display:flex}',
'.schedule-toolbar{display:flex;align-items:center;gap:8px;padding:8px 11px;border-bottom:1px solid #35404d;flex-wrap:wrap;font-size:12px}.schedule-toolbar strong{flex:1}.schedule-toolbar button{padding:4px 8px;font-size:11px}',
'.schedule-layout{display:grid;grid-template-columns:minmax(280px,415px) minmax(0,1fr);flex:1;min-height:0}',
'.schedule-form{display:grid;align-content:start;gap:8px;padding:12px;min-width:0;overflow:auto;border-right:1px solid #35404d}',
'.schedule-form label{font-size:11px;display:flex;flex-direction:column;gap:4px}.schedule-form .toggle{flex-direction:row;align-items:center}.schedule-form input,.schedule-form select,.schedule-form textarea{width:100%;min-width:0;font:12px ui-monospace,monospace;padding:6px;background:#18212d;color:inherit;border:1px solid #455262}.schedule-form .toggle input{width:auto}.schedule-form label[hidden]{display:none}',
'.schedule-repo-combo{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,.9fr) 30px;gap:5px;align-items:stretch;min-width:0}',
'.schedule-form .schedule-repo-combo input,.schedule-form .schedule-repo-combo select{width:100%;min-width:0;max-width:100%}',
'.schedule-form .schedule-repo-combo button{width:30px;min-width:0;padding:3px 0;font-size:13px;white-space:nowrap}',
'.schedule-form .actions{display:flex;gap:5px;flex-wrap:wrap}.schedule-form .actions button{font-size:11px;padding:5px 8px}.schedule-form .hint{font-size:10px;opacity:.7}',
'.schedule-toolbar button.active{background:#2d496c;color:#fff;border-color:#689ad0}',
'.schedule-toolbar .schedule-page-actions{display:flex;gap:5px;flex-wrap:wrap}',
'.schedule-output{display:flex;flex-direction:column;min-height:0;min-width:0}.schedule-runs{max-height:34%;min-height:75px;overflow:auto;border-bottom:1px solid #35404d}',
'.schedule-runs button{display:flex;width:100%;gap:8px;justify-content:space-between;padding:5px 10px;font-size:11px;text-align:left;border:0;border-radius:0;background:transparent}.schedule-runs button:hover{background:#253248}',
'.schedule-console-title{padding:7px 10px;border-bottom:1px solid #35404d;font-size:11px}.schedule-console{overflow:auto;flex:1;min-height:0;margin:0;padding:10px;background:#0b1017;white-space:pre-wrap;overflow-wrap:anywhere;font:12px/1.5 ui-monospace,monospace}',
'@media(max-width:900px){.schedule-layout{display:block;overflow:auto}.schedule-form{max-height:65vh;border-bottom:1px solid #35404d}.schedule-output{height:50vh}}',
'html[data-taskmenu-theme="light"] .schedule-nav,html[data-taskmenu-theme="light"] .schedule-workspace{background:#fff;color:#1e293b}',
'html[data-taskmenu-theme="light"] .schedule-console{background:#f8fafc;color:#1e293b}html[data-taskmenu-theme="light"] .schedule-form input,html[data-taskmenu-theme="light"] .schedule-form select,html[data-taskmenu-theme="light"] .schedule-form textarea{background:#fff;color:#1e293b}'
].join('\n');
document.head.append(style);
const el=(tag,text,cls)=>{const x=document.createElement(tag);if(text!==undefined)x.textContent=text;if(cls)x.className=cls;return x};
const types=[['multi','Multiple conditions (AND/OR)'],['cron','Cron / Timed'],['url','URL condition'],['git','Git condition'],['dependency','After schedule']];
const scheduleGroupExpanded=new Map();
let scheduleReturnView='';
let scheduleRepoDiscoveryLoaded=false;
let scheduleGitRepos=[],scheduleDefaultRepoID='',scheduleRepoLoading=null,scheduleRepoLoadSequence=0;
let scheduleRepoSelect=null,scheduleRepoList=null,scheduleRepoStatus=null;
const data={items:[],runs:[],errors:{},selected:'',run:'',page:'overview',panel:null,workspace:null,tab:null,form:null,runBox:null,console:null,consoleTitle:null,groups:null,layout:null,pageHost:null,pageButtons:null,relations:null,expression:null,expressionHost:null};
const field=name=>data.form?.elements.namedItem(name);
const post=(action,extra={})=>app.jsonFetch('/api/schedules',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action,...extra})});
function defaultSchedule(){return {name:'',enabled:false,type:'multi',cron:'*/5 * * * *',timezone:'Local',interval_seconds:60,url:'',match_mode:'contains',match_text:'',json_path:'',git_path:'',git_ref:'HEAD',git_condition:'change',parent_id:'',parent_state:'success',exit_code:null,output_contains:'',output_absent:'',target:'command',task_id:0,command:'',working_directory:'',allow_overlap:false,retain_days:7,retain_runs:30,timeout_seconds:1800,condition_group:{operator:'and',conditions:[globalThis.TaskMenuScheduleConditions.empty()],groups:[]}};}
function options(element,values){element.replaceChildren();for(const [id,name] of values){const option=el('option',name);option.value=id;element.append(option);}}
function label(name,caption,type='text',values=[]){
 const wrap=el('label');wrap.dataset.field=name;const title=el('span',caption);
 const input=type==='textarea'?el('textarea'):type==='select'?el('select'):el('input');
 input.name=name;
 if(type==='select')options(input,values);
 else if(type!=='textarea')input.type=type;
 if(type==='checkbox'){wrap.className='toggle';wrap.append(input,title);}else wrap.append(title,input);
 data.form.append(wrap);return input;
}
function updateFields(){
 const type=field('type').value,target=field('target').value,match=field('match_mode').value;
 if(data.expressionHost)data.expressionHost.hidden=type!=='multi';
 if(type==='git'){
  if(scheduleRepoDiscoveryLoaded)schedulePrefillGitPath();
  else if(!scheduleRepoLoading)void discoverScheduleRepos().catch(app.showError);
 }
 const visible={cron:type==='cron',timezone:type==='cron',
 interval_seconds:type==='url'||type==='git',url:type==='url',match_mode:type==='url',
 match_text:type==='url',json_path:type==='url'&&(match==='json'||match==='xml'),
 git_path:type==='git',git_ref:type==='git',git_condition:type==='git',
 parent_id:type==='dependency',parent_state:type==='dependency',
 exit_code:type==='dependency',output_contains:type==='dependency',output_absent:type==='dependency',
 task_id:target==='task',command:target==='command',working_directory:target==='command'};
 for(const item of data.form.querySelectorAll('label[data-field]')){
  const name=item.dataset.field;
  if(Object.prototype.hasOwnProperty.call(visible,name))item.hidden=!visible[name];
 }
}
function load(id=''){
 data.selected=id;
 const item=data.items.find(x=>x.id===id)||defaultSchedule();
 data.form.dataset.id=item.id||'';
 options(field('parent_id'),[['','Choose parent'],...data.items.filter(x=>x.id!==id).map(x=>[x.id,x.name])]);
 const tasks=Array.isArray(app.taskData?.tasks)?app.taskData.tasks:[];
 options(field('task_id'),[['','Choose task from tasks.json'],...tasks.map(x=>[String(x.id),String(x.label||x.menu_label||x.id)])]);
 for(const [key,value] of Object.entries(item)){
  const input=field(key);if(!input)continue;
  if(input.type==='checkbox')input.checked=Boolean(value);
  else input.value=value==null?'':String(value);
 }
 const gitInput=field('git_path');if(gitInput)delete gitInput.dataset.userEdited;
 if(data.expression){
  const group=item.condition_group||globalThis.TaskMenuScheduleConditions.legacy(item);
  data.expression.load(group,data.items,scheduleGitRepos,item.id||'');
 }
 if(data.relations)globalThis.TaskMenuScheduleViews.relationships(data.relations,item,data.items,id=>openTab(id,'edit'));
 updateFields();renderRuns();
}
function getDefinition(){
 const str=k=>field(k).value,number=k=>Number(str(k))||0;
 const model={...defaultSchedule(),id:data.form.dataset.id||'',
 name:str('name').trim(),enabled:field('enabled').checked,type:str('type'),
 cron:str('cron'),timezone:str('timezone')||'Local',interval_seconds:number('interval_seconds')||60,
 url:str('url'),match_mode:str('match_mode'),match_text:str('match_text'),json_path:str('json_path'),
 git_path:str('git_path'),git_ref:str('git_ref'),git_condition:str('git_condition'),
 parent_id:str('parent_id'),parent_state:str('parent_state'),exit_code:str('exit_code')===''?null:number('exit_code'),
 output_contains:str('output_contains'),output_absent:str('output_absent'),
 target:str('target'),task_id:number('task_id'),command:str('command'),working_directory:str('working_directory'),
 allow_overlap:field('allow_overlap').checked,retain_days:number('retain_days')||7,
 retain_runs:number('retain_runs')||30,timeout_seconds:number('timeout_seconds')||1800};
 model.condition_group=model.type==='multi'?data.expression?.value()||null:null;
 return model;
}
function entries(parent,title,items){
 const details=el('details');details.open=scheduleGroupExpanded.get(title)!==false;details.append(el('summary',title+' ('+items.length+')'));
 details.addEventListener('toggle',()=>scheduleGroupExpanded.set(title,details.open));
 for(const schedule of items){const button=el('button',(schedule.enabled?'● ':'○ ')+schedule.name,'entry');
  button.title=data.errors[schedule.id]||schedule.name;button.onclick=()=>openTab(schedule.id,'edit');details.append(button);}
 parent.append(details);
}
function renderPanel(){
 if(!data.groups)return;
 const oldScroll=data.groups.scrollTop;
 data.groups.replaceChildren();
 const active=new Set(data.runs.filter(r=>r.status==='running').map(r=>r.schedule_id));
 entries(data.groups,'Running',data.items.filter(x=>active.has(x.id)));
 for(const [kind,name] of types)entries(data.groups,name,data.items.filter(x=>x.type===kind));
 entries(data.groups,'Disabled',data.items.filter(x=>!x.enabled));
 const recent=el('details');recent.open=true;recent.append(el('summary','Recent runs ('+data.runs.length+')'));
 for(const run of data.runs.slice(0,35)){
  const button=el('button',run.status+' · '+run.name,'entry');
  button.onclick=()=>{openTab(run.schedule_id,'edit');data.run=run.id;void consoleLog(run.id);};
  recent.append(button);
 }data.groups.append(recent);
 data.groups.scrollTop=oldScroll;
}
function renderRuns(){
 if(!data.runBox)return;
 const oldScroll=data.runBox.scrollTop;
 data.runBox.replaceChildren();
 data.runBox.append(el('div','Run history · '+data.runs.length,'schedule-console-title'));
 const subset=data.runs.filter(r=>!data.selected||r.schedule_id===data.selected);
 for(const run of subset.slice(0,125)){
  const button=el('button',run.name+' · '+new Date(run.started_at).toLocaleString());
  button.append(el('strong',run.status+(run.status==='failure'?' ('+run.exit_code+')':'')));
  button.title=run.reason||'';
  button.onclick=()=>{data.run=run.id;void consoleLog(run.id);};
  data.runBox.append(button);
 }
 data.runBox.scrollTop=oldScroll;
}
async function consoleLog(id){
 const run=data.runs.find(x=>x.id===id);if(!run)return;
 data.consoleTitle.textContent=run.name+' · '+run.status+' · '+run.reason;
 try{
  const response=await fetch('/api/schedules/log?run_id='+encodeURIComponent(id),{cache:'no-store',credentials:'same-origin'});
  if(!response.ok)throw Error('Log HTTP '+response.status);
  data.console.textContent=await response.text();
 }catch(err){data.console.textContent=String(err)}
}
async function refresh(){
 const result=await app.jsonFetch('/api/schedules',{cache:'no-store'});
 data.items=Array.isArray(result.schedules)?result.schedules:[];
 data.runs=Array.isArray(result.runs)?result.runs:[];
 data.errors=result.trigger_errors||{};
 renderPanel();renderRuns();
 if(data.page!=='edit')scheduleShowPage(data.page);
 if(data.run)void consoleLog(data.run);
}
function closePanel(){
 if(data.panel?.classList.contains('visible')){
  data.panel.classList.remove('visible');
  window.dispatchEvent(new CustomEvent('taskmenu:schedules-sidebar-visible',{detail:{visible:false}}));
 }
}
function openPanel(){
 if(!data.panel)return false;
 data.panel.classList.add('visible');
 window.dispatchEvent(new CustomEvent('taskmenu:schedules-sidebar-visible',{detail:{visible:true}}));
 void refresh().catch(app.showError);
 return true;
}
function syncScheduleActiveTab(){
 if(!data.workspace||!data.tab)return;
 const active=String(app.active||'')==='external:schedules'&&!data.tab.hidden;
 data.tab.classList.toggle('active',active);
 data.tab.setAttribute('aria-selected',active?'true':'false');
 data.workspace.classList.toggle('visible',active);
}
function scheduleShowPage(page='overview'){
 if(!data.workspace||!data.pageHost)return;
 if(!['overview','runs','graph','edit'].includes(page))page='overview';
 data.page=page;
 data.layout.hidden=page!=='edit';
 data.pageHost.hidden=page==='edit';
 for(const button of data.pageButtons.querySelectorAll('button[data-page]')){
  const active=button.dataset.page===page;
  button.classList.toggle('active',active);button.setAttribute('aria-pressed',active?'true':'false');
 }
 if(page==='edit')return;
 // Live refresh must not wipe a search/filter or jump the user to the top.
 const query=data.pageHost.querySelector('.schedule-main-title input');
 const select=data.pageHost.querySelector('.schedule-main-title select');
 const previousQuery=query?.value||'',previousFilter=select?.value||'';
 const previousScroll=data.pageHost.querySelector('.schedule-main-scroll, .schedule-graph-scroll')?.scrollTop||0;
 globalThis.TaskMenuScheduleViews.render(data.pageHost,page,{
  schedules:data.items,runs:data.runs,errors:data.errors,
  open:id=>openTab(id,'edit'),
  openRun:(runID,scheduleID)=>{openTab(scheduleID,'edit');data.run=runID;void consoleLog(runID)},
  execute:async id=>{try{const result=await post('run',{id});data.run=result.run_id;await refresh()}catch(error){app.showError(error)}},
  stop:async id=>{try{await post('stop',{run_id:id});await refresh()}catch(error){app.showError(error)}},
  toggle:async item=>{try{await post('upsert',{schedule:{...item,enabled:!item.enabled}});await refresh()}catch(error){app.showError(error)}},
  remove:async item=>{try{if(!confirm('Delete schedule '+item.name+'?'))return;await post('delete',{id:item.id});await refresh()}catch(error){app.showError(error)}},
 });
 const nextQuery=data.pageHost.querySelector('.schedule-main-title input');
 const nextFilter=data.pageHost.querySelector('.schedule-main-title select');
 if(nextQuery&&previousQuery){nextQuery.value=previousQuery;nextQuery.dispatchEvent(new Event('input'))}
 if(nextFilter&&previousFilter){nextFilter.value=previousFilter;nextFilter.dispatchEvent(new Event('change'))}
 const nextScroll=data.pageHost.querySelector('.schedule-main-scroll, .schedule-graph-scroll');
 if(nextScroll)nextScroll.scrollTop=previousScroll;
}
function openTab(id='',page=''){
 if(!data.workspace)return false;
 if(!scheduleRepoDiscoveryLoaded)void discoverScheduleRepos().catch(app.showError);
 // Do not close the Schedule sidebar when a schedule or run is selected.
 // It is navigation, just like Explorer, and must remain available.
 const previous=String(app.active||'');
 if(previous&&previous!=='external:schedules')scheduleReturnView=previous;
 data.tab.hidden=false;
 if(app.activateExternalView('schedules',{force:true})===false){
  syncScheduleActiveTab();return false;
 }
 app.claimForegroundView?.('external:schedules');
 syncScheduleActiveTab();
 if(id)load(id);
 scheduleShowPage(page||(id?'edit':'overview'));
 window.dispatchEvent(new Event('taskmenu:workspace-tab-changed'));
 void refresh().catch(app.showError);return true;
}
function schedulePreferredGitRepo(){
 // Git Quick Actions and Schedule both use discoverGitRepositories. Prefer
 // the currently selected Git-bar repository, then the workspace default.
 const activeID=String(globalThis.TaskMenuGitFiles?.snapshotState?.()?.repository_id||'');
 return scheduleGitRepos.find(repo=>repo.id===activeID)
   ||scheduleGitRepos.find(repo=>repo.id===scheduleDefaultRepoID)
   ||scheduleGitRepos[0]||null;
}
function scheduleSyncRepoSelect(rebuild=false){
 const input=field('git_path');
 if(!input||!scheduleRepoSelect||!scheduleRepoList)return;
 const current=String(input.value||'').trim();
 if(rebuild){
  // Rebuilding datalist on every keystroke closes browser suggestion popups.
  scheduleRepoList.replaceChildren();
  options(scheduleRepoSelect,[['','Choose repo…'],...scheduleGitRepos.map(repo=>[
   repo.path,(repo.name||repo.id)+' · '+repo.path
  ])]);
  for(const repo of scheduleGitRepos){
   const option=el('option');
   option.value=repo.path;
   option.label=(repo.name||repo.id)+(repo.path==='.'?' · workspace root':' · '+repo.path);
   scheduleRepoList.append(option);
  }
 }
 scheduleRepoSelect.value=scheduleGitRepos.some(repo=>repo.path===current)?current:'';
 scheduleRepoSelect.disabled=!scheduleGitRepos.length;
 scheduleRepoSelect.title=scheduleGitRepos.length
  ?'Select a discovered Git repo, or type any relative path in the editable field'
  :'No repositories detected; enter the repository root manually';
}
function schedulePrefillGitPath(){
 const input=field('git_path');
 if(!input||field('type')?.value!=='git'||data.form?.dataset.id||input.dataset.userEdited==='1'||input.value.trim())return;
 const preferred=schedulePreferredGitRepo();
 if(!preferred)return;
 input.value=preferred.path;
 scheduleSyncRepoSelect();
}
async function discoverScheduleRepos({force=false}={}){
 if(scheduleRepoLoading&&!force)return scheduleRepoLoading;
 const requestNumber=++scheduleRepoLoadSequence;
 if(scheduleRepoStatus)scheduleRepoStatus.textContent='Scanning Git repositories…';
 const url='/api/schedules/repositories'+(force?'?refresh=1':'');
 const pending=app.jsonFetch(url,{cache:'no-store'}).then(result=>{
  if(requestNumber!==scheduleRepoLoadSequence)return;
  scheduleGitRepos=(Array.isArray(result?.repositories)?result.repositories:[])
   .filter(repo=>repo&&typeof repo.path==='string'&&repo.path!=='');
  scheduleDefaultRepoID=String(result?.default_repository||'');
  scheduleRepoDiscoveryLoaded=true;
  data.expression?.setRepositories(scheduleGitRepos);
  scheduleSyncRepoSelect(true);
  schedulePrefillGitPath();
  if(scheduleRepoStatus){
   scheduleRepoStatus.textContent=scheduleGitRepos.length
    ?scheduleGitRepos.length+' detected · choose a repo or type a custom directory'
    :'No repositories found · type the repository path or press Rescan';
  }
 }).catch(error=>{
  if(requestNumber===scheduleRepoLoadSequence){
   scheduleRepoDiscoveryLoaded=false;
   if(scheduleRepoStatus)scheduleRepoStatus.textContent='Repo scan failed · manual path entry is still available';
  }
  throw error;
 }).finally(()=>{if(scheduleRepoLoading===pending)scheduleRepoLoading=null;});
 scheduleRepoLoading=pending;
 return pending;
}
function closeTab(){
 if(!data.tab)return;
 const wasActive=String(app.active||'')==='external:schedules';
 data.tab.hidden=true;
 app.releaseForegroundView?.('external:schedules');
 syncScheduleActiveTab();
 window.dispatchEvent(new Event('taskmenu:workspace-tab-changed'));
 if(!wasActive)return;
 // Restore a real workspace tab; do not activate a fake external:empty view.
 const wanted=String(scheduleReturnView||'');
 scheduleReturnView='';
 if(wanted&&!wanted.startsWith('external:')&&app.views.has(wanted)){
  if(app.activateView(wanted,{force:true})!==false)return;
 }
 if(wanted.startsWith('external:')){
  const id=wanted.slice('external:'.length);
  const target=[...document.querySelectorAll('#tabs>[data-id]')].find(tab=>tab.dataset.id===id&&!tab.hidden);
  if(target){target.click();return;}
 }
 const first=[...app.views.keys()][0];
 if(first)app.activateView(first,{force:true});
}
function install(){
 if(data.panel||!app?.taskData?.workspace)return;
 const tabs=document.querySelector('#tabs'),panes=document.querySelector('#panes');if(!tabs||!panes)return;
 data.panel=el('div',undefined,'schedule-nav');const head=el('div',undefined,'head');
 head.append(el('strong','SCHEDULES'));
 const add=el('button','+ New');add.onclick=()=>{openTab('','edit');load('')};
 const reload=el('button','↻');reload.onclick=()=>refresh().catch(app.showError);
 const close=el('button','×');close.onclick=closePanel;head.append(add,reload,close);
 data.groups=el('div',undefined,'groups');data.panel.append(head,data.groups);document.body.append(data.panel);
 data.tab=el('button',undefined,'tab schedule-tab');data.tab.dataset.id='schedules';data.tab.dataset.viewKind='external';data.tab.hidden=true;
 data.tab.append(el('span','Schedule'));
 const tabClose=el('span','×','close');data.tab.append(tabClose);tabs.append(data.tab);
 data.tab.onclick=()=>openTab();tabClose.onclick=event=>{event.stopPropagation();closeTab()};
 globalThis.TaskMenuTabContext?.registerTab?.(data.tab);
 data.workspace=el('div',undefined,'pane schedule-workspace');data.workspace.dataset.id='schedules';data.workspace.dataset.viewKind='external';
 const toolbar=el('div',undefined,'schedule-toolbar');toolbar.append(el('strong','SCHEDULES'));
 data.pageButtons=el('div',undefined,'schedule-page-actions');
 for(const [key,title] of [['overview','All schedules'],['runs','All runs'],['graph','Dependency graph'],['edit','Details / Edit']]){
  const button=el('button',title);button.type='button';button.dataset.page=key;
  button.onclick=()=>scheduleShowPage(key);
  data.pageButtons.append(button);
 }
 toolbar.append(data.pageButtons);
 const refreshButton=el('button','Refresh');refreshButton.onclick=()=>refresh().catch(app.showError);
 const newButton=el('button','+ New');newButton.onclick=()=>{load('');scheduleShowPage('edit')};
 const closeButton=el('button','Close');closeButton.onclick=closeTab;
 toolbar.append(refreshButton,newButton,closeButton);
 const layout=el('div',undefined,'schedule-layout');data.layout=layout;
 data.pageHost=el('section',undefined,'schedule-main-page');data.pageHost.hidden=true;
 data.form=el('form',undefined,'schedule-form');
 data.relations=el('div',undefined,'schedule-relation-panel');data.form.append(data.relations);
 label('name','Name');label('enabled','Enabled','checkbox');
 label('type','Trigger type','select',types);
 data.expressionHost=el('div',undefined,'schedule-expression-host');
 data.expression=globalThis.TaskMenuScheduleConditions.create(data.expressionHost,{schedules:[],repositories:[]});
 data.form.append(data.expressionHost);
 label('cron','Cron expression (5 fields)');label('timezone','Timezone (IANA or Local)');
 label('interval_seconds','Poll interval seconds (min 30)','number');
 label('url','URL (http/https public)');
 label('match_mode','URL response match','select',[['contains','Text contains'],['equals','Exact text'],['json','JSON path equals'],['xml','XML path contains']]);
 label('match_text','Match value / keyword');label('json_path','JSON dot path / XML element path');
 const gitInput=label('git_path','Git repository directory (relative to workspace)');
 const gitField=gitInput.closest('label');
 gitInput.placeholder='e.g. ., apps/client, packages/server';
 gitInput.autocomplete='off';
 gitInput.title='Git repository root relative to workspace; custom entries are allowed';
 const repoWrapper=el('div',undefined,'schedule-repo-combo');
 scheduleRepoSelect=el('select');
 scheduleRepoSelect.setAttribute('aria-label','Detected Git repositories');
 scheduleRepoSelect.title='Select a repository discovered by Git Quick Actions';
 scheduleRepoList=el('datalist');
 scheduleRepoList.id='taskdeck-schedule-git-repositories';
 gitInput.setAttribute('list',scheduleRepoList.id);
 const repoScan=el('button','↻');repoScan.type='button';
 repoScan.title='Rescan repositories using Git Quick Actions discovery';
 repoWrapper.append(gitInput,scheduleRepoSelect,repoScan);
 gitField.append(repoWrapper,scheduleRepoList);
 scheduleRepoStatus=el('span','Choose a repository or type its relative directory','hint');
 gitField.append(scheduleRepoStatus);
 gitInput.addEventListener('input',()=>{gitInput.dataset.userEdited='1';scheduleSyncRepoSelect();});
 scheduleRepoSelect.addEventListener('change',()=>{
  if(!scheduleRepoSelect.value)return;
  gitInput.value=scheduleRepoSelect.value;
  gitInput.dataset.userEdited='1';
  scheduleSyncRepoSelect();
 });
 repoScan.onclick=()=>{void discoverScheduleRepos({force:true}).catch(app.showError);};
 scheduleSyncRepoSelect();
 label('git_ref','Git ref (HEAD, branch)');label('git_condition','Git trigger','select',[['change','New commit SHA'],['dirty','Tracked changes present'],['clean','Working tree clean']]);
 label('parent_id','Parent schedule','select');label('parent_state','Parent outcome','select',[['success','Successful'],['failure','Failed'],['any','Any']]);
 label('exit_code','Specific exit code (blank = any)','number');
 label('output_contains','Console must contain');label('output_absent','Console must not contain');
 label('target','Run target','select',[['command','Shell command'],['task','tasks.json task']]);
 label('task_id','Task ID','select');label('command','Command','textarea');label('working_directory','Working directory');
 label('allow_overlap','Allow overlapping runs','checkbox');
 label('retain_days','Retain at least N days','number');label('retain_runs','Retain at least N runs','number');label('timeout_seconds','Timeout seconds','number');
 data.form.append(el('div','Retention prunes only when age AND count limits are exceeded. New schedules are disabled by default.','hint'));
 const actions=el('div',undefined,'actions');
 const save=el('button','Save');save.type='submit';
 const run=el('button','Run now');run.type='button';
 run.onclick=async()=>{try{const id=data.form.dataset.id;if(!id)throw Error('Save this schedule first');const result=await post('run',{id});data.run=result.run_id;await refresh();}catch(err){app.showError(err)}};
 const stop=el('button','Stop selected run');stop.type='button';
 stop.onclick=async()=>{try{if(!data.run)throw Error('Select a run');await post('stop',{run_id:data.run});await refresh();}catch(err){app.showError(err)}};
 const del=el('button','Delete');del.type='button';
 del.onclick=async()=>{try{const id=data.form.dataset.id;if(!id)return;if(!confirm('Delete schedule?'))return;await post('delete',{id});data.selected='';await refresh();load('');}catch(err){app.showError(err)}};
 actions.append(save,run,stop,del);data.form.append(actions);
 data.form.onsubmit=async event=>{event.preventDefault();try{const result=await post('upsert',{schedule:getDefinition()});data.selected=result.schedule.id;await refresh();load(data.selected);}catch(err){app.showError(err)}};
 for(const k of ['type','target','match_mode'])field(k).addEventListener('change',updateFields);
 const output=el('div',undefined,'schedule-output');data.runBox=el('div',undefined,'schedule-runs');
 data.consoleTitle=el('div','Choose a run to display the console','schedule-console-title');
 data.console=el('pre',undefined,'schedule-console');output.append(data.runBox,data.consoleTitle,data.console);
 layout.append(data.form,output);data.workspace.append(toolbar,layout,data.pageHost);panes.append(data.workspace);
 scheduleShowPage('overview');
 load('');
 window.addEventListener('taskmenu:view-activated',syncScheduleActiveTab);
 setInterval(()=>{if(data.workspace.classList.contains('visible')||data.panel.classList.contains('visible'))void refresh().catch(()=>{});},4000);
}
if(app?.taskData?.workspace)install();else window.addEventListener('taskmenu:tasks',install,{once:true});
globalThis.TaskMenuSchedules={openPanel,closePanel,open:openTab,get panel(){return data.panel},get visible(){return !!data.panel?.classList.contains('visible')}};
