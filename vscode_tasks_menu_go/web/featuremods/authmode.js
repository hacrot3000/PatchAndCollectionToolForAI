const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for authentication mode wizard');

const endpoint='/api/config/auth-mode';
let dialog=null;
let status=null;
let selectedMode='';
let step=1;

const style=document.createElement('style');
style.textContent=
'.auth-mode-settings{white-space:normal}'+
'.auth-mode-backdrop{position:fixed;inset:0;z-index:6500;background:rgba(0,0,0,.58);display:flex;align-items:center;justify-content:center;padding:18px}'+
'.auth-mode-dialog{width:min(680px,96vw);max-height:92vh;overflow:auto;background:#171a20;color:#eef2f6;border:1px solid #3b414d;border-radius:11px;box-shadow:0 20px 60px rgba(0,0,0,.55)}'+
'.auth-mode-head{display:flex;align-items:flex-start;gap:12px;padding:16px 18px 12px;border-bottom:1px solid #303641}'+
'.auth-mode-head h2{margin:0;font-size:18px}.auth-mode-head p{margin:4px 0 0;font-size:11px;opacity:.7}'+
'.auth-mode-close{margin-left:auto;border:0;background:transparent;color:inherit;font-size:20px;padding:0 5px}'+
'.auth-mode-steps{display:grid;grid-template-columns:repeat(3,1fr);gap:6px;padding:12px 18px 0}'+
'.auth-mode-step{font-size:10px;padding:6px 8px;border-radius:6px;border:1px solid #363d49;opacity:.6}'+
'.auth-mode-step.active{opacity:1;border-color:#79b8ef;background:#183149}.auth-mode-step.done{opacity:.85;border-color:#4f775d}'+
'.auth-mode-body{padding:16px 18px}.auth-mode-current{padding:9px 11px;border:1px solid #343b47;border-radius:7px;background:#10141a;font-size:12px;margin-bottom:13px}'+
'.auth-mode-grid{display:grid;grid-template-columns:1fr 1fr;gap:10px}.auth-mode-card{border:1px solid #3b414d;border-radius:8px;padding:12px;text-align:left;background:#11161d;color:inherit}'+
'.auth-mode-card.selected{border-color:#79b8ef;box-shadow:0 0 0 1px #79b8ef inset}.auth-mode-card.current{opacity:.65}'+
'.auth-mode-card strong{display:block;margin-bottom:5px}.auth-mode-card span{display:block;font-size:11px;opacity:.72;line-height:1.45}'+
'.auth-mode-form{display:grid;grid-template-columns:160px minmax(0,1fr);gap:9px 12px;align-items:center}'+
'.auth-mode-form label{font-size:11px;font-weight:700;opacity:.8}.auth-mode-form input{width:100%;box-sizing:border-box;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:8px 9px}'+
'.auth-mode-hint{grid-column:2;font-size:10px;opacity:.65;margin-top:-5px;line-height:1.4}.auth-mode-warning{margin-top:13px;padding:9px 11px;border-left:3px solid #c59b4b;background:#2b2417;font-size:11px;line-height:1.5}'+
'.auth-mode-review{display:grid;grid-template-columns:160px 1fr;gap:8px 12px;font-size:12px}.auth-mode-review dt{opacity:.65}.auth-mode-review dd{margin:0;overflow-wrap:anywhere}'+
'.auth-mode-checks{margin:14px 0 0;padding-left:20px;font-size:11px;line-height:1.65}.auth-mode-result{margin-top:12px;padding:10px;border-radius:7px;background:#142a1d;border:1px solid #335c40;font-size:11px;white-space:pre-wrap}'+
'.auth-mode-actions{display:flex;justify-content:flex-end;gap:8px;padding:12px 18px 16px;border-top:1px solid #303641}.auth-mode-actions button{min-width:88px}.auth-mode-actions .primary{font-weight:700}.auth-mode-actions .danger{background:#49292d;border-color:#704047;color:#ffe2e4}'+
'html[data-taskmenu-theme="light"] .auth-mode-dialog{background:#fff;color:#20242b;border-color:#b9c0c8}'+
'html[data-taskmenu-theme="light"] .auth-mode-current,html[data-taskmenu-theme="light"] .auth-mode-card{background:#f7f9fb;border-color:#c7ced7}'+
'html[data-taskmenu-theme="light"] .auth-mode-form input{background:#fff;color:#20242b;border-color:#b9c0c8}'+
'html[data-taskmenu-theme="light"] .auth-mode-warning{background:#fff7df}'+
'@media(max-width:620px){.auth-mode-grid{grid-template-columns:1fr}.auth-mode-form,.auth-mode-review{grid-template-columns:1fr}.auth-mode-hint{grid-column:1}.auth-mode-dialog{max-height:96vh}.auth-mode-steps{grid-template-columns:1fr}}';
document.head.append(style);

