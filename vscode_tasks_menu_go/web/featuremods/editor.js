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
.editor-head .editor-readonly,.editor-head .editor-warning{font-size:10px;padding:2px 6px;border:1px solid #7d6733;border-radius:10px;color:#ffe29a;background:#493b1d;white-space:nowrap}.editor-head .editor-warning{max-width:260px;overflow:hidden;text-overflow:ellipsis}
.editor-host{flex:1;min-height:0;overflow:hidden}
.editor-host .cm-editor{height:100%;font-size:13px}
.editor-host .cm-scroller{overflow:auto;font-family:ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace}
.cm-legacy-keyword{color:#c792ea}.cm-legacy-comment{color:#6a9955;font-style:italic}.cm-legacy-string{color:#ce9178}.cm-legacy-number{color:#b5cea8}.cm-legacy-variable{color:#9cdcfe}.cm-legacy-command{color:#dcdcaa}
.editor-tab .editor-dirty{display:none;margin-left:5px;color:#f2c96d}
.editor-tab.dirty .editor-dirty{display:inline}
.editor-tab .close{margin-left:8px}
.editor-save{background:#203f31;border-color:#3a7058;color:#dcf6e7}
.editor-dirty-backdrop{position:fixed;inset:0;z-index:7000;background:rgba(0,0,0,.55);display:flex;align-items:center;justify-content:center;padding:18px}
.editor-dirty-dialog{width:min(430px,94vw);background:#171a20;border:1px solid #3b414d;border-radius:10px;box-shadow:0 18px 48px rgba(0,0,0,.5);padding:16px}
.editor-dirty-dialog h3{margin:0 0 8px;font-size:15px}.editor-dirty-dialog p{margin:0 0 14px;font-size:12px;opacity:.75;word-break:break-word}
.editor-dirty-actions{display:flex;justify-content:flex-end;gap:8px}.editor-dirty-actions .discard{margin-right:auto;background:#4a252a;border-color:#7a4048;color:#ffe2e4}
.editor-conflict-dialog{width:min(560px,96vw)}
.editor-conflict-actions{display:flex;justify-content:flex-end;gap:8px;flex-wrap:wrap}.editor-conflict-actions .compare{margin-right:auto}
.editor-compare-dialog{width:min(1100px,96vw);max-height:90vh;display:flex;flex-direction:column}
.editor-compare-grid{display:grid;grid-template-columns:1fr 1fr;gap:10px;min-height:0}.editor-compare-side{min-width:0;display:flex;flex-direction:column;gap:5px}
.editor-compare-label{font-size:11px;font-weight:600;opacity:.75}.editor-compare-text{margin:0;min-height:180px;max-height:62vh;overflow:auto;white-space:pre;tab-size:4;background:#0d1117;border:1px solid #343a45;border-radius:6px;padding:9px;font:12px/1.45 ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace}
@media(max-width:760px){.editor-compare-grid{grid-template-columns:1fr}.editor-compare-text{max-height:28vh}}
html[data-taskmenu-theme="light"] .editor-pane{background:#fff}
html[data-taskmenu-theme="light"] .editor-head .editor-readonly{color:#6c5314;background:#fff6d9;border-color:#c9ab61}
html[data-taskmenu-theme="light"] .editor-dirty-dialog{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .editor-compare-text{background:#f8f9fb;border-color:#c8ced6}
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
const legacyKeywordSets={
  shell:new Set('if then else elif fi for while until do done case esac in function select time coproc readonly local export declare typeset unset shift break continue return'.split(' ')),
  cmake:new Set('if elseif else endif foreach endforeach while endwhile function endfunction macro endmacro return break continue'.split(' ')),
  lua:new Set('and break do else elseif end false for function goto if in local nil not or repeat return then true until while'.split(' '))
};
const legacyMarks=new Map();
function legacyHighlightKind(pathValue){
  const lower=String(pathValue||'').toLowerCase();
  const name=basename(lower);
  const ext=(name.includes('.')?name.slice(name.lastIndexOf('.')+1):'');
  if(name==='cmakelists.txt'||ext==='cmake')return 'cmake';
  if(['sh','bash','zsh','fish','ksh'].includes(ext)||name==='.bashrc'||name==='.zshrc')return 'shell';
  if(ext==='lua')return 'lua';
  return '';
}
function legacyMark(type){
  if(!legacyMarks.has(type))legacyMarks.set(type,globalThis.cm6.Decoration.mark({class:'cm-legacy-'+type}));
  return legacyMarks.get(type);
}
function legacyLineTokens(kind,text){
  const tokens=[];const keywords=legacyKeywordSets[kind]||new Set();let i=0;let firstWord=true;
  const identStart=ch=>/[A-Za-z_]/.test(ch);
  const identPart=ch=>/[A-Za-z0-9_]/.test(ch);
  while(i<text.length){
    const ch=text[i],next=text[i+1]||'';
    if((kind==='lua'&&ch==='-'&&next==='-')||(kind!=='lua'&&ch==='#')){tokens.push({from:i,to:text.length,type:'comment'});break;}
    if(ch==='"'||ch==="'"){
      const quote=ch;let j=i+1;
      while(j<text.length){if(text[j]==='\\'){j+=2;continue;}if(text[j]===quote){j++;break;}j++;}
      tokens.push({from:i,to:j,type:'string'});i=j;firstWord=false;continue;
    }
    if(kind==='shell'&&ch==='
function languageLabel(pathValue){
  const options=languageOptions(pathValue);
  for(const key of ['cpp','go','python','javascript','json','yaml','markdown','html','css','xml','java','php','sql','rust','vue']){
    if(options[key])return key==='cpp'?'C/C++':key==='javascript'?'JS/TS':key.toUpperCase();
  }
  const lower=String(pathValue||'').toLowerCase();
  if(/(^|\/)(cmakelists\.txt)$/.test(lower)||lower.endsWith('.cmake'))return 'CMake';
  if(/\.(sh|bash|zsh|fish|ksh)$/.test(lower)||/(^|\/)(\.bashrc|\.zshrc)$/.test(lower))return 'Shell';
  if(lower.endsWith('.lua'))return 'Lua';
  return 'Plain text';
}
function editorMetaText(file){
  const ending=(file.line_ending||'lf').toUpperCase();
  const bom=file.bom?' + BOM':'';
  return [languageLabel(file.path),'UTF-8'+bom,ending,formatBytes(file.size)].join(' • ');
}
function setDirty(view,dirty){
  if(!view||view.closed)return;
  view.dirty=Boolean(dirty);
  view.tab.classList.toggle('dirty',view.dirty);
  view.tab.title=(view.dirty?'● ':'')+view.file.path;
  view.save.disabled=Boolean(view.file.read_only)||!view.dirty||view.saving;
}
function applyReadOnly(view){
  const readonly=Boolean(view.file.read_only);
  view.readonlyBadge.hidden=!readonly;
  view.cm.contentDOM.setAttribute('contenteditable',readonly?'false':'true');
  view.cm.contentDOM.setAttribute('aria-readonly',readonly?'true':'false');
  view.save.disabled=readonly||!view.dirty||view.saving;
}
function installEditorDispatchGuard(view){
  const originalDispatch=view.cm.dispatch.bind(view.cm);
  view.dispatchRaw=originalDispatch;
  view.cm.dispatch=(...input)=>{
    const transaction=input.length===1&&input[0]?.startState
      ? input[0]
      : view.cm.state.update(...input);
    if(transaction.docChanged&&view.file.read_only&&!view.internalUpdate)return;
    originalDispatch(transaction);
    if(transaction.docChanged&&!view.internalUpdate)setDirty(view,true);
  };
}
function setEditorDocument(view,file){
  const currentLength=view.cm.state.doc.length;
  view.internalUpdate=true;
  try{
    view.cm.dispatch({changes:{from:0,to:currentLength,insert:file.content||''}});
  }finally{
    view.internalUpdate=false;
  }
  view.file={...file};
  view.path.textContent=file.path;
  view.path.title=file.path;
  view.meta.textContent=editorMetaText(file);
  view.warningBadge.hidden=!file.warning;
  view.warningBadge.textContent=file.warning?'WARNING':'';
  view.warningBadge.title=file.warning||'';
  applyReadOnly(view);
  setDirty(view,false);
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
function dirtyCloseChoice(view){
  if(!view?.dirty)return Promise.resolve('discard');
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
    const dialog=document.createElement('div');dialog.className='editor-dirty-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='Save changes?';
    const message=document.createElement('p');message.textContent=view.file.path+' has unsaved changes.';
    const actions=document.createElement('div');actions.className='editor-dirty-actions';
    const discard=document.createElement('button');discard.type='button';discard.className='discard';discard.textContent='Discard';
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
    const save=document.createElement('button');save.type='button';save.className='editor-save';save.textContent='Save';
    actions.append(discard,cancel,save);dialog.append(title,message,actions);backdrop.append(dialog);document.body.append(backdrop);
    let done=false;
    const finish=value=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();resolve(value);};
    const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish('cancel');}};
    document.addEventListener('keydown',onKey,true);
    backdrop.onmousedown=event=>{if(event.target===backdrop)finish('cancel');};
    discard.onclick=()=>finish('discard');cancel.onclick=()=>finish('cancel');save.onclick=()=>finish('save');
    save.focus();
  });
}
async function putEditorFile(view,expectedSHA256){
  const response=await fetch('/api/project/file',{
    method:'PUT',
    cache:'no-store',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({
      path:view.file.path,
      content:view.cm.state.doc.toString(),
      expected_sha256:expectedSHA256
    })
  });
  if(response.status===409)return {conflict:true};
  if(!response.ok)throw new Error((await response.text())||response.statusText);
  return {file:await response.json()};
}
async function readLatestEditorFile(view){
  return app.jsonFetch('/api/project/file?path='+encodeURIComponent(view.file.path));
}
function conflictChoice(view){
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
    const dialog=document.createElement('div');dialog.className='editor-dirty-dialog editor-conflict-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='File changed outside the editor.';
    const message=document.createElement('p');message.textContent='Choose how to resolve '+view.file.path+'. Your unsaved editor text will not be overwritten unless you choose Reload.';
    const actions=document.createElement('div');actions.className='editor-conflict-actions';
    const compare=document.createElement('button');compare.type='button';compare.className='compare';compare.textContent='Compare';
    const reload=document.createElement('button');reload.type='button';reload.textContent='Reload';
    const overwrite=document.createElement('button');overwrite.type='button';overwrite.className='editor-save';overwrite.textContent='Overwrite';
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
    actions.append(compare,reload,overwrite,cancel);dialog.append(title,message,actions);backdrop.append(dialog);document.body.append(backdrop);
    let done=false;
    const finish=value=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();resolve(value);};
    const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish('cancel');}};
    document.addEventListener('keydown',onKey,true);
    backdrop.onmousedown=event=>{if(event.target===backdrop)finish('cancel');};
    compare.onclick=()=>finish('compare');reload.onclick=()=>finish('reload');overwrite.onclick=()=>finish('overwrite');cancel.onclick=()=>finish('cancel');
    cancel.focus();
  });
}
function showConflictCompare(view,latest){
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
    const dialog=document.createElement('div');dialog.className='editor-dirty-dialog editor-compare-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='Compare changes';
    const message=document.createElement('p');message.textContent=view.file.path+' — editor copy versus current file on disk.';
    const grid=document.createElement('div');grid.className='editor-compare-grid';
    const makeSide=(label,text)=>{
      const side=document.createElement('div');side.className='editor-compare-side';
      const head=document.createElement('div');head.className='editor-compare-label';head.textContent=label;
      const pre=document.createElement('pre');pre.className='editor-compare-text';pre.textContent=text;
      side.append(head,pre);return side;
    };
    grid.append(makeSide('Editor (unsaved)',view.cm.state.doc.toString()),makeSide('Disk (latest)',latest.content||''));
    const actions=document.createElement('div');actions.className='editor-dirty-actions';
    const close=document.createElement('button');close.type='button';close.textContent='Back';actions.append(close);
    dialog.append(title,message,grid,actions);backdrop.append(dialog);document.body.append(backdrop);
    let done=false;
    const finish=()=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();resolve();};
    const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish();}};
    document.addEventListener('keydown',onKey,true);
    backdrop.onmousedown=event=>{if(event.target===backdrop)finish();};close.onclick=finish;close.focus();
  });
}
async function resolveSaveConflict(view){
  while(!view.closed){
    const latest=await readLatestEditorFile(view);
    const choice=await conflictChoice(view);
    if(choice==='cancel')return null;
    if(choice==='compare'){await showConflictCompare(view,latest);continue;}
    if(choice==='reload'){setEditorDocument(view,latest);return latest;}
    if(choice==='overwrite'){
      const result=await putEditorFile(view,latest.sha256);
      if(result.conflict)continue;
      setEditorDocument(view,result.file);
      return result.file;
    }
  }
  return null;
}
async function saveEditor(view){
  if(!view||view.closed||view.file.read_only||!view.dirty||view.saving)return view?.file||null;
  view.saving=true;view.save.disabled=true;view.save.textContent='Saving…';
  try{
    const result=await putEditorFile(view,view.file.sha256);
    if(result.conflict)return resolveSaveConflict(view);
    setEditorDocument(view,result.file);
    return result.file;
  }finally{
    view.saving=false;
    if(!view.closed){
      view.save.textContent='Save';
      view.save.disabled=Boolean(view.file.read_only)||!view.dirty;
    }
  }
}
async function closeEditor(id){
  const view=editors.get(id);if(!view)return false;
  if(view.dirty){
    const choice=await dirtyCloseChoice(view);
    if(choice==='cancel')return false;
    if(choice==='save'){
      await saveEditor(view);
      if(view.dirty)return false;
    }
  }
  destroyEditor(id);
  return true;
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
  const warningBadge=document.createElement('span');warningBadge.className='editor-warning';warningBadge.textContent=file.warning?'WARNING':'';warningBadge.title=file.warning||'';warningBadge.hidden=!file.warning;
  const save=document.createElement('button');save.type='button';save.className='editor-save';save.textContent='Save';save.title='Save file (Ctrl/Cmd+S)';
  const reload=document.createElement('button');reload.type='button';reload.textContent='Reload';reload.title='Reload file from disk';
  head.append(pathNode,meta,readonlyBadge,warningBadge,save,reload);
  const host=document.createElement('div');host.className='editor-host';
  pane.append(head,host);panesHost.append(pane);

  const cm=cmFactory.newEditor(host,file.content||'',languageOptions(file.path));
  const view={id,file:{...file},tab,label,dirty,pane,head,path:pathNode,meta,readonlyBadge,warningBadge,save,reload,host,cm,closed:false,dirty:false,saving:false,internalUpdate:false,dispatchRaw:null};
  editors.set(id,view);
  installEditorDispatchGuard(view);
  applyReadOnly(view);
  setDirty(view,false);

  cm.contentDOM.addEventListener('beforeinput',event=>{if(view.file.read_only)event.preventDefault();},true);
  cm.contentDOM.addEventListener('keydown',event=>{
    if(event.key==='Tab'&&!event.ctrlKey&&!event.metaKey&&!event.altKey&&!event.shiftKey&&!view.file.read_only){
      event.preventDefault();
      view.cm.dispatch(view.cm.state.replaceSelection('\t'));
    }
  },true);
  tab.onclick=()=>activateEditor(id);
  close.onclick=event=>{event.stopPropagation();closeEditor(id).catch(app.showError);};
  save.onclick=()=>saveEditor(view).catch(app.showError);
  reload.onclick=()=>reloadEditor(view).catch(app.showError);
  return view;
}
async function reloadEditor(view){
  if(!view||view.closed)return;
  if(view.dirty&&!window.confirm('Discard unsaved changes and reload '+view.file.path+'?'))return;
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


function goToLine(view){
  if(!view||view.closed)return;
  const raw=window.prompt('Go to line (1-'+view.cm.state.doc.lines+'):','1');
  if(raw===null)return;
  const lineNumber=Math.max(1,Math.min(view.cm.state.doc.lines,Number.parseInt(raw,10)||1));
  const line=view.cm.state.doc.line(lineNumber);
  view.cm.dispatch({selection:{anchor:line.from},scrollIntoView:true});
  view.cm.focus();
}

document.addEventListener('keydown',event=>{
  if(!(event.ctrlKey||event.metaKey)||!activeEditorID)return;
  const view=editors.get(activeEditorID);if(!view||view.closed)return;
  const key=event.key.toLowerCase();
  if(key==='s'){
    event.preventDefault();
    saveEditor(view).catch(app.showError);
  }else if(key==='g'){
    event.preventDefault();
    goToLine(view);
  }
});

window.addEventListener('beforeunload',event=>{
  if(![...editors.values()].some(view=>!view.closed&&view.dirty))return;
  event.preventDefault();
  event.returnValue='';
});

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
  saveEditor,
  closeEditor,
  goToLine,
  activateEditor,
  destroyEditor,
  get active(){return activeEditorID;}
};
){
      let j=i+1;
      if(text[j]==='{'){j++;while(j<text.length&&text[j]!=='}')j++;if(j<text.length)j++;}
      else while(j<text.length&&/[A-Za-z0-9_@*#?$!\-]/.test(text[j]))j++;
      if(j>i+1){tokens.push({from:i,to:j,type:'variable'});i=j;continue;}
    }
    if(kind==='cmake'&&ch==='
function languageLabel(pathValue){
  const options=languageOptions(pathValue);
  for(const key of ['cpp','go','python','javascript','json','yaml','markdown','html','css','xml','java','php','sql','rust','vue']){
    if(options[key])return key==='cpp'?'C/C++':key==='javascript'?'JS/TS':key.toUpperCase();
  }
  const lower=String(pathValue||'').toLowerCase();
  if(/(^|\/)(cmakelists\.txt)$/.test(lower)||lower.endsWith('.cmake'))return 'CMake';
  if(/\.(sh|bash|zsh|fish)$/.test(lower))return 'Shell';
  if(lower.endsWith('.lua'))undefined
  return 'Plain text';
}
function editorMetaText(file){
  const ending=(file.line_ending||'lf').toUpperCase();
  const bom=file.bom?' + BOM':'';
  return [languageLabel(file.path),'UTF-8'+bom,ending,formatBytes(file.size)].join(' • ');
}
function setDirty(view,dirty){
  if(!view||view.closed)return;
  view.dirty=Boolean(dirty);
  view.tab.classList.toggle('dirty',view.dirty);
  view.tab.title=(view.dirty?'● ':'')+view.file.path;
  view.save.disabled=Boolean(view.file.read_only)||!view.dirty||view.saving;
}
function applyReadOnly(view){
  const readonly=Boolean(view.file.read_only);
  view.readonlyBadge.hidden=!readonly;
  view.cm.contentDOM.setAttribute('contenteditable',readonly?'false':'true');
  view.cm.contentDOM.setAttribute('aria-readonly',readonly?'true':'false');
  view.save.disabled=readonly||!view.dirty||view.saving;
}
function installEditorDispatchGuard(view){
  const originalDispatch=view.cm.dispatch.bind(view.cm);
  view.dispatchRaw=originalDispatch;
  view.cm.dispatch=(...input)=>{
    const transaction=input.length===1&&input[0]?.startState
      ? input[0]
      : view.cm.state.update(...input);
    if(transaction.docChanged&&view.file.read_only&&!view.internalUpdate)return;
    originalDispatch(transaction);
    if(transaction.docChanged&&!view.internalUpdate)setDirty(view,true);
  };
}
function setEditorDocument(view,file){
  const currentLength=view.cm.state.doc.length;
  view.internalUpdate=true;
  try{
    view.cm.dispatch({changes:{from:0,to:currentLength,insert:file.content||''}});
  }finally{
    view.internalUpdate=false;
  }
  view.file={...file};
  view.path.textContent=file.path;
  view.path.title=file.path;
  view.meta.textContent=editorMetaText(file);
  view.warningBadge.hidden=!file.warning;
  view.warningBadge.textContent=file.warning?'WARNING':'';
  view.warningBadge.title=file.warning||'';
  applyReadOnly(view);
  setDirty(view,false);
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
function dirtyCloseChoice(view){
  if(!view?.dirty)return Promise.resolve('discard');
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
    const dialog=document.createElement('div');dialog.className='editor-dirty-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='Save changes?';
    const message=document.createElement('p');message.textContent=view.file.path+' has unsaved changes.';
    const actions=document.createElement('div');actions.className='editor-dirty-actions';
    const discard=document.createElement('button');discard.type='button';discard.className='discard';discard.textContent='Discard';
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
    const save=document.createElement('button');save.type='button';save.className='editor-save';save.textContent='Save';
    actions.append(discard,cancel,save);dialog.append(title,message,actions);backdrop.append(dialog);document.body.append(backdrop);
    let done=false;
    const finish=value=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();resolve(value);};
    const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish('cancel');}};
    document.addEventListener('keydown',onKey,true);
    backdrop.onmousedown=event=>{if(event.target===backdrop)finish('cancel');};
    discard.onclick=()=>finish('discard');cancel.onclick=()=>finish('cancel');save.onclick=()=>finish('save');
    save.focus();
  });
}
async function putEditorFile(view,expectedSHA256){
  const response=await fetch('/api/project/file',{
    method:'PUT',
    cache:'no-store',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({
      path:view.file.path,
      content:view.cm.state.doc.toString(),
      expected_sha256:expectedSHA256
    })
  });
  if(response.status===409)return {conflict:true};
  if(!response.ok)throw new Error((await response.text())||response.statusText);
  return {file:await response.json()};
}
async function readLatestEditorFile(view){
  return app.jsonFetch('/api/project/file?path='+encodeURIComponent(view.file.path));
}
function conflictChoice(view){
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
    const dialog=document.createElement('div');dialog.className='editor-dirty-dialog editor-conflict-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='File changed outside the editor.';
    const message=document.createElement('p');message.textContent='Choose how to resolve '+view.file.path+'. Your unsaved editor text will not be overwritten unless you choose Reload.';
    const actions=document.createElement('div');actions.className='editor-conflict-actions';
    const compare=document.createElement('button');compare.type='button';compare.className='compare';compare.textContent='Compare';
    const reload=document.createElement('button');reload.type='button';reload.textContent='Reload';
    const overwrite=document.createElement('button');overwrite.type='button';overwrite.className='editor-save';overwrite.textContent='Overwrite';
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
    actions.append(compare,reload,overwrite,cancel);dialog.append(title,message,actions);backdrop.append(dialog);document.body.append(backdrop);
    let done=false;
    const finish=value=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();resolve(value);};
    const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish('cancel');}};
    document.addEventListener('keydown',onKey,true);
    backdrop.onmousedown=event=>{if(event.target===backdrop)finish('cancel');};
    compare.onclick=()=>finish('compare');reload.onclick=()=>finish('reload');overwrite.onclick=()=>finish('overwrite');cancel.onclick=()=>finish('cancel');
    cancel.focus();
  });
}
function showConflictCompare(view,latest){
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
    const dialog=document.createElement('div');dialog.className='editor-dirty-dialog editor-compare-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='Compare changes';
    const message=document.createElement('p');message.textContent=view.file.path+' — editor copy versus current file on disk.';
    const grid=document.createElement('div');grid.className='editor-compare-grid';
    const makeSide=(label,text)=>{
      const side=document.createElement('div');side.className='editor-compare-side';
      const head=document.createElement('div');head.className='editor-compare-label';head.textContent=label;
      const pre=document.createElement('pre');pre.className='editor-compare-text';pre.textContent=text;
      side.append(head,pre);return side;
    };
    grid.append(makeSide('Editor (unsaved)',view.cm.state.doc.toString()),makeSide('Disk (latest)',latest.content||''));
    const actions=document.createElement('div');actions.className='editor-dirty-actions';
    const close=document.createElement('button');close.type='button';close.textContent='Back';actions.append(close);
    dialog.append(title,message,grid,actions);backdrop.append(dialog);document.body.append(backdrop);
    let done=false;
    const finish=()=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();resolve();};
    const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish();}};
    document.addEventListener('keydown',onKey,true);
    backdrop.onmousedown=event=>{if(event.target===backdrop)finish();};close.onclick=finish;close.focus();
  });
}
async function resolveSaveConflict(view){
  while(!view.closed){
    const latest=await readLatestEditorFile(view);
    const choice=await conflictChoice(view);
    if(choice==='cancel')return null;
    if(choice==='compare'){await showConflictCompare(view,latest);continue;}
    if(choice==='reload'){setEditorDocument(view,latest);return latest;}
    if(choice==='overwrite'){
      const result=await putEditorFile(view,latest.sha256);
      if(result.conflict)continue;
      setEditorDocument(view,result.file);
      return result.file;
    }
  }
  return null;
}
async function saveEditor(view){
  if(!view||view.closed||view.file.read_only||!view.dirty||view.saving)return view?.file||null;
  view.saving=true;view.save.disabled=true;view.save.textContent='Saving…';
  try{
    const result=await putEditorFile(view,view.file.sha256);
    if(result.conflict)return resolveSaveConflict(view);
    setEditorDocument(view,result.file);
    return result.file;
  }finally{
    view.saving=false;
    if(!view.closed){
      view.save.textContent='Save';
      view.save.disabled=Boolean(view.file.read_only)||!view.dirty;
    }
  }
}
async function closeEditor(id){
  const view=editors.get(id);if(!view)return false;
  if(view.dirty){
    const choice=await dirtyCloseChoice(view);
    if(choice==='cancel')return false;
    if(choice==='save'){
      await saveEditor(view);
      if(view.dirty)return false;
    }
  }
  destroyEditor(id);
  return true;
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
  const warningBadge=document.createElement('span');warningBadge.className='editor-warning';warningBadge.textContent=file.warning?'WARNING':'';warningBadge.title=file.warning||'';warningBadge.hidden=!file.warning;
  const save=document.createElement('button');save.type='button';save.className='editor-save';save.textContent='Save';save.title='Save file (Ctrl/Cmd+S)';
  const reload=document.createElement('button');reload.type='button';reload.textContent='Reload';reload.title='Reload file from disk';
  head.append(pathNode,meta,readonlyBadge,warningBadge,save,reload);
  const host=document.createElement('div');host.className='editor-host';
  pane.append(head,host);panesHost.append(pane);

  const cm=cmFactory.newEditor(host,file.content||'',languageOptions(file.path));
  const view={id,file:{...file},tab,label,dirty,pane,head,path:pathNode,meta,readonlyBadge,warningBadge,save,reload,host,cm,closed:false,dirty:false,saving:false,internalUpdate:false,dispatchRaw:null};
  editors.set(id,view);
  installEditorDispatchGuard(view);
  applyReadOnly(view);
  setDirty(view,false);

  cm.contentDOM.addEventListener('beforeinput',event=>{if(view.file.read_only)event.preventDefault();},true);
  cm.contentDOM.addEventListener('keydown',event=>{
    if(event.key==='Tab'&&!event.ctrlKey&&!event.metaKey&&!event.altKey&&!event.shiftKey&&!view.file.read_only){
      event.preventDefault();
      view.cm.dispatch(view.cm.state.replaceSelection('\t'));
    }
  },true);
  tab.onclick=()=>activateEditor(id);
  close.onclick=event=>{event.stopPropagation();closeEditor(id).catch(app.showError);};
  save.onclick=()=>saveEditor(view).catch(app.showError);
  reload.onclick=()=>reloadEditor(view).catch(app.showError);
  return view;
}
async function reloadEditor(view){
  if(!view||view.closed)return;
  if(view.dirty&&!window.confirm('Discard unsaved changes and reload '+view.file.path+'?'))return;
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


function goToLine(view){
  if(!view||view.closed)return;
  const raw=window.prompt('Go to line (1-'+view.cm.state.doc.lines+'):','1');
  if(raw===null)return;
  const lineNumber=Math.max(1,Math.min(view.cm.state.doc.lines,Number.parseInt(raw,10)||1));
  const line=view.cm.state.doc.line(lineNumber);
  view.cm.dispatch({selection:{anchor:line.from},scrollIntoView:true});
  view.cm.focus();
}

document.addEventListener('keydown',event=>{
  if(!(event.ctrlKey||event.metaKey)||!activeEditorID)return;
  const view=editors.get(activeEditorID);if(!view||view.closed)return;
  const key=event.key.toLowerCase();
  if(key==='s'){
    event.preventDefault();
    saveEditor(view).catch(app.showError);
  }else if(key==='g'){
    event.preventDefault();
    goToLine(view);
  }
});

window.addEventListener('beforeunload',event=>{
  if(![...editors.values()].some(view=>!view.closed&&view.dirty))return;
  event.preventDefault();
  event.returnValue='';
});

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
  saveEditor,
  closeEditor,
  goToLine,
  activateEditor,
  destroyEditor,
  get active(){return activeEditorID;}
};
&&next==='{'){
      let j=i+2;while(j<text.length&&text[j]!=='}')j++;if(j<text.length)j++;
      tokens.push({from:i,to:j,type:'variable'});i=j;continue;
    }
    if(/[0-9]/.test(ch)){
      let j=i+1;while(j<text.length&&/[0-9A-Fa-fxX._]/.test(text[j]))j++;
      tokens.push({from:i,to:j,type:'number'});i=j;firstWord=false;continue;
    }
    if(identStart(ch)){
      let j=i+1;while(j<text.length&&identPart(text[j]))j++;
      const word=text.slice(i,j);const lower=word.toLowerCase();
      let type='';
      if(keywords.has(lower))type='keyword';
      else if(kind==='cmake'&&/^\s*\(/.test(text.slice(j)))type='command';
      else if(kind==='shell'&&firstWord)type='command';
      if(type)tokens.push({from:i,to:j,type});
      i=j;firstWord=false;continue;
    }
    if(!/\s/.test(ch))firstWord=false;
    i++;
  }
  return tokens;
}
function buildLegacyDecorations(view,kind){
  const builder=new globalThis.cm6.RangeSetBuilder();
  for(const range of view.visibleRanges){
    let pos=range.from;
    while(pos<=range.to){
      const line=view.state.doc.lineAt(pos);
      const text=line.text;
      for(const token of legacyLineTokens(kind,text)){
        const from=line.from+token.from,to=line.from+token.to;
        if(to<=range.from||from>=range.to||to<=from)continue;
        builder.add(Math.max(from,range.from),Math.min(to,range.to),legacyMark(token.type));
      }
      if(line.to>=range.to)break;
      pos=line.to+1;
    }
  }
  return builder.finish();
}
function legacySyntaxExtension(kind){
  return globalThis.cm6.ViewPlugin.fromClass(class{
    constructor(view){this.decorations=buildLegacyDecorations(view,kind);}
    update(update){if(update.docChanged||update.viewportChanged)this.decorations=buildLegacyDecorations(update.view,kind);}
  },{decorations:value=>value.decorations});
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
  else if(['js','mjs','cjs'].includes(ext))options.javascript=true;
  else if(ext==='jsx')options.jsx=true;
  else if(ext==='ts')options.typescript=true;
  else if(ext==='tsx')options.tsx=true;
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
  const legacy=legacyHighlightKind(pathValue);
  if(legacy)options.extraExtensions=[legacySyntaxExtension(legacy)];
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
function setDirty(view,dirty){
  if(!view||view.closed)return;
  view.dirty=Boolean(dirty);
  view.tab.classList.toggle('dirty',view.dirty);
  view.tab.title=(view.dirty?'● ':'')+view.file.path;
  view.save.disabled=Boolean(view.file.read_only)||!view.dirty||view.saving;
}
function applyReadOnly(view){
  const readonly=Boolean(view.file.read_only);
  view.readonlyBadge.hidden=!readonly;
  view.cm.contentDOM.setAttribute('contenteditable',readonly?'false':'true');
  view.cm.contentDOM.setAttribute('aria-readonly',readonly?'true':'false');
  view.save.disabled=readonly||!view.dirty||view.saving;
}
function installEditorDispatchGuard(view){
  const originalDispatch=view.cm.dispatch.bind(view.cm);
  view.dispatchRaw=originalDispatch;
  view.cm.dispatch=(...input)=>{
    const transaction=input.length===1&&input[0]?.startState
      ? input[0]
      : view.cm.state.update(...input);
    if(transaction.docChanged&&view.file.read_only&&!view.internalUpdate)return;
    originalDispatch(transaction);
    if(transaction.docChanged&&!view.internalUpdate)setDirty(view,true);
  };
}
function setEditorDocument(view,file){
  const currentLength=view.cm.state.doc.length;
  view.internalUpdate=true;
  try{
    view.cm.dispatch({changes:{from:0,to:currentLength,insert:file.content||''}});
  }finally{
    view.internalUpdate=false;
  }
  view.file={...file};
  view.path.textContent=file.path;
  view.path.title=file.path;
  view.meta.textContent=editorMetaText(file);
  view.warningBadge.hidden=!file.warning;
  view.warningBadge.textContent=file.warning?'WARNING':'';
  view.warningBadge.title=file.warning||'';
  applyReadOnly(view);
  setDirty(view,false);
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
function dirtyCloseChoice(view){
  if(!view?.dirty)return Promise.resolve('discard');
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
    const dialog=document.createElement('div');dialog.className='editor-dirty-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='Save changes?';
    const message=document.createElement('p');message.textContent=view.file.path+' has unsaved changes.';
    const actions=document.createElement('div');actions.className='editor-dirty-actions';
    const discard=document.createElement('button');discard.type='button';discard.className='discard';discard.textContent='Discard';
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
    const save=document.createElement('button');save.type='button';save.className='editor-save';save.textContent='Save';
    actions.append(discard,cancel,save);dialog.append(title,message,actions);backdrop.append(dialog);document.body.append(backdrop);
    let done=false;
    const finish=value=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();resolve(value);};
    const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish('cancel');}};
    document.addEventListener('keydown',onKey,true);
    backdrop.onmousedown=event=>{if(event.target===backdrop)finish('cancel');};
    discard.onclick=()=>finish('discard');cancel.onclick=()=>finish('cancel');save.onclick=()=>finish('save');
    save.focus();
  });
}
async function putEditorFile(view,expectedSHA256){
  const response=await fetch('/api/project/file',{
    method:'PUT',
    cache:'no-store',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({
      path:view.file.path,
      content:view.cm.state.doc.toString(),
      expected_sha256:expectedSHA256
    })
  });
  if(response.status===409)return {conflict:true};
  if(!response.ok)throw new Error((await response.text())||response.statusText);
  return {file:await response.json()};
}
async function readLatestEditorFile(view){
  return app.jsonFetch('/api/project/file?path='+encodeURIComponent(view.file.path));
}
function conflictChoice(view){
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
    const dialog=document.createElement('div');dialog.className='editor-dirty-dialog editor-conflict-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='File changed outside the editor.';
    const message=document.createElement('p');message.textContent='Choose how to resolve '+view.file.path+'. Your unsaved editor text will not be overwritten unless you choose Reload.';
    const actions=document.createElement('div');actions.className='editor-conflict-actions';
    const compare=document.createElement('button');compare.type='button';compare.className='compare';compare.textContent='Compare';
    const reload=document.createElement('button');reload.type='button';reload.textContent='Reload';
    const overwrite=document.createElement('button');overwrite.type='button';overwrite.className='editor-save';overwrite.textContent='Overwrite';
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
    actions.append(compare,reload,overwrite,cancel);dialog.append(title,message,actions);backdrop.append(dialog);document.body.append(backdrop);
    let done=false;
    const finish=value=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();resolve(value);};
    const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish('cancel');}};
    document.addEventListener('keydown',onKey,true);
    backdrop.onmousedown=event=>{if(event.target===backdrop)finish('cancel');};
    compare.onclick=()=>finish('compare');reload.onclick=()=>finish('reload');overwrite.onclick=()=>finish('overwrite');cancel.onclick=()=>finish('cancel');
    cancel.focus();
  });
}
function showConflictCompare(view,latest){
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
    const dialog=document.createElement('div');dialog.className='editor-dirty-dialog editor-compare-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='Compare changes';
    const message=document.createElement('p');message.textContent=view.file.path+' — editor copy versus current file on disk.';
    const grid=document.createElement('div');grid.className='editor-compare-grid';
    const makeSide=(label,text)=>{
      const side=document.createElement('div');side.className='editor-compare-side';
      const head=document.createElement('div');head.className='editor-compare-label';head.textContent=label;
      const pre=document.createElement('pre');pre.className='editor-compare-text';pre.textContent=text;
      side.append(head,pre);return side;
    };
    grid.append(makeSide('Editor (unsaved)',view.cm.state.doc.toString()),makeSide('Disk (latest)',latest.content||''));
    const actions=document.createElement('div');actions.className='editor-dirty-actions';
    const close=document.createElement('button');close.type='button';close.textContent='Back';actions.append(close);
    dialog.append(title,message,grid,actions);backdrop.append(dialog);document.body.append(backdrop);
    let done=false;
    const finish=()=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();resolve();};
    const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish();}};
    document.addEventListener('keydown',onKey,true);
    backdrop.onmousedown=event=>{if(event.target===backdrop)finish();};close.onclick=finish;close.focus();
  });
}
async function resolveSaveConflict(view){
  while(!view.closed){
    const latest=await readLatestEditorFile(view);
    const choice=await conflictChoice(view);
    if(choice==='cancel')return null;
    if(choice==='compare'){await showConflictCompare(view,latest);continue;}
    if(choice==='reload'){setEditorDocument(view,latest);return latest;}
    if(choice==='overwrite'){
      const result=await putEditorFile(view,latest.sha256);
      if(result.conflict)continue;
      setEditorDocument(view,result.file);
      return result.file;
    }
  }
  return null;
}
async function saveEditor(view){
  if(!view||view.closed||view.file.read_only||!view.dirty||view.saving)return view?.file||null;
  view.saving=true;view.save.disabled=true;view.save.textContent='Saving…';
  try{
    const result=await putEditorFile(view,view.file.sha256);
    if(result.conflict)return resolveSaveConflict(view);
    setEditorDocument(view,result.file);
    return result.file;
  }finally{
    view.saving=false;
    if(!view.closed){
      view.save.textContent='Save';
      view.save.disabled=Boolean(view.file.read_only)||!view.dirty;
    }
  }
}
async function closeEditor(id){
  const view=editors.get(id);if(!view)return false;
  if(view.dirty){
    const choice=await dirtyCloseChoice(view);
    if(choice==='cancel')return false;
    if(choice==='save'){
      await saveEditor(view);
      if(view.dirty)return false;
    }
  }
  destroyEditor(id);
  return true;
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
  const warningBadge=document.createElement('span');warningBadge.className='editor-warning';warningBadge.textContent=file.warning?'WARNING':'';warningBadge.title=file.warning||'';warningBadge.hidden=!file.warning;
  const save=document.createElement('button');save.type='button';save.className='editor-save';save.textContent='Save';save.title='Save file (Ctrl/Cmd+S)';
  const reload=document.createElement('button');reload.type='button';reload.textContent='Reload';reload.title='Reload file from disk';
  head.append(pathNode,meta,readonlyBadge,warningBadge,save,reload);
  const host=document.createElement('div');host.className='editor-host';
  pane.append(head,host);panesHost.append(pane);

  const cm=cmFactory.newEditor(host,file.content||'',languageOptions(file.path));
  const view={id,file:{...file},tab,label,dirty,pane,head,path:pathNode,meta,readonlyBadge,warningBadge,save,reload,host,cm,closed:false,dirty:false,saving:false,internalUpdate:false,dispatchRaw:null};
  editors.set(id,view);
  installEditorDispatchGuard(view);
  applyReadOnly(view);
  setDirty(view,false);

  cm.contentDOM.addEventListener('beforeinput',event=>{if(view.file.read_only)event.preventDefault();},true);
  cm.contentDOM.addEventListener('keydown',event=>{
    if(event.key==='Tab'&&!event.ctrlKey&&!event.metaKey&&!event.altKey&&!event.shiftKey&&!view.file.read_only){
      event.preventDefault();
      view.cm.dispatch(view.cm.state.replaceSelection('\t'));
    }
  },true);
  tab.onclick=()=>activateEditor(id);
  close.onclick=event=>{event.stopPropagation();closeEditor(id).catch(app.showError);};
  save.onclick=()=>saveEditor(view).catch(app.showError);
  reload.onclick=()=>reloadEditor(view).catch(app.showError);
  return view;
}
async function reloadEditor(view){
  if(!view||view.closed)return;
  if(view.dirty&&!window.confirm('Discard unsaved changes and reload '+view.file.path+'?'))return;
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


function goToLine(view){
  if(!view||view.closed)return;
  const raw=window.prompt('Go to line (1-'+view.cm.state.doc.lines+'):','1');
  if(raw===null)return;
  const lineNumber=Math.max(1,Math.min(view.cm.state.doc.lines,Number.parseInt(raw,10)||1));
  const line=view.cm.state.doc.line(lineNumber);
  view.cm.dispatch({selection:{anchor:line.from},scrollIntoView:true});
  view.cm.focus();
}

document.addEventListener('keydown',event=>{
  if(!(event.ctrlKey||event.metaKey)||!activeEditorID)return;
  const view=editors.get(activeEditorID);if(!view||view.closed)return;
  const key=event.key.toLowerCase();
  if(key==='s'){
    event.preventDefault();
    saveEditor(view).catch(app.showError);
  }else if(key==='g'){
    event.preventDefault();
    goToLine(view);
  }
});

window.addEventListener('beforeunload',event=>{
  if(![...editors.values()].some(view=>!view.closed&&view.dirty))return;
  event.preventDefault();
  event.returnValue='';
});

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
  saveEditor,
  closeEditor,
  goToLine,
  activateEditor,
  destroyEditor,
  get active(){return activeEditorID;}
};
