const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for file compare');

const style=document.createElement('style');
style.textContent=`
.file-compare-pane{padding:0!important;overflow:hidden!important;background:#10151c}
.file-compare-tab{display:inline-flex;align-items:center;flex:0 0 auto;max-width:260px;min-width:0;white-space:nowrap;box-sizing:border-box}.file-compare-tab .close{margin-left:7px;flex:0 0 auto}.file-compare-tab-label{display:flex;align-items:center;flex:0 1 auto;min-width:0;max-width:220px;overflow:hidden;white-space:nowrap}.file-compare-tab-filename{display:block;flex:0 1 auto;min-width:0;overflow:hidden;white-space:nowrap;text-overflow:ellipsis}.file-compare-tab-separator{flex:0 0 auto;margin:0 5px}.file-compare-tab.dirty::after{content:'●';color:#f2c96d;margin-left:5px}
.file-compare-dialog{width:100%;height:100%;display:flex;flex-direction:column;min-width:0;min-height:0;background:#10151c;border:0;overflow:hidden}
.file-compare-head{display:flex;align-items:center;gap:7px;padding:7px 9px;border-bottom:1px solid #303843}
.file-compare-title{font-weight:700;font-size:12px;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.file-compare-head button{padding:4px 7px;font-size:11px}.file-compare-head button.active{background:#34445a;border-color:#6f91bb}
.file-compare-summary{font:10px ui-monospace,monospace;opacity:.66;white-space:nowrap}
.file-compare-head .file-compare-edit-toggle.active{background:#294c3a;border-color:#4d8769}
.file-compare-editor-deck{height:36%;min-height:190px;display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);border-bottom:1px solid #303843;background:#0d1218}.file-compare-editor-deck.hidden{display:none}
.file-compare-editor-pane{min-width:0;min-height:0;display:flex;flex-direction:column}.file-compare-editor-pane+.file-compare-editor-pane{border-left:1px solid #303843}
.file-compare-editor-head{display:flex;align-items:center;gap:7px;padding:4px 8px;border-bottom:1px solid #252d37;background:#131a22;font:10px ui-monospace,monospace}.file-compare-editor-label{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.file-compare-dirty{color:#f2c96d;font-weight:800}.file-compare-dirty.hidden{visibility:hidden}.file-compare-save{padding:3px 8px;font-size:10px}.file-compare-save.dirty{background:#266340;border-color:#4c9a6b;color:#ecfff3}.file-compare-save:disabled{opacity:.42}
.file-compare-editor-host{flex:1;min-height:0;overflow:hidden}.file-compare-editor-host .cm-editor{height:100%;font-size:12px}.file-compare-editor-host .cm-scroller{overflow:auto;font-family:ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace}
@media(max-width:850px){.file-compare-editor-deck{grid-template-columns:1fr;grid-template-rows:minmax(140px,1fr) minmax(140px,1fr);height:48%}.file-compare-editor-pane+.file-compare-editor-pane{border-left:0;border-top:1px solid #303843}}
.file-compare-filters{display:flex;align-items:center;gap:6px;padding:5px 9px;border-bottom:1px solid #303843;background:#111820;flex-wrap:wrap}.file-compare-filters button{padding:3px 7px;font-size:10px}.file-compare-filters button.active{background:#34445a;border-color:#6f91bb}.file-compare-filter-note{font:10px ui-monospace,monospace;opacity:.52;margin-left:auto}
.file-compare-gap{grid-column:1/-1;padding:2px 8px;text-align:center;font:10px/1.4 ui-monospace,monospace;opacity:.52;background:rgba(120,140,165,.07);border-bottom:1px solid rgba(255,255,255,.035)}
.file-compare-columns{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);border-bottom:1px solid #303843;background:#151b23}
.file-compare-column{padding:6px 9px;font:11px ui-monospace,monospace;font-weight:700;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.file-compare-column+.file-compare-column{border-left:1px solid #303843}
.file-compare-body{flex:1;min-height:0;overflow-y:auto;overflow-x:hidden;background:#0b0f14;--file-compare-shift-x:0px}
.file-compare-horizontal-scroll{flex:0 0 auto;box-sizing:border-box;width:100%;height:19px;overflow-x:scroll;overflow-y:hidden;scrollbar-width:auto;background:#151b23;border-top:1px solid #303843}
.file-compare-horizontal-scroll[hidden]{display:none}
.file-compare-horizontal-size{height:1px;pointer-events:none}
.file-compare-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);min-width:0}
.file-compare-cell{display:grid;grid-template-columns:48px minmax(0,1fr);min-width:0;border-bottom:1px solid rgba(255,255,255,.035)}
.file-compare-row{position:relative}.file-compare-cell.compare-line-selected{outline:1px solid #63b6ff;outline-offset:-1px}
.file-compare-line-copy{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);z-index:3;white-space:nowrap;font-size:10px;padding:3px 7px;background:#243f57;border:1px solid #6da6d0;color:#f4faff;border-radius:5px;box-shadow:0 2px 9px #0009;cursor:pointer}
.file-compare-line-copy:hover{background:#315c7c}
.file-compare-cell+.file-compare-cell{border-left:1px solid #303843}.file-compare-no{padding:1px 7px;text-align:right;user-select:none;opacity:.45;font:10px/1.45 ui-monospace,monospace;border-right:1px solid rgba(255,255,255,.06)}
.file-compare-code{display:block;min-width:0;padding:1px 7px;white-space:pre;overflow:hidden;font:11px/1.45 ui-monospace,monospace}
.file-compare-code-inner{display:inline-block;min-width:100%;white-space:pre;transform:translateX(var(--file-compare-shift-x,0px))}.file-compare-cell.removed.important{background:rgba(229,72,86,.28)}.file-compare-cell.added.important{background:rgba(232,174,55,.28)}.file-compare-cell.removed.unimportant,.file-compare-cell.added.unimportant{background:rgba(58,149,214,.23)}.file-compare-cell.blank{opacity:.3}
.file-compare-syntax-keyword{color:#c792ea}.file-compare-syntax-comment{color:#6a9955;font-style:italic}.file-compare-syntax-string{color:#ce9178}.file-compare-syntax-number{color:#b5cea8}.file-compare-syntax-command{color:#dcdcaa}.file-compare-syntax-variable{color:#9cdcfe}
.file-compare-inline-change{border-radius:2px;box-shadow:inset 0 -1px 0 rgba(255,255,255,.28)}.file-compare-inline-change.removed{background:rgba(255,84,98,.38)}.file-compare-inline-change.added{background:rgba(255,196,74,.40)}
.file-compare-hunk{border-top:1px solid #3a4350;border-bottom:1px solid #3a4350;margin:5px 0}
.file-compare-hunk-head{position:sticky;left:0;display:grid;grid-template-columns:minmax(0,1fr) auto minmax(0,1fr);align-items:center;gap:8px;padding:4px 8px;background:#171e27;font:10px ui-monospace,monospace;z-index:1}
.file-compare-hunk-label{opacity:.72;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.file-compare-hunk-actions{display:flex;justify-content:center;align-items:center;gap:5px;grid-column:2}
.file-compare-hunk-info{grid-column:3;text-align:right;opacity:.66;white-space:nowrap;min-width:0;overflow:hidden;text-overflow:ellipsis}
.file-compare-hunk-head button{padding:2px 6px;font-size:10px}
@media(max-width:930px){.file-compare-hunk-info{display:none}.file-compare-hunk-head{grid-template-columns:minmax(0,1fr) auto}}
.file-compare-inline{min-width:0;width:100%}.file-compare-inline-line{display:grid;grid-template-columns:52px 20px minmax(0,1fr);border-bottom:1px solid rgba(255,255,255,.035);font:11px/1.45 ui-monospace,monospace}.file-compare-inline-line>span{padding:1px 7px}
.file-compare-inline-code{display:block;min-width:0;white-space:pre;overflow:hidden}.file-compare-inline-no{text-align:right;opacity:.45}.file-compare-inline-line.removed.important{background:rgba(229,72,86,.28)}.file-compare-inline-line.added.important{background:rgba(232,174,55,.28)}.file-compare-inline-line.removed.unimportant,.file-compare-inline-line.added.unimportant{background:rgba(58,149,214,.23)}
.file-compare-empty{padding:24px;text-align:center;opacity:.62}
html[data-taskmenu-theme="light"] .file-compare-dialog{background:#fff;border-color:#b9c0c8}.file-compare-body{color:inherit}html[data-taskmenu-theme="light"] .file-compare-filters{background:#f8fafc;border-color:#d0d7de}html[data-taskmenu-theme="light"] .file-compare-editor-deck{background:#fff;border-color:#d0d7de}html[data-taskmenu-theme="light"] .file-compare-editor-head{background:#f6f8fa;border-color:#d0d7de}html[data-taskmenu-theme="light"] .file-compare-columns,html[data-taskmenu-theme="light"] .file-compare-hunk-head{background:#f6f8fa;border-color:#d0d7de}html[data-taskmenu-theme="light"] .file-compare-cell.removed.important,html[data-taskmenu-theme="light"] .file-compare-inline-line.removed.important{background:#ffe2e5}html[data-taskmenu-theme="light"] .file-compare-cell.added.important,html[data-taskmenu-theme="light"] .file-compare-inline-line.added.important{background:#fff1c9}html[data-taskmenu-theme="light"] .file-compare-cell.removed.unimportant,html[data-taskmenu-theme="light"] .file-compare-cell.added.unimportant,html[data-taskmenu-theme="light"] .file-compare-inline-line.removed.unimportant,html[data-taskmenu-theme="light"] .file-compare-inline-line.added.unimportant{background:#e3f3ff}
`;
document.head.append(style);

