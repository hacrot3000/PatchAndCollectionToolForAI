// Recursive AND/OR Schedule condition editor; all values are DOM text and
// never interpolated as HTML. Conditions receive persistent IDs on save.
const scheduleExprStyles=document.createElement('style');
scheduleExprStyles.textContent=[
'.schedule-expression{border:1px solid #475567;border-radius:6px;padding:9px;display:flex;flex-direction:column;gap:9px;min-width:0}',
'.schedule-expression .expr-group{border-left:2px solid #637b9d;margin-left:5px;padding:7px 0 5px 10px;display:flex;flex-direction:column;gap:9px;min-width:0}',
'.schedule-expression .expr-heading{display:flex;gap:5px;align-items:center;flex-wrap:wrap;font-size:11px}',
'.schedule-expression .expr-heading strong{flex:1;min-width:80px}',
'.schedule-expression .expr-leaf{padding:8px;border:1px solid #425267;border-radius:5px;display:grid;gap:6px;min-width:0}',
'.schedule-expression .expr-leaf header{display:flex;align-items:center;gap:6px;font-size:11px}',
'.schedule-expression .expr-leaf header strong{flex:1}',
'.schedule-expression .expr-field{display:flex;flex-direction:column;gap:3px;font-size:11px;min-width:0}',
'.schedule-expression input,.schedule-expression select{min-width:0;max-width:100%;width:100%;padding:5px}',
'.schedule-expression button{font-size:11px;padding:3px 7px}',
'.schedule-expression .expr-note{font-size:10px;opacity:.68}',
'.schedule-expression .expr-status{font-size:10px;opacity:.8}',
'.schedule-expression .expr-git-actions{display:flex;align-items:center;gap:6px;min-width:0}.schedule-expression .expr-git-actions select{min-width:0;flex:1}.schedule-expression .expr-git-actions button{flex:none}'
].join('\n');
document.head.append(scheduleExprStyles);

