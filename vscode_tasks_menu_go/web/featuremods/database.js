const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Database workspace');

const dbViews=new Map();
let profilesByID=new Map();

const style=document.createElement('style');
style.textContent=`
.db-tab{border-radius:6px 6px 0 0;border-bottom:0;margin-left:4px}
.db-tab.active{background:#343b48}
.db-tab .close{margin-left:8px}
.db-pane{position:absolute;inset:0;display:flex;flex-direction:column;background:#0d1015}
.db-pane.hidden{display:none}
.db-pane-head{height:42px;display:flex;align-items:center;gap:8px;padding:6px 10px;border-bottom:1px solid #30343b}
.db-pane-title{font-weight:600;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.db-pane-meta{font-size:11px;opacity:.55;white-space:nowrap}
.db-pane-body{flex:1;min-height:0;display:grid;grid-template-columns:minmax(210px,280px) minmax(0,1fr)}
.db-browser{min-width:0;min-height:0;overflow:hidden;border-right:1px solid #30343b;display:flex;flex-direction:column}
.db-browser-head{display:flex;gap:5px;padding:7px;border-bottom:1px solid #30343b}
.db-browser-head select{min-width:0;flex:1}
.db-browser-objects{flex:1;min-height:0;overflow:auto;padding:5px}
.db-object{display:block;width:100%;text-align:left;padding:6px 7px;margin:1px 0;background:transparent;border-color:transparent}
.db-object:hover,.db-object.selected{background:#222934}
.db-object-kind{font-size:9px;opacity:.5;text-transform:uppercase;margin-right:5px}
.db-object-name{font-size:12px}
.db-object-detail{max-height:35%;overflow:auto;border-top:1px solid #30343b;padding:7px;font-family:ui-monospace,monospace;font-size:11px;white-space:pre-wrap}
.db-query{min-width:0;display:flex;flex-direction:column}
.db-query-tools{display:flex;align-items:center;gap:6px;padding:7px;border-bottom:1px solid #30343b}
.db-query-tools .db-run{background:#244c70;border-color:#3f79a8}
.db-query-tools label{font-size:10px;opacity:.65}
.db-query-tools input{width:72px;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:6px}
.db-query-editor{min-height:130px;height:32%;resize:vertical;background:#090c10;color:inherit;border:0;border-bottom:1px solid #30343b;padding:10px;font-family:ui-monospace,monospace;font-size:13px;line-height:1.45;outline:none}
.db-result-wrap{flex:1;min-height:0;overflow:auto}
.db-result-status{position:sticky;top:0;z-index:2;padding:5px 8px;background:#11151b;border-bottom:1px solid #30343b;font-size:11px;opacity:.8}
.db-result-table{border-collapse:collapse;min-width:100%;font-family:ui-monospace,monospace;font-size:11px}
.db-result-table th,.db-result-table td{border-right:1px solid #272d36;border-bottom:1px solid #272d36;padding:5px 7px;text-align:left;vertical-align:top;white-space:pre-wrap;max-width:520px}
.db-result-table th{position:sticky;top:27px;background:#171c23;z-index:1}
.db-null{opacity:.45;font-style:italic}
html[data-taskmenu-theme="light"] .db-pane{background:#fff}
html[data-taskmenu-theme="light"] .db-query-editor{background:#f7f9fb}
html[data-taskmenu-theme="light"] .db-result-status{background:#f2f5f8}
html[data-taskmenu-theme="light"] .db-result-table th{background:#e9eef3}
`;
document.head.append(style);

function profileFor(id){
  return profilesByID.get(String(id||''))||null;
}

async function refreshProfiles(){
  const data=await app.jsonFetch('/api/db/profiles');
  profilesByID=new Map((Array.isArray(data?.profiles)?data.profiles:[]).map(profile=>[profile.id,profile]));
  return profilesByID;
}

async function sessionRequest(id,operation,payload){
  const body={operation};
  if(payload!==undefined)body.payload=payload;
  const response=await app.jsonFetch('/api/db/sessions/'+encodeURIComponent(id)+'/request',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify(body)
  });
  return response?.result;
}