const tabsHost=document.querySelector('#tabs'),panesHost=document.querySelector('#panes');
if(!tabsHost||!panesHost)throw new Error('File Compare tab hosts unavailable');
const compareTabID='taskdeck-file-compare';
const pane=document.createElement('div');pane.className='pane file-compare-pane hidden';pane.dataset.id=compareTabID;pane.dataset.viewKind='file-compare';
panesHost.append(pane);
let compareTab=null,lastWorkspaceTab=null;
const dialog=document.createElement('div');dialog.className='file-compare-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','File compare');
const head=document.createElement('div');head.className='file-compare-head';
const title=document.createElement('div');title.className='file-compare-title';
const summary=document.createElement('div');summary.className='file-compare-summary';
const sideButton=document.createElement('button');sideButton.type='button';sideButton.textContent='Side by side';
const inlineButton=document.createElement('button');inlineButton.type='button';inlineButton.textContent='Inline';
const editButton=document.createElement('button');editButton.type='button';editButton.className='file-compare-edit-toggle';editButton.textContent='Edit';editButton.title='Edit both writable files with syntax highlighting';
const gitLocalButton=document.createElement('button');gitLocalButton.type='button';gitLocalButton.textContent='Open local WORKTREE ↗';gitLocalButton.title='Compare with your local working tree so only the local file can be edited; no automatic staging';gitLocalButton.hidden=true;
const leftTopSave=document.createElement('button');leftTopSave.type='button';leftTopSave.className='file-compare-save';leftTopSave.textContent='Save Left';leftTopSave.title='Save the left compare file';
const rightTopSave=document.createElement('button');rightTopSave.type='button';rightTopSave.className='file-compare-save';rightTopSave.textContent='Save Right';rightTopSave.title='Save the right compare file';
const reloadButton=document.createElement('button');reloadButton.type='button';reloadButton.textContent='↻';reloadButton.title='Reload both compare sources';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close compare';
head.append(title,summary,sideButton,inlineButton,editButton,gitLocalButton,leftTopSave,rightTopSave,reloadButton,closeButton);
const filters=document.createElement('div');filters.className='file-compare-filters';
const viewAllButton=document.createElement('button');viewAllButton.type='button';viewAllButton.textContent='View all';viewAllButton.title='Show the complete file comparison';
const viewDiffButton=document.createElement('button');viewDiffButton.type='button';viewDiffButton.textContent='View diff';viewDiffButton.title='Show changed lines only';
const viewContextButton=document.createElement('button');viewContextButton.type='button';viewContextButton.textContent='View diff context';viewContextButton.title='Show changed lines plus the enclosing brace/indentation block when it can be identified';
const viewUnimportantButton=document.createElement('button');viewUnimportantButton.type='button';viewUnimportantButton.textContent='View unimportant';viewUnimportantButton.title='Toggle confirmed comment/cosmetic changes (blue)';
const filterNote=document.createElement('span');filterNote.className='file-compare-filter-note';filterNote.textContent='Select changed lines to copy just those lines · blue = cosmetic · red/amber = logic-sensitive';
filters.append(viewAllButton,viewDiffButton,viewContextButton,viewUnimportantButton,filterNote);
const columns=document.createElement('div');columns.className='file-compare-columns';
const leftLabel=document.createElement('div');leftLabel.className='file-compare-column';
const rightLabel=document.createElement('div');rightLabel.className='file-compare-column';columns.append(leftLabel,rightLabel);
const editorDeck=document.createElement('div');editorDeck.className='file-compare-editor-deck hidden';
function compareEditorPane(side){
  const pane=document.createElement('div');pane.className='file-compare-editor-pane '+side;
  const paneHead=document.createElement('div');paneHead.className='file-compare-editor-head';
  const paneLabel=document.createElement('span');paneLabel.className='file-compare-editor-label';
  const dirty=document.createElement('span');dirty.className='file-compare-dirty hidden';dirty.textContent='●';dirty.title='Unsaved compare changes';
  const save=document.createElement('button');save.type='button';save.className='file-compare-save';save.textContent='Save';save.title='Save this compare side';
  const host=document.createElement('div');host.className='file-compare-editor-host';
  paneHead.append(paneLabel,dirty,save);pane.append(paneHead,host);
  return {pane,label:paneLabel,dirty,save,host,editor:null};
}
const leftEditorUI=compareEditorPane('left'),rightEditorUI=compareEditorPane('right');editorDeck.append(leftEditorUI.pane,rightEditorUI.pane);
const body=document.createElement('div');body.className='file-compare-body';
const horizontalScroll=document.createElement('div');horizontalScroll.className='file-compare-horizontal-scroll';horizontalScroll.hidden=true;
horizontalScroll.tabIndex=0;horizontalScroll.setAttribute('aria-label','Scroll both diff columns horizontally');
const horizontalSize=document.createElement('div');horizontalSize.className='file-compare-horizontal-size';horizontalScroll.append(horizontalSize);
dialog.append(head,filters,columns,editorDeck,body,horizontalScroll);pane.append(dialog);
function ensureCompareTab(){
  if(compareTab)return compareTab;
  const tab=document.createElement('button');tab.type='button';tab.className='tab file-compare-tab';tab.dataset.id=compareTabID;tab.dataset.viewKind='file-compare';
  const label=document.createElement('span');label.textContent='Compare';label.className='file-compare-tab-label';
  const dismiss=document.createElement('span');dismiss.className='close';dismiss.textContent='×';dismiss.title='Close File Compare';
  tab.append(label,dismiss);tabsHost.append(tab);compareTab=tab;
  tab.onclick=()=>activateCompareTab();
  dismiss.onclick=event=>{event.stopPropagation();close();};
  globalThis.TaskMenuTabContext?.registerTab?.(tab);
  return tab;
}
function activateCompareTab(){
  const existing=ensureCompareTab();
  const activeTab=tabsHost.querySelector('.tab.active,.db-tab.active');
  if(activeTab&&activeTab!==existing)lastWorkspaceTab=activeTab;
  return app.activateExternalView(compareTabID,{force:true});
}
window.addEventListener('taskmenu:view-activated',event=>{
  const active=event.detail?.kind==='external'&&event.detail?.id===compareTabID;
  pane.classList.toggle('hidden',!active);
  compareTab?.classList.toggle('active',Boolean(active));
  if(active)scheduleCompareHorizontalMeasure();
});

let current=null;
let viewMode='side';
let contentMode='all';
let showUnimportant=true;
let compareSelection=null;
let editMode=false;
let renderTimer=0;
let horizontalMeasureFrame=0;
function syncCompareHorizontalOffset(){
  body.style.setProperty('--file-compare-shift-x',(-horizontalScroll.scrollLeft)+'px');
}
horizontalScroll.addEventListener('scroll',syncCompareHorizontalOffset);
body.addEventListener('wheel',event=>{
  if(horizontalScroll.hidden)return;
  const delta=Math.abs(event.deltaX)>Math.abs(event.deltaY)?event.deltaX:event.shiftKey?(event.deltaX||event.deltaY):0;
  if(!delta)return;
  const before=horizontalScroll.scrollLeft;
  horizontalScroll.scrollLeft+=delta;
  if(horizontalScroll.scrollLeft!==before)event.preventDefault();
},{passive:false});
function compareSharedHorizontalOverflow(contentWidth,viewportWidth,paddingLeft=0,paddingRight=0){
  return Math.max(0,Math.ceil(contentWidth-Math.max(0,viewportWidth-paddingLeft-paddingRight)));
}
function updateCompareHorizontalMeasure(){
  if(!current){
    horizontalScroll.hidden=true;horizontalScroll.scrollLeft=0;syncCompareHorizontalOffset();return;
  }
  if(pane.classList.contains('hidden')||!body.clientWidth)return;
  let maxOverflow=0;
  const paddingByClass=new Map();
  for(const inner of body.querySelectorAll('.file-compare-code-inner')){
    const viewport=inner.parentElement;
    if(!viewport?.clientWidth)continue;
    const kind=viewport.className;
    let padding=paddingByClass.get(kind);
    if(padding===undefined){
      const css=getComputedStyle(viewport);
      padding=(parseFloat(css.paddingLeft)||0)+(parseFloat(css.paddingRight)||0);
      paddingByClass.set(kind,padding);
    }
    const overflow=compareSharedHorizontalOverflow(inner.scrollWidth,viewport.clientWidth,padding);
    maxOverflow=Math.max(maxOverflow,overflow);
  }
  if(maxOverflow<=1){
    horizontalScroll.hidden=true;horizontalScroll.scrollLeft=0;syncCompareHorizontalOffset();return;
  }
  horizontalScroll.hidden=false;
  horizontalSize.style.width=Math.ceil(horizontalScroll.clientWidth+maxOverflow)+'px';
  // Setting a smaller track can clamp scrollLeft without dispatching a scroll event.
  syncCompareHorizontalOffset();
}
function scheduleCompareHorizontalMeasure(){
  if(horizontalMeasureFrame)return;
  horizontalMeasureFrame=requestAnimationFrame(()=>{
    horizontalMeasureFrame=0;updateCompareHorizontalMeasure();
  });
}
if(typeof ResizeObserver==='function'){
  const horizontalObserver=new ResizeObserver(scheduleCompareHorizontalMeasure);
  horizontalObserver.observe(body);
}else window.addEventListener('resize',scheduleCompareHorizontalMeasure);
let selectedCompareLines=null;

