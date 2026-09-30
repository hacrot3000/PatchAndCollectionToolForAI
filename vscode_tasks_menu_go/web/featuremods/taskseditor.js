const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for tasks.json editor');

const TASKS_PATH='.vscode/tasks.json';
const WORKSPACE_TOKEN='$'+'{workspaceFolder}';

const style=document.createElement('style');
style.textContent=`
.tasks-editor-overlay{position:fixed;inset:0;z-index:15500;display:none;align-items:stretch;justify-content:center;background:rgba(0,0,0,.62);padding:22px}
.tasks-editor-overlay.visible{display:flex}
.tasks-editor-dialog{width:min(1180px,98vw);height:min(880px,95vh);display:flex;flex-direction:column;background:#15191f;border:1px solid #48515f;border-radius:10px;box-shadow:0 18px 55px rgba(0,0,0,.55);overflow:hidden}
.tasks-editor-head{display:flex;align-items:center;gap:8px;padding:10px 12px;border-bottom:1px solid #30343b}
.tasks-editor-head strong{font-size:13px}.tasks-editor-head .spacer{flex:1}.tasks-editor-head .dirty{font-size:10px;opacity:.7}
.tasks-editor-mode{display:flex;gap:4px}.tasks-editor-mode button.active{background:#244c70;border-color:#3f79a8}
.tasks-editor-body{display:grid;grid-template-columns:270px minmax(0,1fr);min-height:0;flex:1}
.tasks-editor-list{display:flex;flex-direction:column;min-height:0;border-right:1px solid #30343b}
.tasks-editor-list-tools{display:flex;gap:5px;padding:7px;border-bottom:1px solid #30343b}.tasks-editor-list-tools button{font-size:11px}
.tasks-editor-items{overflow:auto;min-height:0;padding:5px}.tasks-editor-item{display:block;width:100%;text-align:left;border:0;background:transparent;padding:7px 8px;border-radius:5px;font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.tasks-editor-item:hover{background:#202630}.tasks-editor-item.active{background:#29384b}
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
html[data-taskmenu-theme="light"] .tasks-editor-form input,html[data-taskmenu-theme="light"] .tasks-editor-form select,html[data-taskmenu-theme="light"] .tasks-editor-form textarea,html[data-taskmenu-theme="light"] .tasks-editor-exec-row code,html[data-taskmenu-theme="light"] .tasks-editor-raw{background:#f7f9fb;border-color:#b9c0c8;color:#202124}
html[data-taskmenu-theme="light"] .tasks-editor-item:hover{background:#edf1f5}html[data-taskmenu-theme="light"] .tasks-editor-item.active{background:#dce8f4}
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

let overlay=null,list=null,main=null,rawArea=null,saveButton=null,dirtyBadge=null;
let visualButton=null,rawButton=null;
let fileMeta=null,doc=null,selectedIndex=0,mode='visual',dirty=false;

function canRead(){return app.hasPermission?.('files.read')!==false;}
function canWrite(){return app.hasPermission?.('files.write')!==false;}

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

function updateList(){
  list.replaceChildren();
  (doc?.tasks||[]).forEach((task,index)=>{
    const button=document.createElement('button');button.type='button';button.className='tasks-editor-item'+(index===selectedIndex?' active':'');
    button.textContent=taskLabel(task,index);button.title=String(task?.label||button.textContent);
    button.onclick=()=>{selectedIndex=index;render();};
    list.append(button);
  });
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
  addField(form,'Menu group',makeInput(task.menuGroup||'',value=>{if(value)task.menuGroup=value;else delete task.menuGroup;}));
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

  renderExecutionFiles(form,task);

  const advanced=document.createElement('div');advanced.className='tasks-editor-help';
  advanced.textContent='Advanced VS Code fields such as problemMatcher, presentation, dependsOn, inputs and custom extension fields remain in the document. Switch to Raw JSON to edit them directly.';
  form.append(advanced);
  main.append(form);
}

function renderRaw(){
  main.replaceChildren();
  rawArea=document.createElement('textarea');rawArea.className='tasks-editor-raw';
  rawArea.value=fileMeta?.content??JSON.stringify(doc,null,2)+'\n';
  rawArea.oninput=markDirty;
  main.append(rawArea);
}

function updateList(){
  list.replaceChildren();
  (doc?.tasks||[]).forEach((task,index)=>{
    const button=document.createElement('button');button.type='button';button.className='tasks-editor-item'+(index===selectedIndex?' active':'');
    button.textContent=taskLabel(task,index);button.title=String(task?.label||button.textContent);
    button.onclick=()=>{selectedIndex=index;render();};
    list.append(button);
  });
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
  const response=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(TASKS_PATH));
  fileMeta=response;doc=parseTasksDocument(response.content);selectedIndex=0;mode='visual';clearDirty();render();
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
    const saved=await app.jsonFetch('/api/project/file',{
      method:'PUT',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({path:TASKS_PATH,content,expected_sha256:fileMeta.sha256})
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
  const path=document.createElement('code');path.textContent=TASKS_PATH;
  dirtyBadge=document.createElement('span');dirtyBadge.className='dirty';
  const spacer=document.createElement('span');spacer.className='spacer';
  const modes=document.createElement('div');modes.className='tasks-editor-mode';
  visualButton=document.createElement('button');visualButton.type='button';visualButton.textContent='Visual';
  rawButton=document.createElement('button');rawButton.type='button';rawButton.textContent='Raw JSON';
  visualButton.onclick=()=>switchMode('visual');rawButton.onclick=()=>switchMode('raw');modes.append(visualButton,rawButton);
  const reloadButton=document.createElement('button');reloadButton.type='button';reloadButton.textContent='Reload';reloadButton.onclick=()=>{if(dirty&&!confirm('Discard unsaved tasks.json changes?'))return;load().catch(app.showError);};
  const close=document.createElement('button');close.type='button';close.textContent='×';close.onclick=()=>closeEditor();
  head.append(title,path,dirtyBadge,spacer,modes,reloadButton,close);

  const body=document.createElement('div');body.className='tasks-editor-body';
  const sidebar=document.createElement('div');sidebar.className='tasks-editor-list';
  const listTools=document.createElement('div');listTools.className='tasks-editor-list-tools';
  const add=document.createElement('button');add.type='button';add.textContent='+ Add';add.onclick=addTask;
  const clone=document.createElement('button');clone.type='button';clone.textContent='Clone';clone.onclick=cloneTask;
  const up=document.createElement('button');up.type='button';up.textContent='↑';up.title='Move task up';up.onclick=()=>moveTask(-1);
  const down=document.createElement('button');down.type='button';down.textContent='↓';down.title='Move task down';down.onclick=()=>moveTask(1);
  const remove=document.createElement('button');remove.type='button';remove.textContent='Delete';remove.onclick=deleteTask;
  listTools.append(add,clone,up,down,remove);
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
globalThis.TaskMenuTasksEditor={open,close:closeEditor,reload:load};
