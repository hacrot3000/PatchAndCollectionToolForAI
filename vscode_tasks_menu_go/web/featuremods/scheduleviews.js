// Schedule overview, all-run history and branched dependency DAG.
// Read-only presentation of schedule state; mutations go through callbacks.
const scheduleViewsStyles=document.createElement('style');
scheduleViewsStyles.textContent=[
'.schedule-main-page{flex:1;min-height:0;overflow:auto;padding:12px;display:flex;flex-direction:column;gap:12px}',
'.schedule-main-page[hidden],.schedule-layout[hidden]{display:none!important}',
'.schedule-main-title{display:flex;align-items:center;gap:8px;flex-wrap:wrap}',
'.schedule-main-title strong{font-size:14px;flex:1}',
'.schedule-main-title input,.schedule-main-title select{font-size:12px;min-width:120px;max-width:260px;padding:5px}',
'.schedule-main-table{width:100%;border-collapse:collapse;font:12px/1.5 system-ui,sans-serif}',
'.schedule-main-table th,.schedule-main-table td{padding:8px 9px;border-bottom:1px solid #394756;text-align:left;vertical-align:top}',
'.schedule-main-table th{position:sticky;top:0;background:#1c2633;z-index:1;white-space:nowrap}',
'.schedule-main-table td button{font-size:11px;padding:3px 7px;margin:0 3px 3px 0}',
'.schedule-main-table td .mono{font:11px ui-monospace,monospace;overflow-wrap:anywhere}',
'.schedule-main-scroll{overflow:auto;flex:1;min-height:0}',
'.schedule-main-hint{opacity:.72;font-size:11px}',
'.schedule-relation-panel{border:1px solid #465365;border-radius:6px;padding:8px;display:flex;flex-direction:column;gap:6px}',
'.schedule-relation-panel .links{display:flex;gap:5px;flex-wrap:wrap;align-items:center}',
'.schedule-relation-panel button{font-size:11px;padding:3px 7px}',
'.schedule-graph-scroll{flex:1;overflow:auto;min-height:270px;border:1px solid #36475a;border-radius:6px;background:#101823}',
'.schedule-graph-svg{display:block}.schedule-graph-node{cursor:pointer}.schedule-graph-node:focus{outline:2px solid #8eb6ff;outline-offset:3px}',
'.schedule-graph-node rect{fill:#26364b;stroke:#7c97b9;stroke-width:1.5}',
'.schedule-graph-node text{fill:#ecf3ff;font:12px system-ui,sans-serif;pointer-events:none}',
'.schedule-graph-line{stroke:#7996b9;stroke-width:1.7;fill:none;opacity:.8}',
'html[data-taskmenu-theme="light"] .schedule-main-table th{background:#edf1f5}',
'html[data-taskmenu-theme="light"] .schedule-graph-scroll{background:#f9fafc}',
'html[data-taskmenu-theme="light"] .schedule-graph-node rect{fill:#dce9f9;stroke:#657f9d}',
'html[data-taskmenu-theme="light"] .schedule-graph-node text{fill:#1d344c}'
].join('\n');
document.head.append(scheduleViewsStyles);

