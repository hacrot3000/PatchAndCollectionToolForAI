const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for detected file controls');

const installed=new WeakSet();
const style=document.createElement('style');
style.textContent=`
.detected-file-controls{display:inline-flex;align-items:center;gap:4px}
.detected-files-toggle.off{opacity:.72;background:#34281b;border-color:#6b4f31}
`;
document.head.append(style);

function placeControls(view,controls){
  const head=view?.pane?.querySelector('.pane-head');
  if(!head)return;
  const host=head.querySelector('.pane-action-menus');
  if(host){
    const sessionMenu=host.querySelector('.taskmenu-menu');
    if(sessionMenu){sessionMenu.after(controls);return;}
    host.prepend(controls);return;
  }
  head.append(controls);
}

function bindDetectedBar(view,button){
  const bar=view?.pane?.querySelector('.detected-actions')||null;
  if(button._detectedBar===bar)return bar;
  button._detectedBarObserver?.disconnect();
  button._detectedBarObserver=null;
  button._detectedBar=bar;
  if(bar){
    const observer=new MutationObserver(()=>updateClearButton(view,button));
    observer.observe(bar,{childList:true});
    button._detectedBarObserver=observer;
  }
  return bar;
}

function updateClearButton(view,button){
  const bar=bindDetectedBar(view,button);
  const hasItems=Boolean(bar?.querySelector('.detected-ignore'));
  button.hidden=!hasItems;
  button.disabled=!hasItems;
  button.title=hasItems?'Ignore all detected files and URLs in this session':'No detected files or URLs to clear';
}

function updateToggle(view,button){
  const api=globalThis.TaskMenuFileDetection;
  const enabled=api?.isEnabled?.(view)!==false;
  button.textContent=enabled?'Files/URLs: ON':'Files/URLs: OFF';
  button.classList.toggle('off',!enabled);
  button.setAttribute('aria-pressed',enabled?'true':'false');
  button.title=enabled
    ?'File and URL detection is enabled for this tab. Click to disable both.'
    :'File and URL detection is disabled for this tab. Click to enable both.';
}

function clearDetectedItems(view,button){
  const bar=bindDetectedBar(view,button);
  if(!bar)return;
  // Ignore is the source of truth for this session. Clicking each row's
  // handler persists its ignored file/URL in sessionStorage, so later output
  // cannot immediately add the same item back. render() is synchronous and
  // reveals the next rows as earlier rows disappear.
  let count=0;
  for(let guard=0;guard<1024;guard++){
    const ignore=bar.querySelector('.detected-ignore');
    if(!ignore)break;
    ignore.click();count++;
  }
  updateClearButton(view,button);
  if(count){
    const old=button.textContent;button.textContent='Cleared '+count;
    setTimeout(()=>{if(button.isConnected)button.textContent=old;},1000);
  }
}

function install(view){
  if(!view?.pane||installed.has(view))return;
  installed.add(view);

  const controls=document.createElement('span');
  controls.className='detected-file-controls';
  const toggle=document.createElement('button');
  toggle.className='detected-files-toggle';
  const clear=document.createElement('button');
  clear.className='detected-clear-all';
  clear.textContent='Clear detected';
  clear.hidden=true;
  controls.append(toggle,clear);

  toggle.onclick=()=>{
    const api=globalThis.TaskMenuFileDetection;
    if(!api?.setEnabled||!api?.isEnabled){app.showError(new Error('File detection control unavailable'));return;}
    api.setEnabled(view,!api.isEnabled(view));
    updateToggle(view,toggle);
    updateClearButton(view,clear);
  };
  clear.onclick=()=>clearDetectedItems(view,clear);
  placeControls(view,controls);

  const paneObserver=new MutationObserver(()=>{
    placeControls(view,controls);
    updateToggle(view,toggle);
    updateClearButton(view,clear);
  });
  paneObserver.observe(view.pane,{childList:true});
  controls._detectedFilesObserver=paneObserver;
  updateToggle(view,toggle);
  updateClearButton(view,clear);
}

window.addEventListener('taskmenu:file-detection-changed',event=>{
  const view=event.detail?.view;
  if(!view)return;
  const toggle=view.pane?.querySelector('.detected-files-toggle');
  const clear=view.pane?.querySelector('.detected-clear-all');
  if(toggle)updateToggle(view,toggle);
  if(clear)updateClearButton(view,clear);
});
window.addEventListener('taskmenu:session',event=>{
  const view=event.detail?.view;
  if(view)setTimeout(()=>install(view),0);
});
window.addEventListener('taskmenu:output',event=>{
  const view=event.detail?.view;
  if(!view)return;
  const clear=view.pane?.querySelector('.detected-clear-all');
  if(clear)setTimeout(()=>updateClearButton(view,clear),260);
});
for(const view of app.views.values())install(view);