function sourceIdentity(source){
  if(!source)return '';
  return [
    String(source.kind||'source'),
    String(source.profileID||source.repoID||source.editorID||''),
    String(source.path||''),
    String(source.ref||source.state||'')
  ].join('|');
}
function sourceWritable(source){return typeof source?.saveText==='function'||typeof source?.writeText==='function';}
function selectionSnapshot(){
  if(!compareSelection)return null;
  return {identity:compareSelection.identity,label:compareSelection.label,path:compareSelection.source?.path||'',kind:compareSelection.source?.kind||''};
}
function announceCompareSelection(){
  window.dispatchEvent(new CustomEvent('taskmenu:file-compare-selection-changed',{detail:selectionSnapshot()}));
}
function selectForCompare(source){
  if(!source?.load)throw new Error('Compare selection requires a readable file source');
  compareSelection={source,identity:sourceIdentity(source),label:String(source.label||source.path||'Selected file')};
  announceCompareSelection();
  return selectionSnapshot();
}
function clearCompareSelection(){compareSelection=null;announceCompareSelection();}
function canCompareWithSelected(source){
  return Boolean(compareSelection&&source?.load&&compareSelection.identity!==sourceIdentity(source));
}
async function compareWithSelected(source,{title='Selected files'}={}){
  if(!compareSelection){selectForCompare(source);return false;}
  if(!canCompareWithSelected(source))throw new Error('Choose a different second file for compare');
  const selected=compareSelection;
  const opened=await open({title,left:selected.source,right:source});
  if(opened&&compareSelection===selected)clearCompareSelection();
  return Boolean(opened);
}
async function openSources(sources,{title='Selected files'}={}){
  sources=(Array.isArray(sources)?sources:[]).filter(source=>source?.load);
  if(sources.length!==2)throw new Error('File Compare requires exactly two readable files');
  const opened=await open({title,left:sources[0],right:sources[1]});
  if(opened)clearCompareSelection();
  return opened;
}
function compareSideUI(side){return side==='left'?leftEditorUI:rightEditorUI;}
function compareSideSource(side){return current?.[side]||null;}
function compareHasDirty(){return Boolean(current&&(current.left?.dirty||current.right?.dirty));}
function compareHasSaving(){return Boolean(current&&(current.left?.saving||current.right?.saving));}
function syncCompareSaveState(side){
  const source=compareSideSource(side),ui=compareSideUI(side);if(!ui)return;
  const topSave=side==='left'?leftTopSave:rightTopSave;
  ui.label.textContent=source?.label||side;
  ui.label.title=ui.label.textContent;
  ui.dirty.classList.toggle('hidden',!source?.dirty);
  const disabled=!sourceWritable(source)||!source?.dirty||Boolean(source?.saving);
  ui.save.disabled=disabled;topSave.disabled=disabled;
  ui.save.classList.toggle('dirty',Boolean(source?.dirty&&!source?.saving));
  topSave.classList.toggle('dirty',Boolean(source?.dirty&&!source?.saving));
  ui.save.textContent=source?.saving?'Saving…':'Save';
  topSave.textContent=source?.saving?'Saving…':(side==='left'?'Save Left':'Save Right');
  compareTab?.classList.toggle('dirty',compareHasDirty());
}
function scheduleCompareRender(){
  clearTimeout(renderTimer);
  renderTimer=setTimeout(()=>{renderTimer=0;render();},120);
}
function setSourceBuffer(side,text,{syncEditor=true,immediate=false}={}){
  const source=compareSideSource(side);if(!source)return;
  source.text=String(text??'');
  source.dirty=source.text!==String(source.baselineText??'');
  current.syntax=null;
  if(syncEditor)compareSideUI(side)?.editor?.setText(source.text);
  syncCompareSaveState(side);
  if(immediate)render();else scheduleCompareRender();
}
function destroyCompareEditors(){
  clearTimeout(renderTimer);renderTimer=0;
  for(const ui of [leftEditorUI,rightEditorUI]){
    try{ui.editor?.destroy?.();}catch{}
    ui.editor=null;ui.host.replaceChildren();
  }
}
function ensureCompareEditors(){
  if(!current||!editMode)return;
  const api=globalThis.TaskMenuEditor;
  if(!api?.createDetachedEditor)throw new Error('Syntax editor is unavailable for File Compare');
  for(const side of ['left','right']){
    const source=compareSideSource(side),ui=compareSideUI(side);
    if(!ui.editor){
      ui.editor=api.createDetachedEditor(ui.host,source?.text||'',compareSyntaxPath(source,side==='left'?current.right:current.left),{
        readOnly:!sourceWritable(source),
        onChange:text=>setSourceBuffer(side,text,{syncEditor:false})
      });
    }else{
      ui.editor.setReadOnly(!sourceWritable(source));
      ui.editor.setText(source?.text||'');
    }
    syncCompareSaveState(side);
  }
}
function setEditMode(enabled){
  editMode=Boolean(enabled);
  editorDeck.classList.toggle('hidden',!editMode);
  editButton.classList.toggle('active',editMode);
  editButton.setAttribute('aria-pressed',editMode?'true':'false');
  if(editMode)ensureCompareEditors();
}
async function saveCompareSide(side){
  const source=compareSideSource(side);
  if(source?.kind==='git-head'||source?.kind==='git-index'||source?.kind==='git-commit')throw new Error('Git snapshots cannot be edited or saved');
  if(!source||!sourceWritable(source))throw new Error((source?.label||side)+' is read-only');
  if(!source.dirty||source.saving)return false;
  const savingText=source.text;
  source.saving=true;syncCompareSaveState(side);
  try{
    await saveSource(source,savingText);
    source.baselineText=savingText;
    source.dirty=source.text!==savingText;
    if(source.kind==='remote'){
      window.dispatchEvent(new CustomEvent('taskmenu:file-transfer-remote-edited',{detail:{
        profile_id:String(source.profileID||source.meta?.profile_id||''),path:String(source.path||source.meta?.path||''),sha256:String(source.meta?.sha256||'')
      }}));
    }
    window.dispatchEvent(new CustomEvent('taskmenu:file-compare-write',{detail:{label:source.label,path:source.path||'',profile_id:source.profileID||'',source_kind:source.kind||''}}));
    render();return true;
  }finally{source.saving=false;syncCompareSaveState(side);}
}

