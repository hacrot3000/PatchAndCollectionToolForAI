const app=globalThis.TaskMenuApp;
const database=globalThis.TaskMenuDatabase;
if(!app||!database)throw new Error('TaskMenuDatabase unavailable for Database Workbench');

const adaptersByID=new Map();
let adapterLoadPromise=null;
let contextMenu=null;

const style=document.createElement('style');
style.textContent=`
.db-main{min-width:0;min-height:0;overflow:hidden;display:flex;flex-direction:column}
.db-workbench-tabs{height:34px;display:flex;align-items:end;gap:2px;padding:0 7px;border-bottom:1px solid #30343b;background:#11151b;overflow-x:auto;overflow-y:hidden;flex:0 0 auto}
.db-workbench-tab{border-radius:5px 5px 0 0;border-bottom:0;padding:6px 8px;font-size:11px;opacity:.7;display:inline-flex;align-items:center;gap:7px;max-width:260px;flex:0 0 auto}
.db-workbench-tab-label{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.db-workbench-tab-close{font-size:13px;line-height:1;opacity:.55}
.db-workbench-tab-close:hover{opacity:1}
.db-workbench-tab.active{background:#202630;opacity:1}
.db-workbench-panel{flex:1;min-height:0}
.db-workbench-panel.hidden{display:none}
.db-browser-filter{min-width:0;flex:1;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:5px 7px;font-size:11px}
.db-data{display:flex;flex-direction:column;min-width:0;min-height:0;overflow:hidden}
.db-data-tools{display:flex;align-items:center;gap:5px;flex-wrap:wrap;padding:6px 7px;border-bottom:1px solid #30343b}
.db-data-tools button,.db-data-tools select{font-size:11px}
.db-data-tools select{background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:5px}
.db-data-spacer{flex:1}
.db-data-status{font-size:10px;opacity:.65;white-space:nowrap}
.db-data-grid-wrap{flex:1;min-width:0;min-height:0;overflow:auto;scrollbar-gutter:stable}
.db-data-grid{border-collapse:collapse;min-width:100%;font-family:ui-monospace,monospace;font-size:11px}
.db-data-grid th,.db-data-grid td{border-right:1px solid #272d36;border-bottom:1px solid #272d36;padding:5px 7px;text-align:left;vertical-align:top;white-space:pre-wrap;max-width:520px}
.db-data-grid th{position:sticky;top:0;background:#171c23;z-index:2;cursor:pointer;user-select:none}
.db-data-grid th.db-row-number,.db-data-grid td.db-row-number{position:sticky;left:0;z-index:3;width:42px;min-width:42px;text-align:right;opacity:.55;background:#11151b}
.db-data-grid th.db-row-number{z-index:4}
.db-data-grid td.db-editable{cursor:text;outline:none}
.db-data-grid td.db-editable:focus{box-shadow:inset 0 0 0 1px #3f79a8;background:#121a23}
.db-data-grid td.db-dirty{background:#332d18}
.db-data-grid tr.db-deleted td{text-decoration:line-through;opacity:.5}
.db-data-grid tr.db-selected td{background:#19334d}
.db-data-grid tr.db-selected td.db-dirty{background:#3c3920}
.db-data-grid td.db-row-number{cursor:default;user-select:none}
.db-data-grid th.db-row-number{cursor:pointer;user-select:none}
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

function rootWorkbenchView(view){
  return view?.workbenchRoot||view;
}

function activatePanel(view,name){
  const root=rootWorkbenchView(view);
  const wb=root?.workbench;
  if(!wb)return;
  wb.active=name;
  if(wb.pages instanceof Map){
    for(const [key,page] of wb.pages){
      page.tab?.classList.toggle('active',key===name);
      page.panel?.classList.toggle('hidden',key!==name);
    }
    return;
  }
  for(const [key,button] of Object.entries(wb.tabs||{}))button.classList.toggle('active',key===name);
  for(const [key,panel] of Object.entries(wb.panels||{}))panel.classList.toggle('hidden',key!==name);
}

function workbenchObjectPageKey(mode,object){
  return mode+':'+objectKey(object);
}

function createWorkbenchTab(view,key,label,{closable=true}={}){
  const root=rootWorkbenchView(view);const wb=root.workbench;
  const button=document.createElement('button');button.type='button';button.className='db-workbench-tab';button.title=label;
  const text=document.createElement('span');text.className='db-workbench-tab-label';text.textContent=label;button.append(text);
  if(closable){
    const close=document.createElement('span');close.className='db-workbench-tab-close';close.textContent='×';close.title='Close tab';
    close.onclick=event=>{event.preventDefault();event.stopPropagation();closeWorkbenchPage(root,key);};
    button.append(close);
  }
  button.onclick=()=>activatePanel(root,key);
  button.oncontextmenu=event=>{event.preventDefault();event.stopPropagation();showWorkbenchTabMenu(root,key,event.clientX,event.clientY);};
  wb.tabsBar.append(button);
  return button;
}

function workbenchPageKeys(view){
  const root=rootWorkbenchView(view);
  return Array.from(root.workbench?.pages?.keys?.()||[]);
}

function closeWorkbenchPages(view,keys){
  const root=rootWorkbenchView(view);const wb=root?.workbench;
  if(!wb)return false;
  const pages=keys.map(key=>wb.pages.get(key)).filter(page=>page&&page.mode!=='query');
  if(!pages.length)return false;
  const dirty=pages.filter(page=>page.mode==='data'&&page.ctx&&hasPendingChanges(page.ctx));
  if(dirty.length&&!confirm('Discard unsaved database grid changes in '+dirty.length+' tab'+(dirty.length===1?'':'s')+'?'))return false;
  const activeWillClose=pages.some(page=>page.key===wb.active);
  for(const page of pages)closeWorkbenchPage(root,page.key,{force:true,activateFallback:false});
  if(activeWillClose)activatePanel(root,'query');
  return true;
}

function showWorkbenchTabMenu(view,key,x,y){
  const root=rootWorkbenchView(view);const keys=workbenchPageKeys(root);const index=keys.indexOf(key);
  const page=root.workbench?.pages?.get(key);if(!page||index<0)return;
  const left=keys.slice(0,index).filter(item=>root.workbench.pages.get(item)?.mode!=='query');
  const right=keys.slice(index+1).filter(item=>root.workbench.pages.get(item)?.mode!=='query');
  const others=keys.filter(item=>item!==key&&root.workbench.pages.get(item)?.mode!=='query');
  showContextMenu([
    {label:'Close this',disabled:page.mode==='query',action:()=>closeWorkbenchPage(root,key)},
    {label:'Close all but this',disabled:others.length===0,action:()=>closeWorkbenchPages(root,others)},
    {separator:true},
    {label:'Close all right tabs',disabled:right.length===0,action:()=>closeWorkbenchPages(root,right)},
    {label:'Close all left tabs',disabled:left.length===0,action:()=>closeWorkbenchPages(root,left)}
  ],x,y);
}

function closeWorkbenchPage(view,key,{force=false,activateFallback=true}={}){
  const root=rootWorkbenchView(view);const wb=root?.workbench;const page=wb?.pages?.get(key);
  if(!page||page.mode==='query')return false;
  if(!force&&page.mode==='data'&&page.ctx&&hasPendingChanges(page.ctx)&&!confirm('Discard unsaved database grid changes?'))return false;
  page.tab?.remove();page.panel?.remove();wb.pages.delete(key);
  if(activateFallback&&wb.active===key)activatePanel(root,'query');
  return true;
}

function createWorkbenchChildView(view,mode){
  const root=rootWorkbenchView(view);
  const child=Object.create(root);
  child.workbenchRoot=root;
  child.workbench={
    active:mode,
    controls:null,
    grid:null,
    panels:{},
    details:root.workbench.details,
    data:{object:null,offset:0,limit:100,sort:[],filters:[],result:null,busy:false,dirtyRows:new Map(),deletedRows:new Set(),newRows:[],selectedRows:new Set(),selectionAnchor:null}
  };
  return child;
}

function ensureDataPage(view,object){
  const root=rootWorkbenchView(view);const wb=root.workbench;const key=workbenchObjectPageKey('data',object);
  const existing=wb.pages.get(key);if(existing)return existing;
  const ctx=createWorkbenchChildView(root,'data');const state=dataState(ctx);
  state.object=object;state.limit=storedPageSize(root,object);
  const panel=createDataPanel(ctx);ctx.workbench.panels.data=panel;
  ctx.workbench.controls.pageSize.value=String(state.limit);wbSetDataTitle(ctx,object);
  const tab=createWorkbenchTab(root,key,object.name+' - Data');
  const page={key,mode:'data',object,ctx,tab,panel};wb.pages.set(key,page);wb.main.append(panel);
  return page;
}

function ensureStructurePage(view,object){
  const root=rootWorkbenchView(view);const wb=root.workbench;const key=workbenchObjectPageKey('structure',object);
  const existing=wb.pages.get(key);if(existing)return existing;
  const ctx=createWorkbenchChildView(root,'structure');
  const panel=document.createElement('div');panel.className='db-workbench-panel db-structure hidden';ctx.workbench.panels.structure=panel;
  const empty=document.createElement('div');empty.className='db-data-empty';empty.textContent='Loading structure…';panel.append(empty);
  const tab=createWorkbenchTab(root,key,object.name+' - Structure');
  const page={key,mode:'structure',object,ctx,tab,panel};wb.pages.set(key,page);wb.main.append(panel);
  return page;
}

function setQuery(view,text){
  view.editor.value=text;
  activatePanel(view,'query');
  view.editor.focus();
}

function appendStructureHeading(panel,text){
  const heading=document.createElement('div');heading.className='db-structure-title';heading.style.marginTop='14px';heading.textContent=text;panel.append(heading);
}

function appendStructureTable(panel,headers,rows){
  const table=document.createElement('table');table.className='db-structure-table';
  const thead=document.createElement('thead');const hr=document.createElement('tr');
  for(const label of headers){const th=document.createElement('th');th.textContent=label;hr.append(th);}
  thead.append(hr);table.append(thead);
  const tbody=document.createElement('tbody');
  for(const values of rows){
    const tr=document.createElement('tr');
    for(const value of values){const td=document.createElement('td');td.textContent=value==null?'':(typeof value==='object'?JSON.stringify(value):String(value));tr.append(td);}
    tbody.append(tr);
  }
  table.append(tbody);panel.append(table);
}

function renderStructure(view,object,detail){
  const panel=view.workbench?.panels?.structure;if(!panel)return;
  panel.replaceChildren();
  const title=document.createElement('div');title.className='db-structure-title';title.textContent=(object.kind||'object')+' · '+object.name;
  const meta=document.createElement('div');meta.className='db-structure-meta';meta.textContent=object.catalog||view.catalog?.value||'';
  panel.append(title,meta);

  const columns=Array.isArray(detail?.columns)?detail.columns:null;
  if(columns){
    appendStructureHeading(panel,'Columns');
    appendStructureTable(panel,['Column','Type','Key','Nullable','Default','Extra'],columns.map(column=>{
      const nullable=typeof column?.nullable==='boolean'
        ?column.nullable
        :(typeof column?.not_null==='boolean'?!column.not_null:true);
      const key=column?.primary_key?'PK':'';
      return [column?.name,column?.type,key,nullable?'YES':'NO',column?.default??'NULL',column?.extra||''];
    }));
  }

  const indexes=Array.isArray(detail?.indexes)
    ?detail.indexes
    :(Array.isArray(detail?.detail?.indexes)?detail.detail.indexes:null);
  if(indexes){
    appendStructureHeading(panel,'Indexes / Keys');
    appendStructureTable(panel,['Name','Column / Key','Unique','Type / Origin','Sequence'],indexes.map(index=>{
      const key=index?.column_name??index?.key??'';
      const unique=typeof index?.unique==='boolean'?index.unique:(index?.name==='_id_'?true:'');
      return [index?.name||'',key,unique===true?'YES':(unique===false?'NO':''),index?.type||index?.origin||'',index?.sequence??index?.seq??''];
    }));
  }

  const sql=detail?.sql;
  if(sql){
    appendStructureHeading(panel,'Definition');
    const pre=document.createElement('pre');pre.className='db-structure-json';pre.textContent=String(sql);panel.append(pre);
  }

  if(!columns&&!indexes){
    const raw=detail?.detail??detail;
    const pre=document.createElement('pre');pre.className='db-structure-json';pre.textContent=JSON.stringify(raw,null,2);panel.append(pre);
  }else if(detail?.detail?.info){
    appendStructureHeading(panel,'Collection Info');
    const pre=document.createElement('pre');pre.className='db-structure-json';pre.textContent=JSON.stringify(detail.detail.info,null,2);panel.append(pre);
  }
}

async function inspectObject(view,object,{activate=false,open=true}={}){
  const root=rootWorkbenchView(view);
  const page=open?ensureStructurePage(root,object):null;
  if(page&&activate)activatePanel(root,page.key);
  const payload={name:object.name,kind:object.kind};
  if(object.catalog||root.catalog.value)payload.catalog=object.catalog||root.catalog.value;
  const detail=await database.request(root.meta.id,'describe_object',payload);
  root.workbench.details.set(objectKey(object),detail);
  if(page)renderStructure(page.ctx,object,detail);
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


const MAX_COPY_ALL_ROWS=100000;

function clipboardValue(value){
  if(value===null||value===undefined)return '';
  if(typeof value==='object')return JSON.stringify(value);
  return String(value);
}

function currentPageCopyData(view,{selectedOnly=false}={}){
  const state=dataState(view);const result=state.result||{};
  const columns=Array.isArray(result.columns)?result.columns.map(column=>String(column?.name||'')):[];
  const sourceRows=Array.isArray(result.rows)?result.rows:[];
  const indexes=selectedOnly?selectedRowIndexes(view):sourceRows.map((_,index)=>index);
  const rows=indexes.map(rowIndex=>columns.map((_,columnIndex)=>currentCellValue(view,rowIndex,columnIndex)));
  if(!selectedOnly){
    for(const values of state.newRows||[]){
      rows.push(columns.map(name=>Object.prototype.hasOwnProperty.call(values,name)?values[name]:null));
    }
  }
  return {columns,rows};
}

async function allPagesCopyData(view){
  const state=dataState(view);const object=state.object;
  if(!object)throw new Error('Open database data before copying');
  if(!supports(view,'browse_rows'))throw new Error('This database adapter does not support data browsing');
  if(hasPendingChanges(view)&&!confirm('Copy all pages uses saved database values and excludes unsaved grid changes. Continue?'))return null;
  const rows=[];let columns=[];let offset=0;
  while(true){
    const result=await database.request(view.meta.id,'browse_rows',{
      catalog:object.catalog||view.catalog.value||'',
      kind:object.kind||'table',
      name:object.name,
      offset,
      limit:1000,
      sort:state.sort,
      filters:state.filters
    })||{};
    const pageColumns=Array.isArray(result.columns)?result.columns:[];
    if(!columns.length)columns=pageColumns.map(column=>String(column?.name||''));
    const pageRows=Array.isArray(result.rows)?result.rows:[];
    for(const row of pageRows){
      rows.push(Array.isArray(row?.values)?row.values:[]);
      if(rows.length>MAX_COPY_ALL_ROWS)throw new Error('Copy all pages is limited to '+MAX_COPY_ALL_ROWS.toLocaleString()+' rows');
    }
    if(!result.has_more||pageRows.length===0)break;
    offset+=pageRows.length;
  }
  return {columns,rows};
}

function delimitedClipboardText(columns,rows,{delimiter,includeHeaders}){
  const quote=value=>{
    const text=clipboardValue(value);
    if(text.includes('"')||text.includes('\n')||text.includes('\r')||text.includes(delimiter))return '"'+text.replaceAll('"','""')+'"';
    return text;
  };
  const lines=[];
  if(includeHeaders)lines.push(columns.map(quote).join(delimiter));
  for(const row of rows)lines.push(columns.map((_,index)=>quote(row?.[index])).join(delimiter));
  return lines.join('\n');
}

function serializeClipboardData(format,columns,rows,includeHeaders){
  switch(format){
  case 'csv':
    return delimitedClipboardText(columns,rows,{delimiter:',',includeHeaders});
  case 'json':
    if(includeHeaders){
      return JSON.stringify(rows.map(row=>Object.fromEntries(columns.map((name,index)=>[name,row?.[index]??null]))),null,2);
    }
    return JSON.stringify(rows,null,2);
  case 'txt':
  default:
    return delimitedClipboardText(columns,rows,{delimiter:'\t',includeHeaders});
  }
}

async function collectClipboardData(view,scope){
  if(scope==='all')return await allPagesCopyData(view);
  return currentPageCopyData(view,{selectedOnly:scope==='selected'});
}

function openCopyDataDialog(view,{defaultScope='current'}={}){
  const state=dataState(view);const selected=selectedRowIndexes(view).length;
  const dialog=document.createElement('div');dialog.className='task-connection-dialog';
  const card=document.createElement('div');card.className='task-connection-dialog-card';
  const title=document.createElement('h3');title.textContent='Copy Data';

  const formatLabel=document.createElement('label');formatLabel.textContent='Format';
  const format=document.createElement('select');
  for(const [value,label] of [['txt','TXT / tab-separated'],['csv','CSV'],['json','JSON']]){
    const option=document.createElement('option');option.value=value;option.textContent=label;format.append(option);
  }

  const scopeLabel=document.createElement('label');scopeLabel.textContent='Scope';
  const scope=document.createElement('select');
  if(selected){
    const option=document.createElement('option');option.value='selected';option.textContent='Selected rows ('+selected+')';scope.append(option);
  }
  for(const [value,label] of [['current','Current page'],['all','All pages']]){
    const option=document.createElement('option');option.value=value;option.textContent=label;scope.append(option);
  }
  if(Array.from(scope.options).some(option=>option.value===defaultScope))scope.value=defaultScope;

  const headerLabel=document.createElement('label');headerLabel.style.display='flex';headerLabel.style.alignItems='center';headerLabel.style.gap='7px';
  const headers=document.createElement('input');headers.type='checkbox';headers.checked=true;
  const headerText=document.createElement('span');headerText.textContent='Include column names';
  headerLabel.append(headers,headerText);

  const hint=document.createElement('div');hint.className='db-data-status';
  hint.textContent='All pages follows the current sort/filter, reads up to 1,000 rows per request, and refuses copies above '+MAX_COPY_ALL_ROWS.toLocaleString()+' rows. JSON with column names copies objects; without them it copies arrays.';

  const actions=document.createElement('div');actions.className='task-connection-dialog-actions';
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=()=>dialog.remove();
  const copy=document.createElement('button');copy.type='button';copy.className='task-connection-primary';copy.textContent='Copy';
  copy.onclick=async()=>{
    copy.disabled=true;const oldText=copy.textContent;copy.textContent='Copying…';
    try{
      const data=await collectClipboardData(view,scope.value);
      if(!data)return;
      await copyText(serializeClipboardData(format.value,data.columns,data.rows,headers.checked));
      dialog.remove();
    }finally{copy.disabled=false;copy.textContent=oldText;}
  };
  actions.append(cancel,copy);
  card.append(title,formatLabel,format,scopeLabel,scope,headerLabel,hint,actions);dialog.append(card);document.body.append(dialog);
  dialog.addEventListener('pointerdown',event=>{if(event.target===dialog)dialog.remove();});
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
  const busy=Boolean(wb.data.busy);
  const editable=Boolean(result.editable)&&supports(view,'mutate_rows');
  const pending=pendingChangeCount(view);
  const selected=wb.data.selectedRows?.size||0;
  wb.controls.add.disabled=busy||!editable;
  wb.controls.apply.disabled=busy||!editable||pending===0;
  wb.controls.revert.disabled=busy||!hasPendingChanges(view);
  const base=wb.controls.status.dataset.base||wb.controls.status.textContent||'';
  wb.controls.status.textContent=base+(selected?' · '+selected+' selected':'')+(pending?' · '+pending+' pending':'');
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


function selectedRowIndexes(view){
  const state=dataState(view);const rowCount=Array.isArray(state.result?.rows)?state.result.rows.length:0;
  return Array.from(state.selectedRows||[]).filter(index=>Number.isInteger(index)&&index>=0&&index<rowCount).sort((a,b)=>a-b);
}

function syncGridSelection(view){
  const state=dataState(view);const selected=state.selectedRows||new Set();const rowCount=Array.isArray(state.result?.rows)?state.result.rows.length:0;
  for(const tr of view.workbench.grid?.querySelectorAll?.('tbody tr[data-row-index]')||[]){
    const index=Number(tr.dataset.rowIndex);tr.classList.toggle('db-selected',selected.has(index));
  }
  const selectAll=view.workbench.grid?.querySelector?.('th.db-row-number[data-select-all]');
  if(selectAll){
    const selectedCount=selectedRowIndexes(view).length;
    selectAll.textContent=rowCount>0&&selectedCount===rowCount?'☑':(selectedCount>0?'◩':'☐');
    selectAll.title=rowCount>0&&selectedCount===rowCount?'Clear row selection':'Select all rows on this page';
  }
  updateEditControls(view);
}

function selectGridRow(view,rowIndex,event={}){
  const state=dataState(view);const rowCount=Array.isArray(state.result?.rows)?state.result.rows.length:0;
  if(rowIndex<0||rowIndex>=rowCount)return;
  if(!(state.selectedRows instanceof Set))state.selectedRows=new Set();
  const additive=Boolean(event.ctrlKey||event.metaKey);
  if(event.shiftKey&&Number.isInteger(state.selectionAnchor)){
    const start=Math.min(state.selectionAnchor,rowIndex);const end=Math.max(state.selectionAnchor,rowIndex);
    if(!additive)state.selectedRows.clear();
    for(let index=start;index<=end;index++)state.selectedRows.add(index);
  }else if(additive){
    if(state.selectedRows.has(rowIndex))state.selectedRows.delete(rowIndex);else state.selectedRows.add(rowIndex);
    state.selectionAnchor=rowIndex;
  }else{
    state.selectedRows.clear();state.selectedRows.add(rowIndex);state.selectionAnchor=rowIndex;
  }
  syncGridSelection(view);
}

function toggleSelectAllPage(view){
  const state=dataState(view);const rowCount=Array.isArray(state.result?.rows)?state.result.rows.length:0;
  if(!(state.selectedRows instanceof Set))state.selectedRows=new Set();
  const allSelected=rowCount>0&&selectedRowIndexes(view).length===rowCount;
  state.selectedRows.clear();
  if(!allSelected)for(let index=0;index<rowCount;index++)state.selectedRows.add(index);
  state.selectionAnchor=rowCount?0:null;
  syncGridSelection(view);
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

function refreshWorkbenchCapabilities(view){
  const wb=view.workbench;if(!wb?.controls)return;
  wb.controls.count.disabled=!supports(view,'object_actions');
  wb.controls.filter.disabled=!filterCapable(view);
  updateEditControls(view);
}

function setDataBusy(view,busy){
  const wb=view.workbench;if(!wb)return;
  wb.data.busy=busy;
  for(const button of [wb.controls.first,wb.controls.prev,wb.controls.next,wb.controls.refresh,wb.controls.count,wb.controls.filter])if(button)button.disabled=busy;
  if(wb.controls.pageSize)wb.controls.pageSize.disabled=busy;
  if(!busy)refreshWorkbenchCapabilities(view);
  else updateEditControls(view);
}

async function loadData(view,{resetOffset=false}={}){
  const state=dataState(view);
  if(!state.object)return;
  await ensureAdapters();
  if(!supports(view,'browse_rows'))throw new Error('This database adapter does not support data browsing');
  if(resetOffset)state.offset=0;
  setDataBusy(view,true);
  const status=view.workbench?.controls?.status;
  if(status){status.dataset.base='Loading…';status.textContent='Loading…';}
  try{
    const result=await database.request(view.meta.id,'browse_rows',dataPayload(view));
    state.result=result||{};
    state.selectedRows?.clear?.();state.selectionAnchor=null;
    clearPendingChanges(view);
    renderDataGrid(view);
  }catch(error){
    if(status){
      const message=String(error?.message||error||'Load failed');
      status.dataset.base='Error · '+message.slice(0,180);status.textContent=status.dataset.base;
    }
    throw error;
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
  const nr=document.createElement('th');nr.className='db-row-number';nr.dataset.selectAll='1';nr.textContent='☐';nr.title='Select all rows on this page';
  nr.onclick=event=>{event.preventDefault();toggleSelectAllPage(view);};
  nr.oncontextmenu=event=>{
    event.preventDefault();event.stopPropagation();
    const selected=selectedRowIndexes(view).length;
    showContextMenu([
      {label:selected?'Clear row selection':'Select all rows on this page',action:()=>toggleSelectAllPage(view)},
      {separator:true},
      {label:'Copy Data…',action:()=>openCopyDataDialog(view,{defaultScope:selected?'selected':'current'})}
    ],event.clientX,event.clientY);
  };
  hr.append(nr);
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
    if(state.selectedRows?.has(rowIndex))tr.classList.add('db-selected');
    const rowNo=document.createElement('td');rowNo.className='db-row-number';rowNo.textContent=String(state.offset+rowIndex+1);rowNo.title='Click to select row · Ctrl/Cmd-click multi-select · Shift-click range';
    rowNo.onclick=event=>{event.preventDefault();selectGridRow(view,rowIndex,event);};tr.append(rowNo);
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
        td.addEventListener('keydown',event=>{
          if(event.key==='Escape'){event.preventDefault();renderDataGrid(view);return;}
          if(event.key==='Enter'&&!event.shiftKey){
            event.preventDefault();
            const nextColumn=Math.min(columns.length-1,columnIndex+1);
            td.blur();
            requestAnimationFrame(()=>view.workbench.grid.querySelector('td[data-row-index="'+rowIndex+'"][data-column-index="'+nextColumn+'"]')?.focus());
          }
        });
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
      event.preventDefault();
      if(!state.selectedRows?.has(rowIndex))selectGridRow(view,rowIndex,{});
      showRowMenu(view,rowIndex,event.clientX,event.clientY);
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
      td.addEventListener('keydown',event=>{
        if(event.key==='Escape'){event.preventDefault();renderDataGrid(view);return;}
        if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();td.blur();}
      });
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
  table.append(tbody);wb.grid.append(table);syncGridSelection(view);
  if(rows.length===0&&state.newRows.length===0){
    const empty=document.createElement('div');empty.className='db-data-empty';empty.textContent=state.filters.length?'No rows match the current filters.':'This object contains no rows/documents/entries.';wb.grid.append(empty);
  }
}

function showCellMenu(view,rowIndex,columnIndex,x,y){
  const state=dataState(view);const result=state.result||{};const row=result.rows?.[rowIndex];const column=result.columns?.[columnIndex];
  if(!state.selectedRows?.has(rowIndex))selectGridRow(view,rowIndex,{});
  const value=currentCellValue(view,rowIndex,columnIndex);
  const editable=Boolean(result.editable)&&supports(view,'mutate_rows')&&column?.editable!==false&&!state.deletedRows.has(rowIndex);
  const dirty=state.dirtyRows.get(rowIndex)?.has(column?.name);
  showContextMenu([
    {label:'Copy Value',action:()=>copyText(displayValue(value))},
    {label:'Copy Column Name',action:()=>copyText(column?.name||'')},
    {label:'Copy Data…',action:()=>openCopyDataDialog(view,{defaultScope:'selected'})},
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
    {label:'Copy Data…',action:()=>openCopyDataDialog(view,{defaultScope:'selected'})},
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

function pageSizePreferenceKey(view,object){
  return 'taskdeck.db.pageSize.'+[
    view.meta?.profile_id||'session',
    object?.catalog||view.catalog?.value||'',
    object?.kind||'object',
    object?.name||''
  ].map(value=>encodeURIComponent(String(value))).join('.');
}

function storedPageSize(view,object){
  try{
    const value=Number(localStorage.getItem(pageSizePreferenceKey(view,object)));
    return [25,50,100,250,500,1000].includes(value)?value:100;
  }catch{return 100;}
}

function storePageSize(view,object,value){
  try{localStorage.setItem(pageSizePreferenceKey(view,object),String(value));}catch{}
}

async function openTableData(view,object){
  const root=rootWorkbenchView(view);
  const page=ensureDataPage(root,object);
  activatePanel(root,page.key);
  const state=dataState(page.ctx);
  if(state.result||state.busy)return;
  await loadData(page.ctx);
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
  const items=Array.isArray(response?.results)?response.results:null;
  if(!items||items.length!==mutations.length){
    clearPendingChanges(view);
    await loadData(view);
    throw new Error('Database adapter returned an incomplete mutation result; data was reloaded');
  }
  const errors=items.filter(item=>item?.error);
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
  return kind==='mysql'||kind==='sqlite'||kind==='mongo'||kind==='redis';
}

function redisCommandArg(value){
  const text=String(value??'');
  if(/[\u0000-\u001f\u007f]/.test(text))throw new Error('Redis Query template does not support control characters in key names');
  return '"'+text.replaceAll('\\','\\\\').replaceAll('"','\\"')+'"';
}

async function generatedQuery(view,object,action){
  const root=rootWorkbenchView(view);
  const kind=root.meta.adapter_kind||adapterForView(root)?.kind||'';
  if(kind==='redis'){
    if(action!=='select')throw new Error('Redis write templates are not generated');
    let detail=root.workbench.details.get(objectKey(object));
    if(!detail)detail=await inspectObject(root,object,{open:false});
    const key=redisCommandArg(object.name);
    const type=String(detail?.type||'').toLowerCase();
    const command=type==='string'?'GET '+key
      :type==='hash'?'HGETALL '+key
      :type==='list'?'LRANGE '+key+' 0 99'
      :type==='set'?'SMEMBERS '+key
      :type==='zset'?'ZRANGE '+key+' 0 99 WITHSCORES'
      :'TYPE '+key;
    setQuery(view,command);
    return;
  }
  if(kind==='mongo'){
    if(action!=='select')throw new Error('MongoDB write templates are intentionally not generated; edit documents in Data or use the safe Query find DSL');
    const payload={op:'find',database:object.catalog||root.catalog.value||'',collection:object.name,filter:{},limit:100};
    setQuery(root,JSON.stringify(payload,null,2));
    return;
  }
  let detail=root.workbench.details.get(objectKey(object));
  if(!detail&&(action==='insert'||action==='update'))detail=await inspectObject(root,object,{open:false});
  const text=queryTemplate(root,object,action,detail);
  if(!text)throw new Error('Query template generation is not available for this adapter');
  setQuery(root,text);
}

async function countRows(view,object){
  const root=rootWorkbenchView(view);
  const result=await database.request(root.meta.id,'object_action',{
    catalog:object.catalog||root.catalog.value||'',kind:object.kind,name:object.name,action:'count_rows'
  });
  const page=ensureStructurePage(root,object);
  const detail={action:'count_rows',count:result?.count};
  renderStructure(page.ctx,object,detail);activatePanel(root,page.key);
}

function objectMenu(view,object,x,y){
  const objectKind=String(object.kind||'').toLowerCase();
  const adapterKind=view.meta.adapter_kind||adapterForView(view)?.kind||'';
  const canBrowse=supports(view,'browse_rows')&&['table','view','collection','key'].includes(objectKind);
  const canAction=supports(view,'object_actions')&&['table','view','collection'].includes(objectKind);
  const writable=canAction&&!profileForView(view)?.read_only;
  const qualified=qualifiedName(view,object);
  const isCollection=adapterKind==='mongo'&&objectKind==='collection';
  const isRedisKey=adapterKind==='redis'&&objectKind==='key';
  const isView=objectKind==='view';
  showContextMenu([
    {label:isRedisKey?'View Value':'View Data',disabled:!canBrowse,action:()=>openTableData(view,object)},
    {label:'Inspect / Structure',action:()=>inspectObject(view,object,{activate:true})},
    {label:'Refresh Objects',action:()=>view.refresh.click()},
    {separator:true},
    {label:'Copy Name',action:()=>copyText(object.name)},
    {label:'Copy Qualified Name',action:()=>copyText(qualified)},
    {label:'Open in Query Editor',disabled:!queryTemplateCapable(view),action:()=>generatedQuery(view,object,'select')},
    {separator:true},
    {label:isRedisKey?'Count Entries':(isCollection?'Count Documents':'Count Rows'),disabled:!canAction,action:()=>countRows(view,object)},
    {label:isRedisKey?'Generate Read Command':(isCollection?'Generate Find':'Generate SELECT'),disabled:!queryTemplateCapable(view),action:()=>generatedQuery(view,object,'select')},
    {label:'Generate INSERT',disabled:!['mysql','sqlite'].includes(adapterKind),action:()=>generatedQuery(view,object,'insert')},
    {label:'Generate UPDATE',disabled:!['mysql','sqlite'].includes(adapterKind),action:()=>generatedQuery(view,object,'update')},
    {label:'Generate DELETE',disabled:!['mysql','sqlite'].includes(adapterKind),action:()=>generatedQuery(view,object,'delete')},
    {separator:true},
    {label:isRedisKey?'Clear Key':(isCollection?'Clear Collection':'Truncate Table'),danger:true,disabled:isRedisKey||!writable||isView,action:()=>runObjectAction(view,object,'truncate')},
    {label:isRedisKey?'Delete Key':(isCollection?'Drop Collection':'Drop '+(isView?'View':'Table')),danger:true,disabled:!writable,action:()=>runObjectAction(view,object,'drop')}
  ],x,y);
}

async function runObjectAction(view,object,action){
  const root=rootWorkbenchView(view);
  const adapterKind=root.meta.adapter_kind||adapterForView(root)?.kind||'';
  const objectKind=String(object.kind||'').toLowerCase();
  const isCollection=adapterKind==='mongo'&&objectKind==='collection';
  const isRedisKey=adapterKind==='redis'&&objectKind==='key';
  if(action==='truncate'){
    const message=isCollection
      ?'Clear collection "'+object.name+'"? All documents will be permanently removed.'
      :'Truncate table "'+object.name+'"? All rows will be permanently removed.';
    if(!confirm(message))return;
  }else if(action==='drop'){
    const verb=isCollection?'dropping collection':(isRedisKey?'deleting key':'dropping '+(object.kind||'object'));
    const typed=prompt('Type "'+object.name+'" to confirm '+verb+':','');
    if(typed!==object.name)return;
  }
  await database.request(root.meta.id,'object_action',{
    catalog:object.catalog||root.catalog.value||'',kind:object.kind,name:object.name,action
  });
  const matchingPages=Array.from(root.workbench.pages.values()).filter(page=>page.object&&objectKey(page.object)===objectKey(object));
  if(action==='drop'){
    for(const page of matchingPages)closeWorkbenchPage(root,page.key,{force:true});
    root.refresh.click();
    return;
  }
  for(const page of matchingPages){
    if(page.mode==='data'&&page.ctx)await loadData(page.ctx,{resetOffset:true});
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

async function loadTotalCount(view){
  const state=dataState(view);const object=state.object;
  if(!object)throw new Error('Select a database object first');
  if(!supports(view,'object_actions'))throw new Error('This adapter does not support object counts');
  const result=await database.request(view.meta.id,'object_action',{
    catalog:object.catalog||view.catalog.value||'',kind:object.kind,name:object.name,action:'count_rows'
  });
  const count=Number(result?.count);
  if(!Number.isFinite(count)||count<0)throw new Error('Database adapter returned an invalid total count');
  if(!state.result)state.result={columns:[],rows:[],offset:state.offset,limit:state.limit,has_more:false,editable:false};
  state.result.total_rows=count;
  renderDataGrid(view);
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
  const count=document.createElement('button');count.type='button';count.textContent='Count';count.title='Load total row/document/entry count';
  const filter=document.createElement('button');filter.type='button';filter.textContent='Filter…';filter.title='Server-side row filter';
  const add=document.createElement('button');add.type='button';add.textContent='+ Row';add.title='Insert row';
  const apply=document.createElement('button');apply.type='button';apply.className='db-apply';apply.textContent='Apply changes';
  const revert=document.createElement('button');revert.type='button';revert.textContent='Revert';
  const spacer=document.createElement('span');spacer.className='db-data-spacer';
  const status=document.createElement('span');status.className='db-data-status';status.textContent='Select a table';
  tools.append(objectTitle,first,prev,next,pageSize,refresh,count,filter,add,apply,revert,spacer,status);
  const grid=document.createElement('div');grid.className='db-data-grid-wrap';
  const empty=document.createElement('div');empty.className='db-data-empty';empty.textContent='Double-click a table or choose View Data from its context menu.';grid.append(empty);
  panel.append(tools,grid);
  view.workbench.controls={objectTitle,first,prev,next,pageSize,refresh,count,filter,add,apply,revert,status};
  view.workbench.grid=grid;
  first.onclick=()=>{if(!confirmDiscardChanges(view))return;const state=dataState(view);state.offset=0;loadData(view).catch(app.showError);};
  prev.onclick=()=>{if(!confirmDiscardChanges(view))return;const state=dataState(view);state.offset=Math.max(0,state.offset-state.limit);loadData(view).catch(app.showError);};
  next.onclick=()=>{if(!confirmDiscardChanges(view))return;const state=dataState(view);state.offset+=state.limit;loadData(view).catch(app.showError);};
  pageSize.onchange=()=>{if(!confirmDiscardChanges(view)){pageSize.value=String(dataState(view).limit);return;}const state=dataState(view);state.limit=Math.max(1,Math.min(1000,Number(pageSize.value)||100));state.offset=0;if(state.object)storePageSize(view,state.object,state.limit);loadData(view).catch(app.showError);};
  refresh.onclick=()=>{if(!confirmDiscardChanges(view))return;loadData(view).catch(app.showError);};
  count.disabled=!supports(view,'object_actions');count.onclick=()=>loadTotalCount(view).catch(app.showError);
  filter.disabled=!filterCapable(view);filter.onclick=()=>openFilterDialog(view);
  ensureAdapters().then(()=>refreshWorkbenchCapabilities(view)).catch(error=>console.warn('Database Workbench capabilities unavailable',error));
  add.onclick=()=>addGridRow(view);
  apply.onclick=()=>applyGridChanges(view).catch(app.showError);
  revert.onclick=()=>{clearPendingChanges(view);renderDataGrid(view);};
  return panel;
}

function paneKeyboardShortcuts(view){
  const root=rootWorkbenchView(view);
  root.pane.addEventListener('keydown',event=>{
    const page=root.workbench?.pages?.get(root.workbench.active);
    if(page?.mode!=='data'||!page.ctx)return;
    const ctx=page.ctx;
    if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='s'){
      event.preventDefault();
      if(pendingChangeCount(ctx)>0)applyGridChanges(ctx).catch(app.showError);
      return;
    }
    if(event.altKey&&(event.key==='Insert'||event.key.toLowerCase()==='n')){
      event.preventDefault();addGridRow(ctx);
    }
  });
}

function enhanceView(view){
  if(!view||view.workbench)return;
  const query=view.editor?.closest('.db-query');const body=query?.parentElement;
  if(!query||!body)return;
  const main=document.createElement('div');main.className='db-main';
  body.insertBefore(main,query);main.append(query);
  query.classList.add('db-workbench-panel');

  const tabsBar=document.createElement('div');tabsBar.className='db-workbench-tabs';
  view.workbench={
    active:'query',
    tabsBar,
    main,
    pages:new Map(),
    details:new Map(),
    objectFilter:null
  };
  const queryTab=createWorkbenchTab(view,'query','Query',{closable:false});
  view.workbench.pages.set('query',{key:'query',mode:'query',tab:queryTab,panel:query,ctx:view});
  main.prepend(tabsBar);
  paneKeyboardShortcuts(view);
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
