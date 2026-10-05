const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Git semantics wizard');

const style=document.createElement('style');
style.textContent=`
.git-semantics-backdrop{display:none;position:fixed;inset:0;z-index:2860;background:rgba(0,0,0,.56);padding:18px}
.git-semantics-backdrop.visible{display:flex}
.git-semantics-dialog{width:min(980px,calc(100vw - 36px));max-height:calc(100vh - 36px);margin:auto;display:flex;flex-direction:column;background:#10151c;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 58px rgba(0,0,0,.58);overflow:hidden}
.git-semantics-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #303843}.git-semantics-title{font-weight:700;flex:1}.git-semantics-body{padding:10px;overflow:auto}
.git-semantics-summary{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:6px;margin-bottom:8px}.git-semantics-summary div{border:1px solid #303843;border-radius:7px;padding:6px}.git-semantics-summary strong{display:block;font-size:10px}.git-semantics-summary span{font:10px ui-monospace,monospace;opacity:.68}
.git-semantics-options{display:grid;gap:6px}.git-semantics-option{display:grid;grid-template-columns:24px minmax(120px,.7fr) minmax(0,2fr);gap:6px;align-items:start;border:1px solid #303843;border-radius:8px;padding:7px;cursor:pointer}.git-semantics-option.selected{border-color:#6f8db3;background:#1d2835}.git-semantics-option input{margin-top:3px}.git-semantics-name{font-weight:700;font-size:11px}.git-semantics-matrix{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:4px;margin-top:4px}.git-semantics-cell{font-size:9px;border-left:2px solid #46505d;padding-left:5px}.git-semantics-cell strong{display:block}.git-semantics-warning{margin-top:5px;font-size:9px;color:#ffb4a6}
.git-semantics-files{margin-top:10px;border:1px solid #303843;border-radius:8px;overflow:hidden}.git-semantics-file{display:grid;grid-template-columns:36px minmax(0,1fr) auto;gap:5px;align-items:center;padding:5px 7px;border-top:1px solid #252c35}.git-semantics-file:first-child{border-top:0}.git-semantics-file span{font:10px ui-monospace,monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.git-semantics-footer{display:flex;align-items:center;gap:7px;padding:8px 10px;border-top:1px solid #303843}.git-semantics-status{flex:1;font-size:10px;opacity:.68}.git-semantics-footer button,.git-semantics-head button,.git-semantics-file button{padding:5px 8px;font-size:10px}
html[data-taskmenu-theme="light"] .git-semantics-dialog{background:#fff;color:#202124;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .git-semantics-summary div,html[data-taskmenu-theme="light"] .git-semantics-option,html[data-taskmenu-theme="light"] .git-semantics-files{border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-semantics-option.selected{background:#eef5ff;border-color:#6f8db3}
@media(max-width:760px){.git-semantics-summary{grid-template-columns:repeat(2,minmax(0,1fr))}.git-semantics-option{grid-template-columns:24px 1fr}.git-semantics-option>div:last-child{grid-column:2}.git-semantics-matrix{grid-template-columns:1fr}}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='git-semantics-backdrop';
const dialog=document.createElement('div');dialog.className='git-semantics-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Git state semantics wizard');
const head=document.createElement('div');head.className='git-semantics-head';
const title=document.createElement('div');title.className='git-semantics-title';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';head.append(title,closeButton);
const body=document.createElement('div');body.className='git-semantics-body';
const footer=document.createElement('div');footer.className='git-semantics-footer';
const status=document.createElement('span');status.className='git-semantics-status';
const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
const confirm=document.createElement('button');confirm.type='button';confirm.textContent='Confirm';
footer.append(status,cancel,confirm);dialog.append(head,body,footer);backdrop.append(dialog);document.body.append(backdrop);

let session=null;

const commitOperations=[
  {
    id:'soft',label:'Reset soft',
    head:'Move HEAD to target',index:'Keep Index unchanged',worktree:'Keep Working tree unchanged',
    note:'Previously committed changes become staged changes.'
  },
  {
    id:'mixed',label:'Reset mixed',
    head:'Move HEAD to target',index:'Reset Index to target',worktree:'Keep Working tree unchanged',
    note:'Previously committed/staged differences become unstaged changes.'
  },
  {
    id:'hard',label:'Reset hard',
    head:'Move HEAD to target',index:'Reset Index to target',worktree:'Reset tracked files to target',
    note:'Destructive: tracked staged and working-tree changes are discarded. Untracked files are not removed.'
  },
  {
    id:'revert',label:'Revert commit',
    head:'Create a new revert commit',index:'Expected clean after success',worktree:'Expected clean after success',
    note:'History is preserved. Conflict resolution may be required.'
  }
];
const fileOperations=[
  {
    id:'worktree',label:'Restore file from commit',
    head:'HEAD unchanged',index:'Index unchanged',worktree:'Replace this Working tree path from target',
    note:'Local unstaged content in this file will be overwritten.'
  },
  {
    id:'staged',label:'Restore staged only',
    head:'HEAD unchanged',index:'Replace this Index path from target',worktree:'Working tree unchanged',
    note:'Only what would be committed for this path changes.'
  }
];

function summaryBox(label,value){
  const root=document.createElement('div');root.append(Object.assign(document.createElement('strong'),{textContent:label}),Object.assign(document.createElement('span'),{textContent:String(value??'')}));return root;
}
function renderSummary(preview){
  const root=document.createElement('div');root.className='git-semantics-summary';
  root.append(
    summaryBox('Branch',preview.detached?'(detached)':(preview.branch||'(unknown)')),
    summaryBox('HEAD',String(preview.head_sha||'').slice(0,12)),
    summaryBox('Target',String(preview.target_sha||'').slice(0,12)),
    summaryBox('Local state','staged '+(preview.staged||0)+' · unstaged '+(preview.unstaged||0)+' · untracked '+(preview.untracked||0)+' · conflict '+(preview.conflicted||0))
  );
  return root;
}
function renderMatrix(option){
  const matrix=document.createElement('div');matrix.className='git-semantics-matrix';
  for(const [label,value] of [['HEAD',option.head],['Index',option.index],['Working tree',option.worktree]]){
    const cell=document.createElement('div');cell.className='git-semantics-cell';
    cell.append(Object.assign(document.createElement('strong'),{textContent:label}),document.createTextNode(value));matrix.append(cell);
  }
  return matrix;
}
function selectOperation(id){
  if(!session)return;
  session.selected=id;
  body.querySelectorAll('.git-semantics-option').forEach(node=>node.classList.toggle('selected',node.dataset.id===id));
  const input=body.querySelector('input[name="git-semantics-op"][value="'+CSS.escape(id)+'"]');if(input)input.checked=true;
  const selected=session.operations.find(item=>item.id===id);
  confirm.textContent=selected?.label||'Confirm';
}
function renderOptions(operations){
  const root=document.createElement('div');root.className='git-semantics-options';
  for(const option of operations){
    const row=document.createElement('label');row.className='git-semantics-option';row.dataset.id=option.id;
    const radio=document.createElement('input');radio.type='radio';radio.name='git-semantics-op';radio.value=option.id;
    const name=document.createElement('div');name.className='git-semantics-name';name.textContent=option.label;
    const detail=document.createElement('div');detail.append(renderMatrix(option));
    if(option.note){const note=document.createElement('div');note.className='git-semantics-warning';note.textContent=option.note;detail.append(note);}
    row.append(radio,name,detail);row.onclick=()=>selectOperation(option.id);root.append(row);
  }
  return root;
}
function renderFiles(preview,onCompareFile){
  const files=Array.isArray(preview.changed_files)?preview.changed_files:[];
  if(!files.length)return null;
  const root=document.createElement('div');root.className='git-semantics-files';
  for(const file of files.slice(0,100)){
    const row=document.createElement('div');row.className='git-semantics-file';
    const code=document.createElement('span');code.textContent=String(file.status||'');
    const path=document.createElement('span');path.textContent=file.old_path?(file.old_path+' → '+file.path):String(file.path||'');path.title=path.textContent;
    const compare=document.createElement('button');compare.type='button';compare.textContent='Visual diff';compare.onclick=event=>{event.preventDefault();event.stopPropagation();Promise.resolve(onCompareFile?.(file)).catch(app.showError);};
    row.append(code,path,compare);root.append(row);
  }
  return root;
}
function close(result=false){
  if(!session)return;
  const resolve=session.resolve;session=null;backdrop.classList.remove('visible');body.replaceChildren();status.textContent='';resolve?.(result);
}
function open(options={}){
  if(session)close(false);
  return new Promise(resolve=>{
    const preview=options.preview||{},kind=options.kind==='file'?'file':'commit';
    const operations=kind==='file'?fileOperations:commitOperations;
    session={resolve,preview,kind,operations,selected:options.preferred||operations[0].id,onConfirm:options.onConfirm};
    title.textContent=kind==='file'?'Git restore semantics · '+String(options.path||''):'Git reset / revert semantics';
    body.replaceChildren(renderSummary(preview),renderOptions(operations));
    if(kind==='file'){
      const pathState=document.createElement('div');pathState.className='git-semantics-warning';
      pathState.textContent='Selected path · staged '+Boolean(preview.path_staged)+' · unstaged '+Boolean(preview.path_unstaged)+' · untracked '+Boolean(preview.path_untracked)+' · conflicted '+Boolean(preview.path_conflicted)+' · target exists '+Boolean(preview.target_path_exists);
      body.append(pathState);
      if(options.onCompareFile){
        const compare=document.createElement('button');compare.type='button';compare.textContent='Visual compare target ↔ current file';compare.onclick=()=>Promise.resolve(options.onCompareFile()).catch(app.showError);body.append(compare);
      }
    }else{
      const files=renderFiles(preview,options.onCompareFile);if(files)body.append(files);
    }
    status.textContent='Snapshot guard · HEAD '+String(preview.head_sha||'').slice(0,12)+' · target '+String(preview.target_sha||'').slice(0,12);
    backdrop.classList.add('visible');selectOperation(session.selected);
  });
}
confirm.onclick=async()=>{
  if(!session)return;
  const current=session,selected=current.selected;
  confirm.disabled=true;
  try{
    const result=await current.onConfirm?.(selected,current.preview);
    if(session===current)close(result||true);
  }catch(error){app.showError(error);}finally{confirm.disabled=false;}
};
cancel.onclick=()=>close(false);closeButton.onclick=()=>close(false);
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close(false);});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))close(false);});

globalThis.TaskDeckGitSemanticsWizard={open,close};
