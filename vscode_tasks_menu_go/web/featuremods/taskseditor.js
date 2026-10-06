const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for tasks.json editor');

const PRIMARY_TASKS_PATH='.vscode/tasks.json';
const WORKSPACE_TOKEN='$'+'{workspaceFolder}';

const style=document.createElement('style');
style.textContent=`
.tasks-editor-overlay{position:fixed;inset:0;z-index:15500;display:none;align-items:stretch;justify-content:center;background:rgba(0,0,0,.62);padding:22px}
.tasks-editor-overlay.visible{display:flex}
.tasks-editor-dialog{width:min(1180px,98vw);height:min(880px,95vh);display:flex;flex-direction:column;background:#15191f;border:1px solid #48515f;border-radius:10px;box-shadow:0 18px 55px rgba(0,0,0,.55);overflow:hidden}
.tasks-editor-head{display:flex;align-items:center;gap:8px;padding:10px 12px;border-bottom:1px solid #30343b}
.tasks-editor-head strong{font-size:13px}.tasks-editor-head .spacer{flex:1}.tasks-editor-head .dirty{font-size:10px;opacity:.7}
.tasks-editor-root{max-width:220px;min-width:120px;height:25px;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;font:11px ui-monospace,monospace}
.tasks-editor-mode{display:flex;gap:4px}.tasks-editor-mode button.active{background:#244c70;border-color:#3f79a8}
.tasks-editor-body{display:grid;grid-template-columns:270px minmax(0,1fr);min-height:0;flex:1}
.tasks-editor-list{display:flex;flex-direction:column;min-height:0;border-right:1px solid #30343b}
.tasks-editor-list-tools{display:flex;gap:5px;padding:7px;border-bottom:1px solid #30343b}.tasks-editor-list-tools button{font-size:11px}
.tasks-editor-items{overflow:auto;min-height:0;padding:5px}.tasks-editor-group{margin:2px 0 5px}.tasks-editor-group-title{padding:5px 7px 3px;font-size:10px;font-weight:800;letter-spacing:.03em;opacity:.62;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.tasks-editor-group-children{margin-left:10px;padding-left:6px;border-left:1px solid #2b313b}.tasks-editor-item{display:block;width:100%;text-align:left;border:0;background:transparent;padding:7px 8px;border-radius:5px;font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.tasks-editor-item:hover{background:#202630}.tasks-editor-item.active{background:#29384b}
.tasks-editor-main{min-width:0;min-height:0;overflow:auto;padding:12px}
.tasks-editor-form{display:grid;grid-template-columns:155px minmax(0,1fr);gap:8px 10px;align-items:center}
.tasks-editor-form label{font-size:11px;font-weight:700;opacity:.72}
.tasks-editor-form input,.tasks-editor-form select,.tasks-editor-form textarea{width:100%;box-sizing:border-box;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:7px;font:12px ui-monospace,monospace}
.tasks-editor-form textarea{min-height:72px;resize:vertical}
.tasks-editor-inline{display:flex;gap:6px;align-items:center}.tasks-editor-inline>*:first-child{flex:1;min-width:0}
.tasks-editor-section{grid-column:1/-1;margin-top:7px;padding-top:9px;border-top:1px solid #30343b;font-size:11px;font-weight:800}
.tasks-editor-exec-list{grid-column:1/-1;display:flex;flex-direction:column;gap:5px}.tasks-editor-exec-row{display:flex;gap:5px;align-items:center}.tasks-editor-exec-row code{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;padding:6px;background:#0d1117;border:1px solid #30343b;border-radius:4px}
.tasks-editor-help{grid-column:1/-1;font-size:10px;opacity:.66;line-height:1.45}
.tasks-editor-raw{width:100%;height:100%;min-height:520px;box-sizing:border-box;resize:none;background:#090c10;color:inherit;border:1px solid #30343b;border-radius:6px;padding:10px;font:12px/1.45 ui-monospace,monospace}
.tasks-editor-actions{display:flex;justify-content:flex-end;gap:7px;padding:9px 12px;border-top:1px solid #30343b}
.tasks-editor-save{background:#24472f;border-color:#3b7850}
.tasks-editor-warning{padding:7px 9px;margin-bottom:10px;border:1px solid #755d2b;background:#302714;border-radius:5px;font-size:10px}
html[data-taskmenu-theme="light"] .tasks-editor-dialog{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .tasks-editor-root,html[data-taskmenu-theme="light"] .tasks-editor-form input,html[data-taskmenu-theme="light"] .tasks-editor-form select,html[data-taskmenu-theme="light"] .tasks-editor-form textarea,html[data-taskmenu-theme="light"] .tasks-editor-exec-row code,html[data-taskmenu-theme="light"] .tasks-editor-raw{background:#f7f9fb;border-color:#b9c0c8;color:#202124}
html[data-taskmenu-theme="light"] .tasks-editor-item:hover{background:#edf1f5}html[data-taskmenu-theme="light"] .tasks-editor-item.active{background:#dce8f4}html[data-taskmenu-theme="light"] .tasks-editor-group-children{border-color:#dfe3e8}
`;
document.head.append(style);

