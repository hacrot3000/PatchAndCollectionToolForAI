// L3 guided-plan review, only. There is intentionally no execution method.
const planApp=globalThis.TaskMenuApp;
if(!planApp)throw Error('TaskMenuApp unavailable for Guided AI Plans');
const css=document.createElement('style');
css.textContent=[
'.taskdeck-ai-plans-backdrop{display:none;position:fixed;inset:0;z-index:17800;background:rgba(0,0,0,.65);justify-content:center;align-items:flex-start;padding:5vh 12px}',
'.taskdeck-ai-plans-backdrop.visible{display:flex}',
'.taskdeck-ai-plans-dialog{box-sizing:border-box;width:min(780px,96vw);max-height:90vh;display:flex;flex-direction:column;gap:8px;padding:12px;background:#171e29;border:1px solid #52647b;border-radius:9px;color:#edf5ff}',
'.taskdeck-ai-plans-dialog h2{margin:0;font-size:15px}.taskdeck-ai-plans-dialog p{font-size:12px;margin:0;line-height:1.5}',
'.taskdeck-ai-plans-dialog textarea{min-height:110px;max-height:190px;resize:vertical;background:#0e1823;color:inherit;border:1px solid #586980;border-radius:5px;font:12px/1.5 ui-monospace,monospace;padding:7px}',
'.taskdeck-ai-plans-toolbar{display:flex;gap:6px;flex-wrap:wrap}.taskdeck-ai-plans-toolbar button{padding:6px 9px}',
'.taskdeck-ai-plans-list{overflow:auto;min-height:60px;flex:1;display:flex;flex-direction:column;gap:8px}',
'.taskdeck-ai-plan-card{padding:8px;border:1px solid #3c4d62;border-radius:6px;display:flex;flex-direction:column;gap:5px}',
'.taskdeck-ai-plan-card code{font:12px/1.5 ui-monospace,monospace;white-space:pre-wrap;overflow-wrap:anywhere}',
'.taskdeck-ai-plan-card button{margin-right:6px}.taskdeck-ai-plans-status{font-size:11px;color:#e2c178}',
'html[data-taskmenu-theme="light"] .taskdeck-ai-plans-dialog{background:#fff;color:#1c2a3a;border-color:#b4c2d0}',
'html[data-taskmenu-theme="light"] .taskdeck-ai-plans-dialog textarea{background:#f8fbff;color:#17283a;border-color:#aebdce}',
'html[data-taskmenu-theme="light"] .taskdeck-ai-plan-card{border-color:#cfdae6}',
'html[data-taskmenu-theme="light"] .taskdeck-ai-plans-status{color:#76520b}'
].join('\n');document.head.append(css);
const el=(tag,cls,text)=>{const e=document.createElement(tag);if(cls)e.className=cls;if(text!==undefined)e.textContent=text;return e;};
const overlay=el('div','taskdeck-ai-plans-backdrop');
const dialog=el('section','taskdeck-ai-plans-dialog');dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label','Guided AI Plan Review');
const heading=el('h2','','AI Guided Plans · Approval Only');
const notice=el('p','','Every plan step is a proposal. Approve/Reject records your decision but DOES NOT execute shell commands. Autonomous L3 execution is disabled.');
const editor=el('textarea');editor.placeholder='JSON plan from Smart Terminal: {"steps":[{"command":"pwd","reason":"Inspect directory","risk":"read-only"}]}';
const buttons=el('div','taskdeck-ai-plans-toolbar');
const loadLatest=el('button','','Use latest AI plan'),save=el('button','','Save plan'),refresh=el('button','','Reload plans'),close=el('button','','Close');
for(const item of [loadLatest,save,refresh,close])item.type='button';buttons.append(loadLatest,save,refresh,close);
const target=el('p','taskdeck-ai-plans-status','Target: none');
const status=el('p','taskdeck-ai-plans-status','');
const list=el('div','taskdeck-ai-plans-list');
dialog.append(heading,notice,target,editor,buttons,status,list);overlay.append(dialog);document.body.append(overlay);
let selectedTarget=null;
function terminalTarget(){
 const active=String(planApp.active||'').replace(/^external:/,'');
 const view=planApp.views.get(active);
 if(!view)return null;
 const state=globalThis.TaskDeckShellIntegration?.getState?.(view.meta?.id);
 return {
  session_id:String(view.meta?.id||''),
  host:String(view.meta?.target_profile_id||view.meta?.label||'local').slice(0,256),
  cwd:String(state?.cwd||view.meta?.cwd||'').slice(0,1024),
  remote:Boolean(view.meta?.target_type==='ssh'||view.meta?.target_profile_id)
 };
}
function parseSteps(input){
 let raw=String(input||'').trim();
 if(raw.startsWith('~~~')||raw.startsWith('```')){
  const firstNewline=raw.indexOf('\n');
  if(firstNewline>=0)raw=raw.slice(firstNewline+1);
  if(raw.endsWith('```'))raw=raw.slice(0,-3).trim();
  if(raw.endsWith('~~~'))raw=raw.slice(0,-3).trim();
 }
 const parsed=JSON.parse(raw);
 if(!Array.isArray(parsed.steps)||parsed.steps.length<1||parsed.steps.length>8)throw Error('Expected JSON with 1–8 steps');
 return parsed.steps.map(x=>{
  const command=String(x.command||'').trim().slice(0,400);
  const reason=String(x.reason||'').trim().slice(0,700);
  const risk=['read-only','low','medium','high','critical'].includes(x.risk)?x.risk:'high';
  if(!command)throw Error('Each step requires a command');
  return {command,reason,risk};
 });
}
async function savePlan(){
 if(!selectedTarget)throw Error('Select an active terminal before saving a plan');
 const now=terminalTarget();
 if(!now||now.session_id!==selectedTarget.session_id)throw Error('Target terminal changed. Reopen plan review.');
 const steps=parseSteps(editor.value);
 if(!window.confirm('Save '+steps.length+' proposals for '+selectedTarget.host+'? No commands will be executed.'))return;
 const result=await planApp.jsonFetch('/api/ai/plans',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({target:selectedTarget,steps})});
 status.textContent='Plan '+result.id+' saved. Approval remains non-executable.';
 await reloadPlans();
}
function card(plan){
 const node=el('div','taskdeck-ai-plan-card');
 node.append(el('strong','','Plan '+plan.id+' · '+plan.target.host+(plan.target.remote?' · SSH':'')));
 node.append(el('p','',new Date(plan.created_at).toLocaleString()+' · execution disabled'));
 plan.steps.forEach((step,index)=>{
  const row=el('div','taskdeck-ai-plan-card');
  row.append(el('p','', 'Step '+(index+1)+' · '+step.risk+' · '+step.decision));
  row.append(el('code','',step.command));
  row.append(el('p','',step.reason));
  if(step.decision==='pending'){
   for(const d of ['approved','rejected']){
    const b=el('button','',d==='approved'?'Approve proposal':'Reject');
    b.type='button';b.onclick=async()=>{
     if(!window.confirm('Mark proposed step '+(index+1)+' '+d+'? NO EXECUTION occurs.'))return;
     b.disabled=true;
     try{
      await planApp.jsonFetch('/api/ai/plans/decision',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:plan.id,step:index,decision:d})});
      status.textContent='Decision recorded. No command executed.';await reloadPlans();
     }catch(e){planApp.showError(e);b.disabled=false;}
    };row.append(b);
   }
  }
  node.append(row);
 });
 return node;
}
async function reloadPlans(){
 const data=await planApp.jsonFetch('/api/ai/plans');
 list.replaceChildren();
 const all=Array.isArray(data.plans)?data.plans:[];
 if(!all.length){list.append(el('p','','No plans saved. Use a Guided Plan response or paste JSON.'));return;}
 all.slice().reverse().forEach(p=>list.append(card(p)));
}
async function open(){
 selectedTarget=terminalTarget();
 const latest=globalThis.TaskMenuAI?.getLastPlan?.();
 if(latest&&latest.target?.session_id===selectedTarget?.session_id){editor.value=latest.text;selectedTarget=latest.target;}
 else editor.value='';
 target.textContent='Target: '+(selectedTarget?selectedTarget.host+' · '+selectedTarget.session_id:'Select a terminal');
 status.textContent='Approval records intent only. L3 execution broker is not available.';
 overlay.classList.add('visible');await reloadPlans();editor.focus();
}
function dismiss(){overlay.classList.remove('visible');selectedTarget=null;}
loadLatest.onclick=()=>{
 const latest=globalThis.TaskMenuAI?.getLastPlan?.();
 if(!latest||latest.target?.session_id!==terminalTarget()?.session_id){status.textContent='Generate Guided Plan on the current terminal first.';return;}
 editor.value=latest.text;selectedTarget=latest.target;status.textContent='Proposal loaded. Verify commands before saving.';
};
save.onclick=()=>savePlan().catch(planApp.showError);
refresh.onclick=()=>reloadPlans().catch(planApp.showError);
close.onclick=dismiss;
overlay.addEventListener('mousedown',e=>{if(e.target===overlay)dismiss();});
dialog.addEventListener('keydown',e=>{if(e.key==='Escape'){e.preventDefault();e.stopPropagation();dismiss();}});
globalThis.TaskMenuAIPlans={open,close:dismiss,parseSteps,get visible(){return overlay.classList.contains('visible')}};
