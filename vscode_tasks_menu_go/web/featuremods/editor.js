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
.editor-head .editor-format{font:10px ui-monospace,monospace;background:#151b23;color:inherit;border:1px solid #39414d;border-radius:5px;padding:3px 5px;max-width:110px}
.editor-head .editor-format:disabled{opacity:.45}
html[data-taskmenu-theme="light"] .editor-head .editor-format{background:#fff;border-color:#c8ced6}
.editor-head .editor-readonly,.editor-head .editor-warning{font-size:10px;padding:2px 6px;border:1px solid #7d6733;border-radius:10px;color:#ffe29a;background:#493b1d;white-space:nowrap}.editor-head .editor-warning{max-width:260px;overflow:hidden;text-overflow:ellipsis}
.editor-head .editor-large-file{font-size:10px;padding:2px 6px;border:1px solid #5b6f90;border-radius:10px;color:#cfe2ff;background:#24354b;white-space:nowrap}
.editor-body{flex:1;min-height:0;display:flex;overflow:hidden}
.editor-host{flex:1;min-width:0;min-height:0;overflow:hidden}
.editor-minimap{width:88px;min-width:88px;height:100%;border-left:1px solid #30363d;background:#0b0f14;cursor:pointer}
.editor-minimap.hidden{display:none}
html[data-taskmenu-theme="light"] .editor-minimap{background:#f4f6f8;border-color:#d0d7de}
.editor-host .cm-editor{height:100%;font-size:13px}
.editor-host .cm-scroller{overflow:auto;font-family:ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace}
.cm-legacy-keyword{color:#c792ea}.cm-legacy-comment{color:#6a9955;font-style:italic}.cm-legacy-string{color:#ce9178}.cm-legacy-number{color:#b5cea8}.cm-legacy-variable{color:#9cdcfe}.cm-legacy-command{color:#dcdcaa}
.cm-taskdeck-active-line{background:rgba(110,160,220,.075)}
.cm-taskdeck-bracket-match{background:rgba(90,170,255,.20);outline:1px solid rgba(120,190,255,.65);border-radius:2px}
.cm-taskdeck-bracket-mismatch{background:rgba(255,90,105,.18);outline:1px solid rgba(255,105,120,.75);border-radius:2px}
.editor-pane.editor-show-whitespace .cm-taskdeck-space::before{content:'·';position:absolute;color:rgba(155,175,205,.42);pointer-events:none}
.editor-pane.editor-show-whitespace .cm-taskdeck-tab::before{content:'→';position:absolute;color:rgba(155,175,205,.42);pointer-events:none}
.editor-pane.editor-show-whitespace .cm-taskdeck-space,.editor-pane.editor-show-whitespace .cm-taskdeck-tab{position:relative}
.editor-tab .editor-dirty{display:none;margin-left:5px;color:#f2c96d}
.editor-tab.dirty .editor-dirty{display:inline}
.editor-tab .close{margin-left:8px}
.editor-save{background:#203f31;border-color:#3a7058;color:#dcf6e7}
.editor-find-panel{display:flex;align-items:center;gap:6px;padding:5px 9px;border-bottom:1px solid #30363d;background:#11161e;flex-wrap:wrap}
.editor-find-panel.hidden{display:none}
.editor-find-panel input[type="text"]{min-width:160px;max-width:260px;flex:0 1 220px;background:#0d1117;color:inherit;border:1px solid #39414d;border-radius:5px;padding:4px 6px;font:12px ui-monospace,monospace}
.editor-find-panel .editor-find-status{min-width:80px;font-size:10px;opacity:.7}
.editor-find-panel label{display:flex;align-items:center;gap:4px;font-size:10px;white-space:nowrap}
.editor-find-panel button{font-size:11px;padding:3px 7px}
html[data-taskmenu-theme="light"] .editor-find-panel{background:#f5f7fa;border-color:#d0d7de}
html[data-taskmenu-theme="light"] .editor-find-panel input[type="text"]{background:#fff;border-color:#c8ced6}
#panes.editor-split-mode{position:relative;display:block;overflow:hidden}
#panes.editor-split-mode>.editor-pane.editor-split-leaf{position:absolute!important;right:auto!important;bottom:auto!important;display:flex!important;min-width:0;min-height:0;overflow:hidden}
.editor-split-resizer{position:absolute;background:transparent;touch-action:none;z-index:9}
.editor-split-resizer.vertical{cursor:col-resize}.editor-split-resizer.horizontal{cursor:row-resize}
.editor-split-resizer::after{content:"";position:absolute;background:#3c4657;border-radius:2px;opacity:.55}
.editor-split-resizer.vertical::after{left:2px;top:0;bottom:0;width:2px}.editor-split-resizer.horizontal::after{top:2px;left:0;right:0;height:2px}
.editor-split-resizer.dragging::after,.editor-split-resizer:hover::after{opacity:1}
.editor-tab.editor-split-peer{box-shadow:inset 0 -2px 0 rgba(120,170,255,.35)}
.editor-split-action{font-size:11px;padding:3px 7px}
.editor-dirty-backdrop{position:fixed;inset:0;z-index:7000;background:rgba(0,0,0,.55);display:flex;align-items:center;justify-content:center;padding:18px}
.editor-dirty-dialog{width:min(430px,94vw);background:#171a20;border:1px solid #3b414d;border-radius:10px;box-shadow:0 18px 48px rgba(0,0,0,.5);padding:16px}
.editor-dirty-dialog h3{margin:0 0 8px;font-size:15px}.editor-dirty-dialog p{margin:0 0 14px;font-size:12px;opacity:.75;word-break:break-word}
.editor-dirty-actions{display:flex;justify-content:flex-end;gap:8px}.editor-dirty-actions .discard{margin-right:auto;background:#4a252a;border-color:#7a4048;color:#ffe2e4}
.editor-conflict-dialog{width:min(560px,96vw)}
.editor-conflict-actions{display:flex;justify-content:flex-end;gap:8px;flex-wrap:wrap}.editor-conflict-actions .compare{margin-right:auto}
.editor-compare-dialog{width:min(1100px,96vw);max-height:90vh;display:flex;flex-direction:column}
.editor-history-dialog{width:min(760px,96vw);max-height:88vh;display:flex;flex-direction:column}
.editor-outline-dialog{width:min(720px,96vw);max-height:88vh;display:flex;flex-direction:column}
.editor-outline-filter{width:100%;box-sizing:border-box;background:#0d1117;color:inherit;border:1px solid #39414d;border-radius:6px;padding:6px 8px;margin:7px 0}
.editor-outline-list{display:flex;flex-direction:column;gap:3px;overflow:auto;max-height:62vh}
.editor-outline-row{display:grid;grid-template-columns:72px minmax(0,1fr) 64px;gap:8px;align-items:center;text-align:left;padding:6px 8px;border:0;border-radius:5px;background:transparent}
.editor-outline-row:hover,.editor-outline-row:focus{background:#222a35}
.editor-outline-kind{font-size:9px;text-transform:uppercase;letter-spacing:.04em;opacity:.58}
.editor-outline-name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:12px ui-monospace,monospace}
.editor-outline-line{text-align:right;font:10px ui-monospace,monospace;opacity:.55}
.editor-breadcrumb{min-width:0;max-width:min(38vw,520px);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:10px;opacity:.68}
html[data-taskmenu-theme="light"] .editor-outline-filter{background:#fff;border-color:#c8ced6}
html[data-taskmenu-theme="light"] .editor-outline-row:hover,html[data-taskmenu-theme="light"] .editor-outline-row:focus{background:#e8edf3}
.editor-history-list{display:flex;flex-direction:column;gap:6px;overflow:auto;min-height:80px;max-height:60vh;margin:8px 0 14px}
.editor-history-row{display:grid;grid-template-columns:minmax(0,1fr) auto auto;gap:7px;align-items:center;padding:7px 8px;border:1px solid #343a45;border-radius:6px;background:#10151c}
.editor-history-main{min-width:0}.editor-history-time{font-size:11px;font-weight:600}.editor-history-meta{font:10px ui-monospace,monospace;opacity:.65;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
html[data-taskmenu-theme="light"] .editor-history-row{background:#f8f9fb;border-color:#c8ced6}
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
const editorSplitSupported=app.layoutProfile!=='mobile';
let editorSplitRoots=[];
let renderedEditorSplitRoot=null;
let editorSplitDragging=null;
const editorSplitResizers=new Map();
const editorSplitRects=new Map();
const closedEditorPaths=[];
const maxClosedEditorPaths=30;
const editorAutoSavePreferenceKey='vscode-tasks-menu:editor-auto-save';
let editorAutoSaveEnabled=localStorage.getItem(editorAutoSavePreferenceKey)==='1';
const editorAutoSaveDelayMS=1000;
const editorMinimapPreferenceKey='vscode-tasks-menu:editor-minimap';
let editorMinimapEnabled=localStorage.getItem(editorMinimapPreferenceKey)==='1';

function editorSplitLeaf(id){return {type:'leaf',id};}
function editorSplitNode(first,second,orientation,ratio=.5){return {type:'split',first,second,orientation:orientation==='horizontal'?'horizontal':'vertical',ratio};}
function editorIsSplit(node){return Boolean(node&&node.type==='split'&&node.first&&node.second);}
function editorClampRatio(value){value=Number(value);return Number.isFinite(value)?Math.max(.15,Math.min(.85,value)):.5;}
function editorSplitLeafIDs(node,out=[]){
  if(!node)return out;
  if(node.type==='leaf'){out.push(node.id);return out;}
  editorSplitLeafIDs(node.first,out);editorSplitLeafIDs(node.second,out);return out;
}
function editorSplitContains(node,id){return editorSplitLeafIDs(node,[]).includes(id);}
function editorFindSplitRoot(id){return editorSplitRoots.find(root=>editorSplitContains(root,id))||null;}
function editorFindSplitLeaf(id,node=null){
  const roots=node?[node]:editorSplitRoots;
  for(const root of roots){
    if(root?.type==='leaf'&&root.id===id)return root;
    if(editorIsSplit(root)){
      const found=editorFindSplitLeaf(id,root.first)||editorFindSplitLeaf(id,root.second);
      if(found)return found;
    }
  }
  return null;
}
function editorFindSplitParent(node,id,parent=null){
  if(!node)return null;
  if(node.type==='leaf')return node.id===id?parent:null;
  return editorFindSplitParent(node.first,id,node)||editorFindSplitParent(node.second,id,node);
}
function editorSplitParentFor(id){
  const root=editorFindSplitRoot(id);return root?editorFindSplitParent(root,id,null):null;
}
function remapEditorSplitID(node,oldID,newID){
  if(!node)return;
  if(node.type==='leaf'){
    if(node.id===oldID)node.id=newID;
    return;
  }
  remapEditorSplitID(node.first,oldID,newID);
  remapEditorSplitID(node.second,oldID,newID);
}
function editorReplaceSplitNode(target,replacement){
  const inside=node=>{
    if(!editorIsSplit(node))return false;
    if(node.first===target){node.first=replacement;return true;}
    if(node.second===target){node.second=replacement;return true;}
    return inside(node.first)||inside(node.second);
  };
  for(let i=0;i<editorSplitRoots.length;i++){
    if(editorSplitRoots[i]===target){if(replacement)editorSplitRoots[i]=replacement;else editorSplitRoots.splice(i,1);return true;}
    if(inside(editorSplitRoots[i]))return true;
  }
  return false;
}
function pruneEditorSplitRoots(){
  const prune=node=>{
    if(!node)return null;
    if(node.type==='leaf')return editors.has(node.id)?node:null;
    const first=prune(node.first),second=prune(node.second);
    if(!first)return second;if(!second)return first;
    node.first=first;node.second=second;return node;
  };
  editorSplitRoots=editorSplitRoots.map(prune).filter(editorIsSplit);
}
function detachEditorFromSplit(id){
  const root=editorFindSplitRoot(id);if(!root)return null;
  const target=editorFindSplitLeaf(id,root);if(!target)return null;
  const parent=editorFindSplitParent(root,id,null);
  if(!parent){editorSplitRoots=editorSplitRoots.filter(item=>item!==root);return target;}
  const sibling=parent.first===target?parent.second:parent.first;
  editorReplaceSplitNode(parent,sibling);
  pruneEditorSplitRoots();
  return target;
}
function splitEditorLeaf(firstID,secondID,orientation,ratio=.5){
  if(!firstID||!secondID||firstID===secondID)return null;
  detachEditorFromSplit(secondID);
  let first=editorFindSplitLeaf(firstID);
  if(first){
    const node=editorSplitNode(first,editorSplitLeaf(secondID),orientation,ratio);
    editorReplaceSplitNode(first,node);return node;
  }
  const node=editorSplitNode(editorSplitLeaf(firstID),editorSplitLeaf(secondID),orientation,ratio);
  editorSplitRoots.push(node);return node;
}
function cleanupEditorSplitPresentation(){
  renderedEditorSplitRoot=null;editorSplitDragging=null;
  panesHost.classList.remove('editor-split-mode');
  for(const view of editors.values()){
    view.pane.classList.remove('editor-split-leaf');
    view.tab.classList.remove('editor-split-peer');
    for(const prop of ['left','top','width','height'])view.pane.style.removeProperty(prop);
  }
  for(const bar of editorSplitResizers.values())bar.remove();
  editorSplitResizers.clear();editorSplitRects.clear();
}
function editorSplitResizer(node){
  let bar=editorSplitResizers.get(node);
  if(bar?.isConnected)return bar;
  bar=document.createElement('div');bar.className='editor-split-resizer '+node.orientation;
  bar.title='Drag to resize this editor split · double-click to reset';
  panesHost.append(bar);editorSplitResizers.set(node,bar);
  bar.onpointerdown=event=>{
    if(event.button!==0||!renderedEditorSplitRoot)return;
    editorSplitDragging={node,pointerId:event.pointerId};bar.classList.add('dragging');bar.setPointerCapture(event.pointerId);event.preventDefault();
  };
  bar.onpointermove=event=>{
    if(!editorSplitDragging||editorSplitDragging.node!==node)return;
    const rect=editorSplitRects.get(node),host=panesHost.getBoundingClientRect();if(!rect)return;
    const gap=6;
    const raw=node.orientation==='vertical'
      ?(event.clientX-host.left-rect.x)/Math.max(1,rect.w-gap)
      :(event.clientY-host.top-rect.y)/Math.max(1,rect.h-gap);
    if(!Number.isFinite(raw))return;
    node.ratio=editorClampRatio(raw);layoutEditorSplit();event.preventDefault();
  };
  const finish=event=>{
    if(!editorSplitDragging||editorSplitDragging.node!==node)return;
    editorSplitDragging=null;bar.classList.remove('dragging');
    try{if(bar.hasPointerCapture(event.pointerId))bar.releasePointerCapture(event.pointerId);}catch{}
  };
  bar.onpointerup=finish;bar.onpointercancel=finish;
  bar.ondblclick=()=>{node.ratio=.5;layoutEditorSplit();};
  return bar;
}
function placeEditorSplitPane(id,rect){
  const view=editors.get(id);if(!view)return;
  view.pane.classList.remove('hidden');view.pane.classList.add('editor-split-leaf');view.tab.classList.add('editor-split-peer');
  view.pane.style.left=Math.round(rect.x)+'px';view.pane.style.top=Math.round(rect.y)+'px';
  view.pane.style.width=Math.max(0,Math.round(rect.w))+'px';view.pane.style.height=Math.max(0,Math.round(rect.h))+'px';
  setTimeout(()=>{try{view.cm.requestMeasure?.();}catch{}},0);
}
function layoutEditorSplitNode(node,rect,used){
  if(!node)return;
  if(node.type==='leaf'){placeEditorSplitPane(node.id,rect);return;}
  editorSplitRects.set(node,rect);
  const gap=6,bar=editorSplitResizer(node);used.add(node);
  bar.className='editor-split-resizer '+node.orientation;
  if(node.orientation==='vertical'){
    const available=Math.max(0,rect.w-gap),firstW=Math.round(available*editorClampRatio(node.ratio)),secondW=Math.max(0,available-firstW);
    bar.style.left=Math.round(rect.x+firstW)+'px';bar.style.top=Math.round(rect.y)+'px';bar.style.width=gap+'px';bar.style.height=Math.round(rect.h)+'px';
    layoutEditorSplitNode(node.first,{x:rect.x,y:rect.y,w:firstW,h:rect.h},used);
    layoutEditorSplitNode(node.second,{x:rect.x+firstW+gap,y:rect.y,w:secondW,h:rect.h},used);
  }else{
    const available=Math.max(0,rect.h-gap),firstH=Math.round(available*editorClampRatio(node.ratio)),secondH=Math.max(0,available-firstH);
    bar.style.left=Math.round(rect.x)+'px';bar.style.top=Math.round(rect.y+firstH)+'px';bar.style.width=Math.round(rect.w)+'px';bar.style.height=gap+'px';
    layoutEditorSplitNode(node.first,{x:rect.x,y:rect.y,w:rect.w,h:firstH},used);
    layoutEditorSplitNode(node.second,{x:rect.x,y:rect.y+firstH+gap,w:rect.w,h:secondH},used);
  }
}
function layoutEditorSplit(){
  if(!renderedEditorSplitRoot||!editorIsSplit(renderedEditorSplitRoot))return;
  panesHost.classList.add('editor-split-mode');
  for(const view of editors.values()){
    view.pane.classList.add('hidden');view.pane.classList.remove('editor-split-leaf');view.tab.classList.remove('editor-split-peer');
    for(const prop of ['left','top','width','height'])view.pane.style.removeProperty(prop);
  }
  const host=panesHost.getBoundingClientRect(),used=new Set();
  layoutEditorSplitNode(renderedEditorSplitRoot,{x:0,y:0,w:host.width,h:host.height},used);
  for(const [node,bar] of [...editorSplitResizers])if(!used.has(node)){bar.remove();editorSplitResizers.delete(node);}
}
function renderEditorSplit(root){
  if(!root||!editorIsSplit(root)){cleanupEditorSplitPresentation();return false;}
  if(renderedEditorSplitRoot!==root){cleanupEditorSplitPresentation();renderedEditorSplitRoot=root;}
  layoutEditorSplit();return true;
}
function syncEditorSplitForActive(){
  if(!editorSplitSupported){cleanupEditorSplitPresentation();return;}
  const root=editorFindSplitRoot(activeEditorID);
  if(root&&editorIsSplit(root))renderEditorSplit(root);
  else if(renderedEditorSplitRoot||panesHost.classList.contains('editor-split-mode'))cleanupEditorSplitPresentation();
}
function editorSplitPeerCandidates(view){
  return [...editors.values()].filter(candidate=>candidate.id!==view.id&&!candidate.closed);
}
async function chooseEditorSplitTarget(view){
  const candidates=editorSplitPeerCandidates(view);
  if(candidates.length){
    const lines=candidates.map((candidate,index)=>`${index+1}. ${candidate.file.path}`).join('\n');
    const answer=window.prompt(`Split “${view.file.path}” with which editor?\n\n${lines}\n\nEnter a number, or 0 to open another file:`,'1');
    if(answer===null)return null;
    const value=Number.parseInt(String(answer).trim(),10);
    if(Number.isInteger(value)&&value>=1&&value<=candidates.length)return candidates[value-1];
    if(value!==0)throw new Error('Invalid editor split target');
  }
  const pathValue=window.prompt('Project-relative file path to open in the split:','');
  if(pathValue===null)return null;
  const clean=String(pathValue).trim();if(!clean)return null;
  const target=await openFile(clean);
  if(!target||target.id===view.id)throw new Error('Choose a different file for the split');
  return target;
}
async function splitEditor(view,orientation){
  if(!editorSplitSupported||!view||view.closed)return false;
  const target=await chooseEditorSplitTarget(view);if(!target)return false;
  splitEditorLeaf(view.id,target.id,orientation,.5);
  activateEditor(view.id,{force:true});
  setTimeout(()=>{syncEditorSplitForActive();try{view.cm.focus();}catch{}},0);
  return true;
}
function swapEditorSplit(view){
  const parent=editorSplitParentFor(view.id);if(!parent)return false;
  const first=parent.first;parent.first=parent.second;parent.second=first;
  syncEditorSplitForActive();return true;
}
function unsplitEditor(view){
  const root=editorFindSplitRoot(view.id),target=editorFindSplitLeaf(view.id,root);if(!root||!target)return false;
  const parent=editorFindSplitParent(root,view.id,null);if(!parent)return false;
  const sibling=parent.first===target?parent.second:parent.first;
  if(parent===root){
    editorSplitRoots=editorSplitRoots.filter(item=>item!==root);
    if(editorIsSplit(sibling))editorSplitRoots.push(sibling);
  }else{
    editorReplaceSplitNode(parent,sibling);
  }
  pruneEditorSplitRoots();activateEditor(view.id,{force:true});return true;
}
function updateEditorSplitButtons(view){
  const parent=editorSplitParentFor(view.id);
  if(view.splitSwap)view.splitSwap.hidden=!parent;
  if(view.splitUnsplit)view.splitUnsplit.hidden=!parent;
  if(view.splitVertical)view.splitVertical.disabled=!editorSplitSupported;
  if(view.splitHorizontal)view.splitHorizontal.disabled=!editorSplitSupported;
}

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
  lua:new Set('and break do else elseif end false for function goto if in local nil not or repeat return then true until while'.split(' ')),
  nim:new Set('addr and as asm bind block break case cast concept const continue converter defer discard distinct div do elif else end enum except export finally for from func if import in include interface is isnot iterator let macro method mixin mod nil not notin object of or out proc ptr raise ref return shl shr static template try tuple type using var when while xor yield'.split(' '))
};
const legacyMarks=new Map();
function legacyHighlightKind(pathValue){
  const lower=String(pathValue||'').toLowerCase();
  const name=basename(lower);
  const ext=(name.includes('.')?name.slice(name.lastIndexOf('.')+1):'');
  if(name==='cmakelists.txt'||ext==='cmake')return 'cmake';
  if(['sh','bash','zsh','fish','ksh'].includes(ext)||name==='.bashrc'||name==='.zshrc')return 'shell';
  if(['nim','nims','nimble'].includes(ext))return 'nim';
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
    if(kind==='nim'&&ch.charCodeAt(0)===96){
      let j=i+1;while(j<text.length&&text.charCodeAt(j)!==96)j++;if(j<text.length)j++;
      i=j;firstWord=false;continue;
    }
    if(kind==='nim'&&text.startsWith('\"\"\"',i)){
      const end=text.indexOf('\"\"\"',i+3),j=end<0?text.length:end+3;
      tokens.push({from:i,to:j,type:'string'});i=j;firstWord=false;continue;
    }
    if(ch==='"'||ch==="'"){
      const quote=ch;let j=i+1;
      while(j<text.length){if(text[j]==='\\'){j+=2;continue;}if(text[j]===quote){j++;break;}j++;}
      tokens.push({from:i,to:j,type:'string'});i=j;firstWord=false;continue;
    }
    if(kind==='shell'&&ch.charCodeAt(0)===36){
      let j=i+1;
      if(text[j]==='{'){j++;while(j<text.length&&text[j]!=='}')j++;if(j<text.length)j++;}
      else while(j<text.length&&/[A-Za-z0-9_@*#?$!\-]/.test(text[j]))j++;
      if(j>i+1){tokens.push({from:i,to:j,type:'variable'});i=j;continue;}
    }
    if(kind==='cmake'&&ch.charCodeAt(0)===36&&next==='{'){
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
      const keywordWord=kind==='nim'?lower.replaceAll('_',''):lower;
      let type='';
      if(keywords.has(keywordWord))type='keyword';
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
      for(const token of legacyLineTokens(kind,line.text)){
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
const taskDeckWhitespaceMark=globalThis.cm6.Decoration.mark({class:'cm-taskdeck-space'});
const taskDeckTabMark=globalThis.cm6.Decoration.mark({class:'cm-taskdeck-tab'});
const taskDeckActiveLineMark=globalThis.cm6.Decoration.line({class:'cm-taskdeck-active-line'});
function buildWhitespaceDecorations(view){
  const builder=new globalThis.cm6.RangeSetBuilder();
  for(const range of view.visibleRanges){
    const text=view.state.doc.sliceString(range.from,range.to);
    for(let index=0;index<text.length;index++){
      const code=text.charCodeAt(index);
      if(code===32)builder.add(range.from+index,range.from+index+1,taskDeckWhitespaceMark);
      else if(code===9)builder.add(range.from+index,range.from+index+1,taskDeckTabMark);
    }
  }
  return builder.finish();
}
function whitespaceDecorationExtension(){
  return globalThis.cm6.ViewPlugin.fromClass(class{
    constructor(view){this.decorations=buildWhitespaceDecorations(view);}
    update(update){if(update.docChanged||update.viewportChanged)this.decorations=buildWhitespaceDecorations(update.view);}
  },{decorations:value=>value.decorations});
}
function activeLineDecorationExtension(){
  return globalThis.cm6.ViewPlugin.fromClass(class{
    constructor(view){this.decorations=this.build(view);}
    build(view){
      const builder=new globalThis.cm6.RangeSetBuilder();
      const line=view.state.doc.lineAt(view.state.selection.main.head);
      builder.add(line.from,line.from,taskDeckActiveLineMark);
      return builder.finish();
    }
    update(update){if(update.docChanged||update.selectionSet||update.viewportChanged)this.decorations=this.build(update.view);}
  },{decorations:value=>value.decorations});
}

const taskDeckBracketMatchMark=globalThis.cm6.Decoration.mark({class:'cm-taskdeck-bracket-match'});
const taskDeckBracketMismatchMark=globalThis.cm6.Decoration.mark({class:'cm-taskdeck-bracket-mismatch'});
const taskDeckBracketPairs={40:41,91:93,123:125};
const taskDeckBracketReverse={41:40,93:91,125:123};
const taskDeckBracketScanLimit=200000;
function bracketCandidateAtCursor(state){
  const head=state.selection.main.head,doc=state.doc;
  for(const pos of [head,head-1]){
    if(pos<0||pos>=doc.length)continue;
    const code=doc.sliceString(pos,pos+1).charCodeAt(0);
    if(taskDeckBracketPairs[code]||taskDeckBracketReverse[code])return {pos,code};
  }
  return null;
}
function findMatchingBracket(state,candidate){
  const doc=state.doc,{pos,code}=candidate;
  const forward=Boolean(taskDeckBracketPairs[code]);
  const open=forward?code:taskDeckBracketReverse[code];
  const close=forward?taskDeckBracketPairs[code]:code;
  let depth=0,steps=0;
  if(forward){
    for(let at=pos;at<doc.length&&steps<taskDeckBracketScanLimit;at++,steps++){
      const current=doc.sliceString(at,at+1).charCodeAt(0);
      if(current===open)depth++;
      else if(current===close&&--depth===0)return at;
    }
  }else{
    for(let at=pos;at>=0&&steps<taskDeckBracketScanLimit;at--,steps++){
      const current=doc.sliceString(at,at+1).charCodeAt(0);
      if(current===close)depth++;
      else if(current===open&&--depth===0)return at;
    }
  }
  return -1;
}
function buildBracketDecorations(view){
  const builder=new globalThis.cm6.RangeSetBuilder();
  const candidate=bracketCandidateAtCursor(view.state);
  if(!candidate)return builder.finish();
  const match=findMatchingBracket(view.state,candidate);
  if(match<0){
    builder.add(candidate.pos,candidate.pos+1,taskDeckBracketMismatchMark);
    return builder.finish();
  }
  const first=Math.min(candidate.pos,match),second=Math.max(candidate.pos,match);
  builder.add(first,first+1,taskDeckBracketMatchMark);
  if(second!==first)builder.add(second,second+1,taskDeckBracketMatchMark);
  return builder.finish();
}
function bracketMatchingExtension(){
  return globalThis.cm6.ViewPlugin.fromClass(class{
    constructor(view){this.decorations=buildBracketDecorations(view);}
    update(update){if(update.docChanged||update.selectionSet)this.decorations=buildBracketDecorations(update.view);}
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
  options.extraExtensions=[activeLineDecorationExtension(),whitespaceDecorationExtension(),bracketMatchingExtension()];
  if(legacy)options.extraExtensions.push(legacySyntaxExtension(legacy));
  return options;
}
function languageLabel(pathValue){
  const options=languageOptions(pathValue);
  if(options.tsx)return 'TSX';
  if(options.typescript)return 'TypeScript';
  if(options.jsx)return 'JSX';
  for(const key of ['cpp','go','python','javascript','json','yaml','markdown','html','css','xml','java','php','sql','rust','vue']){
    if(options[key])return key==='cpp'?'C/C++':key==='javascript'?'JavaScript':key.toUpperCase();
  }
  const legacy=legacyHighlightKind(pathValue);
  if(legacy==='cmake')return 'CMake';
  if(legacy==='shell')return 'Shell';
  if(legacy==='nim')return 'Nim';
  if(legacy==='lua')return 'Lua';
  return 'Plain text';
}
function editorMetaText(file){
  const ending=(file.line_ending||'lf').toUpperCase();
  const bom=file.bom?' + BOM':'';
  return [languageLabel(file.path),'UTF-8'+bom,ending,formatBytes(file.size)].join(' • ');
}
function editorOptionsForFile(file){
  return file?.large_file?{}:languageOptions(file.path);
}
function editorEncodingChoice(file){
  return file?.bom?'utf-8-bom':'utf-8';
}
function syncEditorFormatControls(view){
  if(!view)return;
  if(view.lineEndingSelect)view.lineEndingSelect.value=view.desiredLineEnding||view.file?.line_ending||'lf';
  if(view.encodingSelect)view.encodingSelect.value=view.desiredEncoding||editorEncodingChoice(view.file);
  const readonly=editorReadOnly(view);
  if(view.lineEndingSelect)view.lineEndingSelect.disabled=readonly;
  if(view.encodingSelect)view.encodingSelect.disabled=readonly;
}
function updateAutoSaveButton(view){
  if(!view?.autoSave)return;
  view.autoSave.textContent=editorAutoSaveEnabled?'Auto ✓':'Auto';
  view.autoSave.classList.toggle('active',editorAutoSaveEnabled);
  view.autoSave.setAttribute('aria-pressed',editorAutoSaveEnabled?'true':'false');
}
function scheduleEditorAutoSave(view){
  if(!view||view.closed)return;
  clearTimeout(view.autoSaveTimer);view.autoSaveTimer=null;
  if(view.minimapRAF){cancelAnimationFrame(view.minimapRAF);view.minimapRAF=0;}
  if(!editorAutoSaveEnabled||!view.dirty||editorReadOnly(view)||view.saving)return;
  view.autoSaveTimer=setTimeout(()=>{
    view.autoSaveTimer=null;
    if(!editorAutoSaveEnabled||view.closed||!view.dirty||editorReadOnly(view)||view.saving)return;
    saveEditor(view).catch(app.showError);
  },editorAutoSaveDelayMS);
}
function setEditorAutoSave(enabled){
  editorAutoSaveEnabled=Boolean(enabled);
  try{localStorage[editorAutoSavePreferenceKey]=editorAutoSaveEnabled?'1':'0';}catch{}
  for(const view of editors.values()){
    updateAutoSaveButton(view);
    if(editorAutoSaveEnabled)scheduleEditorAutoSave(view);
    else{clearTimeout(view.autoSaveTimer);view.autoSaveTimer=null;}
  }
  return editorAutoSaveEnabled;
}
function updateMinimapButton(view){
  if(!view?.minimapToggle)return;
  view.minimapToggle.textContent=editorMinimapEnabled?'Map ✓':'Map';
  view.minimapToggle.classList.toggle('active',editorMinimapEnabled);
  view.minimapToggle.setAttribute('aria-pressed',editorMinimapEnabled?'true':'false');
  if(view.minimap)view.minimap.classList.toggle('hidden',!editorMinimapEnabled);
}
function renderEditorMinimap(view){
  if(!view||view.closed||!view.minimap||!editorMinimapEnabled)return;
  const canvas=view.minimap,rect=canvas.getBoundingClientRect();
  const cssWidth=Math.max(1,Math.floor(rect.width||88)),cssHeight=Math.max(1,Math.floor(rect.height));
  if(cssHeight<8)return;
  const ratio=Math.min(2,window.devicePixelRatio||1);
  const width=Math.max(1,Math.floor(cssWidth*ratio)),height=Math.max(1,Math.floor(cssHeight*ratio));
  if(canvas.width!==width)canvas.width=width;
  if(canvas.height!==height)canvas.height=height;
  const ctx=canvas.getContext('2d');if(!ctx)return;
  ctx.setTransform(ratio,0,0,ratio,0,0);ctx.clearRect(0,0,cssWidth,cssHeight);
  const doc=view.cm.state.doc,total=Math.max(1,doc.lines);
  const rowHeight=Math.max(.6,cssHeight/Math.min(total,cssHeight));
  const step=Math.max(1,Math.ceil(total/cssHeight));
  const computed=getComputedStyle(view.pane);
  ctx.fillStyle=computed.color||'#c9d1d9';ctx.globalAlpha=.24;
  let row=0;
  for(let lineNo=1;lineNo<=total;lineNo+=step){
    const line=doc.line(lineNo);
    const visual=Math.min(cssWidth-8,Math.max(1,line.text.replace(/\t/g,'    ').length*.72));
    const y=Math.min(cssHeight-1,row*rowHeight);
    ctx.fillRect(4,y,visual,Math.max(.55,rowHeight*.55));
    row++;
    if(y>=cssHeight)break;
  }
  const scroller=view.scroller;
  if(scroller&&scroller.scrollHeight>0){
    const top=Math.max(0,Math.min(1,scroller.scrollTop/scroller.scrollHeight));
    const span=Math.max(.025,Math.min(1,scroller.clientHeight/scroller.scrollHeight));
    ctx.globalAlpha=.22;ctx.fillStyle='#6ea8ff';
    ctx.fillRect(0,top*cssHeight,cssWidth,Math.max(3,span*cssHeight));
  }
  ctx.globalAlpha=1;
}
function scheduleEditorMinimap(view){
  if(!view||view.closed||!editorMinimapEnabled)return;
  if(view.minimapRAF)return;
  view.minimapRAF=requestAnimationFrame(()=>{
    view.minimapRAF=0;
    renderEditorMinimap(view);
  });
}
function setEditorMinimap(enabled){
  editorMinimapEnabled=Boolean(enabled);
  try{localStorage[editorMinimapPreferenceKey]=editorMinimapEnabled?'1':'0';}catch{}
  for(const view of editors.values()){
    updateMinimapButton(view);
    if(editorMinimapEnabled)scheduleEditorMinimap(view);
    else if(view.minimapRAF){cancelAnimationFrame(view.minimapRAF);view.minimapRAF=0;}
  }
  return editorMinimapEnabled;
}
function jumpFromEditorMinimap(view,event){
  if(!view||view.closed||!editorMinimapEnabled)return;
  const rect=view.minimap.getBoundingClientRect();if(rect.height<=0)return;
  const ratio=Math.max(0,Math.min(1,(event.clientY-rect.top)/rect.height));
  const lineNo=Math.max(1,Math.min(view.cm.state.doc.lines,Math.round(ratio*(view.cm.state.doc.lines-1))+1));
  const line=view.cm.state.doc.line(lineNo);
  view.cm.dispatch({selection:{anchor:line.from},scrollIntoView:true});
  view.cm.focus();
}

function setDirty(view,dirty){
  if(!view||view.closed)return;
  view.dirty=Boolean(dirty);
  view.tab.classList.toggle('dirty',view.dirty);
  view.tab.title=(view.dirty?'● ':'')+view.file.path;
  view.save.disabled=Boolean(view.file.read_only)||!view.dirty||view.saving;
  if(view.dirty)scheduleEditorAutoSave(view);
  else{clearTimeout(view.autoSaveTimer);view.autoSaveTimer=null;}
}
function editorReadOnly(view){
  return Boolean(view?.file?.read_only||view?.tabReadOnly);
}
function applyReadOnly(view){
  const readonly=editorReadOnly(view);
  view.readonlyBadge.hidden=!readonly;
  view.readonlyBadge.textContent=view.file.read_only?'READ-ONLY':'TAB READ-ONLY';
  view.cm.contentDOM.setAttribute('contenteditable',readonly?'false':'true');
  view.cm.contentDOM.setAttribute('aria-readonly',readonly?'true':'false');
  view.save.disabled=readonly||!view.dirty||view.saving;
  if(view.lineEndingSelect)view.lineEndingSelect.disabled=readonly;
  if(view.encodingSelect)view.encodingSelect.disabled=readonly;
  if(view.whitespace)view.whitespace.disabled=Boolean(view.file?.large_file);
  syncEditorFindReadOnly(view);
  view.tab.classList.toggle('taskdeck-tab-readonly',Boolean(view.tabReadOnly));
  if(view.tabReadOnly)view.pane.dataset.taskdeckReadonly='1';else delete view.pane.dataset.taskdeckReadonly;
  view.pane.setAttribute('aria-readonly',readonly?'true':'false');
}
function setTabReadOnly(viewOrID,enabled){
  const view=typeof viewOrID==='string'?editors.get(viewOrID):viewOrID;
  if(!view||view.closed)return false;
  view.tabReadOnly=Boolean(enabled);
  applyReadOnly(view);
  return view.tabReadOnly;
}
function installEditorDispatchGuard(view){
  const originalDispatch=view.cm.dispatch.bind(view.cm);
  view.dispatchRaw=originalDispatch;
  view.cm.dispatch=(...input)=>{
    const transaction=input.length===1&&input[0]?.startState
      ? input[0]
      : view.cm.state.update(...input);
    if(transaction.docChanged&&editorReadOnly(view)&&!view.internalUpdate)return;
    originalDispatch(transaction);
    if(transaction.docChanged){
      if(!view.internalUpdate)setDirty(view,true);
      scheduleEditorMinimap(view);
    }
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
  view.desiredLineEnding=file.line_ending||'lf';
  view.desiredEncoding=editorEncodingChoice(file);
  syncEditorFormatControls(view);
  view.path.textContent=file.path;
  view.path.title=file.path;
  updateEditorBreadcrumb(view);
  view.meta.textContent=editorMetaText(file);
  view.warningBadge.hidden=!file.warning;
  view.warningBadge.textContent=file.warning?'WARNING':'';
  view.warningBadge.title=file.warning||'';
  if(view.largeFileBadge)view.largeFileBadge.hidden=!file.large_file;
  view.pane.classList.toggle('editor-large-file-mode',Boolean(file.large_file));
  applyReadOnly(view);
  updateAutoSaveButton(view);
  setDirty(view,false);
  scheduleEditorMinimap(view);
}
function applySavedEditorFile(view,file){
  if(!view||view.closed||!file)return;
  const editorContent=view.cm.state.doc.toString();
  view.file={...file,content:editorContent};
  view.desiredLineEnding=file.line_ending||view.desiredLineEnding||'lf';
  view.desiredEncoding=editorEncodingChoice(file);
  syncEditorFormatControls(view);
  view.path.textContent=file.path;
  view.path.title=file.path;
  updateEditorBreadcrumb(view);
  view.meta.textContent=editorMetaText(file);
  view.warningBadge.hidden=!file.warning;
  view.warningBadge.textContent=file.warning?'WARNING':'';
  view.warningBadge.title=file.warning||'';
  if(view.largeFileBadge)view.largeFileBadge.hidden=!file.large_file;
  view.pane.classList.toggle('editor-large-file-mode',Boolean(file.large_file));
  applyReadOnly(view);
  updateAutoSaveButton(view);
  setDirty(view,false);
  scheduleEditorMinimap(view);
}
function activateEditorDOM(id){
  activeEditorID=id;
  const root=editorFindSplitRoot(id);
  for(const [editorIDValue,view] of editors){
    const yes=editorIDValue===id;
    const peer=Boolean(root&&editorSplitContains(root,editorIDValue));
    view.tab.classList.toggle('active',yes);
    view.pane.classList.toggle('hidden',root? !peer:!yes);
    updateEditorSplitButtons(view);
    if(yes)setTimeout(()=>{try{view.cm.focus();scheduleEditorMinimap(view);}catch{}},0);
    else if(peer)setTimeout(()=>scheduleEditorMinimap(view),0);
  }
  if(root)renderEditorSplit(root);else syncEditorSplitForActive();
}
function activateEditor(id,{force=false}={}){
  if(!editors.has(id))return false;
  return app.activateExternalView(id,{force});
}
function deactivateEditors(){
  activeEditorID='';
  cleanupEditorSplitPresentation();
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
function remoteEditorFileFromResponse(base,data){
  const content=String(data?.content??'');
  const bom=content.charCodeAt(0)===0xfeff;
  const normalized=bom?content.slice(1):content;
  const lineEnding=/\r\n/.test(normalized)?'crlf':'lf';
  const size=new TextEncoder().encode(content).length;
  return {...base,content:normalized,sha256:String(data?.sha256||''),size,line_ending:lineEnding,bom,encoding:'utf-8',read_only:false,large_file:false};
}
function remoteEditorSerializedText(view){
  let text=view.cm.state.doc.toString();
  text=(view.desiredLineEnding||view.file.line_ending)==='crlf'?text.replace(/\r?\n/g,'\r\n'):text.replace(/\r\n/g,'\n');
  if((view.desiredEncoding||editorEncodingChoice(view.file))==='utf-8-bom')text='\ufeff'+text.replace(/^\ufeff/,'');
  else text=text.replace(/^\ufeff/,'');
  return text;
}
async function putEditorFile(view,expectedSHA256){
  if(view.file?.remote_workspace_id){
    const response=await app.fetchWithLease('/api/remote-workspace-files',{
      method:'POST',
      cache:'no-store',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({
        workspace_id:view.file.remote_workspace_id,
        operation:'write',
        path:view.file.remote_path,
        content:remoteEditorSerializedText(view),
        expected_sha256:expectedSHA256
      })
    });
    if(response.status===409)return {conflict:true};
    if(!response.ok)throw new Error((await response.text())||response.statusText);
    return {file:remoteEditorFileFromResponse(view.file,await response.json())};
  }
  const response=await app.fetchWithLease('/api/project/file',{
    method:'PUT',
    cache:'no-store',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({
      path:view.file.path,
      content:view.cm.state.doc.toString(),
      expected_sha256:expectedSHA256,
      line_ending:view.desiredLineEnding||view.file.line_ending||'preserve',
      encoding:view.desiredEncoding||editorEncodingChoice(view.file)
    })
  });
  if(response.status===409)return {conflict:true};
  if(!response.ok)throw new Error((await response.text())||response.statusText);
  return {file:await response.json()};
}
async function readLatestEditorFile(view){
  if(view.file?.remote_workspace_id){
    const data=await app.jsonFetch('/api/remote-workspace-files',{
      method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({workspace_id:view.file.remote_workspace_id,operation:'read',path:view.file.remote_path})
    });
    return remoteEditorFileFromResponse(view.file,data);
  }
  return app.jsonFetch('/api/project/file?path='+encodeURIComponent(view.file.path));
}
const editorSymbolControlWords=new Set(['if','for','while','switch','catch','else','do','return','new','sizeof','typeof','delete','case','with','when']);
function editorSymbolLanguage(pathValue){
  const lower=String(pathValue||'').toLowerCase();
  const dot=lower.lastIndexOf('.'),ext=dot>=0?lower.slice(dot):'';
  if(ext==='.go')return 'go';
  if(['.js','.jsx','.mjs','.cjs','.ts','.tsx'].includes(ext))return 'javascript';
  if(ext==='.py'||ext==='.pyw')return 'python';
  if(['.java','.kt','.kts','.cs','.c','.h','.cc','.cpp','.cxx','.hpp','.hh','.hxx'].includes(ext))return 'c-family';
  if(ext==='.php'||ext==='.phtml')return 'php';
  if(ext==='.rs')return 'rust';
  if(['.sh','.bash','.zsh','.fish'].includes(ext))return 'shell';
  return 'generic';
}
function editorSymbolIndent(line){
  const match=String(line||'').match(/^[ \t]*/);let n=0;
  for(const ch of match?.[0]||'')n+=ch==='\t'?4:1;
  return n;
}
function editorSymbolFromLine(line,language){
  const text=String(line||''),trimmed=text.trim();
  if(!trimmed||trimmed.startsWith('//')||trimmed.startsWith('#')||trimmed.startsWith('*'))return null;
  let match;
  if(language==='python'){
    match=trimmed.match(/^(?:async\s+)?(def|class)\s+([A-Za-z_]\w*)\b/);
    if(match)return {kind:match[1]==='def'?'function':'class',name:match[2]};
  }
  if(language==='go'){
    match=trimmed.match(/^func\s*(?:\([^)]*\)\s*)?([A-Za-z_]\w*)\s*\(/);
    if(match)return {kind:'function',name:match[1]};
    match=trimmed.match(/^type\s+([A-Za-z_]\w*)\s+(struct|interface)\b/);
    if(match)return {kind:match[2],name:match[1]};
  }
  if(language==='javascript'){
    match=trimmed.match(/^(?:export\s+(?:default\s+)?)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\b/);
    if(match)return {kind:'function',name:match[1]};
    match=trimmed.match(/^(?:export\s+(?:default\s+)?)?(class|interface|enum|type)\s+([A-Za-z_$][\w$]*)\b/);
    if(match)return {kind:match[1],name:match[2]};
    match=trimmed.match(/^(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>/);
    if(match)return {kind:'function',name:match[1]};
    match=trimmed.match(/^(?:(?:public|private|protected|static|async|get|set|override|readonly)\s+)*([A-Za-z_$][\w$]*)\s*\([^;]*\)\s*(?::[^={]+)?\s*\{/);
    if(match&&!editorSymbolControlWords.has(match[1]))return {kind:'method',name:match[1]};
  }
  if(language==='c-family'){
    match=trimmed.match(/^(?:(?:public|private|protected|internal|static|final|abstract|sealed|open|data|record)\s+)*(class|struct|interface|enum|namespace)\s+([A-Za-z_]\w*)\b/);
    if(match)return {kind:match[1],name:match[2]};
    match=trimmed.match(/^(?:(?:public|private|protected|internal|static|virtual|override|inline|constexpr|extern|synchronized|native|final|abstract|async)\s+)*(?:[A-Za-z_][\w:<>,.?*&\[\]\s]+\s+)?([A-Za-z_]\w*)\s*\([^;{}]*\)\s*(?:const\s*)?(?:->[^\{]+)?\s*\{/);
    if(match&&!editorSymbolControlWords.has(match[1]))return {kind:'method',name:match[1]};
  }
  if(language==='php'){
    match=trimmed.match(/^(?:(?:final|abstract|readonly)\s+)?(class|interface|trait|enum)\s+([A-Za-z_]\w*)\b/i);
    if(match)return {kind:match[1].toLowerCase(),name:match[2]};
    match=trimmed.match(/^(?:(?:public|private|protected|static|final|abstract)\s+)*(?:async\s+)?function\s+&?\s*([A-Za-z_]\w*)\s*\(/i);
    if(match)return {kind:'function',name:match[1]};
  }
  if(language==='rust'){
    match=trimmed.match(/^(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?fn\s+([A-Za-z_]\w*)\b/);
    if(match)return {kind:'function',name:match[1]};
    match=trimmed.match(/^(?:pub(?:\([^)]*\))?\s+)?(struct|enum|trait|type|mod)\s+([A-Za-z_]\w*)\b/);
    if(match)return {kind:match[1],name:match[2]};
    match=trimmed.match(/^impl(?:<[^>]+>)?\s+([^\s{]+(?:\s+for\s+[^\s{]+)?)\s*\{/);
    if(match)return {kind:'impl',name:match[1]};
  }
  if(language==='shell'){
    match=trimmed.match(/^(?:function\s+)?([A-Za-z_][\w.-]*)\s*(?:\(\s*\))?\s*\{/);
    if(match&&!editorSymbolControlWords.has(match[1]))return {kind:'function',name:match[1]};
  }
  if(language==='generic'){
    match=trimmed.match(/^(?:class|struct|interface)\s+([A-Za-z_]\w*)\b/);
    if(match)return {kind:'type',name:match[1]};
  }
  return null;
}
function extractEditorSymbols(text,pathValue){
  const language=editorSymbolLanguage(pathValue),lines=String(text||'').split(/\r?\n/),out=[];
  for(let i=0;i<lines.length&&out.length<1000;i++){
    const found=editorSymbolFromLine(lines[i],language);if(!found)continue;
    out.push({name:found.name,kind:found.kind,line:i+1,indent:editorSymbolIndent(lines[i]),signature:lines[i].trim().slice(0,300)});
  }
  return out;
}
function jumpEditorToLine(view,lineNumber){
  if(!view||view.closed)return false;
  lineNumber=Math.max(1,Math.min(view.cm.state.doc.lines,Number(lineNumber)||1));
  const line=view.cm.state.doc.line(lineNumber);
  view.cm.dispatch({selection:{anchor:line.from},scrollIntoView:true});
  view.cm.focus();updateEditorBreadcrumb(view);return true;
}
function currentEditorSymbol(view,symbols=null){
  if(!view||view.closed)return null;
  const line=view.cm.state.doc.lineAt(view.cm.state.selection.main.head).number;
  symbols=symbols||extractEditorSymbols(view.cm.state.doc.toString(),view.file.path);
  let current=null;
  for(const symbol of symbols){if(symbol.line>line)break;current=symbol;}
  return current;
}
function updateEditorBreadcrumb(view){
  if(!view?.breadcrumb)return;
  const symbol=currentEditorSymbol(view);
  view.breadcrumb.textContent=basename(view.file.path)+(symbol?' › '+symbol.kind+' '+symbol.name:'');
  view.breadcrumb.title=view.file.path+(symbol?' › '+symbol.signature:'');
}
function showEditorOutline(view){
  if(!view||view.closed)return;
  const symbols=extractEditorSymbols(view.cm.state.doc.toString(),view.file.path);
  const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
  const dialog=document.createElement('div');dialog.className='editor-dirty-dialog editor-outline-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
  const title=document.createElement('h3');title.textContent='Outline';
  const message=document.createElement('p');message.textContent=view.file.path+' · '+symbols.length+' symbol(s) · heuristic parser';
  const filter=document.createElement('input');filter.type='search';filter.className='editor-outline-filter';filter.placeholder='Filter symbols…';
  const list=document.createElement('div');list.className='editor-outline-list';
  const render=()=>{
    list.replaceChildren();const query=String(filter.value||'').trim().toLowerCase();
    const rows=symbols.filter(item=>!query||item.name.toLowerCase().includes(query)||item.kind.toLowerCase().includes(query));
    if(!rows.length){const empty=document.createElement('div');empty.className='editor-history-meta';empty.textContent=symbols.length?'No matching symbols':'No symbols detected in this file';list.append(empty);return;}
    for(const item of rows){
      const row=document.createElement('button');row.type='button';row.className='editor-outline-row';row.title=item.signature;
      const kind=document.createElement('span');kind.className='editor-outline-kind';kind.textContent=item.kind;
      const name=document.createElement('span');name.className='editor-outline-name';name.textContent=item.name;
      const line=document.createElement('span');line.className='editor-outline-line';line.textContent='L'+item.line;
      row.append(kind,name,line);row.onclick=()=>{finish();jumpEditorToLine(view,item.line);};list.append(row);
    }
  };
  const actions=document.createElement('div');actions.className='editor-dirty-actions';const close=document.createElement('button');close.type='button';close.textContent='Close';actions.append(close);
  dialog.append(title,message,filter,list,actions);backdrop.append(dialog);document.body.append(backdrop);
  let done=false;const finish=()=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();};
  const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish();}};
  document.addEventListener('keydown',onKey,true);backdrop.onmousedown=event=>{if(event.target===backdrop)finish();};close.onclick=finish;filter.oninput=render;render();filter.focus();
}

async function loadEditorLocalHistory(view){
  if(!view||view.closed)return [];
  const data=await app.jsonFetch('/api/project/file-history?path='+encodeURIComponent(view.file.path));
  return Array.isArray(data?.entries)?data.entries:[];
}
async function loadEditorLocalHistoryEntry(view,id){
  if(!view||view.closed)throw new Error('Editor is closed');
  return app.jsonFetch('/api/project/file-history?path='+encodeURIComponent(view.file.path)+'&id='+encodeURIComponent(id));
}
function editorHistoryTimestamp(value){
  const date=new Date(String(value||''));
  return Number.isFinite(date.getTime())?date.toLocaleString():String(value||'');
}
function showEditorHistoryCompare(view,entry){
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
    const dialog=document.createElement('div');dialog.className='editor-dirty-dialog editor-compare-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='Local history compare';
    const message=document.createElement('p');message.textContent=view.file.path+' — current editor versus '+editorHistoryTimestamp(entry.saved_at)+'.';
    const grid=document.createElement('div');grid.className='editor-compare-grid';
    const makeSide=(label,text)=>{
      const side=document.createElement('div');side.className='editor-compare-side';
      const head=document.createElement('div');head.className='editor-compare-label';head.textContent=label;
      const pre=document.createElement('pre');pre.className='editor-compare-text';pre.textContent=text;
      side.append(head,pre);return side;
    };
    grid.append(makeSide('Current editor',view.cm.state.doc.toString()),makeSide('Local history',entry.content||''));
    const actions=document.createElement('div');actions.className='editor-dirty-actions';
    const close=document.createElement('button');close.type='button';close.textContent='Close';actions.append(close);
    dialog.append(title,message,grid,actions);backdrop.append(dialog);document.body.append(backdrop);
    let done=false;
    const finish=()=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();resolve();};
    const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish();}};
    document.addEventListener('keydown',onKey,true);backdrop.onmousedown=event=>{if(event.target===backdrop)finish();};close.onclick=finish;close.focus();
  });
}
async function restoreEditorLocalHistory(view,entry){
  if(!view||view.closed||editorReadOnly(view))return false;
  if(view.dirty&&!window.confirm('Restore local history and discard current unsaved editor changes for '+view.file.path+'?'))return false;
  const latest=await readLatestEditorFile(view);
  if(!window.confirm('Restore '+editorHistoryTimestamp(entry.saved_at)+' for '+view.file.path+'?\n\nThe current disk version will be saved into local history first.'))return false;
  const response=await app.fetchWithLease('/api/project/file',{
    method:'PUT',cache:'no-store',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({
      path:view.file.path,
      content:String(entry.content||''),
      expected_sha256:latest.sha256,
      line_ending:entry.line_ending||'preserve',
      encoding:entry.bom?'utf-8-bom':'utf-8'
    })
  });
  if(response.status===409)throw new Error('File changed again while restoring local history. Refresh and retry.');
  if(!response.ok)throw new Error((await response.text())||response.statusText);
  const restored=await response.json();
  setEditorDocument(view,restored);
  if(restored.history_warning)app.showError(new Error(restored.history_warning));
  return true;
}
async function showEditorLocalHistory(view){
  if(!view||view.closed)return;
  const entries=await loadEditorLocalHistory(view);
  const backdrop=document.createElement('div');backdrop.className='editor-dirty-backdrop';
  const dialog=document.createElement('div');dialog.className='editor-dirty-dialog editor-history-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
  const title=document.createElement('h3');title.textContent='Local file history';
  const message=document.createElement('p');message.textContent=view.file.path+' · versions captured before TaskDeck editor saves';
  const list=document.createElement('div');list.className='editor-history-list';
  if(!entries.length){
    const empty=document.createElement('div');empty.className='editor-history-meta';empty.textContent='No local history yet. A version is captured before each successful editor save.';list.append(empty);
  }
  for(const item of entries){
    const row=document.createElement('div');row.className='editor-history-row';
    const main=document.createElement('div');main.className='editor-history-main';
    const when=document.createElement('div');when.className='editor-history-time';when.textContent=editorHistoryTimestamp(item.saved_at);
    const meta=document.createElement('div');meta.className='editor-history-meta';meta.textContent=String(item.sha256||'').slice(0,12)+' · '+formatBytes(item.size||0);
    main.append(when,meta);
    const compare=document.createElement('button');compare.type='button';compare.textContent='Compare';
    const restore=document.createElement('button');restore.type='button';restore.textContent='Restore';restore.disabled=editorReadOnly(view);restore.title=restore.disabled?'Restore is disabled in read-only mode':'Restore this version';
    compare.onclick=async()=>{try{const entry=await loadEditorLocalHistoryEntry(view,item.id);await showEditorHistoryCompare(view,entry);}catch(error){app.showError(error);}};
    restore.onclick=async()=>{try{const entry=await loadEditorLocalHistoryEntry(view,item.id);if(await restoreEditorLocalHistory(view,entry))finish();}catch(error){app.showError(error);}};
    row.append(main,compare,restore);list.append(row);
  }
  const actions=document.createElement('div');actions.className='editor-dirty-actions';
  const close=document.createElement('button');close.type='button';close.textContent='Close';actions.append(close);
  dialog.append(title,message,list,actions);backdrop.append(dialog);document.body.append(backdrop);
  let done=false;
  const finish=()=>{if(done)return;done=true;document.removeEventListener('keydown',onKey,true);backdrop.remove();};
  const onKey=event=>{if(event.key==='Escape'){event.preventDefault();finish();}};
  document.addEventListener('keydown',onKey,true);backdrop.onmousedown=event=>{if(event.target===backdrop)finish();};close.onclick=finish;close.focus();
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
      applySavedEditorFile(view,result.file);
      return result.file;
    }
  }
  return null;
}
async function saveEditor(view){
  if(!view||view.closed||editorReadOnly(view)||!view.dirty||view.saving)return view?.file||null;
  clearTimeout(view.autoSaveTimer);view.autoSaveTimer=null;
  view.saving=true;view.save.disabled=true;view.save.textContent='Saving…';
  try{
    const result=await putEditorFile(view,view.file.sha256);
    if(result.conflict)return resolveSaveConflict(view);
    applySavedEditorFile(view,result.file);
    if(result.file.history_warning)app.showError(new Error(result.file.history_warning));
    return result.file;
  }finally{
    view.saving=false;
    if(!view.closed){
      view.save.textContent='Save';
      view.save.disabled=editorReadOnly(view)||!view.dirty;
    }
  }
}
function rememberClosedEditor(view){
  if(view?.file?.remote_workspace_id)return;
  const pathValue=String(view?.file?.path||'').trim();
  if(!pathValue)return;
  const index=closedEditorPaths.indexOf(pathValue);
  if(index>=0)closedEditorPaths.splice(index,1);
  closedEditorPaths.unshift(pathValue);
  if(closedEditorPaths.length>maxClosedEditorPaths)closedEditorPaths.length=maxClosedEditorPaths;
}
async function reopenClosedEditor(){
  while(closedEditorPaths.length){
    const pathValue=closedEditorPaths.shift();
    if(!pathValue||editors.has(editorID(pathValue)))continue;
    try{return await openFile(pathValue);}
    catch(error){console.warn('Reopen closed editor skipped '+pathValue,error);}
  }
  return null;
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
  rememberClosedEditor(view);
  destroyEditor(id);
  return true;
}
function destroyEditor(id){
  const view=editors.get(id);if(!view)return;
  clearTimeout(view.autoSaveTimer);view.autoSaveTimer=null;
  if(view.minimapRAF){cancelAnimationFrame(view.minimapRAF);view.minimapRAF=0;}
  view.closed=true;
  detachEditorFromSplit(id);
  try{view.cm.destroy();}catch{}
  view.tab.remove();view.pane.remove();editors.delete(id);pruneEditorSplitRoots();
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
  const breadcrumb=document.createElement('div');breadcrumb.className='editor-breadcrumb';breadcrumb.textContent=basename(file.path);breadcrumb.title=file.path;
  const meta=document.createElement('div');meta.className='editor-meta';meta.textContent=editorMetaText(file);
  const lineEndingSelect=document.createElement('select');lineEndingSelect.className='editor-format editor-line-ending';lineEndingSelect.title='Line ending used when saving';
  for(const [value,labelText] of [['lf','LF'],['crlf','CRLF']]){const option=document.createElement('option');option.value=value;option.textContent=labelText;lineEndingSelect.append(option);}
  lineEndingSelect.value=file.line_ending==='crlf'?'crlf':'lf';
  const encodingSelect=document.createElement('select');encodingSelect.className='editor-format editor-encoding';encodingSelect.title='UTF-8 encoding used when saving';
  for(const [value,labelText] of [['utf-8','UTF-8'],['utf-8-bom','UTF-8 BOM']]){const option=document.createElement('option');option.value=value;option.textContent=labelText;encodingSelect.append(option);}
  encodingSelect.value=editorEncodingChoice(file);
  const largeFileBadge=document.createElement('span');largeFileBadge.className='editor-large-file';largeFileBadge.textContent='LARGE FILE';largeFileBadge.title='Large-file mode: read-only plain text without syntax/whitespace decorations';largeFileBadge.hidden=!file.large_file;
  const readonlyBadge=document.createElement('span');readonlyBadge.className='editor-readonly';readonlyBadge.textContent='READ-ONLY';readonlyBadge.hidden=!file.read_only;
  const warningBadge=document.createElement('span');warningBadge.className='editor-warning';warningBadge.textContent=file.warning?'WARNING':'';warningBadge.title=file.warning||'';warningBadge.hidden=!file.warning;
  const save=document.createElement('button');save.type='button';save.className='editor-save';save.textContent='Save';save.title='Save file (Ctrl/Cmd+S)';
  const autoSave=document.createElement('button');autoSave.type='button';autoSave.className='editor-auto-save';autoSave.title='Toggle editor auto-save (1 second debounce)';autoSave.setAttribute('aria-pressed','false');
  const reload=document.createElement('button');reload.type='button';reload.textContent='Reload';reload.title='Reload file from disk';
  const whitespace=document.createElement('button');whitespace.type='button';whitespace.className='editor-whitespace-toggle';whitespace.textContent='WS';whitespace.title='Toggle visible spaces and tabs';whitespace.setAttribute('aria-pressed','false');
  const find=document.createElement('button');find.type='button';find.className='editor-find-toggle';find.textContent='Find';find.title='Find / replace in this file (Ctrl/Cmd+F)';
  const history=document.createElement('button');history.type='button';history.className='editor-history-toggle';history.textContent='History';history.title='Local file history captured before editor saves';history.hidden=Boolean(file.remote_workspace_id);
  const outline=document.createElement('button');outline.type='button';outline.className='editor-outline-toggle';outline.textContent='Outline';outline.title='File symbol outline';
  const minimapToggle=document.createElement('button');minimapToggle.type='button';minimapToggle.className='editor-minimap-toggle';minimapToggle.title='Toggle editor minimap';minimapToggle.setAttribute('aria-pressed','false');
  const splitVertical=document.createElement('button');splitVertical.type='button';splitVertical.className='editor-split-action editor-split-vertical';splitVertical.textContent='Split ↔';splitVertical.title='Split editor vertically with another/open file';
  const splitHorizontal=document.createElement('button');splitHorizontal.type='button';splitHorizontal.className='editor-split-action editor-split-horizontal';splitHorizontal.textContent='Split ↕';splitHorizontal.title='Split editor horizontally with another/open file';
  const splitSwap=document.createElement('button');splitSwap.type='button';splitSwap.className='editor-split-action editor-split-swap';splitSwap.textContent='Swap';splitSwap.title='Swap this editor split';splitSwap.hidden=true;
  const splitUnsplit=document.createElement('button');splitUnsplit.type='button';splitUnsplit.className='editor-split-action editor-unsplit';splitUnsplit.textContent='Unsplit';splitUnsplit.title='Remove this editor from its split';splitUnsplit.hidden=true;
  head.append(pathNode,breadcrumb,meta,lineEndingSelect,encodingSelect,largeFileBadge,readonlyBadge,warningBadge,save,autoSave,reload,find,outline,history,minimapToggle,whitespace,splitVertical,splitHorizontal,splitSwap,splitUnsplit);
  const findPanel=document.createElement('div');findPanel.className='editor-find-panel hidden';
  const findInput=document.createElement('input');findInput.type='text';findInput.placeholder='Find';findInput.setAttribute('aria-label','Find text');
  const findReplaceInput=document.createElement('input');findReplaceInput.type='text';findReplaceInput.placeholder='Replace';findReplaceInput.setAttribute('aria-label','Replace text');findReplaceInput.hidden=true;
  const findPrevious=document.createElement('button');findPrevious.type='button';findPrevious.textContent='↑';findPrevious.title='Previous match (Shift+Enter)';
  const findNext=document.createElement('button');findNext.type='button';findNext.textContent='↓';findNext.title='Next match (Enter)';
  const findReplace=document.createElement('button');findReplace.type='button';findReplace.textContent='Replace';findReplace.hidden=true;
  const findReplaceAll=document.createElement('button');findReplaceAll.type='button';findReplaceAll.textContent='All';findReplaceAll.hidden=true;
  const findCaseLabel=document.createElement('label');const findCase=document.createElement('input');findCase.type='checkbox';findCaseLabel.append(findCase,document.createTextNode('Match case'));
  const findStatus=document.createElement('span');findStatus.className='editor-find-status';
  const findClose=document.createElement('button');findClose.type='button';findClose.textContent='×';findClose.title='Close find';
  findPanel.append(findInput,findReplaceInput,findPrevious,findNext,findReplace,findReplaceAll,findCaseLabel,findStatus,findClose);
  const body=document.createElement('div');body.className='editor-body';
  const host=document.createElement('div');host.className='editor-host';
  const minimap=document.createElement('canvas');minimap.className='editor-minimap';minimap.title='File minimap · click to jump';
  body.append(host,minimap);pane.append(head,findPanel,body);panesHost.append(pane);

  const cm=cmFactory.newEditor(host,file.content||'',editorOptionsForFile(file));
  const view={id,file:{...file},desiredLineEnding:file.line_ending||'lf',desiredEncoding:editorEncodingChoice(file),tab,label,dirty,pane,head,path:pathNode,breadcrumb,meta,lineEndingSelect,encodingSelect,largeFileBadge,readonlyBadge,warningBadge,save,autoSave,reload,find,outline,history,findPanel,findInput,findReplaceInput,findPrevious,findNext,findReplace,findReplaceAll,findCase,findStatus,findClose,minimapToggle,minimap,body,whitespace,splitVertical,splitHorizontal,splitSwap,splitUnsplit,host,cm,closed:false,dirty:false,saving:false,tabReadOnly:false,internalUpdate:false,dispatchRaw:null,autoSaveTimer:null,minimapRAF:0,scroller:cm.dom.querySelector('.cm-scroller')};
  editors.set(id,view);
  pane.classList.toggle('editor-large-file-mode',Boolean(file.large_file));
  installEditorDispatchGuard(view);
  applyReadOnly(view);
  setDirty(view,false);
  updateEditorBreadcrumb(view);
  updateMinimapButton(view);
  if(view.scroller)view.scroller.addEventListener('scroll',()=>scheduleEditorMinimap(view),{passive:true});
  minimap.onclick=event=>jumpFromEditorMinimap(view,event);
  minimapToggle.onclick=()=>setEditorMinimap(!editorMinimapEnabled);

  cm.contentDOM.addEventListener('beforeinput',event=>{if(editorReadOnly(view))event.preventDefault();},true);
  cm.contentDOM.addEventListener('keyup',()=>updateEditorBreadcrumb(view));
  cm.contentDOM.addEventListener('click',()=>updateEditorBreadcrumb(view));
  cm.contentDOM.addEventListener('keydown',event=>{
    const shortcut=(event.ctrlKey||event.metaKey)&&!event.altKey;
    const shortcutKey=String(event.key||'').toLowerCase();
    if(shortcut&&(shortcutKey==='f'||shortcutKey==='h')){
      event.preventDefault();event.stopPropagation();
      openEditorFind(view,{replace:shortcutKey==='h'});
      return;
    }
    if(event.key==='Tab'&&!event.ctrlKey&&!event.metaKey&&!event.altKey&&!event.shiftKey&&!editorReadOnly(view)){
      event.preventDefault();
      view.cm.dispatch(view.cm.state.replaceSelection('\t'));
    }
  },true);
  tab.onclick=()=>activateEditor(id,{force:true});
  close.onclick=event=>{event.stopPropagation();closeEditor(id).catch(app.showError);};
  save.onmousedown=event=>event.preventDefault();
  save.onclick=()=>saveEditor(view).catch(app.showError);
  autoSave.onclick=()=>setEditorAutoSave(!editorAutoSaveEnabled);
  lineEndingSelect.onchange=()=>{
    if(editorReadOnly(view)){syncEditorFormatControls(view);return;}
    view.desiredLineEnding=lineEndingSelect.value;
    setDirty(view,true);
  };
  encodingSelect.onchange=()=>{
    if(editorReadOnly(view)){syncEditorFormatControls(view);return;}
    view.desiredEncoding=encodingSelect.value;
    setDirty(view,true);
  };
  reload.onclick=()=>reloadEditor(view).catch(app.showError);
  find.onclick=()=>openEditorFind(view,{replace:false});
  outline.onclick=()=>showEditorOutline(view);
  history.onclick=()=>showEditorLocalHistory(view).catch(app.showError);
  findNext.onclick=()=>editorFindMatch(view,{direction:1});
  findPrevious.onclick=()=>editorFindMatch(view,{direction:-1});
  findReplace.onclick=()=>replaceEditorMatch(view);
  findReplaceAll.onclick=()=>replaceAllEditorMatches(view);
  findCase.onchange=()=>{view.findStatus.textContent='';};
  findClose.onclick=()=>closeEditorFind(view);
  findInput.onkeydown=event=>{
    if(event.key==='Enter'){event.preventDefault();editorFindMatch(view,{direction:event.shiftKey?-1:1});}
    else if(event.key==='Escape'){event.preventDefault();closeEditorFind(view);}
  };
  findReplaceInput.onkeydown=event=>{
    if(event.key==='Enter'){event.preventDefault();replaceEditorMatch(view);}
    else if(event.key==='Escape'){event.preventDefault();closeEditorFind(view);}
  };
  whitespace.onclick=()=>{
    const enabled=!pane.classList.contains('editor-show-whitespace');
    pane.classList.toggle('editor-show-whitespace',enabled);
    whitespace.setAttribute('aria-pressed',enabled?'true':'false');
    whitespace.classList.toggle('active',enabled);
  };
  splitVertical.onclick=()=>splitEditor(view,'vertical').catch(app.showError);
  splitHorizontal.onclick=()=>splitEditor(view,'horizontal').catch(app.showError);
  splitSwap.onclick=()=>swapEditorSplit(view);
  splitUnsplit.onclick=()=>unsplitEditor(view);
  pane.addEventListener('pointerdown',()=>{if(activeEditorID!==id)activateEditor(id,{force:true});},true);
  updateEditorSplitButtons(view);
  return view;
}
async function reloadEditor(view){
  if(!view||view.closed)return;
  if(view.dirty&&!window.confirm('Discard unsaved changes and reload '+view.file.path+'?'))return;
  const file=await readLatestEditorFile(view);
  setEditorDocument(view,file);
}

async function handleExternalFileChange(view,latest=null){
  if(!view||view.closed||view.saving)return false;
  latest=latest||await readLatestEditorFile(view);
  if(!latest||latest.sha256===view.file?.sha256)return false;
  if(!view.dirty){
    setEditorDocument(view,latest);
    window.dispatchEvent(new CustomEvent('taskmenu:editor-external-reload',{detail:{path:view.file.path}}));
    return true;
  }
  if(view.externalConflictPending)return false;
  view.externalConflictPending=true;
  try{
    while(!view.closed){
      const choice=await conflictChoice(view);
      if(choice==='cancel')return false;
      if(choice==='compare'){await showConflictCompare(view,latest);continue;}
      if(choice==='reload'){setEditorDocument(view,latest);return true;}
      if(choice==='overwrite'){
        const result=await putEditorFile(view,latest.sha256);
        if(result.conflict){latest=await readLatestEditorFile(view);continue;}
        applySavedEditorFile(view,result.file);
        return true;
      }
    }
  }finally{view.externalConflictPending=false;}
  return false;
}
function announceOpenedFile(pathValue){
  window.dispatchEvent(new CustomEvent('taskmenu:project-file-opened',{detail:{path:pathValue}}));
}

function closeEditorsForTrashedPath(pathValue){
  pathValue=String(pathValue||'').trim();
  if(!pathValue)return;
  const prefix=pathValue+'/';
  const affected=[...editors.values()].filter(view=>view.file.path===pathValue||view.file.path.startsWith(prefix));
  for(const view of affected){
    if(view.dirty)continue;
    destroyEditor(view.id);
  }
}
function remapOpenedEditorPaths(oldPath,newPath){
  oldPath=String(oldPath||'').trim();newPath=String(newPath||'').trim();
  if(!oldPath||!newPath||oldPath===newPath)return;
  const prefix=oldPath+'/';
  const changes=[...editors.entries()].filter(([,view])=>view.file.path===oldPath||view.file.path.startsWith(prefix));
  for(const [oldID,view] of changes){
    const suffix=view.file.path===oldPath?'':view.file.path.slice(oldPath.length);
    const nextPath=newPath+suffix;
    const nextID=editorID(nextPath);
    if(nextID!==oldID&&editors.has(nextID))continue;
    editors.delete(oldID);
    view.id=nextID;
    view.file.path=nextPath;
    view.tab.dataset.id=nextID;
    view.pane.dataset.id=nextID;
    view.label.textContent=basename(nextPath);
    view.path.textContent=nextPath;
    view.path.title=nextPath;
    view.tab.title=(view.dirty?'● ':'')+nextPath;
    editors.set(nextID,view);
    if(activeEditorID===oldID)activeEditorID=nextID;
    for(const root of editorSplitRoots)remapEditorSplitID(root,oldID,nextID);
  }
  pruneEditorSplitRoots();
  syncEditorSplitForActive();
}
async function openDocument(file){
  if(!file?.path)throw new Error('Editor document path is required');
  const id=editorID(file.path);
  if(editors.has(id)){
    const view=editors.get(id);
    if(!view.dirty)setEditorDocument(view,file);
    activateEditor(id,{force:true});
    return view;
  }
  const view=createEditor(file);
  activateEditor(view.id,{force:true});
  return view;
}

async function openFile(pathValue){
  pathValue=String(pathValue||'').trim();
  if(!pathValue)return;
  const id=editorID(pathValue);
  if(editors.has(id)){
    activateEditor(id,{force:true});
    announceOpenedFile(pathValue);
    return editors.get(id);
  }
  if(opening.has(pathValue)){
    const pending=await opening.get(pathValue);
    activateEditor(pending.id,{force:true});
    announceOpenedFile(pathValue);
    return pending;
  }
  const promise=(async()=>{
    const file=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(pathValue));
    return createEditor(file);
  })();
  opening.set(pathValue,promise);
  try{
    const view=await promise;
    activateEditor(view.id,{force:true});
    announceOpenedFile(pathValue);
    return view;
  }finally{opening.delete(pathValue);}
}


function goToLine(view){
  if(!view||view.closed)return;
  const raw=window.prompt('Go to line (1-'+view.cm.state.doc.lines+'):','1');
  if(raw===null)return;
  const lineNumber=Math.max(1,Math.min(view.cm.state.doc.lines,Number.parseInt(raw,10)||1));
  jumpEditorToLine(view,lineNumber);
}

function editorFindComparable(value,matchCase){
  value=String(value??'');
  return matchCase?value:value.toLowerCase();
}
function editorFindMatch(view,{direction=1,from=null,wrap=true}={}){
  if(!view||view.closed||!view.findInput)return null;
  const query=String(view.findInput.value||'');
  if(!query){view.findStatus.textContent='Enter text';return null;}
  const matchCase=Boolean(view.findCase?.checked);
  const text=view.cm.state.doc.toString();
  const source=editorFindComparable(text,matchCase),needle=editorFindComparable(query,matchCase);
  const selection=view.cm.state.selection.main;
  let start=Number.isInteger(from)?from:(direction<0?selection.from-1:selection.to);
  let index=direction<0?source.lastIndexOf(needle,Math.max(-1,start)):source.indexOf(needle,Math.max(0,start));
  if(index<0&&wrap){
    index=direction<0?source.lastIndexOf(needle):source.indexOf(needle);
  }
  if(index<0){view.findStatus.textContent='No matches';return null;}
  const to=index+query.length;
  view.cm.dispatch({selection:{anchor:index,head:to},scrollIntoView:true});
  const line=view.cm.state.doc.lineAt(index).number;
  view.findStatus.textContent='Line '+line;
  view.cm.focus();
  return {from:index,to,query};
}
function editorSelectionMatchesFind(view){
  const query=String(view?.findInput?.value||'');
  if(!query)return false;
  const selection=view.cm.state.selection.main;
  if(selection.from===selection.to)return false;
  const selected=view.cm.state.doc.sliceString(selection.from,selection.to);
  return editorFindComparable(selected,Boolean(view.findCase?.checked))===editorFindComparable(query,Boolean(view.findCase?.checked));
}
function syncEditorFindReadOnly(view){
  if(!view?.findPanel)return;
  const readonly=editorReadOnly(view);
  view.findReplace.disabled=readonly;
  view.findReplaceAll.disabled=readonly;
  view.findReplaceInput.disabled=readonly;
  view.findReplace.title=readonly?'Replace is disabled in read-only mode':'Replace current match';
  view.findReplaceAll.title=readonly?'Replace is disabled in read-only mode':'Replace all matches';
}
function openEditorFind(view,{replace=false}={}){
  if(!view||view.closed||!view.findPanel)return;
  view.findPanel.classList.remove('hidden');
  view.findPanel.dataset.replace=replace?'1':'0';
  view.findReplaceInput.hidden=!replace;
  view.findReplace.hidden=!replace;
  view.findReplaceAll.hidden=!replace;
  const selection=view.cm.state.selection.main;
  if(!view.findInput.value&&selection.to>selection.from&&selection.to-selection.from<=240){
    view.findInput.value=view.cm.state.doc.sliceString(selection.from,selection.to);
  }
  syncEditorFindReadOnly(view);
  view.findStatus.textContent='';
  view.findInput.focus();
  view.findInput.select();
}
function closeEditorFind(view){
  if(!view?.findPanel)return;
  view.findPanel.classList.add('hidden');
  view.findStatus.textContent='';
  try{view.cm.focus();}catch{}
}
function replaceEditorMatch(view){
  if(!view||view.closed||editorReadOnly(view))return false;
  if(!editorSelectionMatchesFind(view)){
    if(!editorFindMatch(view,{direction:1}))return false;
  }
  if(!editorSelectionMatchesFind(view))return false;
  const selection=view.cm.state.selection.main;
  const replacement=String(view.findReplaceInput.value||'');
  view.cm.dispatch({changes:{from:selection.from,to:selection.to,insert:replacement},selection:{anchor:selection.from+replacement.length}});
  editorFindMatch(view,{direction:1,from:selection.from+replacement.length,wrap:true});
  return true;
}
function replaceAllEditorMatches(view){
  if(!view||view.closed||editorReadOnly(view))return 0;
  const query=String(view.findInput.value||'');
  if(!query){view.findStatus.textContent='Enter text';return 0;}
  const replacement=String(view.findReplaceInput.value||'');
  const matchCase=Boolean(view.findCase?.checked);
  const text=view.cm.state.doc.toString(),source=editorFindComparable(text,matchCase),needle=editorFindComparable(query,matchCase);
  const changes=[];let at=0;
  while(at<=source.length-needle.length&&changes.length<10000){
    const found=source.indexOf(needle,at);if(found<0)break;
    changes.push({from:found,to:found+query.length,insert:replacement});
    at=found+Math.max(1,query.length);
  }
  if(!changes.length){view.findStatus.textContent='No matches';return 0;}
  view.cm.dispatch({changes});
  view.findStatus.textContent='Replaced '+changes.length;
  return changes.length;
}

document.addEventListener('keydown',event=>{
  if(!(event.ctrlKey||event.metaKey))return;
  const key=event.key.toLowerCase();
  if(key==='t'&&event.shiftKey){
    if(!closedEditorPaths.length)return;
    event.preventDefault();
    reopenClosedEditor().catch(app.showError);
    return;
  }
  if(!activeEditorID)return;
  const view=editors.get(activeEditorID);if(!view||view.closed)return;
  if(key==='o'&&event.shiftKey){
    event.preventDefault();
    showEditorOutline(view);
  }else if(key==='s'){
    event.preventDefault();
    saveEditor(view).catch(app.showError);
  }else if(key==='f'){
    event.preventDefault();
    openEditorFind(view,{replace:false});
  }else if(key==='h'){
    event.preventDefault();
    openEditorFind(view,{replace:true});
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
window.addEventListener('taskmenu:project-path-renamed',event=>{
  remapOpenedEditorPaths(event.detail?.old_path,event.detail?.new_path);
});
window.addEventListener('taskmenu:project-path-trashed',event=>{
  closeEditorsForTrashedPath(event.detail?.path);
});
window.addEventListener('taskmenu:view-activated',event=>{
  if(event.detail?.kind==='terminal'){deactivateEditors();return;}
  if(event.detail?.kind==='external'){
    if(editors.has(event.detail.id))activateEditorDOM(event.detail.id);
    else deactivateEditors();
  }
});
window.addEventListener('resize',()=>{if(renderedEditorSplitRoot)layoutEditorSplit();for(const view of editors.values())scheduleEditorMinimap(view);});

globalThis.TaskMenuEditor={
  editors,
  snapshotState(){
    const localViews=[...editors.values()].filter(view=>!view.closed&&!view.file?.remote_workspace_id);
    const files=localViews.map(view=>String(view.file?.path||'')).filter(Boolean);
    const activeView=activeEditorID?editors.get(activeEditorID):null;
    return {files,active:activeView?.file?.remote_workspace_id?'':String(activeView?.file?.path||'')};
  },
  async restoreState(state){
    const files=Array.isArray(state?.files)?state.files.map(String).filter(Boolean):[];
    for(const pathValue of files){
      try{await openFile(pathValue);}catch(error){console.warn('Snapshot editor restore skipped '+pathValue,error);}
    }
    const active=String(state?.active||'').trim();
    if(active&&files.includes(active)){
      try{await openFile(active);}catch(error){console.warn('Snapshot active editor restore failed',error);}
    }
    return true;
  },
  openFile,
  openDocument,
  reloadEditor,
  handleExternalFileChange,
  saveEditor,
  setEditorAutoSave,
  setEditorMinimap,
  closeEditor,
  reopenClosedEditor,
  goToLine,
  jumpEditorToLine,
  extractEditorSymbols,
  showEditorOutline,
  updateEditorBreadcrumb,
  openEditorFind,
  editorFindMatch,
  replaceEditorMatch,
  replaceAllEditorMatches,
  showEditorLocalHistory,
  restoreEditorLocalHistory,
  activateEditor,
  destroyEditor,
  splitEditor,
  swapEditorSplit,
  unsplitEditor,
  syncEditorSplitForActive,
  setTabReadOnly,
  get splitSupported(){return editorSplitSupported;},
  getSplitParent:viewOrID=>{
    const id=typeof viewOrID==='string'?viewOrID:viewOrID?.id;
    return id?editorSplitParentFor(id):null;
  },
  isTabReadOnly:viewOrID=>{
    const view=typeof viewOrID==='string'?editors.get(viewOrID):viewOrID;
    return Boolean(view?.tabReadOnly);
  },
  get autoSaveEnabled(){return editorAutoSaveEnabled;},
  get minimapEnabled(){return editorMinimapEnabled;},
  get active(){return activeEditorID;}
};