const templates=[
  {id:'custom',label:'Custom task'},
  {id:'bash',label:'Bash script',type:'shell',command:'bash',runner:'bash'},
  {id:'python',label:'Python script',type:'shell',command:'python3',runner:'python3'},
  {id:'node',label:'Node.js script',type:'shell',command:'node',runner:'node'},
  {id:'go-run',label:'Go · run package',type:'shell',command:'go',args:['run','.']},
  {id:'go-test',label:'Go · test all',type:'shell',command:'go',args:['test','./...']},
  {id:'go-build',label:'Go · build all',type:'shell',command:'go',args:['build','./...']},
  {id:'rust-run',label:'Rust · cargo run',type:'shell',command:'cargo',args:['run']},
  {id:'rust-test',label:'Rust · cargo test',type:'shell',command:'cargo',args:['test']},
  {id:'make',label:'Make',type:'shell',command:'make',args:[]},
  {id:'cmake-build',label:'CMake · build',type:'shell',command:'cmake',args:['--build','build']},
  {id:'gradle-build',label:'Gradle · build',type:'shell',command:'./gradlew',args:['build']},
  {id:'maven-test',label:'Maven · test',type:'shell',command:'mvn',args:['test']},
  {id:'docker-compose',label:'Docker Compose · up',type:'shell',command:'docker',args:['compose','up','--build']}
];
const templateByID=new Map(templates.map(item=>[item.id,item]));

let overlay=null,list=null,main=null,rawArea=null,saveButton=null,dirtyBadge=null,pathNode=null,rootSelect=null;
let visualButton=null,rawButton=null;
let fileMeta=null,doc=null,selectedIndex=0,mode='visual',dirty=false;
let workspaceRoots=[],selectedRootID='primary';

function canRead(){return app.hasPermission?.('files.read')!==false;}
function canWrite(){return app.hasPermission?.('files.write')!==false;}

function tasksRootBase(root){
  return root?.primary?'':'@root/'+String(root?.id||'');
}
function selectedWorkspaceRoot(){
  return workspaceRoots.find(root=>String(root?.id||'')===selectedRootID)||workspaceRoots.find(root=>root?.primary)||null;
}
function tasksPathForRoot(root){
  const base=tasksRootBase(root);
  return base?base+'/'+PRIMARY_TASKS_PATH:PRIMARY_TASKS_PATH;
}
function currentTasksPath(){return tasksPathForRoot(selectedWorkspaceRoot());}
function currentTasksDir(){
  const path=currentTasksPath();
  const index=path.lastIndexOf('/');
  return index<0?'':path.slice(0,index);
}
async function loadWorkspaceRoots(){
  const data=await app.jsonFetch('/api/workspace-roots');
  workspaceRoots=(Array.isArray(data?.roots)?data.roots:[]).filter(root=>root?.available!==false);
  if(!workspaceRoots.some(root=>String(root?.id||'')===selectedRootID)){
    selectedRootID=String(workspaceRoots.find(root=>root?.primary)?.id||workspaceRoots[0]?.id||'primary');
  }
  if(rootSelect){
    rootSelect.replaceChildren();
    for(const root of workspaceRoots){
      const option=document.createElement('option');option.value=String(root.id||'');option.textContent=String(root.name||root.id||'Workspace')+(root.primary?' · primary':'');rootSelect.append(option);
    }
    rootSelect.value=selectedRootID;
  }
}
async function projectTree(path){
  return app.jsonFetch('/api/project/tree?path='+encodeURIComponent(path||''));
}
async function currentTasksFileExists(){
  const root=selectedWorkspaceRoot();if(!root)return false;
  const base=tasksRootBase(root);
  const rootItems=await projectTree(base);
  const vscode=(Array.isArray(rootItems)?rootItems:[]).find(item=>item?.name==='.vscode'&&item?.type==='dir');
  if(!vscode)return false;
  const dir=base?base+'/.vscode':'.vscode';
  const items=await projectTree(dir);
  return (Array.isArray(items)?items:[]).some(item=>item?.name==='tasks.json'&&item?.type==='file');
}
async function ensureCurrentTasksFile(){
  if(!fileMeta?.missing)return;
  const root=selectedWorkspaceRoot();if(!root)throw new Error('Workspace root is unavailable');
  const base=tasksRootBase(root);
  const rootItems=await projectTree(base);
  const hasVSCode=(Array.isArray(rootItems)?rootItems:[]).some(item=>item?.name==='.vscode'&&item?.type==='dir');
  if(!hasVSCode){
    await app.jsonFetch('/api/project/mutate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'mkdir',path:currentTasksDir()})});
  }
  await app.jsonFetch('/api/project/mutate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'create_file',path:currentTasksPath()})});
  fileMeta=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(currentTasksPath()));
}

function stripJSONC(text){
  const src=String(text||'');let out='';let string=false,escape=false,line=false,block=false;
  for(let i=0;i<src.length;i++){
    const ch=src[i],next=src[i+1]||'';
    if(line){if(ch==='\n'||ch==='\r'){line=false;out+=ch;}continue;}
    if(block){if(ch==='*'&&next==='/'){block=false;i++;}else if(ch==='\n'||ch==='\r')out+=ch;continue;}
    if(string){
      out+=ch;
      if(escape)escape=false;
      else if(ch==='\\')escape=true;
      else if(ch==='"')string=false;
      continue;
    }
    if(ch==='"'){string=true;out+=ch;continue;}
    if(ch==='/'&&next==='/'){line=true;i++;continue;}
    if(ch==='/'&&next==='*'){block=true;i++;continue;}
    out+=ch;
  }
  out=out.replace(/,(\s*[}\]])/g,'$1');
  return out;
}

