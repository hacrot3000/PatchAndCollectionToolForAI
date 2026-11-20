const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for hex viewer');

const tabsHost=document.querySelector('#tabs');
const panesHost=document.querySelector('#panes');
if(!tabsHost||!panesHost)throw new Error('Hex viewer tab hosts unavailable');

const style=document.createElement('style');
style.textContent=`
.hex-pane{background:#0b1016}
.hex-head{min-height:34px;padding:4px 7px}
.hex-path{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:11px ui-monospace,monospace;opacity:.78}
.hex-meta{font:10px ui-monospace,monospace;opacity:.58;white-space:nowrap}
.hex-offset{width:118px;height:22px;padding:2px 5px;font:10px ui-monospace,monospace}
.hex-page-size{height:22px;padding:1px 5px;font-size:10px}
.hex-body{flex:1;min-height:0;overflow:auto;padding:8px;background:#070b10}
.hex-table{display:table;min-width:760px;font:11px/1.55 ui-monospace,SFMono-Regular,Consolas,monospace}
.hex-row{display:table-row}
.hex-address,.hex-values,.hex-ascii{display:table-cell;white-space:pre;padding:0 8px}
.hex-address{color:#79a6d2;text-align:right;user-select:none;border-right:1px solid #27303b}
.hex-values{letter-spacing:.04em}
.hex-ascii{color:#b8c4cf;border-left:1px solid #27303b}
.hex-empty{padding:26px;text-align:center;opacity:.6}
.hex-tab .close{margin-left:7px}
html[data-taskmenu-theme="light"] .hex-pane{background:#fff}
html[data-taskmenu-theme="light"] .hex-body{background:#f8fafc}
html[data-taskmenu-theme="light"] .hex-address{color:#255d91;border-color:#d0d7de}
html[data-taskmenu-theme="light"] .hex-ascii{border-color:#d0d7de;color:#36404a}
`;
document.head.append(style);

const views=new Map();
let activeID='';
const DEFAULT_LIMIT=4096;

