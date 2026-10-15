// Lightweight semantic viewer. Never inserts remote HTML or executes page JS.
// All clicks, form events and JavaScript run in the isolated Playwright worker.
const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp is unavailable for Semantic Browser');
const browserViews=new Map();
let browserEnabled=false;
const tabHost=document.querySelector('#tabs'),paneHost=document.querySelector('#panes');
const css=document.createElement('style');
css.textContent=`
.semantic-browser-tab{border-radius:6px 6px 0 0;border-bottom:0;margin-left:4px}
.semantic-browser-tab.active{background:#343b48}
.semantic-browser-tab .close{margin-left:8px;opacity:.7}
.semantic-browser-pane{background:#10151c;color:inherit}
.semantic-browser-toolbar{display:flex;gap:5px;align-items:center;padding:7px;border-bottom:1px solid #30343b;flex-wrap:wrap}
.semantic-browser-toolbar button{padding:5px 8px}
.semantic-browser-address{flex:1;min-width:140px;background:#090e15;color:inherit;border:1px solid #455064;border-radius:6px;padding:7px;font:12px ui-monospace,monospace}
.semantic-browser-content{flex:1;overflow:auto;min-height:0;padding:16px 20px;font:13px/1.6 system-ui,sans-serif}
.semantic-browser-entry{margin:6px 0;max-width:100%;overflow-wrap:anywhere}
.semantic-browser-action{display:inline-flex;max-width:100%;padding:5px 9px;margin:2px 0;text-align:left;white-space:normal;align-items:center;gap:6px}
.semantic-browser-link{color:#84bfff;background:transparent;border:0;text-decoration:underline;padding:1px 2px}
.semantic-browser-field{display:flex;flex-direction:column;max-width:650px;gap:3px;margin:8px 0}
.semantic-browser-field input,.semantic-browser-field textarea,.semantic-browser-field select{background:#0b111a;color:inherit;border:1px solid #465064;border-radius:5px;padding:7px;min-width:120px}
.semantic-browser-field textarea{min-height:64px}
.semantic-browser-field label{font-size:11px;opacity:.8}
.semantic-browser-heading{font-size:17px;font-weight:700;margin:15px 0 6px}
.semantic-browser-muted{opacity:.64;font-size:11px}
.semantic-browser-error{color:#ff9a9a;white-space:pre-wrap}
html[data-taskmenu-theme="light"] .semantic-browser-pane{background:#fff;color:#202124}
html[data-taskmenu-theme="light"] .semantic-browser-address,html[data-taskmenu-theme="light"] .semantic-browser-field input,html[data-taskmenu-theme="light"] .semantic-browser-field textarea,html[data-taskmenu-theme="light"] .semantic-browser-field select{background:#f8fafc;color:#202124;border-color:#bbc4cc}
`;
document.head.append(css);
const openButton=document.createElement('button');
openButton.type='button';openButton.textContent='Browser';openButton.title='Open a semantic web browser tab (JavaScript runs only on the server)';openButton.hidden=true;
document.querySelector('#open-terminal')?.after(openButton);
const storageKey=()=>['taskdeck:semantic-browser:v1',app.taskData?.workspace||'',app.currentUser?.user_id||'local'].map(encodeURIComponent).join(':');
function persist(){
  const items=[...browserViews.values()].filter(v=>v.sessionID).map(v=>({id:v.id,session_id:v.sessionID}));
  try{sessionStorage.setItem(storageKey(),JSON.stringify(items));}catch{}
}
function element(tag,cls='',text=''){
  const e=document.createElement(tag);
  if(cls)e.className=cls;
  if(text)e.textContent=text;
  return e;
}
function button(label,fn,cls=''){
  const b=element('button',cls,label);b.type='button';b.onclick=()=>Promise.resolve().then(fn).catch(error=>showError(error));return b;
}
function showError(error,view){
  if(view){view.error.textContent=String(error?.message||error);view.error.hidden=false;}
  else app.showError?.(error);
}
function setLoading(view,loading){
  view.loading=Boolean(loading);
  view.goto.disabled=loading;
  view.address.disabled=loading;
  view.back.disabled=loading||!view.sessionID;
  view.forward.disabled=loading||!view.sessionID;
  view.reload.disabled=loading||!view.sessionID;
  view.status.textContent=loading?'Loading…':(view.page?.title||'Semantic browser');
}
function activate(view){
  if(app.activateExternalView('semantic-browser:'+view.id,{force:true})===false)return;
  for(const other of browserViews.values()){
    other.tab.classList.toggle('active',other===view);
    other.pane.classList.toggle('hidden',other!==view);
  }
  view.address.focus({preventScroll:true});
}
window.addEventListener('taskmenu:view-activated',event=>{
  if(event.detail?.kind==='external'&&String(event.detail?.id||'').startsWith('semantic-browser:'))return;
  for(const view of browserViews.values()){
    view.tab.classList.remove('active');
    view.pane.classList.add('hidden');
  }
});
function isActive(view){return view.tab.isConnected&&view.tab.classList.contains('active');}
async function call(view,action,fields={}){
  if(view.loading)return;
  setLoading(view,true);view.error.hidden=true;
  try{
    let payload={action,...fields};
    if(view.sessionID){
      payload.session_id=view.sessionID;
      if(view.page?.revision)payload.revision=view.page.revision;
    }else if(action==='navigate'){
      payload={action:'create',url:fields.url};
    }else throw new Error('Navigate to a website first');
    const page=await app.jsonFetch('/api/semantic-browser',{
      method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)
    });
    if(!view.tab.isConnected)return;
    if(page?.session_id)view.sessionID=String(page.session_id);
    view.page=page;
    view.address.value=page.url||view.address.value;
    view.title.textContent='🌐 '+String(page.title||new URL(page.url).hostname).slice(0,32);
    persist();
    render(view);
  }catch(error){showError(error,view);}
  finally{setLoading(view,false);}
}
function render(view){
  const container=view.content;
  const position=container.scrollTop;
  container.replaceChildren();
  const page=view.page;
  if(!page){container.append(element('p','semantic-browser-muted','Enter an HTTPS address. JavaScript runs in the remote Chromium worker; this tab renders text and safe controls only.'));return;}
  const heading=element('div','semantic-browser-heading',page.title||page.url||'Website');
  container.append(heading);
  container.append(element('p','semantic-browser-muted',page.url||''));
  const entries=Array.isArray(page.elements)?page.elements:[];
  for(const item of entries){
    const row=element('div','semantic-browser-entry');
    const name=String(item.text||'').slice(0,400);
    if(item.kind==='action'){
      const isLink=item.tag==='a'||item.role==='link';
      row.append(button(name||'Interactive element',()=>call(view,'click',{element_id:item.id}),isLink?'semantic-browser-link':'semantic-browser-action'));
    }else if(item.kind==='input'){
      const wrapper=element('div','semantic-browser-field');
      const label=element('label','',name||item.placeholder||item.input_type||'Input');
      const input=item.tag==='textarea'?element('textarea'):element('input');
      if(item.tag!=='textarea')input.type=['password','number','email','search','tel','url'].includes(item.input_type)?item.input_type:'text';
      input.value=String(item.value||'');input.placeholder=String(item.placeholder||'');
      input.addEventListener('change',()=>call(view,'fill',{element_id:item.id,value:input.value}));
      input.addEventListener('keydown',event=>{
        if(event.key==='Enter'&&item.tag!=='textarea'){
          event.preventDefault();
          call(view,'fill',{element_id:item.id,value:input.value})
            .then(()=>{const fresh=view.page?.elements?.find(el=>el.tag===item.tag&&el.text===item.text);
              if(fresh)return call(view,'press',{element_id:fresh.id,value:'Enter'});});
        }
      });
      wrapper.append(label,input);row.append(wrapper);
    }else if(item.kind==='check'){
      const label=element('label','semantic-browser-entry');
      const input=element('input');input.type=item.input_type==='radio'?'radio':'checkbox';input.checked=Boolean(item.checked);
      input.onchange=()=>call(view,'check',{element_id:item.id,value:String(input.checked)});
      label.append(input,document.createTextNode(' '+name));row.append(label);
    }else if(item.kind==='select'){
      const wrapper=element('label','semantic-browser-field');
      wrapper.append(element('span','',name||'Select'));
      const select=element('select');
      for(const option of item.options||[]){
        const el=element('option','',String(option.text||''));el.value=String(option.value||'');select.append(el);
      }
      select.value=String(item.value||'');
      select.onchange=()=>call(view,'select',{element_id:item.id,value:select.value});
      wrapper.append(select);row.append(wrapper);
    }else{
      row.textContent=name;
      if(/^h[1-6]$/.test(item.tag||''))row.classList.add('semantic-browser-heading');
    }
    container.append(row);
  }
  if(page.truncated)container.append(element('p','semantic-browser-muted','Element limit reached; narrow the page or use native browser for complex visual content.'));
  if(!entries.length)container.append(element('p','semantic-browser-muted','The page has no accessible textual elements yet. Try Refresh.'));
  container.scrollTop=position;
}
async function close(view){
  browserViews.delete(view.id);view.tab.remove();view.pane.remove();
  persist();
  if(view.sessionID){
    try{await app.jsonFetch('/api/semantic-browser',{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:view.sessionID})});}
    catch(error){console.warn('Unable to close semantic browser session',error);}
  }
  document.querySelector('#tabs > button:not(.hidden)')?.click();
}
function createView({id='',sessionID=''}={}){
  id=id||'semantic-browser:'+(globalThis.crypto?.randomUUID?.()||String(Date.now())+'-'+Math.random().toString(36).slice(2));
  const tab=element('button','tab semantic-browser-tab');tab.type='button';tab.dataset.id=id;
  const title=element('span','','🌐 Browser');
  const tabClose=element('span','close','×');
  tabClose.onclick=event=>{event.stopPropagation();close(view).catch(showError);};
  tab.append(title,tabClose);tab.onclick=()=>activate(view);tabHost.append(tab);
  const pane=element('div','pane semantic-browser-pane hidden');pane.dataset.id=id;
  const toolbar=element('div','semantic-browser-toolbar');
  const back=button('←',()=>call(view,'back'));
  const forward=button('→',()=>call(view,'forward'));
  const reload=button('↻',()=>call(view,'reload'));
  const address=element('input','semantic-browser-address');address.type='url';address.placeholder='https://example.com';
  const goto=button('Go',()=>call(view,'navigate',{url:address.value.trim()}));
  address.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();goto.click();}});
  toolbar.append(back,forward,reload,address,goto);
  const status=element('div','semantic-browser-muted','Enter a URL');
  status.style.padding='5px 12px';
  const error=element('div','semantic-browser-error');error.hidden=true;error.style.padding='6px 12px';
  const content=element('div','semantic-browser-content');
  pane.append(toolbar,status,error,content);paneHost.append(pane);
  const view={id,sessionID,page:null,tab,pane,title,back,forward,reload,address,goto,status,error,content,loading:false};
  browserViews.set(id,view);setLoading(view,false);render(view);activate(view);
  persist();
  return view;
}
async function restore(){
  let saved=[];
  try{const candidate=JSON.parse(sessionStorage.getItem(storageKey())||'[]');if(Array.isArray(candidate))saved=candidate.slice(0,8);}catch{}
  for(const item of saved){
    if(!item?.session_id||!item?.id)continue;
    const view=createView({id:String(item.id),sessionID:String(item.session_id)});
    await call(view,'snapshot');
    if(!view.page){browserViews.delete(view.id);view.tab.remove();view.pane.remove();}
  }
  persist();
}
async function init(){
  try{
    const info=await app.jsonFetch('/api/semantic-browser',{cache:'no-store'});
    browserEnabled=Boolean(info?.enabled);
  }catch{browserEnabled=false;}
  openButton.hidden=!browserEnabled;
  if(browserEnabled)await restore();
}
openButton.onclick=()=>createView();
const ready=new Promise(resolve=>{
  if(app.taskData?.workspace)init().finally(resolve);
  else window.addEventListener('taskmenu:tasks',()=>init().finally(resolve),{once:true});
});
globalThis.TaskMenuSemanticBrowser={views:browserViews,open:()=>browserEnabled?createView():null,ready};