function parseTasksDocument(text){
  const value=JSON.parse(stripJSONC(text));
  if(!value||typeof value!=='object'||Array.isArray(value))throw new Error('tasks.json root must be an object');
  if(!Array.isArray(value.tasks))value.tasks=[];
  if(!value.version)value.version='2.0.0';
  return value;
}

function markDirty(){
  dirty=true;
  if(dirtyBadge)dirtyBadge.textContent='Unsaved changes';
  if(saveButton)saveButton.disabled=!canWrite();
}

function clearDirty(){
  dirty=false;
  if(dirtyBadge)dirtyBadge.textContent='';
  if(saveButton)saveButton.disabled=!canWrite();
}

function ensureTaskdeckMeta(task){
  if(!task.taskdeck||typeof task.taskdeck!=='object'||Array.isArray(task.taskdeck))task.taskdeck={};
  return task.taskdeck;
}

function ensureWorkflowMeta(task){
  const meta=ensureTaskdeckMeta(task);
  if(!meta.workflow||typeof meta.workflow!=='object'||Array.isArray(meta.workflow))meta.workflow={};
  return meta.workflow;
}
function commaList(value){
  if(Array.isArray(value))return value.map(item=>String(item||'').trim()).filter(Boolean);
  if(typeof value==='string')return value.split(',').map(item=>item.trim()).filter(Boolean);
  return [];
}
function setDependsOn(task,value){
  const items=commaList(value);
  if(items.length===0)delete task.dependsOn;
  else if(items.length===1)task.dependsOn=items[0];
  else task.dependsOn=items;
}
function workflowTaskLabels(except=''){
  return (doc?.tasks||[]).map(task=>String(task?.label||'').trim()).filter(label=>label&&label!==except);
}
function taskDependsOn(task){return commaList(task?.dependsOn);}
function setWorkflowNumber(workflow,key,value,max){
  const parsed=Math.max(0,Math.min(max,Number.parseInt(String(value||'0'),10)||0));
  if(parsed>0)workflow[key]=parsed;else delete workflow[key];
}
function cleanupWorkflow(task){
  const meta=task?.taskdeck;if(!meta?.workflow)return;
  if(Object.keys(meta.workflow).length===0)delete meta.workflow;
  if(meta&&Object.keys(meta).length===0)delete task.taskdeck;
}
function renderWorkflowFields(form,task){
  const section=document.createElement('div');section.className='tasks-editor-section';section.textContent='WORKFLOW / DAG';form.append(section);
  const help=document.createElement('div');help.className='tasks-editor-help';
  help.textContent='dependsOn uses task labels. TaskDeck validates missing dependencies/cycles before launch. Workflow options are stored under taskdeck.workflow.';
  form.append(help);

  const deps=makeInput(taskDependsOn(task).join(', '),value=>setDependsOn(task,value));
  const dataList=document.createElement('datalist');dataList.id='tasks-editor-deps-'+Math.random().toString(36).slice(2);
  for(const label of workflowTaskLabels(task.label)){const option=document.createElement('option');option.value=label;dataList.append(option);}
  deps.setAttribute('list',dataList.id);deps.placeholder='Build, Test';
  const depsWrap=document.createElement('div');depsWrap.append(deps,dataList);addField(form,'Depends on',depsWrap);

  const order=document.createElement('select');
  for(const [value,label] of [['parallel','Parallel dependencies'],['sequence','Sequential dependencies']]){const option=document.createElement('option');option.value=value;option.textContent=label;order.append(option);}
  order.value=String(task.dependsOrder||'').toLowerCase()==='sequence'?'sequence':'parallel';
  order.onchange=()=>{if(order.value==='sequence')task.dependsOrder='sequence';else delete task.dependsOrder;markDirty();};
  addField(form,'Dependency order',order);

  const workflow=ensureWorkflowMeta(task);
  const retry=makeInput(String(workflow.retry||''),value=>{setWorkflowNumber(workflow,'retry',value,10);cleanupWorkflow(task);});
  retry.type='number';retry.min='0';retry.max='10';retry.placeholder='0';addField(form,'Retry count',retry);

  const timeout=makeInput(String(workflow.timeoutSeconds||''),value=>{setWorkflowNumber(workflow,'timeoutSeconds',value,86400);cleanupWorkflow(task);});
  timeout.type='number';timeout.min='0';timeout.max='86400';timeout.placeholder='0';addField(form,'Timeout seconds',timeout);

  const condition=document.createElement('select');
  for(const [value,label] of [['','Success (default)'],['success','Success'],['failure','Failure'],['always','Always']]){const option=document.createElement('option');option.value=value;option.textContent=label;condition.append(option);}
  condition.value=String(workflow.condition||'').toLowerCase();
  condition.onchange=()=>{if(condition.value)workflow.condition=condition.value;else delete workflow.condition;cleanupWorkflow(task);markDirty();};
  addField(form,'Condition',condition);

  const continueWrap=document.createElement('label');continueWrap.className='tasks-editor-inline';
  const continueBox=document.createElement('input');continueBox.type='checkbox';continueBox.checked=Boolean(workflow.continueOnError);
  const continueText=document.createElement('span');continueText.textContent='Continue workflow when this task exits non-zero';
  continueBox.onchange=()=>{if(continueBox.checked)workflow.continueOnError=true;else delete workflow.continueOnError;cleanupWorkflow(task);markDirty();};
  continueWrap.append(continueBox,continueText);addField(form,'Failure policy',continueWrap);

  const outputs=makeInput(commaList(workflow.outputs).join(', '),value=>{
    const values=commaList(value);if(values.length)workflow.outputs=values;else delete workflow.outputs;cleanupWorkflow(task);
  });
  outputs.placeholder='version, artifact';addField(form,'Declared outputs',outputs);

  const outputHelp=document.createElement('div');outputHelp.className='tasks-editor-help';
  outputHelp.textContent='Emit ::taskdeck-output name=value from a task, then reference it downstream as ${output:Task label.name}.';
  form.append(outputHelp);
}
function workflowGraphModel(){
  const tasks=doc?.tasks||[],byLabel=new Map(tasks.map(task=>[String(task?.label||'').trim(),task]));
  const nodes=[],edges=[];const problems=[];
  for(const task of tasks){
    const label=String(task?.label||'').trim();if(!label)continue;
    const deps=taskDependsOn(task);nodes.push({label,deps});
    for(const dep of deps){edges.push({from:dep,to:label});if(!byLabel.has(dep))problems.push(label+' → missing '+dep);}
  }
  const visiting=new Set(),visited=new Set();
  function visit(label){
    if(visited.has(label))return;
    if(visiting.has(label)){problems.push('Cycle detected at '+label);return;}
    visiting.add(label);for(const dep of taskDependsOn(byLabel.get(label)||{})){if(byLabel.has(dep))visit(dep);}
    visiting.delete(label);visited.add(label);
  }
  for(const node of nodes)visit(node.label);
  return {nodes,edges,problems};
}
function openWorkflowGraphPreview(){
  const model=workflowGraphModel();
  const dialog=document.createElement('dialog');dialog.style.cssText='width:min(860px,92vw);max-height:84vh;background:#171b22;color:inherit;border:1px solid #48515f;border-radius:8px;padding:12px';
  const title=document.createElement('h3');title.textContent='Task dependency graph';title.style.margin='0 0 8px';
  const summary=document.createElement('div');summary.style.cssText='font-size:11px;opacity:.7;margin-bottom:8px';summary.textContent=model.nodes.length+' task(s) · '+model.edges.length+' dependency edge(s)';
  const pre=document.createElement('pre');pre.style.cssText='max-height:58vh;overflow:auto;white-space:pre-wrap';
  const roots=model.nodes.filter(node=>node.deps.length===0).map(node=>node.label);
  const lines=['Roots: '+(roots.join(', ')||'(none)'),''];
  for(const node of model.nodes){lines.push((node.deps.length?node.deps.join(' + ')+' → ':'')+node.label);}
  if(model.problems.length){lines.push('','PROBLEMS:');for(const problem of model.problems)lines.push('! '+problem);}
  pre.textContent=lines.join('\n');
  const close=document.createElement('button');close.type='button';close.textContent='Close';close.onclick=()=>dialog.close();
  dialog.append(title,summary,pre,close);document.body.append(dialog);dialog.addEventListener('close',()=>dialog.remove(),{once:true});dialog.showModal();
}

