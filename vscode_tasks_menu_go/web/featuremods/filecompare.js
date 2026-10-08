const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for file compare');

const style=document.createElement('style');
style.textContent=`
.file-compare-backdrop{display:none;position:fixed;inset:0;z-index:2750;background:rgba(0,0,0,.52);padding:20px}
.file-compare-backdrop.visible{display:flex}
.file-compare-dialog{width:min(1500px,calc(100vw - 40px));height:min(900px,calc(100vh - 40px));margin:auto;display:flex;flex-direction:column;min-width:0;min-height:0;background:#10151c;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 58px rgba(0,0,0,.55);overflow:hidden}
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
.file-compare-body{flex:1;min-height:0;overflow:auto;background:#0b0f14}
.file-compare-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);min-width:780px}
.file-compare-cell{display:grid;grid-template-columns:48px minmax(0,1fr);min-width:0;border-bottom:1px solid rgba(255,255,255,.035)}
.file-compare-cell+.file-compare-cell{border-left:1px solid #303843}.file-compare-no{padding:1px 7px;text-align:right;user-select:none;opacity:.45;font:10px/1.45 ui-monospace,monospace;border-right:1px solid rgba(255,255,255,.06)}
.file-compare-code{padding:1px 7px;white-space:pre;overflow-x:auto;font:11px/1.45 ui-monospace,monospace}.file-compare-cell.removed.important{background:rgba(229,72,86,.28)}.file-compare-cell.added.important{background:rgba(232,174,55,.28)}.file-compare-cell.removed.unimportant,.file-compare-cell.added.unimportant{background:rgba(58,149,214,.23)}.file-compare-cell.blank{opacity:.3}
.file-compare-syntax-keyword{color:#c792ea}.file-compare-syntax-comment{color:#6a9955;font-style:italic}.file-compare-syntax-string{color:#ce9178}.file-compare-syntax-number{color:#b5cea8}.file-compare-syntax-command{color:#dcdcaa}.file-compare-syntax-variable{color:#9cdcfe}
.file-compare-inline-change{border-radius:2px;box-shadow:inset 0 -1px 0 rgba(255,255,255,.28)}.file-compare-inline-change.removed{background:rgba(255,84,98,.38)}.file-compare-inline-change.added{background:rgba(255,196,74,.40)}
.file-compare-hunk{border-top:1px solid #3a4350;border-bottom:1px solid #3a4350;margin:5px 0}.file-compare-hunk-head{position:sticky;left:0;display:flex;align-items:center;gap:6px;padding:4px 8px;background:#171e27;font:10px ui-monospace,monospace;z-index:1}.file-compare-hunk-label{flex:1;opacity:.72}.file-compare-hunk-head button{padding:2px 6px;font-size:10px}
.file-compare-inline{min-width:640px}.file-compare-inline-line{display:grid;grid-template-columns:52px 20px minmax(0,1fr);border-bottom:1px solid rgba(255,255,255,.035);font:11px/1.45 ui-monospace,monospace}.file-compare-inline-line span{padding:1px 7px}.file-compare-inline-no{text-align:right;opacity:.45}.file-compare-inline-line.removed.important{background:rgba(229,72,86,.28)}.file-compare-inline-line.added.important{background:rgba(232,174,55,.28)}.file-compare-inline-line.removed.unimportant,.file-compare-inline-line.added.unimportant{background:rgba(58,149,214,.23)}
.file-compare-empty{padding:24px;text-align:center;opacity:.62}
html[data-taskmenu-theme="light"] .file-compare-dialog{background:#fff;border-color:#b9c0c8}.file-compare-body{color:inherit}html[data-taskmenu-theme="light"] .file-compare-filters{background:#f8fafc;border-color:#d0d7de}html[data-taskmenu-theme="light"] .file-compare-editor-deck{background:#fff;border-color:#d0d7de}html[data-taskmenu-theme="light"] .file-compare-editor-head{background:#f6f8fa;border-color:#d0d7de}html[data-taskmenu-theme="light"] .file-compare-columns,html[data-taskmenu-theme="light"] .file-compare-hunk-head{background:#f6f8fa;border-color:#d0d7de}html[data-taskmenu-theme="light"] .file-compare-cell.removed.important,html[data-taskmenu-theme="light"] .file-compare-inline-line.removed.important{background:#ffe2e5}html[data-taskmenu-theme="light"] .file-compare-cell.added.important,html[data-taskmenu-theme="light"] .file-compare-inline-line.added.important{background:#fff1c9}html[data-taskmenu-theme="light"] .file-compare-cell.removed.unimportant,html[data-taskmenu-theme="light"] .file-compare-cell.added.unimportant,html[data-taskmenu-theme="light"] .file-compare-inline-line.removed.unimportant,html[data-taskmenu-theme="light"] .file-compare-inline-line.added.unimportant{background:#e3f3ff}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='file-compare-backdrop';
const dialog=document.createElement('div');dialog.className='file-compare-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','File compare');
const head=document.createElement('div');head.className='file-compare-head';
const title=document.createElement('div');title.className='file-compare-title';
const summary=document.createElement('div');summary.className='file-compare-summary';
const sideButton=document.createElement('button');sideButton.type='button';sideButton.textContent='Side by side';
const inlineButton=document.createElement('button');inlineButton.type='button';inlineButton.textContent='Inline';
const editButton=document.createElement('button');editButton.type='button';editButton.className='file-compare-edit-toggle';editButton.textContent='Edit';editButton.title='Edit both writable files with syntax highlighting';
const leftTopSave=document.createElement('button');leftTopSave.type='button';leftTopSave.className='file-compare-save';leftTopSave.textContent='Save Left';leftTopSave.title='Save the left compare file';
const rightTopSave=document.createElement('button');rightTopSave.type='button';rightTopSave.className='file-compare-save';rightTopSave.textContent='Save Right';rightTopSave.title='Save the right compare file';
const reloadButton=document.createElement('button');reloadButton.type='button';reloadButton.textContent='↻';reloadButton.title='Reload both compare sources';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close compare';
head.append(title,summary,sideButton,inlineButton,editButton,leftTopSave,rightTopSave,reloadButton,closeButton);
const filters=document.createElement('div');filters.className='file-compare-filters';
const viewAllButton=document.createElement('button');viewAllButton.type='button';viewAllButton.textContent='View all';viewAllButton.title='Show the complete file comparison';
const viewDiffButton=document.createElement('button');viewDiffButton.type='button';viewDiffButton.textContent='View diff';viewDiffButton.title='Show changed lines only';
const viewContextButton=document.createElement('button');viewContextButton.type='button';viewContextButton.textContent='View diff context';viewContextButton.title='Show changed lines plus the enclosing brace/indentation block when it can be identified';
const viewUnimportantButton=document.createElement('button');viewUnimportantButton.type='button';viewUnimportantButton.textContent='View unimportant';viewUnimportantButton.title='Toggle confirmed comment/cosmetic changes (blue)';
const filterNote=document.createElement('span');filterNote.className='file-compare-filter-note';filterNote.textContent='blue = unimportant · red/amber = logic-sensitive';
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
dialog.append(head,filters,columns,editorDeck,body);backdrop.append(dialog);document.body.append(backdrop);

let current=null;
let viewMode='side';
let contentMode='all';
let showUnimportant=true;
let compareSelection=null;
let editMode=false;
let renderTimer=0;

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
  const source=compareSideSource(side);if(!source||!sourceWritable(source))throw new Error((source?.label||side)+' is read-only');
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
function classifyCompareRow(row){
  if(row.hunk<0)return 'context';
  const syntax=ensureCompareSyntax();
  const leftText=row.left?.text??'',rightText=row.right?.text??'';
  const leftTokens=row.left?syntax.left[row.left.no-1]||[]:[],rightTokens=row.right?syntax.right[row.right.no-1]||[]:[];
  if(!row.left||!row.right){
    const text=row.left?leftText:rightText,tokens=row.left?leftTokens:rightTokens;
    if(text.trim()===''||compareCommentOnly(text,tokens))return 'unimportant';
    return 'important';
  }
  if(leftText===rightText)return 'context';
  if(compareCommentOnly(leftText,leftTokens)&&compareCommentOnly(rightText,rightTokens))return 'unimportant';
  const leftComments=compareCommentTokens(leftTokens),rightComments=compareCommentTokens(rightTokens);
  if((leftComments.length||rightComments.length)&&compareTextWithoutRanges(leftText,leftComments).trimEnd()===compareTextWithoutRanges(rightText,rightComments).trimEnd())return 'unimportant';
  const language=compareRowLanguageID(row);
  if(compareIndentInsensitiveLanguages.has(language)&&leftText.trim()===rightText.trim())return 'unimportant';
  return 'important';
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
  const no=document.createElement('span');no.className='file-compare-no';no.textContent=spec?.no?String(spec.no):'';
  const code=document.createElement('span');code.className='file-compare-code';
  const syntax=ensureCompareSyntax();
  const tokens=spec?.no?(side==='left'?syntax.left:syntax.right)[spec.no-1]||[]:[];
  renderCompareCode(code,spec?spec.text:'',tokens,changed,spec?.kind==='removed'?'removed':spec?.kind==='added'?'added':'');
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
      const label=document.createElement('span');label.className='file-compare-hunk-label';label.textContent='Change '+(h.index+1)+' · left '+(h.leftStart+1)+'+'+h.leftCount+' ↔ right '+(h.rightStart+1)+'+'+h.rightCount+' · '+important+' important / '+unimportant+' unimportant';
      hh.append(label);
      if(sourceWritable(current?.right)){const b=document.createElement('button');b.type='button';b.textContent='Left → Right';b.title='Apply the entire change block, including rows hidden by filters';b.onclick=()=>copyHunk(h,'left-to-right').catch(app.showError);hh.append(b);}
      if(sourceWritable(current?.left)){const b=document.createElement('button');b.type='button';b.textContent='Right → Left';b.title='Apply the entire change block, including rows hidden by filters';b.onclick=()=>copyHunk(h,'right-to-left').catch(app.showError);hh.append(b);}
      card.append(hh);frag.append(card);
    }
    const ranges=compareInlineRanges(row.left?.text||'',row.right?.text||'');
    const line=document.createElement('div');line.className='file-compare-row';line.append(cell(row.left,'left',ranges.left,row.importance),cell(row.right,'right',ranges.right,row.importance));frag.append(line);
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
      const code=document.createElement('span');const syntax=ensureCompareSyntax();renderCompareCode(code,row.left.text,syntax.left[row.left.no-1]||[]);line.append(no,mark,code);host.append(line);continue;
    }
    if(row.left){
      const line=document.createElement('div');line.className='file-compare-inline-line removed '+row.importance;
      const no=document.createElement('span');no.className='file-compare-inline-no';no.textContent=String(row.left.no);
      const mark=document.createElement('span');mark.textContent='−';const code=document.createElement('span');const syntax=ensureCompareSyntax();const ranges=compareInlineRanges(row.left.text,row.right?.text||'');renderCompareCode(code,row.left.text,syntax.left[row.left.no-1]||[],ranges.left,'removed');line.append(no,mark,code);host.append(line);
    }
    if(row.right){
      const line=document.createElement('div');line.className='file-compare-inline-line added '+row.importance;
      const no=document.createElement('span');no.className='file-compare-inline-no';no.textContent=String(row.right.no);
      const mark=document.createElement('span');mark.textContent='+';const code=document.createElement('span');const syntax=ensureCompareSyntax();const ranges=compareInlineRanges(row.left?.text||'',row.right.text);renderCompareCode(code,row.right.text,syntax.right[row.right.no-1]||[],ranges.right,'added');line.append(no,mark,code);host.append(line);
    }
  }
  body.append(host);
}
function render(){
  body.replaceChildren();if(!current)return;
  const model=enrichCompareModel(buildCompareModel(current.left.text,current.right.text));current.model=model;
  leftLabel.textContent=current.left.label||'Left';rightLabel.textContent=current.right.label||'Right';
  title.textContent=(current.title||'File Compare')+' · '+leftLabel.textContent+' ↔ '+rightLabel.textContent;
  summary.textContent=model.identical?'identical':model.hunks.length+' change block'+(model.hunks.length===1?'':'s')+' · '+model.stats.important+' important · '+model.stats.unimportant+' unimportant';
  sideButton.classList.toggle('active',viewMode==='side');inlineButton.classList.toggle('active',viewMode==='inline');columns.style.display=viewMode==='side'?'grid':'none';syncCompareFilterButtons();
  syncCompareSaveState('left');syncCompareSaveState('right');
  if(model.identical){const empty=document.createElement('div');empty.className='file-compare-empty';empty.textContent='No differences';body.append(empty);return;}
  const indexes=compareVisibleIndexes(model);
  if(!indexes.length){const empty=document.createElement('div');empty.className='file-compare-empty';empty.textContent='No differences match the current filters';body.append(empty);return;}
  if(viewMode==='inline')renderInline(model,indexes);else renderSide(model,indexes);
}
async function copyHunk(hunk,direction){
  if(!current)return;
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
  if(!current)return;
  if(compareHasSaving()){window.alert('A compare file is still being saved. Finish that operation before reloading.');return false;}
  if(compareHasDirty()&&!window.confirm('Discard unsaved compare edits and reload both files?'))return false;
  await Promise.all([loadSource(current.left),loadSource(current.right)]);
  current.syntax=null;
  if(editMode)ensureCompareEditors();
  render();return true;
}
async function open(options){
  if(!options?.left?.load||!options?.right?.load)throw new Error('File compare requires left and right sources');
  if(compareHasSaving()){window.alert('A compare file is still being saved. Finish that operation before opening another comparison.');return false;}
  // Prepare both files first: a failed remote read must not destroy the current comparison.
  const next={title:options.title||'File Compare',left:{...options.left,meta:{...(options.left.meta||{})}},right:{...options.right,meta:{...(options.right.meta||{})}}};
  await Promise.all([loadSource(next.left),loadSource(next.right)]);
  if(compareHasSaving()){window.alert('A compare file started saving while sources were loading. Try again after the save finishes.');return false;}
  if(compareHasDirty()&&!window.confirm('Discard unsaved changes in the current File Compare?'))return false;
  destroyCompareEditors();editMode=false;editorDeck.classList.add('hidden');editButton.classList.remove('active');
  current=next;backdrop.classList.add('visible');render();return true;
}
function close({force=false}={}){
  if(!force&&compareHasSaving()){window.alert('A compare file is still being saved. Finish that operation before closing.');return false;}
  if(!force&&compareHasDirty()&&!window.confirm('Close File Compare and discard unsaved changes?'))return false;
  destroyCompareEditors();backdrop.classList.remove('visible');current=null;body.replaceChildren();return true;
}
function projectSource(pathValue,{writable=true,label=''}={}){
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
    const params=new URLSearchParams({view:'file-content',repo:repoID,path:pathValue,state});
    const data=await app.jsonFetch('/api/git/status?'+params.toString());
    return {text:data.content,commit:data.commit,state:data.state,path:data.path,repo_id:data.repo_id};
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
  if(mode==='staged'){
    return open({title:'HEAD ↔ Staged',left:gitStateSource(repoID,repoPath,'head'),right:gitStateSource(repoID,repoPath,'index')});
  }
  if(mode==='worktree'){
    return open({title:'Staged ↔ Working',left:gitStateSource(repoID,repoPath,'index'),right:workingProjectSource(workspacePath,{label:'WORKTREE · '+workspacePath})});
  }
  if(mode==='head-worktree'){
    return open({title:'HEAD ↔ Working',left:gitStateSource(repoID,repoPath,'head'),right:workingProjectSource(workspacePath,{label:'WORKTREE · '+workspacePath})});
  }
  throw new Error('Unsupported Git compare mode '+mode);
}
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
reloadButton.onclick=()=>reload().catch(app.showError);closeButton.onclick=close;
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();});

globalThis.TaskMenuFileCompare={open,close,reload,buildCompareModel,projectSource,editorSource,savedEditorSource,clipboardSource,remoteSource,browserFileHandleSource,openLeftRemote,gitCommitSource,emptyCompareSource,openGitCommitFileDiff,openGitCommitFileDiffBetween,gitStateSource,workingProjectSource,openProjectFiles,openEditorSaved,openEditorClipboard,openGitCommitAgainstProject,openGitCommits,openGitStatePair,promptProjectCompare,selectForCompare,clearCompareSelection,compareWithSelected,openSources,canCompareWithSelected,sourceIdentity,sourceWritable,get selection(){return selectionSnapshot();},get current(){return current;}};
