const style=document.createElement('style');
style.textContent='.task-workflows-overlay{display:none;position:fixed;inset:0;z-index:17900;background:#0009;align-items:center;justify-content:center;padding:12px}.task-workflows-overlay.open{display:flex}.task-workflows-dialog{display:flex;flex-direction:column;gap:8px;width:min(980px,98vw);max-height:92vh;padding:12px;background:#17212e;color:#e8f0fa;border:1px solid #64758b;border-radius:9px;overflow:hidden}.task-workflows-dialog h2,.task-workflows-dialog p{margin:0}.task-workflows-toolbar{display:flex;gap:7px;flex-wrap:wrap;align-items:end}.task-workflows-toolbar label{display:flex;flex-direction:column;gap:3px;font-size:11px}.task-workflows-toolbar select,.task-workflows-toolbar input{min-width:120px}.task-workflows-dialog textarea{min-height:130px;max-height:260px;resize:vertical;background:#0f1723;color:inherit;border:1px solid #55667b;border-radius:5px;padding:8px;font:12px/1.45 ui-monospace,monospace}.task-workflows-status{font-size:12px;color:#e7c779;min-height:18px}.task-workflows-list{display:flex;flex-direction:column;gap:7px;min-height:80px;overflow:auto}.task-workflow-card,.task-workflow-task{border:1px solid #42536b;border-radius:6px;padding:8px;display:grid;gap:5px}.task-workflow-header,.task-workflow-actions{display:flex;gap:7px;align-items:center;flex-wrap:wrap}.task-workflow-header strong{flex:1}.task-workflow-task pre{margin:0;max-height:130px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;font:11px/1.4 ui-monospace,monospace}.task-workflow-error{color:#ffaaa6}.task-workflow-footer{display:flex;justify-content:flex-end;gap:6px}html[data-taskmenu-theme="light"] .task-workflows-dialog{background:#fff;color:#17283a;border-color:#b5c3d1}html[data-taskmenu-theme="light"] .task-workflows-dialog textarea{background:#f8fbff;color:#17283a;border-color:#aebdce}';
document.head.append(style);

const el=(tag,text='')=>{const node=document.createElement(tag);node.textContent=text;return node;};
let activeDialog=null;

