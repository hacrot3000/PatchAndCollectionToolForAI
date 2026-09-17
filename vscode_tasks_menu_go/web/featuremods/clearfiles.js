const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for clear detected files');

const installed=new WeakSet();

function placeButton(view,button){
  const head=view?.pane?.querySelector('.pane-head');
  if(!head)return;
  const host=head.querySelector('.pane-action-menus');
  if(host){
    const sessionMenu=host.querySelector('.taskmenu-menu');
    if(sessionMenu){sessionMenu.after(button);return;}
    host.prepend(button);return;
  }
  head.append(button);
}

function bindDetectedBar(view,button){
  const bar=view?.pane?.querySelector('.detected-actions')||null;
  if(button._detectedBar===bar)return bar;
  button._detectedBarObserver?.disconnect();
  button._detectedBarObserver=null;
  button._detectedBar=bar;
  if(bar){
    const observer=new MutationObserver(()=>updateButton(view,button));
    observer.observe(bar,{childList:true});
    button._detectedBarObserver=observer;
  }
  return bar;
}

function updateButton(view,button){
  const bar=bindDetectedBar(view,button);
  const hasFiles=Boolean(bar?.querySelector('.detected-ignore'));
  button.hidden=!hasFiles;
  button.disabled=!hasFiles;
  button.title=hasFiles?'Ignore toàn bộ file download đang được detect trong session này':'Không có file download để clear';
}

function clearDetectedFiles(view,button){
  const bar=bindDetectedBar(view,button);
  if(!bar)return;
  // Ignore is the source of truth for this session. Clicking the existing
  // handler also persists the ignored path in sessionStorage, so a later scan
  // cannot immediately add the same file back. render() is synchronous and
  // reveals the next rows as earlier rows disappear.
  let count=0;
  for(let guard=0;guard<1024;guard++){
    const ignore=bar.querySelector('.detected-ignore');
    if(!ignore)break;
    ignore.click();count++;
  }
  updateButton(view,button);
  if(count){
    const old=button.textContent;button.textContent='Cleared '+count;
    setTimeout(()=>{if(button.isConnected)button.textContent=old;},1000);
  }
}

function install(view){
  if(!view?.pane||installed.has(view))return;
  installed.add(view);
  const button=document.createElement('button');
  button.className='detected-clear-all';
  button.textContent='Clear files';
  button.hidden=true;
  button.onclick=()=>clearDetectedFiles(view,button);
  placeButton(view,button);

  const paneObserver=new MutationObserver(()=>{
    placeButton(view,button);
    updateButton(view,button);
  });
  paneObserver.observe(view.pane,{childList:true});
  button._detectedFilesObserver=paneObserver;
  updateButton(view,button);
}

window.addEventListener('taskmenu:session',event=>{
  const view=event.detail?.view;
  if(view)setTimeout(()=>install(view),0);
});
window.addEventListener('taskmenu:output',event=>{
  const view=event.detail?.view;
  if(!view)return;
  const button=view.pane?.querySelector('.detected-clear-all');
  if(button)setTimeout(()=>updateButton(view,button),260);
});
for(const view of app.views.values())install(view);
