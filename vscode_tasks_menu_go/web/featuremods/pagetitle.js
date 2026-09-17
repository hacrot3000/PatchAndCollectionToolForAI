const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for page title config');

const defaultTitle='VS Code Tasks Menu';
const button=document.querySelector('#edit-title');

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
  document.title=String(data?.title||'').trim()||defaultTitle;
}

async function editConfiguredTitle(){
  const current=document.title===defaultTitle?'':document.title;
  const value=window.prompt('Page title (leave blank to use the default):',current);
  if(value===null)return;
  const data=await app.jsonFetch('/api/config/page-title',{
    method:'PUT',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({title:value.trim()})
  });
  clearLegacyBrowserTitle();
  document.title=String(data?.title||'').trim()||defaultTitle;
}

if(button){
  button.title="Edit the title and save it to the project's vscode_tasks_menu.ini";
  button.onclick=()=>editConfiguredTitle().catch(app.showError);
}
window.addEventListener('taskmenu:tasks',()=>loadConfiguredTitle().catch(app.showError));
setTimeout(()=>loadConfiguredTitle().catch(app.showError),0);