const settingsButton=document.createElement('button');
settingsButton.type='button';
settingsButton.id='auth-mode-settings';
settingsButton.className='auth-mode-settings';
settingsButton.textContent='Authentication mode…';
settingsButton.hidden=true;
settingsButton.onclick=()=>openWizard().catch(app.showError);
document.body.append(settingsButton);

function el(tag,className,text){
  const node=document.createElement(tag);
  if(className)node.className=className;
  if(text!==undefined)node.textContent=text;
  return node;
}
function field(label,value,type='text'){
  const labelNode=el('label','',label);
  const input=document.createElement('input');
  input.type=type;input.value=value||'';
  return {label:labelNode,input};
}
function currentModeLabel(){
  return status?.mode==='shared'?'Multi-user shared-server':'Single authentication';
}
function targetLabel(){
  return selectedMode==='shared'?'Multi-user shared-server':'Single authentication';
}
function closeWizard(){
  if(dialog){dialog.remove();dialog=null;}
  status=null;selectedMode='';step=1;
}
async function loadStatus(){
  return app.jsonFetch(endpoint);
}
async function openWizard(){
  status=await loadStatus();
  if(status.mode==='shared'&&!status.can_migrate){
    throw new Error('Project administrator permission is required to change authentication mode.');
  }
  selectedMode=status.mode==='shared'?'single':'shared';
  step=1;
  renderWizard();
}
function shell(){
  const backdrop=el('div','auth-mode-backdrop');
  const card=el('div','auth-mode-dialog');
  const head=el('div','auth-mode-head');
  const titles=el('div');
  titles.append(el('h2','', 'Authentication migration wizard'),el('p','',
    'Switch authentication mode without manually editing TaskDeck configuration.'));
  const close=el('button','auth-mode-close','×');close.type='button';close.title='Close';close.onclick=closeWizard;
  head.append(titles,close);
  const steps=el('div','auth-mode-steps');
  ['1 · Mode','2 · Setup','3 · Review'].forEach((label,index)=>{
    const item=el('div','auth-mode-step',label);
    const number=index+1;
    item.classList.toggle('active',number===step);
    item.classList.toggle('done',number<step);
    steps.append(item);
  });
  const body=el('div','auth-mode-body');
  const actions=el('div','auth-mode-actions');
  card.append(head,steps,body,actions);backdrop.append(card);
  backdrop.addEventListener('pointerdown',event=>{if(event.target===backdrop)closeWizard();});
  return {backdrop,body,actions};
}
function renderWizard(){
  if(dialog)dialog.remove();
  const made=shell();dialog=made.backdrop;document.body.append(dialog);
  if(step===1)renderModeStep(made.body,made.actions);
  else if(step===2)renderSetupStep(made.body,made.actions);
  else renderReviewStep(made.body,made.actions);
}
function currentSummary(){
  const box=el('div','auth-mode-current');
  box.append(document.createTextNode('Current mode: '));
  const strong=el('strong','',currentModeLabel());box.append(strong);
  if(status.mode==='shared'){
    box.append(document.createTextNode(' · Project: '+(status.shared_project_id||'—')));
  }else{
    box.append(document.createTextNode(status.auth_enabled?' · Basic Auth enabled':' · Local/no Basic Auth'));
  }
  return box;
}
function modeCard(mode,title,description){
  const button=el('button','auth-mode-card');
  button.type='button';button.dataset.mode=mode;
  if(status.mode===mode)button.classList.add('current');
  if(selectedMode===mode)button.classList.add('selected');
  button.append(el('strong','',title),el('span','',description));
  button.onclick=()=>{
    selectedMode=mode;
    for(const card of dialog.querySelectorAll('.auth-mode-card'))card.classList.toggle('selected',card.dataset.mode===mode);
  };
  return button;
}
function renderModeStep(body,actions){
  body.append(currentSummary());
  const grid=el('div','auth-mode-grid');
  grid.append(
    modeCard('single','Single authentication','One Basic Auth credential for this TaskDeck workspace. Best for local or single-operator use.'),
    modeCard('shared','Multi-user shared-server','HTTPS login, shared identity DB, users, roles, permissions, sessions and audit for a shared project.')
  );
  body.append(grid);
  const cancel=el('button','','Cancel');cancel.type='button';cancel.onclick=closeWizard;
  const next=el('button','primary','Continue');next.type='button';next.onclick=()=>{
    if(selectedMode===status.mode){alert('That authentication mode is already active.');return;}
    step=2;renderWizard();
  };
  actions.append(cancel,next);
}
function setupState(){
  return globalThis.__taskdeckAuthModeSetup||(globalThis.__taskdeckAuthModeSetup={});
}
function renderSetupStep(body,actions){
  body.append(currentSummary());
  const saved=setupState();
  const form=el('div','auth-mode-form');
  if(selectedMode==='shared'){
    const project=field('Shared project ID',saved.project_id||status.shared_project_id||status.suggested_project_id);
    project.input.id='auth-mode-project-id';
    const db=field('Identity DB',saved.identity_db!==undefined?saved.identity_db:(status.shared_identity_db||''));
    db.input.id='auth-mode-identity-db';db.input.placeholder=status.resolved_identity_db||'Use TaskDeck default identity DB';
    const user=field('Initial admin username',saved.username||status.username||'admin');
    user.input.id='auth-mode-username';user.input.autocomplete='username';
    const password=field('Admin password',saved.password||'','password');
    password.input.id='auth-mode-password';password.input.autocomplete='current-password';
    if(status.legacy_credential_reusable)password.input.placeholder='Leave blank to reuse current single-auth password';
    form.append(project.label,project.input);
    const pHint=el('div','auth-mode-hint','Stable project key used to scope users, roles and sessions in the shared identity DB.');form.append(pHint);
    form.append(db.label,db.input);
    const dHint=el('div','auth-mode-hint','Leave blank to use '+status.resolved_identity_db+'. The DB must remain outside the served workspace.');form.append(dHint);
    form.append(user.label,user.input,password.label,password.input);
    const passHint=el('div','auth-mode-hint',status.legacy_credential_reusable?
      'Password is optional: leaving it blank securely reuses the current single-auth credential for the first shared admin.':
      'A new shared password is required because the current single-auth credential cannot be reused under the shared password policy.');
    form.append(passHint);
    body.append(form);
    body.append(el('div','auth-mode-warning',
      'TaskDeck will provision or verify the shared identity, grant this user project admin access, enable HTTPS, disable Basic Auth, scrub the old plaintext Basic Auth password from the active config, then reload the web daemon.'));
  }else{
    const user=field('Single-auth username',saved.username||app.currentUser?.username||status.username||'admin');
    user.input.id='auth-mode-username';user.input.autocomplete='username';
    const password=field('New password',saved.password||'','password');
    password.input.id='auth-mode-password';password.input.autocomplete='new-password';
    const confirm=field('Confirm password',saved.confirm||'','password');
    confirm.input.id='auth-mode-confirm';confirm.input.autocomplete='new-password';
    form.append(user.label,user.input,password.label,password.input,confirm.label,confirm.input);
    body.append(form);
    body.append(el('div','auth-mode-warning',
      'Shared users and the identity DB will be retained, but all browser sessions for this project will be revoked. The new Basic Auth password cannot be recovered from a shared password hash, so it must be set explicitly.'));
  }
  const back=el('button','','Back');back.type='button';back.onclick=()=>{captureSetup();step=1;renderWizard();};
  const next=el('button','primary','Review');next.type='button';next.onclick=()=>{
    if(!captureSetup(true))return;
    step=3;renderWizard();
  };
  actions.append(back,next);
  setTimeout(()=>dialog.querySelector('input')?.focus(),0);
}
function captureSetup(validate=false){
  const saved=setupState();
  saved.username=dialog.querySelector('#auth-mode-username')?.value.trim()||'';
  saved.password=dialog.querySelector('#auth-mode-password')?.value||'';
  if(selectedMode==='shared'){
    saved.project_id=dialog.querySelector('#auth-mode-project-id')?.value.trim()||'';
    saved.identity_db=dialog.querySelector('#auth-mode-identity-db')?.value.trim()||'';
    if(validate&&(!saved.project_id||!saved.username)){
      alert('Project ID and admin username are required.');return false;
    }
    if(validate&&!status.legacy_credential_reusable&&!saved.password){
      alert('A shared admin password is required.');return false;
    }
  }else{
    saved.confirm=dialog.querySelector('#auth-mode-confirm')?.value||'';
    if(validate&&(!saved.username||!saved.password)){
      alert('Username and a new password are required.');return false;
    }
    if(validate&&saved.password!==saved.confirm){
      alert('Password confirmation does not match.');return false;
    }
  }
  return true;
}
function addReviewRow(dl,label,value){
  dl.append(el('dt','',label),el('dd','',value||'—'));
}
function renderReviewStep(body,actions){
  body.append(currentSummary());
  const saved=setupState();
  const dl=el('dl','auth-mode-review');
  addReviewRow(dl,'Target mode',targetLabel());
  addReviewRow(dl,'Username',saved.username);
  if(selectedMode==='shared'){
    addReviewRow(dl,'Project ID',saved.project_id);
    addReviewRow(dl,'Identity DB',saved.identity_db||status.resolved_identity_db);
    addReviewRow(dl,'Protocol','HTTPS');
    addReviewRow(dl,'Password source',saved.password?'Password entered in wizard':'Current single-auth credential');
  }else{
    addReviewRow(dl,'Shared identity DB','Retained; not deleted');
    addReviewRow(dl,'Shared sessions','Revoked for this project');
    addReviewRow(dl,'Protocol',status.protocol.toUpperCase());
    addReviewRow(dl,'Password','New password entered in wizard');
  }
  body.append(dl);
  const checks=el('ul','auth-mode-checks');
  const items=selectedMode==='shared'
    ?[
      'Identity provisioning completes before shared mode is enabled.',
      'An existing global username is never overwritten; its password must verify before project admin membership is granted.',
      'Basic Auth is disabled and its plaintext password is replaced in the active config.',
      status.restart_preserves_sessions?'Web daemon reload preserves the independent session broker and running terminals/tasks.':'A manual daemon restart is required on this platform.'
    ]
    :[
      'A new single-auth credential is written atomically; shared password hashes are never decrypted or exported.',
      'All current shared browser sessions for this project are revoked before switching.',
      'Shared users, roles and identity DB remain intact for a future switch back.',
      status.restart_preserves_sessions?'Web daemon reload preserves the independent session broker and running terminals/tasks.':'A manual daemon restart is required on this platform.'
    ];
  for(const item of items)checks.append(el('li','',item));
  body.append(checks);

  const back=el('button','','Back');back.type='button';back.onclick=()=>{step=2;renderWizard();};
  const apply=el('button',selectedMode==='single'?'danger primary':'primary','Switch authentication mode');
  apply.type='button';apply.onclick=()=>submitMigration(apply,back).catch(app.showError);
  actions.append(back,apply);
}
async function submitMigration(apply,back){
  const saved=setupState();
  const payload={target_mode:selectedMode,username:saved.username,password:saved.password||''};
  if(selectedMode==='shared'){
    payload.project_id=saved.project_id;
    payload.identity_db=saved.identity_db||'';
  }
  apply.disabled=true;back.disabled=true;apply.textContent='Applying…';
  let result;
  try{
    result=await app.jsonFetch(endpoint,{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify(payload)
    });
  }catch(error){
    apply.disabled=false;back.disabled=false;apply.textContent='Switch authentication mode';
    throw error;
  }finally{
    saved.password='';saved.confirm='';
  }
  const resultBox=el('div','auth-mode-result');
  resultBox.textContent=String(result.message||'Authentication mode updated.')+
    (result.restart_scheduled?'\nRestarting TaskDeck web daemon…':'\nRestart the TaskDeck daemon to activate the new mode.');
  dialog.querySelector('.auth-mode-body').append(resultBox);
  apply.disabled=true;back.disabled=true;
  apply.textContent=result.restart_scheduled?'Restarting…':'Configured';
  if(result.restart_scheduled){
    const scheme=result.protocol==='https'?'https:':window.location.protocol;
    const target=scheme+'//'+window.location.host+(result.login_path||'/');
    setTimeout(()=>window.location.assign(target),1800);
  }
}

globalThis.TaskMenuAuthModeWizard={open:openWizard,loadStatus};
