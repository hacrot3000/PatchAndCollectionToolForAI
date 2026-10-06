const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for approvals');

const style=document.createElement('style');
style.textContent=`
.approval-backdrop{display:none;position:fixed;inset:0;z-index:19000;background:rgba(0,0,0,.58);align-items:center;justify-content:center;padding:18px}
.approval-backdrop.visible{display:flex}
.approval-card{width:min(620px,96vw);background:#151a21;border:1px solid #48515f;border-radius:10px;box-shadow:0 18px 55px rgba(0,0,0,.55);padding:14px}
.approval-card h3{margin:0 0 7px;font-size:14px}.approval-card p{font-size:11px;line-height:1.45;margin:6px 0}.approval-resource{font:11px/1.4 ui-monospace,monospace;padding:7px;background:#0b0f14;border:1px solid #303843;border-radius:6px;overflow-wrap:anywhere}
.approval-input{width:100%;box-sizing:border-box;margin-top:7px;padding:7px;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px}
.approval-status{font-size:10px;margin-top:8px;opacity:.75}.approval-actions{display:flex;justify-content:flex-end;gap:7px;margin-top:12px}
html[data-taskmenu-theme="light"] .approval-card{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .approval-resource,html[data-taskmenu-theme="light"] .approval-input{background:#f6f8fa;color:#202124;border-color:#d0d7de}
.approval-manager{width:min(880px,96vw);max-height:88vh;display:flex;flex-direction:column}.approval-manager-body{overflow:auto}.approval-policy-grid{display:grid;grid-template-columns:minmax(210px,1fr) 170px;gap:6px 10px;align-items:center;margin:8px 0 14px}.approval-policy-grid label{font-size:11px}.approval-policy-grid select{width:100%;padding:6px}.approval-request-list{display:flex;flex-direction:column;gap:6px}.approval-request-row{display:grid;grid-template-columns:minmax(170px,1fr) minmax(180px,1.4fr) 90px auto;gap:7px;align-items:center;padding:7px;border:1px solid #303843;border-radius:6px}.approval-request-row span{font-size:10px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.approval-request-actions{display:flex;gap:4px}.approval-manager-footer{display:flex;gap:7px;justify-content:flex-end;margin-top:12px}.approval-empty{padding:12px;opacity:.6;text-align:center}
`;
document.head.append(style);

let active=null;

function wait(ms){return new Promise(resolve=>setTimeout(resolve,ms));}
function requestApproval(action,resource,confirmation=''){
  return app.jsonFetch('/api/approvals/request',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({action,resource,confirmation})
  });
}
function approvalStatus(id){
  return app.jsonFetch('/api/approvals/request?id='+encodeURIComponent(id),{cache:'no-store'});
}
function closeActive(value=null,error=null){
  const state=active;if(!state)return;
  active=null;state.overlay.remove();
  if(error)state.reject(error);else state.resolve(value);
}
function cancelError(){const error=new Error('Dangerous action approval cancelled');error.approvalCancelled=true;return error;}

async function waitForAdminApproval(state,id){
  while(active===state){
    const item=await approvalStatus(id);
    const status=String(item?.status||'').toLowerCase();
    state.status.textContent=status==='pending'
      ?'Waiting for a project administrator to approve this request…'
      :('Approval '+status+'.');
    if(status==='approved')return id;
    if(status==='rejected')throw new Error('Dangerous action approval was rejected');
    if(status==='expired')throw new Error('Dangerous action approval expired');
    await wait(1800);
  }
  throw cancelError();
}

