const app=globalThis.TaskMenuApp;
const database=globalThis.TaskMenuDatabase;
if(!app||!database)throw new Error('TaskMenuDatabase unavailable for Database Workbench');

const adaptersByID=new Map();
let adapterLoadPromise=null;
let contextMenu=null;

const style=document.createElement('style');
style.textContent=`
.db-main{min-width:0;display:flex;flex-direction:column}
.db-workbench-tabs{height:34px;display:flex;align-items:end;gap:2px;padding:0 7px;border-bottom:1px solid #30343b;background:#11151b}
.db-workbench-tab{border-radius:5px 5px 0 0;border-bottom:0;padding:6px 10px;font-size:11px;opacity:.7}
.db-workbench-tab.active{background:#202630;opacity:1}
.db-workbench-panel{flex:1;min-height:0}
.db-workbench-panel.hidden{display:none}
.db-browser-filter{min-width:0;flex:1;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:5px 7px;font-size:11px}
.db-data{display:flex;flex-direction:column;min-height:0}
.db-data-tools{display:flex;align-items:center;gap:5px;flex-wrap:wrap;padding:6px 7px;border-bottom:1px solid #30343b}
.db-data-tools button,.db-data-tools select{font-size:11px}
.db-data-tools select{background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:5px}
.db-data-spacer{flex:1}
.db-data-status{font-size:10px;opacity:.65;white-space:nowrap}
.db-data-grid-wrap{flex:1;min-height:0;overflow:auto}
.db-data-grid{border-collapse:collapse;min-width:100%;font-family:ui-monospace,monospace;font-size:11px}
.db-data-grid th,.db-data-grid td{border-right:1px solid #272d36;border-bottom:1px solid #272d36;padding:5px 7px;text-align:left;vertical-align:top;white-space:pre-wrap;max-width:520px}
.db-data-grid th{position:sticky;top:0;background:#171c23;z-index:2;cursor:pointer;user-select:none}
.db-data-grid th.db-row-number,.db-data-grid td.db-row-number{position:sticky;left:0;z-index:3;width:42px;min-width:42px;text-align:right;opacity:.55;background:#11151b}
.db-data-grid th.db-row-number{z-index:4}
.db-data-grid td.db-editable{cursor:text;outline:none}
.db-data-grid td.db-editable:focus{box-shadow:inset 0 0 0 1px #3f79a8;background:#121a23}
.db-data-grid td.db-dirty{background:#332d18}
.db-data-grid tr.db-deleted td{text-decoration:line-through;opacity:.5}
.db-data-grid tr.db-new-row td{background:#14261d}
.db-data-tools .db-apply{background:#244c70;border-color:#3f79a8}
.db-data-tools .db-danger{background:#54252a;border-color:#7b3941}
.db-filter-summary{font-size:10px;opacity:.7;max-width:280px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.db-filter-list{display:flex;flex-direction:column;gap:6px;max-height:50vh;overflow:auto;margin:8px 0}
.db-filter-row{display:grid;grid-template-columns:minmax(120px,1fr) minmax(120px,1fr) minmax(160px,1.5fr) auto;gap:6px;align-items:center}
.db-filter-row select,.db-filter-row input{min-width:0;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:6px}
.db-data-null{opacity:.45;font-style:italic}
.db-data-empty{padding:18px;font-size:12px;opacity:.6}
.db-structure{overflow:auto;padding:10px}
.db-structure-title{font-size:13px;font-weight:700;margin-bottom:8px}
.db-structure-meta{font-size:11px;opacity:.65;margin-bottom:10px}
.db-structure-table{border-collapse:collapse;width:100%;font-size:11px}
.db-structure-table th,.db-structure-table td{border:1px solid #30343b;padding:6px;text-align:left;vertical-align:top}
.db-structure-table th{background:#171c23}
.db-structure-json{font-family:ui-monospace,monospace;white-space:pre-wrap;font-size:11px}
.db-context-menu{position:fixed;z-index:15000;min-width:220px;max-width:min(360px,90vw);padding:4px;background:#171b22;border:1px solid #48515f;border-radius:7px;box-shadow:0 14px 38px rgba(0,0,0,.45)}
.db-context-item{display:block;width:100%;text-align:left;border:0;background:transparent;color:inherit;padding:7px 9px;border-radius:4px;font-size:11px}
.db-context-item:hover:not(:disabled){background:#2b3440}
.db-context-item:disabled{opacity:.35}
.db-context-item.danger{color:#ff9a9a}
.db-context-separator{height:1px;background:#30343b;margin:4px 2px}
html[data-taskmenu-theme="light"] .db-workbench-tabs{background:#f2f5f8;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .db-workbench-tab.active{background:#fff}
html[data-taskmenu-theme="light"] .db-browser-filter,html[data-taskmenu-theme="light"] .db-data-tools select{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .db-data-grid th,html[data-taskmenu-theme="light"] .db-structure-table th{background:#e9eef3}
html[data-taskmenu-theme="light"] .db-data-grid th.db-row-number,html[data-taskmenu-theme="light"] .db-data-grid td.db-row-number{background:#f2f5f8}
html[data-taskmenu-theme="light"] .db-context-menu{background:#fff;border-color:#b9c0c8}
`;
document.head.append(style);

function ensureAdapters(){
  if(adapterLoadPromise)return adapterLoadPromise;
  adapterLoadPromise=app.jsonFetch('/api/db/adapters').then(data=>{
    adaptersByID.clear();
    for(const adapter of Array.isArray(data?.adapters)?data.adapters:[])adaptersByID.set(String(adapter.id||''),adapter);
    return adaptersByID;
  }).catch(error=>{
    adapterLoadPromise=null;
    console.warn('Database Workbench adapter metadata unavailable',error);
    return adaptersByID;
  });
  return adapterLoadPromise;
}