function activateDatabaseView(id){
  const view=dbViews.get(id);
  if(!view)return;
  app.activateExternalView('database:'+id);
  for(const [otherID,other] of dbViews){
    const active=otherID===id;
    other.tab.classList.toggle('active',active);
    other.pane.classList.toggle('hidden',!active);
  }
}

function teardownDatabaseView(id){
  const view=dbViews.get(id);
  if(!view)return;
  view.tab.remove();
  view.pane.remove();
  dbViews.delete(id);
}

async function closeDatabaseView(id){
  const view=dbViews.get(id);
  if(!view)return;
  try{await app.jsonFetch('/api/db/sessions/'+encodeURIComponent(id),{method:'DELETE'});}
  catch(error){if(!String(error?.message||'').includes('404'))console.warn('Database session close failed',error);}
  teardownDatabaseView(id);
}

function resultCell(value){
  const td=document.createElement('td');
  if(value===null||value===undefined){
    td.textContent='NULL';td.className='db-null';
  }else if(typeof value==='object'){
    td.textContent=JSON.stringify(value);
  }else{
    td.textContent=String(value);
  }
  return td;
}

function renderResult(view,result,elapsed){
  view.result.replaceChildren();
  const status=document.createElement('div');status.className='db-result-status';
  const columns=Array.isArray(result?.columns)?result.columns:[];
  const rows=Array.isArray(result?.rows)?result.rows:[];
  const parts=[rows.length+' row'+(rows.length===1?'':'s')];
  if(result?.truncated)parts.push('truncated');
  if(Number.isFinite(result?.affected_rows))parts.push('affected '+result.affected_rows);
  if(Number.isFinite(elapsed))parts.push(elapsed+' ms');
  status.textContent=parts.join(' · ');
  view.result.append(status);
  if(!columns.length){
    const empty=document.createElement('div');empty.className='task-connection-empty';empty.textContent='Statement completed with no tabular result';view.result.append(empty);return;
  }
  const table=document.createElement('table');table.className='db-result-table';
  const thead=document.createElement('thead');const header=document.createElement('tr');
  for(const column of columns){const th=document.createElement('th');th.textContent=column?.name||'';if(column?.type)th.title=column.type;header.append(th);}
  thead.append(header);table.append(thead);
  const tbody=document.createElement('tbody');
  for(const row of rows){
    const tr=document.createElement('tr');
    for(const value of Array.isArray(row)?row:[])tr.append(resultCell(value));
    tbody.append(tr);
  }
  table.append(tbody);view.result.append(table);
}

async function executeQuery(view){
  const statement=view.editor.value.trim();
  if(!statement)throw new Error('Enter a database statement first');
  const maxRows=Math.max(1,Math.min(1000,Number(view.maxRows.value)||100));
  view.run.disabled=true;view.run.textContent='Running…';
  const started=performance.now();
  try{
    const result=await sessionRequest(view.meta.id,'execute',{statement,max_rows:maxRows});
    renderResult(view,result,Math.round(performance.now()-started));
  }finally{
    view.run.disabled=false;view.run.textContent='Run';
  }
}

function renderObjectDetail(view,object,detail){
  view.detail.textContent=JSON.stringify(detail,null,2);
  for(const button of view.objects.querySelectorAll('.db-object'))button.classList.toggle('selected',button.dataset.name===object.name);
}

async function describeObject(view,object){
  const payload={name:object.name,kind:object.kind};
  if(view.catalog.value)payload.catalog=view.catalog.value;
  const detail=await sessionRequest(view.meta.id,'describe_object',payload);
  renderObjectDetail(view,object,detail);
}

function renderObjects(view,result){
  view.objects.replaceChildren();
  const objects=Array.isArray(result)?result:(Array.isArray(result?.objects)?result.objects:[]);
  view.objectData=objects;
  if(!objects.length){
    const empty=document.createElement('div');empty.className='task-connection-empty';
    empty.textContent=view.meta.adapter_kind==='redis'?'No keys':(view.meta.adapter_kind==='mongo'?'No collections':'No tables or views');
    view.objects.append(empty);return;
  }
  for(const object of objects){
    const button=document.createElement('button');button.type='button';button.className='db-object';button.dataset.name=object?.name||'';button.dataset.kind=object?.kind||'';button.dataset.catalog=object?.catalog||view.catalog.value||'';
    const kind=document.createElement('span');kind.className='db-object-kind';kind.textContent=object?.kind||'object';
    const name=document.createElement('span');name.className='db-object-name';name.textContent=object?.name||'';
    button.append(kind,name);button.onclick=()=>describeObject(view,object).catch(app.showError);view.objects.append(button);
    globalThis.TaskMenuDatabaseWorkbench?.bindObject?.(view,object,button);
  }
}

