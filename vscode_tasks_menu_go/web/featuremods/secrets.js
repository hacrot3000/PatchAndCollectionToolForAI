const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Secrets Manager');

const style=document.createElement('style');
style.textContent=`
.secret-manager-backdrop{display:none;position:fixed;inset:0;z-index:17780;background:rgba(0,0,0,.52);align-items:flex-start;justify-content:center;padding:5vh 16px}
.secret-manager-backdrop.visible{display:flex}
.secret-manager-dialog{width:min(980px,96vw);max-height:88vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 60px rgba(0,0,0,.58);overflow:hidden}
.secret-manager-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #303843}.secret-manager-head strong{flex:1}
.secret-manager-note{padding:8px 11px;font-size:10px;opacity:.68;border-bottom:1px solid #303843}
.secret-manager-create{display:grid;grid-template-columns:150px minmax(220px,1fr) auto;gap:7px;padding:9px 11px;border-bottom:1px solid #303843}.secret-manager-create input,.secret-manager-create select{min-width:0}
.secret-manager-list{overflow:auto;min-height:180px;max-height:60vh}.secret-manager-row{display:grid;grid-template-columns:105px minmax(260px,1fr) minmax(180px,.8fr) auto;gap:8px;align-items:center;padding:8px 10px;border-bottom:1px solid #252c35;font-size:11px}
.secret-manager-kind{font-weight:700;text-transform:uppercase;font-size:9px}.secret-manager-ref{font:10px ui-monospace,monospace;overflow-wrap:anywhere}.secret-manager-usage{font-size:10px;opacity:.7;overflow-wrap:anywhere}.secret-manager-actions{display:flex;gap:5px;flex-wrap:wrap;justify-content:flex-end}.secret-manager-empty{padding:28px;text-align:center;opacity:.6}.secret-manager-missing{color:#f0c66b}.secret-manager-dialog-card{padding:12px}.secret-manager-dialog-card h3{margin:0 0 10px}.secret-manager-dialog-card input{width:100%;box-sizing:border-box}.secret-manager-dialog-actions{display:flex;justify-content:flex-end;gap:7px;margin-top:10px}
html[data-taskmenu-theme="light"] .secret-manager-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .secret-manager-row{border-color:#d0d7de}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='secret-manager-backdrop';
const dialog=document.createElement('div');dialog.className='secret-manager-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Secrets Manager');
const head=document.createElement('div');head.className='secret-manager-head';
const title=document.createElement('strong');title.textContent='SECRETS / CREDENTIALS';
const refresh=document.createElement('button');refresh.type='button';refresh.textContent='Refresh';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close';
head.append(title,refresh,closeButton);
const note=document.createElement('div');note.className='secret-manager-note';note.textContent='Encrypted at rest. Secret values are write-only: TaskDeck never returns plaintext values to the browser or serializes them into project config.';
const create=document.createElement('div');create.className='secret-manager-create';
const kind=document.createElement('select');
for(const [value,label] of [['generic','Generic'],['git-token','Git token'],['deploy','Deploy secret'],['ssh','SSH credential'],['database','Database credential'],['ftp','FTP credential']]){
  const option=document.createElement('option');option.value=value;option.textContent=label;kind.append(option);
}
const value=document.createElement('input');value.type='password';value.autocomplete='new-password';value.placeholder='New secret value (never displayed again)';
const createButton=document.createElement('button');createButton.type='button';createButton.textContent='Create secret';
create.append(kind,value,createButton);
const list=document.createElement('div');list.className='secret-manager-list';
dialog.append(head,note,create,list);backdrop.append(dialog);document.body.append(backdrop);

let items=[];
let loading=false;

function canManage(){return !app.sharedMode||Boolean(app.hasPermission?.('secrets.manage')||app.hasPermission?.('project.admin'));}
function usageText(item){
  const refs=Array.isArray(item?.referenced_by)?item.referenced_by:[];
  if(!refs.length)return 'Unused';
  return refs.map(ref=>[ref.kind,ref.name||ref.profile_id].filter(Boolean).join(' · ')).join(' | ');
}
async function api(path,options={}){
  return app.jsonFetch(path,{cache:'no-store',...options});
}
function clearSecretInput(input){if(input){input.value='';input.setAttribute('value','');}}
function render(){
  list.replaceChildren();
  create.style.display=canManage()?'grid':'none';
  if(!items.length){const empty=document.createElement('div');empty.className='secret-manager-empty';empty.textContent='No managed secrets';list.append(empty);return;}
  for(const item of items){
    const row=document.createElement('div');row.className='secret-manager-row';
    const type=document.createElement('div');type.className='secret-manager-kind';type.textContent=item.kind||'generic';
    const ref=document.createElement('div');ref.className='secret-manager-ref';ref.textContent=item.id;ref.title=item.id;
    if(!item.present){ref.classList.add('secret-manager-missing');ref.title='Referenced secret is missing from encrypted store';}
    const usage=document.createElement('div');usage.className='secret-manager-usage';usage.textContent=usageText(item);
    const actions=document.createElement('div');actions.className='secret-manager-actions';
    if(canManage()){
      const rotate=document.createElement('button');rotate.type='button';rotate.textContent='Rotate';rotate.disabled=!item.present;rotate.onclick=()=>openRotate(item);
      const remove=document.createElement('button');remove.type='button';remove.textContent='Delete';remove.disabled=Boolean(item.referenced);remove.title=item.referenced?'Remove/change the referencing profile first':'Delete encrypted secret';
      remove.onclick=()=>deleteSecret(item);
      actions.append(rotate,remove);
    }
    row.append(type,ref,usage,actions);list.append(row);
  }
}
async function load(){
  if(loading)return;loading=true;refresh.disabled=true;
  try{const data=await api('/api/secrets');items=Array.isArray(data?.secrets)?data.secrets:[];render();}
  finally{loading=false;refresh.disabled=false;}
}
function openRotate(item){
  const overlay=document.createElement('div');overlay.className='secret-manager-backdrop visible';overlay.style.zIndex='17820';
  const card=document.createElement('div');card.className='secret-manager-dialog secret-manager-dialog-card';card.style.width='min(620px,94vw)';
  const heading=document.createElement('h3');heading.textContent='Rotate secret';
  const ref=document.createElement('div');ref.className='secret-manager-ref';ref.textContent=item.id;
  const input=document.createElement('input');input.type='password';input.autocomplete='new-password';input.placeholder='Replacement secret value';
  const actions=document.createElement('div');actions.className='secret-manager-dialog-actions';
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
  const save=document.createElement('button');save.type='button';save.textContent='Rotate';
  const close=()=>{clearSecretInput(input);overlay.remove();};
  cancel.onclick=close;overlay.onmousedown=event=>{if(event.target===overlay)close();};
  save.onclick=async()=>{
    if(!input.value)return;
    save.disabled=true;
    try{
      const secret=input.value;clearSecretInput(input);
      await api('/api/secrets',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'rotate',id:item.id,value:secret})});
      close();await load();
    }catch(error){clearSecretInput(input);save.disabled=false;app.showError(error);}
  };
  actions.append(cancel,save);card.append(heading,ref,input,actions);overlay.append(card);document.body.append(overlay);requestAnimationFrame(()=>input.focus());
}
async function deleteSecret(item){
  if(item.referenced)return;
  if(!window.confirm('Delete encrypted secret '+item.id+'? This cannot be undone.'))return;
  await api('/api/secrets?id='+encodeURIComponent(item.id),{method:'DELETE'});
  await load();
}
createButton.onclick=async()=>{
  if(!value.value)return;
  createButton.disabled=true;
  try{
    const secret=value.value;clearSecretInput(value);
    await api('/api/secrets',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'create',kind:kind.value,value:secret})});
    await load();
  }catch(error){clearSecretInput(value);app.showError(error);}
  finally{createButton.disabled=false;}
};
refresh.onclick=()=>load().catch(app.showError);closeButton.onclick=()=>{backdrop.classList.remove('visible');clearSecretInput(value);};
backdrop.onmousedown=event=>{if(event.target===backdrop){backdrop.classList.remove('visible');clearSecretInput(value);}};
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible')){backdrop.classList.remove('visible');clearSecretInput(value);}});

function open(){backdrop.classList.add('visible');load().catch(app.showError);}
function close(){backdrop.classList.remove('visible');clearSecretInput(value);}
globalThis.TaskMenuSecrets={open,close,refresh:load,get items(){return [...items];},get visible(){return backdrop.classList.contains('visible');}};