function profileForView(view){
  return database.getProfile?.(view.meta.profile_id)||view.profile||null;
}

function adapterForView(view){
  const profile=profileForView(view);
  return adaptersByID.get(String(profile?.adapter_id||view.meta.adapter_id||''))||null;
}

function supports(view,name){
  return Boolean(adapterForView(view)?.capabilities?.[name]);
}

function closeContextMenu(){
  if(contextMenu?.isConnected)contextMenu.remove();
  contextMenu=null;
}

function showContextMenu(items,x,y){
  closeContextMenu();
  const menu=document.createElement('div');menu.className='db-context-menu';
  for(const item of items){
    if(item.separator){
      const sep=document.createElement('div');sep.className='db-context-separator';menu.append(sep);continue;
    }
    const button=document.createElement('button');button.type='button';button.className='db-context-item'+(item.danger?' danger':'');
    button.textContent=item.label;button.disabled=Boolean(item.disabled);
    button.onclick=event=>{
      event.preventDefault();event.stopPropagation();closeContextMenu();
      Promise.resolve(item.action?.()).catch(app.showError);
    };
    menu.append(button);
  }
  document.body.append(menu);contextMenu=menu;
  const rect=menu.getBoundingClientRect();
  const left=Math.max(4,Math.min(x,window.innerWidth-rect.width-4));
  const top=Math.max(4,Math.min(y,window.innerHeight-rect.height-4));
  menu.style.left=left+'px';menu.style.top=top+'px';
}
document.addEventListener('pointerdown',event=>{if(contextMenu&&!contextMenu.contains(event.target))closeContextMenu();},true);
window.addEventListener('blur',closeContextMenu);
document.addEventListener('keydown',event=>{if(event.key==='Escape')closeContextMenu();},true);

async function copyText(value){
  const text=String(value??'');
  if(navigator.clipboard?.writeText){
    await navigator.clipboard.writeText(text);
    return;
  }
  const area=document.createElement('textarea');area.value=text;area.style.position='fixed';area.style.opacity='0';
  document.body.append(area);area.select();
  try{document.execCommand('copy');}finally{area.remove();}
}

function sqlIdentifier(kind,value){
  const text=String(value||'');
  if(kind==='mysql')return '`'+text.replaceAll('`','``')+'`';
  return '"'+text.replaceAll('"','""')+'"';
}

function qualifiedName(view,object){
  const kind=view.meta.adapter_kind||adapterForView(view)?.kind||'';
  const name=sqlIdentifier(kind,object.name);
  const catalog=String(object.catalog||view.catalog?.value||'').trim();
  if(kind==='sqlite'||!catalog)return name;
  if(kind==='mysql')return sqlIdentifier(kind,catalog)+'.'+name;
  return catalog+'.'+object.name;
}

function queryTemplate(view,object,action,detail=null){
  const adapterKind=view.meta.adapter_kind||adapterForView(view)?.kind||'';
  if(adapterKind!=='mysql'&&adapterKind!=='sqlite')return '';
  const target=qualifiedName(view,object);
  const columns=Array.isArray(detail?.columns)?detail.columns.map(column=>String(column?.name||'').trim()).filter(Boolean):[];
  const quotedColumns=columns.map(name=>sqlIdentifier(adapterKind,name));
  switch(action){
  case 'select':
    return 'SELECT * FROM '+target+' LIMIT 100;';
  case 'insert':
    return quotedColumns.length
      ?'INSERT INTO '+target+' (\n  '+quotedColumns.join(',\n  ')+'\n) VALUES (\n  '+quotedColumns.map(()=>'?').join(',\n  ')+'\n);'
      :'INSERT INTO '+target+' (...) VALUES (...);';
  case 'update':
    return quotedColumns.length
      ?'UPDATE '+target+'\nSET '+quotedColumns.map(name=>name+' = ?').join(',\n    ')+'\nWHERE ...;'
      :'UPDATE '+target+' SET ... WHERE ...;';
  case 'delete':
    return 'DELETE FROM '+target+' WHERE ...;';
  default:
    return '';
  }
}

function activatePanel(view,name){
  const wb=view.workbench;
  if(!wb)return;
  wb.active=name;
  for(const [key,button] of Object.entries(wb.tabs))button.classList.toggle('active',key===name);
  for(const [key,panel] of Object.entries(wb.panels))panel.classList.toggle('hidden',key!==name);
}

function setQuery(view,text){
  view.editor.value=text;
  activatePanel(view,'query');
  view.editor.focus();
}

function renderStructure(view,object,detail){
  const panel=view.workbench?.panels?.structure;if(!panel)return;
  panel.replaceChildren();
  const title=document.createElement('div');title.className='db-structure-title';title.textContent=(object.kind||'object')+' · '+object.name;
  const meta=document.createElement('div');meta.className='db-structure-meta';meta.textContent=object.catalog||view.catalog?.value||'';
  panel.append(title,meta);
  const columns=Array.isArray(detail?.columns)?detail.columns:null;
  if(columns){
    const table=document.createElement('table');table.className='db-structure-table';
    const thead=document.createElement('thead');const hr=document.createElement('tr');
    for(const label of ['Column','Type','Nullable','Default','Extra']){const th=document.createElement('th');th.textContent=label;hr.append(th);}
    thead.append(hr);table.append(thead);
    const tbody=document.createElement('tbody');
    for(const column of columns){
      const tr=document.createElement('tr');
      const values=[column?.name,column?.type,column?.nullable?'YES':'NO',column?.default??'NULL',column?.extra||''];
      for(const value of values){const td=document.createElement('td');td.textContent=String(value??'');tr.append(td);}
      tbody.append(tr);
    }
    table.append(tbody);panel.append(table);
  }else{
    const pre=document.createElement('pre');pre.className='db-structure-json';pre.textContent=JSON.stringify(detail,null,2);panel.append(pre);
  }
}

