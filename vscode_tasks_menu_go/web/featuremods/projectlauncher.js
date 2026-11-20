// Installs an OS-native launcher for the currently loaded project.
// The server (not the browser) decides its workspace, executable and host OS.
const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for project launcher');
const button=document.createElement('button');
button.id='taskdeck-project-launcher';
button.type='button';
button.textContent='Create project launcher icon…';
button.title='Create an icon in the host desktop application launcher for this project';
button.hidden=true;
let activeRequest=null;
let ready=false;

function launcherReady(){
 return Boolean(app.taskData?.workspace&&!app.sharedMode);
}
function launcherApply(data){
 if(!data?.supported||!launcherReady()){button.hidden=true;ready=false;return}
 ready=true;
 button.hidden=false;
 button.textContent=data.installed?'Update project launcher icon…':'Create project launcher icon…';
 button.title=data.path?('Host launcher: '+data.path):'Create a host launcher for this project';
}
async function launcherRefresh(){
 if(!launcherReady()){button.hidden=true;return}
 try{
  const data=await app.jsonFetch('/api/system/project-launcher');
  launcherApply(data);
 }catch(error){
  button.hidden=true;
  console.warn('TaskDeck native launcher unavailable',error);
 }
}
button.onclick=async()=>{
 if(!ready||activeRequest)return;
 button.disabled=true;
 button.textContent='Installing launcher…';
 activeRequest=app.jsonFetch('/api/system/project-launcher',{method:'POST'});
 try{
  const data=await activeRequest;
  launcherApply(data);
  button.textContent='Launcher created · Update icon…';
  button.title='Created launcher: '+(data.path||'');
 }catch(error){
  app.showError(error);
  await launcherRefresh();
 }finally{
  activeRequest=null;
  button.disabled=false;
 }
};
if(app.taskData?.workspace)void launcherRefresh();
else window.addEventListener('taskmenu:tasks',()=>{void launcherRefresh()},{once:true});
globalThis.TaskMenuProjectLauncher={button,refresh:launcherRefresh};
