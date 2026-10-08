// The type marker is a small LEFT-SIDE stripe, never a full foreground/background
// color. Broadcast groups intentionally retain exclusive control over tab text,
// background and border colors.
const app=globalThis.TaskMenuApp;
const host=document.querySelector('#tabs');
if(!app||!host)throw new Error('Tab colors require TaskMenuApp and #tabs');
const style=document.createElement('style');
style.textContent=`
#tabs>[data-taskdeck-tab-type]{position:relative;--taskdeck-type-accent:#9da8b8}
#tabs>[data-taskdeck-tab-type]::before{content:'';position:absolute;left:2px;top:5px;bottom:5px;width:3px;border-radius:3px;background:var(--taskdeck-type-accent);opacity:.85;pointer-events:none}
#tabs>[data-taskdeck-tab-type]:not(.broadcast-grouped){background-image:linear-gradient(90deg,color-mix(in srgb,var(--taskdeck-type-accent) 10%,transparent),transparent 76%)}
#tabs>[data-taskdeck-tab-type].active:not(.broadcast-grouped){background-image:linear-gradient(90deg,color-mix(in srgb,var(--taskdeck-type-accent) 17%,transparent),transparent 80%)}
#tabs>[data-taskdeck-tab-type="terminal"]{--taskdeck-type-accent:#69bea2}
#tabs>[data-taskdeck-tab-type="ssh"]{--taskdeck-type-accent:#63c3c9}
#tabs>[data-taskdeck-tab-type="task"]{--taskdeck-type-accent:#a3a9b6}
#tabs>[data-taskdeck-tab-type="editor"]{--taskdeck-type-accent:#83a8ef}
#tabs>[data-taskdeck-tab-type="diff"]{--taskdeck-type-accent:#bf91e7}
#tabs>[data-taskdeck-tab-type="patch"]{--taskdeck-type-accent:#e7b96c}
#tabs>[data-taskdeck-tab-type="transfer"]{--taskdeck-type-accent:#8cc5dd}
#tabs>[data-taskdeck-tab-type="ftp"]{--taskdeck-type-accent:#88c7dc}
#tabs>[data-taskdeck-tab-type="sftp"]{--taskdeck-type-accent:#9fb6e9}
#tabs>[data-taskdeck-tab-type="database"]{--taskdeck-type-accent:#dd9c91}
#tabs>[data-taskdeck-tab-type="hex"]{--taskdeck-type-accent:#c0acc3}
html[data-taskmenu-theme="light"] #tabs>[data-taskdeck-tab-type]::before{opacity:1}
`;
document.head.append(style);
function tabType(tab){
  if(tab.classList.contains('task-patch-tab'))return 'patch';
  if(tab.classList.contains('file-compare-tab'))return 'diff';
  if(tab.classList.contains('editor-tab'))return 'editor';
  if(tab.classList.contains('db-tab'))return 'database';
  if(tab.classList.contains('ft-tab')){
    const id=String(tab.dataset.id||'').replace(/^file-transfer:/,'');
    const protocol=String(globalThis.TaskMenuFileTransfer?.views?.get?.(id)?.profile?.protocol||'').toLowerCase();
    return protocol==='sftp'?'sftp':protocol==='ftp'?'ftp':'transfer';
  }
  if(tab.classList.contains('hex-tab'))return 'hex';
  const meta=app.views.get(tab.dataset.id||'')?.meta;
  if(meta){
    if(String(meta.target_type||'').toLowerCase()==='ssh'||meta.ssh_profile_id)return 'ssh';
    if(meta.kind==='terminal')return 'terminal';
    return 'task';
  }
  return '';
}
function updateTab(tab){
  if(!(tab instanceof HTMLElement)||tab.parentElement!==host)return;
  const type=tabType(tab);
  if(type)tab.dataset.taskdeckTabType=type;
  else delete tab.dataset.taskdeckTabType;
}
function refresh(){
  for(const tab of host.children)updateTab(tab);
}
const observer=new MutationObserver(records=>{
  for(const record of records){
    for(const node of record.addedNodes){
      if(node instanceof HTMLElement&&node.parentElement===host)updateTab(node);
    }
  }
});
observer.observe(host,{childList:true});
window.addEventListener('taskmenu:session',event=>{if(event.detail?.view?.tab)updateTab(event.detail.view.tab);});
window.addEventListener('taskmenu:workspace-tabs-restored',refresh);
refresh();
globalThis.TaskMenuTabColors={refresh,tabType};