async function loadObjects(view){
  if(!view.catalog.value){view.objects.replaceChildren();return;}
  view.refresh.disabled=true;
  try{
    const result=await sessionRequest(view.meta.id,'list_objects',{catalog:view.catalog.value});
    renderObjects(view,result);
    view.detail.textContent='';
  }finally{view.refresh.disabled=false;}
}

async function loadCatalogs(view){
  const result=await sessionRequest(view.meta.id,'list_catalogs');
  const catalogs=Array.isArray(result)?result:(Array.isArray(result?.catalogs)?result.catalogs:[]);
  const previous=view.catalog.value;
  view.catalog.replaceChildren();
  for(const catalog of catalogs){
    const name=String(catalog?.name||catalog?.catalog||'').trim();if(!name)continue;
    const option=document.createElement('option');option.value=name;option.textContent=name;view.catalog.append(option);
  }
  const profile=profileFor(view.meta.profile_id);
  const preferred=profile?.database||previous;
  if(preferred&&[...view.catalog.options].some(option=>option.value===preferred))view.catalog.value=preferred;
  if(view.catalog.options.length)await loadObjects(view);
  else renderObjects(view,[]);
}

function attachDatabaseView(meta,activate){
  if(dbViews.has(meta.id)){
    if(activate)activateDatabaseView(meta.id);
    return dbViews.get(meta.id);
  }
  const tabs=document.querySelector('#tabs');const panes=document.querySelector('#panes');
  if(!tabs||!panes)throw new Error('Workspace tabs are unavailable');
  const profile=profileFor(meta.profile_id);

  const tab=document.createElement('button');tab.type='button';tab.className='db-tab';tab.dataset.id=meta.id;
  const label=document.createElement('span');label.textContent='DB · '+(profile?.name||meta.adapter_kind||'Database');
  const close=document.createElement('span');close.className='close';close.textContent='×';close.onclick=event=>{event.stopPropagation();closeDatabaseView(meta.id).catch(app.showError);};
  tab.append(label,close);tab.onclick=()=>activateDatabaseView(meta.id);tabs.append(tab);

  const pane=document.createElement('div');pane.className='db-pane hidden';pane.dataset.id=meta.id;
  const head=document.createElement('div');head.className='db-pane-head';
  const title=document.createElement('div');title.className='db-pane-title';title.textContent=profile?.name||'Database';
  const metaText=document.createElement('div');metaText.className='db-pane-meta';metaText.textContent=(meta.adapter_kind||'database')+' · '+(profile?.read_only?'read-only':'read/write');
  const reload=document.createElement('button');reload.type='button';reload.textContent='Refresh schema';
  const disconnect=document.createElement('button');disconnect.type='button';disconnect.textContent='Disconnect';disconnect.onclick=()=>closeDatabaseView(meta.id).catch(app.showError);
  head.append(title,metaText,reload,disconnect);

  const body=document.createElement('div');body.className='db-pane-body';
  const browser=document.createElement('div');browser.className='db-browser';
  const browserHead=document.createElement('div');browserHead.className='db-browser-head';
  const catalog=document.createElement('select');catalog.title='Database / catalog';
  const refresh=document.createElement('button');refresh.type='button';refresh.textContent='↻';refresh.title='Refresh objects';
  browserHead.append(catalog,refresh);
  const objects=document.createElement('div');objects.className='db-browser-objects';
  const detail=document.createElement('div');detail.className='db-object-detail';
  browser.append(browserHead,objects,detail);

  const query=document.createElement('div');query.className='db-query';
  const tools=document.createElement('div');tools.className='db-query-tools';
  const run=document.createElement('button');run.type='button';run.className='db-run';run.textContent='Run';
  const rowsLabel=document.createElement('label');rowsLabel.textContent='Max rows';
  const maxRows=document.createElement('input');maxRows.type='number';maxRows.min='1';maxRows.max='1000';maxRows.value='100';
  tools.append(run,rowsLabel,maxRows);
  const editor=document.createElement('textarea');editor.className='db-query-editor';editor.spellcheck=false;
  editor.value=meta.adapter_kind==='redis'
    ?'PING'
    :(meta.adapter_kind==='mongo'
      ?'{\n  "op": "find",\n  "collection": "users",\n  "filter": {},\n  "limit": 100\n}'
      :'SELECT 1');
  const result=document.createElement('div');result.className='db-result-wrap';
  query.append(tools,editor,result);
  body.append(browser,query);pane.append(head,body);panes.append(pane);

  const view={meta,profile,tab,pane,catalog,refresh,objects,detail,editor,run,maxRows,result,objectData:[]};
  dbViews.set(meta.id,view);
  globalThis.TaskMenuDatabaseWorkbench?.enhanceView?.(view);
  reload.onclick=()=>loadCatalogs(view).catch(app.showError);
  refresh.onclick=()=>loadObjects(view).catch(app.showError);
  catalog.onchange=()=>loadObjects(view).catch(app.showError);
  run.onclick=()=>executeQuery(view).catch(app.showError);
  editor.addEventListener('keydown',event=>{
    if((event.ctrlKey||event.metaKey)&&event.key==='Enter'){event.preventDefault();executeQuery(view).catch(app.showError);}
  });
  loadCatalogs(view).catch(error=>{view.detail.textContent=String(error?.message||error);});
  if(activate)activateDatabaseView(meta.id);
  return view;
}