function scheduleParents(def){
 const group=def.condition_group;
 if(group)return globalThis.TaskMenuScheduleConditions.parents(group);
 return def.type==='dependency'&&def.parent_id?[def.parent_id]:[];
}
function scheduleSummary(def,schedules=[]){
 const expr=def.condition_group;
 if(expr){
  let n=0;const parts=[];
  const walk=g=>{n+=(g.conditions||[]).length;for(const c of g.conditions||[])parts.push(c.type);for(const child of g.groups||[])walk(child)};
  walk(expr);return n+' conditions · '+(expr.operator==='and'?'AND':'OR')+' · '+[...new Set(parts)].join(', ');
 }
 if(def.type==='cron')return def.cron+' · '+(def.timezone||'Local');
 if(def.type==='git')return def.git_path+' · '+(def.git_condition||'change')+' · '+(def.git_ref||'HEAD');
 if(def.type==='file')return def.file_path+' · '+(def.file_condition||'change');
 if(def.type==='url')return def.url+' · '+def.match_mode;
 if(def.type==='dependency'){
  const parent=schedules.find(item=>item.id===def.parent_id);
  return 'after '+(parent?.name||'Unknown schedule')+' ('+(def.parent_id||'unknown ID')+') · '+(def.parent_state||'success');
 }
 return def.type||'Unknown';
}
function scheduleTriggerReferences(def,schedules){
 const byID=new Map(schedules.map(s=>[s.id,s.name]));
 const references=scheduleParents(def);
 return [...new Set(references)].map(id=>
  (byID.get(id)||'Unknown schedule')+' ('+id+')'
 );
}
function scheduleDependencyGraph(schedules){
 const nodes=new Map(schedules.map(s=>[s.id,s]));
 const parents=new Map(),children=new Map();
 for(const def of schedules){
  const refs=[...new Set(scheduleParents(def))].filter(id=>nodes.has(id));
  parents.set(def.id,refs);
  for(const id of refs){if(!children.has(id))children.set(id,[]);children.get(id).push(def.id)}
 }
 const depth=new Map(),visiting=new Set();
 const layer=id=>{
  if(depth.has(id))return depth.get(id);
  if(visiting.has(id))return 0;
  visiting.add(id);
  const own=parents.get(id)||[];
  const d=own.length?Math.min(32,Math.max(...own.map(p=>layer(p)))+1):0;
  visiting.delete(id);depth.set(id,d);return d;
 };
 for(const def of schedules)layer(def.id);
 return {nodes,parents,children,depth};
}
function scheduleDetailRelationships(host,def,schedules,open){
 host.replaceChildren();
 if(!def?.id)return;
 const el=(tag,text,cls)=>{const x=document.createElement(tag);if(text!==undefined)x.textContent=text;if(cls)x.className=cls;return x};
 const graph=scheduleDependencyGraph(schedules);
 host.append(el('strong','Dependencies'));
 const lines=[
  ['Depends on',graph.parents.get(def.id)||[]],
  ['Triggered dependents',graph.children.get(def.id)||[]]
 ];
 for(const [label,ids] of lines){
  const row=el('div',undefined,'links');row.append(el('span',label+':'));
  if(!ids.length)row.append(el('span','None'));
  for(const id of ids){
   const button=el('button',graph.nodes.get(id)?.name||id);
   button.type='button';button.title=id;button.onclick=()=>open(id);row.append(button);
  }
  host.append(row);
 }
}
function scheduleRenderWorkspacePage(host,page,{schedules=[],runs=[],errors={},statistics=[],open,openRun,copyRun,execute,remove,toggle,stop}={}){
 const el=(tag,text,cls)=>{const x=document.createElement(tag);if(text!==undefined)x.textContent=text;if(cls)x.className=cls;return x};
 const title=el('div',undefined,'schedule-main-title');
 const heading=el('strong',page==='runs'?'All schedule runs':page==='graph'?'Schedule dependency graph':page==='workflows'?'Workflow executions':page==='analytics'?'Schedule analytics':page==='alerts'?'Schedule alerts':'All schedules');
 title.append(heading);
 const search=el('input');search.type='search';search.placeholder='Filter by name, trigger, ID or status';
 search.setAttribute('aria-label','Filter schedule view');
 title.append(search);host.replaceChildren(title);
 const holder=el('div',undefined,'schedule-main-scroll');host.append(holder);
 const filter=()=>search.value.trim().toLowerCase();
 if(page==='overview'){
  const kind=el('select');kind.setAttribute('aria-label','Filter trigger type');
  for(const [value,label] of [['','All triggers'],['cron','Cron'],['url','URL'],['git','Git'],['file','File'],['dependency','Following schedule'],['multi','Multiple conditions']]){
   const opt=el('option',label);opt.value=value;kind.append(opt);
  }
  title.append(kind);
  const table=el('table',undefined,'schedule-main-table'),thead=el('thead'),tbody=el('tbody');
  const tr=el('tr');
  for(const h of ['Schedule','Enabled','Trigger conditions','Execute target','Running','Last run','Actions'])tr.append(el('th',h));
  thead.append(tr);table.append(thead,tbody);holder.append(table);
  const draw=()=>{
   tbody.replaceChildren();
   const recent=new Map(),running=new Map();
   for(const run of runs){
    if(!recent.has(run.schedule_id))recent.set(run.schedule_id,run);
    if(run.status==='running')running.set(run.schedule_id,(running.get(run.schedule_id)||0)+1);
   }
   let count=0;
   for(const s of schedules){
    if(kind.value&&s.type!==kind.value)continue;
    if(filter()&&![s.name,s.id,s.type,s.git_path,s.url,s.command,scheduleSummary(s)].join(' ').toLowerCase().includes(filter()))continue;
    count++;
    const row=el('tr'),identity=el('td');
    const name=el('button',s.name||'Unnamed schedule');
    name.title='Schedule ID: '+s.id;name.onclick=()=>open(s.id);
    identity.append(name);row.append(identity);
    row.append(el('td',s.paused?'Paused':s.enabled?'Enabled':'Disabled'));
    const trigger=el('td');trigger.append(el('div',scheduleSummary(s,schedules)));
    const parentRefs=scheduleTriggerReferences(s,schedules);
    if(s.condition_group&&parentRefs.length){
     trigger.append(el('div','Depends on: '+parentRefs.join(' · '),'schedule-main-hint'));
    }else if(!parentRefs.length){
     trigger.append(el('div','No parent schedules','schedule-main-hint'));
    }
    if(errors[s.id])trigger.append(el('div','Check failed: '+errors[s.id],'mono'));
    row.append(trigger);
    row.append(el('td',s.target==='task'?'Task #'+s.task_id:(s.command||'Command').slice(0,130)));
    row.append(el('td',String(running.get(s.id)||0)));
    const last=recent.get(s.id);
    row.append(el('td',last?last.status+' · '+new Date(last.started_at).toLocaleString():'Never'));
    const actions=el('td');
    const action=(label,cb)=>{const b=el('button',label);b.onclick=()=>cb(s);actions.append(b)};
    action('Edit',item=>open(item.id));
    action('Run',item=>execute(item.id));
    action(s.enabled?'Disable':'Enable',item=>toggle(item));
    action('Delete',item=>remove(item));
    row.append(actions);tbody.append(row);
   }
   if(!count){const row=el('tr'),cell=el('td','No schedules match the filter');cell.colSpan=7;row.append(cell);tbody.append(row)}
  };
  search.oninput=draw;kind.onchange=draw;draw();
 }else if(page==='runs'){
  const status=el('select');status.setAttribute('aria-label','Filter run status');
  for(const [key,name] of [['','All statuses'],['running','Running'],['success','Success'],['failure','Failure'],['interrupted','Interrupted']]){
   const option=el('option',name);option.value=key;status.append(option);
  }title.append(status);
  const table=el('table',undefined,'schedule-main-table'),header=el('tr'),thead=el('thead'),tbody=el('tbody');
  for(const h of ['Schedule','Status / exit','Started','Finished / duration','Cause','Console / Actions'])header.append(el('th',h));
  thead.append(header);table.append(thead,tbody);holder.append(table);
  const names=new Map(schedules.map(s=>[s.id,s.name]));
  const draw=()=>{
   tbody.replaceChildren();let count=0;
   for(const r of runs){
    if(status.value&&r.status!==status.value)continue;
    if(filter()&&![r.name,r.schedule_id,r.id,r.status,r.reason,r.error].join(' ').toLowerCase().includes(filter()))continue;
    count++;
    const row=el('tr');
    const name=el('td'),link=el('button',names.get(r.schedule_id)||r.name);link.onclick=()=>open(r.schedule_id);
    link.title='Run ID: '+r.id;
    name.append(link);row.append(name);
    row.append(el('td',r.status+(r.status==='failure'?' · '+r.exit_code:'')));
    row.append(el('td',new Date(r.started_at).toLocaleString()));
    const finished=r.finished_at?new Date(r.finished_at):null;
    row.append(el('td',finished?finished.toLocaleString()+' · '+Math.max(0,Math.round((finished-new Date(r.started_at))/1000))+'s':'Running…'));
    row.append(el('td',r.reason||''));
    const actions=el('td');const consoleBtn=el('button','Open console');
    consoleBtn.onclick=()=>openRun(r.id,r.schedule_id);actions.append(consoleBtn);
    if(copyRun){
     const copyBtn=el('button','Copy console');
     copyBtn.title='Copy the console output from this run';
     copyBtn.onclick=()=>copyRun(r.id,copyBtn);actions.append(copyBtn);
    }
    if(r.status==='running'){const stopBtn=el('button','Stop');stopBtn.onclick=()=>stop(r.id);actions.append(stopBtn)}
    row.append(actions);tbody.append(row);
   }
   if(!count){const tr=el('tr'),td=el('td','No runs match the filter');td.colSpan=6;tr.append(td);tbody.append(tr)}
  };
  search.oninput=draw;status.onchange=draw;draw();
 }else if(page==='alerts'){
  const alerts=[];
  const limit=Date.now()-7*24*60*60*1000;
  for(const r of runs){
   const started=Date.parse(r.started_at);
   if(!Number.isFinite(started)||started<limit)continue;
   const schedule=schedules.find(s=>s.id===r.schedule_id);
   const age=Date.now()-started;
   if(r.status==='failure'||r.status==='interrupted'){
    alerts.push({level:'Failure',run:r,reason:r.error||'Task ended with '+r.status});
   }else if(r.status==='running'&&age>Math.max(1800000,1000*(schedule?.timeout_seconds||1800))){
    alerts.push({level:'Long running',run:r,reason:'Running longer than the configured timeout'});
   }else if(r.status==='queued'&&age>600000){
    alerts.push({level:'Queue delayed',run:r,reason:'Waiting in queue for more than ten minutes'});
   }
  }
  const table=el('table',undefined,'schedule-main-table'),head=el('tr'),body=el('tbody');
  for(const name of ['Severity','Schedule','Date','Details','Actions'])head.append(el('th',name));
  const thead=el('thead');thead.append(head);table.append(thead,body);holder.append(table);
  const draw=()=>{
   body.replaceChildren();
   for(const a of alerts){
    if(filter()&&![a.level,a.run.name,a.reason].join(' ').toLowerCase().includes(filter()))continue;
    const row=el('tr'),button=el('button',a.run.name);
    button.onclick=()=>open(a.run.schedule_id);
    const sched=el('td');sched.append(button);
    const actions=el('td'),log=el('button','Open console');
    log.onclick=()=>openRun(a.run.id,a.run.schedule_id);actions.append(log);
    row.append(el('td',a.level),sched,el('td',new Date(a.run.started_at).toLocaleString()),
     el('td',a.reason),actions);body.append(row);
   }
  };
  search.oninput=draw;draw();
 }else if(page==='workflows'){
  const groups=new Map();
  for(const run of runs){
   const id=run.workflow_id||run.id;
   if(!groups.has(id))groups.set(id,[]);
   groups.get(id).push(run);
  }
  const entries=[...groups].sort((a,b)=>new Date(b[1][0].started_at)-new Date(a[1][0].started_at));
  const draw=()=>{
   holder.replaceChildren();
   for(const [flowID,chain] of entries){
    if(filter()&&!chain.some(r=>[r.name,r.schedule_id,r.status,flowID].join(' ').toLowerCase().includes(filter())))continue;
    const details=el('details');details.open=entries.length===1;
    const summary=el('summary','Workflow '+flowID.slice(0,12)+' · '+chain.length+' runs');
    details.append(summary);
    const table=el('table',undefined,'schedule-main-table'),body=el('tbody');
    for(const run of [...chain].sort((a,b)=>new Date(a.started_at)-new Date(b.started_at))){
     const row=el('tr');
     const button=el('button',run.name);button.onclick=()=>openRun(run.id,run.schedule_id);
     const identity=el('td');identity.append(button);
     row.append(identity,el('td',run.status),el('td','Attempts: '+(run.attempts||1)),
      el('td',run.trigger_run_id?'Triggered by '+run.trigger_run_id.slice(0,12):'Root event'),
      el('td',new Date(run.started_at).toLocaleString()));
     body.append(row);
    }
    table.append(body);details.append(table);holder.append(details);
   }
  };
  search.oninput=draw;draw();
 }else if(page==='analytics'){
  const summary=new Map((statistics||[]).map(row=>[row.schedule_id,row]));
  const table=el('table',undefined,'schedule-main-table'),head=el('tr'),body=el('tbody');
  for(const name of ['Schedule','Runs','Success','Failure','Success rate','Mean duration (s)','Most recent error'])head.append(el('th',name));
  const thead=el('thead');thead.append(head);table.append(thead,body);holder.append(table);
  const draw=()=>{
   body.replaceChildren();
   for(const sched of schedules){
    if(filter()&&!sched.name.toLowerCase().includes(filter()))continue;
    const stat=summary.get(sched.id)||{};
    const completed=stat.completed||0;
    const row=el('tr'),cell=el('td'),link=el('button',sched.name);
    link.onclick=()=>open(sched.id);cell.append(link);row.append(cell,
     el('td',String(completed)),el('td',String(stat.success||0)),
     el('td',String(stat.failure||0)),
     el('td',completed?(100*(stat.success||0)/completed).toFixed(1)+'%':'—'),
     el('td',Number(stat.average_duration_seconds||0).toFixed(1)),
     el('td',stat.last_error||'—'));
    body.append(row);
   }
  };search.oninput=draw;draw();
 }else if(page==='graph'){
  host.removeChild(holder);
  const help=el('div','Arrows: parent → dependent. Click a schedule node to open its details; each schedule may have multiple parents and dependents.','schedule-main-hint');
  host.append(help);
  const graphScroll=el('div',undefined,'schedule-graph-scroll');host.append(graphScroll);
  const graph=scheduleDependencyGraph(schedules);
  const columns=new Map();
  for(const item of schedules){
   const d=graph.depth.get(item.id)||0;
   if(!columns.has(d))columns.set(d,[]);
   columns.get(d).push(item);
  }
  const positions=new Map(),rowHeight=100,columnWidth=265,nodeWidth=222,nodeHeight=66;
  const depthMax=Math.max(0,...graph.depth.values());
  for(const [column,items] of columns){
   items.sort((a,b)=>a.name.localeCompare(b.name));
   items.forEach((item,index)=>positions.set(item.id,{x:32+column*columnWidth,y:42+index*rowHeight}));
  }
  const maxRows=Math.max(1,...[...columns.values()].map(x=>x.length));
  const svg=document.createElementNS('http://www.w3.org/2000/svg','svg');
  svg.setAttribute('class','schedule-graph-svg');
  svg.setAttribute('width',String(Math.max(640,(depthMax+1)*columnWidth+90)));
  svg.setAttribute('height',String(Math.max(300,maxRows*rowHeight+100)));
  svg.setAttribute('role','img');
  svg.setAttribute('aria-label','Schedule dependency graph with branching relations');
  const node=(tag,attributes={})=>{
   const e=document.createElementNS('http://www.w3.org/2000/svg',tag);
   for(const [key,value] of Object.entries(attributes))e.setAttribute(key,String(value));
   return e;
  };
  const defs=node('defs'),marker=node('marker',{id:'schedule-dependency-arrow',markerWidth:8,markerHeight:8,refX:7,refY:4,orient:'auto'});
  const tip=node('path',{d:'M0,0 L8,4 L0,8 z',fill:'#8ca6ca'});marker.append(tip);defs.append(marker);svg.append(defs);
  for(const [target,parents] of graph.parents){
   for(const parent of parents){
    const from=positions.get(parent),to=positions.get(target);if(!from||!to)continue;
    const x1=from.x+nodeWidth,y1=from.y+nodeHeight/2,x2=to.x,y2=to.y+nodeHeight/2;
    const mid=(x1+x2)/2;
    const path=node('path',{d:'M'+x1+' '+y1+' C'+mid+' '+y1+' '+mid+' '+y2+' '+x2+' '+y2,
     class:'schedule-graph-line','marker-end':'url(#schedule-dependency-arrow)'});
    svg.append(path);
   }
  }
  for(const item of schedules){
   const pos=positions.get(item.id),group=node('g',{class:'schedule-graph-node',tabindex:0,role:'button','aria-label':'Open '+item.name});
   group.append(node('rect',{x:pos.x,y:pos.y,width:nodeWidth,height:nodeHeight,rx:8}));
   const text=node('text',{x:pos.x+10,y:pos.y+24});
   text.textContent=item.name.length>30?item.name.slice(0,29)+'…':item.name;group.append(text);
   const caption=node('text',{x:pos.x+10,y:pos.y+46});
   caption.textContent=(item.enabled?'● ':'○ ')+(item.condition_group?'Multi-condition':item.type);
   group.append(caption);group.addEventListener('click',()=>open(item.id));
   group.addEventListener('keydown',event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();open(item.id)}});
   svg.append(group);
  }
  graphScroll.append(svg);
  search.oninput=()=>{
   const needle=filter();
   for(const group of svg.querySelectorAll('.schedule-graph-node')){
    const id=group.getAttribute('aria-label').slice(5);
    group.style.opacity=!needle||id.toLowerCase().includes(needle)?'1':'.25';
   }
  };
 }
}
globalThis.TaskMenuScheduleViews={render:scheduleRenderWorkspacePage,relationships:scheduleDetailRelationships,
 parents:scheduleParents,graph:scheduleDependencyGraph,summary:scheduleSummary};
