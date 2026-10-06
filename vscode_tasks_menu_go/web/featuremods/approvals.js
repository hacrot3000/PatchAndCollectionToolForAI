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

globalThis.TaskMenuApprovals={authorize,get active(){return Boolean(active);}};