async function open(){
 const app=globalThis.TaskMenuApp;
 if(!app?.jsonFetch)throw new Error('Task workflow API client is unavailable.');
 if(activeDialog){activeDialog.open();return;}
 const overlay=el('div');overlay.className='task-workflows-overlay';
 const dialog=el('section');dialog.className='task-workflows-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label','Task Manifest Run All');
 const title=el('h2','Task Manifest · Run All');
 const note=el('p','Each task runs in its own detached worktree. Dependencies unlock only after explicit task review. Patch execution still requires individual approval. No owner-branch merge/commit/push is performed.');
 const input=el('textarea');input.placeholder='# Implementation plan\n## Task A — ...\nDepends on: None\nAcceptance:\n- ...';
 const toolbar=el('div');toolbar.className='task-workflows-toolbar';
 const providerLabel=el('label','Provider');const providerSelect=document.createElement('select');providerLabel.append(providerSelect);
 const modelLabel=el('label','Model');const modelSelect=document.createElement('select');modelLabel.append(modelSelect);
 const groupLabel=el('label','Shared context group');const groupSelect=document.createElement('select');groupLabel.append(groupSelect);
 const concurrencyLabel=el('label','Concurrency (max 5)');const concurrency=document.createElement('input');concurrency.type='number';concurrency.min='1';concurrency.max='5';concurrency.value='5';concurrencyLabel.append(concurrency);
 const validateButton=el('button','Validate');const runButton=el('button','Run All');const refreshButton=el('button','Refresh workflows');
 for(const button of [validateButton,runButton,refreshButton])button.type='button';
 toolbar.append(providerLabel,modelLabel,groupLabel,concurrencyLabel,validateButton,runButton,refreshButton);
 const status=el('div');status.className='task-workflows-status';status.setAttribute('role','status');
 const list=el('div');list.className='task-workflows-list';
 const footer=el('div');footer.className='task-workflows-footer';const closeButton=el('button','Close');closeButton.type='button';footer.append(closeButton);
 dialog.append(title,note,input,toolbar,status,list,footer);overlay.append(dialog);document.body.append(overlay);
 let providers=[],groups=[],workflows=[],selectedWorkflowID='',timer=0,loading=false,runDetails=new Map();
 function selectedProvider(){return providers.find(item=>item.id===providerSelect.value)||null;}
 function updateModels(){
  const provider=selectedProvider();modelSelect.replaceChildren();
  for(const model of provider?.models||[]){const option=el('option',model.name||model.id);option.value=model.id;modelSelect.append(option);}
 }
 providerSelect.onchange=updateModels;
 function setMessage(message,error=false){status.textContent=message||'';status.classList.toggle('task-workflow-error',Boolean(error));}
 function decodeRunPayload(value){
  if(!value)return '';
  if(Array.isArray(value))return new TextDecoder().decode(new Uint8Array(value));
  if(typeof value!=='string')return String(value);
  try{const binary=atob(value);return new TextDecoder().decode(Uint8Array.from(binary,char=>char.charCodeAt(0)));}catch{return '';}
 }
 function pendingPatchCalls(run){
  const pending=new Map();
  for(const item of run?.history||[]){
   if(item.type==='function_call')pending.set(item.call_id,item);
   else if(item.type==='function_call_output')pending.delete(item.call_id);
  }
  return [...pending.values()].filter(item=>item.name==='taskdeck_submit_patch');
 }
 function addAction(card,label,handler){const button=el('button',label);button.type='button';button.onclick=async()=>{button.disabled=true;try{await handler();}catch(error){setMessage(String(error?.message||error),true);}finally{button.disabled=false;}};card.append(button);}
 function render(){
  list.replaceChildren();
  if(!workflows.length){list.append(el('p','No task workflows for this owner/project.'));return;}
  for(const workflow of workflows.slice().reverse()){
   const card=el('article');card.className='task-workflow-card';
   const header=el('div');header.className='task-workflow-header';
   const label=el('strong',workflow.manifest?.name||workflow.id);const badge=el('span',String(workflow.status||'unknown').toUpperCase());header.append(label,badge);
   const openButton=el('button',selectedWorkflowID===workflow.id?'Selected':'Inspect');openButton.type='button';openButton.onclick=()=>{selectedWorkflowID=workflow.id;render();};header.append(openButton);card.append(header);
   card.append(el('p',String(workflow.provider_id||'')+' · '+String(workflow.model_id||'')+' · concurrency '+String(workflow.concurrency||0)));
   if(workflow.status==='running'||workflow.status==='queued'||workflow.status==='waiting_review')addAction(card,'Stop workflow',()=>api('/api/agent/task-manifests/runs/'+encodeURIComponent(workflow.id)+'/stop',{method:'POST',body:'{}'}));
   if(selectedWorkflowID===workflow.id){
    for(const task of workflow.manifest?.tasks||[]){
     const state=workflow.dag?.states?.[task.id]||{};
     const row=el('section');row.className='task-workflow-task';
     row.append(el('strong',task.id+' · '+task.title+' · '+String(state.status||'unknown').toUpperCase()));
     if(state.wait_reason)row.append(el('p',state.wait_reason==='dependency'?'Waiting for dependencies':state.wait_reason==='concurrency_slot'?'Waiting for a configured workflow slot':state.wait_reason==='global_concurrency_slot'?'Waiting for the global Agent run slot':String(state.wait_reason)));
     if(state.run_id){
      row.append(el('p','Run '+state.run_id+' · workspace '+String(state.execution_workspace_id||'').slice(0,12)));
      if(['running','waiting_slot'].includes(state.status))addAction(row,'Stop this run',()=>api('/api/agent/runs/'+encodeURIComponent(state.run_id)+'/stop',{method:'POST',body:'{}'}));
      const run=runDetails.get(state.run_id);
      if(run){
       row.append(el('p','Agent Run: '+String(run.status||'unknown').toUpperCase()));
       const events=run.events||[];let text='';for(const event of events){if(event.type==='assistant.delta')text+=decodeRunPayload(event.payload);}
       if(text){const output=el('pre');output.textContent=text;row.append(output);}
       for(const call of pendingPatchCalls(run)){
        const argumentsView=el('pre');try{const args=JSON.parse(call.arguments||'{}');argumentsView.textContent=String(args.patch_python||JSON.stringify(args,null,2));}catch{argumentsView.textContent='Patch arguments are not displayable.';}
        row.append(el('strong','Patch proposal · explicit approval required'),argumentsView);
        addAction(row,'Approve Patch execution',async()=>{if(!rootConfirm('Approve this Patch in its isolated worktree? The owner workspace will not be modified.'))return;return api('/api/agent/runs/'+encodeURIComponent(state.run_id)+'/tools/'+encodeURIComponent(call.call_id)+'/decision',{method:'POST',body:JSON.stringify({action:'approve',confirm_execution:true})});});
        addAction(row,'Reject Patch',()=>api('/api/agent/runs/'+encodeURIComponent(state.run_id)+'/tools/'+encodeURIComponent(call.call_id)+'/decision',{method:'POST',body:JSON.stringify({action:'reject'})}));
       }
       if(['queued','running','waiting_tool'].includes(run.status))addAction(row,'Stop this run',()=>api('/api/agent/runs/'+encodeURIComponent(state.run_id)+'/stop',{method:'POST',body:'{}'}));
      }
     }
     if(state.report){const report=el('pre');try{report.textContent=JSON.stringify(JSON.parse(state.report),null,2);}catch{report.textContent=state.report;}row.append(report);}
     if(state.status==='waiting_review'){
      addAction(row,'Approve task / unlock dependents',()=>api('/api/agent/task-manifests/runs/'+encodeURIComponent(workflow.id)+'/tasks/'+encodeURIComponent(task.id)+'/decision',{method:'POST',body:JSON.stringify({approve:true,fence:state.fence,reason:'reviewed in Task Workflow UI'})}));
      addAction(row,'Reject task',()=>api('/api/agent/task-manifests/runs/'+encodeURIComponent(workflow.id)+'/tasks/'+encodeURIComponent(task.id)+'/decision',{method:'POST',body:JSON.stringify({approve:false,fence:state.fence,reason:'rejected in Task Workflow UI'})}));
     }else if(state.status==='failed'){
      addAction(row,'Retry task',()=>api('/api/agent/task-manifests/runs/'+encodeURIComponent(workflow.id)+'/tasks/'+encodeURIComponent(task.id)+'/retry',{method:'POST',body:'{}'}));
      addAction(row,'Resume after manual side-effect review',async()=>{if(!rootConfirm('Confirm previous side effects were inspected and are safe to resume?'))return;return api('/api/agent/task-manifests/runs/'+encodeURIComponent(workflow.id)+'/tasks/'+encodeURIComponent(task.id)+'/resume',{method:'POST',body:JSON.stringify({confirm_no_side_effect:true})});});
     }else if(state.status==='interrupted'||state.status==='unknown'){
      const unresolved=pendingPatchCalls(runDetails.get(state.run_id));
      if(unresolved.length){
       for(const call of unresolved)addAction(row,'Reconcile native Patch result and continue',()=>api('/api/agent/runs/'+encodeURIComponent(state.run_id)+'/tools/'+encodeURIComponent(call.call_id)+'/continue',{method:'POST',body:'{}'}));
      }else{
       addAction(row,'Resume after manual side-effect review',async()=>{if(!rootConfirm('Confirm previous side effects were inspected and are safe to resume?'))return;return api('/api/agent/task-manifests/runs/'+encodeURIComponent(workflow.id)+'/tasks/'+encodeURIComponent(task.id)+'/resume',{method:'POST',body:JSON.stringify({confirm_no_side_effect:true})});});
      }
     }
     card.append(row);
    }
   }
   list.append(card);
  }
 }
 function rootConfirm(message){return typeof window.confirm==='function'&&window.confirm(message);}
 async function api(url,init){
  const options={...(init||{}),headers:{'Content-Type':'application/json',...(init?.headers||{})}};
  const result=await app.jsonFetch(url,options);await refresh();return result;
 }
 async function refresh(){
  if(loading)return;loading=true;
  try{
   const data=await app.jsonFetch('/api/agent/task-manifests/runs');workflows=Array.isArray(data.workflows)?data.workflows:[];
   const runIDs=new Set();for(const workflow of workflows){for(const task of workflow.manifest?.tasks||[]){const state=workflow.dag?.states?.[task.id];if(state?.run_id&&['running','waiting_slot','unknown','interrupted'].includes(state.status))runIDs.add(state.run_id);}}
   const nextRuns=new Map();await Promise.all([...runIDs].map(async id=>{try{const response=await app.jsonFetch('/api/agent/runs/'+encodeURIComponent(id));if(response.run)nextRuns.set(id,response.run);}catch{}}));runDetails=nextRuns;render();
  }catch(error){setMessage(String(error?.message||error),true);}finally{loading=false;}
 }
 async function loadOptions(){
  providerSelect.replaceChildren();modelSelect.replaceChildren();
  const [providerData,groupData]=await Promise.all([app.jsonFetch('/api/agent/task-manifests/runs/providers'),app.jsonFetch('/api/agent/context-groups')]);
  providers=(providerData.providers||[]).filter(provider=>provider.has_api_key);
  for(const provider of providers){const option=el('option',provider.name||provider.id);option.value=provider.id;providerSelect.append(option);}
  updateModels();
  groups=groupData.groups||[];groupSelect.replaceChildren();const noGroup=el('option','No shared context group');noGroup.value='';groupSelect.append(noGroup);
  for(const group of groups){const option=el('option',group.title+' · rev '+group.revision);option.value=group.id;groupSelect.append(option);}
 }
 validateButton.onclick=async()=>{
  try{
   const data=await app.jsonFetch('/api/agent/task-manifests/validate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({format:'markdown',markdown:input.value})});
   setMessage(data.manifest?.valid?'Valid · '+(data.manifest.tasks||[]).length+' tasks · '+String((data.manifest.graph?.topological_order||[]).length)+' topological entries':'Invalid manifest · inspect issues');
  }catch(error){setMessage(String(error?.message||error),true);}
 };
 runButton.onclick=async()=>{
  try{
   const provider=selectedProvider(),model=(provider?.models||[]).find(item=>item.id===modelSelect.value);
   if(!provider||!model)throw new Error('Select a provider/model with configured credentials.');
   const count=Number(concurrency.value);if(!Number.isInteger(count)||count<1||count>5)throw new Error('Concurrency must be between 1 and 5.');
   const validation=await app.jsonFetch('/api/agent/task-manifests/validate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({format:'markdown',markdown:input.value})});
   if(!validation.manifest?.valid)throw new Error('Manifest validation failed; fix issues before Run All.');
   if(!window.confirm('Send task instructions to '+provider.name+' / '+model.name+'? Each task gets an isolated worktree. No automatic merge, commit or push.'))return;
   const body={format:'markdown',markdown:input.value,provider_id:provider.id,model_id:model.id,provider_config_version:model.config_version,concurrency:count,confirm_external:true};
   if(groupSelect.value)body.context_group_id=groupSelect.value;
   const result=await app.jsonFetch('/api/agent/task-manifests/runs',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
   selectedWorkflowID=result.workflow?.id||'';setMessage('Workflow queued: '+selectedWorkflowID);await refresh();
  }catch(error){setMessage(String(error?.message||error),true);}
 };
 refreshButton.onclick=()=>refresh().catch(error=>setMessage(String(error?.message||error),true));
 closeButton.onclick=()=>{overlay.classList.remove('open');if(timer){clearInterval(timer);timer=0;}};
 overlay.addEventListener('mousedown',event=>{if(event.target===overlay)closeButton.click();});
 activeDialog={open:async()=>{overlay.classList.add('open');await loadOptions();await refresh();if(!timer)timer=setInterval(()=>refresh(),1200);}};
 await activeDialog.open();
}

globalThis.TaskMenuTaskWorkflows={open};