async function inspectObject(view,object,{activate=false}={}){
  const payload={name:object.name,kind:object.kind};
  if(object.catalog||view.catalog.value)payload.catalog=object.catalog||view.catalog.value;
  const detail=await database.request(view.meta.id,'describe_object',payload);
  view.workbench.details.set(objectKey(object),detail);
  renderStructure(view,object,detail);
  if(activate)activatePanel(view,'structure');
  return detail;
}

function objectKey(object){
  return [object?.catalog||'',object?.kind||'',object?.name||''].join('\u0000');
}

function displayValue(value){
  if(value===null||value===undefined)return 'NULL';
  if(typeof value==='object')return JSON.stringify(value);
  return String(value);
}

function dataState(view){
  return view.workbench.data;
}

function pendingChangeCount(view){
  const state=dataState(view);
  return state.dirtyRows.size+state.deletedRows.size+state.newRows.filter(row=>Object.keys(row).length!==0).length;
}

function hasPendingChanges(view){
  return pendingChangeCount(view)>0||dataState(view).newRows.length>0;
}

function clearPendingChanges(view){
  const state=dataState(view);
  state.dirtyRows.clear();
  state.deletedRows.clear();
  state.newRows=[];
  updateEditControls(view);
}

function confirmDiscardChanges(view){
  if(!hasPendingChanges(view))return true;
  return confirm('Discard unsaved database grid changes?');
}

function updateEditControls(view){
  const wb=view.workbench;if(!wb?.controls)return;
  const result=wb.data.result||{};
  const editable=Boolean(result.editable)&&supports(view,'mutate_rows');
  const pending=pendingChangeCount(view);
  wb.controls.add.disabled=!editable;
  wb.controls.apply.disabled=!editable||pending===0;
  wb.controls.revert.disabled=!hasPendingChanges(view);
  const base=wb.controls.status.dataset.base||wb.controls.status.textContent||'';
  wb.controls.status.textContent=base+(pending?' · '+pending+' pending':'');
}

function parseEditedValue(text,original,columnType=''){
  const type=String(columnType||'').toLowerCase();
  if(type==='document'||type==='json'||type.includes('json'))return JSON.parse(text);
  if(typeof original==='number'){
    const value=Number(text);if(!Number.isFinite(value))throw new Error('Enter a valid number');return value;
  }
  if(typeof original==='boolean'){
    const value=String(text).trim().toLowerCase();
    if(value==='true'||value==='1')return true;
    if(value==='false'||value==='0')return false;
    throw new Error('Enter true/false');
  }
  if(original&&typeof original==='object'){
    return JSON.parse(text);
  }
  return String(text);
}

function sameValue(a,b){
  return JSON.stringify(a)===JSON.stringify(b);
}

function setDirtyCell(view,rowIndex,columnIndex,value){
  const state=dataState(view);const result=state.result||{};
  const row=result.rows?.[rowIndex];const column=result.columns?.[columnIndex];
  if(!row||!column)return;
  const original=row.values?.[columnIndex];
  let changes=state.dirtyRows.get(rowIndex);
  if(sameValue(value,original)){
    if(changes){changes.delete(column.name);if(changes.size===0)state.dirtyRows.delete(rowIndex);}
  }else{
    if(!changes){changes=new Map();state.dirtyRows.set(rowIndex,changes);}
    changes.set(column.name,value);
  }
  updateEditControls(view);
}

function currentCellValue(view,rowIndex,columnIndex){
  const state=dataState(view);const result=state.result||{};
  const column=result.columns?.[columnIndex];const row=result.rows?.[rowIndex];
  const changes=state.dirtyRows.get(rowIndex);
  return changes?.has(column?.name)?changes.get(column.name):row?.values?.[columnIndex];
}

function dataPayload(view){
  const state=dataState(view);const object=state.object;
  return {
    catalog:object?.catalog||view.catalog.value||'',
    kind:object?.kind||'table',
    name:object?.name||'',
    offset:state.offset,
    limit:state.limit,
    sort:state.sort,
    filters:state.filters
  };
}

function setDataBusy(view,busy){
  const wb=view.workbench;if(!wb)return;
  for(const button of [wb.controls.first,wb.controls.prev,wb.controls.next,wb.controls.refresh])if(button)button.disabled=busy;
  if(wb.controls.pageSize)wb.controls.pageSize.disabled=busy;
}

async function loadData(view,{resetOffset=false}={}){
  const state=dataState(view);
  if(!state.object)return;
  if(!supports(view,'browse_rows'))throw new Error('This database adapter does not support table data browsing');
  if(resetOffset)state.offset=0;
  setDataBusy(view,true);
  try{
    const result=await database.request(view.meta.id,'browse_rows',dataPayload(view));
    state.result=result||{};
    clearPendingChanges(view);
    renderDataGrid(view);
  }finally{setDataBusy(view,false);}
}