function cleanPath(value){return String(value||'').trim().replace(/^\.\//,'').replace(/\\/g,'/');}
function viewID(pathValue){return 'hex:'+cleanPath(pathValue);}
function basename(value){value=cleanPath(value);const i=value.lastIndexOf('/');return i>=0?value.slice(i+1):value;}
function formatBytes(value){
  const n=Math.max(0,Number(value)||0);
  if(n<1024)return n+' B';
  if(n<1024*1024)return (n/1024).toFixed(1)+' KiB';
  if(n<1024*1024*1024)return (n/(1024*1024)).toFixed(1)+' MiB';
  return (n/(1024*1024*1024)).toFixed(2)+' GiB';
}
function decodeBase64(value){
  const text=atob(String(value||'')),out=new Uint8Array(text.length);
  for(let i=0;i<text.length;i++)out[i]=text.charCodeAt(i);
  return out;
}
function addressWidth(size){
  const max=Math.max(0,Number(size||0)-1);
  return Math.max(8,max.toString(16).length);
}
function render(view,data){
  view.body.replaceChildren();
  view.data=data;
  view.offset.value='0x'+Number(data.offset||0).toString(16).toUpperCase();
  view.meta.textContent=formatBytes(data.size)+' · '+formatBytes(data.length)+' page';
  view.prev.disabled=Number(data.offset||0)<=0;
  view.next.disabled=Boolean(data.eof);
  const bytes=decodeBase64(data.base64);
  if(!bytes.length){
    const empty=document.createElement('div');empty.className='hex-empty';empty.textContent=data.size?'No bytes at this offset':'Empty file';view.body.append(empty);return;
  }
  const table=document.createElement('div');table.className='hex-table';
  const width=addressWidth(data.size);
  for(let row=0;row<bytes.length;row+=16){
    const rowNode=document.createElement('div');rowNode.className='hex-row';
    const address=document.createElement('span');address.className='hex-address';address.textContent=(Number(data.offset)+row).toString(16).toUpperCase().padStart(width,'0');
    const chunk=bytes.slice(row,Math.min(row+16,bytes.length));
    const hex=[];const ascii=[];
    for(let i=0;i<16;i++){
      if(i<chunk.length){
        const value=chunk[i];hex.push(value.toString(16).toUpperCase().padStart(2,'0'));ascii.push(value>=32&&value<=126?String.fromCharCode(value):'.');
      }else{hex.push('  ');ascii.push(' ');}
    }
    const values=document.createElement('span');values.className='hex-values';values.textContent=hex.slice(0,8).join(' ')+'  '+hex.slice(8).join(' ');
    const asciiNode=document.createElement('span');asciiNode.className='hex-ascii';asciiNode.textContent=ascii.join('');
    rowNode.append(address,values,asciiNode);table.append(rowNode);
  }
  view.body.append(table);
}
async function load(view,offset=view.currentOffset||0){
  offset=Math.max(0,Math.floor(Number(offset)||0));
  const limit=Math.max(256,Math.min(16384,Number(view.pageSize.value)||DEFAULT_LIMIT));
  view.currentOffset=offset;
  view.prev.disabled=true;view.next.disabled=true;view.refresh.disabled=true;
  try{
    const data=await app.jsonFetch('/api/project/bytes?path='+encodeURIComponent(view.path)+'&offset='+offset+'&limit='+limit,{cache:'no-store'});
    view.currentOffset=Number(data.offset)||0;render(view,data);
  }finally{view.refresh.disabled=false;}
}
function activate(id,{force=false}={}){
  const view=views.get(id);if(!view)return false;
  if(app.activateExternalView(id,{force})===false)return false;
  activeID=id;
  for(const [otherID,other] of views){
    const active=otherID===id;other.tab.classList.toggle('active',active);other.pane.classList.toggle('hidden',!active);
  }
  return true;
}
function close(id){
  const view=views.get(id);if(!view)return;
  view.tab.remove();view.pane.remove();views.delete(id);
  if(activeID===id){
    activeID='';
    const next=views.values().next();
    if(!next.done)activate(next.value.id,{force:true});
  }
}
function create(pathValue){
  pathValue=cleanPath(pathValue);const id=viewID(pathValue);
  const existing=views.get(id);if(existing)return existing;
  const tab=document.createElement('button');tab.type='button';tab.className='tab hex-tab';tab.dataset.id=id;tab.dataset.viewKind='hex';tab.title=pathValue;
  const label=document.createElement('span');label.textContent='HEX · '+basename(pathValue);
  const closeButton=document.createElement('span');closeButton.className='close';closeButton.textContent='×';closeButton.title='Close hex viewer';tab.append(label,closeButton);tabsHost.append(tab);

  const pane=document.createElement('div');pane.className='pane hex-pane hidden';pane.dataset.id=id;pane.dataset.viewKind='hex';
  const head=document.createElement('div');head.className='pane-head hex-head';
  const path=document.createElement('div');path.className='hex-path';path.textContent=pathValue;path.title=pathValue;
  const meta=document.createElement('div');meta.className='hex-meta';
  const prev=document.createElement('button');prev.type='button';prev.textContent='←';prev.title='Previous chunk';
  const next=document.createElement('button');next.type='button';next.textContent='→';next.title='Next chunk';
  const offset=document.createElement('input');offset.className='hex-offset';offset.type='text';offset.value='0x0';offset.title='Jump to byte offset (decimal or 0x hex)';
  const pageSize=document.createElement('select');pageSize.className='hex-page-size';pageSize.title='Bytes per page';
  for(const value of [256,1024,4096,16384]){const option=document.createElement('option');option.value=String(value);option.textContent=formatBytes(value);if(value===DEFAULT_LIMIT)option.selected=true;pageSize.append(option);}
  const refresh=document.createElement('button');refresh.type='button';refresh.textContent='↻';refresh.title='Reload bytes';
  head.append(path,meta,prev,next,offset,pageSize,refresh);
  const body=document.createElement('div');body.className='hex-body';pane.append(head,body);panesHost.append(pane);

  const view={id,path:pathValue,tab,pane,meta,prev,next,offset,pageSize,refresh,body,currentOffset:0,data:null};
  views.set(id,view);
  tab.onclick=()=>activate(id,{force:true});
  closeButton.onclick=event=>{event.stopPropagation();close(id);};
  prev.onclick=()=>load(view,Math.max(0,view.currentOffset-(Number(pageSize.value)||DEFAULT_LIMIT))).catch(app.showError);
  next.onclick=()=>load(view,view.data?.next_offset??(view.currentOffset+(Number(pageSize.value)||DEFAULT_LIMIT))).catch(app.showError);
  refresh.onclick=()=>load(view,view.currentOffset).catch(app.showError);
  pageSize.onchange=()=>load(view,Math.floor(view.currentOffset/(Number(pageSize.value)||DEFAULT_LIMIT))*(Number(pageSize.value)||DEFAULT_LIMIT)).catch(app.showError);
  offset.onkeydown=event=>{
    if(event.key!=='Enter')return;
    event.preventDefault();
    const raw=offset.value.trim();
    const value=/^0x[0-9a-f]+$/i.test(raw)?Number.parseInt(raw.slice(2),16):Number.parseInt(raw,10);
    if(!Number.isFinite(value)||value<0){app.showError(new Error('Offset must be a non-negative decimal or 0x hexadecimal value'));return;}
    load(view,value).catch(app.showError);
  };
  return view;
}
async function open(pathValue){
  pathValue=cleanPath(pathValue);if(!pathValue)return null;
  const view=create(pathValue);activate(view.id,{force:true});await load(view,view.currentOffset);return view;
}

window.addEventListener('taskmenu:project-hex-open-request',event=>{const pathValue=event.detail?.path;if(pathValue)open(pathValue).catch(app.showError);});
window.addEventListener('taskmenu:view-activated',event=>{
  if(event.detail?.kind==='terminal'){
    activeID='';for(const view of views.values()){view.tab.classList.remove('active');view.pane.classList.add('hidden');}
    return;
  }
  if(event.detail?.kind==='external'){
    const id=String(event.detail.id||'');
    if(views.has(id)){activeID=id;for(const [otherID,view] of views){const active=otherID===id;view.tab.classList.toggle('active',active);view.pane.classList.toggle('hidden',!active);}}
    else{activeID='';for(const view of views.values()){view.tab.classList.remove('active');view.pane.classList.add('hidden');}}
  }
});

function snapshotHexState(){
  return [...views.values()].map(view=>({path:String(view.path||''),offset:Math.max(0,Number(view.currentOffset)||0)})).filter(item=>item.path);
}
async function restoreHexState(items){
  for(const item of Array.isArray(items)?items:[]){
    const path=String(item?.path||'').trim();if(!path)continue;
    try{
      const view=await open(path);
      const offset=Number(item.offset)||0;
      if(view&&offset>0)await load(view,offset);
    }catch(error){console.warn('Cannot restore HEX tab '+path,error);}
  }
}
globalThis.TaskMenuHexViewer={open,close,views,snapshotState:snapshotHexState,restoreState:restoreHexState};
