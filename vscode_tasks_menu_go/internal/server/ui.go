package server

import (
	"net/http"
	"strings"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func staticUI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/", "/index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
		_, _ = w.Write([]byte(indexHTML))
	case "/app.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
		_, _ = w.Write([]byte(appCSS))
	case "/app.js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
		_, _ = w.Write([]byte(appJS))
	case "/features.js":
		serveEmbeddedAsset(w, "application/javascript; charset=utf-8", "features.js")
	case "/vendor/xterm.js":
		serveEmbeddedAsset(w, "application/javascript; charset=utf-8", "vendor/xterm.js")
	case "/vendor/codemirror6-all.min.js":
		serveEmbeddedAsset(w, "application/javascript; charset=utf-8", "vendor/codemirror6-all.min.js")
	case "/vendor/addon-fit.js":
		serveEmbeddedAsset(w, "application/javascript; charset=utf-8", "vendor/addon-fit.js")
	case "/vendor/xterm.css":
		serveEmbeddedAsset(w, "text/css; charset=utf-8", "vendor/xterm.css")
	default:
		if strings.HasPrefix(r.URL.Path, "/featuremods/") {
			serveEmbeddedAsset(w, "application/javascript; charset=utf-8", strings.TrimPrefix(r.URL.Path, "/"))
			return
		}
		http.NotFound(w, r)
	}
}