function shellQuote(value){
  const text=String(value||'');
  if(/^[A-Za-z0-9_./:@+-]+$/.test(text))return text;
  return "'"+text.replaceAll("'","'\\\"'\\\"'")+"'"; 
}

function executionRunner(task){
  const meta=task?.taskdeck&&typeof task.taskdeck==='object'?task.taskdeck:{};
  if(typeof meta.runner==='string')return meta.runner.trim();
  return templateByID.get(String(meta.template||''))?.runner||'';
}

function rebuildMultiExecutionCommand(task){
  const meta=ensureTaskdeckMeta(task);
  const files=Array.isArray(meta.executionFiles)?meta.executionFiles.filter(Boolean):[];
  if(!files.length)return;
  const runner=executionRunner(task);
  const commands=files.map(path=>(runner?runner+' ':'')+shellQuote(path));
  task.type='shell';
  task.command=commands.join(meta.executionMode==='continue'?'; ':' && ');
  task.args=[];
}

function taskLabel(task,index){
  return String(task?.menuLabel||task?.label||('Task '+(index+1)));
}

function taskMenuGroupParts(task){
  const value=String(task?.menuGroup||'').trim();
  if(!value)return [];
  return value.split('/').map(part=>part.trim()).filter(Boolean);
}

function buildTaskEditorTree(){
  const root={children:new Map(),tasks:[]};
  (doc?.tasks||[]).forEach((task,index)=>{
    const group=taskMenuGroupParts(task);
    if(group.length===1&&group[0]==='build'){root.tasks.push({task,index});return;}
    let node=root;
    for(const name of group){
      if(!node.children.has(name))node.children.set(name,{children:new Map(),tasks:[]});
      node=node.children.get(name);
    }
    node.tasks.push({task,index});
  });
  return root;
}

function selectTask(index){
  selectedIndex=index;
  if(mode==='raw'&&rawArea){
    updateList();
    focusRawTask(index);
    return;
  }
  render();
}

