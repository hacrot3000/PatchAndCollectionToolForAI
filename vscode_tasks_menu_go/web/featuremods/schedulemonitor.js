// Schedule observability. All text is rendered via textContent.
const monitorCSS=document.createElement('style');
monitorCSS.textContent=[
'.schedule-monitor{display:grid;gap:8px;border:1px solid #3f5167;border-radius:6px;padding:10px;font-size:11px}',
'.schedule-monitor header{display:flex;align-items:center;justify-content:space-between;gap:8px}',
'.schedule-monitor .status{display:flex;align-items:center;justify-content:space-between;gap:8px;border-bottom:1px solid #344151;padding:5px 0}',
'.schedule-monitor .status:last-child{border:0}',
'.schedule-monitor button{font-size:11px;padding:4px 8px}',
'.schedule-monitor .actions{display:flex;gap:5px;flex-wrap:wrap}',
'.schedule-monitor .hint{font-size:10px;opacity:.8}',
'.schedule-monitor .conditions{display:grid;gap:3px}',
'.schedule-monitor .badge-ok{color:#65cba8}',
'.schedule-monitor .badge-pending{color:#dfb775}',
'.schedule-monitor .badge-error{color:#ee8d8d}'
].join('\n');
document.head.append(monitorCSS);

function scheduleMonitor(host,{inspect,simulate,onError=()=>{}}={}){
 const element=(name,text,cls)=>{const node=document.createElement(name);if(text!==undefined)node.textContent=text;if(cls)node.className=cls;return node};
 const panel=element('section',undefined,'schedule-monitor');host.append(panel);
 let definitionID='',snapshot=null;
 const date=value=>value?new Date(value).toLocaleString():'—';
 function display(state,nextCron=[],simulated=false){
  panel.replaceChildren();
  const head=element('header'),title=element('strong','Condition Inspector');
  const status=element('span',state?.ready?'Ready':'Waiting');
  status.className=state?.ready?'badge-ok':'badge-pending';
  head.append(title,status);panel.append(head);
  if(!definitionID||!state){
   panel.append(element('div','Save the schedule to inspect condition states.','hint'));
   return;
  }
  if(state.paused)panel.append(element('div','Schedule paused. No automatic starts.','badge-pending'));
  const list=element('div',undefined,'conditions');
  for(const c of state.conditions||[]){
   const row=element('div',undefined,'status'),label=element('span',c.type+' · '+c.id);
   const result=element('span',c.error?'Error':c.latched?'Signaled':c.matched?'Matched':'Waiting');
   result.className=c.error?'badge-error':c.latched||c.matched?'badge-ok':'badge-pending';
   row.append(label,result);list.append(row);
   if(c.error)list.append(element('div',c.error,'badge-error'));
   if(c.expires_at)list.append(element('div','Event expires: '+date(c.expires_at),'hint'));
   if(c.next_check)list.append(element('div','Next check: '+date(c.next_check),'hint'));
  }
  panel.append(list);
  if(nextCron.length)panel.append(element('div','Upcoming cron: '+nextCron.slice(0,5).map(date).join(' · '),'hint'));
  if(simulated)panel.append(element('div','Simulation only; no command was executed.','hint'));
  const actions=element('div',undefined,'actions');
  const refresh=element('button','Refresh inspector');refresh.type='button';refresh.onclick=()=>void load(false);
  const test=element('button','Simulate all matched');test.type='button';test.onclick=()=>void load(true);
  actions.append(refresh,test);panel.append(actions);
 }
 async function load(simulateAll=false){
  const expected=definitionID;
  if(!expected)return;
  try{
   const matches={};
   if(simulateAll)for(const c of snapshot?.conditions||[])matches[c.id]=true;
   const result=await(simulateAll?simulate(expected,matches):inspect(expected));
   if(definitionID!==expected)return;
   snapshot=result.inspection;display(snapshot,result.next_cron||[],simulateAll);
  }catch(error){onError(error)}
 }
 return {
  update(id,state){
   definitionID=id||'';snapshot=state||null;display(snapshot);
  },
  inspect:()=>load(false),
  simulate:()=>load(true)
 };
}
globalThis.TaskMenuScheduleMonitor={create:scheduleMonitor};
