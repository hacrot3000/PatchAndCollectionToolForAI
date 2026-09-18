const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for editor');
if(!globalThis.cm6?.load)throw new Error('Vendored CodeMirror 6 bundle unavailable');

const cmFactory=globalThis.cm6.load();
const tabsHost=document.querySelector('#tabs');
const panesHost=document.querySelector('#panes');
if(!tabsHost||!panesHost)throw new Error('Editor tab hosts unavailable');

const style=document.createElement('style');
style.textContent=`
.editor-pane{background:#0d1117}
.editor-head{min-height:40px;padding:5px 9px}
.editor-head .editor-path{font:12px ui-monospace,monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1;opacity:.82}
.editor-head .editor-meta{font-size:10px;opacity:.58;white-space:nowrap}
.editor-head .editor-readonly{font-size:10px;padding:2px 6px;border:1px solid #7d6733;border-radius:10px;color:#ffe29a;background:#493b1d;white-space:nowrap}
.editor-host{flex:1;min-height:0;overflow:hidden}
.editor-host .cm-editor{height:100%;font-size:13px}
.editor-host .cm-scroller{overflow:auto;font-family:ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace}
.editor-tab .editor-dirty{display:none;margin-left:5px;color:#f2c96d}
.editor-tab.dirty .editor-dirty{display:inline}
.editor-tab .close{margin-left:8px}
html[data-taskmenu-theme="light"] .editor-pane{background:#fff}
html[data-taskmenu-theme="light"] .editor-head .editor-readonly{color:#6c5314;background:#fff6d9;border-color:#c9ab61}
`;
document.head.append(style);

const editors=new Map();
const opening=new Map();
let activeEditorID='';