function renderDataGrid(view){
  const wb=view.workbench;const state=wb.data;const result=state.result||{};
  const columns=Array.isArray(result.columns)?result.columns:[];
  const rows=Array.isArray(result.rows)?result.rows:[];
  wb.grid.replaceChildren();
  wb.controls.first.disabled=state.offset<=0;
  wb.controls.prev.disabled=state.offset<=0;
  wb.controls.next.disabled=!result.has_more;
  const page=Math.floor(state.offset/state.limit)+1;
  const start=rows.length?state.offset+1:0;
  const end=state.offset+rows.length;
  const total=Number.isFinite(result.total_rows)?' / '+result.total_rows:'';
  const editText=result.editable?'editable':(result.editability_reason||'read-only');
  wb.controls.status.dataset.base='Page '+page+' · rows '+start+'–'+end+total+' · '+editText+(state.filters.length?' · '+state.filters.length+' filter'+(state.filters.length===1?'':'s'):'');
  wb.controls.status.textContent=wb.controls.status.dataset.base;
  updateEditControls(view);
  if(!columns.length){
    const empty=document.createElement('div');empty.className='db-data-empty';empty.textContent='No tabular data';wb.grid.append(empty);return;
  }
  const table=document.createElement('table');table.className='db-data-grid';
  const thead=document.createElement('thead');const hr=document.createElement('tr');
  const nr=document.createElement('th');nr.className='db-row-number';nr.textContent='#';hr.append(nr);
  for(const column of columns){
    const th=document.createElement('th');th.textContent=column.name||'';th.title=column.type||'';
    const active=state.sort.find(item=>item.column===column.name);
    if(active)th.textContent+=(active.direction==='desc'?' ▼':' ▲');
    th.onclick=()=>{
      if(!confirmDiscardChanges(view))return;
      const current=state.sort.find(item=>item.column===column.name);
      state.sort=current?(current.direction==='asc'?[{column:column.name,direction:'desc'}]:[]):[{column:column.name,direction:'asc'}];
      state.offset=0;loadData(view).catch(app.showError);
    };
    hr.append(th);
  }
  thead.append(hr);table.append(thead);
  const tbody=document.createElement('tbody');
  const gridEditable=Boolean(result.editable)&&supports(view,'mutate_rows');
  rows.forEach((row,rowIndex)=>{
    const tr=document.createElement('tr');tr.dataset.rowIndex=String(rowIndex);
    if(state.deletedRows.has(rowIndex))tr.classList.add('db-deleted');
    const rowNo=document.createElement('td');rowNo.className='db-row-number';rowNo.textContent=String(state.offset+rowIndex+1);tr.append(rowNo);
    columns.forEach((column,columnIndex)=>{
      const value=currentCellValue(view,rowIndex,columnIndex);
      const td=document.createElement('td');td.dataset.rowIndex=String(rowIndex);td.dataset.columnIndex=String(columnIndex);
      td.textContent=displayValue(value);if(value===null||value===undefined)td.classList.add('db-data-null');
      if(state.dirtyRows.get(rowIndex)?.has(column.name))td.classList.add('db-dirty');
      td.title=column.type||'';
      const cellEditable=gridEditable&&column.editable!==false&&!state.deletedRows.has(rowIndex);
      if(cellEditable){
        td.contentEditable='true';td.spellcheck=false;td.classList.add('db-editable');
        td.addEventListener('focus',()=>{if(td.classList.contains('db-data-null')){td.textContent='';td.classList.remove('db-data-null');}});
        td.addEventListener('blur',()=>{
          try{
            const original=row.values?.[columnIndex];
            const next=parseEditedValue(td.textContent,original,column.type);
            setDirtyCell(view,rowIndex,columnIndex,next);renderDataGrid(view);
          }catch(error){app.showError(error);renderDataGrid(view);}
        });
      }
      td.oncontextmenu=event=>{event.preventDefault();showCellMenu(view,rowIndex,columnIndex,event.clientX,event.clientY);};
      tr.append(td);
    });
    tr.oncontextmenu=event=>{
      if(event.target.closest('td:not(.db-row-number)'))return;
      event.preventDefault();showRowMenu(view,rowIndex,event.clientX,event.clientY);
    };
    tbody.append(tr);
  });
  state.newRows.forEach((values,newIndex)=>{
    const tr=document.createElement('tr');tr.className='db-new-row';
    const rowNo=document.createElement('td');rowNo.className='db-row-number';rowNo.textContent='+';tr.append(rowNo);
    columns.forEach((column,columnIndex)=>{
      const td=document.createElement('td');const has=Object.prototype.hasOwnProperty.call(values,column.name);const value=has?values[column.name]:null;
      td.textContent=has?displayValue(value):'';td.classList.add('db-editable');td.contentEditable='true';td.spellcheck=false;td.title=column.type||'';
      if(has&&value===null)td.classList.add('db-data-null');
      td.addEventListener('focus',()=>{if(td.classList.contains('db-data-null')){td.textContent='';td.classList.remove('db-data-null');}});
      td.addEventListener('blur',()=>{
        try{
          const text=td.textContent;
          if(text===''){delete values[column.name];}
          else values[column.name]=parseEditedValue(text,'',column.type);
          updateEditControls(view);renderDataGrid(view);
        }catch(error){app.showError(error);renderDataGrid(view);}
      });
      td.oncontextmenu=event=>{event.preventDefault();showNewCellMenu(view,newIndex,columnIndex,event.clientX,event.clientY);};
      tr.append(td);
    });
    tr.oncontextmenu=event=>{
      if(event.target.closest('td:not(.db-row-number)'))return;
      event.preventDefault();
      showContextMenu([{label:'Remove New Row',danger:true,action:()=>{state.newRows.splice(newIndex,1);renderDataGrid(view);updateEditControls(view);}}],event.clientX,event.clientY);
    };
    tbody.append(tr);
  });
  table.append(tbody);wb.grid.append(table);
}

