const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for page title config');

const defaultTitle='TaskDeck';
const button=document.querySelector('#edit-title');

function titleSuffix(value){
  let title=String(value||'').trim();
  if(!title||title===defaultTitle||title==='VS Code Tasks Menu'||title==='VSCode Tasks Menu')return '';
  if(title.startsWith(defaultTitle+' - '))title=title.slice((defaultTitle+' - ').length).trim();
  return title;
}

function renderTitle(value){
  const suffix=titleSuffix(value);
  return suffix?defaultTitle+' - '+suffix:defaultTitle;
}

function legacyStorageKey(){
  const workspace=app.taskData?.workspace;
  return workspace?'vscode-tasks-menu:page-title:'+workspace:'';
}

function clearLegacyBrowserTitle(){
  const key=legacyStorageKey();if(!key)return;
  try{localStorage.removeItem(key);}catch{}
}

async function loadConfiguredTitle(){
  if(!app.taskData)return;
  clearLegacyBrowserTitle();
  const data=await app.jsonFetch('/api/config/page-title');
  document.title=renderTitle(data?.title);
}

async function editConfiguredTitle(){
  const current=titleSuffix(document.title);
  const value=window.prompt('Page title suffix (leave blank to use TaskDeck):',current);
  if(value===null)return;
  const data=await app.jsonFetch('/api/config/page-title',{
    method:'PUT',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({title:titleSuffix(value)})
  });
  clearLegacyBrowserTitle();
  document.title=renderTitle(data?.title);
}

if(button){
  button.title="Edit the title and save it to the project's .vscode/vscode_tasks_menu.ini";
  button.onclick=()=>editConfiguredTitle().catch(app.showError);
}
window.addEventListener('taskmenu:tasks',()=>loadConfiguredTitle().catch(app.showError));
setTimeout(()=>loadConfiguredTitle().catch(app.showError),0);