function authorize(challenge={}){
  if(active)return Promise.reject(new Error('Another dangerous action approval is already open'));
  const action=String(challenge.action||'').trim();
  const resource=String(challenge.resource||'').trim();
  const mode=String(challenge.mode||'off').trim().toLowerCase();
  if(!action)return Promise.reject(new Error('Approval challenge is missing an action'));
  if(mode==='off')return Promise.resolve('');

  return new Promise((resolve,reject)=>{
    const overlay=document.createElement('div');overlay.className='approval-backdrop visible';
    const card=document.createElement('div');card.className='approval-card';card.setAttribute('role','dialog');card.setAttribute('aria-label','Dangerous action approval');
    const title=document.createElement('h3');title.textContent='Dangerous action approval';
    const intro=document.createElement('p');
    intro.textContent=mode==='admin'
      ?'This project requires administrator approval before TaskDeck can continue.'
      :(mode==='type'?'Type the exact resource below to authorize this one action.':'Confirm this dangerous action before TaskDeck continues.');
    const actionText=document.createElement('p');actionText.textContent='Action: '+action;
    const resourceBox=document.createElement('div');resourceBox.className='approval-resource';resourceBox.textContent=resource||'(project-wide action)';
    const status=document.createElement('div');status.className='approval-status';
    const actions=document.createElement('div');actions.className='approval-actions';
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
    const submit=document.createElement('button');submit.type='button';submit.textContent=mode==='admin'?'Request approval':'Authorize';
    let input=null;
    if(mode==='type'){
      input=document.createElement('input');input.className='approval-input';input.autocomplete='off';input.placeholder=resource;
    }else if(mode==='confirm'){
      input=document.createElement('input');input.className='approval-input';input.autocomplete='off';input.placeholder='Type CONFIRM';
    }
    actions.append(cancel,submit);card.append(title,intro,actionText,resourceBox);
    if(input)card.append(input);
    card.append(status,actions);overlay.append(card);document.body.append(overlay);
    const state={overlay,status,resolve,reject};active=state;

    cancel.onclick=()=>closeActive(null,cancelError());
    overlay.onmousedown=event=>{if(event.target===overlay)closeActive(null,cancelError());};
    const keyHandler=event=>{
      if(active!==state)return document.removeEventListener('keydown',keyHandler,true);
      if(event.key==='Escape'){event.preventDefault();closeActive(null,cancelError());document.removeEventListener('keydown',keyHandler,true);}
    };
    document.addEventListener('keydown',keyHandler,true);

    submit.onclick=async()=>{
      submit.disabled=true;cancel.disabled=true;
      try{
        const confirmation=mode==='type'?String(input?.value||'').trim():(mode==='confirm'?'CONFIRM':'');
        if(mode==='type'&&confirmation!==resource)throw new Error('The typed resource does not match exactly');
        if(mode==='confirm'&&input&&String(input.value||'').trim()!=='CONFIRM')throw new Error('Type CONFIRM to continue');
        status.textContent=mode==='admin'?'Creating approval request…':'Authorizing…';
        const result=await requestApproval(action,resource,confirmation);
        const item=result?.request;
        if(!item?.id)throw new Error('Approval server did not return a request id');
        let id=item.id;
        if(String(item.status||'').toLowerCase()==='pending'){
          submit.hidden=true;cancel.disabled=false;
          id=await waitForAdminApproval(state,id);
        }
        if(active===state)closeActive(id);
      }catch(error){
        if(active!==state)return;
        status.textContent=String(error?.message||error);
        submit.disabled=false;cancel.disabled=false;
      }
    };
    requestAnimationFrame(()=>input?.focus());
  });
}


const managedActions=[
  ['git.remote.delete','Delete remote Git branch'],
  ['git.force_delete','Force delete local Git branch'],
  ['git.reset_hard','Git reset --hard'],
  ['transfer.recursive_delete','Recursive remote directory delete'],
  ['db.production.write','Production database write/schema'],
  ['task.deploy','Deploy-marked task']
];
const approvalModes=[
  ['off','Off'],
  ['confirm','Confirm'],
  ['type','Type exact resource'],
  ['admin','Admin approval']
];