function showCellMenu(view,rowIndex,columnIndex,x,y){
  const state=dataState(view);const result=state.result||{};const row=result.rows?.[rowIndex];const column=result.columns?.[columnIndex];
  const value=currentCellValue(view,rowIndex,columnIndex);
  const editable=Boolean(result.editable)&&supports(view,'mutate_rows')&&column?.editable!==false&&!state.deletedRows.has(rowIndex);
  const dirty=state.dirtyRows.get(rowIndex)?.has(column?.name);
  showContextMenu([
    {label:'Copy Value',action:()=>copyText(displayValue(value))},
    {label:'Copy Column Name',action:()=>copyText(column?.name||'')},
    {separator:true},
    {label:'Open Value in Editor',action:()=>openValueViewer(column?.name||'',value,{editable,onSave:next=>{setDirtyCell(view,rowIndex,columnIndex,next);renderDataGrid(view);}})},
    {label:'Set NULL',disabled:!editable||!column?.nullable,action:()=>{setDirtyCell(view,rowIndex,columnIndex,null);renderDataGrid(view);}},
    {label:'Revert Cell',disabled:!dirty,action:()=>{
      const changes=state.dirtyRows.get(rowIndex);changes?.delete(column.name);if(changes?.size===0)state.dirtyRows.delete(rowIndex);renderDataGrid(view);updateEditControls(view);
    }}
  ],x,y);
}

function showNewCellMenu(view,newIndex,columnIndex,x,y){
  const state=dataState(view);const column=state.result?.columns?.[columnIndex];const values=state.newRows[newIndex]||{};
  const has=Object.prototype.hasOwnProperty.call(values,column?.name);const value=has?values[column.name]:null;
  showContextMenu([
    {label:'Copy Value',action:()=>copyText(displayValue(value))},
    {label:'Open Value in Editor',action:()=>openValueViewer(column?.name||'',value,{editable:true,onSave:next=>{values[column.name]=next;renderDataGrid(view);updateEditControls(view);}})},
    {label:'Set NULL',disabled:!column?.nullable,action:()=>{values[column.name]=null;renderDataGrid(view);updateEditControls(view);}},
    {label:'Clear Field',disabled:!has,action:()=>{delete values[column.name];renderDataGrid(view);updateEditControls(view);}}
  ],x,y);
}

function showRowMenu(view,rowIndex,x,y){
  const state=dataState(view);const row=state.result?.rows?.[rowIndex];
  const editable=Boolean(state.result?.editable)&&supports(view,'mutate_rows');
  const deleted=state.deletedRows.has(rowIndex);
  showContextMenu([
    {label:'Copy Row as JSON',action:()=>{
      const columns=state.result?.columns||[];const out={};
      columns.forEach((column,index)=>{out[column.name]=currentCellValue(view,rowIndex,index);});
      return copyText(JSON.stringify(out,null,2));
    }},
    {separator:true},
    {label:deleted?'Restore Row':'Delete Row',danger:!deleted,disabled:!editable,action:()=>{
      if(deleted)state.deletedRows.delete(rowIndex);else state.deletedRows.add(rowIndex);
      renderDataGrid(view);updateEditControls(view);
    }}
  ],x,y);
}

function openValueViewer(titleText,value,{editable=false,onSave=null}={}){
  const dialog=document.createElement('div');dialog.className='task-connection-dialog';
  const card=document.createElement('div');card.className='task-connection-dialog-card';
  const title=document.createElement('h3');title.textContent=titleText||'Value';
  const area=document.createElement('textarea');area.readOnly=!editable;area.style.width='100%';area.style.minHeight='320px';area.value=value===null||value===undefined?'':displayValue(value);
  const actions=document.createElement('div');actions.className='task-connection-dialog-actions';
  const copy=document.createElement('button');copy.type='button';copy.textContent='Copy';copy.onclick=()=>copyText(area.value).catch(app.showError);
  const close=document.createElement('button');close.type='button';close.textContent='Close';close.onclick=()=>dialog.remove();
  actions.append(copy);
  if(editable){
    const setNull=document.createElement('button');setNull.type='button';setNull.textContent='Set NULL';setNull.onclick=()=>{onSave?.(null);dialog.remove();};
    const save=document.createElement('button');save.type='button';save.className='task-connection-primary';save.textContent='Use Value';save.onclick=()=>{
      try{onSave?.(parseEditedValue(area.value,value,titleText==='document'?'document':''));dialog.remove();}catch(error){app.showError(error);}
    };
    actions.append(setNull,save);
  }
  actions.append(close);card.append(title,area,actions);dialog.append(card);document.body.append(dialog);
  dialog.addEventListener('pointerdown',event=>{if(event.target===dialog)dialog.remove();});
}

async function openTableData(view,object){
  if(!confirmDiscardChanges(view))return;
  const state=dataState(view);
  state.object=object;state.offset=0;state.sort=[];state.filters=[];state.result=null;clearPendingChanges(view);
  wbSetDataTitle(view,object);
  activatePanel(view,'data');
  await loadData(view);
}

async function applyGridChanges(view){
  const state=dataState(view);const result=state.result||{};const object=state.object;
  if(!object||!result.editable||!supports(view,'mutate_rows'))throw new Error('This table is not editable');
  const mutations=[];
  for(const values of state.newRows){
    if(Object.keys(values).length)mutations.push({action:'insert',values:{...values}});
  }
  for(const [rowIndex,changes] of state.dirtyRows){
    if(state.deletedRows.has(rowIndex)||changes.size===0)continue;
    const row=result.rows?.[rowIndex];if(!row?.identity)continue;
    mutations.push({action:'update',identity:row.identity,values:Object.fromEntries(changes)});
  }
  for(const rowIndex of state.deletedRows){
    const row=result.rows?.[rowIndex];if(!row?.identity)continue;
    mutations.push({action:'delete',identity:row.identity});
  }
  if(!mutations.length){clearPendingChanges(view);renderDataGrid(view);return;}
  const response=await database.request(view.meta.id,'mutate_rows',{
    catalog:object.catalog||view.catalog.value||'',kind:object.kind,name:object.name,mutations
  });
  const errors=(Array.isArray(response?.results)?response.results:[]).filter(item=>item?.error);
  clearPendingChanges(view);
  await loadData(view);
  if(errors.length){
    throw new Error(errors.map(item=>(item.action||'mutation')+': '+(item.error?.message||item.error?.code||'failed')).join('\n'));
  }
}