function appendTaskEditorTree(node,host){
  for(const entry of node.tasks){
    const button=document.createElement('button');button.type='button';button.className='tasks-editor-item'+(entry.index===selectedIndex?' active':'');
    button.textContent=taskLabel(entry.task,entry.index);button.title=String(entry.task?.label||button.textContent);
    button.onclick=()=>selectTask(entry.index);
    host.append(button);
  }
  for(const [name,child] of node.children){
    const group=document.createElement('div');group.className='tasks-editor-group';
    const title=document.createElement('div');title.className='tasks-editor-group-title';title.textContent=name;title.title=name;
    const children=document.createElement('div');children.className='tasks-editor-group-children';
    appendTaskEditorTree(child,children);group.append(title,children);host.append(group);
  }
}

function updateList(){
  list.replaceChildren();
  appendTaskEditorTree(buildTaskEditorTree(),list);
}

function menuGroupOptions(){
  const values=new Set();
  for(const task of doc?.tasks||[]){
    const parts=taskMenuGroupParts(task);
    for(let i=1;i<=parts.length;i++)values.add(parts.slice(0,i).join('/'));
  }
  return [...values].sort((a,b)=>a.localeCompare(b));
}

function makeMenuGroupInput(task){
  const wrap=document.createElement('div');
  const input=makeInput(task.menuGroup||'',value=>{const group=value.trim();if(group)task.menuGroup=group;else delete task.menuGroup;});
  const dataList=document.createElement('datalist');dataList.id='tasks-editor-menu-groups-'+Math.random().toString(36).slice(2);
  for(const value of menuGroupOptions()){const option=document.createElement('option');option.value=value;dataList.append(option);}
  input.setAttribute('list',dataList.id);input.placeholder='Group/Subgroup';
  wrap.append(input,dataList);return wrap;
}

function skipJSONTrivia(text,index){
  let i=index;
  while(i<text.length){
    if(/\s/.test(text[i])){i++;continue;}
    if(text[i]==='/'&&text[i+1]==='/'){
      i+=2;while(i<text.length&&text[i]!=='\n'&&text[i]!=='\r')i++;continue;
    }
    if(text[i]==='/'&&text[i+1]==='*'){
      i+=2;while(i+1<text.length&&!(text[i]==='*'&&text[i+1]==='/'))i++;
      if(i+1<text.length)i+=2;continue;
    }
    break;
  }
  return i;
}

function readJSONString(text,index){
  if(text[index]!=='"')return null;
  let i=index+1,value='';
  while(i<text.length){
    const ch=text[i];
    if(ch==='\\'){
      if(i+1>=text.length)return null;
      value+=text.slice(i,i+2);i+=2;continue;
    }
    if(ch==='"')return {end:i+1,raw:text.slice(index,i+1),value};
    value+=ch;i++;
  }
  return null;
}

function findTasksArrayStart(text){
  let i=0;
  while(i<text.length){
    i=skipJSONTrivia(text,i);
    if(i>=text.length)break;
    if(text[i]==='"'){
      const token=readJSONString(text,i);if(!token)return -1;
      const key=token.raw;
      i=skipJSONTrivia(text,token.end);
      if(key==='"tasks"'&&text[i]===':'){
        i=skipJSONTrivia(text,i+1);
        return text[i]==='['?i:-1;
      }
      continue;
    }
    i++;
  }
  return -1;
}

function findRawTaskRange(text,targetIndex){
  const arrayStart=findTasksArrayStart(text);if(arrayStart<0)return null;
  let i=arrayStart+1,index=0;
  while(i<text.length){
    i=skipJSONTrivia(text,i);
    if(text[i]===']')break;
    if(text[i]!== '{'){i++;continue;}
    const start=i;let depth=0,string=false,escape=false,line=false,block=false;
    for(;i<text.length;i++){
      const ch=text[i],next=text[i+1]||'';
      if(line){if(ch==='\n'||ch==='\r')line=false;continue;}
      if(block){if(ch==='*'&&next==='/'){block=false;i++;}continue;}
      if(string){
        if(escape)escape=false;
        else if(ch==='\\')escape=true;
        else if(ch==='"')string=false;
        continue;
      }
      if(ch==='"'){string=true;continue;}
      if(ch==='/'&&next==='/'){line=true;i++;continue;}
      if(ch==='/'&&next==='*'){block=true;i++;continue;}
      if(ch==='{')depth++;
      else if(ch==='}'){
        depth--;
        if(depth===0){
          const end=i+1;
          if(index===targetIndex)return {start,end};
          index++;i=end;break;
        }
      }
    }
  }
  return null;
}

function focusRawTask(index){
  if(!rawArea)return;
  const range=findRawTaskRange(rawArea.value,index);
  if(!range)return;
  rawArea.focus();
  rawArea.setSelectionRange(range.start,range.end);
}

function makeInput(value,onchange,{type='text'}={}){
  const input=document.createElement('input');input.type=type;input.value=value??'';
  input.oninput=()=>{onchange(input.value);markDirty();updateList();};
  return input;
}

function makeTextarea(value,onchange){
  const area=document.createElement('textarea');area.value=value??'';
  area.oninput=()=>{onchange(area.value);markDirty();};
  return area;
}

function addField(form,labelText,node){
  const label=document.createElement('label');label.textContent=labelText;form.append(label,node);
}

function parseArgsText(text){
  return String(text||'').split('\n').map(line=>line.trim()).filter(Boolean);
}

function envToText(task){
  const env=task?.options?.env;
  if(!env||typeof env!=='object'||Array.isArray(env))return '';
  return Object.entries(env).map(([key,value])=>key+'='+String(value??'')).join('\n');
}

