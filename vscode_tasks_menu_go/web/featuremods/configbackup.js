const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for configuration backup');

const style=document.createElement('style');
style.textContent=`
.config-backup-backdrop{display:none;position:fixed;inset:0;z-index:17850;background:rgba(0,0,0,.52);align-items:flex-start;justify-content:center;padding:6vh 16px}.config-backup-backdrop.visible{display:flex}
.config-backup-dialog{width:min(780px,96vw);max-height:86vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 60px rgba(0,0,0,.58);overflow:hidden}
.config-backup-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #303843}.config-backup-head strong{flex:1}
.config-backup-body{overflow:auto;padding:12px}.config-backup-note{font-size:10px;opacity:.68;line-height:1.5;margin-bottom:10px}.config-backup-actions{display:flex;gap:7px;flex-wrap:wrap;margin:10px 0}
.config-backup-preview{border:1px solid #303843;border-radius:7px;padding:9px;font-size:11px}.config-backup-preview-grid{display:grid;grid-template-columns:190px 1fr;gap:5px 10px}.config-backup-warning{color:#f0c66b;margin-top:9px;white-space:pre-wrap}
.config-backup-import{display:none;margin-top:12px}.config-backup-import.visible{display:block}.config-backup-mode{display:flex;gap:14px;margin:9px 0}.config-backup-file{display:none}
html[data-taskmenu-theme="light"] .config-backup-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .config-backup-preview{border-color:#d0d7de}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='config-backup-backdrop';
const dialog=document.createElement('div');dialog.className='config-backup-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Backup and restore TaskDeck configuration');
const head=document.createElement('div');head.className='config-backup-head';
const title=document.createElement('strong');title.textContent='BACKUP / RESTORE CONFIGURATION';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';
head.append(title,closeButton);
const body=document.createElement('div');body.className='config-backup-body';
const note=document.createElement('div');note.className='config-backup-note';note.textContent='Portable backup includes project settings, command presets, task/terminal layout, Project Profiles and connection profiles. Environment values are opt-in. SecretStore values and original secret references are never exported.';
const actions=document.createElement('div');actions.className='config-backup-actions';
const includeEnvLabel=document.createElement('label');includeEnvLabel.title='Environment variables can contain tokens/passwords. Off is the safe portable default.';
const includeEnv=document.createElement('input');includeEnv.type='checkbox';includeEnv.checked=false;
includeEnvLabel.append(includeEnv,document.createTextNode(' Include environment variables (may contain secrets)'));
const exportButton=document.createElement('button');exportButton.type='button';exportButton.textContent='Export backup';
const importButton=document.createElement('button');importButton.type='button';importButton.textContent='Import backup…';
const fileInput=document.createElement('input');fileInput.type='file';fileInput.accept='application/json,.json';fileInput.className='config-backup-file';
actions.append(includeEnvLabel,exportButton,importButton,fileInput);
const preview=document.createElement('div');preview.className='config-backup-preview';preview.textContent='No backup loaded.';
const importPane=document.createElement('div');importPane.className='config-backup-import';
const mode=document.createElement('div');mode.className='config-backup-mode';
const mergeLabel=document.createElement('label');const mergeRadio=document.createElement('input');mergeRadio.type='radio';mergeRadio.name='config-backup-mode';mergeRadio.value='merge';mergeRadio.checked=true;mergeLabel.append(mergeRadio,document.createTextNode(' Merge with current configuration'));
const replaceLabel=document.createElement('label');const replaceRadio=document.createElement('input');replaceRadio.type='radio';replaceRadio.name='config-backup-mode';replaceRadio.value='replace';replaceLabel.append(replaceRadio,document.createTextNode(' Replace portable configuration'));
mode.append(mergeLabel,replaceLabel);
const restoreButton=document.createElement('button');restoreButton.type='button';restoreButton.textContent='Restore configuration';
const warning=document.createElement('div');warning.className='config-backup-warning';
importPane.append(mode,restoreButton,warning);
body.append(note,actions,preview,importPane);dialog.append(head,body);backdrop.append(dialog);document.body.append(backdrop);

let loadedBundle=null;

function envClientSnapshot(){
  return {
    environment_profiles:globalThis.TaskMenuEnvProfiles?.list?.()||{},
    active_environment:String(globalThis.TaskMenuEnvProfiles?.currentName?.()||'')
  };
}
function downloadJSON(name,value){
  const blob=new Blob([JSON.stringify(value,null,2)+'\n'],{type:'application/json'});
  const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download=name;document.body.append(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(url),1000);
}
function backupCounts(bundle){
  return [
    ['Version',bundle?.version||'—'],
    ['Command presets',bundle?.command_presets?.presets?.length||0],
    ['Task favorites / recent',(bundle?.task_state?.favorites?.length||0)+' / '+(bundle?.task_state?.recent?.length||0)],
    ['Desktop terminals',bundle?.terminal_desktop?.terminals?.length||0],
    ['Mobile terminals',bundle?.terminal_mobile?.terminals?.length||0],
    ['Project Profiles',bundle?.project_profiles?.profiles?.length||0],
    ['Environment Profiles',Object.keys(bundle?.client?.environment_profiles||{}).length],
    ['SSH profiles',bundle?.connections?.ssh?.length||0],
    ['Database profiles',bundle?.connections?.database?.length||0],
    ['FTP/SFTP profiles',bundle?.connections?.file_transfer?.length||0]
  ];
}
function credentialCount(bundle){
  return ['ssh','database','file_transfer'].reduce((sum,key)=>sum+(bundle?.connections?.[key]||[]).filter(item=>item?.credential_required).length,0);
}
function renderPreview(bundle){
  preview.replaceChildren();const grid=document.createElement('div');grid.className='config-backup-preview-grid';
  for(const [label,value] of backupCounts(bundle)){const a=document.createElement('div');a.textContent=label;const b=document.createElement('div');b.textContent=String(value);grid.append(a,b);}
  preview.append(grid);
  const credentials=credentialCount(bundle);
  if(credentials){const msg=document.createElement('div');msg.className='config-backup-warning';msg.textContent=credentials+' connection profile(s) require credentials. Secret values/references are not in this backup; a new machine must re-enter them.';preview.append(msg);}
}
async function exportBackup(){
  exportButton.disabled=true;
  try{
    const includeEnvironment=includeEnv.checked;
    const bundle=await app.jsonFetch('/api/config-backup'+(includeEnvironment?'?include_environment=1':''),{cache:'no-store'});
    if(includeEnvironment)bundle.client=envClientSnapshot();
    renderPreview(bundle);
    const stamp=new Date().toISOString().replace(/[:.]/g,'-');
    downloadJSON('taskdeck-config-backup-'+stamp+'.json',bundle);
  }finally{exportButton.disabled=false;}
}
function validateClientBundle(bundle){
  if(!bundle||typeof bundle!=='object'||Array.isArray(bundle))throw new Error('Backup must be a JSON object');
  if(Number(bundle.version)!==1)throw new Error('Unsupported TaskDeck backup version: '+String(bundle.version));
  const profiles=bundle?.client?.environment_profiles;
  if(profiles!==undefined&&(typeof profiles!=='object'||profiles===null||Array.isArray(profiles)))throw new Error('Invalid environment_profiles section');
  return bundle;
}
async function loadFile(file){
  if(!file)return;const text=await file.text();
  if(text.length>8*1024*1024)throw new Error('Backup file exceeds 8 MiB');
  loadedBundle=validateClientBundle(JSON.parse(text));renderPreview(loadedBundle);importPane.classList.add('visible');
  const credentials=credentialCount(loadedBundle);
  warning.textContent=(credentials?credentials+' connection credential(s) will remain local if matching profile IDs exist; otherwise they will be marked missing.\n':'')
    +(loadedBundle?.includes_environment?'This backup explicitly includes environment variable values; review its origin before restore.\n':'Environment variable values are not included in this backup.\n')
    +'Restore never imports SecretStore plaintext.';
}
async function restore(){
  if(!loadedBundle)throw new Error('Choose a TaskDeck backup first');
  const restoreMode=replaceRadio.checked?'replace':'merge';
  const message=(restoreMode==='replace'
    ?'Replace portable TaskDeck configuration with this backup? Existing connection secrets with matching profile IDs are preserved locally.'
    :'Merge this backup into current TaskDeck configuration?')+'\n\nActive processes are not stopped, but reloading the browser is recommended after restore.';
  if(!window.confirm(message))return;
  restoreButton.disabled=true;
  try{
    await app.jsonFetch('/api/config-backup',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({mode:restoreMode,bundle:loadedBundle})});
    if(loadedBundle?.includes_environment){
      const profiles=loadedBundle?.client?.environment_profiles||{};
      const selected=String(loadedBundle?.client?.active_environment||'');
      if(restoreMode==='replace')globalThis.TaskMenuEnvProfiles?.replaceAll?.(profiles,{selected});
      else globalThis.TaskMenuEnvProfiles?.mergeAll?.(profiles,{selected});
    }
    alert('TaskDeck configuration restored. Reloading the page to apply the restored UI state.');
    location.reload();
  }finally{restoreButton.disabled=false;}
}
function open(){backdrop.classList.add('visible');}
function close(){backdrop.classList.remove('visible');}
exportButton.onclick=()=>exportBackup().catch(app.showError);
importButton.onclick=()=>{fileInput.value='';fileInput.click();};
fileInput.onchange=()=>loadFile(fileInput.files?.[0]).catch(app.showError);
restoreButton.onclick=()=>restore().catch(app.showError);
closeButton.onclick=close;backdrop.onmousedown=event=>{if(event.target===backdrop)close();};
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();});

function installSettingsLauncher(){
  if(document.querySelector('#config-backup-settings'))return true;
  const header=document.querySelector('header');if(!header)return false;
  const button=document.createElement('button');button.type='button';button.id='config-backup-settings';button.textContent='Backup / Restore…';button.title='Export or restore portable TaskDeck configuration';button.onclick=open;
  header.append(button);return true;
}
if(!installSettingsLauncher())window.addEventListener('taskmenu:tasks',installSettingsLauncher,{once:true});

globalThis.TaskMenuConfigBackup={open,close,exportBackup,get bundle(){return loadedBundle;},get visible(){return backdrop.classList.contains('visible');}};