const compareKeywordText={
  cpp:'alignas alignof and and_eq asm atomic_cancel atomic_commit atomic_noexcept auto bitand bitor bool break case catch char char8_t char16_t char32_t class compl concept const consteval constexpr constinit const_cast continue co_await co_return co_yield decltype default delete do double dynamic_cast else enum explicit export extern false float for friend goto if inline int long mutable namespace new noexcept not not_eq nullptr operator or or_eq private protected public reflexpr register reinterpret_cast requires return short signed sizeof static static_assert static_cast struct switch synchronized template this thread_local throw true try typedef typeid typename union unsigned using virtual void volatile wchar_t while xor xor_eq',
  go:'break default func interface select case defer go map struct chan else goto package switch const fallthrough if range type continue for import return var',
  python:'and as assert async await break class continue def del elif else except False finally for from global if import in is lambda None nonlocal not or pass raise return True try while with yield match case',
  javascript:'as async await break case catch class const continue debugger default delete do else export extends false finally for from function get if import in instanceof let new null of return set static super switch this throw true try typeof undefined var void while with yield',
  typescript:'abstract any as asserts async await bigint boolean break case catch class const constructor continue debugger declare default delete do else enum export extends false finally for from function get if implements import in infer instanceof interface is keyof let module namespace never new null number object of override private protected public readonly require return set static string super switch symbol this throw true try type typeof undefined unique unknown var void while with yield',
  java:'abstract assert boolean break byte case catch char class const continue default do double else enum extends final finally float for goto if implements import instanceof int interface long native new null package private protected public return short static strictfp super switch synchronized this throw throws transient true try void volatile while false',
  php:'abstract and array as break callable case catch class clone const continue declare default die do echo else elseif empty enddeclare endfor endforeach endif endswitch endwhile eval exit extends final finally fn for foreach function global goto if implements include include_once instanceof insteadof interface isset list match namespace new or print private protected public readonly require require_once return static switch throw trait try unset use var while xor yield',
  rust:'as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while',
  sql:'add all alter and any as asc backup between by case check column constraint create database default delete desc distinct drop exec exists foreign from full group having in index inner insert into is join key left like limit not null or order outer primary procedure right rownum select set table top truncate union unique update values view where with',
  json:'true false null'
};
const compareKeywordSets=Object.fromEntries(Object.entries(compareKeywordText).map(([key,value])=>[key,new Set(value.split(/\s+/))]));
function compareLanguageID(pathValue){
  const language=globalThis.TaskMenuEditor?.languageForPath?.(String(pathValue||''));
  if(!language)return '';
  if(language.id==='jsx')return 'javascript';
  if(language.id==='tsx')return 'typescript';
  return String(language.id||'');
}
function compareSyntaxProfile(pathValue){
  const id=compareLanguageID(pathValue);
  const cLike=new Set(['cpp','go','javascript','typescript','java','php','rust','csharp','kotlin','dart','protobuf','actionscript']);
  const hashLike=new Set(['python','yaml','shell','dockerfile','makefile','cmake','toml','nim']);
  const profile={id,keywords:compareKeywordSets[id]||new Set(),line:[],block:[],quotes:['"',"'"]};
  if(cLike.has(id)){profile.line=['//'];profile.block=[['/*','*/']];}
  if(hashLike.has(id))profile.line=['#'];
  if(id==='javascript'||id==='typescript')profile.quotes.push('`');
  if(id==='css'||id==='scss'||id==='less')profile.block=[['/*','*/']];
  if(id==='sql'){profile.line=['--'];profile.block=[['/*','*/']];}
  if(id==='lua'){profile.line=['--'];profile.block=[['--[[',']]']];}
  if(id==='config')profile.line=['#',';'];
  if(id==='html'||id==='xml'||id==='vue'||id==='markdown')profile.block=[['<!--','-->']];
  return profile;
}
function compareSyntaxSource(pathValue,text){
  const profile=compareSyntaxProfile(pathValue),lines=splitLines(text),result=[];let blockClose='';
  for(const line of lines){
    const tokens=[];let i=0;
    while(i<line.length){
      if(blockClose){
        const end=line.indexOf(blockClose,i),to=end<0?line.length:end+blockClose.length;
        tokens.push({from:i,to,type:'comment'});i=to;
        if(end<0)break;
        blockClose='';continue;
      }
      let block=null;
      for(const pair of profile.block){if(line.startsWith(pair[0],i)){block=pair;break;}}
      if(block){
        const end=line.indexOf(block[1],i+block[0].length),to=end<0?line.length:end+block[1].length;
        tokens.push({from:i,to,type:'comment'});i=to;
        if(end<0){blockClose=block[1];break;}
        continue;
      }
      const lineMarker=profile.line.find(marker=>line.startsWith(marker,i));
      if(lineMarker){tokens.push({from:i,to:line.length,type:'comment'});break;}
      const ch=line[i];
      if(profile.quotes.includes(ch)){
        let j=i+1;
        while(j<line.length){if(line[j]==='\\'){j+=2;continue;}if(line[j]===ch){j++;break;}j++;}
        tokens.push({from:i,to:j,type:'string'});i=j;continue;
      }
      if((profile.id==='html'||profile.id==='xml'||profile.id==='vue')&&ch==='<'){
        const end=line.indexOf('>',i+1),to=end<0?line.length:end+1;
        tokens.push({from:i,to,type:'command'});i=to;continue;
      }
      if(/[0-9]/.test(ch)){
        let j=i+1;while(j<line.length&&/[0-9A-Fa-fxX._]/.test(line[j]))j++;
        tokens.push({from:i,to:j,type:'number'});i=j;continue;
      }
      if(/[A-Za-z_$]/.test(ch)){
        let j=i+1;while(j<line.length&&/[A-Za-z0-9_$]/.test(line[j]))j++;
        const word=line.slice(i,j);
        if(profile.keywords.has(word)||profile.keywords.has(word.toLowerCase()))tokens.push({from:i,to:j,type:'keyword'});
        i=j;continue;
      }
      i++;
    }
    result.push(tokens);
  }
  return result;
}
function compareSyntaxPath(source,other){return String(source?.path||other?.path||'');}
function ensureCompareSyntax(){
  if(!current)return {left:[],right:[]};
  const leftPath=compareSyntaxPath(current.left,current.right),rightPath=compareSyntaxPath(current.right,current.left);
  const key=[leftPath,rightPath,current.left.text,current.right.text];
  if(current.syntax&&current.syntax.key.every((value,index)=>value===key[index]))return current.syntax;
  current.syntax={key,left:compareSyntaxSource(leftPath,current.left.text),right:compareSyntaxSource(rightPath,current.right.text)};
  return current.syntax;
}
function compareWordParts(text){
  const parts=[];const re=/[\p{L}\p{N}_$]+|\s+|./gu;let match;
  while((match=re.exec(String(text||''))))parts.push({text:match[0],from:match.index,to:match.index+match[0].length});
  return parts;
}
function compareFallbackRanges(left,right){
  let prefix=0;while(prefix<left.length&&prefix<right.length&&left[prefix]===right[prefix])prefix++;
  let suffix=0;while(left.length-1-suffix>=prefix&&right.length-1-suffix>=prefix&&left[left.length-1-suffix]===right[right.length-1-suffix])suffix++;
  return {left:left.length-suffix>prefix?[{from:prefix,to:left.length-suffix}]:[],right:right.length-suffix>prefix?[{from:prefix,to:right.length-suffix}]:[]};
}
function compareInlineRanges(left,right){
  left=String(left||'');right=String(right||'');
  if(left===right)return {left:[],right:[]};
  const a=compareWordParts(left),b=compareWordParts(right);
  if(!a.length||!b.length||a.length*b.length>24000)return compareFallbackRanges(left,right);
  const dp=Array.from({length:a.length+1},()=>new Uint16Array(b.length+1));
  for(let i=a.length-1;i>=0;i--)for(let j=b.length-1;j>=0;j--)dp[i][j]=a[i].text===b[j].text?dp[i+1][j+1]+1:Math.max(dp[i+1][j],dp[i][j+1]);
  const leftRanges=[],rightRanges=[];let i=0,j=0;
  while(i<a.length&&j<b.length){
    if(a[i].text===b[j].text){i++;j++;continue;}
    if(dp[i+1][j]>=dp[i][j+1]){leftRanges.push({from:a[i].from,to:a[i].to});i++;}
    else{rightRanges.push({from:b[j].from,to:b[j].to});j++;}
  }
  while(i<a.length){leftRanges.push({from:a[i].from,to:a[i].to});i++;}
  while(j<b.length){rightRanges.push({from:b[j].from,to:b[j].to});j++;}
  return {left:leftRanges,right:rightRanges};
}
function compareRangeContains(ranges,offset){return ranges.some(range=>offset>=range.from&&offset<range.to);}
function compareCommentTokens(tokens){return (tokens||[]).filter(token=>token.type==='comment');}
function compareTextWithoutRanges(text,ranges){
  text=String(text||'');if(!ranges?.length)return text;
  let at=0,out='';
  for(const range of ranges){if(range.from>at)out+=text.slice(at,range.from);at=Math.max(at,range.to);}
  return out+text.slice(at);
}
function compareCommentOnly(text,tokens){
  const comments=compareCommentTokens(tokens);
  return comments.length>0&&compareTextWithoutRanges(text,comments).trim()==='';
}
function compareRowLanguageID(row){
  const leftPath=compareSyntaxPath(current?.left,current?.right),rightPath=compareSyntaxPath(current?.right,current?.left);
  return compareLanguageID(row.left?leftPath:rightPath);
}
const compareIndentInsensitiveLanguages=new Set(['cpp','go','javascript','typescript','java','php','rust','csharp','kotlin','dart','protobuf','actionscript','css','scss','less','sql','json','html','xml','vue']);
// Shared importance classification for full File Compare and Git-panel preview.
function classifyCompareTextRow(row,leftTokens=[],rightTokens=[],language=''){
  const leftText=row.left?.text??'',rightText=row.right?.text??'';
  if(!row.left||!row.right){
    const text=row.left?leftText:rightText,tokens=row.left?leftTokens:rightTokens;
    return text.trim()===''||compareCommentOnly(text,tokens)?'unimportant':'important';
  }
  if(leftText===rightText)return 'context';
  if(compareCommentOnly(leftText,leftTokens)&&compareCommentOnly(rightText,rightTokens))return 'unimportant';
  const leftComments=compareCommentTokens(leftTokens),rightComments=compareCommentTokens(rightTokens);
  if((leftComments.length||rightComments.length)&&compareTextWithoutRanges(leftText,leftComments).trimEnd()===compareTextWithoutRanges(rightText,rightComments).trimEnd())return 'unimportant';
  if(compareIndentInsensitiveLanguages.has(language)&&leftText.trim()===rightText.trim())return 'unimportant';
  return 'important';
}
function classifyCompareRow(row){
  if(row.hunk<0)return 'context';
  const syntax=ensureCompareSyntax();
  const leftTokens=row.left?syntax.left[row.left.no-1]||[]:[],rightTokens=row.right?syntax.right[row.right.no-1]||[]:[];
  return classifyCompareTextRow(row,leftTokens,rightTokens,compareRowLanguageID(row));
}
// Git patch hunks have absolute line numbers but omit unchanged regions.
// Decorate hunk rows with the same syntax, inline ranges and importance rules
// without mutating whichever full compare tab is currently open.
function decorateGitPreviewRows(rows,path){
  const leftRows=rows.filter(row=>row.left).map(row=>row.left.text);
  const rightRows=rows.filter(row=>row.right).map(row=>row.right.text);
  const leftSyntax=compareSyntaxSource(path,leftRows.join('\n'));
  const rightSyntax=compareSyntaxSource(path,rightRows.join('\n'));
  const language=compareLanguageID(path);
  let leftIndex=0,rightIndex=0;
  for(const row of rows){
    if(row.note)continue;
    const leftTokens=row.left?leftSyntax[leftIndex++]||[]:[];
    const rightTokens=row.right?rightSyntax[rightIndex++]||[]:[];
    if(row.left)row.left.tokens=leftTokens;
    if(row.right)row.right.tokens=rightTokens;
    row.importance=classifyCompareTextRow(row,leftTokens,rightTokens,language);
    row.ranges=compareInlineRanges(row.left?.text||'',row.right?.text||'');
  }
  return rows;
}
function renderGitPreviewCode(node,spec,ranges=[]){
  renderCompareCode(node,spec?.text??'',spec?.tokens||[],ranges,spec?.kind==='removed'?'removed':spec?.kind==='added'?'added':'');
}
function enrichCompareModel(model){
  let important=0,unimportant=0;
  for(const row of model.rows){
    row.importance=classifyCompareRow(row);
    if(row.hunk<0)continue;
    if(row.importance==='unimportant')unimportant++;else important++;
  }
  model.stats={important,unimportant};return model;
}
function compareRowText(row){return String(row?.right?.text??row?.left?.text??'');}
function compareIndent(text){
  const match=String(text||'').match(/^[ \t]*/)?.[0]||'';
  return [...match].reduce((count,ch)=>count+(ch==='\t'?4:1),0);
}
function compareStructuralCode(row){
  const spec=row?.right||row?.left;if(!spec)return '';
  const syntax=ensureCompareSyntax(),tokens=(row?.right?syntax.right:syntax.left)[spec.no-1]||[];
  return compareTextWithoutRanges(spec.text,tokens.filter(token=>token.type==='comment'||token.type==='string'));
}
function compareFallbackContext(model,index){
  return [Math.max(0,index-3),Math.min(model.rows.length-1,index+3)];
}
function compareStructuralContextRange(model,index){
  const row=model.rows[index],language=compareRowLanguageID(row),text=compareRowText(row),indent=compareIndent(text);
  const indentLanguages=new Set(['python','yaml','nim']);
  if(indentLanguages.has(language)){
    let start=-1,parentIndent=-1;
    for(let i=index;i>=Math.max(0,index-80);i--){
      const candidate=compareRowText(model.rows[i]);if(!candidate.trim())continue;
      const candidateIndent=compareIndent(candidate);
      if(i===index&&candidate.trimEnd().endsWith(':')){start=i;parentIndent=candidateIndent;break;}
      if(candidateIndent<indent&&candidate.trimEnd().endsWith(':')){start=i;parentIndent=candidateIndent;break;}
    }
    if(start>=0){
      let end=index;
      for(let i=Math.max(index+1,start+1);i<Math.min(model.rows.length,start+160);i++){
        const candidate=compareRowText(model.rows[i]);if(!candidate.trim()){end=i;continue;}
        if(compareIndent(candidate)<=parentIndent){break;}
        end=i;
      }
      if(end-start<160)return [start,end];
    }
  }
  const braceLanguages=new Set(['cpp','go','javascript','typescript','java','php','rust','csharp','kotlin','dart','protobuf','actionscript','css','scss','less']);
  if(braceLanguages.has(language)){
    let start=-1;
    for(let i=index;i>=Math.max(0,index-80);i--){
      const code=compareStructuralCode(model.rows[i]);
      if(code.includes('{')&&compareIndent(compareRowText(model.rows[i]))<=indent){start=i;break;}
    }
    if(start>=0){
      let depth=0,opened=false,end=index;
      for(let i=start;i<Math.min(model.rows.length,start+160);i++){
        const code=compareStructuralCode(model.rows[i]);
        for(const ch of code){if(ch==='{'){depth++;opened=true;}else if(ch==='}')depth--;}
        end=i;
        if(opened&&i>=index&&depth<=0)break;
      }
      if(opened&&end-start<160)return [start,end];
    }
  }
  return compareFallbackContext(model,index);
}
function compareVisibleIndexes(model){
  const visibleChange=index=>{
    const row=model.rows[index];
    return row.hunk>=0&&(showUnimportant||row.importance!=='unimportant');
  };
  if(contentMode==='all'){
    return model.rows.map((_,index)=>index).filter(index=>{
      const row=model.rows[index];
      return row.hunk<0||showUnimportant||row.importance!=='unimportant';
    });
  }
  const changed=model.rows.map((_,index)=>index).filter(visibleChange);
  if(contentMode==='diff')return changed;
  const indexes=new Set(changed);
  for(const index of changed){
    const [start,end]=compareStructuralContextRange(model,index);
    for(let i=start;i<=end;i++)indexes.add(i);
  }
  return [...indexes].sort((a,b)=>a-b).filter(index=>{
    const row=model.rows[index];
    return row.hunk<0||showUnimportant||row.importance!=='unimportant';
  });
}
function appendCompareGap(parent,count){
  if(count<=0)return;
  const gap=document.createElement('div');gap.className='file-compare-gap';gap.textContent='⋯ '+count+' line'+(count===1?'':'s')+' hidden ⋯';parent.append(gap);
}
function syncCompareFilterButtons(){
  viewAllButton.classList.toggle('active',contentMode==='all');
  viewDiffButton.classList.toggle('active',contentMode==='diff');
  viewContextButton.classList.toggle('active',contentMode==='context');
  viewUnimportantButton.classList.toggle('active',showUnimportant);
  viewUnimportantButton.setAttribute('aria-pressed',showUnimportant?'true':'false');
  viewUnimportantButton.textContent=showUnimportant?'View unimportant ✓':'View unimportant';
}
function renderClippedCompareCode(code,text,tokens=[],changed=[],changeKind=''){
  const inner=document.createElement('span');inner.className='file-compare-code-inner';
  renderCompareCode(inner,text,tokens,changed,changeKind);
  code.append(inner);
}
function renderCompareCode(node,text,tokens=[],changed=[],changeKind=''){
  text=String(text??'');
  if(!text){node.textContent='\u00a0';return;}
  const points=new Set([0,text.length]);
  for(const token of tokens){points.add(token.from);points.add(token.to);}
  for(const range of changed){points.add(range.from);points.add(range.to);}
  const sorted=[...points].filter(value=>value>=0&&value<=text.length).sort((a,b)=>a-b);
  for(let index=0;index<sorted.length-1;index++){
    const from=sorted[index],to=sorted[index+1];if(to<=from)continue;
    const span=document.createElement('span'),syntax=tokens.find(token=>from>=token.from&&from<token.to);
    if(syntax)span.classList.add('file-compare-syntax-'+syntax.type);
    if(compareRangeContains(changed,from))span.classList.add('file-compare-inline-change',changeKind);
    span.textContent=text.slice(from,to);node.append(span);
  }
}