func serveEmbeddedAsset(w http.ResponseWriter, contentType, path string) {
	data, err := webassets.Files.ReadFile(path)
	if err != nil {
		http.Error(w, "embedded asset unavailable", http.StatusInternalServerError)
		return
	}
	if path == "featuremods/next.js" {
		text := string(data)
		text = strings.ReplaceAll(text, ".js';", ".js?v=2';")
		data = []byte(text)
	}
	w.Header().Set("Content-Type", contentType)
	if path == "features.js" || strings.HasPrefix(path, "featuremods/") {
		w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	_, _ = w.Write(data)
}

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>VS Code Tasks Menu</title>
  <link rel="stylesheet" href="/vendor/xterm.css">
  <link rel="stylesheet" href="/app.css">
</head>
<body>
  <header><strong>TaskDeck</strong><span id="workspace"></span><button id="open-terminal" title="Open an interactive shell at the project root">Open Terminal</button><button id="edit-title" title="Edit the browser page title">Edit title</button><button id="reload">Reload tasks.json</button></header>
  <main><aside id="menu"></aside><section><div id="tabs"></div><div id="panes"></div></section></main>
  <script src="/vendor/xterm.js"></script>
  <script src="/vendor/addon-fit.js"></script>
  <script src="/vendor/codemirror6-all.min.js?v=8b031d22"></script>
  <script type="module" src="/app.js"></script>
  <script type="module" src="/features.js?v=2"></script>
  <script type="module" src="/featuremods/all.js?v=2"></script>
  <script type="module" src="/featuremods/next.js?v=2"></script>
</body>
</html>`

const appCSS = `:root{font-family:system-ui,sans-serif;color-scheme:dark;background:#101216;color:#e8eaed}*{box-sizing:border-box}body{margin:0}header{height:52px;display:flex;align-items:center;gap:16px;padding:0 16px;border-bottom:1px solid #30343b}header #workspace{opacity:.65;flex:1;font-size:12px}button,select{background:#252a33;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:7px 10px}button{cursor:pointer}button:disabled{opacity:.45;cursor:default}#open-terminal{background:#223b55;border-color:#365f84;white-space:nowrap}#edit-title{white-space:nowrap}main{display:grid;grid-template-columns:310px 1fr;height:calc(100vh - 52px)}aside{overflow:auto;border-right:1px solid #30343b;padding:10px}.group{margin:5px 0}.group>button,.task{width:100%;text-align:left}.children{padding-left:14px}.task{margin:2px 0;background:#171a20}.task:hover{background:#242a34}section{min-width:0;display:flex;flex-direction:column}#tabs{height:42px;border-bottom:1px solid #30343b;display:flex;align-items:end;overflow:auto;white-space:nowrap}.tab{border-radius:6px 6px 0 0;border-bottom:0;margin-left:4px}.tab.active{background:#343b48}.tab .status{opacity:.65;margin-left:6px}.tab .close{margin-left:8px}#panes{flex:1;min-height:0;position:relative}.pane{position:absolute;inset:0;display:flex;flex-direction:column}.pane.hidden{display:none}.pane-head{display:flex;gap:8px;align-items:center;padding:7px 10px;border-bottom:1px solid #30343b}.pane-head .command{font-family:ui-monospace,monospace;font-size:12px;opacity:.7;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1}.download-select{max-width:280px;min-width:140px}.download{white-space:nowrap;background:#24472f;border-color:#3b7850}.copy-console{margin-left:24px;white-space:nowrap}.terminal{flex:1;min-height:0;padding:6px;background:#050607}.browser-lease-lost>header,.browser-lease-lost>main{filter:blur(1px);opacity:.45;pointer-events:none}.browser-lease-overlay{position:fixed;inset:0;z-index:10000;display:grid;place-items:center;padding:20px;background:rgba(4,6,9,.72)}.browser-lease-card{width:min(520px,100%);padding:22px;border:1px solid #48515f;border-radius:12px;background:#171a20;box-shadow:0 18px 55px rgba(0,0,0,.45);text-align:center}.browser-lease-card strong{display:block;font-size:18px;margin-bottom:10px}.browser-lease-card p{margin:8px 0;line-height:1.45}.browser-lease-card .browser-lease-hint{font-size:12px;opacity:.7}.browser-lease-card button{margin-top:10px;background:#244c70;border-color:#3f79a8;padding:10px 14px}`

const appJS = `const TerminalCtor=globalThis.Terminal;
const FitAddonCtor=globalThis.FitAddon?.FitAddon;
if(!TerminalCtor||!FitAddonCtor)throw new Error('Embedded xterm assets did not load');

const defaultPageTitle='VS Code Tasks Menu';
const layoutProfile=(matchMedia('(pointer: coarse)').matches||matchMedia('(max-width: 900px)').matches)?'mobile':'desktop';
document.documentElement.dataset.taskmenuLayout=layoutProfile;
const menu=document.querySelector('#menu');
const tabs=document.querySelector('#tabs');
const panes=document.querySelector('#panes');
let taskData=null;
let active=null;
const views=new Map();
const hidden=new Set();
const outputFilters=[];
let browserLease='';
let browserLeaseLost=false;
let browserLeaseHeartbeat=null;

function addOutputFilter(filter){
  if(typeof filter!=='function')throw new Error('Output filter must be a function');
  outputFilters.push(filter);
  return ()=>{
    const index=outputFilters.indexOf(filter);
    if(index>=0)outputFilters.splice(index,1);
  };
}

function filterOutput(view,data){
  let current=data;
  for(const filter of [...outputFilters]){
    if(current==null)break;
    try{current=filter(view,current);}catch(e){console.warn('Output filter failed',e);}
  }
  return current;
}

function writeOutput(view,data){
  if(data==null)return;
  if(typeof data==='string'){
    if(data)view.term.write(data);
    return;
  }
  if(data instanceof ArrayBuffer){
    if(data.byteLength)view.term.write(new Uint8Array(data));
    return;
  }
  if(ArrayBuffer.isView(data)){
    if(data.byteLength)view.term.write(new Uint8Array(data.buffer,data.byteOffset,data.byteLength));
  }
}

function showLeaseLost(message='This browser is no longer the active controller.'){
  if(browserLeaseLost)return;
  browserLeaseLost=true;
  clearInterval(browserLeaseHeartbeat);
  document.body.classList.add('browser-lease-lost');
  for(const view of views.values()){
    clearTimeout(view.reconnectTimer);
    try{view.ws?.close();}catch{}
  }
  let overlay=document.querySelector('#browser-lease-overlay');
  if(!overlay){
    overlay=document.createElement('div');overlay.id='browser-lease-overlay';overlay.className='browser-lease-overlay';
    const card=document.createElement('div');card.className='browser-lease-card';
    const title=document.createElement('strong');title.textContent='Control moved to another browser';
    const text=document.createElement('p');text.textContent=message;
    const hint=document.createElement('p');hint.className='browser-lease-hint';hint.textContent='Reload this page to take control. The other browser will then be disconnected.';
    const reload=document.createElement('button');reload.type='button';reload.textContent='Reload and take control';reload.onclick=()=>location.reload();
    card.append(title,text,hint,reload);overlay.append(card);document.body.append(overlay);
  }
}

async function acquireBrowserLease(){
  const r=await fetch('/api/browser/lease',{method:'POST',cache:'no-store'});
  if(!r.ok)throw new Error((await r.text())||r.statusText);
  const data=await r.json();
  browserLease=String(data?.lease||'').trim();
  if(!browserLease)throw new Error('Server did not return a browser control lease');
  browserLeaseLost=false;
  document.body.classList.remove('browser-lease-lost');
}

async function checkBrowserLease(){
  if(!browserLease||browserLeaseLost)return;
  const r=await fetch('/api/browser/lease',{cache:'no-store',headers:{'X-TaskMenu-Lease':browserLease}});
  if(r.ok)return;
  if(r.status===409||r.headers.get('X-TaskMenu-Lease-Revoked')==='1'){
    showLeaseLost((await r.text()).trim()||undefined);
  }
}

async function fetchWithLease(url,opts={}){
  const headers=new Headers(opts.headers||{});
  if(browserLease)headers.set('X-TaskMenu-Lease',browserLease);
  const response=await fetch(url,{...opts,headers});
  if(response.status===409&&response.headers.get('X-TaskMenu-Lease-Revoked')==='1'){
    showLeaseLost();
  }
  return response;
}

async function jsonFetch(url,opts={}){
  const r=await fetchWithLease(url,{cache:'no-store',...opts});
  if(!r.ok){
    const message=(await r.text())||r.statusText;
    if(r.status===409&&r.headers.get('X-TaskMenu-Lease-Revoked')==='1'){
      showLeaseLost(message.trim()||undefined);
      const error=new Error('Browser control lease revoked');
      error.browserLeaseRevoked=true;
      throw error;
    }
    throw new Error(message);
  }
  if(r.status===204)return null;
  return r.json();
}

function pageTitleStorageKey(){return 'vscode-tasks-menu:page-title:'+taskData.workspace;}

function restorePageTitle(){
  let saved='';
  try{saved=localStorage.getItem(pageTitleStorageKey())||'';}catch(e){console.warn('Cannot read saved page title',e);}
  document.title=saved||defaultPageTitle;
}

function editPageTitle(){
  const current=document.title===defaultPageTitle?'':document.title;
  const value=window.prompt('Page title (leave blank to use the default):',current);
  if(value===null)return;
  const title=value.trim();
  try{
    if(title)localStorage.setItem(pageTitleStorageKey(),title);
    else localStorage.removeItem(pageTitleStorageKey());
  }catch(e){throw new Error('Cannot save page title: '+e.message);}
  document.title=title||defaultPageTitle;
}

async function loadTasks(){
  taskData=await jsonFetch('/api/tasks');
  document.querySelector('#workspace').textContent=taskData.workspace;
  restorePageTitle();
  renderMenu();
  window.dispatchEvent(new CustomEvent('taskmenu:tasks',{detail:{taskData}}));
}

function buildTree(){
  const root={children:new Map(),tasks:[]};
  for(const t of taskData.tasks){
    const group=t.group||[];
    if(group.length===1&&group[0]==='build'){root.tasks.push(t);continue;}
    let node=root;
    for(const name of group){
      if(!node.children.has(name))node.children.set(name,{children:new Map(),tasks:[]});
      node=node.children.get(name);
    }
    node.tasks.push(t);
  }
  return root;
}

function renderMenu(){menu.innerHTML='';renderNode(buildTree(),menu,false);}
function renderNode(node,host,open){
  for(const t of node.tasks){
    const b=document.createElement('button');
    b.className='task';b.textContent=t.menu_label;b.title=t.detail||t.label;
    b.onclick=()=>startTask(t).catch(showError);host.append(b);
  }
  for(const [name,child] of node.children){
    const box=document.createElement('div'),b=document.createElement('button'),kids=document.createElement('div');
    box.className='group';kids.className='children';kids.hidden=!open;
    b.textContent=(kids.hidden?'▸ ':'▾ ')+name;
    b.onclick=()=>{kids.hidden=!kids.hidden;b.textContent=(kids.hidden?'▸ ':'▾ ')+name;};
    box.append(b,kids);renderNode(child,kids,false);host.append(box);
  }
}

async function startTask(t){
  const meta=await jsonFetch('/api/sessions',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({task_id:t.id})});
  hidden.delete(meta.id);attach(meta,true);
}

async function startTerminal(){
  const meta=await jsonFetch('/api/sessions',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({kind:'terminal'})});
  hidden.delete(meta.id);attach(meta,true);
}

function wsURL(id){
  const base=(location.protocol==='https:'?'wss:':'ws:')+'//'+location.host+'/api/sessions/'+encodeURIComponent(id)+'/ws';
  return browserLease?base+'?lease='+encodeURIComponent(browserLease):base;
}

function attach(meta,activate){
  let view=views.get(meta.id);
  if(view){updateMeta(meta);if(activate)activateView(meta.id);return view;}
  const tab=document.createElement('button');tab.className='tab';tab.dataset.id=meta.id;
  const label=document.createElement('span');label.textContent=meta.label;
  const status=document.createElement('span');status.className='status';
  const close=document.createElement('span');close.className='close';close.textContent='×';
  close.onclick=e=>{e.stopPropagation();closeView(meta.id).catch(showError);};
  tab.append(label,status,close);tab.onclick=()=>activateView(meta.id);tabs.append(tab);

  const pane=document.createElement('div');pane.className='pane hidden';pane.dataset.id=meta.id;
  const head=document.createElement('div');head.className='pane-head';
  const cmd=document.createElement('div');cmd.className='command';cmd.textContent=meta.command_preview;
  const downloadSelect=document.createElement('select');downloadSelect.className='download-select';downloadSelect.hidden=true;
  const download=document.createElement('button');download.className='download';download.textContent='Download';download.hidden=true;
  const stop=document.createElement('button');stop.className='stop';stop.textContent='Stop';
  stop.onclick=async()=>{try{updateMeta(await jsonFetch('/api/sessions/'+meta.id+'/stop',{method:'POST'}));}catch(e){showError(e);}};
  const copy=document.createElement('button');copy.className='copy-console';copy.textContent='📋 Copy console';copy.title='Copy the entire console output';
  copy.onclick=()=>copyConsole(view).catch(showError);
  head.append(cmd,downloadSelect,download,stop,copy);
  const terminalHost=document.createElement('div');terminalHost.className='terminal';
  pane.append(head,terminalHost);panes.append(pane);

  const term=new TerminalCtor({convertEol:false,cursorBlink:true,scrollback:10000,fontSize:13,theme:{background:'#050607'}});
  const fit=new FitAddonCtor();term.loadAddon(fit);term.open(terminalHost);fit.fit();
  view={meta,tab,status,pane,term,fit,ws:null,ro:null,closed:false,reconnectTimer:null,selectionTimer:null,selectionSeq:0,downloadFiles:[],downloadSelect,download,stop,copy,copyTimer:null};
  views.set(meta.id,view);
  downloadSelect.onchange=()=>updateDownloadButton(view);
  download.onclick=()=>downloadSelectedFile(view);
  term.onSelectionChange(()=>scheduleSelectionScan(view));
  term.onData(data=>{if(!browserLeaseLost&&view.ws&&view.ws.readyState===WebSocket.OPEN)view.ws.send(data);});
  const resize=()=>{
    try{fit.fit();}catch{}
    clearTimeout(view.resizeTimer);
    view.resizeTimer=setTimeout(()=>jsonFetch('/api/sessions/'+meta.id+'/resize',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({rows:term.rows,cols:term.cols})}).catch(()=>{}),60);
  };
  view.ro=new ResizeObserver(resize);view.ro.observe(terminalHost);
  connect(view,false);updateMeta(meta);if(activate)activateView(meta.id);return view;
}

function setDownloadFiles(view,files){
  view.downloadFiles=Array.isArray(files)?files:[];
  view.downloadSelect.replaceChildren();
  for(let i=0;i<view.downloadFiles.length;i++){
    const file=view.downloadFiles[i];
    const option=document.createElement('option');option.value=String(i);option.textContent=file.name||file.path;option.title=file.path||'';
    view.downloadSelect.append(option);
  }
  view.download.hidden=view.downloadFiles.length===0;
  view.downloadSelect.hidden=view.downloadFiles.length<=1;
  updateDownloadButton(view);
}

function updateDownloadButton(view){
  if(!view.downloadFiles.length){view.download.textContent='Download';view.download.title='';return;}
  const idx=view.downloadSelect.hidden?0:Number(view.downloadSelect.value||0);
  const file=view.downloadFiles[idx]||view.downloadFiles[0];
  view.download.textContent=view.downloadFiles.length>1?'Download ('+view.downloadFiles.length+')':'Download';
  view.download.title=file?.path?'Download '+file.path:'Download the selected file';
}

function downloadSelectedFile(view){
  if(!view.downloadFiles.length)return;
  const idx=view.downloadSelect.hidden?0:Number(view.downloadSelect.value||0);
  const file=view.downloadFiles[idx]||view.downloadFiles[0];
  if(!file?.url)return;
  const a=document.createElement('a');a.href=file.url;a.download=file.name||'';a.rel='noopener';document.body.append(a);a.click();a.remove();
}

function consoleText(view){
  const buffer=view.term.buffer.active;
  const lines=[];
  let current='';
  for(let i=0;i<buffer.length;i++){
    const line=buffer.getLine(i);
    if(!line)continue;
    const text=line.translateToString(true);
    if(line.isWrapped){current+=text;continue;}
    if(i>0)lines.push(current);
    current=text;
  }
  if(buffer.length)lines.push(current);
  while(lines.length&&lines[lines.length-1]==='')lines.pop();
  return lines.join('\n');
}

async function copyConsole(view){
  const text=consoleText(view);
  if(!text){showCopyFeedback(view,'Console is empty');return;}
  let copied=false;
  if(navigator.clipboard&&window.isSecureContext){
    try{await navigator.clipboard.writeText(text);copied=true;}catch{}
  }
  if(!copied){
    const area=document.createElement('textarea');
    area.value=text;area.setAttribute('readonly','');area.style.position='fixed';area.style.left='-9999px';area.style.top='0';
    document.body.append(area);area.select();
    try{copied=document.execCommand('copy');}finally{area.remove();}
  }
  if(!copied)throw new Error('Cannot copy console to clipboard');
  showCopyFeedback(view,'✓ Copied');
}

function showCopyFeedback(view,label){
  clearTimeout(view.copyTimer);view.copy.textContent=label;
  view.copyTimer=setTimeout(()=>{if(!view.closed)view.copy.textContent='📋 Copy console';},1200);
}

function scheduleSelectionScan(view){
  clearTimeout(view.selectionTimer);
  view.selectionSeq++;
  const text=view.term.getSelection();
  if(!text.trim()){setDownloadFiles(view,[]);return;}
  const seq=view.selectionSeq;
  view.selectionTimer=setTimeout(()=>scanSelection(view,text,seq),120);
}

async function scanSelection(view,text,seq){
  try{
    const data=await jsonFetch('/api/files/selection',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({text:text.slice(0,65536)})});
    if(view.closed||seq!==view.selectionSeq)return;
    setDownloadFiles(view,data.files||[]);
  }catch(e){
    if(seq===view.selectionSeq)setDownloadFiles(view,[]);
    console.warn('Selected-file detection failed',e);
  }
}

function connect(view,replay){
  if(view.closed||browserLeaseLost)return;
  if(replay)view.term.reset();
  const ws=new WebSocket(wsURL(view.meta.id));view.ws=ws;ws.binaryType='arraybuffer';
  ws.onopen=()=>{try{view.fit.fit();}catch{}};
  ws.onmessage=e=>{
    const data=e.data;
    writeOutput(view,filterOutput(view,data));
    window.dispatchEvent(new CustomEvent('taskmenu:output',{detail:{view,data}}));
  };
  ws.onclose=async()=>{
    if(view.closed||browserLeaseLost)return;
    try{
      const meta=await jsonFetch('/api/sessions/'+view.meta.id);updateMeta(meta);
      if(meta.status==='running')view.reconnectTimer=setTimeout(()=>connect(view,true),600);
    }catch{}
  };
}

function activateView(id){
  active=id;
  for(const [sid,v] of views){
    const yes=sid===id;v.tab.classList.toggle('active',yes);v.pane.classList.toggle('hidden',!yes);
    if(yes)setTimeout(()=>{try{v.fit.fit();v.term.focus();}catch{}},0);
  }
  window.dispatchEvent(new CustomEvent('taskmenu:view-activated',{detail:{kind:'terminal',id}}));
}

function activateExternalView(token){
  active='external:'+String(token||'view');
  for(const [,v] of views){v.tab.classList.remove('active');v.pane.classList.add('hidden');}
  window.dispatchEvent(new CustomEvent('taskmenu:view-activated',{detail:{kind:'external',id:String(token||'view')}}));
}

async function closeView(id){
  hidden.add(id);
  const view=views.get(id);if(!view)return;
  if(view.meta.status!=='running')await jsonFetch('/api/sessions/'+id,{method:'DELETE'});
  teardownView(id);
}

function teardownView(id){
  const view=views.get(id);if(!view)return;
  view.closed=true;clearTimeout(view.reconnectTimer);clearTimeout(view.resizeTimer);clearTimeout(view.selectionTimer);clearTimeout(view.copyTimer);
  if(view.ro)view.ro.disconnect();try{if(view.ws)view.ws.close();}catch{}view.term.dispose();view.tab.remove();view.pane.remove();views.delete(id);
  if(active===id){active=null;const next=views.keys().next();if(!next.done)activateView(next.value);}
}

function updateMeta(meta){
  const view=views.get(meta.id);if(!view)return;
  view.meta=meta;view.status.textContent=meta.status+(meta.exit_code!=null?' '+meta.exit_code:'');
  view.stop.textContent=meta.task_id===0?'Close terminal':'Stop';
  view.stop.title=meta.task_id===0?'Close the terminal and its running processes':'Stop task';
  view.stop.disabled=meta.status!=='running';
  window.dispatchEvent(new CustomEvent('taskmenu:session',{detail:{view,meta}}));
}

async function syncSessions(){
  const data=await jsonFetch('/api/sessions');const seen=new Set();
  for(const meta of data.sessions){
    seen.add(meta.id);if(hidden.has(meta.id))continue;
    if(views.has(meta.id))updateMeta(meta);else attach(meta,false);
  }
  for(const id of [...views.keys()])if(!seen.has(id))teardownView(id);
  if(!active&&views.size)activateView(views.keys().next().value);
}

function showError(e){console.error(e);if(e?.browserLeaseRevoked)return;alert('ERROR: '+e.message);}
globalThis.TaskMenuApp={
  get taskData(){return taskData;},
  get active(){return active;},
  get browserLease(){return browserLease;},
  get browserLeaseLost(){return browserLeaseLost;},
  get layoutProfile(){return layoutProfile;},
  views,jsonFetch,fetchWithLease,showError,consoleText,startTask,startTerminal,activateView,activateExternalView,loadTasks,syncSessions,attachSession:attach,addOutputFilter
};
document.querySelector('#open-terminal').onclick=()=>startTerminal().catch(showError);
document.querySelector('#edit-title').onclick=()=>{try{editPageTitle();}catch(e){showError(e);}};
document.querySelector('#reload').onclick=()=>loadTasks().catch(showError);
await acquireBrowserLease();await loadTasks();await syncSessions();
setInterval(()=>{if(!browserLeaseLost)syncSessions().catch(()=>{});},1000);
browserLeaseHeartbeat=setInterval(()=>checkBrowserLease().catch(e=>console.warn('Browser lease heartbeat failed',e)),1000);`