function canViewApprovals(){
  return Boolean(app.sharedMode&&(app.hasPermission?.('approvals.view')||app.hasPermission?.('approvals.manage')||app.hasPermission?.('project.admin')));
}
function canManageApprovals(){
  return Boolean(app.sharedMode&&(app.hasPermission?.('approvals.manage')||app.hasPermission?.('project.admin')));
}
async function loadManagerData(){
  const [policy,requests]=await Promise.all([
    app.jsonFetch('/api/approvals/policy',{cache:'no-store'}),
    app.jsonFetch('/api/approvals',{cache:'no-store'})
  ]);
  return {policy,requests:Array.isArray(requests?.requests)?requests.requests:[]};
}
async function resolveRequest(id,status){
  return app.jsonFetch('/api/approvals/resolve',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({id,status})
  });
}
async function savePolicy(rules){
  return app.jsonFetch('/api/approvals/policy',{
    method:'PUT',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({version:1,rules})
  });
}
async function openManager(){
  if(!canViewApprovals())throw new Error('Approval workflow view permission is required');
  const overlay=document.createElement('div');overlay.className='approval-backdrop visible';
  const card=document.createElement('div');card.className='approval-card approval-manager';
  const title=document.createElement('h3');title.textContent='Dangerous Action Approvals';
  const intro=document.createElement('p');intro.textContent='Project policy is OFF by default. Approval adds a second gate; normal RBAC permissions are still required.';
  const body=document.createElement('div');body.className='approval-manager-body';
  const footer=document.createElement('div');footer.className='approval-manager-footer';
  const refresh=document.createElement('button');refresh.type='button';refresh.textContent='Refresh';
  const save=document.createElement('button');save.type='button';save.textContent='Save policy';save.hidden=!canManageApprovals();
  const close=document.createElement('button');close.type='button';close.textContent='Close';
  footer.append(refresh,save,close);card.append(title,intro,body,footer);overlay.append(card);document.body.append(overlay);
  const policySelects=new Map();

  async function render(){
    refresh.disabled=true;save.disabled=true;
    try{
      const data=await loadManagerData();body.replaceChildren();policySelects.clear();
      const policyTitle=document.createElement('strong');policyTitle.textContent='Policy';
      const grid=document.createElement('div');grid.className='approval-policy-grid';
      const current=new Map((Array.isArray(data.policy?.rules)?data.policy.rules:[]).map(rule=>[String(rule.action),String(rule.mode)]));
      for(const [action,labelText] of managedActions){
        const label=document.createElement('label');label.textContent=labelText;label.title=action;
        const select=document.createElement('select');select.disabled=!canManageApprovals();
        for(const [value,text] of approvalModes){
          const option=document.createElement('option');option.value=value;option.textContent=text;select.append(option);
        }
        select.value=current.get(action)||'off';policySelects.set(action,select);grid.append(label,select);
      }
      body.append(policyTitle,grid);
      const requestTitle=document.createElement('strong');requestTitle.textContent='Recent requests';
      const list=document.createElement('div');list.className='approval-request-list';
      const rows=data.requests.slice().reverse().slice(0,100);
      if(!rows.length){const empty=document.createElement('div');empty.className='approval-empty';empty.textContent='No approval requests yet';list.append(empty);}
      for(const item of rows){
        const row=document.createElement('div');row.className='approval-request-row';
        const action=document.createElement('span');action.textContent=String(item.action||'');action.title=action.textContent;
        const resource=document.createElement('span');resource.textContent=String(item.resource||'');resource.title=resource.textContent;
        const status=document.createElement('span');status.textContent=String(item.status||'').toUpperCase();
        const actions=document.createElement('div');actions.className='approval-request-actions';
        if(canManageApprovals()&&item.status==='pending'){
          const approve=document.createElement('button');approve.type='button';approve.textContent='Approve';
          const reject=document.createElement('button');reject.type='button';reject.textContent='Reject';
          approve.onclick=()=>resolveRequest(item.id,'approved').then(render).catch(app.showError);
          reject.onclick=()=>resolveRequest(item.id,'rejected').then(render).catch(app.showError);
          actions.append(approve,reject);
        }
        row.append(action,resource,status,actions);list.append(row);
      }
      body.append(requestTitle,list);
    }finally{refresh.disabled=false;save.disabled=false;}
  }
  refresh.onclick=()=>render().catch(app.showError);
  save.onclick=async()=>{
    save.disabled=true;
    try{
      const rules=[];
      for(const [action,select] of policySelects){
        if(select.value!=='off')rules.push({action,mode:select.value});
      }
      await savePolicy(rules);await render();
    }catch(error){app.showError(error);}finally{save.disabled=false;}
  };
  const remove=()=>overlay.remove();close.onclick=remove;overlay.onmousedown=event=>{if(event.target===overlay)remove();};
  await render();
}
function installManagerLauncher(){
  let button=document.querySelector('#approval-manager');
  if(!canViewApprovals()){button?.remove();return false;}
  if(button)return true;
  button=document.createElement('button');button.id='approval-manager';button.type='button';button.textContent='Approvals';button.title='Dangerous action approval policy and requests';
  button.onclick=()=>openManager().catch(app.showError);
  const sharedAdmin=document.querySelector('#shared-admin');
  if(sharedAdmin?.parentElement)sharedAdmin.insertAdjacentElement('afterend',button);
  else document.querySelector('header')?.append(button);
  return true;
}
installManagerLauncher();
window.addEventListener('taskmenu:permissions-changed',installManagerLauncher);

globalThis.TaskMenuApprovals={authorize,openManager,canView:canViewApprovals,canManage:canManageApprovals,get active(){return Boolean(active);}};