function splitLines(text){return String(text??'').replace(/\r\n/g,'\n').replace(/\r/g,'\n').split('\n');}
function commonPrefix(a,b){let i=0;while(i<a.length&&i<b.length&&a[i]===b[i])i++;return i;}
function commonSuffix(a,b,prefix){let i=0;while(a.length-1-i>=prefix&&b.length-1-i>=prefix&&a[a.length-1-i]===b[b.length-1-i])i++;return i;}
function lcsBlock(a,b){
  const n=a.length,m=b.length;
  if(!n)return b.map(text=>({kind:'add',text}));
  if(!m)return a.map(text=>({kind:'remove',text}));
  if(n*m>1200000){
    return [...a.map(text=>({kind:'remove',text})),...b.map(text=>({kind:'add',text}))];
  }
  const dp=Array.from({length:n+1},()=>new Uint32Array(m+1));
  for(let i=n-1;i>=0;i--)for(let j=m-1;j>=0;j--)dp[i][j]=a[i]===b[j]?dp[i+1][j+1]+1:Math.max(dp[i+1][j],dp[i][j+1]);
  const ops=[];let i=0,j=0;
  while(i<n&&j<m){
    if(a[i]===b[j]){ops.push({kind:'equal',text:a[i]});i++;j++;}
    else if(dp[i+1][j]>=dp[i][j+1]){ops.push({kind:'remove',text:a[i++]});}
    else{ops.push({kind:'add',text:b[j++]});}
  }
  while(i<n)ops.push({kind:'remove',text:a[i++]});
  while(j<m)ops.push({kind:'add',text:b[j++]});
  return ops;
}
function diffOperations(leftText,rightText){
  const a=splitLines(leftText),b=splitLines(rightText),prefix=commonPrefix(a,b),suffix=commonSuffix(a,b,prefix);
  const ops=[];
  for(let i=0;i<prefix;i++)ops.push({kind:'equal',text:a[i]});
  ops.push(...lcsBlock(a.slice(prefix,a.length-suffix),b.slice(prefix,b.length-suffix)));
  for(let i=suffix;i>0;i--)ops.push({kind:'equal',text:a[a.length-i]});
  return ops;
}
function buildCompareModel(leftText,rightText){
  const ops=diffOperations(leftText,rightText);
  const rows=[];const hunks=[];
  let leftNo=1,rightNo=1,hunk=null,pendingRemoved=[],pendingAdded=[];
  const flushChange=()=>{
    if(!pendingRemoved.length&&!pendingAdded.length)return;
    const count=Math.max(pendingRemoved.length,pendingAdded.length);
    if(!hunk)hunk={index:hunks.length,leftStart:leftNo-1,rightStart:rightNo-1,leftCount:0,rightCount:0,leftLines:[],rightLines:[],rowStart:rows.length};
    for(let i=0;i<count;i++){
      const l=i<pendingRemoved.length?{no:leftNo++,text:pendingRemoved[i],kind:'removed'}:null;
      const r=i<pendingAdded.length?{no:rightNo++,text:pendingAdded[i],kind:'added'}:null;
      if(l){hunk.leftCount++;hunk.leftLines.push(l.text);}if(r){hunk.rightCount++;hunk.rightLines.push(r.text);}
      rows.push({left:l,right:r,hunk:hunk.index});
    }
    pendingRemoved=[];pendingAdded=[];
  };
  for(const op of ops){
    if(op.kind==='remove'){pendingRemoved.push(op.text);continue;}
    if(op.kind==='add'){pendingAdded.push(op.text);continue;}
    flushChange();
    if(hunk){hunk.rowEnd=rows.length;hunks.push(hunk);hunk=null;}
    rows.push({left:{no:leftNo++,text:op.text,kind:'context'},right:{no:rightNo++,text:op.text,kind:'context'},hunk:-1});
  }
  flushChange();if(hunk){hunk.rowEnd=rows.length;hunks.push(hunk);}
  return {rows,hunks,identical:hunks.length===0};
}
// Reuse the SAME language-aware row criteria for Directory Compare without
// mutating the open File Compare tab or its syntax cache.
function analyzeCompareTexts(leftText,rightText,path=''){
  const model=buildCompareModel(leftText,rightText);
  const leftTokens=compareSyntaxSource(path,leftText),rightTokens=compareSyntaxSource(path,rightText);
  const language=compareLanguageID(path);
  let important=0,unimportant=0,added=0,removed=0,modified=0;
  for(const row of model.rows){
    if(row.hunk<0)continue;
    const lt=row.left?.text??'',rt=row.right?.text??'';
    const l=row.left?leftTokens[row.left.no-1]||[]:[],r=row.right?rightTokens[row.right.no-1]||[]:[];
    const importance=classifyCompareTextRow(row,l,r,language);
    if(importance==='unimportant')unimportant++;else important++;
    if(row.left&&row.right)modified++;
    else if(row.left)removed++;
    else if(row.right)added++;
  }
  return {identical:model.identical,important,unimportant,added,removed,modified,hunks:model.hunks.length};
}
function replaceLineRange(text,start,count,replacementLines){
  const lines=splitLines(text);
  lines.splice(start,count,...replacementLines);
  return lines.join('\n');
}
async function loadSource(source){
  const loaded=await source.load();
  source.text=String(loaded?.text??loaded?.content??'');
  source.baselineText=source.text;source.dirty=false;source.saving=false;
  if(loaded&&typeof loaded==='object')Object.assign(source.meta||(source.meta={}),loaded);
  return source;
}
async function saveSource(source,text){
  const saver=typeof source.saveText==='function'?source.saveText:source.writeText;
  if(typeof saver!=='function')throw new Error((source.label||'Compare side')+' is read-only');
  const result=await saver(text,source);
  // Never overwrite an editor buffer that changed while the remote write was in flight.
  if(result&&typeof result==='object')Object.assign(source.meta||(source.meta={}),result);
}
function cell(spec,side,changed=[],importance='context'){
  const node=document.createElement('div');node.className='file-compare-cell '+(spec?.kind||'blank')+(spec&&spec.kind!=='context'?' '+importance:'');
  node.dataset.compareSide=side;
  const no=document.createElement('span');no.className='file-compare-no';no.textContent=spec?.no?String(spec.no):'';
  const code=document.createElement('span');code.className='file-compare-code';
  const syntax=ensureCompareSyntax();
  const tokens=spec?.no?(side==='left'?syntax.left:syntax.right)[spec.no-1]||[]:[];
  renderClippedCompareCode(code,spec?spec.text:'',tokens,changed,spec?.kind==='removed'?'removed':spec?.kind==='added'?'added':'');
  node.append(no,code);return node;
}
function renderSide(model,indexes){
  const frag=document.createDocumentFragment(),shownHunks=new Set();let previous=-1;
  for(const index of indexes){
    const row=model.rows[index];if(previous>=0&&index>previous+1)appendCompareGap(frag,index-previous-1);previous=index;
    if(row.hunk>=0&&!shownHunks.has(row.hunk)){
      shownHunks.add(row.hunk);const h=model.hunks[row.hunk];
      const card=document.createElement('div');card.className='file-compare-hunk';
      const hh=document.createElement('div');hh.className='file-compare-hunk-head';
      const hRows=model.rows.slice(h.rowStart,h.rowEnd),important=hRows.filter(item=>item.importance==='important').length,unimportant=hRows.filter(item=>item.importance==='unimportant').length;
      const label=document.createElement('span');label.className='file-compare-hunk-label';label.textContent='Change '+(h.index+1)+' · left '+(h.leftStart+1)+'+'+h.leftCount+' ↔ right '+(h.rightStart+1)+'+'+h.rightCount;
      const actions=document.createElement('span');actions.className='file-compare-hunk-actions';
      if(sourceWritable(current?.right)){const b=document.createElement('button');b.type='button';b.textContent='Left → Right';b.title='Apply the entire change block, including rows hidden by filters';b.onclick=()=>copyHunk(h,'left-to-right').catch(app.showError);actions.append(b);}
      if(sourceWritable(current?.left)){const b=document.createElement('button');b.type='button';b.textContent='Right → Left';b.title='Apply the entire change block, including rows hidden by filters';b.onclick=()=>copyHunk(h,'right-to-left').catch(app.showError);actions.append(b);}
      const info=document.createElement('span');info.className='file-compare-hunk-info';info.textContent=important+' important / '+unimportant+' unimportant';
      hh.append(label,actions,info);card.append(hh);frag.append(card);
    }
    const ranges=compareInlineRanges(row.left?.text||'',row.right?.text||'');
    const line=document.createElement('div');line.className='file-compare-row';line.dataset.compareIndex=String(index);line.append(cell(row.left,'left',ranges.left,row.importance),cell(row.right,'right',ranges.right,row.importance));frag.append(line);
  }
  body.append(frag);
}
function renderInline(model,indexes){
  const host=document.createElement('div');host.className='file-compare-inline';let previous=-1;
  for(const index of indexes){
    if(previous>=0&&index>previous+1)appendCompareGap(host,index-previous-1);previous=index;
    const row=model.rows[index];
    if(row.left&&row.right&&row.left.kind==='context'){
      const line=document.createElement('div');line.className='file-compare-inline-line';
      const no=document.createElement('span');no.className='file-compare-inline-no';no.textContent=String(row.left.no);
      const mark=document.createElement('span');mark.textContent=' ';
      const code=document.createElement('span');code.className='file-compare-inline-code';const syntax=ensureCompareSyntax();renderClippedCompareCode(code,row.left.text,syntax.left[row.left.no-1]||[]);line.append(no,mark,code);host.append(line);continue;
    }
    if(row.left){
      const line=document.createElement('div');line.className='file-compare-inline-line removed '+row.importance;
      const no=document.createElement('span');no.className='file-compare-inline-no';no.textContent=String(row.left.no);
      const mark=document.createElement('span');mark.textContent='−';const code=document.createElement('span');code.className='file-compare-inline-code';const syntax=ensureCompareSyntax();const ranges=compareInlineRanges(row.left.text,row.right?.text||'');renderClippedCompareCode(code,row.left.text,syntax.left[row.left.no-1]||[],ranges.left,'removed');line.append(no,mark,code);host.append(line);
    }
    if(row.right){
      const line=document.createElement('div');line.className='file-compare-inline-line added '+row.importance;
      const no=document.createElement('span');no.className='file-compare-inline-no';no.textContent=String(row.right.no);
      const mark=document.createElement('span');mark.textContent='+';const code=document.createElement('span');code.className='file-compare-inline-code';const syntax=ensureCompareSyntax();const ranges=compareInlineRanges(row.left?.text||'',row.right.text);renderClippedCompareCode(code,row.right.text,syntax.right[row.right.no-1]||[],ranges.right,'added');line.append(no,mark,code);host.append(line);
    }
  }
  body.append(host);
}
function compareTabSourceName(source){
  return String(source?.path||source?.label||'file').split(/[\\/]/).pop()||'file';
}
function compactCompareTabName(name,limit){
  if(name.length<=limit)return name;
  const dot=name.lastIndexOf('.');
  const extension=dot>0?name.slice(dot):'';
  const tailLength=extension&&extension.length<=10?extension.length:Math.min(7,Math.floor(limit/2));
  return name.slice(0,Math.max(1,limit-tailLength-1))+'…'+name.slice(-tailLength);
}
function compareTabDisplayNames(left,right){
  const a=compareTabSourceName(left),b=compareTabSourceName(right);
  // Reserve visible space for BOTH filenames, giving unused capacity to the longer name.
  let aLimit=Math.min(17,a.length),bLimit=Math.min(17,b.length);
  let spare=34-aLimit-bLimit;
  const aExtra=Math.min(spare,a.length-aLimit);aLimit+=aExtra;spare-=aExtra;
  bLimit+=Math.min(spare,b.length-bLimit);
  return [compactCompareTabName(a,aLimit),compactCompareTabName(b,bLimit)];
}
function updateCompareTabLabel(left,right){
  if(!compareTab)return;
  const [leftName,rightName]=compareTabDisplayNames(left,right);
  const label=compareTab.querySelector('.file-compare-tab-label');
  const leftSpan=document.createElement('span');leftSpan.className='file-compare-tab-filename';leftSpan.textContent=leftName;
  const separator=document.createElement('span');separator.className='file-compare-tab-separator';separator.textContent='↔';separator.setAttribute('aria-hidden','true');
  const rightSpan=document.createElement('span');rightSpan.className='file-compare-tab-filename';rightSpan.textContent=rightName;
  label.replaceChildren(leftSpan,separator,rightSpan);
  const fullName=source=>String(source?.path||source?.label||'file');
  compareTab.title=fullName(left)+' ↔ '+fullName(right);
  compareTab.dataset.title=compareTab.title;
  compareTab.setAttribute('aria-label','Compare '+fullName(left)+' and '+fullName(right));
}
function render(){
  selectedCompareLines=null;body.replaceChildren();scheduleCompareHorizontalMeasure();if(!current)return;
  const model=enrichCompareModel(buildCompareModel(current.left.text,current.right.text));
  model.sourceText={left:current.left.text,right:current.right.text};current.model=model;
  leftLabel.textContent=current.left.label||'Left';rightLabel.textContent=current.right.label||'Right';
  title.textContent=(current.title||'File Compare')+' · '+leftLabel.textContent+' ↔ '+rightLabel.textContent;
  if(compareTab){
    updateCompareTabLabel(current.left,current.right);
  }
  summary.textContent=model.identical?'identical':model.hunks.length+' change block'+(model.hunks.length===1?'':'s')+' · '+model.stats.important+' important · '+model.stats.unimportant+' unimportant';
  if(current.gitContext){
    summary.textContent+=' · Git snapshots read-only · Save local only; never auto-stage';
    const writableLocal=current.gitContext.mode!=='staged'&&sourceWritable(current.right);
    editButton.textContent=writableLocal?'Edit local file':'Git snapshots (read-only)';
    editButton.title=writableLocal?'Edit and explicitly save only the local working-tree file; Git HEAD/index and staging remain untouched':'Both sides are Git snapshots; select Open local WORKTREE to edit your local file';
    editButton.disabled=!writableLocal;
    gitLocalButton.hidden=current.gitContext.mode!=='staged';
  }else{
    editButton.textContent='Edit';editButton.disabled=false;editButton.title='Edit both writable files with syntax highlighting';
    gitLocalButton.hidden=true;
  }
  sideButton.classList.toggle('active',viewMode==='side');inlineButton.classList.toggle('active',viewMode==='inline');columns.style.display=viewMode==='side'?'grid':'none';syncCompareFilterButtons();
  syncCompareSaveState('left');syncCompareSaveState('right');
  if(model.identical){const empty=document.createElement('div');empty.className='file-compare-empty';empty.textContent='No differences';body.append(empty);return;}
  const indexes=compareVisibleIndexes(model);
  if(!indexes.length){const empty=document.createElement('div');empty.className='file-compare-empty';empty.textContent='No differences match the current filters';body.append(empty);return;}
  if(viewMode==='inline')renderInline(model,indexes);else renderSide(model,indexes);
  globalThis.TaskMenuWorkspaceTabs?.scheduleSave?.();
}
// A browser text selection is interpreted as complete source lines, not a partial character edit.
// Use the aligned diff rows to replace only the corresponding target range within one hunk.
function ensureFreshCompareModel(){
  if(!current?.model||current.model.sourceText?.left!==current.left.text||current.model.sourceText?.right!==current.right.text){
    throw new Error('File Compare changed while this selection was open. Select the lines again.');
  }
}
function selectedLinePatch(model,side,indexes){
  if(side!=='left'&&side!=='right')throw new Error('Select one compare side');
  const positions=[...new Set(indexes)].sort((a,b)=>a-b);
  if(!positions.length||positions.some(i=>!Number.isInteger(i)||i<0||i>=model.rows.length))throw new Error('Select at least one changed source line');
  const first=model.rows[positions[0]],last=model.rows[positions[positions.length-1]];
  if(first.hunk<0||first.hunk!==last.hunk)throw new Error('Select changed lines within one diff block');
  const hunk=model.hunks[first.hunk],sourceSide=side,targetSide=side==='left'?'right':'left';
  if(positions.some(i=>model.rows[i].hunk!==hunk.index||!model.rows[i][sourceSide]))throw new Error('Selected lines must belong to one side of one change block');
  const sourceLines=positions.map(i=>model.rows[i][sourceSide].text);
  // Map each selected source row separately: target-only rows *between* selected lines
  // are not selected and must not be removed by a broad range replacement.
  const steps=positions.map(index=>{
    const row=model.rows[index];
    const preceding=model.rows.slice(hunk.rowStart,index).filter(item=>item[targetSide]).length;
    return {index,start:(targetSide==='right'?hunk.rightStart:hunk.leftStart)+preceding,count:row[targetSide]?1:0,lines:[row[sourceSide].text]};
  });
  steps.sort((a,b)=>b.start-a.start||b.index-a.index);
  return {side:targetSide,steps,sourceLines:sourceLines.length};
}
function applySelectedLinePatch(text,patch){
  for(const step of patch.steps)text=replaceLineRange(text,step.start,step.count,step.lines);
  return text;
}
function compareSelectionSide(node){
  const element=node instanceof Element?node:node?.parentElement;
  return element?.closest?.('.file-compare-cell[data-compare-side]')?.dataset.compareSide||'';
}
function markSelectedCompareLines(){
  body.querySelectorAll('.compare-line-selected').forEach(node=>node.classList.remove('compare-line-selected'));
  body.querySelectorAll('.file-compare-line-copy').forEach(node=>node.remove());
  const selected=selectedCompareLines;
  if(!selected||!current?.model||selected.model!==current.model)return;
  const targetSide=selected.side==='left'?'right':'left';
  if(!sourceWritable(current[targetSide]))return;
  for(const index of selected.indexes){
    const row=body.querySelector('.file-compare-row[data-compare-index="'+index+'"]');
    row?.querySelector('.file-compare-cell[data-compare-side="'+selected.side+'"]')?.classList.add('compare-line-selected');
  }
  const first=body.querySelector('.file-compare-row[data-compare-index="'+selected.indexes[selected.indexes.length-1]+'"]');
  if(!first)return;
  const button=document.createElement('button');button.type='button';button.className='file-compare-line-copy';
  button.textContent=selected.side==='left'?'Copy to right →':'← Copy to left';
  button.title='Copy only the selected full lines; edits remain unsaved until Save';
  button.onmousedown=event=>event.preventDefault();
  button.onclick=event=>{event.stopPropagation();copySelectedCompareLines(selected).catch(app.showError);};
  first.append(button);
}
function updateSelectedCompareLines(){
  selectedCompareLines=null;
  if(viewMode!=='side'||!current?.model)return markSelectedCompareLines();
  const selection=window.getSelection?.();
  if(!selection||selection.isCollapsed||selection.rangeCount!==1)return markSelectedCompareLines();
  const side=compareSelectionSide(selection.anchorNode);
  if(!side||side!==compareSelectionSide(selection.focusNode))return markSelectedCompareLines();
  const range=selection.getRangeAt(0),indexes=[];
  for(const cell of body.querySelectorAll('.file-compare-cell[data-compare-side="'+side+'"]')){
    const code=cell.querySelector('.file-compare-code');
    if(!code||!range.intersectsNode(code))continue;
    const row=cell.closest('.file-compare-row'),index=Number(row?.dataset.compareIndex);
    if(!Number.isInteger(index)||!current.model.rows[index]?.[side])continue;
    indexes.push(index);
  }
  try{
    selectedLinePatch(current.model,side,indexes);
    selectedCompareLines={model:current.model,side,indexes};
  }catch{}
  markSelectedCompareLines();
}
async function copySelectedCompareLines(selected){
  if(!current||selected!==selectedCompareLines||current.model!==selected.model)return;
  ensureFreshCompareModel();
  const patch=selectedLinePatch(current.model,selected.side,selected.indexes);
  const target=current[patch.side];
  if(!sourceWritable(target))throw new Error('Destination file is read-only');
  if(!window.confirm('Copy '+patch.sourceLines+' selected line(s) '+(selected.side==='left'?'left → right':'right → left')+'?\n\nOnly the selected rows will be replaced. Save remains manual.'))return;
  setSourceBuffer(patch.side,applySelectedLinePatch(target.text,patch),{syncEditor:true,immediate:true});
}
body.addEventListener('mouseup',event=>{if(!event.target?.closest?.('button'))updateSelectedCompareLines();});
body.addEventListener('keyup',event=>{if(event.key==='Shift'||event.shiftKey)updateSelectedCompareLines();});