function setEnvFromText(task,text){
  if(!task.options||typeof task.options!=='object'||Array.isArray(task.options))task.options={};
  const env={};
  for(const line of String(text||'').split(/\r?\n/)){
    const trimmed=line.trim();if(!trimmed)continue;
    const at=trimmed.indexOf('=');
    if(at<=0)continue;
    env[trimmed.slice(0,at).trim()]=trimmed.slice(at+1);
  }
  if(Object.keys(env).length)task.options.env=env;else delete task.options.env;
}

function setCWD(task,value){
  const text=String(value||'').trim();
  if(!task.options||typeof task.options!=='object'||Array.isArray(task.options))task.options={};
  if(text)task.options.cwd=text;else delete task.options.cwd;
}

async function pickWorkspacePath(title){
  const browser=globalThis.TaskMenuDirectoryBrowser;
  if(typeof browser?.pickFile!=='function')throw new Error('Workspace file picker is unavailable');
  return browser.pickFile({title,label:'File relative to the workspace:',confirm:'Use file'});
}

function renderExecutionFiles(form,task){
  const meta=ensureTaskdeckMeta(task);
  if(!Array.isArray(meta.executionFiles))meta.executionFiles=[];
  const section=document.createElement('div');section.className='tasks-editor-section';section.textContent='MULTI-FILE / SCRIPT EXECUTION';form.append(section);
  const help=document.createElement('div');help.className='tasks-editor-help';help.textContent='Files are executed in one shell task. Sequence mode stops on the first failure by default. The generated command stays compatible with the normal TaskDeck task runner.';form.append(help);

  const runner=makeInput(executionRunner(task),value=>{meta.runner=value;rebuildMultiExecutionCommand(task);});
  addField(form,'Runner',runner);
  const modeSelect=document.createElement('select');
  for(const [value,label] of [['stop','Sequence · stop on failure (&&)'],['continue','Sequence · continue on failure (;)']]){
    const option=document.createElement('option');option.value=value;option.textContent=label;modeSelect.append(option);
  }
  modeSelect.value=meta.executionMode==='continue'?'continue':'stop';
  modeSelect.onchange=()=>{meta.executionMode=modeSelect.value;rebuildMultiExecutionCommand(task);markDirty();render();};
  addField(form,'Execution mode',modeSelect);

  const files=document.createElement('div');files.className='tasks-editor-exec-list';
  meta.executionFiles.forEach((path,index)=>{
    const row=document.createElement('div');row.className='tasks-editor-exec-row';
    const code=document.createElement('code');code.textContent=path;code.title=path;
    const browse=document.createElement('button');browse.type='button';browse.textContent='Browse…';browse.onclick=async()=>{
      const picked=await pickWorkspacePath('Choose execution file');if(!picked)return;
      meta.executionFiles[index]=picked;rebuildMultiExecutionCommand(task);markDirty();render();
    };
    const remove=document.createElement('button');remove.type='button';remove.textContent='Remove';remove.onclick=()=>{
      meta.executionFiles.splice(index,1);rebuildMultiExecutionCommand(task);markDirty();render();
    };
    row.append(code,browse,remove);files.append(row);
  });
  const add=document.createElement('button');add.type='button';add.textContent='+ Add execution file';add.onclick=async()=>{
    const picked=await pickWorkspacePath('Choose execution file');if(!picked)return;
    meta.executionFiles.push(picked);rebuildMultiExecutionCommand(task);markDirty();render();
  };
  files.append(add);form.append(files);
}

function applyTemplate(task,id){
  const template=templateByID.get(id)||templateByID.get('custom');
  const meta=ensureTaskdeckMeta(task);meta.template=id;
  if(id==='custom'){markDirty();render();return;}
  task.type=template.type||'shell';
  task.command=template.command||'';
  task.args=Array.isArray(template.args)?[...template.args]:[];
  if(template.runner!==undefined)meta.runner=template.runner;
  if(!Array.isArray(meta.executionFiles))meta.executionFiles=[];
  if(meta.executionFiles.length)rebuildMultiExecutionCommand(task);
  markDirty();render();
}

