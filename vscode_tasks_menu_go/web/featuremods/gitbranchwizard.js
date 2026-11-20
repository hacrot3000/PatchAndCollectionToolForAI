const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Git branch choice wizard');

const style=document.createElement('style');
style.textContent=`
.git-branch-choice-backdrop{position:fixed;inset:0;z-index:18500;display:none;align-items:center;justify-content:center;padding:16px;background:rgba(0,0,0,.63)}
.git-branch-choice-backdrop.visible{display:flex}
.git-branch-choice-dialog{width:min(650px,100%);max-height:calc(100vh - 32px);display:flex;flex-direction:column;background:#11161d;color:#e6edf3;border:1px solid #4a5362;border-radius:12px;box-shadow:0 22px 65px rgba(0,0,0,.55);overflow:hidden}
.git-branch-choice-head{display:flex;align-items:start;gap:12px;padding:15px 16px 12px;border-bottom:1px solid #30343b}
.git-branch-choice-heading{flex:1;min-width:0;font-size:15px;font-weight:700}
.git-branch-choice-close{padding:3px 9px}
.git-branch-choice-body{min-height:0;overflow-y:auto;padding:13px 16px}
.git-branch-choice-summary{white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px;line-height:1.55;margin:0 0 13px}
.git-branch-choice-options{display:grid;gap:9px}
.git-branch-choice-option{display:flex;gap:11px;align-items:start;border:1px solid #424b59;border-radius:8px;background:#151b23;padding:11px 12px;cursor:pointer}
.git-branch-choice-option.selected{border-color:#7aa7e3;background:#1c2c40;box-shadow:0 0 0 1px #7aa7e3 inset}
.git-branch-choice-option input{margin:3px 0 0;flex:none;accent-color:#7aa7e3}
.git-branch-choice-copy{display:grid;min-width:0;gap:5px}
.git-branch-choice-name{font-size:13px;font-weight:700}
.git-branch-choice-ref{font:12px/1.4 ui-monospace,monospace;overflow-wrap:anywhere}
.git-branch-choice-desc{font-size:11px;line-height:1.5;opacity:.75}
.git-branch-choice-foot{display:flex;align-items:center;justify-content:flex-end;gap:8px;padding:11px 16px;border-top:1px solid #30343b}
.git-branch-choice-foot button{padding:7px 12px}
.git-branch-choice-confirm{background:#28517e;color:#fff;border:1px solid #628abb;border-radius:6px}
html[data-taskmenu-theme="light"] .git-branch-choice-dialog{background:#fff;color:#202124;border-color:#c2c8d0}
html[data-taskmenu-theme="light"] .git-branch-choice-option{background:#f8fafc;border-color:#d0d7de}
html[data-taskmenu-theme="light"] .git-branch-choice-option.selected{background:#eaf3ff;border-color:#5486c9;box-shadow:0 0 0 1px #5486c9 inset}
@media(max-width:520px){.git-branch-choice-head,.git-branch-choice-body,.git-branch-choice-foot{padding-left:10px;padding-right:10px}}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='git-branch-choice-backdrop';
const dialog=document.createElement('div');dialog.className='git-branch-choice-dialog';
dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label','Git branch choice');
const header=document.createElement('div');header.className='git-branch-choice-head';
const heading=document.createElement('div');heading.className='git-branch-choice-heading';
heading.id='taskdeck-git-branch-choice-title';
dialog.setAttribute('aria-labelledby',heading.id);
const dismiss=document.createElement('button');dismiss.type='button';dismiss.textContent='×';dismiss.className='git-branch-choice-close';dismiss.title='Cancel';
header.append(heading,dismiss);
const body=document.createElement('div');body.className='git-branch-choice-body';
const summary=document.createElement('p');summary.className='git-branch-choice-summary';
const optionsNode=document.createElement('div');optionsNode.className='git-branch-choice-options';
body.append(summary,optionsNode);
const footer=document.createElement('div');footer.className='git-branch-choice-foot';
const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
const confirm=document.createElement('button');confirm.type='button';confirm.className='git-branch-choice-confirm';confirm.textContent='Continue';
footer.append(cancel,confirm);dialog.append(header,body,footer);backdrop.append(dialog);document.body.append(backdrop);

let session=null;
let serial=0;
function close(value=null){
  if(!session)return;
  const old=session;session=null;
  backdrop.classList.remove('visible');optionsNode.replaceChildren();
  document.removeEventListener('keydown',onKeydown,true);
  old.resolve(value);
  if(old.previousFocus?.isConnected)old.previousFocus.focus();
}
function onKeydown(event){
  if(!session)return;
  if(event.key==='Escape'){event.preventDefault();event.stopPropagation();close(null);return;}
  if(event.key!=='Tab')return;
  const focusable=Array.from(dialog.querySelectorAll('button:not(:disabled),input:not(:disabled)')).filter(el=>el.offsetParent!==null);
  if(!focusable.length)return;
  const first=focusable[0],last=focusable[focusable.length-1];
  if(event.shiftKey&&document.activeElement===first){event.preventDefault();last.focus();}
  else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first.focus();}
}
function choose(id){
  if(!session)return;
  session.selected=id;
  for(const label of optionsNode.querySelectorAll('.git-branch-choice-option')){
    const selected=label.dataset.option===id;
    label.classList.toggle('selected',selected);
    label.querySelector('input').checked=selected;
  }
  confirm.disabled=!id;
}
function open({title,description,options,defaultChoice,confirmLabel}={}){
  if(session)close(null);
  const choices=Array.isArray(options)?options.filter(o=>o?.id&&o?.label):[];
  if(!choices.length)return Promise.resolve(null);
  return new Promise(resolve=>{
    const current={resolve,previousFocus:document.activeElement,selected:null,id:++serial};
    session=current;
    heading.textContent=String(title||'Choose Git branch action');
    summary.textContent=String(description||'');
    summary.hidden=!description;
    confirm.textContent=String(confirmLabel||'Continue');
    optionsNode.replaceChildren();
    const radioName='git-branch-choice-'+current.id;
    for(const choice of choices){
      const label=document.createElement('label');label.className='git-branch-choice-option';label.dataset.option=choice.id;
      const radio=document.createElement('input');radio.type='radio';radio.name=radioName;radio.value=choice.id;
      const copy=document.createElement('span');copy.className='git-branch-choice-copy';
      const name=document.createElement('span');name.className='git-branch-choice-name';name.textContent=choice.label;copy.append(name);
      if(choice.ref){const ref=document.createElement('span');ref.className='git-branch-choice-ref';ref.textContent=choice.ref;copy.append(ref);}
      if(choice.description){const desc=document.createElement('span');desc.className='git-branch-choice-desc';desc.textContent=choice.description;copy.append(desc);}
      radio.onchange=()=>{if(radio.checked)choose(choice.id);};
      label.append(radio,copy);optionsNode.append(label);
    }
    backdrop.classList.add('visible');
    choose(choices.some(item=>item.id===defaultChoice)?defaultChoice:choices[0].id);
    document.addEventListener('keydown',onKeydown,true);
    optionsNode.querySelector('.git-branch-choice-option.selected input')?.focus();
  });
}
dismiss.onclick=()=>close(null);
cancel.onclick=()=>close(null);
confirm.onclick=()=>{if(session?.selected)close(session.selected);};
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close(null);});
globalThis.TaskDeckGitBranchChoiceWizard={open,close};
