const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for LSP integration');

const editorAPI=()=>globalThis.TaskMenuEditor;

const style=document.createElement('style');
style.textContent=`
.editor-lsp{font-size:10px}.editor-lsp.available{border-color:#416b4b}.editor-lsp.unavailable{opacity:.5}
.lsp-backdrop{display:none;position:fixed;inset:0;z-index:17880;background:rgba(0,0,0,.52);align-items:flex-start;justify-content:center;padding:7vh 16px}.lsp-backdrop.visible{display:flex}
.lsp-dialog{width:min(820px,96vw);max-height:82vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 60px rgba(0,0,0,.58);overflow:hidden}
.lsp-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #303843}.lsp-head strong{flex:1}.lsp-meta{font-size:10px;opacity:.62}
.lsp-actions{display:flex;gap:6px;flex-wrap:wrap;padding:8px 10px;border-bottom:1px solid #303843}.lsp-body{overflow:auto;padding:10px;min-height:100px}.lsp-empty{padding:24px;text-align:center;opacity:.6}
.lsp-result{white-space:pre-wrap;overflow-wrap:anywhere;font:11px/1.5 ui-monospace,monospace}.lsp-row{display:grid;grid-template-columns:72px minmax(0,1fr) auto;gap:7px;align-items:start;width:100%;text-align:left;padding:7px 8px;border:1px solid transparent;background:transparent}.lsp-row:hover{background:#28313e;border-color:#415168}.lsp-severity{font-size:9px;font-weight:800}.lsp-location{font:10px/1.35 ui-monospace,monospace;opacity:.65}.lsp-message{white-space:pre-wrap;overflow-wrap:anywhere}.lsp-rename-preview{white-space:pre-wrap;font:10px/1.45 ui-monospace,monospace;background:#0b0f14;border:1px solid #303843;border-radius:6px;padding:8px;max-height:45vh;overflow:auto}.lsp-warning{color:#f0c66b;margin-bottom:8px}
html[data-taskmenu-theme="light"] .lsp-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .lsp-rename-preview{background:#f6f8fa;border-color:#d0d7de}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='lsp-backdrop';
const dialog=document.createElement('div');dialog.className='lsp-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Language Server');
const head=document.createElement('div');head.className='lsp-head';
const title=document.createElement('strong');title.textContent='LANGUAGE SERVER';
const meta=document.createElement('span');meta.className='lsp-meta';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';
head.append(title,meta,closeButton);
const actions=document.createElement('div');actions.className='lsp-actions';
const diagnosticsButton=document.createElement('button');diagnosticsButton.type='button';diagnosticsButton.textContent='Diagnostics';
const hoverButton=document.createElement('button');hoverButton.type='button';hoverButton.textContent='Hover';
const definitionButton=document.createElement('button');definitionButton.type='button';definitionButton.textContent='Definition';
const referencesButton=document.createElement('button');referencesButton.type='button';referencesButton.textContent='References';
const renameButton=document.createElement('button');renameButton.type='button';renameButton.textContent='Rename';
actions.append(diagnosticsButton,hoverButton,definitionButton,referencesButton,renameButton);
const body=document.createElement('div');body.className='lsp-body';
dialog.append(head,actions,body);backdrop.append(dialog);document.body.append(backdrop);

let currentView=null;
const supportCache=new Map();

function pathExtension(pathValue){
  const name=String(pathValue||'').toLowerCase();const slash=name.lastIndexOf('/');const dot=name.lastIndexOf('.');
  return dot>slash?name.slice(dot):'';
}
function lspMapped(pathValue){return ['.go','.c','.h','.cc','.cpp','.cxx','.hpp','.hh','.hxx','.py','.pyi','.ts','.tsx','.js','.jsx','.mjs','.cjs'].includes(pathExtension(pathValue));}
function cursorPosition(view){
  const pos=view.cm.state.selection.main.head;
  const line=view.cm.state.doc.lineAt(pos);
  return {line:line.number-1,character:pos-line.from};
}
function requestPayload(view,action,extra={}){
  if((view?.file?.remote_workspace_id||view?.file?.remote_transfer_profile_id))throw new Error('Language Server actions are not available for Remote Workspace files');
  return {
    action,path:String(view.file.path||''),text:view.cm.state.doc.toString(),
    position:cursorPosition(view),...extra
  };
}
async function request(view,action,extra={}){
  return app.jsonFetch('/api/lsp',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify(requestPayload(view,action,extra))
  });
}
async function support(pathValue,{refresh=false}={}){
  pathValue=String(pathValue||'');
  if(!refresh&&supportCache.has(pathValue))return supportCache.get(pathValue);
  if(!lspMapped(pathValue)){const value={supported:false,available:false};supportCache.set(pathValue,value);return value;}
  const value=await app.jsonFetch('/api/lsp?path='+encodeURIComponent(pathValue),{cache:'no-store'});
  supportCache.set(pathValue,value);return value;
}
function activeView(){
  const api=editorAPI();if(!api?.active)return null;
  return api.editors?.get?.(api.active)||null;
}
function setEmpty(message){body.replaceChildren();const node=document.createElement('div');node.className='lsp-empty';node.textContent=message;body.append(node);}
function severityLabel(value){
  return ({1:'ERROR',2:'WARN',3:'INFO',4:'HINT'})[Number(value)]||'DIAG';
}
function lineLabel(range){
  const start=range?.start||{};const end=range?.end||{};
  return 'L'+(Number(start.line||0)+1)+':'+(Number(start.character||0)+1)+' → L'+(Number(end.line||0)+1)+':'+(Number(end.character||0)+1);
}
async function jumpLocation(location){
  const api=editorAPI();if(!api?.openFile)throw new Error('Editor navigation unavailable');
  const view=await api.openFile(String(location?.path||''));
  const lineNumber=Number(location?.range?.start?.line||0)+1;
  api.jumpEditorToLine?.(view,lineNumber);
  const line=view.cm.state.doc.line(Math.max(1,Math.min(view.cm.state.doc.lines,lineNumber)));
  const character=Math.max(0,Number(location?.range?.start?.character||0));
  const target=Math.min(line.to,line.from+character);
  view.cm.dispatch({selection:{anchor:target},scrollIntoView:true});view.cm.focus();
}
function renderLocations(locations,label='location'){
  body.replaceChildren();locations=Array.isArray(locations)?locations:[];
  if(!locations.length){setEmpty('No '+label+' found');return;}
  for(const item of locations){
    const row=document.createElement('button');row.type='button';row.className='lsp-row';
    const kind=document.createElement('span');kind.className='lsp-severity';kind.textContent=label.toUpperCase();
    const path=document.createElement('span');path.className='lsp-message';path.textContent=String(item.path||'');
    const loc=document.createElement('span');loc.className='lsp-location';loc.textContent=lineLabel(item.range);
    row.append(kind,path,loc);row.onclick=()=>{close();jumpLocation(item).catch(app.showError);};body.append(row);
  }
}
function renderDiagnostics(items){
  body.replaceChildren();items=Array.isArray(items)?items:[];
  if(!items.length){setEmpty('No diagnostics reported');return;}
  for(const item of items){
    const row=document.createElement('button');row.type='button';row.className='lsp-row';
    const severity=document.createElement('span');severity.className='lsp-severity';severity.textContent=severityLabel(item.severity);
    const message=document.createElement('span');message.className='lsp-message';message.textContent=String(item.message||'');
    const loc=document.createElement('span');loc.className='lsp-location';loc.textContent=lineLabel(item.range);
    row.append(severity,message,loc);
    row.onclick=()=>jumpLocation({path:currentView.file.path,range:item.range}).catch(app.showError);
    body.append(row);
  }
}
function renderText(text,empty='No information returned'){
  body.replaceChildren();
  if(!String(text||'').trim()){setEmpty(empty);return;}
  const node=document.createElement('div');node.className='lsp-result';node.textContent=String(text);body.append(node);
}
async function runAction(action){
  const view=currentView||activeView();if(!view)throw new Error('No active editor');
  setEmpty('Running '+action+'…');
  const result=await request(view,action);
  meta.textContent=String(result.server||'LSP')+' · '+String(result.language_id||'');
  if(action==='diagnostics')renderDiagnostics(result.diagnostics);
  else if(action==='hover')renderText(result.hover,'No hover information');
  else renderLocations(result.locations,action==='definition'?'definition':'reference');
  return result;
}
function positionOffset(text,position){
  const lines=String(text).split('\n');
  const line=Math.max(0,Math.min(lines.length-1,Number(position?.line)||0));
  let offset=0;for(let i=0;i<line;i++)offset+=lines[i].length+1;
  return Math.min(offset+lines[line].length,offset+Math.max(0,Number(position?.character)||0));
}
function applyTextEdits(text,edits){
  const prepared=(Array.isArray(edits)?edits:[]).map(edit=>({
    from:positionOffset(text,edit.range?.start),to:positionOffset(text,edit.range?.end),text:String(edit.new_text??'')
  })).sort((a,b)=>b.from-a.from||b.to-a.to);
  let next=String(text);
  for(const edit of prepared){
    if(edit.from<0||edit.to<edit.from||edit.to>next.length)throw new Error('Language server returned an invalid rename range');
    next=next.slice(0,edit.from)+edit.text+next.slice(edit.to);
  }
  return next;
}
function editorForPath(pathValue){
  for(const view of editorAPI()?.editors?.values?.()||[]){
    if(!view.closed&&String(view.file?.path||'')===String(pathValue||''))return view;
  }
  return null;
}
async function preflightRename(edits){
  const files=[];
  for(const item of Array.isArray(edits)?edits:[]){
    const pathValue=String(item?.path||'').trim();if(!pathValue)continue;
    const opened=editorForPath(pathValue);
    if(opened?.dirty)throw new Error('Save or revert unsaved editor before rename: '+pathValue);
    const file=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(pathValue),{cache:'no-store'});
    files.push({path:pathValue,file,content:applyTextEdits(String(file.content||''),item.edits||[])});
  }
  return files;
}
function renamePreview(files,newName){
  return ['Rename symbol → '+newName,'',...files.flatMap(file=>[
    file.path+' · '+String(file.file?.content?.split?.('\n')?.length||0)+' line(s)',
    '  '+(file.content===file.file.content?'(no text change)':'will update'),
  ])].join('\n');
}
async function writeRenamedFile(item){
  const response=await app.fetchWithLease('/api/project/file',{
    method:'PUT',cache:'no-store',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({path:item.path,content:item.content,expected_sha256:item.file.sha256,line_ending:'preserve',encoding:item.file.encoding||'utf-8'})
  });
  if(response.status===409)throw new Error('Rename stopped because file changed externally: '+item.path);
  if(!response.ok)throw new Error((await response.text()).trim()||('HTTP '+response.status));
  return response.json();
}
async function renameSymbol(){
  const view=currentView||activeView();if(!view)throw new Error('No active editor');
  if(view.dirty)throw new Error('Save the active editor before Rename Symbol');
  const currentWord=(()=>{
    const pos=view.cm.state.selection.main.head,text=view.cm.state.doc.toString();
    let a=pos,b=pos;while(a>0&&/[\w$]/.test(text[a-1]))a--;while(b<text.length&&/[\w$]/.test(text[b]))b++;
    return text.slice(a,b);
  })();
  const newName=window.prompt('Rename symbol to:',currentWord||'');
  if(newName===null)return;
  if(!String(newName).trim())throw new Error('New symbol name is required');
  setEmpty('Preparing rename preview…');
  const result=await request(view,'rename',{new_name:String(newName).trim()});
  const files=await preflightRename(result.edits);
  if(!files.length){setEmpty('Language server returned no rename edits');return;}
  body.replaceChildren();
  const warning=document.createElement('div');warning.className='lsp-warning';warning.textContent='Preview all files before apply. Writes use optimistic SHA checks and stop if a target changed.';
  const pre=document.createElement('pre');pre.className='lsp-rename-preview';pre.textContent=renamePreview(files,String(newName).trim());
  const apply=document.createElement('button');apply.type='button';apply.textContent='Apply rename to '+files.length+' file(s)';
  body.append(warning,pre,apply);
  apply.onclick=async()=>{
    if(!window.confirm('Apply Rename Symbol to '+files.length+' file(s)?'))return;
    apply.disabled=true;
    try{
      for(const item of files)await writeRenamedFile(item);
      for(const item of files){
        const opened=editorForPath(item.path);if(opened)await editorAPI().reloadEditor(opened,{force:true});
      }
      setEmpty('Rename applied to '+files.length+' file(s).');
    }finally{apply.disabled=false;}
  };
}
async function open(view=null){
  view=view||activeView();if(!view)throw new Error('No active editor');
  const info=await support(view.file.path,{refresh:true});
  if(!info.supported)throw new Error('No LSP mapping for '+pathExtension(view.file.path));
  if(!info.available)throw new Error(String(info.server||'Language server')+' is not installed or not on PATH');
  currentView=view;title.textContent='LANGUAGE SERVER · '+String(view.file.path);meta.textContent=String(info.server||'')+' · '+String(info.language_id||'');
  setEmpty('Choose Diagnostics, Hover, Definition, References or Rename.');
  backdrop.classList.add('visible');
}
function close(){backdrop.classList.remove('visible');currentView=null;}

async function decorateView(view){
  if(!view||view.closed||view.lspButton?.isConnected)return;
  if((view.file?.remote_workspace_id||view.file?.remote_transfer_profile_id))return;
  if(!lspMapped(view.file?.path))return;
  const head=view.pane?.querySelector?.('.editor-head');if(!head)return;
  const button=document.createElement('button');button.type='button';button.className='editor-lsp';button.textContent='LSP';button.title='Language Server actions';
  view.lspButton=button;
  const firstSplit=head.querySelector('.editor-split-action');head.insertBefore(button,firstSplit||null);
  try{
    const info=await support(view.file.path);
    button.classList.toggle('available',Boolean(info.available));button.classList.toggle('unavailable',!info.available);
    button.title=info.available?('Language Server · '+String(info.server||'')):String(info.server||'Language server')+' not found on host PATH';
  }catch(error){button.classList.add('unavailable');button.title=String(error?.message||error);}
  button.onclick=()=>open(view).catch(app.showError);
}
function decorateAll(){for(const view of editorAPI()?.editors?.values?.()||[])decorateView(view);}
window.addEventListener('taskmenu:project-file-opened',event=>{
  const pathValue=String(event.detail?.path||'');
  const view=editorForPath(pathValue);if(view)decorateView(view);
});
window.addEventListener('taskmenu:tasks',decorateAll);
queueMicrotask(decorateAll);

diagnosticsButton.onclick=()=>runAction('diagnostics').catch(error=>{setEmpty(String(error?.message||error));app.showError(error);});
hoverButton.onclick=()=>runAction('hover').catch(error=>{setEmpty(String(error?.message||error));app.showError(error);});
definitionButton.onclick=()=>runAction('definition').catch(error=>{setEmpty(String(error?.message||error));app.showError(error);});
referencesButton.onclick=()=>runAction('references').catch(error=>{setEmpty(String(error?.message||error));app.showError(error);});
renameButton.onclick=()=>renameSymbol().catch(error=>{setEmpty(String(error?.message||error));app.showError(error);});
closeButton.onclick=close;backdrop.onmousedown=event=>{if(event.target===backdrop)close();};
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();});

globalThis.TaskMenuLSP={
  open,request,support,decorateView,diagnostics:view=>request(view||activeView(),'diagnostics'),
  hover:view=>request(view||activeView(),'hover'),
  definition:view=>request(view||activeView(),'definition'),
  references:view=>request(view||activeView(),'references'),
  rename:renameSymbol
};
