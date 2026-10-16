// Browser window + JavaScript dialog lifecycle. Everything remains within a
// single isolated Chromium context. No cookies, storage or page scripts cross
// the TaskDeck JSON boundary.
const MAX_WINDOWS=6;
const DIALOG_TIMEOUT_MS=60000;
export function browserWindow(session,id){
  const entry=session.windows.get(id);
  if(!entry||entry.page.isClosed())throw Object.assign(new Error('Browser window closed; refresh to continue'),{status:409});
  return entry;
}
export function registerWindow(session,page,{main=false,openerId=''}={}){
  const existing=[...session.windows.values()].find(w=>w.page===page);
  if(existing)return existing;
  if(session.windows.size>=MAX_WINDOWS&&!main){page.close().catch(()=>{});return null;}
  const id=main?'main':'popup-'+(++session.windowCounter);
  const entry={id,page,opener_id:openerId,kind:main?'main':'popup',opened_at:Date.now()};
  session.windows.set(id,entry);
  session.activeWindowId=id;session.page=page;
  session.windowEvent={type:'opened',window_id:id,kind:entry.kind};
  if(!main){
    page.opener().then(opener=>{
      if(opener){const origin=[...session.windows.values()].find(v=>v.page===opener);if(origin)entry.opener_id=origin.id;}
    }).catch(()=>{});
  }
  page.on('close',()=>{
    session.windows.delete(id);
    if(session.httpChallenge?.window_id===id){
      session.httpChallenge=null;
      session.httpAuthGeneration=(session.httpAuthGeneration||0)+1;
    }
    if(session.dialog?.window_id===id){session.dialogTimer&&clearTimeout(session.dialogTimer);session.dialogNative?.dismiss().catch(()=>{});session.dialog=null;session.dialogNative=null;}
    if(session.activeWindowId===id){
      const fallback=browserWindowOrNull(session,entry.opener_id)||browserWindowOrNull(session,'main')||[...session.windows.values()].at(-1);
      session.activeWindowId=fallback?.id||'';
      session.page=fallback?.page||null;
    }
    session.windowEvent={type:'closed',window_id:id,kind:entry.kind,returned_to:session.activeWindowId};
  });
  page.on('dialog',dialog=>{
    if(session.dialog){dialog.dismiss().catch(()=>{});return;}
    // The website can open a confirm in an inactive OAuth popup. Surface the
    // actual page that owns the blocking dialog before asking the user.
    session.activeWindowId=id;
    session.page=page;
    const type=dialog.type();
    const dialogID='dialog-'+(++session.dialogCounter);
    session.dialog={
      id:dialogID,window_id:id,type,
      // Never echo prompt defaults (may be secrets) or whole URLs.
      message:String(dialog.message()||'').slice(0,700)
    };
    session.dialogNative=dialog;
    session.dialogTimer=setTimeout(()=>{
      if(session.dialog?.id!==dialogID)return;
      session.dialog=null;session.dialogNative=null;
      dialog.dismiss().catch(()=>{});
      session.windowEvent={type:'dialog_timeout',window_id:id};
    },DIALOG_TIMEOUT_MS);
    session.dialogTimer.unref?.();
    for(const notify of session.dialogObservers)notify();
    session.dialogObservers.clear();
  });
  page.on('download',download=>download.cancel().catch(()=>{}));
  return entry;
}
function browserWindowOrNull(session,id){
  const w=session.windows.get(id);
  return w&&!w.page.isClosed()?w:null;
}
// OAuth providers often create a provisional about:blank popup and only then
// assign its HTTPS location. A snapshot taken on that same click otherwise
// reports an empty page. Wait briefly and report a bounded pending state;
// never rewrite the destination or attempt to bypass an identity provider.
export async function settlePopupNavigation(session,timeoutMs=2400){
  const entry=session.windows.get(session.activeWindowId);
  if(!entry||entry.kind!=='popup'||entry.page.isClosed())return {pending:false};
  if(entry.page.url()!=='about:blank')return {pending:false};
  if(session.blockedNavigation?.window_id===entry.id)return {pending:false};
  const age=Date.now()-entry.opened_at;
  if(age>=10000)return {pending:false,expired:true};
  const timeout=Math.max(0,Math.min(timeoutMs,10000-age));
  if(timeout>0){
    try{
      await entry.page.waitForURL(url=>String(url)!=='about:blank',{timeout});
    }catch{
      // The popup may be waiting for a website script, or its navigation may
      // have been blocked by the worker allowlist. Do not throw or deadlock.
    }
  }
  const pending=!entry.page.isClosed()&&entry.page.url()==='about:blank'&&
    session.blockedNavigation?.window_id!==entry.id;
  return {pending,expired:!pending&&entry.page.url()==='about:blank'};
}
export function activeWindow(session){
  const current=browserWindowOrNull(session,session.activeWindowId);
  if(current)return current;
  const fallback=browserWindowOrNull(session,'main')||[...session.windows.values()].find(w=>!w.page.isClosed());
  if(!fallback)throw Object.assign(new Error('No browser windows remain'),{status:409});
  session.activeWindowId=fallback.id;session.page=fallback.page;
  return fallback;
}
export function selectWindow(session,id){
  const entry=browserWindow(session,id);
  session.activeWindowId=entry.id;session.page=entry.page;
  session.windowEvent={type:'focused',window_id:id};
  return entry;
}
export async function closeWindow(session,id){
  const entry=browserWindow(session,id);
  if(entry.kind==='main')throw Object.assign(new Error('The primary browser window cannot be closed here'),{status:400});
  if(session.dialog?.window_id===id)await resolveDialog(session,{dialog_id:session.dialog.id,accept:false});
  await entry.page.close({runBeforeUnload:false});
}
export async function windowDescriptors(session,publicURL){
  const results=[];
  for(const entry of [...session.windows.values()].slice(0,MAX_WINDOWS)){
    if(entry.page.isClosed())continue;
    const title=session.dialog?'Website dialog':
      await entry.page.title().catch(()=>entry.kind==='main'?'Main':'Popup');
    results.push({
      id:entry.id,kind:entry.kind,opener_id:entry.opener_id||'',active:session.activeWindowId===entry.id,
      title:String(title|| (entry.kind==='main'?'Main':'Popup')).slice(0,100),
      url:publicURL(entry.page.url())
    });
  }
  return results;
}
export async function waitForActionOrDialog(session,fn){
  if(session.dialog)throw Object.assign(new Error('Respond to the pending website dialog first'),{status:409});
  if(session.pendingAction)throw Object.assign(new Error('A browser operation is still pending'),{status:409});
  let onDialog;
  const signaled=new Promise(resolve=>{onDialog=()=>resolve('dialog');session.dialogObservers.add(onDialog);});
  const running=Promise.resolve().then(fn);
  session.pendingAction=running;
  // A dismissed/timed-out dialog must never leave the session blocked.
  running.then(()=>{if(session.pendingAction===running)session.pendingAction=null;},
    ()=>{if(session.pendingAction===running)session.pendingAction=null;});
  // The action can stay blocked inside Playwright until a JS confirm/prompt
  // is answered. Return a dialog snapshot instead of waiting for click timeout.
  const result=await Promise.race([running.then(()=> 'done',()=> 'done'),signaled]);
  session.dialogObservers.delete(onDialog);
  if(result==='done'){
    session.pendingAction=null;
    return await running;
  }
  running.catch(()=>{}); // no unhandled rejection while dialog UI is visible
}
export async function resolveDialog(session,{dialog_id,accept,prompt_text}){
  if(!session.dialog||session.dialog.id!==dialog_id)
    throw Object.assign(new Error('Dialog expired or already handled'),{status:409});
  const native=session.dialogNative;
  const kind=session.dialog.type;
  const value=String(prompt_text||'');
  if(value.length>4096)throw Object.assign(new Error('Dialog input exceeds limit'),{status:413});
  clearTimeout(session.dialogTimer);
  session.dialog=null;session.dialogNative=null;
  if(accept===true){
    if(kind==='prompt')await native.accept(value);
    else await native.accept();
  }else await native.dismiss();
  // The callback may have opened the next dialog synchronously while
  // native.accept() was in flight. Surface it instead of waiting for click.
  if(session.dialog)return;
  const pending=session.pendingAction;
  if(pending){
    // A confirm may synchronously trigger another alert/prompt, which leaves
    // the original Playwright click unresolved. Wait for EITHER completion or
    // the next dialog, never wait indefinitely on the first click.
    let onNext;
    const nextDialog=new Promise(resolve=>{
      onNext=()=>resolve('next-dialog');
      session.dialogObservers.add(onNext);
    });
    const settled=pending.then(()=> 'done',()=> 'done');
    let abortWait;
    const guard=new Promise(resolve=>{
      abortWait=setTimeout(()=>resolve('still-loading'),12000);
      abortWait.unref?.();
    });
    try{
      // Do not hold the Go→Unix proxy connection beyond its 40s deadline
      // after a dialog answer. Long redirects can complete asynchronously.
      const state=await Promise.race([settled,nextDialog,guard]);
      if(state==='done'){
        try{await pending;}
        finally{if(session.pendingAction===pending)session.pendingAction=null;}
      }
    }finally{
      clearTimeout(abortWait);
      session.dialogObservers.delete(onNext);
    }
  }
}
export function createWindowSession(context,id){
  return {id,context,page:null,windows:new Map(),windowCounter:0,dialogCounter:0,
    activeWindowId:'',windowEvent:null,dialog:null,dialogNative:null,dialogTimer:null,
    dialogObservers:new Set(),pendingAction:null,revision:0,seen:Date.now(),queue:Promise.resolve(),snapshotFrames:[],
    blockedNavigation:null};
}
export async function disposeWindowSession(session){
  clearTimeout(session.dialogTimer);
  if(session.dialogNative)await session.dialogNative.dismiss().catch(()=>{});
  session.dialog=null;session.dialogNative=null;
  for(const waiter of session.dialogObservers)waiter();
  session.dialogObservers.clear();
}