const scheduleExprTypes=[['cron','Time / Cron'],['url','URL response'],['git','Git repository'],['dependency','After schedule']];
function scheduleEmptyCondition(type='cron'){
 return {id:'',type,cron:'*/5 * * * *',timezone:'Local',interval_seconds:60,
  url:'',match_mode:'contains',match_text:'',json_path:'',
  git_path:'',git_ref:'HEAD',git_condition:'change',
  parent_id:'',parent_state:'success',exit_code:null,output_contains:'',output_absent:''};
}
function scheduleLegacyGroup(value){
 const source=value||{};
 const fields=Object.keys(scheduleEmptyCondition());
 return {operator:'and',conditions:[Object.fromEntries(fields.map(key=>[key,
   key==='id'?'legacy':source[key]===undefined?scheduleEmptyCondition()[key]:source[key]]))],groups:[]};
}
function scheduleNormalizeGroup(source){
 if(!source||typeof source!=='object')return {operator:'and',conditions:[scheduleEmptyCondition()],groups:[]};
 return {operator:source.operator==='or'?'or':'and',
  conditions:(Array.isArray(source.conditions)?source.conditions:[]).map(c=>({...scheduleEmptyCondition(c?.type),...(c||{})})),
  groups:(Array.isArray(source.groups)?source.groups:[]).map(scheduleNormalizeGroup)};
}
function scheduleCountConditions(group){
 return (group.conditions||[]).length+(group.groups||[]).reduce((count,child)=>count+scheduleCountConditions(child),0);
}
function scheduleSingleCondition(group){
 if(!group||scheduleCountConditions(group)!==1)return null;
 if(group.conditions?.length)return group.conditions[0];
 for(const child of group.groups||[]){
  const only=scheduleSingleCondition(child);
  if(only)return only;
 }
 return null;
}
function scheduleCanonicalGroup(group){
 // A one-condition expression has no meaningful Boolean operator.
 // Automatically collapse leftover nested wrappers after a removal.
 const only=scheduleSingleCondition(group);
 return only?{operator:'and',conditions:[only],groups:[]}:group;
}
function scheduleGroupParents(group){
 const set=new Set();
 const visit=g=>{for(const c of g.conditions||[])if(c.type==='dependency'&&c.parent_id)set.add(c.parent_id);
  for(const child of g.groups||[])visit(child)};
 visit(group);return [...set];
}
function scheduleExpressionEditor(host,{
 schedules=[],repositories=[],currentID='',onChange=()=>{},onRescanRepos=()=>{},preferredRepository=()=>null
}={}){
 const root=document.createElement('section');root.className='schedule-expression';
 host.append(root);
 let expr={operator:'and',conditions:[scheduleEmptyCondition()],groups:[]};
 const notify=()=>onChange(JSON.parse(JSON.stringify(expr)));
 const create=(tag,text,cls)=>{
  const node=document.createElement(tag);
  if(text!==undefined)node.textContent=text;
  if(cls)node.className=cls;
  return node;
 };
 const picker=(pairs,value,change)=>{
  const select=create('select');
  for(const [id,label] of pairs){const option=create('option',label);option.value=id;select.append(option)}
  select.value=value||pairs[0]?.[0]||'';select.onchange=()=>change(select.value);
  return select;
 };
 function inputField(parent,title,value,changed,{options=null,number=false,placeholder='',list=null}={}){
  const label=create('label',undefined,'expr-field');
  label.append(create('span',title));
  const node=options?picker(options,String(value??''),changed):create('input');
  if(!options){
   node.type=number?'number':'text';node.value=value==null?'':String(value);node.placeholder=placeholder;
   node.oninput=()=>changed(number?(node.value===''?null:Number(node.value)):node.value);
   if(list)node.setAttribute('list',list);
  }
  label.append(node);parent.append(label);
  return node;
 }
 function renderLeaf(container,condition,index,group,depth){
  const leaf=create('section',undefined,'expr-leaf');
  const header=create('header');header.append(create('strong','Condition '+(index+1)));
  const kind=picker(scheduleExprTypes,condition.type,value=>{
   // Reset irrelevant fields when changing trigger kind, retaining leaf ID.
   const id=condition.id;
   const next={...scheduleEmptyCondition(value),id};
   if(value==='git'){
    const suggested=preferredRepository();
    if(suggested?.path)next.git_path=suggested.path;
   }
   group.conditions[index]=next;render();notify();
  });
  header.append(kind);
  const remove=create('button','Remove');remove.type='button';
  remove.disabled=scheduleCountConditions(expr)<=1;
  remove.onclick=()=>{
   if(scheduleCountConditions(expr)<=1)return;
   group.conditions.splice(index,1);
   pruneEmptyGroups(expr);expr=scheduleCanonicalGroup(expr);
   render();notify();
  };header.append(remove);
  leaf.append(header);
  const bind=(name,title,opts={})=>inputField(leaf,title,condition[name],value=>{condition[name]=value;notify()},opts);
  if(condition.type==='cron'){
   bind('cron','Cron expression (5 fields)',{placeholder:'*/5 * * * *'});
   bind('timezone','Timezone (IANA or Local)',{placeholder:'Asia/Ho_Chi_Minh'});
  }else if(condition.type==='url'){
   bind('interval_seconds','Interval (seconds, min 30)',{number:true});
   bind('url','URL (public HTTP/S)',{placeholder:'https://example.com/api/status'});
   bind('match_mode','Match mode',{options:[['contains','Contains text/keyword'],['equals','Exact text'],['json','JSON dot path value'],['xml','XML element path text']]});
   bind('json_path','JSON dot path / XML element path');
   bind('match_text','Match value');
  }else if(condition.type==='git'){
   bind('interval_seconds','Interval (seconds, min 30)',{number:true});
   const optionsID='taskdeck-schedule-expression-repos';
   const gitPath=bind('git_path','Git root (relative path)',{placeholder:'. or services/api',list:optionsID});
   const gitControls=create('div',undefined,'expr-git-actions');
   const choose=picker([['','Choose repository…'],...repositories.map(repo=>[repo.path,(repo.name||repo.id)+' · '+repo.path])],
    condition.git_path,value=>{
     if(!value)return;
     condition.git_path=value;gitPath.value=value;notify();
    });
   choose.className='expr-git-repositories';
   choose.title='Select a discovered Git repository or enter a relative path above';
   gitControls.append(choose);
   const scan=create('button','↻ Rescan');scan.type='button';
   scan.onclick=()=>{Promise.resolve(onRescanRepos()).catch(error=>{console.warn('Schedule Git repository rescan failed',error)})};
   gitControls.append(scan);leaf.append(gitControls);
   bind('git_ref','Git ref',{placeholder:'HEAD'});
   bind('git_condition','Git trigger',{options:[['change','New commit SHA'],['dirty','Tracked worktree dirty'],['clean','Tracked worktree clean']]});
  }else if(condition.type==='dependency'){
   bind('parent_id','Depends on schedule',{options:[['','Choose parent schedule'],...schedules.filter(s=>s.id!==currentID).map(s=>[s.id,s.name])]});
   bind('parent_state','Parent result',{options:[['success','Success'],['failure','Failure'],['any','Any']]});
   bind('exit_code','Exit code (leave empty for any)',{number:true});
   bind('output_contains','Console must contain text');
   bind('output_absent','Console must not contain text');
  }
  container.append(leaf);
 }
 function renderGroup(group,host,depth){
  const branch=create('section',undefined,'expr-group');
  const header=create('div',undefined,'expr-heading');
  header.append(create('strong',depth===0?'Trigger conditions':'Nested condition group'));
  const op=picker([['and','ALL / AND'],['or','ANY / OR']],group.operator,value=>{group.operator=value;notify()});
  op.setAttribute('aria-label',depth===0?'Root AND OR operator':'Nested AND OR operator');
  op.disabled=scheduleCountConditions(expr)===1;
  header.append(op);
  const addLeaf=create('button','+ Condition');addLeaf.type='button';
  addLeaf.onclick=()=>{
   if(scheduleCountConditions(expr)===1)expr.operator='and';
   group.conditions.push(scheduleEmptyCondition());render();notify();
  };
  header.append(addLeaf);
  if(depth<12){
   const addGroup=create('button','+ Group');addGroup.type='button';
   addGroup.onclick=()=>{
    if(scheduleCountConditions(expr)===1)expr.operator='and';
    group.groups.push({operator:'or',conditions:[scheduleEmptyCondition()],groups:[]});
    render();notify();
   };
   header.append(addGroup);
  }
  if(depth>0){
   const remove=create('button','Remove group');remove.type='button';
   remove.disabled=scheduleCountConditions(expr)<=scheduleCountConditions(group);
   remove.onclick=()=>{
    if(scheduleCountConditions(expr)<=scheduleCountConditions(group))return;
    const parent=findParent(expr,group);
    if(parent){
     parent.groups.splice(parent.groups.indexOf(group),1);
     pruneEmptyGroups(expr);expr=scheduleCanonicalGroup(expr);
     render();notify();
    }
   };
   header.append(remove);
  }
  branch.append(header);
  group.conditions.forEach((c,i)=>renderLeaf(branch,c,i,group,depth));
  group.groups.forEach(child=>renderGroup(child,branch,depth+1));
  if(!group.conditions.length&&!group.groups.length)branch.append(create('div','Empty group — add a condition before saving','expr-note'));
  host.append(branch);
 }
 function pruneEmptyGroups(group){
  group.groups=(group.groups||[]).filter(child=>{
   pruneEmptyGroups(child);
   return scheduleCountConditions(child)>0;
  });
 }
 function findParent(parent,target){
  if(parent.groups.includes(target))return parent;
  for(const child of parent.groups){const found=findParent(child,target);if(found)return found}
  return null;
 }
 function render(){
  root.replaceChildren();
  const count=scheduleCountConditions(expr);
  root.append(create('div',count===1
   ?'Single condition · Use + Condition or + Group to combine triggers.'
   :'Multiple conditions ('+count+') · AND waits for all signals; OR starts after any signal.','expr-note'));
  const list=create('datalist');list.id='taskdeck-schedule-expression-repos';
  for(const repo of repositories){const option=create('option');option.value=repo.path;option.label=(repo.name||repo.id||repo.path);list.append(option)}
  root.append(list);
  renderGroup(expr,root,0);
 }
 render();
 return {
  load(group,schedulesNext,reposNext,currentNext){
   expr=scheduleCanonicalGroup(scheduleNormalizeGroup(group));
   if(schedulesNext)schedules=schedulesNext;
   if(reposNext)repositories=reposNext;
   currentID=currentNext||'';
   render();
  },
  value(){return JSON.parse(JSON.stringify(expr));},
  singleCondition(){return scheduleSingleCondition(expr);},
  count(){return scheduleCountConditions(expr);},
  parents(){return scheduleGroupParents(expr);},
  setRepositories(nextRepos){
   repositories=Array.isArray(nextRepos)?nextRepos:[];
   const list=root.querySelector('datalist');
   if(list){
    list.replaceChildren();
    for(const repo of repositories){
     const option=create('option');option.value=repo.path;option.label=repo.name||repo.id||repo.path;list.append(option);
    }
   }
   // Discovery can finish after a Git condition has already been opened.
   // Refresh just the picker options, not the entire editor: preserve all
   // currently typed values and open browser autocomplete lists.
   for(const select of root.querySelectorAll?.('.expr-git-repositories')||[]){
    const selected=select.value;
    select.replaceChildren();
    for(const [value,title] of [['','Choose repository…'],...repositories.map(repo=>
     [repo.path,(repo.name||repo.id)+' · '+repo.path])]){
     const option=create('option',title);option.value=value;select.append(option);
    }
    select.value=repositories.some(repo=>repo.path===selected)?selected:'';
   }
  },
  refreshOptions(nextSchedules,nextRepos,id){
   schedules=nextSchedules||schedules;repositories=nextRepos||repositories;currentID=id||currentID;
   // Rebuild option menus only when called explicitly (not on each poll).
   render();
  }
 };
}
globalThis.TaskMenuScheduleConditions={create:scheduleExpressionEditor,empty:scheduleEmptyCondition,
 legacy:scheduleLegacyGroup,normalize:scheduleNormalizeGroup,parents:scheduleGroupParents,
 count:scheduleCountConditions,single:scheduleSingleCondition,canonical:scheduleCanonicalGroup};
