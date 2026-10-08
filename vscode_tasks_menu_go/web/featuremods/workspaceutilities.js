const app=globalThis.TaskMenuApp;

// One activity-bar entry for the existing Workspace Snapshots, Operation Center
// and Project Health dialogs. Keep the dialogs and their independent APIs intact.
function installWorkspaceUtilitiesLauncher(){
  if(!app||app.layoutProfile==='mobile'||!app.taskData?.workspace)return false;
  const rail=document.querySelector('.task-activity-bar');
  if(!rail)return false;
  if(rail.querySelector('[data-view="workspace-utilities"]'))return true;

  // Remove obsolete individual buttons if an older module injected them.
  for(const legacy of ['snapshots','operations','health']){
    rail.querySelector('[data-view="'+legacy+'"]')?.remove();
  }

  const style=document.createElement('style');
  style.textContent=`
  .task-workspace-utilities-menu{display:none;position:fixed;z-index:6500;width:min(280px,calc(100vw - 16px));padding:5px;background:#171e27;border:1px solid #475260;border-radius:9px;box-shadow:0 12px 34px rgba(0,0,0,.36)}
  .task-workspace-utilities-menu.visible{display:grid;gap:3px}
  .task-workspace-utilities-item{display:grid;grid-template-columns:28px minmax(0,1fr);align-items:center;gap:7px;width:100%;padding:9px 10px;background:transparent;border:0;border-radius:6px;color:#e3e8f0;text-align:left;cursor:pointer}
  .task-workspace-utilities-item:hover,.task-workspace-utilities-item:focus-visible{background:#304154;outline:none}
  .task-workspace-utilities-icon{text-align:center;font-size:19px;line-height:1.2;color:#a4c8ed}
  .task-workspace-utilities-copy{min-width:0;display:grid;gap:3px}
  .task-workspace-utilities-copy strong{font-size:12px;font-weight:650}
  .task-workspace-utilities-copy small{font-size:10px;opacity:.65}
  html[data-taskmenu-theme="light"] .task-workspace-utilities-menu{background:#fff;border-color:#c8d0da;box-shadow:0 10px 28px rgba(0,0,0,.18)}
  html[data-taskmenu-theme="light"] .task-workspace-utilities-item{color:#26323e}
  html[data-taskmenu-theme="light"] .task-workspace-utilities-item:hover,html[data-taskmenu-theme="light"] .task-workspace-utilities-item:focus-visible{background:#e9f0f8}
  html[data-taskmenu-theme="light"] .task-workspace-utilities-icon{color:#4a729b}
  `;
  document.head.append(style);

  const launcher=document.createElement('button');
  launcher.type='button';
  launcher.className='task-activity-button task-workspace-utilities-launcher';
  launcher.dataset.view='workspace-utilities';
  launcher.title='Workspace snapshots · Operation Center · Project Health';
  launcher.setAttribute('aria-label','Workspace tools: Snapshots, Operation Center, Project Health');
  launcher.setAttribute('aria-haspopup','menu');
  launcher.setAttribute('aria-expanded','false');
  launcher.innerHTML='<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="4" width="7" height="7" rx="1"/><rect x="13" y="4" width="7" height="7" rx="1"/><rect x="4" y="13" width="7" height="7" rx="1"/><rect x="13" y="13" width="7" height="7" rx="1"/></svg>';

  const menu=document.createElement('div');
  menu.className='task-workspace-utilities-menu';
  menu.id='task-workspace-utilities-menu';
  menu.setAttribute('role','menu');
  menu.setAttribute('aria-label','Workspace tools');
  launcher.setAttribute('aria-controls',menu.id);
  const entries=[
    {name:'Workspace snapshots',description:'Save or restore workspace state',icon:'◫',open:()=>globalThis.TaskMenuWorkspaceSnapshots?.open?.()},
    {name:'Operation Center',description:'Track and manage operations',icon:'⇅',open:()=>globalThis.TaskMenuOperationCenter?.open?.()},
    {name:'Project Health',description:'Review project status and diagnostics',icon:'♥',open:()=>globalThis.TaskMenuProjectHealth?.open?.()}
  ];
  const items=entries.map(entry=>{
    const item=document.createElement('button');
    item.type='button';item.className='task-workspace-utilities-item';item.setAttribute('role','menuitem');
    const icon=document.createElement('span');icon.className='task-workspace-utilities-icon';icon.textContent=entry.icon;
    const copy=document.createElement('span');copy.className='task-workspace-utilities-copy';
    const label=document.createElement('strong');label.textContent=entry.name;
    const hint=document.createElement('small');hint.textContent=entry.description;
    copy.append(label,hint);item.append(icon,copy);
    item.onclick=()=>{
      closeMenu();
      try{Promise.resolve(entry.open()).catch(error=>app.showError?.(error));}
      catch(error){app.showError?.(error);}
    };
    menu.append(item);
    return item;
  });

  function positionMenu(){
    const rect=launcher.getBoundingClientRect();
    const inVerticalRail=document.body.classList.contains('task-sidebar-auto-hide');
    menu.style.visibility='hidden';
    menu.style.display='grid';
    const width=menu.offsetWidth,height=menu.offsetHeight;
    const desiredLeft=inVerticalRail?rect.right+7:rect.left;
    const desiredTop=inVerticalRail?rect.top:rect.bottom+6;
    const left=Math.max(8,Math.min(desiredLeft,window.innerWidth-width-8));
    const top=Math.max(8,Math.min(desiredTop,window.innerHeight-height-8));
    menu.style.left=left+'px';menu.style.top=top+'px';
    menu.style.removeProperty('display');menu.style.removeProperty('visibility');
  }
  function openMenu(){
    menu.classList.add('visible');launcher.classList.add('active');
    launcher.setAttribute('aria-expanded','true');
    positionMenu();items[0]?.focus();
  }
  function closeMenu({focus=false}={}){
    if(!menu.classList.contains('visible'))return;
    menu.classList.remove('visible');launcher.classList.remove('active');
    launcher.setAttribute('aria-expanded','false');
    if(focus)launcher.focus();
  }
  launcher.onclick=()=>menu.classList.contains('visible')?closeMenu({focus:true}):openMenu();
  menu.addEventListener('keydown',event=>{
    if(event.key==='Escape'){event.preventDefault();closeMenu({focus:true});return;}
    if(event.key==='Tab'){closeMenu();return;}
    if(event.key!=='ArrowDown'&&event.key!=='ArrowUp'&&event.key!=='Home'&&event.key!=='End')return;
    event.preventDefault();
    const index=items.indexOf(document.activeElement);
    const next=event.key==='Home'?0:event.key==='End'?items.length-1:
      event.key==='ArrowDown'?(index+1)%items.length:(index-1+items.length)%items.length;
    items[next]?.focus();
  });
  document.addEventListener('pointerdown',event=>{
    if(!menu.classList.contains('visible'))return;
    if(menu.contains(event.target)||launcher.contains(event.target))return;
    closeMenu();
  },true);
  document.addEventListener('keydown',event=>{
    if(event.key==='Escape'&&menu.classList.contains('visible')){event.preventDefault();closeMenu({focus:true});}
  });
  window.addEventListener('resize',()=>{if(menu.classList.contains('visible'))positionMenu();});
  window.addEventListener('scroll',()=>{if(menu.classList.contains('visible'))positionMenu();},true);

  rail.append(launcher);
  document.body.append(menu);
  globalThis.TaskMenuWorkspaceUtilities={launcher,menu,open:openMenu,close:closeMenu,get visible(){return menu.classList.contains('visible');}};
  return true;
}
function safeInstallWorkspaceUtilities(){
  try{installWorkspaceUtilitiesLauncher();}
  catch(error){console.warn('Workspace utilities activity launcher unavailable',error);}
}
if(!safeInstallWorkspaceUtilities()){
  const observer=new MutationObserver(()=>{
    if(installWorkspaceUtilitiesLauncher())observer.disconnect();
  });
  observer.observe(document.body,{childList:true,subtree:true});
}