async function openProfile(profileOrID){
  const id=typeof profileOrID==='string'?profileOrID:profileOrID?.id;
  if(!id)throw new Error('Database profile is required');
  const meta=await app.jsonFetch('/api/db/sessions',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({profile_id:id})
  });
  await refreshProfiles();
  return attachDatabaseView(meta,true);
}

async function testProfile(profileOrID){
  const id=typeof profileOrID==='string'?profileOrID:profileOrID?.id;
  if(!id)throw new Error('Database profile is required');
  const meta=await app.jsonFetch('/api/db/sessions',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({profile_id:id})
  });
  try{
    await sessionRequest(meta.id,'ping');
    return {ok:true};
  }finally{
    try{await app.jsonFetch('/api/db/sessions/'+encodeURIComponent(meta.id),{method:'DELETE'});}catch{}
  }
}

async function testDraft(profileID,profile){
  if(!profile||typeof profile!=='object')throw new Error('Database profile draft is required');
  const body={profile};
  if(profileID)body.profile_id=profileID;
  return app.jsonFetch('/api/db/test',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify(body)
  });
}

async function restoreDatabaseSessions(){
  await refreshProfiles();
  const data=await app.jsonFetch('/api/db/sessions');
  for(const meta of Array.isArray(data?.sessions)?data.sessions:[])attachDatabaseView(meta,false);
}

window.addEventListener('taskmenu:view-activated',event=>{
  const detail=event.detail||{};
  const external=detail.kind==='external'&&String(detail.id||'').startsWith('database:');
  const activeID=external?String(detail.id).slice('database:'.length):'';
  for(const [id,view] of dbViews){
    const active=id===activeID;
    view.tab.classList.toggle('active',active);
    view.pane.classList.toggle('hidden',!active);
  }
});

globalThis.TaskMenuDatabase={
  openProfile,
  testProfile,
  testDraft,
  refreshProfiles,
  request:sessionRequest,
  getProfile:profileFor,
  restoreSessions:restoreDatabaseSessions,
  get views(){return dbViews;}
};

if(app.taskData?.workspace)restoreDatabaseSessions().catch(error=>console.warn('Database session restore failed',error));
else window.addEventListener('taskmenu:tasks',()=>restoreDatabaseSessions().catch(error=>console.warn('Database session restore failed',error)),{once:true});
