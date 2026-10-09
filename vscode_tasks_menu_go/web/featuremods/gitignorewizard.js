const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Git ignore wizard');

const style=document.createElement('style');
style.textContent=`
.git-ignore-backdrop{position:fixed;inset:0;z-index:18100;display:none;align-items:center;justify-content:center;background:rgba(0,0,0,.62);padding:18px}
.git-ignore-backdrop.visible{display:flex}
.git-ignore-dialog{width:min(780px,96vw);max-height:90vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #4a5362;border-radius:11px;box-shadow:0 22px 70px rgba(0,0,0,.6);overflow:hidden}
.git-ignore-head{display:flex;align-items:flex-start;gap:10px;padding:13px 14px;border-bottom:1px solid #30343b}
.git-ignore-head-main{flex:1;min-width:0}.git-ignore-title{font-size:15px;font-weight:700}.git-ignore-subtitle{margin-top:4px;font:11px ui-monospace,monospace;opacity:.68;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.git-ignore-close{padding:4px 8px}
.git-ignore-body{padding:12px 14px;overflow:auto}
.git-ignore-intro{font-size:12px;line-height:1.5;margin-bottom:10px}
.git-ignore-editor{display:grid;gap:5px;margin:8px 0 12px;padding:9px 10px;border:1px solid #30343b;border-radius:8px;background:#10151c}
.git-ignore-editor-title{font-size:10px;font-weight:800;letter-spacing:.04em;opacity:.65}
.git-ignore-editor input{width:100%;box-sizing:border-box;font:12px ui-monospace,monospace;background:#090d12;color:inherit;border:1px solid #3a424e;border-radius:5px;padding:7px 8px}
.git-ignore-editor input:focus{outline:none;border-color:#6f91bb;box-shadow:0 0 0 1px rgba(111,145,187,.25)}
.git-ignore-editor-meta{min-height:16px;font:10px/1.4 ui-monospace,monospace;opacity:.7;white-space:pre-wrap}
.git-ignore-group{margin-top:12px}.git-ignore-group-title{font-size:10px;font-weight:800;letter-spacing:.04em;opacity:.6;margin-bottom:6px}
.git-ignore-options{display:grid;gap:8px}
.git-ignore-option{display:grid;grid-template-columns:auto minmax(0,1fr);gap:9px;border:1px solid #30343b;border-radius:8px;background:#151b23;padding:9px 10px;cursor:pointer}
.git-ignore-option:hover{border-color:#52719a}.git-ignore-option.selected{border-color:#6f91bb;box-shadow:0 0 0 1px rgba(111,145,187,.28) inset;background:#182332}
.git-ignore-option input{margin-top:3px}
.git-ignore-option-main{min-width:0}.git-ignore-option-head{display:flex;align-items:center;gap:6px;flex-wrap:wrap}
.git-ignore-option-label{font-size:12px;font-weight:700}.git-ignore-badge{font-size:9px;font-weight:800;border:1px solid #4b596b;border-radius:999px;padding:1px 5px;opacity:.85}
.git-ignore-badge.recommended{border-color:#4f8d61;color:#8ed29f}.git-ignore-badge.broad{border-color:#9b7540;color:#e8bf79}
.git-ignore-pattern{margin-top:5px;font:11px ui-monospace,monospace;background:#090d12;border:1px solid #30343b;border-radius:5px;padding:5px 7px;word-break:break-all}
.git-ignore-desc{margin-top:5px;font-size:11px;line-height:1.4;opacity:.72}
.git-ignore-impact{margin-top:5px;font-size:10px;font-weight:700}.git-ignore-samples{margin-top:4px;padding-left:17px;font:10px/1.4 ui-monospace,monospace;opacity:.65}
.git-ignore-samples li{word-break:break-all}
.git-ignore-result{display:none;margin-top:10px;padding:8px 9px;border:1px solid #365f84;border-radius:7px;background:#0d1721;white-space:pre-wrap;font:11px/1.4 ui-monospace,monospace}.git-ignore-result.visible{display:block}
.git-ignore-foot{display:flex;align-items:center;justify-content:space-between;gap:8px;padding:10px 14px;border-top:1px solid #30343b}
.git-ignore-foot-left,.git-ignore-foot-right{display:flex;align-items:center;gap:7px}.git-ignore-foot button{padding:5px 8px}
.git-ignore-apply{font-weight:700}
html[data-taskmenu-theme="light"] .git-ignore-dialog{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .git-ignore-option{background:#f8fafc;border-color:#d0d7de}
html[data-taskmenu-theme="light"] .git-ignore-option.selected{background:#eef6ff;border-color:#7aa7d7}
html[data-taskmenu-theme="light"] .git-ignore-pattern,html[data-taskmenu-theme="light"] .git-ignore-editor input{background:#f6f8fa;border-color:#d0d7de;color:#202124}
html[data-taskmenu-theme="light"] .git-ignore-editor{background:#f8fafc;border-color:#d0d7de}
html[data-taskmenu-theme="light"] .git-ignore-result{background:#eef6ff;border-color:#9ebfe0}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='git-ignore-backdrop';
const dialog=document.createElement('div');dialog.className='git-ignore-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label','Git ignore wizard');
const head=document.createElement('div');head.className='git-ignore-head';
const headMain=document.createElement('div');headMain.className='git-ignore-head-main';
const title=document.createElement('div');title.className='git-ignore-title';title.textContent='Ignore file / similar files';
const subtitle=document.createElement('div');subtitle.className='git-ignore-subtitle';
const close=document.createElement('button');close.className='git-ignore-close';close.textContent='×';close.title='Close';
headMain.append(title,subtitle);head.append(headMain,close);
const body=document.createElement('div');body.className='git-ignore-body';
const intro=document.createElement('div');intro.className='git-ignore-intro';
const editor=document.createElement('div');editor.className='git-ignore-editor';
const editorTitle=document.createElement('div');editorTitle.className='git-ignore-editor-title';editorTitle.textContent='GIT-IGNORE PATTERN (EDITABLE)';
const patternInput=document.createElement('input');patternInput.type='text';patternInput.spellcheck=false;patternInput.autocomplete='off';patternInput.placeholder='Select a suggestion, then customize the pattern if needed';
const editorMeta=document.createElement('div');editorMeta.className='git-ignore-editor-meta';
editor.append(editorTitle,patternInput,editorMeta);
const groups=document.createElement('div');
const result=document.createElement('div');result.className='git-ignore-result';
body.append(intro,editor,groups,result);
const foot=document.createElement('div');foot.className='git-ignore-foot';
const footLeft=document.createElement('div');footLeft.className='git-ignore-foot-left';
const footRight=document.createElement('div');footRight.className='git-ignore-foot-right';
const copyPattern=document.createElement('button');copyPattern.textContent='Copy pattern';
const cancel=document.createElement('button');cancel.textContent='Cancel';
const apply=document.createElement('button');apply.className='git-ignore-apply';apply.textContent='Add to .gitignore';
footLeft.append(copyPattern);footRight.append(cancel,apply);foot.append(footLeft,footRight);
dialog.append(head,body,foot);backdrop.append(dialog);document.body.append(backdrop);

let current=null;
let selectedID='';
let previewTimer=0;
let previewSequence=0;
let previewItem=null;
const editedPatterns=new Map();

function closeWizard(){
  backdrop.classList.remove('visible');
  if(previewTimer)clearTimeout(previewTimer);
  previewTimer=0;previewSequence++;
  current=null;selectedID='';previewItem=null;editedPatterns.clear();patternInput.value='';patternInput.disabled=true;editorMeta.textContent='';groups.replaceChildren();result.classList.remove('visible');result.textContent='';
}
close.onclick=closeWizard;cancel.onclick=closeWizard;
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)closeWizard();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))closeWizard();});

async function copyText(value){
  value=String(value??'');
  if(navigator.clipboard&&window.isSecureContext){
    try{await navigator.clipboard.writeText(value);return;}catch{}
  }
  const area=document.createElement('textarea');area.value=value;area.setAttribute('readonly','');area.style.position='fixed';area.style.left='-9999px';document.body.append(area);area.select();
  let copied=false;try{copied=document.execCommand('copy');}finally{area.remove();}
  if(!copied)throw new Error('Cannot copy ignore pattern');
}

function groupName(kind){
  if(kind==='exact'||kind==='directory')return 'SELECTED FILE / FOLDER';
  if(kind==='common')return 'COMMON IGNORE RULES';
  return 'SIMILAR FILES';
}
function optionByID(){return (current?.suggestions||[]).find(item=>item.id===selectedID)||null;}
function currentPattern(){return String(patternInput.value||'').trim();}

function renderPatternPreview(item){
  previewItem=item||null;
  if(!item){editorMeta.textContent='';return;}
  const count=Number(item.match_count||0);
  const samples=Array.isArray(item.samples)?item.samples:[];
  const custom=currentPattern()!==String(optionByID()?.pattern||'');
  const bits=[count+' untracked '+(count===1?'path':'paths')+' currently match'+(custom?' · customized pattern':'')];
  if(samples.length)bits.push(samples.slice(0,4).join('\n'));
  if(count>samples.length)bits.push('… and '+(count-samples.length)+' more');
  editorMeta.textContent=bits.join('\n');
}

async function previewPatternNow(){
  const item=optionByID();
  if(!item||!current){previewItem=null;apply.disabled=true;return null;}
  const pattern=currentPattern();
  if(!pattern){previewItem=null;editorMeta.textContent='Pattern cannot be empty.';apply.disabled=true;return null;}
  if(pattern===String(item.pattern||'')){
    renderPatternPreview(item);apply.disabled=false;return item;
  }
  if(typeof current.previewPattern!=='function'){
    previewItem=null;editorMeta.textContent='Pattern preview is unavailable.';apply.disabled=true;return null;
  }
  const seq=++previewSequence;
  editorMeta.textContent='Checking customized pattern…';
  apply.disabled=true;
  try{
    const data=await current.previewPattern(pattern);
    if(seq!==previewSequence||!current)return null;
    const checked=data?.suggestion||null;
    if(!checked)throw new Error('No preview returned');
    renderPatternPreview(checked);apply.disabled=false;return checked;
  }catch(error){
    if(seq!==previewSequence||!current)return null;
    previewItem=null;editorMeta.textContent='Invalid pattern: '+(error?.message||String(error));apply.disabled=true;return null;
  }
}

function schedulePatternPreview(){
  if(previewTimer)clearTimeout(previewTimer);
  previewTimer=setTimeout(()=>{previewTimer=0;previewPatternNow();},220);
}

function selectOption(id){
  if(selectedID)editedPatterns.set(selectedID,patternInput.value);
  selectedID=id;
  for(const card of groups.querySelectorAll('.git-ignore-option')){
    const selected=card.dataset.id===id;
    card.classList.toggle('selected',selected);
    const radio=card.querySelector('input[type="radio"]');if(radio)radio.checked=selected;
  }
  const item=optionByID();
  patternInput.disabled=!item;
  patternInput.value=item?(editedPatterns.has(id)?editedPatterns.get(id):String(item.pattern||'')):'';
  copyPattern.disabled=!item;
  previewItem=null;
  if(item)previewPatternNow();else{editorMeta.textContent='';apply.disabled=true;}
}

function renderOption(item){
  const card=document.createElement('label');card.className='git-ignore-option';card.dataset.id=item.id;
  const radio=document.createElement('input');radio.type='radio';radio.name='git-ignore-choice';radio.value=item.id;
  const main=document.createElement('div');main.className='git-ignore-option-main';
  const optionHead=document.createElement('div');optionHead.className='git-ignore-option-head';
  const label=document.createElement('span');label.className='git-ignore-option-label';label.textContent=item.label||item.pattern;
  optionHead.append(label);
  if(item.recommended){const badge=document.createElement('span');badge.className='git-ignore-badge recommended';badge.textContent='RECOMMENDED';optionHead.append(badge);}
  if(item.broad){const badge=document.createElement('span');badge.className='git-ignore-badge broad';badge.textContent='BROAD';optionHead.append(badge);}
  const pattern=document.createElement('div');pattern.className='git-ignore-pattern';pattern.textContent=item.pattern;
  const desc=document.createElement('div');desc.className='git-ignore-desc';desc.textContent=item.description||'';
  const impact=document.createElement('div');impact.className='git-ignore-impact';
  impact.textContent=(item.match_count||0)+' untracked '+((item.match_count||0)===1?'path':'paths')+' currently match this rule';
  main.append(optionHead,pattern,desc,impact);
  const samples=Array.isArray(item.samples)?item.samples:[];
  if(samples.length){
    const list=document.createElement('ul');list.className='git-ignore-samples';
    for(const sample of samples){const li=document.createElement('li');li.textContent=sample;list.append(li);}
    if((item.match_count||0)>samples.length){const li=document.createElement('li');li.textContent='… and '+((item.match_count||0)-samples.length)+' more';list.append(li);}
    main.append(list);
  }
  card.append(radio,main);
  card.addEventListener('click',()=>selectOption(item.id));
  radio.addEventListener('change',()=>selectOption(item.id));
  return card;
}

function renderSuggestions(rows){
  groups.replaceChildren();
  const order=['SELECTED FILE / FOLDER','SIMILAR FILES','COMMON IGNORE RULES'];
  const buckets=new Map(order.map(name=>[name,[]]));
  for(const item of rows){
    const name=groupName(item.kind);
    if(!buckets.has(name))buckets.set(name,[]);
    buckets.get(name).push(item);
  }
  for(const name of order){
    const items=buckets.get(name)||[];if(!items.length)continue;
    const section=document.createElement('section');section.className='git-ignore-group';
    const heading=document.createElement('div');heading.className='git-ignore-group-title';heading.textContent=name;
    const host=document.createElement('div');host.className='git-ignore-options';
    for(const item of items)host.append(renderOption(item));
    section.append(heading,host);groups.append(section);
  }
  const preferred=rows.find(item=>item.recommended&&!item.broad)||rows.find(item=>item.recommended)||rows[0];
  selectOption(preferred?.id||'');
}

async function open(ctx){
  current={...ctx,suggestions:[]};
  subtitle.textContent=[ctx.repository?.name||ctx.repository?.id||'Git repository',ctx.path].filter(Boolean).join(' · ');
  intro.textContent='Choose a smart ignore suggestion, then edit the git-ignore pattern if needed. TaskDeck previews how many untracked paths the current pattern matches before it can be appended to the repository root .gitignore.';
  groups.replaceChildren();
  result.classList.add('visible');result.textContent='Analyzing untracked files and candidate patterns…';
  patternInput.disabled=true;patternInput.value='';editorMeta.textContent='';copyPattern.disabled=true;apply.disabled=true;
  backdrop.classList.add('visible');
  close.focus();
  try{
    const data=await ctx.loadSuggestions();
    if(!current)return;
    const rows=Array.isArray(data?.suggestions)?data.suggestions:[];
    current.suggestions=rows;
    result.classList.remove('visible');result.textContent='';
    if(!rows.length){
      result.classList.add('visible');result.textContent='No safe ignore suggestion is available for this path.';
      return;
    }
    renderSuggestions(rows);
  }catch(error){
    result.classList.add('visible');result.textContent='Could not analyze ignore rules: '+(error?.message||String(error));
  }
}

patternInput.addEventListener('input',()=>{
  if(selectedID)editedPatterns.set(selectedID,patternInput.value);
  previewSequence++;
  previewItem=null;apply.disabled=true;
  schedulePatternPreview();
});

copyPattern.onclick=async()=>{
  const item=optionByID();if(!item)return;
  try{await copyText(currentPattern());const old=copyPattern.textContent;copyPattern.textContent='✓ Copied';setTimeout(()=>{if(copyPattern.isConnected)copyPattern.textContent=old;},1000);}
  catch(error){app.showError(error);}
};

apply.onclick=async()=>{
  const item=optionByID();if(!item||!current)return;
  if(previewTimer){clearTimeout(previewTimer);previewTimer=0;}
  const checked=await previewPatternNow();
  if(!checked)return;
  const pattern=currentPattern();
  if(checked.broad||Number(checked.match_count||0)>1){
    const ok=window.confirm('This ignore rule matches multiple untracked paths:\n\n'+pattern+'\n\nIt currently matches '+checked.match_count+' untracked paths. Add it to .gitignore?');
    if(!ok)return;
  }
  apply.disabled=true;apply.textContent='Applying…';
  result.classList.add('visible');result.textContent='Adding '+pattern+' to .gitignore…';
  try{
    const response=await current.apply(item,pattern);
    result.textContent=response?.output||'Ignore rule added.';
    if(current.refresh)await current.refresh();
    setTimeout(closeWizard,700);
  }catch(error){
    result.textContent='Failed: '+(error?.message||String(error));
  }finally{
    if(apply.isConnected){apply.disabled=false;apply.textContent='Add to .gitignore';}
  }
};

globalThis.TaskDeckGitIgnoreWizard={open};