async function copyHunk(hunk,direction){
  if(!current)return;
  ensureFreshCompareModel();
  if(current.model.hunks[hunk.index]!==hunk)throw new Error('Diff block changed. Reload the current comparison.');
  const from=direction==='left-to-right'?current.left:current.right;
  const targetSide=direction==='left-to-right'?'right':'left';
  const to=compareSideSource(targetSide);
  if(!sourceWritable(to))throw new Error((to?.label||'Destination')+' is read-only');
  const start=direction==='left-to-right'?hunk.rightStart:hunk.leftStart;
  const count=direction==='left-to-right'?hunk.rightCount:hunk.leftCount;
  const replacement=direction==='left-to-right'?hunk.leftLines:hunk.rightLines;
  const next=replaceLineRange(to.text,start,count,replacement);
  if(!window.confirm('Apply change '+(hunk.index+1)+' from '+(from.label||'source')+' to '+(to.label||'destination')+'?\n\nThe destination will be marked unsaved until you press Save.'))return;
  setSourceBuffer(targetSide,next,{syncEditor:true,immediate:true});
}
async function reload(){
  if(!current)return false;
  if(compareHasSaving()){window.alert('A compare file is still being saved. Finish that operation before reloading.');return false;}
  const original=current;
  const refreshed={...original,left:{...original.left,meta:{...(original.left.meta||{})}},right:{...original.right,meta:{...(original.right.meta||{})}}};
  // Reload into detached source copies so a failed read leaves both visible buffers untouched.
  await Promise.all([loadSource(refreshed.left),loadSource(refreshed.right)]);
  if(current!==original)return false;
  if(compareHasSaving()){window.alert('A compare file started saving during reload. Retry when the save finishes.');return false;}
  if(compareHasDirty()&&!window.confirm('Discard unsaved compare edits and reload both files?'))return false;
  current=refreshed;current.syntax=null;
  if(editMode)ensureCompareEditors();
  render();return true;
}
async function open(options){
  if(!options?.left?.load||!options?.right?.load)throw new Error('File compare requires left and right sources');
  if(compareHasSaving()){window.alert('A compare file is still being saved. Finish that operation before opening another comparison.');return false;}
  // Prepare both files first: a failed remote read must not destroy the current comparison.
  const next={title:options.title||'File Compare',gitContext:options.gitContext||null,left:{...options.left,meta:{...(options.left.meta||{})}},right:{...options.right,meta:{...(options.right.meta||{})}}};
  await Promise.all([loadSource(next.left),loadSource(next.right)]);
  if(compareHasSaving()){window.alert('A compare file started saving while sources were loading. Try again after the save finishes.');return false;}
  if(compareHasDirty()&&!window.confirm('Discard unsaved changes in the current File Compare?'))return false;
  destroyCompareEditors();editMode=false;editorDeck.classList.add('hidden');editButton.classList.remove('active');
  current=next;ensureCompareTab();render();activateCompareTab();
  window.dispatchEvent(new Event('taskmenu:workspace-tab-changed'));return true;
}
function close({force=false}={}){
  if(!force&&compareHasSaving()){window.alert('A compare file is still being saved. Finish that operation before closing.');return false;}
  if(!force&&compareHasDirty()&&!window.confirm('Close File Compare and discard unsaved changes?'))return false;
  const wasActive=Boolean(compareTab?.classList.contains('active'));
  destroyCompareEditors();current=null;body.replaceChildren();
  try{sessionStorage.removeItem(compareDraftKey());}catch{}
  window.dispatchEvent(new Event('taskmenu:workspace-tab-changed'));
  compareTab?.remove();compareTab=null;pane.classList.add('hidden');
  if(wasActive){
    const next=lastWorkspaceTab?.isConnected?lastWorkspaceTab:tabsHost.querySelector('.tab,.db-tab');
    if(next?.click)next.click();else app.activateExternalView('empty',{force:true});
  }
  return true;
}
function projectSource(pathValue,{writable=!app.sharedMode||Boolean(app.hasPermission?.('files.write')||app.hasPermission?.('project.admin')),label=''}={}){
  pathValue=String(pathValue||'').trim();
  const source={kind:'project',path:pathValue,label:label||pathValue,meta:{},load:async()=>{
    const file=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(pathValue));
    return {text:file.content,sha256:file.sha256,file};
  }};
  if(writable)source.writeText=async(text,activeSource=source)=>{
    const sha=activeSource.meta?.sha256||activeSource.meta?.file?.sha256;
    if(!sha)throw new Error('Project compare source SHA is unavailable');
    const saved=await app.jsonFetch('/api/project/file',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({path:pathValue,content:text,expected_sha256:sha})});
    return {text:saved.content,sha256:saved.sha256,file:saved};
  };
  return source;
}
function editorSource(view,{label='Current editor'}={}){
  const source={kind:'editor',editorID:String(view?.id||''),path:view?.file?.path||'',label:(label+' · '+(view?.file?.path||'')),meta:{},load:async()=>{
    if(view?.closed)throw new Error('Editor is closed');
    const text=view.cm.state.doc.toString();return {text,editor_snapshot:text};
  }};
  if(view?.file?.read_only||view?.tabReadOnly||globalThis.TaskMenuEditor?.isTabReadOnly?.(view))return source;
  source.saveText=async(text,activeSource=source)=>{
    if(view.closed)throw new Error('Editor is closed');
    const currentText=view.cm.state.doc.toString(),expected=String(activeSource.meta?.editor_snapshot??currentText);
    if(currentText!==expected)throw new Error('Editor changed after File Compare opened. Reload compare before saving this side.');
    const doc=view.cm.state.doc;view.cm.dispatch({changes:{from:0,to:doc.length,insert:text}});
    await globalThis.TaskMenuEditor?.saveEditor?.(view);
    if(view.dirty)throw new Error('Editor still has unsaved changes after save');
    const savedText=view.cm.state.doc.toString();
    return {text:savedText,editor_snapshot:savedText,sha256:String(view.file?.sha256||''),file:view.file};
  };
  source.writeText=source.saveText;
  return source;
}
function savedEditorSource(view){
  return projectSource(view?.file?.path,{writable:false,label:'Saved · '+(view?.file?.path||'')});
}
function clipboardSource({label='Clipboard'}={}){
  return {kind:'clipboard',label,load:async()=>{
    if(!navigator.clipboard?.readText)throw new Error('Clipboard read is unavailable in this browser/context');
    return {text:await navigator.clipboard.readText()};
  }};
}
function remoteSource(profileID,pathValue,{writable=true,label=''}={}){
  profileID=String(profileID||'').trim();pathValue=String(pathValue||'').trim();
  const source={kind:'remote',profileID,path:pathValue,label:label||('Remote · '+pathValue),meta:{},load:async()=>{
    const params=new URLSearchParams({profile_id:profileID,path:pathValue});
    const data=await app.jsonFetch('/api/file-transfer/text?'+params.toString());
    return {text:data.content,sha256:data.sha256,profile_id:data.profile_id,path:data.path};
  }};
  if(writable)source.writeText=async(text,activeSource=source)=>{
    const sha=activeSource.meta?.sha256;
    if(!sha)throw new Error('Remote compare source SHA is unavailable');
    return app.jsonFetch('/api/file-transfer/text',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:profileID,path:pathValue,content:text,expected_sha256:sha})});
  };
  return source;
}
function browserFileHandleSource(handle,{label='Local browser file',path=''}={}){
  if(!handle||handle.kind!=='file')throw new Error('Local browser file handle is required');
  const source={kind:'browser-local',path,label,meta:{},load:async()=>{
    const file=await handle.getFile();
    return {text:await file.text(),size:file.size,lastModified:file.lastModified};
  },writeText:async(text,activeSource=source)=>{
    const before=await handle.getFile();
    if(activeSource.meta?.lastModified!==undefined&&(before.lastModified!==activeSource.meta.lastModified||before.size!==activeSource.meta.size))throw new Error('Local browser file changed after compare load');
    if(typeof handle.createWritable!=='function')throw new Error('Local browser file is read-only');
    const writer=await handle.createWritable();
    try{await writer.write(text);await writer.close();}catch(error){try{await writer.abort?.();}catch{}throw error;}
    const after=await handle.getFile();
    return {text,size:after.size,lastModified:after.lastModified};
  }};
  if(typeof handle.createWritable!=='function')delete source.writeText;
  return source;
}
async function openLeftRemote(leftSource,profileID,remotePath){
  return open({title:'Left ↔ Remote',left:leftSource,right:remoteSource(profileID,remotePath)});
}
function gitCommitSource(repoID,pathValue,ref,{label='',allowMissing=false}={}){
  repoID=String(repoID||'').trim();pathValue=String(pathValue||'').trim();ref=String(ref||'').trim();
  return {kind:'git-commit',label:label||('Git '+ref.slice(0,8)+' · '+pathValue),repoID,path:pathValue,ref,load:async()=>{
    const params=new URLSearchParams({view:'file-content',repo:repoID,path:pathValue,ref});
    if(allowMissing)params.set('allow_missing','1');
    const data=await app.jsonFetch('/api/git/status?'+params.toString());
    return {text:data.content,exists:data.exists!==false,commit:data.commit,ref:data.ref,path:data.path,repo_id:data.repo_id};
  }};
}
function emptyCompareSource(label='(file absent)'){
  return {kind:'empty',label,load:async()=>({text:'',exists:false})};
}
async function openGitCommitFileDiffBetween(repoID,file,leftRef,rightRef,{title='Commit ↔ Commit file diff'}={}){
  const pathValue=String(file?.path||'').trim(),oldPath=String(file?.old_path||pathValue).trim();
  if(!pathValue||!leftRef||!rightRef)throw new Error('Commit file compare requires path and both commits');
  const left=gitCommitSource(repoID,oldPath,leftRef,{label:'Git '+String(leftRef).slice(0,8)+' · '+oldPath,allowMissing:true});
  const right=gitCommitSource(repoID,pathValue,rightRef,{label:'Git '+String(rightRef).slice(0,8)+' · '+pathValue,allowMissing:true});
  return open({title,left,right});
}
async function openGitCommitFileDiff(repoID,file,commit){
  const pathValue=String(file?.path||'').trim(),oldPath=String(file?.old_path||pathValue).trim();
  if(!pathValue||!commit?.sha)throw new Error('Commit file compare requires file path and commit');
  const parents=Array.isArray(commit.parents)?commit.parents.filter(Boolean):[];
  let parent=parents[0]||'';
  if(parents.length>1){
    const answer=window.prompt('Merge commit has '+parents.length+' parents. Choose parent number to compare against:', '1');
    if(answer===null)return;
    const index=Number(answer)-1;
    if(!Number.isInteger(index)||index<0||index>=parents.length)throw new Error('Parent number must be between 1 and '+parents.length);
    parent=parents[index];
  }
  const status=String(file.status||'');
  const left=parent?gitCommitSource(repoID,oldPath,parent,{label:'Parent '+parent.slice(0,8)+' · '+oldPath,allowMissing:true}):emptyCompareSource('Parent · file absent');
  const right=gitCommitSource(repoID,pathValue,commit.sha,{label:'Commit '+commit.short+' · '+pathValue,allowMissing:true});
  return open({title:'Commit file diff · '+status,left,right});
}
function gitStateSource(repoID,pathValue,state,{label=''}={}){
  repoID=String(repoID||'').trim();pathValue=String(pathValue||'').trim();state=String(state||'').trim().toLowerCase();
  if(state!=='head'&&state!=='index')throw new Error('Git compare state must be head or index');
  const defaultLabel=state==='head'?'HEAD · committed':'INDEX · staged';
  return {kind:'git-'+state,label:label||(defaultLabel+' · '+pathValue),repoID,path:pathValue,state,load:async()=>{
    const params=new URLSearchParams({view:'file-content',repo:repoID,path:pathValue,state,allow_missing:'1'});
    const data=await app.jsonFetch('/api/git/status?'+params.toString());
    return {text:data.content,exists:data.exists!==false,commit:data.commit,state:data.state,path:data.path,repo_id:data.repo_id};
  }};
}
function editorForProjectPath(pathValue){
  pathValue=String(pathValue||'').replace(/\\/g,'/').replace(/^\.\//,'');
  const editor=globalThis.TaskMenuEditor;
  if(!editor?.editors)return null;
  for(const view of editor.editors.values()){
    if(view?.closed)continue;
    const current=String(view?.file?.path||'').replace(/\\/g,'/').replace(/^\.\//,'');
    if(current===pathValue)return view;
  }
  return null;
}
function workingProjectSource(pathValue,{label=''}={}){
  const view=editorForProjectPath(pathValue);
  if(view)return editorSource(view,{label:label||'WORKTREE editor'});
  return projectSource(pathValue,{label:label||('WORKTREE · '+pathValue)});
}
async function openGitCommitAgainstProject(repoID,repoPath,workspacePath,ref){
  return open({title:'Git commit ↔ Current',left:gitCommitSource(repoID,repoPath,ref),right:workingProjectSource(workspacePath,{label:'CURRENT · '+workspacePath})});
}
async function openGitCommits(repoID,pathValue,leftRef,rightRef){
  return open({title:'Git commit ↔ Git commit',left:gitCommitSource(repoID,pathValue,leftRef),right:gitCommitSource(repoID,pathValue,rightRef)});
}
async function openGitStatePair(repoID,repoPath,workspacePath,mode){
  mode=String(mode||'').trim();
  const gitContext={repoID,repoPath,workspacePath,mode};
  if(mode==='staged'){
    return open({title:'HEAD ↔ Staged',gitContext,left:gitStateSource(repoID,repoPath,'head'),right:gitStateSource(repoID,repoPath,'index')});
  }
  if(mode==='worktree'){
    return open({title:'Staged ↔ Working',gitContext,left:gitStateSource(repoID,repoPath,'index'),right:workingProjectSource(workspacePath,{label:'WORKTREE · '+workspacePath})});
  }
  if(mode==='head-worktree'){
    return open({title:'HEAD ↔ Working',gitContext,left:gitStateSource(repoID,repoPath,'head'),right:workingProjectSource(workspacePath,{label:'WORKTREE · '+workspacePath})});
  }
  throw new Error('Unsupported Git compare mode '+mode);
}
gitLocalButton.onclick=()=>{
  const selected=current?.gitContext;
  if(!selected)return;
  return openGitStatePair(selected.repoID,selected.repoPath,selected.workspacePath,'head-worktree').catch(app.showError);
};
async function openProjectFiles(leftPath,rightPath){return open({title:'Project files',left:projectSource(leftPath),right:projectSource(rightPath)});}
async function openEditorSaved(view){return open({title:'Current ↔ Saved',left:editorSource(view),right:savedEditorSource(view)});}
async function openEditorClipboard(view){return open({title:'Current ↔ Clipboard',left:editorSource(view),right:clipboardSource()});}
async function promptProjectCompare(pathValue){
  const other=window.prompt('Compare '+pathValue+' with project-relative file:','');
  if(other===null||!String(other).trim())return;
  return openProjectFiles(pathValue,String(other).trim());
}

sideButton.onclick=()=>{viewMode='side';render();};inlineButton.onclick=()=>{viewMode='inline';render();};
editButton.onclick=()=>{try{setEditMode(!editMode);}catch(error){app.showError(error);}};
leftEditorUI.save.onclick=()=>saveCompareSide('left').catch(app.showError);
rightEditorUI.save.onclick=()=>saveCompareSide('right').catch(app.showError);
leftTopSave.onclick=()=>saveCompareSide('left').catch(app.showError);
rightTopSave.onclick=()=>saveCompareSide('right').catch(app.showError);
viewAllButton.onclick=()=>{contentMode='all';render();};
viewDiffButton.onclick=()=>{contentMode='diff';render();};
viewContextButton.onclick=()=>{contentMode='context';render();};
viewUnimportantButton.onclick=()=>{showUnimportant=!showUnimportant;render();};
reloadButton.onclick=()=>reload().catch(app.showError);closeButton.onclick=()=>close();

function compareSourceDescriptor(source){
  if(!source)return null;
  const kind=String(source.kind||'');
  const descriptor={kind,label:String(source.label||'').slice(0,256)};
  if(kind==='project'||kind==='remote'||kind==='git-commit'||kind==='git-head'||kind==='git-index'||kind==='editor'){
    descriptor.path=String(source.path||'').slice(0,4096);
    if(!descriptor.path)return null;
    if(kind==='remote'){descriptor.profileID=String(source.profileID||'');if(!descriptor.profileID)return null;}
    if(kind.startsWith('git-')){descriptor.repoID=String(source.repoID||'');if(!descriptor.repoID)return null;}
    if(kind==='git-commit'){descriptor.ref=String(source.ref||'');if(!descriptor.ref)return null;}
    if(kind==='editor')descriptor.editorID=String(source.editorID||'');
    return descriptor;
  }
  if(kind==='empty')return descriptor;
  // Browser File System Access handles and clipboard data cannot be safely serialized.
  return null;
}
function compareSourceFromDescriptor(spec){
  const path=String(spec?.path||''),label=String(spec?.label||'');
  switch(spec?.kind){
    case 'project':return projectSource(path,{label});
    case 'remote':return remoteSource(spec.profileID,path,{label});
    case 'git-commit':return gitCommitSource(spec.repoID,path,spec.ref,{label,allowMissing:true});
    case 'git-head':return gitStateSource(spec.repoID,path,'head',{label});
    case 'git-index':return gitStateSource(spec.repoID,path,'index',{label});
    case 'editor':{
      const editor=[...(globalThis.TaskMenuEditor?.editors?.values?.()||[])].find(view=>!view.closed&&(view.id===spec.editorID||view.file?.path===path));
      return editor?editorSource(editor,{label:label||'Restored editor'}):projectSource(path,{label});
    }
    case 'empty':return emptyCompareSource(label);
    default:return null;
  }
}
// Session-scoped recovery for unsaved edits. Raw file text is NOT stored in the
// long-lived workspace tab manifest; drafts expire with this browser session.
function compareDraftKey(){
  return 'taskdeck:compare-draft:'+encodeURIComponent(String(app.taskData?.workspace||''))+':'+encodeURIComponent(String(app.currentUser?.user_id||'local'));
}
function compareDraftSignature(left,right){return JSON.stringify([left,right]);}
function storeCompareDraft(left,right){
  try{
    const key=compareDraftKey();
    const draft={version:1,signature:compareDraftSignature(left,right)};
    let hasDirty=false;
    for(const side of ['left','right']){
      const source=current?.[side];
      if(source?.dirty){
        const text=String(source.text??'');
        // Avoid unbounded synchronous storage writes for large remote files.
        if(text.length>512*1024)continue;
        draft[side]={text,sha256:String(source.meta?.sha256||''),editorSnapshot:String(source.meta?.editor_snapshot||'')};
        hasDirty=true;
      }
    }
    if(hasDirty)sessionStorage.setItem(key,JSON.stringify(draft));
    else sessionStorage.removeItem(key);
  }catch(error){console.warn('File Compare draft recovery storage unavailable',error);}
}
function recoverCompareDraft(left,right){
  let saved=null;
  try{saved=JSON.parse(sessionStorage.getItem(compareDraftKey())||'null');}catch{}
  return saved?.version===1&&saved.signature===compareDraftSignature(left,right)?saved:null;
}
function snapshotCompareState(){
  if(!current)return null;
  const left=compareSourceDescriptor(current.left),right=compareSourceDescriptor(current.right);
  if(!left||!right)return null;
  storeCompareDraft(left,right);
  return {version:1,title:String(current.title||'File Compare').slice(0,256),left,right,
    gitContext:current.gitContext||null,
    viewMode,contentMode,showUnimportant,editMode,
    scrollTop:Math.max(0,body.scrollTop),scrollLeft:Math.max(0,horizontalScroll.scrollLeft)};
}
async function restoreCompareState(saved){
  if(!saved||saved.version!==1)return false;
  if(current)return true;
  const left=compareSourceFromDescriptor(saved.left),right=compareSourceFromDescriptor(saved.right);
  if(!left||!right)return false;
  const draft=recoverCompareDraft(saved.left,saved.right);
  const opened=await open({title:String(saved.title||'File Compare'),gitContext:saved.gitContext||null,left,right});
  if(opened){
    for(const side of ['left','right']){
      const recovery=draft?.[side],source=current?.[side];
      if(!recovery||!sourceWritable(source))continue;
      if(recovery.sha256)source.meta.sha256=recovery.sha256;
      if(recovery.editorSnapshot)source.meta.editor_snapshot=recovery.editorSnapshot;
      setSourceBuffer(side,recovery.text,{syncEditor:false});
    }
    viewMode=saved.viewMode==='inline'?'inline':'side';
    contentMode=['all','diff','context'].includes(saved.contentMode)?saved.contentMode:'all';
    showUnimportant=saved.showUnimportant!==false;
    render();
    if(saved.editMode)setEditMode(true);
    requestAnimationFrame(()=>{
      if(!current)return;
      body.scrollTop=Math.max(0,Number(saved.scrollTop)||0);
      horizontalScroll.scrollLeft=Math.max(0,Number(saved.scrollLeft)||0);
      scheduleCompareHorizontalMeasure();
    });
  }
  return Boolean(opened);
}

globalThis.TaskMenuFileCompare={open,close,reload,analyzeTexts:analyzeCompareTexts,decorateGitPreviewRows,renderGitPreviewCode,snapshotState:snapshotCompareState,restoreState:restoreCompareState,buildCompareModel,projectSource,editorSource,savedEditorSource,clipboardSource,remoteSource,browserFileHandleSource,openLeftRemote,gitCommitSource,emptyCompareSource,openGitCommitFileDiff,openGitCommitFileDiffBetween,gitStateSource,workingProjectSource,openProjectFiles,openEditorSaved,openEditorClipboard,openGitCommitAgainstProject,openGitCommits,openGitStatePair,promptProjectCompare,selectForCompare,clearCompareSelection,compareWithSelected,openSources,canCompareWithSelected,sourceIdentity,sourceWritable,get selection(){return selectionSnapshot();},get current(){return current;}};
