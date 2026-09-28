const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Database workspace');

const dbViews=new Map();
let profilesByID=new Map();
const cmFactory=globalThis.cm6?.load?.()||null;
const QUERY_SCHEMA_CONCURRENCY=4;
const SQL_SCRIPT_EDIT_LIMIT=2<<20;

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
.db-script-dialog{position:fixed;inset:0;z-index:16000;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,.58);padding:18px}
.db-script-card{width:min(760px,96vw);max-height:90vh;display:flex;flex-direction:column;background:#171a20;border:1px solid #48515f;border-radius:10px;box-shadow:0 18px 55px rgba(0,0,0,.5);overflow:hidden}
.db-script-head{display:flex;align-items:center;gap:8px;padding:11px 13px;border-bottom:1px solid #30343b}.db-script-head strong{flex:1}
.db-script-body{padding:12px;min-height:0;overflow:auto}
.db-script-actions{display:flex;justify-content:flex-end;gap:8px;padding:10px 12px;border-top:1px solid #30343b}
.db-script-location{display:grid;grid-template-columns:1fr 1fr;gap:10px}.db-script-location button{padding:18px 12px;text-align:left}
.db-script-browser-path{font:11px ui-monospace,monospace;opacity:.7;margin-bottom:8px}
.db-script-browser-list{border:1px solid #30343b;border-radius:6px;min-height:260px;max-height:55vh;overflow:auto}
.db-script-browser-row{display:flex;width:100%;align-items:center;gap:8px;border:0;border-radius:0;background:transparent;text-align:left;padding:7px 9px}.db-script-browser-row:hover,.db-script-browser-row.selected{background:#27313d}
.db-script-browser-name{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.db-script-browser-size{font-size:10px;opacity:.55}
.db-import-warning{padding:9px 10px;border:1px solid #7b6332;background:#302814;border-radius:6px;margin-bottom:10px}
.db-import-grid{display:grid;grid-template-columns:130px 1fr;gap:7px 10px;font-size:12px}.db-import-grid select{min-width:0}
.db-query-filter-head{display:flex;align-items:center;gap:8px;margin-bottom:10px}.db-query-filter-head label{font-size:11px;font-weight:700;opacity:.7}.db-query-filter-head select{min-width:160px}
.db-query-filter-conditions{display:flex;flex-direction:column;gap:7px}.db-query-filter-condition{display:grid;grid-template-columns:minmax(120px,1fr) minmax(110px,1fr) minmax(140px,1.4fr) auto;gap:7px;align-items:center}.db-query-filter-condition input,.db-query-filter-condition select{min-width:0;width:100%;box-sizing:border-box}.db-query-filter-condition button{white-space:nowrap}.db-query-filter-add{margin-top:9px}
html[data-taskmenu-theme="light"] .db-script-card{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .db-script-browser-row:hover,html[data-taskmenu-theme="light"] .db-script-browser-row.selected{background:#e8eef5}
.db-query-editor{min-height:130px;height:32%;resize:vertical;background:#090c10;color:inherit;border:0;border-bottom:1px solid #30343b;padding:10px;font-family:ui-monospace,monospace;font-size:13px;line-height:1.45;outline:none}
.db-query .codemirror{height:32%;min-height:130px;resize:vertical;overflow:hidden;border-bottom:1px solid #30343b;background:#090c10}
.db-query .codemirror .cm-editor{height:100%;font-size:13px}
.db-query .codemirror .cm-scroller{overflow:auto;font-family:ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace}
.db-result-wrap{flex:1;min-height:0;overflow:auto}
.db-query-result-tabs{display:flex;align-items:end;gap:2px;padding:4px 6px 0;border-bottom:1px solid #30343b;background:#11151b;overflow-x:auto;position:sticky;top:0;z-index:5}
.db-query-result-tab{border-radius:5px 5px 0 0;border-bottom:0;padding:5px 8px;font-size:10px;opacity:.68;white-space:nowrap}
.db-query-result-tab.active{background:#202630;opacity:1}
.db-query-result-panels{min-height:0}
.db-query-result-panel.hidden{display:none}
.db-query-result-panel .db-result-status{top:30px}
.db-query-result-panel .db-result-table th{top:57px}
.db-query-result-error{padding:12px;white-space:pre-wrap;font:11px ui-monospace,monospace;color:#ff9a9a}
html[data-taskmenu-theme="light"] .db-query-result-tabs{background:#f2f5f8;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .db-query-result-tab.active{background:#fff}
.db-result-status{position:sticky;top:0;z-index:2;padding:5px 8px;background:#11151b;border-bottom:1px solid #30343b;font-size:11px;opacity:.8}
.db-result-table{border-collapse:collapse;min-width:100%;font-family:ui-monospace,monospace;font-size:11px}
.db-result-table th,.db-result-table td{border-right:1px solid #272d36;border-bottom:1px solid #272d36;padding:5px 7px;text-align:left;vertical-align:top;white-space:pre-wrap;max-width:520px}
.db-result-table th{position:sticky;top:27px;background:#171c23;z-index:1}
.db-null{opacity:.45;font-style:italic}
.db-long-text-cell{white-space:nowrap!important}
.db-long-text-preview{display:flex;align-items:center;gap:5px;min-width:0;max-width:520px}
.db-long-text-preview-text{display:block;min-width:0;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.db-long-text-open{flex:0 0 auto;padding:0 6px;min-width:26px;height:22px;line-height:18px}
.db-value-dialog-card{width:min(900px,96vw)}
.db-value-dialog-area{width:100%;min-height:360px;max-height:70vh;resize:vertical;font-family:ui-monospace,monospace;font-size:12px}
.db-result-edit-tools{display:flex;align-items:center;gap:6px;padding:5px 8px;border-bottom:1px solid #30343b;background:#11151b}
.db-result-edit-tools button{font-size:11px}
.db-result-edit-tools .db-result-apply{background:#244c70;border-color:#3f79a8}
.db-result-edit-info{font-size:10px;opacity:.65;flex:1;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.db-result-table td.db-query-editable{cursor:text;outline:none}
.db-result-table td.db-query-editable:focus{box-shadow:inset 0 0 0 1px #3f79a8;background:#121a23}
.db-result-table td.db-query-popup-editable{cursor:default}
.db-result-table td.db-query-dirty{background:#332d18}
.db-result-table th.db-row-number,.db-result-table td.db-row-number{position:sticky;left:0;z-index:3;width:42px;min-width:42px;text-align:right;opacity:.55;background:#11151b}
.db-result-table th.db-row-number{z-index:4;cursor:pointer;user-select:none}
.db-result-table td.db-row-number{cursor:default;user-select:none}
.db-result-table tr.db-selected td{background:#19334d}
.db-result-table tr.db-selected td.db-query-dirty{background:#3c3920}
.db-result-table tr.db-query-new-row td{background:#14261d}
html[data-taskmenu-theme="light"] .db-pane{background:#fff}
html[data-taskmenu-theme="light"] .db-query-editor,html[data-taskmenu-theme="light"] .db-query .codemirror{background:#f7f9fb}
html[data-taskmenu-theme="light"] .db-result-status{background:#f2f5f8}
html[data-taskmenu-theme="light"] .db-result-table th{background:#e9eef3}
`;
document.head.append(style);

function profileFor(id){
  return profilesByID.get(String(id||''))||null;
}

function formatDBFileSize(value){
  const size=Math.max(0,Number(value)||0);
  if(size<1024)return size+' B';
  if(size<1024*1024)return (size/1024).toFixed(size<10*1024?1:0)+' KiB';
  if(size<1024*1024*1024)return (size/(1024*1024)).toFixed(size<10*1024*1024?1:0)+' MiB';
  return (size/(1024*1024*1024)).toFixed(1)+' GiB';
}

function sqlScriptNameAllowed(name){
  return /\.(sql|txt)$/i.test(String(name||'').trim());
}

function joinProjectPath(parent,name){
  const base=String(parent||'').replace(/^\/+|\/+$/g,'');
  return base?base+'/'+name:name;
}

function parentProjectPath(path){
  const parts=String(path||'').split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

function createDBDialog(titleText){
  const overlay=document.createElement('div');overlay.className='db-script-dialog';
  const card=document.createElement('div');card.className='db-script-card';
  const head=document.createElement('div');head.className='db-script-head';
  const title=document.createElement('strong');title.textContent=titleText;
  const close=document.createElement('button');close.type='button';close.textContent='×';
  const body=document.createElement('div');body.className='db-script-body';
  const actions=document.createElement('div');actions.className='db-script-actions';
  head.append(title,close);card.append(head,body,actions);overlay.append(card);document.body.append(overlay);
  const remove=()=>overlay.remove();close.onclick=remove;
  overlay.onpointerdown=event=>{if(event.target===overlay)remove();};
  overlay.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();remove();}});
  return {overlay,card,body,actions,remove};
}

function chooseScriptLocation(titleText){
  return new Promise(resolve=>{
    const dialog=createDBDialog(titleText);
    const grid=document.createElement('div');grid.className='db-script-location';
    const locationButton=(label,description)=>{
      const button=document.createElement('button');button.type='button';
      const strong=document.createElement('strong');strong.textContent=label;
      const small=document.createElement('small');small.textContent=description;
      button.append(strong,document.createElement('br'),small);return button;
    };
    const host=locationButton('Host','Workspace on the TaskDeck server');
    const client=locationButton('Client','This browser / local computer');
    host.onclick=()=>{dialog.remove();resolve('host');};
    client.onclick=()=>{dialog.remove();resolve('client');};
    dialog.body.append(grid);grid.append(host,client);
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=()=>{dialog.remove();resolve(null);};
    dialog.actions.append(cancel);
  });
}

function chooseHostSQLScript(){
  return new Promise(resolve=>{
    const dialog=createDBDialog('Open SQL script from host');
    const pathLine=document.createElement('div');pathLine.className='db-script-browser-path';
    const list=document.createElement('div');list.className='db-script-browser-list';
    dialog.body.append(pathLine,list);
    let current='',selected=null;
    const open=document.createElement('button');open.type='button';open.textContent='Open';open.disabled=true;
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
    cancel.onclick=()=>{dialog.remove();resolve(null);};
    open.onclick=()=>{if(selected){dialog.remove();resolve(selected);}};
    dialog.actions.append(cancel,open);
    const load=async pathValue=>{
      current=pathValue||'';selected=null;open.disabled=true;pathLine.textContent=current||'. (workspace root)';
      list.replaceChildren();
      if(current){
        const up=document.createElement('button');up.type='button';up.className='db-script-browser-row';
        up.textContent='↰  ..';up.onclick=()=>load(parentProjectPath(current)).catch(app.showError);list.append(up);
      }
      const items=await app.jsonFetch('/api/project/tree?path='+encodeURIComponent(current));
      for(const item of Array.isArray(items)?items:[]){
        if(item?.type!=='dir'&&(item?.type!=='file'||!sqlScriptNameAllowed(item.name)))continue;
        const full=joinProjectPath(current,item.name);
        const row=document.createElement('button');row.type='button';row.className='db-script-browser-row';
        const name=document.createElement('span');name.className='db-script-browser-name';name.textContent=(item.type==='dir'?'📁 ':'')+item.name;
        const size=document.createElement('span');size.className='db-script-browser-size';size.textContent=item.type==='file'?formatDBFileSize(item.size):'';
        row.append(name,size);
        if(item.type==='dir')row.onclick=()=>load(full).catch(app.showError);
        else{
          row.onclick=()=>{selected={kind:'host',path:full,name:item.name,size:Number(item.size)||0};for(const el of list.querySelectorAll('.selected'))el.classList.remove('selected');row.classList.add('selected');open.disabled=false;};
          row.ondblclick=()=>{dialog.remove();resolve({kind:'host',path:full,name:item.name,size:Number(item.size)||0});};
        }
        list.append(row);
      }
    };
    load('').catch(error=>{dialog.remove();app.showError(error);resolve(null);});
  });
}

async function chooseClientSQLScript(){
  if(typeof globalThis.showOpenFilePicker==='function'){
    try{
      const handles=await globalThis.showOpenFilePicker({
        multiple:false,
        types:[{description:'SQL script',accept:{'text/plain':['.sql','.txt']}}]
      });
      const handle=handles?.[0];if(!handle)return null;
      const file=await handle.getFile();
      return {kind:'client',file,handle,name:file.name,size:file.size};
    }catch(error){
      if(error?.name==='AbortError')return null;
      throw error;
    }
  }
  return new Promise(resolve=>{
    const input=document.createElement('input');input.type='file';input.accept='.sql,.txt,text/plain';input.hidden=true;document.body.append(input);
    input.onchange=()=>{const file=input.files?.[0]||null;input.remove();resolve(file?{kind:'client',file,handle:null,name:file.name,size:file.size}:null);};
    input.addEventListener('cancel',()=>{input.remove();resolve(null);},{once:true});
    input.click();
  });
}

async function importSQLSource(view,source){
  const dialog=createDBDialog('Import SQL script');
  const warning=document.createElement('div');warning.className='db-import-warning';
  warning.textContent=source.size>SQL_SCRIPT_EDIT_LIMIT
    ?'This file is too large to open safely in the query editor. TaskDeck refused to load it and switched to streamed SQL import.'
    :'Import executes the SQL script against the selected database. Review the source before continuing.';
  const grid=document.createElement('div');grid.className='db-import-grid';
  const add=(label,value)=>{const key=document.createElement('div');key.textContent=label;const val=document.createElement('div');if(value instanceof Node)val.append(value);else val.textContent=String(value);grid.append(key,val);};
  add('Source',source.kind==='host'?'Host workspace':'Client computer');
  add('File',source.name||source.path||'SQL script');
  add('Size',formatDBFileSize(source.size));
  const catalog=document.createElement('select');
  for(const option of view.catalog.options){const item=document.createElement('option');item.value=option.value;item.textContent=option.textContent;catalog.append(item);}
  catalog.value=view.catalog.value||catalog.value;add('Database',catalog);
  const status=document.createElement('div');status.style.marginTop='10px';status.style.fontSize='11px';status.style.opacity='.75';
  dialog.body.append(warning,grid,status);
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=dialog.remove;
  const run=document.createElement('button');run.type='button';run.className='task-connection-primary';run.textContent='Import';
  run.onclick=async()=>{
    if(!confirm('Import this SQL script into '+(catalog.value||'the selected database')+'?'))return;
    run.disabled=true;cancel.disabled=true;status.textContent='Importing…';
    try{
      const url='/api/db/sessions/'+encodeURIComponent(view.meta.id)+'/import';
      let response;
      if(source.kind==='client'){
        const form=new FormData();form.append('catalog',catalog.value||'');form.append('file',source.file,source.name);
        response=await app.fetchWithLease(url,{method:'POST',body:form,cache:'no-store'});
      }else{
        response=await app.fetchWithLease(url,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({host_path:source.path,catalog:catalog.value||''}),cache:'no-store'});
      }
      if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
      const payload=await response.json();const result=payload?.result||{};
      status.textContent=(result.message||'SQL import completed')+(Number.isFinite(result.imported_bytes)?' · '+formatDBFileSize(result.imported_bytes):'');
      run.textContent='Done';cancel.textContent='Close';cancel.disabled=false;run.disabled=true;
      view.querySchemaCache?.clear?.();loadObjects(view).catch(app.showError);
    }catch(error){status.textContent='Import failed';run.disabled=false;cancel.disabled=false;app.showError(error);}
  };
  dialog.actions.append(cancel,run);
}

function queryScriptTooltip(source){
  if(!source)return '';
  if(source.kind==='host')return String(source.path||source.name||'');
  if(source.kind==='client')return 'Client file: '+String(source.name||'');
  return String(source.name||'');
}

function setQueryScriptIdentity(view,source){
  view.scriptSource=source||null;
  if(source?.name)view.scriptName=source.name;
  if(source?.name)globalThis.TaskMenuDatabaseWorkbench?.updateQueryTabIdentity?.(view,{
    label:source.name,
    tooltip:queryScriptTooltip(source)
  });
}

async function saveSQLScriptToExistingSource(view){
  const source=view.scriptSource;
  if(source?.kind==='host'&&source.path){
    const dir=parentProjectPath(source.path)||'.';
    const name=source.name||String(source.path).split('/').pop()||'query.sql';
    const result=await uploadSQLTextToHost(name,queryEditorText(view),dir,true);
    if(result){setQueryScriptIdentity(view,{...source,name,path:source.path});return true;}
    return false;
  }
  if(source?.kind==='client'&&source.handle?.createWritable){
    const writable=await source.handle.createWritable();
    await writable.write(queryEditorText(view));await writable.close();
    setQueryScriptIdentity(view,{...source,name:source.handle.name||source.name});
    return true;
  }
  return false;
}

async function openSQLScript(view){
  const location=await chooseScriptLocation('Open SQL script');
  if(!location)return;
  const source=location==='host'?await chooseHostSQLScript():await chooseClientSQLScript();
  if(!source)return;
  if(!sqlScriptNameAllowed(source.name||source.path))throw new Error('Choose a .sql or .txt file');
  if(source.size>SQL_SCRIPT_EDIT_LIMIT){await importSQLSource(view,source);return;}
  let text;
  if(source.kind==='client'){
    text=await source.file.text();
  }else{
    const data=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(source.path));
    if(Number(data?.size)>SQL_SCRIPT_EDIT_LIMIT){source.size=Number(data.size)||source.size;await importSQLSource(view,source);return;}
    text=String(data?.content??'');
  }
  if(text.includes('\x00'))throw new Error('SQL script contains NUL bytes and cannot be opened');
  setQueryEditorText(view,text);
  setQueryScriptIdentity(view,{
    kind:source.kind,
    name:source.name||'query.sql',
    ...(source.kind==='host'?{path:source.path}:{handle:source.handle||null})
  });
}


async function uploadTextToHost(name,text,dir,mime='text/plain;charset=utf-8',overwrite=false){
  const form=new FormData();
  form.append('dir',dir||'.');
  form.append('file',new Blob([text],{type:mime}),name);
  const response=await app.fetchWithLease('/api/files/upload'+(overwrite?'?overwrite=1':''),{method:'POST',body:form,cache:'no-store'});
  if(response.status===409&&!overwrite){
    const message=(await response.text()).trim();
    if(confirm((message||name+' already exists')+'\n\nOverwrite it?'))return uploadTextToHost(name,text,dir,mime,true);
    return null;
  }
  if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
  return response.json();
}

function uploadSQLTextToHost(name,text,dir,overwrite=false){
  return uploadTextToHost(name,text,dir,'text/sql;charset=utf-8',overwrite);
}

async function saveSQLScriptToHost(view){
  const browser=globalThis.TaskMenuDirectoryBrowser;
  if(typeof browser?.chooseFile!=='function')throw new Error('Host file browser is unavailable');
  let suggestedName=String(view.scriptName||'query.sql').trim()||'query.sql';
  if(!sqlScriptNameAllowed(suggestedName))suggestedName+='.sql';
  const target=await browser.chooseFile({
    title:'Save SQL script on host',
    label:'Destination directory relative to the workspace:',
    fileLabel:'SQL script file name:',
    fileName:suggestedName,
    confirm:'Save SQL',
    initial:'.'
  });
  if(target===null)return;
  let name=String(target.name||'').trim();
  if(!sqlScriptNameAllowed(name))name+='.sql';
  const dir=String(target.dir||'.').trim()||'.';
  const result=await uploadSQLTextToHost(name,queryEditorText(view),dir);
  if(result){
    const relativeDir=dir==='.'?'':dir;
    setQueryScriptIdentity(view,{kind:'host',name,path:joinProjectPath(relativeDir,name)});
  }
}

async function saveTextToClient(name,text,{description='Text file',mime='text/plain;charset=utf-8',extensions=['.txt']}={}){
  if(typeof globalThis.showSaveFilePicker==='function'){
    try{
      const acceptMime=mime.split(';')[0]||'text/plain';
      const handle=await globalThis.showSaveFilePicker({
        suggestedName:name,
        types:[{description,accept:{[acceptMime]:extensions}}]
      });
      const writable=await handle.createWritable();
      await writable.write(text);await writable.close();
      return handle.name||name;
    }catch(error){
      if(error?.name==='AbortError')return null;
      throw error;
    }
  }
  const blob=new Blob([text],{type:mime});
  const url=URL.createObjectURL(blob);
  try{
    const link=document.createElement('a');link.href=url;link.download=name;link.rel='noopener';document.body.append(link);link.click();link.remove();
  }finally{setTimeout(()=>URL.revokeObjectURL(url),0);}
  return name;
}

async function saveSQLScriptToClient(view){
  let name=String(view.scriptName||'query.sql').trim()||'query.sql';
  if(!sqlScriptNameAllowed(name))name+='.sql';
  if(typeof globalThis.showSaveFilePicker==='function'){
    try{
      const handle=await globalThis.showSaveFilePicker({
        suggestedName:name,
        types:[{description:'SQL script',accept:{'text/plain':['.sql','.txt']}}]
      });
      const writable=await handle.createWritable();
      await writable.write(queryEditorText(view));await writable.close();
      setQueryScriptIdentity(view,{kind:'client',name:handle.name||name,handle});
      return;
    }catch(error){
      if(error?.name==='AbortError')return;
      throw error;
    }
  }
  const saved=await saveTextToClient(name,queryEditorText(view),{
    description:'SQL script',
    mime:'text/sql;charset=utf-8',
    extensions:['.sql','.txt']
  });
  if(saved){
    view.scriptName=saved;
    view.scriptSource=null;
    globalThis.TaskMenuDatabaseWorkbench?.updateQueryTabIdentity?.(view,{label:saved,tooltip:'Client file: '+saved});
  }
}

async function saveTextWithLocation(titleText,suggestedName,text,{description='Text file',mime='text/plain;charset=utf-8',extensions=['.txt'],hostFileLabel='File name:'}={}){
  const location=await chooseScriptLocation(titleText);
  if(!location)return null;
  if(location==='host'){
    const browser=globalThis.TaskMenuDirectoryBrowser;
    if(typeof browser?.chooseFile!=='function')throw new Error('Host file browser is unavailable');
    const target=await browser.chooseFile({
      title:titleText+' on host',
      label:'Destination directory relative to the workspace:',
      fileLabel:hostFileLabel,
      fileName:suggestedName,
      confirm:'Save here',
      initial:'.'
    });
    if(target===null)return null;
    const name=String(target.name||suggestedName).trim()||suggestedName;
    await uploadTextToHost(name,text,String(target.dir||'.').trim()||'.',mime);
    return {location:'host',name};
  }
  const name=await saveTextToClient(suggestedName,text,{description,mime,extensions});
  return name?{location:'client',name}:null;
}

async function saveSQLScript(view){
  if(await saveSQLScriptToExistingSource(view))return;
  const location=await chooseScriptLocation('Save SQL script');
  if(location==='host')return saveSQLScriptToHost(view);
  if(location==='client')return saveSQLScriptToClient(view);
}

function relationalQueryEditor(view){
  return view?.meta?.adapter_kind==='mysql'||view?.meta?.adapter_kind==='sqlite';
}

function queryEditorText(view){
  return view?.queryCM?.state?.doc?.toString?.()??view?.editor?.value??'';
}

function queryEditorExecutionText(view){
  const full=queryEditorText(view);
  if(view?.queryCM?.state){
    const range=view.queryCM.state.selection?.main;
    if(range){
      const from=Math.max(0,Math.min(range.from??range.anchor??0,full.length));
      const to=Math.max(from,Math.min(range.to??range.head??from,full.length));
      if(to>from&&!(from===0&&to===full.length))return full.slice(from,to);
    }
    return full;
  }
  const editor=view?.editor;
  if(editor&&Number.isInteger(editor.selectionStart)&&Number.isInteger(editor.selectionEnd)){
    const from=Math.max(0,Math.min(editor.selectionStart,full.length));
    const to=Math.max(from,Math.min(editor.selectionEnd,full.length));
    if(to>from&&!(from===0&&to===full.length))return full.slice(from,to);
  }
  return full;
}

function focusQueryEditor(view){
  if(view?.queryCM){view.queryCM.focus();return;}
  view?.editor?.focus?.();
}

function setQueryEditorText(view,text,{focus=true}={}){
  const value=String(text??'');
  if(view?.queryCM){
    const length=view.queryCM.state.doc.length;
    view.queryCM.dispatch({changes:{from:0,to:length,insert:value},selection:{anchor:value.length}});
  }
  if(view?.editor)view.editor.value=value;
  if(focus)focusQueryEditor(view);
}

function queryCompletionSchema(view){
  const catalog=String(view?.catalog?.value||'').trim();
  const objects=(view?.objectData||[]).filter(object=>object?.kind==='table'||object?.kind==='view');
  const tables=objects.map(object=>String(object?.name||'').trim()).filter(Boolean);
  const tableSchema={};
  for(const name of tables)tableSchema[name]=view.querySchemaCache?.get(catalog+'\u0000'+name)||[];
  const tableByLower=new Map(tables.map(name=>[name.toLowerCase(),name]));
  const referenced=queryReferencedObjectNames(queryEditorText(view))
    .map(name=>tableByLower.get(name.toLowerCase()))
    .filter(Boolean);
  const uniqueReferenced=[...new Set(referenced)];
  const defaultTable=uniqueReferenced.length===1?uniqueReferenced[0]:'';
  return catalog
    ?{schema:{[catalog]:tableSchema},tables,schemas:[catalog],defaultSchema:catalog,...(defaultTable?{defaultTable}:{})}
    :{schema:tableSchema,tables,...(defaultTable?{defaultTable}:{})};
}

function queryEditorExtensions(view){
  if(!relationalQueryEditor(view)||!globalThis.cm6?.sqlCompletion)return [];
  const dialect=view.meta.adapter_kind==='mysql'?'mysql':'sqlite';
  const schema=queryCompletionSchema(view);
  const extensions=[globalThis.cm6.sqlCompletion({dialect,...schema,upperCaseKeywords:true})];
  if(globalThis.cm6?.EditorView?.updateListener){
    extensions.push(globalThis.cm6.EditorView.updateListener.of(update=>{
      if(update.docChanged){
        const text=update.state.doc.toString();
        if(view.editor)view.editor.value=text;
        scheduleQuerySchemaReferences(view,text);
      }
    }));
  }
  return extensions;
}

function reconfigureQueryEditor(view){
  if(!view?.queryCM||!cmFactory)return;
  const text=queryEditorText(view);
  const cursor=Math.min(view.queryCM.state.selection?.main?.head??text.length,text.length);
  const state=cmFactory.newState(text,{
    dark:document.documentElement.dataset.taskmenuTheme!=='light',
    lineWrapping:false,
    extraExtensions:queryEditorExtensions(view)
  });
  view.queryCM.setState(state);
  view.queryCM.dispatch({selection:{anchor:cursor}});
}

function initQueryEditor(view){
  if(!relationalQueryEditor(view)||!cmFactory||!globalThis.cm6?.sqlCompletion)return;
  if(!(view.querySchemaCache instanceof Map))view.querySchemaCache=new Map();
  const initial=view.editor.value;
  view.queryCM=cmFactory.textarea(view.editor,{
    dark:document.documentElement.dataset.taskmenuTheme!=='light',
    lineWrapping:false,
    extraExtensions:queryEditorExtensions(view)
  });
  view.queryCM.contentDOM.addEventListener('keydown',event=>{
    if((event.ctrlKey||event.metaKey)&&event.key==='Enter'){
      event.preventDefault();executeQuery(view).catch(app.showError);
    }
  });
  view.editor.value=initial;
}

function queryReferencedObjectNames(statement){
  const names=[];const seen=new Set();
  const identifier='(?:`[^`]+`|"[^"]+"|[A-Za-z_][A-Za-z0-9_$]*)';
  const pattern=new RegExp('\\b(?:from|join|update|into)\\s+('+identifier+'(?:\\s*\\.\\s*'+identifier+')?)','ig');
  let match;
  while((match=pattern.exec(String(statement||'')))){
    const parts=match[1].split('.').map(part=>part.trim().replace(/^`|`$/g,'').replace(/^"|"$/g,''));
    const name=parts[parts.length-1];
    const key=name.toLowerCase();
    if(name&&!seen.has(key)){seen.add(key);names.push(name);}
  }
  return names;
}

function scheduleQuerySchemaReferences(view,text=queryEditorText(view)){
  if(!relationalQueryEditor(view))return;
  clearTimeout(view.querySchemaTimer);
  view.querySchemaTimer=setTimeout(()=>loadQuerySchemaReferences(view,text).catch(error=>console.warn('Query autocomplete column metadata load failed',error)),180);
}

async function loadQuerySchemaReferences(view,text){
  if(!relationalQueryEditor(view))return;
  if(!(view.querySchemaCache instanceof Map))view.querySchemaCache=new Map();
  const catalog=String(view.catalog.value||'').trim();
  const objectByName=new Map((view.objectData||[])
    .filter(object=>object?.kind==='table'||object?.kind==='view')
    .map(object=>[String(object?.name||'').toLowerCase(),object]));
  const pending=queryReferencedObjectNames(text)
    .map(name=>objectByName.get(name.toLowerCase()))
    .filter(Boolean)
    .filter(object=>!view.querySchemaCache.has(catalog+'\u0000'+object.name));
  if(!pending.length)return;
  const generation=view.querySchemaGeneration||0;let next=0;let changed=false;
  const worker=async()=>{
    while(next<pending.length){
      const object=pending[next++];
      if(view.querySchemaGeneration!==generation||String(view.catalog.value||'').trim()!==catalog)return;
      const key=catalog+'\u0000'+object.name;
      try{
        const payload={name:object.name,kind:object.kind||'table'};if(catalog)payload.catalog=catalog;
        const detail=await sessionRequest(view.meta.id,'describe_object',payload);
        const columns=Array.isArray(detail?.columns)?detail.columns.map(column=>String(column?.name||'').trim()).filter(Boolean):[];
        view.querySchemaCache.set(key,columns);changed=true;
      }catch(error){
        console.warn('Query autocomplete metadata unavailable for '+object.name,error);
        view.querySchemaCache.set(key,[]);
      }
    }
  };
  await Promise.all(Array.from({length:Math.min(QUERY_SCHEMA_CONCURRENCY,Math.max(1,pending.length))},()=>worker()));
  if(changed&&view.querySchemaGeneration===generation)reconfigureQueryEditor(view);
}

function databaseQueryViews(view){
  const root=view.workbenchRoot||view;
  const pages=root.workbench?.pages;
  if(!(pages instanceof Map))return [root];
  const queries=[];
  for(const page of pages.values()){
    if(page?.mode==='query'&&page.ctx&&!queries.includes(page.ctx))queries.push(page.ctx);
  }
  return queries;
}

async function warmQuerySchema(view){
  for(const queryView of databaseQueryViews(view)){
    if(!relationalQueryEditor(queryView))continue;
    queryView.querySchemaGeneration=(queryView.querySchemaGeneration||0)+1;
    reconfigureQueryEditor(queryView);
    scheduleQuerySchemaReferences(queryView);
  }
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

const LONG_TEXT_PREVIEW_LIMIT=160;

function isLongTextValue(value){
  if(typeof value!=='string')return false;
  return value.length>LONG_TEXT_PREVIEW_LIMIT||value.includes('\n')||value.includes('\r');
}

function isLongTextColumnType(columnType=''){
  const type=String(columnType||'').trim().toLowerCase();
  return type==='document'||type==='json'||type.includes('json')||type.includes('text')||type.includes('clob');
}

function shouldUseQueryValuePopup(value,columnType=''){
  return isLongTextColumnType(columnType)||isLongTextValue(value);
}

async function copySelectedTextArea(area){
  area.focus();area.select();
  try{area.setSelectionRange(0,area.value.length);}catch{}
  if(navigator.clipboard?.writeText){
    await navigator.clipboard.writeText(area.value);
    return;
  }
  if(!document.execCommand('copy'))throw new Error('Clipboard copy failed');
}

function parseQueryEditedValue(text,original,columnType=''){
  const type=String(columnType||'').toLowerCase();
  if(type==='document'||type==='json'||type.includes('json'))return JSON.stringify(JSON.parse(text));
  if(original&&typeof original==='object')return JSON.parse(text);
  return String(text);
}

function sameQueryValue(a,b){
  return JSON.stringify(a)===JSON.stringify(b);
}

function queryCellValue(view,result,rowIndex,columnIndex){
  const changes=view.queryDirtyRows?.get(rowIndex);
  const column=result?.edit?.columns?.[columnIndex]||result?.columns?.[columnIndex];
  if(changes?.has(column?.name))return changes.get(column.name);
  return result?.rows?.[rowIndex]?.[columnIndex];
}

function setQueryDirtyCell(view,result,rowIndex,columnIndex,value){
  const column=result?.edit?.columns?.[columnIndex]||result?.columns?.[columnIndex];
  if(!column)return;
  const original=result?.rows?.[rowIndex]?.[columnIndex];
  let changes=view.queryDirtyRows?.get(rowIndex);
  if(sameQueryValue(value,original)){
    if(changes){changes.delete(column.name);if(changes.size===0)view.queryDirtyRows.delete(rowIndex);}
  }else{
    if(!(view.queryDirtyRows instanceof Map))view.queryDirtyRows=new Map();
    if(!changes){changes=new Map();view.queryDirtyRows.set(rowIndex,changes);}
    changes.set(column.name,value);
  }
}

function queryPendingCount(view){
  let count=0;
  for(const changes of view.queryDirtyRows?.values?.()||[])count+=changes.size;
  count+=(view.queryNewRows?.length||0);
  return count;
}

function setQueryNewCell(view,result,newIndex,columnIndex,value){
  if(!Array.isArray(view.queryNewRows)||!view.queryNewRows[newIndex])return;
  const column=result?.edit?.columns?.[columnIndex]||result?.columns?.[columnIndex];
  if(!column?.name)return;
  view.queryNewRows[newIndex][column.name]=value;
}

function addQueryRow(view,result){
  if(!result?.edit?.editable)throw new Error(result?.edit?.editability_reason||'This query result is read-only');
  if(!Array.isArray(view.queryNewRows))view.queryNewRows=[];
  view.queryNewRows.push({});
  renderResult(view,result,view.queryElapsed,{preserveDirty:true});
  requestAnimationFrame(()=>{
    const row=view.result.querySelector('tr.db-query-new-row:last-child');
    const editable=row?.querySelector('[contenteditable="true"]');
    if(editable){editable.focus();return;}
    row?.querySelector('.db-long-text-open')?.focus();
  });
}

function queryCompareValues(a,b){
  if(a===b)return 0;
  if(a===null||a===undefined)return 1;
  if(b===null||b===undefined)return -1;
  if(typeof a==='number'){
    const numeric=typeof b==='number'?b:Number(String(b).trim());
    if(Number.isFinite(numeric))return a-numeric;
  }
  if(typeof b==='number'){
    const numeric=Number(String(a).trim());
    if(Number.isFinite(numeric))return numeric-b;
  }
  if(typeof a==='boolean'&&typeof b==='boolean')return Number(a)-Number(b);
  return String(a).localeCompare(String(b),undefined,{numeric:true,sensitivity:'base'});
}

function queryFilterMatches(value,filter){
  if(!filter)return true;
  const operator=filter.operator||'contains';
  if(operator==='is_null')return value===null||value===undefined;
  if(operator==='not_null')return value!==null&&value!==undefined;
  const left=value===null||value===undefined?'':String(value);
  const right=String(filter.value??'');
  switch(operator){
  case 'eq':return left===right;
  case 'ne':return left!==right;
  case 'starts_with':return left.toLowerCase().startsWith(right.toLowerCase());
  case 'ends_with':return left.toLowerCase().endsWith(right.toLowerCase());
  case 'gt':return queryCompareValues(value,filter.value)>0;
  case 'gte':return queryCompareValues(value,filter.value)>=0;
  case 'lt':return queryCompareValues(value,filter.value)<0;
  case 'lte':return queryCompareValues(value,filter.value)<=0;
  case 'contains':
  default:return left.toLowerCase().includes(right.toLowerCase());
  }
}

function normalizeQueryFilter(filter){
  if(!filter)return null;
  const raw=Array.isArray(filter.conditions)?filter.conditions:(filter.column?[filter]:[]);
  const conditions=raw.filter(condition=>condition&&String(condition.column||'').trim());
  if(!conditions.length)return null;
  return {logic:filter.logic==='or'?'or':'and',conditions};
}

function queryFilterMatchesRow(view,result,rowIndex,filter){
  const normalized=normalizeQueryFilter(filter);
  if(!normalized)return true;
  const columns=Array.isArray(result?.columns)?result.columns:[];
  const matches=normalized.conditions.map(condition=>{
    const columnIndex=columns.findIndex(column=>String(column?.name||'')===condition.column);
    if(columnIndex<0)return false;
    return queryFilterMatches(queryCellValue(view,result,rowIndex,columnIndex),condition);
  });
  return normalized.logic==='or'?matches.some(Boolean):matches.every(Boolean);
}

function queryDisplayRowIndexes(view,result=view.queryResult){
  const rows=Array.isArray(result?.rows)?result.rows:[];
  const columns=Array.isArray(result?.columns)?result.columns:[];
  let indexes=rows.map((_,index)=>index);
  const filter=normalizeQueryFilter(view.queryFilter);
  if(filter)indexes=indexes.filter(rowIndex=>queryFilterMatchesRow(view,result,rowIndex,filter));
  const order=view.queryOrder;
  if(order&&Number.isInteger(order.columnIndex)&&order.columnIndex>=0&&order.columnIndex<columns.length){
    const direction=order.direction==='desc'?-1:1;
    indexes=indexes.map((rowIndex,position)=>({rowIndex,position})).sort((a,b)=>{
      const compared=queryCompareValues(queryCellValue(view,result,a.rowIndex,order.columnIndex),queryCellValue(view,result,b.rowIndex,order.columnIndex));
      return compared===0?a.position-b.position:compared*direction;
    }).map(item=>item.rowIndex);
  }
  return indexes;
}

function querySelectedRowIndexes(view,result=view.queryResult){
  const rows=Array.isArray(result?.rows)?result.rows:[];
  const selected=view.querySelectedRows||new Set();
  const display=queryDisplayRowIndexes(view,result);
  const ordered=display.filter(index=>selected.has(index));
  for(const index of Array.from(selected).sort((a,b)=>a-b)){
    if(index>=0&&index<rows.length&&!ordered.includes(index))ordered.push(index);
  }
  return ordered;
}

function syncQuerySelection(view,result=view.queryResult){
  const visible=queryDisplayRowIndexes(view,result);
  const selected=view.querySelectedRows instanceof Set?view.querySelectedRows:new Set();
  view.querySelectedRows=selected;
  for(const tr of view.result.querySelectorAll('tbody tr[data-row-index]')){
    tr.classList.toggle('db-selected',selected.has(Number(tr.dataset.rowIndex)));
  }
  const selectAll=view.result.querySelector('th.db-row-number[data-select-all]');
  if(selectAll){
    const visibleSelected=visible.filter(index=>selected.has(index)).length;
    selectAll.textContent=visible.length>0&&visibleSelected===visible.length?'☑':(visibleSelected>0?'◩':'☐');
    selectAll.title=visible.length>0&&visibleSelected===visible.length?'Clear row selection':'Select all visible rows';
  }
  const status=view.result.querySelector('.db-result-status');
  if(status){
    const base=status.dataset.base||status.textContent||'';
    const selectedCount=querySelectedRowIndexes(view,result).length;
    status.textContent=base+(selectedCount?' · '+selectedCount+' selected':'');
  }
}

function selectQueryRow(view,result,rowIndex,event={}){
  const rows=Array.isArray(result?.rows)?result.rows:[];
  if(rowIndex<0||rowIndex>=rows.length)return;
  if(!(view.querySelectedRows instanceof Set))view.querySelectedRows=new Set();
  const additive=Boolean(event.ctrlKey||event.metaKey);
  if(event.shiftKey&&Number.isInteger(view.querySelectionAnchor)){
    const visible=queryDisplayRowIndexes(view,result);
    const anchorPos=visible.indexOf(view.querySelectionAnchor);const rowPos=visible.indexOf(rowIndex);
    if(anchorPos>=0&&rowPos>=0){
      const start=Math.min(anchorPos,rowPos);const end=Math.max(anchorPos,rowPos);
      if(!additive)view.querySelectedRows.clear();
      for(let index=start;index<=end;index++)view.querySelectedRows.add(visible[index]);
    }
  }else if(additive){
    if(view.querySelectedRows.has(rowIndex))view.querySelectedRows.delete(rowIndex);else view.querySelectedRows.add(rowIndex);
    view.querySelectionAnchor=rowIndex;
  }else{
    view.querySelectedRows.clear();view.querySelectedRows.add(rowIndex);view.querySelectionAnchor=rowIndex;
  }
  syncQuerySelection(view,result);
}

function toggleSelectAllQueryRows(view,result){
  const visible=queryDisplayRowIndexes(view,result);
  if(!(view.querySelectedRows instanceof Set))view.querySelectedRows=new Set();
  const allSelected=visible.length>0&&visible.every(index=>view.querySelectedRows.has(index));
  view.querySelectedRows.clear();
  if(!allSelected)for(const index of visible)view.querySelectedRows.add(index);
  view.querySelectionAnchor=visible.length?visible[0]:null;
  syncQuerySelection(view,result);
}

function openQueryFilterDialog(view,result){
  const columns=Array.isArray(result?.columns)?result.columns:[];
  if(!columns.length)return;
  const dialog=createDBDialog('Filter current query result');
  const current=normalizeQueryFilter(view.queryFilter);

  const head=document.createElement('div');head.className='db-query-filter-head';
  const logicLabel=document.createElement('label');logicLabel.textContent='Combine conditions';
  const logic=document.createElement('select');
  for(const [value,label] of [['and','Match all (AND)'],['or','Match any (OR)']]){
    const option=document.createElement('option');option.value=value;option.textContent=label;logic.append(option);
  }
  logic.value=current?.logic||'and';head.append(logicLabel,logic);

  const conditions=document.createElement('div');conditions.className='db-query-filter-conditions';
  const rows=[];
  const operators=[['contains','Contains'],['eq','Equals'],['ne','Not equal'],['starts_with','Starts with'],['ends_with','Ends with'],['gt','>'],['gte','>='],['lt','<'],['lte','<='],['is_null','Is NULL'],['not_null','Is not NULL']];

  const syncRemoveButtons=()=>{for(const row of rows)row.remove.disabled=rows.length<=1;};
  const addCondition=(condition={})=>{
    const wrap=document.createElement('div');wrap.className='db-query-filter-condition';
    const column=document.createElement('select');
    for(const item of columns){const option=document.createElement('option');option.value=item?.name||'';option.textContent=item?.name||'';column.append(option);}
    const operator=document.createElement('select');
    for(const [value,label] of operators){const option=document.createElement('option');option.value=value;option.textContent=label;operator.append(option);}
    const value=document.createElement('input');value.type='text';
    const remove=document.createElement('button');remove.type='button';remove.textContent='Remove';
    if(condition.column)column.value=condition.column;
    if(condition.operator)operator.value=condition.operator;
    value.value=String(condition.value??'');
    const syncValue=()=>{value.disabled=operator.value==='is_null'||operator.value==='not_null';};operator.onchange=syncValue;syncValue();
    const row={wrap,column,operator,value,remove};
    remove.onclick=()=>{const index=rows.indexOf(row);if(index>=0)rows.splice(index,1);wrap.remove();syncRemoveButtons();};
    rows.push(row);wrap.append(column,operator,value,remove);conditions.append(wrap);syncRemoveButtons();
  };

  for(const condition of current?.conditions||[{}])addCondition(condition);
  const add=document.createElement('button');add.type='button';add.className='db-query-filter-add';add.textContent='Add condition';add.onclick=()=>addCondition({});
  dialog.body.append(head,conditions,add);

  const clear=document.createElement('button');clear.type='button';clear.textContent='Clear filter';clear.onclick=()=>{view.queryFilter=null;view.querySelectedRows?.clear?.();dialog.remove();renderResult(view,result,view.queryElapsed,{preserveDirty:true});};
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=dialog.remove;
  const apply=document.createElement('button');apply.type='button';apply.className='task-connection-primary';apply.textContent='Apply';
  apply.onclick=()=>{
    const filterConditions=rows.map(row=>({column:row.column.value,operator:row.operator.value,value:row.value.value}));
    view.queryFilter={logic:logic.value==='or'?'or':'and',conditions:filterConditions};
    view.querySelectedRows?.clear?.();dialog.remove();renderResult(view,result,view.queryElapsed,{preserveDirty:true});
  };
  dialog.actions.append(clear,cancel,apply);
}

function openQueryOrderDialog(view,result){
  const columns=Array.isArray(result?.columns)?result.columns:[];
  if(!columns.length)return;
  const dialog=createDBDialog('Order current query result');
  const grid=document.createElement('div');grid.className='db-import-grid';
  const column=document.createElement('select');
  columns.forEach((item,index)=>{const option=document.createElement('option');option.value=String(index);option.textContent=item?.name||'';column.append(option);});
  const direction=document.createElement('select');
  for(const [value,label] of [['asc','Ascending'],['desc','Descending']]){const option=document.createElement('option');option.value=value;option.textContent=label;direction.append(option);}
  const append=(label,input)=>{const key=document.createElement('div');key.textContent=label;grid.append(key,input);};
  append('Column',column);append('Direction',direction);dialog.body.append(grid);
  if(view.queryOrder){column.value=String(view.queryOrder.columnIndex);direction.value=view.queryOrder.direction;}
  const clear=document.createElement('button');clear.type='button';clear.textContent='Clear order';clear.onclick=()=>{view.queryOrder=null;dialog.remove();renderResult(view,result,view.queryElapsed,{preserveDirty:true});};
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=dialog.remove;
  const apply=document.createElement('button');apply.type='button';apply.className='task-connection-primary';apply.textContent='Apply';
  apply.onclick=()=>{view.queryOrder={columnIndex:Number(column.value),direction:direction.value};dialog.remove();renderResult(view,result,view.queryElapsed,{preserveDirty:true});};
  dialog.actions.append(clear,cancel,apply);
}

function toggleQueryHeaderOrder(view,result,columnIndex){
  const current=view.queryOrder;
  if(!current||current.columnIndex!==columnIndex)view.queryOrder={columnIndex,direction:'asc'};
  else if(current.direction==='asc')view.queryOrder={columnIndex,direction:'desc'};
  else view.queryOrder=null;
  renderResult(view,result,view.queryElapsed,{preserveDirty:true});
}

function queryClipboardData(view,result,{selectedOnly=false}={}){
  const columns=Array.isArray(result?.columns)?result.columns.map(column=>String(column?.name||'')):[];
  const rows=Array.isArray(result?.rows)?result.rows:[];
  const indexes=selectedOnly?querySelectedRowIndexes(view,result):queryDisplayRowIndexes(view,result);
  const outputRows=indexes.map(rowIndex=>columns.map((_,columnIndex)=>queryCellValue(view,result,rowIndex,columnIndex)));
  if(!selectedOnly){
    for(const values of view.queryNewRows||[]){
      outputRows.push(columns.map(name=>Object.prototype.hasOwnProperty.call(values,name)?values[name]:null));
    }
  }
  return {columns,rows:outputRows};
}

async function copyQueryData(view,result,format,includeHeaders,scope){
  const helper=globalThis.TaskMenuDatabaseWorkbench;
  if(typeof helper?.serializeClipboardData!=='function'||typeof helper?.copyText!=='function')throw new Error('Database copy helpers are unavailable');
  const data=queryClipboardData(view,result,{selectedOnly:scope==='selected'});
  await helper.copyText(helper.serializeClipboardData(format,data.columns,data.rows,includeHeaders));
}

function queryCopyScopeMenuItems(view,result,format,includeHeaders){
  const selected=querySelectedRowIndexes(view,result).length;
  const selectedLabel=selected===1?'Selected row (1)':(selected>1?'Selected rows ('+selected+')':'Selected rows');
  return [
    {label:selectedLabel,disabled:selected===0,action:()=>copyQueryData(view,result,format,includeHeaders,'selected')},
    {label:'Current result',action:()=>copyQueryData(view,result,format,includeHeaders,'current')}
  ];
}

function queryCopyMenuItems(view,result){
  return [
    {label:'Copy TXT (without column header)',submenu:queryCopyScopeMenuItems(view,result,'txt',false)},
    {label:'Copy TXT (with column header)',submenu:queryCopyScopeMenuItems(view,result,'txt',true)},
    {label:'Copy CSV (without column header)',submenu:queryCopyScopeMenuItems(view,result,'csv',false)},
    {label:'Copy CSV (with column header)',submenu:queryCopyScopeMenuItems(view,result,'csv',true)},
    {label:'Copy as JSON',submenu:queryCopyScopeMenuItems(view,result,'json',true)}
  ];
}

function queryExportExtension(format){
  return format==='csv'?'.csv':(format==='json'?'.json':'.txt');
}

function queryExportMime(format){
  return format==='csv'?'text/csv;charset=utf-8':(format==='json'?'application/json;charset=utf-8':'text/plain;charset=utf-8');
}

async function exportQueryData(view,result){
  const helper=globalThis.TaskMenuDatabaseWorkbench;
  if(typeof helper?.serializeClipboardData!=='function')throw new Error('Database export serializer is unavailable');
  const dialog=createDBDialog('Export query result');
  const grid=document.createElement('div');grid.className='db-import-grid';
  const format=document.createElement('select');
  for(const value of ['csv','txt','json']){const option=document.createElement('option');option.value=value;option.textContent=value.toUpperCase();format.append(option);}
  const scope=document.createElement('select');
  const current=document.createElement('option');current.value='current';current.textContent='Current result';scope.append(current);
  const selected=document.createElement('option');selected.value='selected';selected.textContent='Selected rows ('+querySelectedRowIndexes(view,result).length+')';selected.disabled=querySelectedRowIndexes(view,result).length===0;scope.append(selected);
  if(!selected.disabled)scope.value='selected';
  const headers=document.createElement('input');headers.type='checkbox';headers.checked=true;
  const append=(label,input)=>{const key=document.createElement('div');key.textContent=label;const value=document.createElement('div');value.append(input);grid.append(key,value);};
  append('Format',format);append('Scope',scope);append('Column header / JSON keys',headers);
  dialog.body.append(grid);
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=dialog.remove;
  const save=document.createElement('button');save.type='button';save.className='task-connection-primary';save.textContent='Choose save location…';
  save.onclick=async()=>{
    const data=queryClipboardData(view,result,{selectedOnly:scope.value==='selected'});
    const text=helper.serializeClipboardData(format.value,data.columns,data.rows,headers.checked);
    const ext=queryExportExtension(format.value);
    const suggestedName='query-result'+ext;
    const mime=queryExportMime(format.value);
    save.disabled=true;cancel.disabled=true;
    try{
      dialog.remove();
      await saveTextWithLocation('Export query result',suggestedName,text,{
        description:'Database query export',
        mime,
        extensions:[ext],
        hostFileLabel:'Export file name:'
      });
    }finally{
      if(save.isConnected){save.disabled=false;cancel.disabled=false;}
    }
  };
  dialog.actions.append(cancel,save);
}

function queryGridActionMenuItems(view,result){
  const helper=globalThis.TaskMenuDatabaseWorkbench;
  if(typeof helper?.gridActionMenuItems!=='function')throw new Error('Shared database grid action menu is unavailable');
  return helper.gridActionMenuItems({
    exportData:()=>exportQueryData(view,result),
    refresh:()=>refreshQueryResult(view),
    filter:()=>openQueryFilterDialog(view,result),
    addRow:()=>addQueryRow(view,result),
    addDisabled:!result?.edit?.editable,
    order:()=>openQueryOrderDialog(view,result)
  });
}

function showQueryContextMenu(view,result,rowIndex,columnIndex,x,y){
  const helper=globalThis.TaskMenuDatabaseWorkbench;
  if(typeof helper?.showContextMenu!=='function')throw new Error('Database context menu is unavailable');
  if(!view.querySelectedRows?.has(rowIndex))selectQueryRow(view,result,rowIndex,{});
  const column=result?.columns?.[columnIndex]||{};
  const value=queryCellValue(view,result,rowIndex,columnIndex);
  helper.showContextMenu(helper.gridContextMenuItems(
    [
      {label:'Copy Value',action:()=>helper.copyText(value===null||value===undefined?'':(typeof value==='object'?JSON.stringify(value):String(value)))},
      {label:'Copy Column Name',action:()=>helper.copyText(column?.name||'')},
      ...queryCopyMenuItems(view,result)
    ],
    queryGridActionMenuItems(view,result)
  ),x,y);
}

function openQueryValueViewer(titleText,value,{editable=false,columnType='',nullable=true,onSave=null}={}){
  const dialog=document.createElement('div');dialog.className='task-connection-dialog';
  const card=document.createElement('div');card.className='task-connection-dialog-card db-value-dialog-card';
  const title=document.createElement('h3');title.textContent=titleText||'Value';
  const area=document.createElement('textarea');area.className='db-value-dialog-area';area.readOnly=!editable;area.value=value===null||value===undefined?'':(typeof value==='object'?JSON.stringify(value,null,2):String(value));
  const actions=document.createElement('div');actions.className='task-connection-dialog-actions';
  const copy=document.createElement('button');copy.type='button';copy.textContent='Copy all';
  copy.onclick=async()=>{try{await copySelectedTextArea(area);const old=copy.textContent;copy.textContent='✓ Copied';setTimeout(()=>{if(copy.isConnected)copy.textContent=old;},1000);}catch(error){app.showError(error);}};
  const close=document.createElement('button');close.type='button';close.textContent=editable?'Cancel':'Close';close.onclick=()=>dialog.remove();
  actions.append(copy);
  if(editable){
    const setNull=document.createElement('button');setNull.type='button';setNull.textContent='Set NULL';setNull.disabled=!nullable;setNull.onclick=()=>{onSave?.(null);dialog.remove();};
    const save=document.createElement('button');save.type='button';save.className='task-connection-primary';save.textContent='Use Value';save.onclick=()=>{
      try{onSave?.(parseQueryEditedValue(area.value,value,columnType));dialog.remove();}catch(error){app.showError(error);}
    };
    actions.append(setNull,save);
  }
  actions.append(close);card.append(title,area,actions);dialog.append(card);document.body.append(dialog);
  dialog.addEventListener('pointerdown',event=>{if(event.target===dialog)dialog.remove();});
  dialog.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();dialog.remove();}});
  if(editable)requestAnimationFrame(()=>{area.focus();area.setSelectionRange(area.value.length,area.value.length);});
}

function renderQueryValuePreview(td,value,onOpen){
  td.classList.add('db-long-text-cell');
  const preview=document.createElement('div');preview.className='db-long-text-preview';
  const text=document.createElement('span');text.className='db-long-text-preview-text';text.textContent=(value===null||value===undefined?'NULL':(typeof value==='object'?JSON.stringify(value):String(value))).replace(/[\r\n]+/g,' ');
  const open=document.createElement('button');open.type='button';open.className='db-long-text-open';open.textContent='…';open.title='Open full value';
  open.onclick=event=>{event.preventDefault();event.stopPropagation();onOpen?.();};
  preview.append(text,open);td.append(preview);
}

function resultCell(view,result,rowIndex,columnIndex){
  const value=queryCellValue(view,result,rowIndex,columnIndex);
  const column=result?.columns?.[columnIndex]||{};
  const editColumn=result?.edit?.columns?.[columnIndex]||column;
  const editable=Boolean(result?.edit?.editable)&&editColumn?.editable!==false&&Boolean(result?.edit?.row_identities?.[rowIndex]);
  const popupOnly=shouldUseQueryValuePopup(value,editColumn?.type||column?.type||'');
  const td=document.createElement('td');td.dataset.rowIndex=String(rowIndex);td.dataset.columnIndex=String(columnIndex);
  if(view.queryDirtyRows?.get(rowIndex)?.has(editColumn?.name||column?.name))td.classList.add('db-query-dirty');
  const openViewer=()=>openQueryValueViewer(editColumn?.name||column?.name||'Value',value,{
    editable,
    columnType:editColumn?.type||column?.type||'',
    nullable:editColumn?.nullable!==false,
    onSave:next=>{setQueryDirtyCell(view,result,rowIndex,columnIndex,next);renderResult(view,result,view.queryElapsed,{preserveDirty:true});}
  });
  if(popupOnly){
    if(editable)td.classList.add('db-query-editable','db-query-popup-editable');
    renderQueryValuePreview(td,value,openViewer);
    td.ondblclick=event=>{event.preventDefault();openViewer();};
    return td;
  }
  if(value===null||value===undefined){td.textContent='NULL';td.classList.add('db-null');}
  else if(typeof value==='object')td.textContent=JSON.stringify(value);
  else td.textContent=String(value);
  if(editable){
    td.contentEditable='true';td.spellcheck=false;td.classList.add('db-query-editable');
    td.addEventListener('focus',()=>{if(td.classList.contains('db-null')){td.textContent='';td.classList.remove('db-null');}});
    td.addEventListener('keydown',event=>{
      if(event.key==='Escape'){event.preventDefault();renderResult(view,result,view.queryElapsed,{preserveDirty:true});return;}
      if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();td.blur();}
    });
    td.addEventListener('blur',()=>{
      try{
        const next=parseQueryEditedValue(td.textContent,value,editColumn?.type||column?.type||'');
        setQueryDirtyCell(view,result,rowIndex,columnIndex,next);
        renderResult(view,result,view.queryElapsed,{preserveDirty:true});
      }catch(error){app.showError(error);renderResult(view,result,view.queryElapsed,{preserveDirty:true});}
    });
  }
  return td;
}

async function applyQueryChanges(view,result){
  const edit=result?.edit;
  if(!edit?.editable)throw new Error(edit?.editability_reason||'This query result is read-only');
  const mutations=[];
  for(const [rowIndex,changes] of view.queryDirtyRows||[]){
    if(!changes?.size)continue;
    const identity=edit.row_identities?.[rowIndex];
    if(!identity)throw new Error('Editable query row identity is unavailable');
    mutations.push({action:'update',identity,values:Object.fromEntries(changes)});
  }
  for(const values of view.queryNewRows||[]){
    if(!values||Object.keys(values).length===0)throw new Error('Enter at least one value for every new row before applying changes');
    mutations.push({action:'insert',values:{...values}});
  }
  if(!mutations.length)return;
  const response=await sessionRequest(view.meta.id,'mutate_rows',{
    catalog:edit.catalog||view.catalog.value||'',
    schema:edit.schema||'',
    kind:edit.kind||'table',
    name:edit.name,
    mutations
  });
  const items=Array.isArray(response?.results)?response.results:null;
  if(!items||items.length!==mutations.length){
    view.queryDirtyRows?.clear?.();
    view.queryNewRows=[];
    await refreshQueryResult(view);
    throw new Error('Database adapter returned an incomplete query mutation result; query was reloaded');
  }
  const errors=items.filter(item=>item?.error);
  view.queryDirtyRows?.clear?.();
  view.queryNewRows=[];
  await refreshQueryResult(view);
  if(errors.length)throw new Error(errors.map(item=>(item.action||'update')+': '+(item.error?.message||item.error?.code||'failed')).join('\n'));
}

function renderResult(view,result,elapsed,{preserveDirty=false}={}){
  view.result.replaceChildren();
  view.queryResult=result;
  view.queryElapsed=elapsed;
  if(!preserveDirty||!(view.queryDirtyRows instanceof Map))view.queryDirtyRows=new Map();
  if(!preserveDirty||!Array.isArray(view.queryNewRows))view.queryNewRows=[];
  if(!preserveDirty||!(view.querySelectedRows instanceof Set)){view.querySelectedRows=new Set();view.querySelectionAnchor=null;}
  const status=document.createElement('div');status.className='db-result-status';
  const columns=Array.isArray(result?.columns)?result.columns:[];
  const rows=Array.isArray(result?.rows)?result.rows:[];
  const displayIndexes=queryDisplayRowIndexes(view,result);
  const parts=[rows.length+' row'+(rows.length===1?'':'s')];
  if(displayIndexes.length!==rows.length)parts.push('shown '+displayIndexes.length);
  if(result?.truncated)parts.push('truncated');
  if(Number.isFinite(result?.affected_rows))parts.push('affected '+result.affected_rows);
  if(Number.isFinite(elapsed))parts.push(elapsed+' ms');
  if(result?.edit?.editable)parts.push('editable');
  else if(result?.edit?.editability_reason)parts.push('read-only');
  status.dataset.base=parts.join(' · ');
  status.textContent=status.dataset.base;
  if(result?.edit?.editability_reason)status.title=result.edit.editability_reason;
  view.result.append(status);
  if(result?.edit){
    const tools=document.createElement('div');tools.className='db-result-edit-tools';
    const info=document.createElement('div');info.className='db-result-edit-info';
    const pending=queryPendingCount(view);
    info.textContent=result.edit.editable
      ?((result.edit.catalog?result.edit.catalog+'.':'')+result.edit.name+(pending?' · '+pending+' pending':''))
      :(result.edit.editability_reason||'Query result is read-only');
    tools.append(info);
    if(result.edit.editable){
      const add=document.createElement('button');add.type='button';add.textContent='Add row';add.onclick=()=>addQueryRow(view,result);
      const revert=document.createElement('button');revert.type='button';revert.textContent='Revert';revert.disabled=pending===0;
      revert.onclick=()=>{view.queryDirtyRows.clear();view.queryNewRows=[];renderResult(view,result,elapsed,{preserveDirty:true});};
      const apply=document.createElement('button');apply.type='button';apply.className='db-result-apply';apply.textContent='Apply changes';apply.disabled=pending===0;
      apply.onclick=async()=>{
        apply.disabled=true;
        try{await applyQueryChanges(view,result);}
        catch(error){app.showError(error);if(apply.isConnected&&queryPendingCount(view)>0)apply.disabled=false;}
      };
      tools.append(add,revert,apply);
    }
    view.result.append(tools);
  }
  if(!columns.length){
    const empty=document.createElement('div');empty.className='task-connection-empty';empty.textContent='Statement completed with no tabular result';view.result.append(empty);return;
  }
  const table=document.createElement('table');table.className='db-result-table';
  const thead=document.createElement('thead');const header=document.createElement('tr');
  const selectAll=document.createElement('th');selectAll.className='db-row-number';selectAll.dataset.selectAll='1';selectAll.textContent='☐';selectAll.title='Select all rows in current result';
  selectAll.onclick=event=>{event.preventDefault();toggleSelectAllQueryRows(view,result);};
  selectAll.oncontextmenu=event=>{
    event.preventDefault();event.stopPropagation();
    const helper=globalThis.TaskMenuDatabaseWorkbench;
    if(typeof helper?.showContextMenu!=='function')return;
    const selected=querySelectedRowIndexes(view,result).length;
    helper.showContextMenu(helper.gridContextMenuItems(
      [{label:selected?'Clear row selection':'Select all visible rows',action:()=>toggleSelectAllQueryRows(view,result)}],
      queryCopyMenuItems(view,result),
      queryGridActionMenuItems(view,result)
    ),event.clientX,event.clientY);
  };
  header.append(selectAll);
  columns.forEach((column,columnIndex)=>{
    const th=document.createElement('th');th.textContent=column?.name||'';if(column?.type)th.title=column.type;
    if(view.queryOrder?.columnIndex===columnIndex)th.textContent+=(view.queryOrder.direction==='desc'?' ▼':' ▲');
    th.style.cursor='pointer';th.onclick=()=>toggleQueryHeaderOrder(view,result,columnIndex);header.append(th);
  });
  thead.append(header);table.append(thead);
  const tbody=document.createElement('tbody');
  displayIndexes.forEach(rowIndex=>{const row=rows[rowIndex];
    const tr=document.createElement('tr');tr.dataset.rowIndex=String(rowIndex);
    if(view.querySelectedRows?.has(rowIndex))tr.classList.add('db-selected');
    const rowNo=document.createElement('td');rowNo.className='db-row-number';rowNo.textContent=String(rowIndex+1);rowNo.title='Click to select row · Ctrl/Cmd-click multi-select · Shift-click range';
    rowNo.onclick=event=>{event.preventDefault();selectQueryRow(view,result,rowIndex,event);};
    rowNo.oncontextmenu=event=>{
      event.preventDefault();event.stopPropagation();
      if(!view.querySelectedRows?.has(rowIndex))selectQueryRow(view,result,rowIndex,{});
      const helper=globalThis.TaskMenuDatabaseWorkbench;
      if(typeof helper?.showContextMenu!=='function')return;
      helper.showContextMenu(helper.gridContextMenuItems(
        queryCopyMenuItems(view,result),
        queryGridActionMenuItems(view,result)
      ),event.clientX,event.clientY);
    };
    tr.append(rowNo);
    (Array.isArray(row)?row:[]).forEach((_,columnIndex)=>{
      const td=resultCell(view,result,rowIndex,columnIndex);
      td.oncontextmenu=event=>{event.preventDefault();event.stopPropagation();showQueryContextMenu(view,result,rowIndex,columnIndex,event.clientX,event.clientY);};
      tr.append(td);
    });
    tbody.append(tr);
  });
  (view.queryNewRows||[]).forEach((values,newIndex)=>{
    const tr=document.createElement('tr');tr.className='db-query-new-row';
    const rowNo=document.createElement('td');rowNo.className='db-row-number';rowNo.textContent='+';
    rowNo.title='New row';tr.append(rowNo);
    columns.forEach((column,columnIndex)=>{
      const editColumn=result?.edit?.columns?.[columnIndex]||column;
      const td=document.createElement('td');td.classList.add('db-query-editable');
      const has=Object.prototype.hasOwnProperty.call(values,editColumn?.name||column?.name);
      const value=has?values[editColumn?.name||column?.name]:null;
      const popupOnly=shouldUseQueryValuePopup(value,editColumn?.type||column?.type||'');
      const openViewer=()=>openQueryValueViewer(editColumn?.name||column?.name||'Value',value,{
        editable:true,
        columnType:editColumn?.type||column?.type||'',
        nullable:editColumn?.nullable!==false,
        onSave:next=>{setQueryNewCell(view,result,newIndex,columnIndex,next);renderResult(view,result,view.queryElapsed,{preserveDirty:true});}
      });
      if(popupOnly){
        td.classList.add('db-query-popup-editable');renderQueryValuePreview(td,value,openViewer);
        td.ondblclick=event=>{event.preventDefault();openViewer();};
      }else{
        td.textContent=has?(value===null?'NULL':String(value)):'';
        if(has&&value===null)td.classList.add('db-null');
        td.contentEditable='true';td.spellcheck=false;
        td.addEventListener('focus',()=>{if(td.classList.contains('db-null')){td.textContent='';td.classList.remove('db-null');}});
        td.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();renderResult(view,result,view.queryElapsed,{preserveDirty:true});return;}if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();td.blur();}});
        td.addEventListener('blur',()=>{
          try{
            const text=td.textContent;
            if(text==='')delete values[editColumn?.name||column?.name];
            else setQueryNewCell(view,result,newIndex,columnIndex,parseQueryEditedValue(text,'',editColumn?.type||column?.type||''));
            renderResult(view,result,view.queryElapsed,{preserveDirty:true});
          }catch(error){app.showError(error);renderResult(view,result,view.queryElapsed,{preserveDirty:true});}
        });
      }
      td.oncontextmenu=event=>{
        event.preventDefault();event.stopPropagation();
        const helper=globalThis.TaskMenuDatabaseWorkbench;
        if(typeof helper?.showContextMenu!=='function')return;
        helper.showContextMenu(helper.gridContextMenuItems(
          queryGridActionMenuItems(view,result),
          [
            {label:'Open Value in Editor',action:openViewer},
            {label:'Remove New Row',danger:true,action:()=>{view.queryNewRows.splice(newIndex,1);renderResult(view,result,view.queryElapsed,{preserveDirty:true});}}
          ]
        ),event.clientX,event.clientY);
      };
      tr.append(td);
    });
    tbody.append(tr);
  });
  table.append(tbody);view.result.append(table);syncQuerySelection(view,result);
}

function splitSQLStatements(script){
  const text=String(script||'');
  const statements=[];let start=0;let quote='';let lineComment=false;let blockComment=false;let segmentHasCode=false;
  const push=end=>{
    const statement=text.slice(start,end).trim();
    if(statement&&segmentHasCode)statements.push(statement);
    start=end+1;segmentHasCode=false;
  };
  for(let i=0;i<text.length;i++){
    const ch=text[i],next=text[i+1]||'';
    if(lineComment){
      if(ch==='\n'||ch==='\r')lineComment=false;
      continue;
    }
    if(blockComment){
      if(ch==='*'&&next==='/'){blockComment=false;i++;}
      continue;
    }
    if(quote){
      if(ch==='\\'){i++;continue;}
      if(ch===quote){
        if(text[i+1]===quote){i++;continue;}
        quote='';
      }
      continue;
    }
    if(ch==='\''||ch==='"'||ch==='\x60'){quote=ch;segmentHasCode=true;continue;}
    if(ch==='#'){lineComment=true;continue;}
    if(ch==='-'&&next==='-'&&(i+2>=text.length||/\s/.test(text[i+2]))){lineComment=true;i++;continue;}
    if(ch==='/'&&next==='*'){blockComment=true;i++;continue;}
    if(ch===';'){push(i);continue;}
    if(!/\s/.test(ch))segmentHasCode=true;
  }
  const tail=text.slice(start).trim();if(tail&&segmentHasCode)statements.push(tail);
  return statements;
}

function queryResultHasPendingChanges(view){
  if(queryPendingCount(view)>0)return true;
  return Array.isArray(view.queryResultContexts)&&view.queryResultContexts.some(ctx=>queryPendingCount(ctx)>0);
}

function activateQueryResult(owner,index){
  for(let i=0;i<(owner.queryResultContexts||[]).length;i++){
    const ctx=owner.queryResultContexts[i];
    ctx.resultTab?.classList.toggle('active',i===index);
    ctx.result?.classList.toggle('hidden',i!==index);
  }
  owner.activeQueryResult=index;
}

function createQueryResultContext(owner,statement,index){
  if(!owner.queryResultTabs){
    owner.result.replaceChildren();
    owner.queryResultTabs=document.createElement('div');owner.queryResultTabs.className='db-query-result-tabs';
    owner.queryResultPanels=document.createElement('div');owner.queryResultPanels.className='db-query-result-panels';
    owner.result.append(owner.queryResultTabs,owner.queryResultPanels);
    owner.queryResultContexts=[];
  }
  const tab=document.createElement('button');tab.type='button';tab.className='db-query-result-tab';tab.textContent='Result '+(index+1);
  tab.title=String(statement||'').replace(/\s+/g,' ').slice(0,240);
  const panel=document.createElement('div');panel.className='db-query-result-panel hidden';
  const ctx=Object.create(owner);
  Object.assign(ctx,{
    result:panel,
    resultTab:tab,
    resultStatement:statement,
    resultOwner:owner,
    queryFilter:null,
    queryOrder:null,
    queryDirtyRows:new Map(),
    queryNewRows:[],
    querySelectedRows:new Set(),
    querySelectionAnchor:null,
    lastExecutedStatement:statement
  });
  owner.queryResultContexts.push(ctx);owner.queryResultTabs.append(tab);owner.queryResultPanels.append(panel);
  tab.onclick=()=>activateQueryResult(owner,index);
  if(index===0)activateQueryResult(owner,0);
  return ctx;
}

function renderQueryResultError(view,error,elapsed){
  view.result.replaceChildren();
  const status=document.createElement('div');status.className='db-result-status';
  status.textContent='ERROR'+(Number.isFinite(elapsed)?' · '+elapsed+' ms':'');
  const message=document.createElement('div');message.className='db-query-result-error';message.textContent=String(error?.message||error||'Query failed');
  view.result.append(status,message);
}

async function refreshQueryResult(view){
  if(!view.resultStatement)return executeQuery(view);
  if(queryPendingCount(view)>0&&!confirm('Discard unsaved query result changes and refresh this result?'))return;
  const maxRows=Math.max(1,Math.min(1000,Number(view.maxRows.value)||100));
  const payload={statement:view.resultStatement,max_rows:maxRows};
  if(view.catalog.value)payload.catalog=view.catalog.value;
  const started=performance.now();
  const result=await sessionRequest(view.meta.id,'execute',payload);
  renderResult(view,result,Math.round(performance.now()-started));
}

async function executeQuery(view,{discardPending=false}={}){
  const owner=view.resultOwner||view;
  if(!discardPending&&queryResultHasPendingChanges(owner)&&!confirm('Discard unsaved query result changes and run again?'))return;
  const script=queryEditorExecutionText(owner).trim();
  if(!script)throw new Error('Enter a database statement or select SQL to run');
  const statements=relationalQueryEditor(owner)?splitSQLStatements(script):[script];
  if(!statements.length)throw new Error('Enter a database statement first');
  if(owner.lastExecutedStatement&&owner.lastExecutedStatement!==script){owner.queryFilter=null;owner.queryOrder=null;owner.querySelectedRows?.clear?.();}
  owner.lastExecutedStatement=script;
  owner.queryDirtyRows=new Map();owner.queryNewRows=[];owner.querySelectedRows=new Set();owner.querySelectionAnchor=null;
  owner.queryFilter=null;owner.queryOrder=null;
  const maxRows=Math.max(1,Math.min(1000,Number(owner.maxRows.value)||100));
  owner.run.disabled=true;owner.run.textContent=statements.length>1?'Running 1/'+statements.length+'…':'Running…';
  owner.queryResultTabs=null;owner.queryResultPanels=null;owner.queryResultContexts=[];
  try{
    if(statements.length===1){
      owner.result.replaceChildren();
      const started=performance.now();
      const payload={statement:statements[0],max_rows:maxRows};
      if(owner.catalog.value)payload.catalog=owner.catalog.value;
      const result=await sessionRequest(owner.meta.id,'execute',payload);
      owner.resultStatement=statements[0];
      renderResult(owner,result,Math.round(performance.now()-started));
      return;
    }
    owner.result.replaceChildren();
    let firstError=null;
    for(let index=0;index<statements.length;index++){
      owner.run.textContent='Running '+(index+1)+'/'+statements.length+'…';
      const ctx=createQueryResultContext(owner,statements[index],index);
      const payload={statement:statements[index],max_rows:maxRows};
      if(owner.catalog.value)payload.catalog=owner.catalog.value;
      const started=performance.now();
      try{
        const result=await sessionRequest(owner.meta.id,'execute',payload);
        renderResult(ctx,result,Math.round(performance.now()-started));
      }catch(error){
        renderQueryResultError(ctx,error,Math.round(performance.now()-started));
        activateQueryResult(owner,index);
        firstError=error;
        break;
      }
    }
    if(firstError)throw firstError;
  }finally{
    owner.run.disabled=false;owner.run.textContent='Run';
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
  if(relationalQueryEditor(view)&&Array.isArray(detail?.columns)){
    const catalog=String(view.catalog.value||'').trim();
    const key=catalog+'\u0000'+String(object?.name||'');
    view.querySchemaCache?.set?.(key,detail.columns.map(column=>String(column?.name||'').trim()).filter(Boolean));
    reconfigureQueryEditor(view);
  }
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
    warmQuerySchema(view).catch(error=>console.warn('Query autocomplete schema load failed',error));
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

function defaultDatabaseQueryText(adapterKind){
  return adapterKind==='redis'
    ?'PING'
    :(adapterKind==='mongo'
      ?'{\n  "op": "find",\n  "collection": "users",\n  "filter": {},\n  "limit": 100\n}'
      :'SELECT 1');
}

function setupQueryPanel(view,{initialText='',scriptName='query.sql',maxRowsValue='100'}={}){
  const query=document.createElement('div');query.className='db-query';
  const tools=document.createElement('div');tools.className='db-query-tools';
  const run=document.createElement('button');run.type='button';run.className='db-run';run.textContent='Run';
  const openSQL=document.createElement('button');openSQL.type='button';openSQL.textContent='Open SQL';openSQL.hidden=!(view.meta.adapter_kind==='mysql'||view.meta.adapter_kind==='sqlite');
  const saveSQL=document.createElement('button');saveSQL.type='button';saveSQL.textContent='Save SQL';saveSQL.hidden=openSQL.hidden;
  const rowsLabel=document.createElement('label');rowsLabel.textContent='Max rows';
  const maxRows=document.createElement('input');maxRows.type='number';maxRows.min='1';maxRows.max='1000';maxRows.value=String(maxRowsValue||'100');
  tools.append(run,openSQL,saveSQL,rowsLabel,maxRows);
  const editor=document.createElement('textarea');editor.className='db-query-editor';editor.spellcheck=false;editor.value=String(initialText||'');
  const result=document.createElement('div');result.className='db-result-wrap';
  query.append(tools,editor,result);

  Object.assign(view,{
    queryPanel:query,editor,run,openSQL,saveSQL,maxRows,result,
    scriptName:scriptName||'query.sql',scriptSource:null,
    queryFilter:null,queryOrder:null,lastExecutedStatement:'',queryNewRows:[],
    queryDirtyRows:new Map(),querySelectedRows:new Set(),querySelectionAnchor:null,
    queryResultTabs:null,queryResultPanels:null,queryResultContexts:[],activeQueryResult:0,resultStatement:'',
    querySchemaTimer:null
  });
  if(!(view.querySchemaCache instanceof Map))view.querySchemaCache=new Map();
  initQueryEditor(view);
  run.onclick=()=>executeQuery(view).catch(app.showError);
  openSQL.onclick=()=>openSQLScript(view).catch(app.showError);
  saveSQL.onclick=()=>saveSQLScript(view).catch(app.showError);
  if(!view.queryCM)editor.addEventListener('keydown',event=>{
    if((event.ctrlKey||event.metaKey)&&event.key==='Enter'){event.preventDefault();executeQuery(view).catch(app.showError);}
  });
  return query;
}

function createAdditionalQueryView(root,{initialText='',scriptName='query.sql'}={}){
  const child=Object.create(root);
  child.workbenchRoot=root;
  const text=initialText||defaultDatabaseQueryText(root.meta.adapter_kind);
  setupQueryPanel(child,{initialText:text,scriptName,maxRowsValue:root.maxRows?.value||'100'});
  return child;
}

function queryViewHasPendingChanges(view){
  return queryResultHasPendingChanges(view);
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

  const view={meta,profile,tab,pane,catalog,refresh,objects,detail,objectData:[],querySchemaCache:new Map()};
  const query=setupQueryPanel(view,{initialText:defaultDatabaseQueryText(meta.adapter_kind),scriptName:'query.sql'});
  body.append(browser,query);pane.append(head,body);panes.append(pane);

  dbViews.set(meta.id,view);
  globalThis.TaskMenuDatabaseWorkbench?.enhanceView?.(view);
  const resetQuerySchema=()=>{
    view.querySchemaCache?.clear?.();
    for(const queryView of databaseQueryViews(view)){
      queryView.querySchemaGeneration=(queryView.querySchemaGeneration||0)+1;
      clearTimeout(queryView.querySchemaTimer);
    }
  };
  reload.onclick=()=>{resetQuerySchema();loadCatalogs(view).catch(app.showError);};
  refresh.onclick=()=>{resetQuerySchema();loadObjects(view).catch(app.showError);};
  catalog.onchange=()=>{resetQuerySchema();loadObjects(view).catch(app.showError);};
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
  getQueryText:queryEditorText,
  getQueryExecutionText:queryEditorExecutionText,
  setQueryText:setQueryEditorText,
  focusQuery:focusQueryEditor,
  createQueryView:createAdditionalQueryView,
  queryHasPendingChanges:queryViewHasPendingChanges,
  saveTextWithLocation,
  restoreSessions:restoreDatabaseSessions,
  get views(){return dbViews;}
};

if(app.taskData?.workspace)restoreDatabaseSessions().catch(error=>console.warn('Database session restore failed',error));
else window.addEventListener('taskmenu:tasks',()=>restoreDatabaseSessions().catch(error=>console.warn('Database session restore failed',error)),{once:true});
