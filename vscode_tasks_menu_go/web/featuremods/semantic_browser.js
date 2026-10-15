// Semantic Browser frontend. Remote page scripts and HTML MUST NOT execute in
// TaskDeck: only bounded, typed JSON becomes local textContent/native controls.
const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Semantic Browser');
const views=new Map();
const tabsHost=document.querySelector('#tabs'),panesHost=document.querySelector('#panes');
let browserEnabled=false;
const style=document.createElement('style');
style.textContent=`
.semantic-browser-tab{border-radius:6px 6px 0 0;border-bottom:0;margin-left:4px}
.semantic-browser-tab.active{background:#343b48}
.semantic-browser-tab .close{margin-left:8px;opacity:.75}
.semantic-browser-pane{background:#10151c;color:inherit}
.semantic-browser-toolbar{display:flex;align-items:center;gap:5px;padding:7px;border-bottom:1px solid #30343b}
.semantic-browser-toolbar button{padding:5px 8px}
.semantic-browser-address{flex:1;min-width:110px;padding:7px;border-radius:6px;background:#090e15;color:inherit;border:1px solid #465064;font:12px ui-monospace,monospace}
.semantic-browser-content{flex:1;overflow:auto;min-height:0;padding:16px 20px;font:13px/1.6 system-ui,sans-serif}
.semantic-browser-entry{margin:6px 0;max-width:100%;overflow-wrap:anywhere}
.semantic-browser-action{display:inline-flex;max-width:100%;padding:5px 9px;text-align:left;white-space:normal;align-items:center;gap:6px}
.semantic-browser-link{color:#84bfff;background:transparent;border:0;text-decoration:underline;padding:1px 2px}
.semantic-browser-field{display:flex;flex-direction:column;max-width:720px;gap:4px;margin:8px 0}
.semantic-browser-field input,.semantic-browser-field textarea,.semantic-browser-field select{background:#0b111a;color:inherit;border:1px solid #465064;border-radius:5px;padding:7px;min-width:120px}
.semantic-browser-field textarea{min-height:75px;resize:vertical;font:12px/1.45 ui-monospace,monospace}
.semantic-browser-field label,.semantic-browser-field>span{font-size:11px;opacity:.8}
.semantic-browser-heading{font-size:17px;font-weight:700;margin:15px 0 6px}
.semantic-browser-muted{opacity:.64;font-size:11px}
.semantic-browser-error{color:#ff9a9a;white-space:pre-wrap}
.semantic-browser-busy{opacity:.75}
.semantic-browser-windows{display:flex;gap:5px;align-items:center;flex-wrap:wrap;padding:5px 8px;border-bottom:1px solid #30343b}
.semantic-browser-window{font-size:11px;padding:5px 8px;max-width:230px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.semantic-browser-window.active{border-color:#659fda;background:#254363}
.semantic-browser-window-close{font-size:12px;padding:4px 8px}
.semantic-browser-dialog{padding:12px;border:1px solid #65778c;border-radius:7px;background:#17212d;margin:10px 0}
.semantic-browser-dialog strong{display:block;margin-bottom:6px}
.semantic-browser-dialog-text{white-space:pre-wrap;word-break:break-word}
.semantic-browser-dialog-actions{display:flex;gap:7px;margin-top:8px}
.semantic-browser-dialog input{padding:7px;background:#0b111a;color:inherit;border:1px solid #465064;max-width:100%;width:100%;margin-top:8px}
.semantic-browser-popup-status{font-size:11px;opacity:.78;padding:6px 8px}
.semantic-browser-auth-help{padding:8px 10px;border-left:3px solid #b49a67;font-size:11px;margin:8px 0;background:rgba(180,154,103,.1)}
html[data-taskmenu-theme="light"] .semantic-browser-pane{background:#fff;color:#202124}
html[data-taskmenu-theme="light"] .semantic-browser-address,html[data-taskmenu-theme="light"] .semantic-browser-field input,html[data-taskmenu-theme="light"] .semantic-browser-field textarea,html[data-taskmenu-theme="light"] .semantic-browser-field select{background:#f8fafc;color:#202124;border-color:#bbc4cc}
`;
document.head.append(style);
function el(tag,cls='',text=''){const node=document.createElement(tag);if(cls)node.className=cls;if(text)node.textContent=text;return node;}
function button(text,fn,cls=''){const node=el('button',cls,text);node.type='button';node.onclick=fn;return node;}
const openButton=button('Browser',()=>createView());
openButton.title='Semantic Browser (page JavaScript runs only on server)';
openButton.hidden=true;
document.querySelector('#open-terminal')?.after(openButton);
function storageKey(){
  return ['taskdeck:semantic-browser:v1',app.taskData?.workspace||'',app.currentUser?.user_id||'local'].map(encodeURIComponent).join(':');
}
function persist(){
  const state=[...views.values()].filter(v=>v.sessionID).map(v=>({id:v.id,session_id:v.sessionID}));
  try{sessionStorage.setItem(storageKey(),JSON.stringify(state));}catch{}
}
function errorOf(error){return String(error?.message||error||'Unknown browser error').slice(0,500);}
function showError(view,error){
  if(!view||!view.pane.isConnected){app.showError?.(error);return;}
  view.error.hidden=false;view.error.textContent=errorOf(error);
}
function showBusy(view,active){
  view.loading=Boolean(active);
  for(const button of [view.back,view.forward,view.reload,view.goto])button.disabled=active||(!view.sessionID&&button!==view.goto);
  view.address.disabled=active;
  view.status.textContent=active?'Loading…':String(view.page?.title||'Semantic browser');
  view.pane.classList.toggle('semantic-browser-busy',Boolean(active));
}
function setCurrent(view){
  if(app.activateExternalView('semantic-browser:'+view.id,{force:true})===false)return;
  for(const item of views.values()){
    item.tab.classList.toggle('active',item===view);
    item.pane.classList.toggle('hidden',item!==view);
  }
}
window.addEventListener('taskmenu:view-activated',event=>{
  if(event.detail?.kind==='external'&&String(event.detail.id||'').startsWith('semantic-browser:'))return;
  for(const view of views.values()){
    view.tab.classList.remove('active');
    view.pane.classList.add('hidden');
  }
});
function clearAuto(view){clearTimeout(view.autoTimer);view.autoTimer=0;}
function controlKey(item){return String(item.frame_index||0)+'/'+String(item.id);}
function rememberInput(view,item,value){
  clearAuto(view); // A secret field must cancel an earlier username timer.
  view.dirty.set(controlKey(item),{element_id:item.id,frame_index:item.frame_index||0,kind:'fill',value:String(value)});
  if(!item.sensitive&&!view.composing)scheduleAuto(view);
}
function scheduleAuto(view){
  clearAuto(view);
  // Sync nonsecret inputs after a pause so autocomplete/AJAX sites can react.
  // Never auto-send passwords or one-time codes.
  if([...view.dirty.values()].some(item=>view.page?.elements?.find(e=>e.id===item.element_id&&(e.frame_index||0)===(item.frame_index||0))?.sensitive))return;
  view.autoTimer=setTimeout(()=>{
    if([...view.dirty.values()].some(field=>
      view.page?.elements?.find(e=>e.id===field.element_id&&(e.frame_index||0)===(field.frame_index||0))?.sensitive))return;
    enqueue(view,'interact',{target_action:'',auto:true});
  },650);
}
function ensureFresh(view){
  if(!view.tab.isConnected)throw new Error('Browser tab was closed');
  if(!view.sessionID)throw new Error('Navigate to a website first');
}
function pendingUpdates(view){
  return [...view.dirty.values()].map(item=>({...item}));
}
// Each user gesture and snapshot is serialized per tab. Unlike the earlier
// implementation, clicking Submit cannot be dropped while blur syncs fields.
function enqueue(view,action,params={}){
  clearAuto(view);
  const task=async()=>{
    if(!view.tab.isConnected)return null;
    const isNavigate=['navigate','back','forward','reload'].includes(action);
    if(action!=='navigate')ensureFresh(view);
    const maySubmit=action==='interact'&&['click','press'].includes(params.target_action);
    const updates=action==='interact'?pendingUpdates(view).filter(field=>
      maySubmit||!view.page?.elements?.find(e=>
        e.id===field.element_id&&(e.frame_index||0)===(field.frame_index||0))?.sensitive):[];
    showBusy(view,true);view.error.hidden=true;
    try{
      const {auto,...remoteParams}=params; // local-only marker: never forward to Go
      let payload={action,...remoteParams};
      if(view.sessionID){
        payload.session_id=view.sessionID;
        if(view.page?.revision)payload.revision=view.page.revision;
      }else if(action==='navigate')payload={action:'create',url:params.url};
      if(action==='interact')payload.field_updates=updates;
      const page=await app.jsonFetch('/api/semantic-browser',{
        method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)
      });
      if(!view.tab.isConnected)return null;
      if(page.session_id)view.sessionID=String(page.session_id);
      const previousWindow=view.page?.active_window_id;
      view.page=page;
      if(previousWindow&&page.active_window_id&&previousWindow!==page.active_window_id){
        // Changes are scoped to a real browser window. Never apply a draft
        // password or form update to a different OAuth/popup window.
        view.dirty.clear();
      }
      if(page.url)view.address.value=String(page.url);
      view.title.textContent='🌐 '+String(page.title||page.url||'Browser').slice(0,32);
      if(action==='interact')for(const field of updates){
        // Don't discard a newer edit typed while the request was in flight.
        const now=view.dirty.get(String(field.frame_index||0)+'/'+field.element_id);
        if(now?.value===field.value&&now?.kind===field.kind)view.dirty.delete(String(field.frame_index||0)+'/'+field.element_id);
      }
      if(isNavigate)view.dirty.clear();
      persist();
      render(view);
      return page;
    }catch(error){
      showError(view,error);
      return null;
    }finally{showBusy(view,false);}
  };
  const promise=view.queue.then(task,task);
  view.queue=promise.catch(()=>null);
  return promise;
}
function focusBrowserWindow(view,id){
  if(view.page?.active_window_id===id)return;
  if(view.dirty.size&&!window.confirm('Unsaved inputs in this window will be discarded. Switch windows?'))return;
  enqueue(view,'switch_window',{window_id:id});
}
function renderBrowserWindows(view,page){
  view.windows.replaceChildren();
  const windows=Array.isArray(page.windows)?page.windows:[];
  if(windows.length<=1){view.windows.hidden=true;return;}
  view.windows.hidden=false;
  for(const win of windows){
    if(!win?.id||!/^main$|^popup-[0-9]+$/.test(win.id))continue;
    const item=button((win.kind==='main'?'Main':'Popup')+' · '+String(win.title||'Website').slice(0,34),
      ()=>focusBrowserWindow(view,win.id),'semantic-browser-window');
    item.classList.toggle('active',Boolean(win.active));
    item.title=String(win.url||'').slice(0,280);
    view.windows.append(item);
    if(win.kind==='popup'){
      const close=button('×',()=>{
        if(view.dirty.size&&!window.confirm('Close popup and discard unsaved input?'))return;
        enqueue(view,'close_window',{window_id:win.id});
      },'semantic-browser-window-close');
      close.title='Close this popup window';view.windows.append(close);
    }
  }
}
function renderHTTPAuthentication(view,page){
  const challenge=page.auth_challenge;
  if(!challenge?.origin)return false;
  const panel=el('div','semantic-browser-dialog');
  panel.setAttribute('role','group');
  panel.setAttribute('aria-label','HTTP authentication challenge');
  panel.append(el('strong','','Website HTTP authentication'));
  panel.append(el('div','semantic-browser-dialog-text',
    'Sign in to '+String(challenge.origin).slice(0,180)+' (HTTP Basic). Credentials will be sent only to this website origin.'));
  if(challenge.window_id&&page.active_window_id!==challenge.window_id){
    panel.append(el('div','semantic-browser-muted','This challenge belongs to another browser window.'));
    const buttons=el('div','semantic-browser-dialog-actions');
    buttons.append(button('Open challenged window',()=>focusBrowserWindow(view,challenge.window_id)));
    buttons.append(button('Dismiss',()=>enqueue(view,'http_auth_cancel')));
    panel.append(buttons);view.content.append(panel);return true;
  }
  const user=el('input');user.type='text';user.autocomplete='username';
  user.placeholder='Username';user.setAttribute('aria-label','HTTP username');
  const pass=el('input');pass.type='password';pass.autocomplete='off';
  pass.placeholder='Password';pass.setAttribute('aria-label','HTTP password');
  panel.append(user,pass);
  const actions=el('div','semantic-browser-dialog-actions');
  const login=button('Sign in',()=>{
    const username=user.value,password=pass.value;
    // Do not cache the password in Tab state, localStorage or snapshot.
    pass.value='';
    enqueue(view,'http_auth',{http_username:username,http_password:password});
  });
  pass.onkeydown=event=>{if(event.key==='Enter'){event.preventDefault();login.click();}};
  const cancel=button('Cancel',()=>enqueue(view,'http_auth_cancel'));
  actions.append(login,cancel);panel.append(actions);
  view.content.append(panel);return true;
}
function renderBrowserDialog(view,page){
  const dialog=page.dialog;
  if(!dialog?.id)return false;
  const panel=el('div','semantic-browser-dialog');
  panel.setAttribute('role','alertdialog');
  panel.setAttribute('aria-modal','true');
  panel.append(el('strong','','Website '+String(dialog.type||'dialog')));
  panel.append(el('div','semantic-browser-dialog-text',String(dialog.message||'').slice(0,700)));
  let answer=null;
  if(dialog.type==='prompt'){
    answer=el('input');answer.type='text';answer.autocomplete='off';
    answer.setAttribute('aria-label','Website prompt answer');panel.append(answer);
  }
  const actions=el('div','semantic-browser-dialog-actions');
  const cancel=button(dialog.type==='alert'?'Dismiss':'Cancel',
    ()=>enqueue(view,'dialog_respond',{dialog_id:dialog.id,accept:false}));
  const ok=button('OK',()=>enqueue(view,'dialog_respond',{
    dialog_id:dialog.id,accept:true,prompt_text:answer?.value||''
  }));
  actions.append(ok,cancel);panel.append(actions);
  view.content.append(panel);
  ok.focus({preventScroll:true});
  return true;
}
function restoreFocus(view){
  const focused=document.activeElement;
  if(!focused||!view.content.contains(focused))return null;
  const id=focused.dataset.semanticId;
  if(!id)return null;
  let start=null,end=null;
  try{start=focused.selectionStart;end=focused.selectionEnd;}catch{}
  return {id,start,end};
}
function render(view){
  if(view.composing){view.renderDeferred=true;return;}
  const state=restoreFocus(view);
  const scroll=view.content.scrollTop;
  view.content.replaceChildren();
  const page=view.page;
  if(!page){
    view.content.append(el('p','semantic-browser-muted','Enter an HTTPS URL. All page scripts run in server-side Chromium.'));
    return;
  }
  renderBrowserWindows(view,page);
  if(page.window_event?.type==='closed')view.content.append(el('div','semantic-browser-popup-status','Popup closed. Returned to the website window.'));
  if(page.window_event?.type==='dialog_timeout')view.content.append(el('div','semantic-browser-popup-status','Website dialog timed out and was dismissed.'));
  view.content.append(el('div','semantic-browser-heading',String(page.title||page.url||'Website')));
  view.content.append(el('p','semantic-browser-muted',String(page.url||'')));
  if(/accounts\.google\.com|login\.microsoftonline\.com/i.test(page.url||'')){
    view.content.append(el('div','semantic-browser-auth-help',
      'This identity provider may block controlled browsers. Do not bypass provider security. An approved external-browser login requires support from the website; external cookies cannot be imported into this session.'));
  }
  if(renderBrowserDialog(view,page))return;
  if(renderHTTPAuthentication(view,page))return;
  const entries=Array.isArray(page.elements)?page.elements:[];
  for(const item of entries){
    if(!item?.id||!/^td-sem-[0-9a-z]+$/.test(item.id))continue;
    const row=el('div','semantic-browser-entry');
    const name=String(item.text||'').slice(0,400);
    const key=controlKey(item);
    if(item.kind==='action'){
      const link=item.tag==='a'||item.role==='link';
      const control=button(name||'Interactive element',()=>{
        enqueue(view,'interact',{target_action:'click',element_id:item.id,frame_index:item.frame_index||0});
      },link?'semantic-browser-link':'semantic-browser-action');
      control.disabled=Boolean(item.disabled);
      row.append(control);
    }else if(item.kind==='input'){
      const wrap=el('div','semantic-browser-field');
      const label=el('label','',name||item.placeholder||'Input');
      // Native password inputs mask reliably in Firefox; textareas do not.
      const multiline=!item.sensitive&&Boolean(item.multiline||item.tag==='textarea');
      const input=el(multiline?'textarea':'input');
      if(!multiline)input.type=item.input_type==='password'||item.sensitive?'password':
        ['number','email','search','tel','url'].includes(item.input_type)?item.input_type:'text';
      input.dataset.semanticId=key;
      const dirty=view.dirty.get(key);
      input.value=dirty?.value??String(item.value||'');
      input.placeholder=String(item.placeholder||'');
      input.disabled=Boolean(item.disabled);
      input.setAttribute('aria-label',name||'Text field');
      input.addEventListener('compositionstart',()=>{view.composing=true;clearAuto(view);});
      input.addEventListener('compositionend',()=>{
        view.composing=false;rememberInput(view,item,input.value);
        if(view.renderDeferred){view.renderDeferred=false;render(view);}
      });
      input.addEventListener('input',()=>{rememberInput(view,item,input.value);});
      input.addEventListener('keydown',event=>{
        if(event.isComposing||view.composing)return;
        if(event.key==='Enter'&&(!multiline||event.ctrlKey||event.metaKey)){
          event.preventDefault();
          // Single-line Enter or Ctrl/Cmd+Enter in editors: flush all fields
          // before the authentic target receives the browser key event.
          enqueue(view,'interact',{target_action:'press',element_id:item.id,frame_index:item.frame_index||0,key:'Enter'});
        }
      });
      wrap.append(label,input);row.append(wrap);
    }else if(item.kind==='check'){
      const wrap=el('label','semantic-browser-entry');
      const input=el('input');input.type=item.input_type==='radio'?'radio':'checkbox';
      input.checked=Boolean(view.dirty.get(key)?.checked??item.checked);
      input.disabled=Boolean(item.disabled);
      input.onchange=()=>{
        view.dirty.set(key,{element_id:item.id,frame_index:item.frame_index||0,kind:'check',value:'',checked:input.checked});
        enqueue(view,'interact',{target_action:''});
      };
      wrap.append(input,document.createTextNode(' '+name));row.append(wrap);
    }else if(item.kind==='select'){
      const wrap=el('label','semantic-browser-field');
      wrap.append(el('span','',name||'Select'));
      const select=el('select');
      for(const option of item.options||[]){
        const entry=el('option','',String(option.text||''));entry.value=String(option.value||'');select.append(entry);
      }
      select.value=String(view.dirty.get(key)?.value??item.value??'');
      select.disabled=Boolean(item.disabled);
      select.onchange=()=>{
        view.dirty.set(key,{element_id:item.id,frame_index:item.frame_index||0,kind:'select',value:select.value});
        enqueue(view,'interact',{target_action:''});
      };
      wrap.append(select);row.append(wrap);
    }else {
      row.textContent=name;
      if(/^h[1-6]$/.test(item.tag||''))row.classList.add('semantic-browser-heading');
    }
    view.content.append(row);
  }
  if(page.truncated)view.content.append(el('p','semantic-browser-muted','Page element limit reached; complex visual content may be omitted.'));
  if(!entries.length)view.content.append(el('p','semantic-browser-muted','No textual controls detected yet. Try Refresh.'));
  view.content.scrollTop=scroll;
  if(state){
    const replacement=[...view.content.querySelectorAll('[data-semantic-id]')].find(node=>node.dataset.semanticId===state.id);
    if(replacement){
      replacement.focus({preventScroll:true});
      if(state.start!==null){try{replacement.setSelectionRange(state.start,state.end);}catch{}}
    }
  }
}
async function closeView(view){
  clearAuto(view);views.delete(view.id);view.tab.remove();view.pane.remove();persist();
  if(view.sessionID){
    try{await app.jsonFetch('/api/semantic-browser',{
      method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:view.sessionID})
    });}catch(error){console.warn('Semantic Browser session close failed',errorOf(error));}
  }
  document.querySelector('#tabs > button:not(.hidden)')?.click();
}
function createView({id='',sessionID=''}={}){
  id=id||'semantic-browser:'+(globalThis.crypto?.randomUUID?.()||Date.now()+'-'+Math.random().toString(36).slice(2));
  const tab=el('button','tab semantic-browser-tab');tab.type='button';tab.dataset.id=id;
  const title=el('span','','🌐 Browser'),close=el('span','close','×');
  tab.append(title,close);tabsHost.append(tab);
  const pane=el('div','pane semantic-browser-pane hidden');pane.dataset.id=id;
  const toolbar=el('div','semantic-browser-toolbar');
  const back=button('←',()=>enqueue(view,'back'));
  const forward=button('→',()=>enqueue(view,'forward'));
  const reload=button('↻',()=>enqueue(view,'reload'));
  const address=el('input','semantic-browser-address');address.type='url';address.placeholder='https://example.com';
  const goto=button('Go',()=>enqueue(view,'navigate',{url:address.value.trim()}));
  address.onkeydown=event=>{if(event.key==='Enter'){event.preventDefault();goto.click();}};
  toolbar.append(back,forward,reload,address,goto);
  const status=el('div','semantic-browser-muted','Enter a URL');status.style.padding='5px 12px';
  const error=el('div','semantic-browser-error');error.hidden=true;error.style.padding='6px 12px';
  const windows=el('div','semantic-browser-windows');windows.hidden=true;
  const content=el('div','semantic-browser-content');
  pane.append(toolbar,windows,status,error,content);panesHost.append(pane);
  const view={id,sessionID,page:null,tab,pane,title,back,forward,reload,address,goto,windows,status,error,content,
    queue:Promise.resolve(),loading:false,dirty:new Map(),autoTimer:0,composing:false,renderDeferred:false};
  views.set(id,view);
  tab.onclick=()=>setCurrent(view);
  close.onclick=event=>{event.stopPropagation();closeView(view).catch(err=>showError(view,err));};
  showBusy(view,false);render(view);setCurrent(view);persist();
  return view;
}
async function restore(){
  let data=[];
  try{const saved=JSON.parse(sessionStorage.getItem(storageKey())||'[]');if(Array.isArray(saved))data=saved.slice(0,4);}catch{}
  for(const item of data){
    if(!item?.id||!item?.session_id)continue;
    const view=createView({id:String(item.id),sessionID:String(item.session_id)});
    await enqueue(view,'snapshot');
    if(!view.page){views.delete(view.id);view.tab.remove();view.pane.remove();}
  }
  persist();
}
async function initialize(){
  try{
    const status=await app.jsonFetch('/api/semantic-browser',{cache:'no-store'});
    browserEnabled=Boolean(status?.enabled)&&app.hasPermission('browser.use');
  }catch{browserEnabled=false;}
  openButton.hidden=!browserEnabled;
  if(browserEnabled)await restore();
}
openButton.onclick=()=>{if(browserEnabled)createView();};
setInterval(()=>{
  for(const view of views.values()){
    if(!view.sessionID||view.loading||!view.tab.classList.contains('active')||view.dirty.size||view.composing||view.page?.dialog||view.page?.auth_challenge)continue;
    const focus=document.activeElement;
    if(focus&&view.content.contains(focus)&&['INPUT','TEXTAREA','SELECT'].includes(focus.tagName))continue;
    enqueue(view,'snapshot');
  }
},4000);
const ready=new Promise(resolve=>{
  if(app.taskData?.workspace)initialize().finally(resolve);
  else window.addEventListener('taskmenu:tasks',()=>initialize().finally(resolve),{once:true});
});
globalThis.TaskMenuSemanticBrowser={views,open:()=>browserEnabled?createView():null,ready};
