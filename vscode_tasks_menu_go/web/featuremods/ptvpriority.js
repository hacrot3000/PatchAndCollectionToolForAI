const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Patch Tool artifact priority');

const style=document.createElement('style');
style.textContent=`
.detected-actions.has-items{display:flex;flex-direction:column}
.detected-row.ptv-primary{background:#17150b;border-left:3px solid #d5ad3b;padding-left:6px}
.detected-row.ptv-primary .detected-kind{color:#f0c85a;opacity:1;font-weight:700}
`;
document.head.append(style);

function artifactRole(path){
  const normalized=String(path||'').replace(/\\/g,'/');
  const name=normalized.split('/').pop()||'';
  const inAliasDir=normalized.includes('/artifacts/ptv_to_ai/');

  if(inAliasDir){
    if(/^CR_[^/]+\.(?:zip|txt)$/i.test(name))return {label:'COLLECT',priority:1000};
    if(/^FH_[^/]+\.(?:zip|txt)$/i.test(name))return {label:'FAIL',priority:990};
    if(/^AS_[^/]+\.(?:zip|txt)$/i.test(name))return {label:'AI SYNC',priority:980};
    if(/^UP_[^/]+\.(?:zip|txt)$/i.test(name))return {label:'UPLOAD',priority:970};
    return {label:'PTV',priority:950};
  }

  if(/^CODE_COLLECTION_RESULT_[^/]+\.(?:zip|txt)$/i.test(name))return {label:'COLLECT',priority:900};
  if(/^FAIL_HANDOFF_[^/]+\.(?:zip|txt)$/i.test(name))return {label:'FAIL',priority:890};
  if(/^AI_TOOL_SYNC_RESULT_[^/]+\.(?:zip|txt)$/i.test(name))return {label:'AI SYNC',priority:880};
  if(/^MANUAL_EXECUTION_RESULT_[^/]+\.(?:zip|txt)$/i.test(name))return {label:'MANUAL',priority:870};
  return null;
}

function prioritizeBar(bar){
  const rows=[...bar.querySelectorAll(':scope > .detected-row')];
  for(const row of rows){
    const link=row.querySelector('.detected-link');
    const kind=row.querySelector('.detected-kind');
    const role=artifactRole(link?.textContent||link?.title||'');
    row.classList.toggle('ptv-primary',Boolean(role));
    const nextOrder=role?String(-role.priority):'0';
    if(row.style.order!==nextOrder)row.style.order=nextOrder;
    if(role&&kind&&kind.textContent!==role.label)kind.textContent=role.label;
  }
}

// Important: never observe document.body/subtree here. xterm mutates its DOM heavily and
// prioritizeBar itself updates labels, so a global subtree observer can recursively wake
// itself and starve the browser event loop. Observe only the direct children of each small
// detected-actions bar and coalesce work to one animation frame.
const watchedBars=new WeakSet();
const queuedBars=new WeakSet();

function scheduleBar(bar){
  if(!bar||!bar.isConnected||queuedBars.has(bar))return;
  queuedBars.add(bar);
  requestAnimationFrame(()=>{
    queuedBars.delete(bar);
    if(bar.isConnected)prioritizeBar(bar);
  });
}

function watchBar(bar){
  if(!bar||watchedBars.has(bar))return;
  watchedBars.add(bar);
  const observer=new MutationObserver(()=>scheduleBar(bar));
  observer.observe(bar,{childList:true});
  scheduleBar(bar);
}

function watchView(view){
  const bar=view?.pane?.querySelector?.('.detected-actions');
  if(bar)watchBar(bar);
}

function attachView(view){
  if(!view)return;
  watchView(view);
  // The detector and this feature are separate modules. Retry once after the current
  // event turn in case the bar was created by another taskmenu:session listener.
  setTimeout(()=>watchView(view),0);
}

window.addEventListener('taskmenu:session',event=>attachView(event.detail?.view));
for(const view of app.views.values())attachView(view);