function renderVisual(){
  main.replaceChildren();
  const tasks=doc?.tasks||[];
  if(!tasks.length){
    const empty=document.createElement('div');empty.textContent='No tasks. Use + Add task or a template to create one.';empty.style.opacity='.65';main.append(empty);return;
  }
  selectedIndex=Math.max(0,Math.min(selectedIndex,tasks.length-1));
  const task=tasks[selectedIndex];
  const warning=document.createElement('div');warning.className='tasks-editor-warning';
  warning.textContent='Visual save preserves task fields but normalizes tasks.json formatting and removes JSONC comments. Use Raw JSON when comment-preserving edits are required.';
  main.append(warning);
  const form=document.createElement('div');form.className='tasks-editor-form';

  const template=document.createElement('select');
  for(const item of templates){const option=document.createElement('option');option.value=item.id;option.textContent=item.label;template.append(option);}
  template.value=String(task?.taskdeck?.template||'custom');template.onchange=()=>applyTemplate(task,template.value);
  addField(form,'Template',template);

  addField(form,'Label',makeInput(task.label||'',value=>task.label=value));
  addField(form,'Menu label',makeInput(task.menuLabel||'',value=>{if(value)task.menuLabel=value;else delete task.menuLabel;}));
  addField(form,'Menu group',makeMenuGroupInput(task));
  addField(form,'Detail',makeInput(task.detail||'',value=>{if(value)task.detail=value;else delete task.detail;}));

  const type=document.createElement('select');
  for(const value of ['shell','process']){const option=document.createElement('option');option.value=value;option.textContent=value;type.append(option);}
  type.value=task.type==='process'?'process':'shell';type.onchange=()=>{task.type=type.value;markDirty();};
  addField(form,'Type',type);

  const commandInput=makeInput(typeof task.command==='string'?task.command:'',value=>{task.command=value;});
  const commandWrap=document.createElement('div');commandWrap.className='tasks-editor-inline';
  const browseCommand=document.createElement('button');browseCommand.type='button';browseCommand.textContent='Browse…';browseCommand.onclick=async()=>{
    const picked=await pickWorkspacePath('Choose command / executable');if(!picked)return;
    commandInput.value=WORKSPACE_TOKEN+'/'+picked;task.command=commandInput.value;markDirty();
  };
  commandWrap.append(commandInput,browseCommand);addField(form,'Command',commandWrap);

  const args=makeTextarea(Array.isArray(task.args)?task.args.map(value=>typeof value==='object'&&value?value.value??JSON.stringify(value):String(value)).join('\n'):'',value=>task.args=parseArgsText(value));
  const argsWrap=document.createElement('div');argsWrap.append(args);
  const addFileArg=document.createElement('button');addFileArg.type='button';addFileArg.textContent='+ File argument…';addFileArg.style.marginTop='5px';addFileArg.onclick=async()=>{
    const picked=await pickWorkspacePath('Choose file argument');if(!picked)return;
    const values=parseArgsText(args.value);values.push(picked);args.value=values.join('\n');task.args=values;markDirty();
  };
  argsWrap.append(addFileArg);addField(form,'Arguments',argsWrap);

  addField(form,'Working dir',makeInput(task?.options?.cwd||'',value=>setCWD(task,value)));
  addField(form,'Environment',makeTextarea(envToText(task),value=>setEnvFromText(task,value)));

  renderWorkflowFields(form,task);
  renderExecutionFiles(form,task);

  const advanced=document.createElement('div');advanced.className='tasks-editor-help';
  advanced.textContent='Advanced VS Code fields such as problemMatcher, presentation, inputs and custom extension fields remain in the document. Switch to Raw JSON to edit them directly.';
  form.append(advanced);
  main.append(form);
}

function renderRaw(){
  main.replaceChildren();
  rawArea=document.createElement('textarea');rawArea.className='tasks-editor-raw';
  rawArea.value=fileMeta?.content??JSON.stringify(doc,null,2)+'\n';
  rawArea.oninput=markDirty;
  main.append(rawArea);
  requestAnimationFrame(()=>focusRawTask(selectedIndex));
}

function render(){
  updateList();
  visualButton?.classList.toggle('active',mode==='visual');
  rawButton?.classList.toggle('active',mode==='raw');
  if(mode==='visual')renderVisual();else renderRaw();
}

function switchMode(next){
  if(next===mode)return;
  if(mode==='raw'&&rawArea){
    try{doc=parseTasksDocument(rawArea.value);fileMeta.content=rawArea.value;}
    catch(error){app.showError(error);return;}
  }else if(mode==='visual'){
    fileMeta.content=JSON.stringify(doc,null,2)+'\n';
  }
  mode=next;render();
}

function addTask(){
  const task={label:'New task',type:'shell',command:'echo',args:['Hello from TaskDeck'],menuGroup:'Tasks'};
  doc.tasks.push(task);selectedIndex=doc.tasks.length-1;markDirty();render();
}

function cloneTask(){
  const task=doc?.tasks?.[selectedIndex];if(!task)return;
  const copy=JSON.parse(JSON.stringify(task));copy.label=(copy.label||'Task')+' copy';
  if(copy.menuLabel)copy.menuLabel+=' copy';
  doc.tasks.splice(selectedIndex+1,0,copy);selectedIndex++;markDirty();render();
}

function deleteTask(){
  const task=doc?.tasks?.[selectedIndex];if(!task)return;
  if(!confirm('Delete task "'+(task.label||taskLabel(task,selectedIndex))+'"?'))return;
  doc.tasks.splice(selectedIndex,1);selectedIndex=Math.max(0,selectedIndex-1);markDirty();render();
}

function moveTask(delta){
  const tasks=doc?.tasks||[];const target=selectedIndex+delta;
  if(target<0||target>=tasks.length)return;
  [tasks[selectedIndex],tasks[target]]=[tasks[target],tasks[selectedIndex]];selectedIndex=target;markDirty();render();
}

async function load(){
  if(!canRead())throw new Error('File read permission is required to edit tasks.json');
  await loadWorkspaceRoots();
  const path=currentTasksPath();
  const exists=await currentTasksFileExists();
  if(exists){
    const response=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(path));
    fileMeta=response;doc=parseTasksDocument(response.content);
  }else{
    const content=JSON.stringify({version:'2.0.0',tasks:[]},null,2)+'\n';
    fileMeta={path,content,sha256:'',missing:true};doc=parseTasksDocument(content);
  }
  if(pathNode){pathNode.textContent=path;pathNode.title=path;}
  selectedIndex=0;mode='visual';clearDirty();render();
}

