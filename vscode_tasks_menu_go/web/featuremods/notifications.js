const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for notifications');

const previousStatus=new Map();
const notified=new Set();
const key=()=>`vscode-tasks-menu:notifications:${app.taskData.workspace}`;
function enabled(){try{return localStorage.getItem(key())==='1';}catch{return false;}}
function setEnabled(value){try{if(value)localStorage.setItem(key(),'1');else localStorage.removeItem(key());}catch(e){console.warn('Cannot persist notification preference',e);}updateButton();}
function supportState(){if(!('Notification' in window))return 'unsupported';return Notification.permission;}
function buttonLabel(){const state=supportState();if(state==='unsupported')return 'Notify: N/A';if(state==='denied')return 'Notify: blocked';return enabled()&&state==='granted'?'Notify: On':'Notify: Off';}

let button=null;
function installButton(){
  if(button?.isConnected)return;
  const header=document.querySelector('header');if(!header)return;
  button=document.createElement('button');button.id='notifications-toggle';button.title='Desktop notification when a task finishes';button.onclick=toggleNotifications;
  const reload=document.querySelector('#reload');if(reload)header.insertBefore(button,reload);else header.append(button);updateButton();
}
function updateButton(){if(!button)return;button.textContent=buttonLabel();button.disabled=supportState()==='unsupported';}
async function toggleNotifications(){
  if(!('Notification' in window))return;
  if(Notification.permission==='denied'){updateButton();return;}
  if(enabled()){setEnabled(false);return;}
  let permission=Notification.permission;
  if(permission!=='granted')permission=await Notification.requestPermission();
  setEnabled(permission==='granted');
}

function stateLabel(meta){if(meta.status==='stopped')return 'STOPPED';if(meta.status==='exited'&&meta.exit_code===0)return 'PASS';if(meta.status==='exited')return 'FAIL';return meta.status||'DONE';}
function duration(meta){const a=Date.parse(meta.started_at||''),b=Date.parse(meta.ended_at||'');if(!Number.isFinite(a)||!Number.isFinite(b)||b<a)return '';const seconds=Math.floor((b-a)/1000);const m=Math.floor(seconds/60),s=seconds%60;return `${m}:${String(s).padStart(2,'0')}`;}
function maybeNotify(meta){
  const before=previousStatus.get(meta.id);previousStatus.set(meta.id,meta.status);
  if(before!=='running'||meta.status==='running'||notified.has(meta.id))return;
  notified.add(meta.id);
  if(!enabled()||Notification.permission!=='granted'||!document.hidden)return;
  const state=stateLabel(meta);const elapsed=duration(meta);const body=[meta.label,state,elapsed&&`Duration ${elapsed}`,meta.exit_code!=null&&`Exit ${meta.exit_code}`].filter(Boolean).join(' · ');
  try{new Notification(`VS Code Task: ${state}`,{body,tag:`task-${meta.id}`});}catch(e){console.warn('Notification failed',e);}
}

window.addEventListener('taskmenu:session',event=>{const meta=event.detail?.meta;if(meta)maybeNotify(meta);});
installButton();
for(const view of app.views.values())previousStatus.set(view.meta.id,view.meta.status);