function basename(pathValue){
  const parts=String(pathValue||'').split('/');
  return parts[parts.length-1]||pathValue;
}
function editorID(pathValue){return 'editor:'+pathValue;}
function formatBytes(size){
  const value=Number(size||0);
  if(value<1024)return value+' B';
  if(value<1024*1024)return (value/1024).toFixed(value<10*1024?1:0)+' KiB';
  return (value/(1024*1024)).toFixed(1)+' MiB';
}
function languageOptions(pathValue){
  const lower=String(pathValue||'').toLowerCase();
  const name=basename(lower);
  const ext=(name.includes('.')?name.slice(name.lastIndexOf('.')+1):'');
  const options={lineWrapping:false};
  if(document.documentElement.dataset.taskmenuTheme!=='light')options.dark=true;
  if(['c','h','cc','cpp','cxx','hh','hpp','hxx','ino'].includes(ext))options.cpp=true;
  else if(ext==='go')options.go=true;
  else if(['py','pyw'].includes(ext))options.python=true;
  else if(['js','mjs','cjs','jsx','ts','tsx'].includes(ext))options.javascript=true;
  else if(['json','json5'].includes(ext))options.json=true;
  else if(['yaml','yml'].includes(ext))options.yaml=true;
  else if(['md','markdown','mkd'].includes(ext))options.markdown=true;
  else if(['html','htm','hbs','handlebars'].includes(ext))options.html=true;
  else if(ext==='css')options.css=true;
  else if(ext==='xml'||ext==='svg')options.xml=true;
  else if(ext==='java')options.java=true;
  else if(ext==='php')options.php=true;
  else if(ext==='sql')options.sql=true;
  else if(ext==='rs')options.rust=true;
  else if(ext==='vue')options.vue=true;
  return options;
}
function languageLabel(pathValue){
  const options=languageOptions(pathValue);
  for(const key of ['cpp','go','python','javascript','json','yaml','markdown','html','css','xml','java','php','sql','rust','vue']){
    if(options[key])return key==='cpp'?'C/C++':key==='javascript'?'JS/TS':key.toUpperCase();
  }
  const lower=String(pathValue||'').toLowerCase();
  if(/(^|\/)(cmakelists\.txt)$/.test(lower)||lower.endsWith('.cmake'))return 'CMake (plain)';
  if(/\.(sh|bash|zsh|fish)$/.test(lower))return 'Shell (plain)';
  if(lower.endsWith('.lua'))return 'Lua (plain)';
  return 'Plain text';
}
function editorMetaText(file){
  const ending=(file.line_ending||'lf').toUpperCase();
  const bom=file.bom?' + BOM':'';
  return [languageLabel(file.path),'UTF-8'+bom,ending,formatBytes(file.size)].join(' • ');
}
function applyReadOnly(view){
  const readonly=Boolean(view.file.read_only);
  view.readonlyBadge.hidden=!readonly;
  view.cm.contentDOM.setAttribute('contenteditable',readonly?'false':'true');
  view.cm.contentDOM.setAttribute('aria-readonly',readonly?'true':'false');
}
function setEditorDocument(view,file){
  const currentLength=view.cm.state.doc.length;
  view.cm.dispatch({changes:{from:0,to:currentLength,insert:file.content||''}});
  view.file={...file};
  view.path.textContent=file.path;
  view.path.title=file.path;
  view.meta.textContent=editorMetaText(file);
  applyReadOnly(view);
}
function activateEditorDOM(id){
  activeEditorID=id;
  for(const [editorIDValue,view] of editors){
    const yes=editorIDValue===id;
    view.tab.classList.toggle('active',yes);
    view.pane.classList.toggle('hidden',!yes);
    if(yes)setTimeout(()=>view.cm.focus(),0);
  }
}
function activateEditor(id){
  if(!editors.has(id))return;
  app.activateExternalView(id);
}
function deactivateEditors(){
  activeEditorID='';
  for(const [,view] of editors){
    view.tab.classList.remove('active');
    view.pane.classList.add('hidden');
  }
}
function destroyEditor(id){
  const view=editors.get(id);if(!view)return;
  view.closed=true;
  try{view.cm.destroy();}catch{}
  view.tab.remove();view.pane.remove();editors.delete(id);
  if(activeEditorID===id){
    activeEditorID='';
    const next=editors.values().next();
    if(!next.done)activateEditor(next.value.id);
    else{
      const terminal=app.views.keys().next();
      if(!terminal.done)app.activateView(terminal.value);
      else app.activateExternalView('empty');
    }
  }
}
function createEditor(file){
  const id=editorID(file.path);
  const existing=editors.get(id);if(existing)return existing;

  const tab=document.createElement('button');tab.className='tab editor-tab';tab.dataset.id=id;tab.dataset.viewKind='editor';tab.type='button';tab.title=file.path;
  const label=document.createElement('span');label.className='editor-tab-label';label.textContent=basename(file.path);
  const dirty=document.createElement('span');dirty.className='editor-dirty';dirty.textContent='●';
  const close=document.createElement('span');close.className='close';close.textContent='×';close.title='Close editor';
  tab.append(label,dirty,close);tabsHost.append(tab);

  const pane=document.createElement('div');pane.className='pane editor-pane hidden';pane.dataset.id=id;pane.dataset.viewKind='editor';
  const head=document.createElement('div');head.className='pane-head editor-head';
  const pathNode=document.createElement('div');pathNode.className='editor-path';pathNode.textContent=file.path;pathNode.title=file.path;
  const meta=document.createElement('div');meta.className='editor-meta';meta.textContent=editorMetaText(file);
  const readonlyBadge=document.createElement('span');readonlyBadge.className='editor-readonly';readonlyBadge.textContent='READ-ONLY';readonlyBadge.hidden=!file.read_only;
  const reload=document.createElement('button');reload.type='button';reload.textContent='Reload';reload.title='Reload file from disk';
  head.append(pathNode,meta,readonlyBadge,reload);
  const host=document.createElement('div');host.className='editor-host';
  pane.append(head,host);panesHost.append(pane);

  const cm=cmFactory.newEditor(host,file.content||'',languageOptions(file.path));
  const view={id,file:{...file},tab,label,dirty,pane,head,path:pathNode,meta,readonlyBadge,reload,host,cm,closed:false};
  editors.set(id,view);
  applyReadOnly(view);

  tab.onclick=()=>activateEditor(id);
  close.onclick=event=>{event.stopPropagation();destroyEditor(id);};
  reload.onclick=()=>reloadEditor(view).catch(app.showError);
  return view;
}
async function reloadEditor(view){
  if(!view||view.closed)return;
  const file=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(view.file.path));
  setEditorDocument(view,file);
}
async function openFile(pathValue){
  pathValue=String(pathValue||'').trim();
  if(!pathValue)return;
  const id=editorID(pathValue);
  if(editors.has(id)){activateEditor(id);return editors.get(id);}
  if(opening.has(pathValue)){const pending=await opening.get(pathValue);activateEditor(pending.id);return pending;}
  const promise=(async()=>{
    const file=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(pathValue));
    return createEditor(file);
  })();
  opening.set(pathValue,promise);
  try{
    const view=await promise;activateEditor(view.id);return view;
  }finally{opening.delete(pathValue);}
}

window.addEventListener('taskmenu:project-file-open-request',event=>{
  const pathValue=event.detail?.path;if(pathValue)openFile(pathValue).catch(app.showError);
});
window.addEventListener('taskmenu:view-activated',event=>{
  if(event.detail?.kind==='terminal'){deactivateEditors();return;}
  if(event.detail?.kind==='external'&&editors.has(event.detail.id))activateEditorDOM(event.detail.id);
});

globalThis.TaskMenuEditor={
  editors,
  openFile,
  reloadEditor,
  activateEditor,
  destroyEditor,
  get active(){return activeEditorID;}
};