function addGridRow(view){
  const state=dataState(view);const result=state.result||{};
  if(!result.editable||!supports(view,'mutate_rows'))return;
  state.newRows.push({});
  renderDataGrid(view);updateEditControls(view);
  requestAnimationFrame(()=>{
    const cells=view.workbench.grid.querySelectorAll('tr.db-new-row:last-child td.db-editable');
    cells[0]?.focus();
  });
}

function wbSetDataTitle(view,object){
  const title=view.workbench?.controls?.objectTitle;
  if(title)title.textContent=object?((object.kind||'table')+' · '+object.name):'No table selected';
}

function queryTemplateCapable(view){
  const kind=view.meta.adapter_kind||adapterForView(view)?.kind||'';
  return kind==='mysql'||kind==='sqlite'||kind==='mongo';
}

async function generatedQuery(view,object,action){
  const kind=view.meta.adapter_kind||adapterForView(view)?.kind||'';
  if(kind==='mongo'){
    if(action!=='select')throw new Error('MongoDB write templates are intentionally not generated; edit documents in Data or use the safe Query find DSL');
    const payload={op:'find',database:object.catalog||view.catalog.value||'',collection:object.name,filter:{},limit:100};
    setQuery(view,JSON.stringify(payload,null,2));
    return;
  }
  let detail=view.workbench.details.get(objectKey(object));
  if(!detail&&(action==='insert'||action==='update'))detail=await inspectObject(view,object);
  const text=queryTemplate(view,object,action,detail);
  if(!text)throw new Error('Query template generation is not available for this adapter');
  setQuery(view,text);
}

async function countRows(view,object){
  const result=await database.request(view.meta.id,'object_action',{
    catalog:object.catalog||view.catalog.value||'',kind:object.kind,name:object.name,action:'count_rows'
  });
  const detail={action:'count_rows',count:result?.count};
  renderStructure(view,object,detail);activatePanel(view,'structure');
}

function objectMenu(view,object,x,y){
  const objectKind=String(object.kind||'').toLowerCase();
  const adapterKind=view.meta.adapter_kind||adapterForView(view)?.kind||'';
  const canBrowse=supports(view,'browse_rows')&&['table','view','collection'].includes(objectKind);
  const canAction=supports(view,'object_actions')&&['table','view','collection'].includes(objectKind);
  const writable=canAction&&!profileForView(view)?.read_only;
  const qualified=qualifiedName(view,object);
  const isCollection=adapterKind==='mongo'&&objectKind==='collection';
  const isView=objectKind==='view';
  showContextMenu([
    {label:'View Data',disabled:!canBrowse,action:()=>openTableData(view,object)},
    {label:'Inspect / Structure',action:()=>inspectObject(view,object,{activate:true})},
    {label:'Refresh Objects',action:()=>view.refresh.click()},
    {separator:true},
    {label:'Copy Name',action:()=>copyText(object.name)},
    {label:'Copy Qualified Name',action:()=>copyText(qualified)},
    {label:'Open in Query Editor',disabled:!queryTemplateCapable(view),action:()=>generatedQuery(view,object,'select')},
    {separator:true},
    {label:isCollection?'Count Documents':'Count Rows',disabled:!canAction,action:()=>countRows(view,object)},
    {label:isCollection?'Generate Find':'Generate SELECT',disabled:!queryTemplateCapable(view),action:()=>generatedQuery(view,object,'select')},
    {label:'Generate INSERT',disabled:!['mysql','sqlite'].includes(adapterKind),action:()=>generatedQuery(view,object,'insert')},
    {label:'Generate UPDATE',disabled:!['mysql','sqlite'].includes(adapterKind),action:()=>generatedQuery(view,object,'update')},
    {label:'Generate DELETE',disabled:!['mysql','sqlite'].includes(adapterKind),action:()=>generatedQuery(view,object,'delete')},
    {separator:true},
    {label:isCollection?'Clear Collection':'Truncate Table',danger:true,disabled:!writable||isView,action:()=>runObjectAction(view,object,'truncate')},
    {label:isCollection?'Drop Collection':'Drop '+(isView?'View':'Table'),danger:true,disabled:!writable,action:()=>runObjectAction(view,object,'drop')}
  ],x,y);
}

async function runObjectAction(view,object,action){
  const adapterKind=view.meta.adapter_kind||adapterForView(view)?.kind||'';
  const isCollection=adapterKind==='mongo'&&String(object.kind||'').toLowerCase()==='collection';
  if(action==='truncate'){
    const message=isCollection
      ?'Clear collection "'+object.name+'"? All documents will be permanently removed.'
      :'Truncate table "'+object.name+'"? All rows will be permanently removed.';
    if(!confirm(message))return;
  }else if(action==='drop'){
    const typed=prompt('Type "'+object.name+'" to confirm dropping this '+(object.kind||'object')+':','');
    if(typed!==object.name)return;
  }
  await database.request(view.meta.id,'object_action',{
    catalog:object.catalog||view.catalog.value||'',kind:object.kind,name:object.name,action
  });
  if(action==='drop'){
    const state=dataState(view);
    if(state.object&&objectKey(state.object)===objectKey(object)){
      state.object=null;state.result=null;clearPendingChanges(view);wbSetDataTitle(view,null);
      view.workbench.grid.replaceChildren();
      const empty=document.createElement('div');empty.className='db-data-empty';empty.textContent='Object was dropped.';view.workbench.grid.append(empty);
    }
    view.refresh.click();
  }else{
    if(dataState(view).object&&objectKey(dataState(view).object)===objectKey(object))await loadData(view,{resetOffset:true});
  }
}

