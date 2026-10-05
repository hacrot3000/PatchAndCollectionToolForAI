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
  if(options.tsx)return 'TSX';
  if(options.typescript)return 'TypeScript';
  if(options.jsx)return 'JSX';
  for(const key of ['cpp','go','python','javascript','json','yaml','markdown','html','css','xml','java','php','sql','rust','vue']){
    if(options[key])return key==='cpp'?'C/C++':key==='javascript'?'JavaScript':key.toUpperCase();
  }
  const legacy=legacyHighlightKind(pathValue);
  if(legacy==='cmake')return 'CMake';
  if(legacy==='shell')return 'Shell';
  if(legacy==='lua')return 'Lua';
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
  const root=editorFindSplitRoot(id);
  for(const [editorIDValue,view] of editors){
    const yes=editorIDValue===id;
    const peer=Boolean(root&&editorSplitContains(root,editorIDValue));
    view.tab.classList.toggle('active',yes);
    view.pane.classList.toggle('hidden',root? !peer:!yes);
    updateEditorSplitButtons(view);
    if(yes)setTimeout(()=>{try{view.cm.focus();}catch{}},0);
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
async function putEditorFile(view,expectedSHA256){
  const response=await app.fetchWithLease('/api/project/file',{
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
  if(!view||view.closed||editorReadOnly(view)||!view.dirty||view.saving)return view?.file||null;
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
      view.save.disabled=editorReadOnly(view)||!view.dirty;
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
  const meta=document.createElement('div');meta.className='editor-meta';meta.textContent=editorMetaText(file);
  const readonlyBadge=document.createElement('span');readonlyBadge.className='editor-readonly';readonlyBadge.textContent='READ-ONLY';readonlyBadge.hidden=!file.read_only;
  const warningBadge=document.createElement('span');warningBadge.className='editor-warning';warningBadge.textContent=file.warning?'WARNING':'';warningBadge.title=file.warning||'';warningBadge.hidden=!file.warning;
  const save=document.createElement('button');save.type='button';save.className='editor-save';save.textContent='Save';save.title='Save file (Ctrl/Cmd+S)';
  const reload=document.createElement('button');reload.type='button';reload.textContent='Reload';reload.title='Reload file from disk';
  const splitVertical=document.createElement('button');splitVertical.type='button';splitVertical.className='editor-split-action editor-split-vertical';splitVertical.textContent='Split ↔';splitVertical.title='Split editor vertically with another/open file';
  const splitHorizontal=document.createElement('button');splitHorizontal.type='button';splitHorizontal.className='editor-split-action editor-split-horizontal';splitHorizontal.textContent='Split ↕';splitHorizontal.title='Split editor horizontally with another/open file';
  const splitSwap=document.createElement('button');splitSwap.type='button';splitSwap.className='editor-split-action editor-split-swap';splitSwap.textContent='Swap';splitSwap.title='Swap this editor split';splitSwap.hidden=true;
  const splitUnsplit=document.createElement('button');splitUnsplit.type='button';splitUnsplit.className='editor-split-action editor-unsplit';splitUnsplit.textContent='Unsplit';splitUnsplit.title='Remove this editor from its split';splitUnsplit.hidden=true;
  head.append(pathNode,meta,readonlyBadge,warningBadge,save,reload,splitVertical,splitHorizontal,splitSwap,splitUnsplit);
  const host=document.createElement('div');host.className='editor-host';
  pane.append(head,host);panesHost.append(pane);

  const cm=cmFactory.newEditor(host,file.content||'',languageOptions(file.path));
  const view={id,file:{...file},tab,label,dirty,pane,head,path:pathNode,meta,readonlyBadge,warningBadge,save,reload,splitVertical,splitHorizontal,splitSwap,splitUnsplit,host,cm,closed:false,dirty:false,saving:false,tabReadOnly:false,internalUpdate:false,dispatchRaw:null};
  editors.set(id,view);
  installEditorDispatchGuard(view);
  applyReadOnly(view);
  setDirty(view,false);

  cm.contentDOM.addEventListener('beforeinput',event=>{if(editorReadOnly(view))event.preventDefault();},true);
  cm.contentDOM.addEventListener('keydown',event=>{
    if(event.key==='Tab'&&!event.ctrlKey&&!event.metaKey&&!event.altKey&&!event.shiftKey&&!editorReadOnly(view)){
      event.preventDefault();
      view.cm.dispatch(view.cm.state.replaceSelection('\t'));
    }
  },true);
  tab.onclick=()=>activateEditor(id,{force:true});
  close.onclick=event=>{event.stopPropagation();closeEditor(id).catch(app.showError);};
  save.onclick=()=>saveEditor(view).catch(app.showError);
  reload.onclick=()=>reloadEditor(view).catch(app.showError);
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
  const file=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(view.file.path));
  setEditorDocument(view,file);
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
async function openFile(pathValue){
  pathValue=String(pathValue||'').trim();
  if(!pathValue)return;
  const id=editorID(pathValue);
  if(editors.has(id)){
    activateEditor(id);
    announceOpenedFile(pathValue);
    return editors.get(id);
  }
  if(opening.has(pathValue)){
    const pending=await opening.get(pathValue);
    activateEditor(pending.id);
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
    activateEditor(view.id);
    announceOpenedFile(pathValue);
    return view;
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
window.addEventListener('resize',()=>{if(renderedEditorSplitRoot)layoutEditorSplit();});

globalThis.TaskMenuEditor={
  editors,
  snapshotState(){
    const files=[...editors.values()].filter(view=>!view.closed).map(view=>String(view.file?.path||'')).filter(Boolean);
    const activeView=activeEditorID?editors.get(activeEditorID):null;
    return {files,active:String(activeView?.file?.path||'')};
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
  reloadEditor,
  saveEditor,
  closeEditor,
  goToLine,
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
  get active(){return activeEditorID;}
};
