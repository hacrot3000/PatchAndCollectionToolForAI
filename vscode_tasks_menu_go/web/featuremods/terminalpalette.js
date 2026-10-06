const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for unified command palette');

const style=document.createElement('style');
style.textContent=`
.terminal-command-backdrop{display:none;position:fixed;inset:0;z-index:2900;background:rgba(0,0,0,.46);align-items:flex-start;justify-content:center;padding-top:min(10vh,82px)}
.terminal-command-backdrop.visible{display:flex}
.terminal-command-dialog{width:min(720px,94vw);max-height:76vh;display:flex;flex-direction:column;border:1px solid #4a5362;border-radius:9px;background:#15191f;box-shadow:0 18px 55px rgba(0,0,0,.5);overflow:hidden}
.terminal-command-input{margin:9px;width:calc(100% - 18px);box-sizing:border-box;border:1px solid #343b46;border-radius:5px;background:#0d1117;color:inherit;padding:8px 10px;font:12px ui-monospace,monospace;outline:none}
.terminal-command-status{padding:0 10px 7px;font-size:10px;opacity:.62}
.terminal-command-results{overflow:auto;min-height:52px;max-height:61vh;padding:4px;border-top:1px solid #30343b}
.terminal-command-row{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px;align-items:center;width:100%;text-align:left;border:0;border-radius:5px;background:transparent;padding:8px 9px}
.terminal-command-row.selected,.terminal-command-row:hover:not(:disabled){background:#293241}
.terminal-command-row:disabled{opacity:.42}
.terminal-command-label{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px}
.terminal-command-key{font:10px ui-monospace,monospace;opacity:.58}
.terminal-command-empty{padding:13px 10px;font-size:12px;opacity:.62}
html[data-taskmenu-theme="light"] .terminal-command-dialog{background:#fff;border-color:#b9c0c8;box-shadow:0 18px 55px rgba(0,0,0,.18)}
html[data-taskmenu-theme="light"] .terminal-command-input{background:#f7f8fa;border-color:#d5d9df}
html[data-taskmenu-theme="light"] .terminal-command-row.selected,html[data-taskmenu-theme="light"] .terminal-command-row:hover:not(:disabled){background:#e8edf3}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='terminal-command-backdrop';
const dialog=document.createElement('div');dialog.className='terminal-command-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','TaskDeck command palette');
const input=document.createElement('input');input.type='search';input.className='terminal-command-input';input.autocomplete='off';input.spellcheck=false;input.placeholder='Command Palette — search files, tasks, Git, SSH, DB, SFTP, settings…';
const status=document.createElement('div');status.className='terminal-command-status';status.textContent='TaskDeck Command Palette · Ctrl/Cmd+Shift+P';
const results=document.createElement('div');results.className='terminal-command-results';
dialog.append(input,status,results);backdrop.append(dialog);document.body.append(backdrop);

let commands=[];
let visible=[];
let selected=0;

function activeTerminal(){
  const view=app.views.get(app.active);
  return view?.term&&!view.closed?view:null;
}
function buttonAction(view,selector,label,keywords='',shortcut=''){
  const button=view?.pane?.querySelector(selector)||null;
  return {
    id:selector,label,keywords,shortcut,
    disabled:!button||button.disabled||button.hidden,
    run:()=>{if(!button||button.disabled||button.hidden)throw new Error(label+' is unavailable');button.click();}
  };
}
function permissionAllowed(permission){
  return !app.sharedMode||Boolean(app.hasPermission?.(permission)||app.hasPermission?.('project.admin'));
}
function staticProjectCommands(){
  return [
    {id:'project-quick-open',label:'File: Quick Open…',keywords:'open file project ctrl p',shortcut:'Ctrl+P',disabled:!permissionAllowed('files.read'),run:()=>globalThis.TaskMenuQuickOpen?.open?.()},
    {id:'project-explorer',label:'Project: Open Explorer',keywords:'files folders tree',disabled:!permissionAllowed('files.read'),run:()=>globalThis.TaskMenuExplorer?.open?.()},
    {id:'project-search',label:'Project: Search in Files…',keywords:'find replace text workspace',shortcut:'Ctrl+Shift+F',disabled:!permissionAllowed('files.read'),run:()=>globalThis.TaskMenuProjectSearch?.open?.()},
    {id:'project-symbols',label:'Project: Symbols…',keywords:'symbols functions classes code',disabled:!permissionAllowed('files.read'),run:()=>globalThis.TaskMenuProjectSymbols?.open?.()},
    {id:'project-profiles',label:'Project: Profiles…',keywords:'workspace profile environment connections tasks',disabled:!permissionAllowed('settings.read'),run:()=>globalThis.TaskMenuProjectProfiles?.open?.()},
    {id:'project-snapshots',label:'Project: Workspace Snapshots…',keywords:'snapshot restore save workspace state',disabled:!permissionAllowed('settings.read'),run:()=>globalThis.TaskMenuWorkspaceSnapshots?.open?.()}
  ];
}
function staticConnectionCommands(){
  return [
    {id:'connections-open',label:'Connections: Open panel',keywords:'ssh database db sftp ftp local terminal remote',run:()=>globalThis.TaskMenuConnections?.open?.()},
    {id:'connections-graph',label:'Connections: Tunnel Graph…',keywords:'ssh database sftp ftp tunnel port pid route reconnect connection graph',run:()=>globalThis.TaskMenuConnectionGraph?.open?.()},
    {id:'connections-ssh',label:'SSH: Open saved connections',keywords:'ssh remote host profile terminal connection',run:()=>globalThis.TaskMenuConnections?.open?.()},
    {id:'connections-db',label:'Database: Open saved connections',keywords:'database db mysql sqlite mongodb redis connection',run:()=>globalThis.TaskMenuConnections?.open?.()},
    {id:'database-schema-diff',label:'Database: Schema Diff…',keywords:'database db compare schema structure migration alter staging production',run:()=>globalThis.TaskMenuDatabaseSchemaDiff?.open?.()},
    {id:'connections-transfer',label:'SFTP/FTP: Open saved connections',keywords:'sftp ftp transfer remote files connection',run:()=>globalThis.TaskMenuConnections?.open?.()}
  ];
}
function staticGitCommands(){
  return [
    {id:'git-open',label:'Git: Open panel',keywords:'git status branches diff log history graph',disabled:!permissionAllowed('git.status'),run:()=>globalThis.TaskMenuGitFiles?.open?.()},
    {id:'git-refresh',label:'Git: Refresh status',keywords:'git rescan status refresh',disabled:!permissionAllowed('git.status'),run:()=>globalThis.TaskMenuGitFiles?.refresh?.()}
  ];
}
function staticSettingsCommands(){
  return [
    {id:'operations-open',label:'Operation Center: Open',keywords:'activity jobs queue running completed failed git transfer patch task update database',run:()=>globalThis.TaskMenuOperationCenter?.open?.()},
    {id:'project-health-open',label:'Project: Health Dashboard…',keywords:'project health dashboard git status tasks terminals transfers database ssh disk build overview',run:()=>globalThis.TaskMenuProjectHealth?.open?.()},
    {id:'approvals-open',label:'Approvals: Open manager',keywords:'dangerous action approval policy admin request pending approve reject',disabled:!globalThis.TaskMenuApprovals?.canView?.(),run:()=>globalThis.TaskMenuApprovals?.openManager?.()},
    {id:'secrets-open',label:'Secrets: Open manager',keywords:'credentials password token deploy ssh database ftp encrypted',disabled:!permissionAllowed('secrets.view'),run:()=>globalThis.TaskMenuSecrets?.open?.()},
    {id:'settings-open',label:'Settings: Open menu',keywords:'settings configuration preferences appearance terminal update workspace',run:()=>globalThis.TaskMenuMenus?.open?.('settings')},
    {id:'settings-backup',label:'Settings: Backup / Restore…',keywords:'settings backup restore export import configuration portable migrate',disabled:!permissionAllowed('settings.read'),run:()=>globalThis.TaskMenuConfigBackup?.open?.()},
    {id:'tasks-editor',label:'Settings: Edit tasks.json…',keywords:'task tasks json editor workflow dependency',disabled:!permissionAllowed('files.write'),run:()=>document.querySelector('#tasks-json-editor')?.click()}
  ];
}
function dynamicTaskCommands(){
  const canRun=permissionAllowed('tasks.run');
  return (Array.isArray(app.taskData?.tasks)?app.taskData.tasks:[]).map(task=>({
    id:'task-'+String(task.id),
    label:'Task: '+String(task.menu_label||task.menuLabel||task.label||('Task '+task.id)),
    keywords:['run task',task.label,task.menu_label,task.menu_group,task.detail].filter(Boolean).join(' '),
    disabled:!canRun,
    run:()=>app.startTask(task)
  }));
}
function terminalCommands(){
  const view=activeTerminal();
  return [
    {
      id:'new-terminal',label:'Terminal: New terminal',keywords:'open create shell',shortcut:'Ctrl+Shift+grave',
      disabled:app.sharedMode&&!app.hasPermission?.('terminal.create'),
      run:()=>app.startTerminal()
    },
    {
      id:'duplicate-terminal',label:'Terminal: Duplicate terminal here',keywords:'clone cwd environment env',
      disabled:!view||!globalThis.TaskMenuTerminalClone?.cloneAllowed?.(view),
      run:()=>globalThis.TaskMenuTerminalClone?.cloneTerminal?.(view)
    },
    {
      id:'search-all',label:'Terminal: Search all terminal scrollback',keywords:'find output logs all terminals',
      disabled:false,run:()=>globalThis.TaskMenuTerminalSearchAll?.open?.()
    },
    buttonAction(view,'.console-search-btn','Terminal: Find in active console','search find current','Ctrl+F'),
    buttonAction(view,'.console-save-btn','Terminal: Save active console log','download export log'),
    buttonAction(view,'.session-split-vertical','Terminal: Split vertical','pane right layout'),
    buttonAction(view,'.session-split-horizontal','Terminal: Split horizontal','pane below layout'),
    buttonAction(view,'.session-move-split-group','Terminal: Move to split group…','pane layout move group'),
    buttonAction(view,'.session-swap-split','Terminal: Swap split panes','layout swap'),
    buttonAction(view,'.session-unsplit','Terminal: Unsplit','layout detach'),
    buttonAction(view,'.session-process-tree','Terminal: Process tree…','process pid children'),
    buttonAction(view,'.terminal-rename','Terminal: Rename tab…','title label'),
    buttonAction(view,'.session-clear-console','Terminal: Clear console','scrollback erase')
  ];
}
function commandList(){
  return [
    ...staticProjectCommands(),
    ...dynamicTaskCommands(),
    ...staticGitCommands(),
    ...staticConnectionCommands(),
    ...staticSettingsCommands(),
    ...terminalCommands()
  ];
}
function filterCommands(){
  commands=commandList();
  const query=String(input.value||'').trim().toLowerCase();
  const tokens=query.split(/\s+/).filter(Boolean);
  visible=commands.filter(command=>{
    if(!tokens.length)return true;
    const hay=(command.label+' '+command.keywords).toLowerCase();
    return tokens.every(token=>hay.includes(token));
  });
  selected=Math.min(selected,Math.max(0,visible.length-1));
  if(visible[selected]?.disabled){
    const next=visible.findIndex(item=>!item.disabled);if(next>=0)selected=next;
  }
  render();
}
function select(index){
  if(!visible.length)return;
  let next=(index+visible.length)%visible.length;
  const direction=index>=selected?1:-1;
  for(let step=0;step<visible.length;step++){
    if(!visible[next]?.disabled){selected=next;break;}
    next=(next+direction+visible.length)%visible.length;
  }
  [...results.querySelectorAll('.terminal-command-row')].forEach((node,i)=>node.classList.toggle('selected',i===selected));
  results.querySelector('.terminal-command-row.selected')?.scrollIntoView({block:'nearest'});
}
function run(index=selected){
  const command=visible[index];if(!command||command.disabled)return;
  close();
  Promise.resolve(command.run()).catch(app.showError);
}
function render(){
  results.replaceChildren();
  status.textContent='TaskDeck Command Palette · '+visible.length+' command(s) · Ctrl/Cmd+Shift+P';
  if(!visible.length){const empty=document.createElement('div');empty.className='terminal-command-empty';empty.textContent='No matching commands';results.append(empty);return;}
  visible.forEach((command,index)=>{
    const row=document.createElement('button');row.type='button';row.className='terminal-command-row'+(index===selected?' selected':'');row.disabled=Boolean(command.disabled);
    const label=document.createElement('span');label.className='terminal-command-label';label.textContent=command.label;
    const key=document.createElement('span');key.className='terminal-command-key';key.textContent=command.shortcut||'';
    row.append(label,key);row.onmousemove=()=>{if(!command.disabled)select(index);};row.onclick=()=>run(index);results.append(row);
  });
}
function open(options={}){
  backdrop.classList.add('visible');
  input.value=String(options.query||'');
  selected=0;filterCommands();
  requestAnimationFrame(()=>{input.focus();input.select();});
}
function close(){
  backdrop.classList.remove('visible');input.value='';commands=[];visible=[];selected=0;
}
function decorate(view){
  if(!view?.term||view.closed)return null;
  const head=view.pane?.querySelector('.pane-head');if(!head)return null;
  let button=head.querySelector('.terminal-command-palette');
  if(button)return button;
  button=document.createElement('button');button.type='button';button.className='terminal-command-palette';
  button.textContent='Commands…';button.title='Open TaskDeck Command Palette (Ctrl/Cmd+Shift+P)';
  button.onclick=()=>open();
  head.append(button);return button;
}

input.addEventListener('input',()=>{selected=0;filterCommands();});
input.addEventListener('keydown',event=>{
  if(event.key==='ArrowDown'){event.preventDefault();select(selected+1);}
  else if(event.key==='ArrowUp'){event.preventDefault();select(selected-1);}
  else if(event.key==='Enter'){event.preventDefault();run();}
  else if(event.key==='Escape'){event.preventDefault();close();}
});
backdrop.onmousedown=event=>{if(event.target===backdrop)close();};
document.addEventListener('keydown',event=>{
  if(!(event.ctrlKey||event.metaKey)||!event.shiftKey||event.altKey||String(event.key||'').toLowerCase()!=='p')return;
  event.preventDefault();event.stopPropagation();
  if(backdrop.classList.contains('visible'))close();else open();
},true);
window.addEventListener('taskmenu:session',event=>{const view=event.detail?.view;if(view)setTimeout(()=>decorate(view),0);});
for(const view of app.views.values())decorate(view);

const paletteAPI={open,close,commandList,decorate,get visible(){return backdrop.classList.contains('visible');}};
globalThis.TaskMenuCommandPalette=paletteAPI;
globalThis.TaskMenuTerminalPalette=paletteAPI;