function catalogMenu(view,x,y){
  const catalog=view.catalog.value||'';
  showContextMenu([
    {label:'Refresh',action:()=>view.refresh.click()},
    {label:'Copy Database / Catalog Name',disabled:!catalog,action:()=>copyText(catalog)},
    {label:'Open Query Editor',action:()=>activatePanel(view,'query')}
  ],x,y);
}

function applyObjectFilter(view){
  const filter=String(view.workbench?.objectFilter?.value||'').trim().toLowerCase();
  for(const button of view.objects.querySelectorAll('.db-object')){
    const haystack=(button.dataset.kind+' '+button.dataset.name+' '+button.dataset.catalog).toLowerCase();
    button.style.display=!filter||haystack.includes(filter)?'':'none';
  }
}

function bindObject(view,object,button){
  if(!view?.workbench||button.dataset.workbenchBound==='1')return;
  button.dataset.workbenchBound='1';
  button.addEventListener('dblclick',event=>{event.preventDefault();openTableData(view,object).catch(app.showError);});
  button.addEventListener('contextmenu',event=>{event.preventDefault();event.stopPropagation();objectMenu(view,object,event.clientX,event.clientY);});
  applyObjectFilter(view);
}

function filterCapable(view){
  const kind=view.meta.adapter_kind||adapterForView(view)?.kind||'';
  return kind==='mysql'||kind==='sqlite';
}

function filterSummary(filters){
  if(!Array.isArray(filters)||!filters.length)return 'Filter…';
  return filters.map(item=>{
    const op=String(item.operator||'eq').replaceAll('_',' ');
    return item.column+' '+op+(item.operator==='is_null'||item.operator==='not_null'?'':' '+String(item.value??''));
  }).join(' AND ');
}

function openFilterDialog(view){
  if(!filterCapable(view))throw new Error('Grid filters are currently available for MySQL/MariaDB and SQLite');
  const state=dataState(view);const columns=Array.isArray(state.result?.columns)?state.result.columns:[];
  if(!columns.length)throw new Error('Open table data before adding a filter');

  const dialog=document.createElement('div');dialog.className='task-connection-dialog';
  const card=document.createElement('div');card.className='task-connection-dialog-card';
  const title=document.createElement('h3');title.textContent='Filter Rows';
  const hint=document.createElement('div');hint.className='db-data-status';hint.textContent='Filters run on the database server. Up to 16 conditions are combined with AND.';
  const list=document.createElement('div');list.className='db-filter-list';
  const actions=document.createElement('div');actions.className='task-connection-dialog-actions';
  const add=document.createElement('button');add.type='button';add.textContent='+ Condition';
  const clear=document.createElement('button');clear.type='button';clear.textContent='Clear';
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
  const apply=document.createElement('button');apply.type='button';apply.className='task-connection-primary';apply.textContent='Apply Filter';

  const operators=[
    ['eq','='],['ne','≠'],['lt','<'],['lte','≤'],['gt','>'],['gte','≥'],
    ['contains','contains'],['starts_with','starts with'],['is_null','is NULL'],['not_null','is not NULL']
  ];
  const addRow=(initial={})=>{
    if(list.children.length>=16)return;
    const row=document.createElement('div');row.className='db-filter-row';
    const column=document.createElement('select');
    for(const item of columns){const option=document.createElement('option');option.value=item.name;option.textContent=item.name;if(initial.column===item.name)option.selected=true;column.append(option);}
    const operator=document.createElement('select');
    for(const [value,label] of operators){const option=document.createElement('option');option.value=value;option.textContent=label;if((initial.operator||'eq')===value)option.selected=true;operator.append(option);}
    const value=document.createElement('input');value.type='text';value.value=initial.value==null?'':String(initial.value);value.placeholder='Value';
    const remove=document.createElement('button');remove.type='button';remove.textContent='×';remove.title='Remove condition';remove.onclick=()=>row.remove();
    const sync=()=>{const noValue=operator.value==='is_null'||operator.value==='not_null';value.disabled=noValue;value.style.visibility=noValue?'hidden':'';};
    operator.onchange=sync;sync();
    row.append(column,operator,value,remove);list.append(row);
  };
  for(const item of state.filters)addRow(item);
  if(!state.filters.length)addRow();
  add.onclick=()=>addRow();
  clear.onclick=()=>{
    if(!confirmDiscardChanges(view))return;
    state.filters=[];state.offset=0;dialog.remove();loadData(view).catch(app.showError);
  };
  cancel.onclick=()=>dialog.remove();
  apply.onclick=()=>{
    if(!confirmDiscardChanges(view))return;
    const filters=[];
    for(const row of list.querySelectorAll('.db-filter-row')){
      const selects=row.querySelectorAll('select');const input=row.querySelector('input');
      const column=selects[0]?.value||'';const operator=selects[1]?.value||'eq';
      if(!column)continue;
      const item={column,operator};
      if(operator!=='is_null'&&operator!=='not_null')item.value=input?.value??'';
      filters.push(item);
    }
    state.filters=filters.slice(0,16);state.offset=0;dialog.remove();loadData(view).catch(app.showError);
  };
  actions.append(add,clear,cancel,apply);card.append(title,hint,list,actions);dialog.append(card);document.body.append(dialog);
  dialog.addEventListener('pointerdown',event=>{if(event.target===dialog)dialog.remove();});
}