async function save(){
  if(!canWrite())throw new Error('File write permission is required to save tasks.json');
  let content;
  if(mode==='raw'){
    content=rawArea?.value??fileMeta.content;
    doc=parseTasksDocument(content);
  }else{
    content=JSON.stringify(doc,null,2)+'\n';
  }
  saveButton.disabled=true;
  try{
    await ensureCurrentTasksFile();
    const saved=await app.jsonFetch('/api/project/file',{
      method:'PUT',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({path:currentTasksPath(),content,expected_sha256:fileMeta.sha256})
    });
    fileMeta=saved;fileMeta.content=content;clearDirty();
    await app.loadTasks();
    if(mode==='raw'&&rawArea)rawArea.value=content;
  }finally{saveButton.disabled=!canWrite();}
}

function closeEditor(){
  if(dirty&&!confirm('Discard unsaved tasks.json changes?'))return;
  overlay?.classList.remove('visible');
}

function install(){
  if(overlay?.isConnected)return;
  const button=document.createElement('button');button.id='tasks-json-editor';button.type='button';button.textContent='Edit tasks.json…';button.title='Visual editor for .vscode/tasks.json';
  button.hidden=!canRead();button.onclick=()=>open().catch(app.showError);
  const header=document.querySelector('header');const reload=document.querySelector('#reload');
  if(header){if(reload)header.insertBefore(button,reload);else header.append(button);}

  overlay=document.createElement('div');overlay.className='tasks-editor-overlay';
  const dialog=document.createElement('div');dialog.className='tasks-editor-dialog';
  const head=document.createElement('div');head.className='tasks-editor-head';
  const title=document.createElement('strong');title.textContent='tasks.json';
  rootSelect=document.createElement('select');rootSelect.className='tasks-editor-root';rootSelect.title='Workspace root whose .vscode/tasks.json is being edited';
  rootSelect.onchange=()=>{
    const next=rootSelect.value;
    if(dirty&&!confirm('Discard unsaved tasks.json changes before switching workspace root?')){rootSelect.value=selectedRootID;return;}
    selectedRootID=next;load().catch(app.showError);
  };
  pathNode=document.createElement('code');pathNode.textContent=PRIMARY_TASKS_PATH;
  dirtyBadge=document.createElement('span');dirtyBadge.className='dirty';
  const spacer=document.createElement('span');spacer.className='spacer';
  const modes=document.createElement('div');modes.className='tasks-editor-mode';
  visualButton=document.createElement('button');visualButton.type='button';visualButton.textContent='Visual';
  rawButton=document.createElement('button');rawButton.type='button';rawButton.textContent='Raw JSON';
  visualButton.onclick=()=>switchMode('visual');rawButton.onclick=()=>switchMode('raw');modes.append(visualButton,rawButton);
  const reloadButton=document.createElement('button');reloadButton.type='button';reloadButton.textContent='Reload';reloadButton.onclick=()=>{if(dirty&&!confirm('Discard unsaved tasks.json changes?'))return;load().catch(app.showError);};
  const close=document.createElement('button');close.type='button';close.textContent='×';close.onclick=()=>closeEditor();
  head.append(title,rootSelect,pathNode,dirtyBadge,spacer,modes,reloadButton,close);

  const body=document.createElement('div');body.className='tasks-editor-body';
  const sidebar=document.createElement('div');sidebar.className='tasks-editor-list';
  const listTools=document.createElement('div');listTools.className='tasks-editor-list-tools';
  const add=document.createElement('button');add.type='button';add.textContent='+ Add';add.onclick=addTask;
  const clone=document.createElement('button');clone.type='button';clone.textContent='Clone';clone.onclick=cloneTask;
  const up=document.createElement('button');up.type='button';up.textContent='↑';up.title='Move task up';up.onclick=()=>moveTask(-1);
  const down=document.createElement('button');down.type='button';down.textContent='↓';down.title='Move task down';down.onclick=()=>moveTask(1);
  const remove=document.createElement('button');remove.type='button';remove.textContent='Delete';remove.onclick=deleteTask;
  const graph=document.createElement('button');graph.type='button';graph.textContent='Graph';graph.title='Preview task dependency graph';graph.onclick=openWorkflowGraphPreview;
  listTools.append(add,clone,up,down,remove,graph);
  list=document.createElement('div');list.className='tasks-editor-items';sidebar.append(listTools,list);
  main=document.createElement('div');main.className='tasks-editor-main';body.append(sidebar,main);

  const actions=document.createElement('div');actions.className='tasks-editor-actions';
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Close';cancel.onclick=()=>closeEditor();
  saveButton=document.createElement('button');saveButton.type='button';saveButton.className='tasks-editor-save';saveButton.textContent='Save tasks.json';saveButton.onclick=()=>save().catch(app.showError);saveButton.disabled=!canWrite();
  actions.append(cancel,saveButton);
  dialog.append(head,body,actions);overlay.append(dialog);document.body.append(overlay);
  overlay.addEventListener('pointerdown',event=>{if(event.target===overlay)closeEditor();});
}

async function open(){
  install();overlay.classList.add('visible');await load();
}

document.addEventListener('keydown',event=>{
  if(!overlay?.classList.contains('visible'))return;
  if(event.key==='Escape'){event.preventDefault();closeEditor();}
  if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='s'){event.preventDefault();save().catch(app.showError);}
});

install();
globalThis.TaskMenuTasksEditor={open,close:closeEditor,reload:load,workflowGraphModel,openWorkflowGraphPreview};