function createDataPanel(view){
  const panel=document.createElement('div');panel.className='db-workbench-panel db-data hidden';
  const tools=document.createElement('div');tools.className='db-data-tools';
  const objectTitle=document.createElement('strong');objectTitle.textContent='No table selected';
  const first=document.createElement('button');first.type='button';first.textContent='⏮';first.title='First page';
  const prev=document.createElement('button');prev.type='button';prev.textContent='◀';prev.title='Previous page';
  const next=document.createElement('button');next.type='button';next.textContent='▶';next.title='Next page';
  const pageSize=document.createElement('select');pageSize.title='Rows per page';
  for(const size of [25,50,100,250,500,1000]){const option=document.createElement('option');option.value=String(size);option.textContent=String(size)+' rows';if(size===100)option.selected=true;pageSize.append(option);}
  const refresh=document.createElement('button');refresh.type='button';refresh.textContent='↻';refresh.title='Refresh data';
  const filter=document.createElement('button');filter.type='button';filter.textContent='Filter…';filter.title='Server-side row filter';
  const add=document.createElement('button');add.type='button';add.textContent='+ Row';add.title='Insert row';
  const apply=document.createElement('button');apply.type='button';apply.className='db-apply';apply.textContent='Apply changes';
  const revert=document.createElement('button');revert.type='button';revert.textContent='Revert';
  const spacer=document.createElement('span');spacer.className='db-data-spacer';
  const status=document.createElement('span');status.className='db-data-status';status.textContent='Select a table';
  tools.append(objectTitle,first,prev,next,pageSize,refresh,filter,add,apply,revert,spacer,status);
  const grid=document.createElement('div');grid.className='db-data-grid-wrap';
  const empty=document.createElement('div');empty.className='db-data-empty';empty.textContent='Double-click a table or choose View Data from its context menu.';grid.append(empty);
  panel.append(tools,grid);
  view.workbench.controls={objectTitle,first,prev,next,pageSize,refresh,filter,add,apply,revert,status};
  view.workbench.grid=grid;
  first.onclick=()=>{if(!confirmDiscardChanges(view))return;const state=dataState(view);state.offset=0;loadData(view).catch(app.showError);};
  prev.onclick=()=>{if(!confirmDiscardChanges(view))return;const state=dataState(view);state.offset=Math.max(0,state.offset-state.limit);loadData(view).catch(app.showError);};
  next.onclick=()=>{if(!confirmDiscardChanges(view))return;const state=dataState(view);state.offset+=state.limit;loadData(view).catch(app.showError);};
  pageSize.onchange=()=>{if(!confirmDiscardChanges(view)){pageSize.value=String(dataState(view).limit);return;}const state=dataState(view);state.limit=Math.max(1,Math.min(1000,Number(pageSize.value)||100));state.offset=0;loadData(view).catch(app.showError);};
  refresh.onclick=()=>{if(!confirmDiscardChanges(view))return;loadData(view).catch(app.showError);};
  filter.disabled=!filterCapable(view);filter.onclick=()=>openFilterDialog(view);
  add.onclick=()=>addGridRow(view);
  apply.onclick=()=>applyGridChanges(view).catch(app.showError);
  revert.onclick=()=>{clearPendingChanges(view);renderDataGrid(view);};
  return panel;
}

function enhanceView(view){
  if(!view||view.workbench)return;
  const query=view.editor?.closest('.db-query');const body=query?.parentElement;
  if(!query||!body)return;
  const main=document.createElement('div');main.className='db-main';
  body.insertBefore(main,query);main.append(query);
  query.classList.add('db-workbench-panel');

  const tabsBar=document.createElement('div');tabsBar.className='db-workbench-tabs';
  const makeTab=(key,label)=>{
    const button=document.createElement('button');button.type='button';button.className='db-workbench-tab';button.textContent=label;
    button.onclick=()=>activatePanel(view,key);tabsBar.append(button);return button;
  };
  const tabs={data:makeTab('data','Data'),structure:makeTab('structure','Structure'),query:makeTab('query','Query')};
  const structure=document.createElement('div');structure.className='db-workbench-panel db-structure hidden';
  const structureEmpty=document.createElement('div');structureEmpty.className='db-data-empty';structureEmpty.textContent='Select an object to inspect its structure.';structure.append(structureEmpty);

  view.workbench={
    active:'query',tabs,panels:{data:null,structure,query},controls:null,grid:null,
    details:new Map(),objectFilter:null,
    data:{object:null,offset:0,limit:100,sort:[],filters:[],result:null,dirtyRows:new Map(),deletedRows:new Set(),newRows:[]}
  };
  const data=createDataPanel(view);view.workbench.panels.data=data;
  main.prepend(tabsBar);main.append(data,structure);
  activatePanel(view,'query');

  const browserHead=view.objects?.closest('.db-browser')?.querySelector('.db-browser-head');
  if(browserHead){
    const filter=document.createElement('input');filter.type='search';filter.className='db-browser-filter';filter.placeholder='Filter objects…';filter.title='Filter database objects';
    browserHead.insertBefore(filter,view.refresh);view.workbench.objectFilter=filter;
    filter.addEventListener('input',()=>applyObjectFilter(view));
    view.catalog.addEventListener('contextmenu',event=>{event.preventDefault();catalogMenu(view,event.clientX,event.clientY);});
  }
  for(const button of view.objects.querySelectorAll('.db-object')){
    const object=(view.objectData||[]).find(item=>String(item?.name||'')===button.dataset.name&&String(item?.kind||'')===button.dataset.kind);
    if(object)bindObject(view,object,button);
  }
}

globalThis.TaskMenuDatabaseWorkbench={enhanceView,bindObject,openTableData,closeContextMenu};

ensureAdapters().then(()=>{
  for(const view of database.views.values())enhanceView(view);
}).catch(app.showError);
for(const view of database.views.values())enhanceView(view);
