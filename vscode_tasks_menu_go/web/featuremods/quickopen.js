const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Quick Open');

const style=document.createElement('style');
style.textContent=`
.quick-open-backdrop{display:none;position:fixed;inset:0;z-index:2500;background:rgba(0,0,0,.48);align-items:flex-start;justify-content:center;padding-top:min(12vh,110px)}
.quick-open-backdrop.visible{display:flex}
.quick-open-dialog{width:min(760px,92vw);max-height:72vh;display:flex;flex-direction:column;border:1px solid #4a5362;border-radius:9px;background:#15191f;box-shadow:0 18px 55px rgba(0,0,0,.5);overflow:hidden}
.quick-open-input{width:100%;border:0;border-bottom:1px solid #343b46;border-radius:0;background:#0d1117;color:inherit;padding:12px 14px;font:14px ui-monospace,monospace;outline:none}
.quick-open-results{overflow:auto;min-height:42px;max-height:56vh;padding:4px}
.quick-open-result{display:block;width:100%;text-align:left;border:1px solid transparent;background:transparent;padding:7px 9px}
.quick-open-result.selected,.quick-open-result:hover{background:#293241;border-color:#42536a}
.quick-open-result-name{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.quick-open-match{font-weight:800;background:rgba(111,168,255,.2);border-radius:2px}
.quick-open-result-path{display:block;margin-top:2px;font-size:11px;opacity:.58;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.quick-open-empty{padding:12px 10px;font-size:12px;opacity:.62}
html[data-taskmenu-theme="light"] .quick-open-dialog{background:#fff;border-color:#b9c0c8;box-shadow:0 18px 55px rgba(0,0,0,.18)}
html[data-taskmenu-theme="light"] .quick-open-input{background:#f7f8fa;border-color:#d5d9df}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='quick-open-backdrop';
const dialog=document.createElement('div');dialog.className='quick-open-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Quick Open');
const input=document.createElement('input');input.className='quick-open-input';input.type='search';input.autocomplete='off';input.spellcheck=false;input.placeholder='Global Quick Open — files, recent, tabs, terminals, DB, connections · > @ # git: ssh:';
const results=document.createElement('div');results.className='quick-open-results';
dialog.append(input,results);backdrop.append(dialog);document.body.append(backdrop);

let items=[];
let selected=0;
let timer=null;
let refreshTimer=null;
let controller=null;
let seq=0;

function setEmpty(message){
  results.replaceChildren();
  const empty=document.createElement('div');empty.className='quick-open-empty';empty.textContent=message;results.append(empty);
}

function itemText(item){return [item.name,item.path,item.detail,item.kind].filter(Boolean).join(' ');}
function localMatches(item,query){
  const tokens=String(query||'').toLowerCase().split(/\s+/).filter(Boolean);
  if(!tokens.length)return true;
  const hay=itemText(item).toLowerCase();
  return tokens.every(token=>hay.includes(token)||fuzzyNameIndexes(item.name||item.path||'',token).length);
}
function fileItem(pathValue,{kind='file',detail='Project file',recent=false}={}){
  pathValue=String(pathValue||'').trim();if(!pathValue)return null;
  return {
    id:'file:'+pathValue,kind,name:pathValue.split('/').pop()||pathValue,path:pathValue,detail,recent,
    run:()=>window.dispatchEvent(new CustomEvent('taskmenu:project-file-open-request',{detail:{path:pathValue,source:'quick-open'}}))
  };
}
function localQuickOpenItems(){
  const out=[],seen=new Set();
  const add=item=>{if(!item||!item.id||seen.has(item.id))return;seen.add(item.id);out.push(item);};
  for(const pathValue of globalThis.TaskMenuExplorer?.recent||[])add(fileItem(pathValue,{kind:'recent',detail:'Recently opened',recent:true}));
  for(const view of globalThis.TaskMenuEditor?.editors?.values?.()||[]){
    if(view?.closed)continue;const pathValue=String(view.file?.path||'').trim();if(!pathValue)continue;
    const remote=Boolean(view.file?.remote_workspace_id||view.file?.remote_transfer_profile_id);
    add({
      id:'editor:'+pathValue,kind:remote?'remote':'tab',
      name:(remote?'Remote Editor · ':'Editor · ')+(pathValue.split('/').pop()||pathValue),
      path:pathValue,detail:remote?'Open remote editor tab':'Open editor tab',
      run:()=>remote?globalThis.TaskMenuEditor?.activateEditor?.(view.id):globalThis.TaskMenuEditor?.openFile?.(pathValue)
    });
  }
  for(const [id,view] of app.views||[]){
    if(!view||view.closed||!view.tab)continue;
    const meta=view.meta||{};
    const title=String(meta.title||meta.label||view.tab.textContent||id).trim()||String(id);
    const terminal=Boolean(view.term);
    add({id:'session:'+id,kind:terminal?'terminal':'tab',name:(terminal?'Terminal · ':'Tab · ')+title,path:String(meta.cwd||''),detail:String(meta.status||''),run:()=>view.tab.click()});
  }
  for(const [id,view] of globalThis.TaskMenuDatabase?.views?.entries?.()||[]){
    const profileID=String(view?.meta?.profile_id||view?.profile?.id||'').trim();
    const label=String(view?.profile?.name||view?.meta?.label||profileID||id).trim();
    add({id:'db-tab:'+id,kind:'database',name:'Database · '+label,path:profileID,detail:'Open database tab',run:()=>view?.tab?.click?.()});
  }
  for(const tab of document.querySelectorAll('#tabs .tab,[role="tab"]')){
    if(tab.hidden||tab.offsetParent===null)continue;
    const id=String(tab.dataset?.id||tab.id||tab.textContent||'').trim();if(!id)continue;
    const label=String(tab.dataset?.title||tab.title||tab.textContent||id).replace(/\s+/g,' ').trim();
    add({id:'generic-tab:'+id,kind:'tab',name:'Tab · '+label,path:id,detail:'Open workspace tab',run:()=>tab.click()});
  }
  for(const [id,view] of globalThis.TaskMenuFileTransfer?.views?.entries?.()||[]){
    const profileID=String(view?.profile?.id||id).trim();if(!profileID)continue;
    const label=String(view?.profile?.name||view?.profile?.host||profileID).trim();
    add({id:'transfer-tab:'+profileID,kind:'transfer',name:'Transfer tab · '+label,path:profileID,detail:String(view?.profile?.protocol||'').toUpperCase(),run:()=>globalThis.TaskMenuFileTransfer?.openProfile?.(profileID)});
  }
  for(const profile of globalThis.TaskMenuConnections?.sshProfiles||[]){
    const id=String(profile?.id||'').trim();if(!id)continue;
    const label=String(profile?.name||profile?.label||profile?.host||id).trim();
    add({id:'ssh:'+id,kind:'ssh',name:'SSH · '+label,path:id,detail:[profile?.host,profile?.user].filter(Boolean).join(' · '),run:()=>globalThis.TaskMenuConnections?.openSSHProfile?.(id)});
  }
  for(const profile of globalThis.TaskMenuDatabase?.profiles||[]){
    const id=String(profile?.id||'').trim();if(!id)continue;
    const label=String(profile?.name||profile?.label||id).trim();
    add({id:'db-profile:'+id,kind:'database',name:'Database profile · '+label,path:id,detail:String(profile?.adapter||profile?.kind||''),run:()=>globalThis.TaskMenuDatabase?.openProfile?.(id)});
  }
  for(const profile of globalThis.TaskMenuFileTransfer?.profiles||[]){
    const id=String(profile?.id||'').trim();if(!id)continue;
    const label=String(profile?.name||profile?.label||profile?.host||id).trim();
    add({id:'transfer:'+id,kind:'transfer',name:String(profile?.protocol||'SFTP').toUpperCase()+' · '+label,path:id,detail:String(profile?.host||''),run:()=>globalThis.TaskMenuFileTransfer?.openProfile?.(id)});
  }
  return out;
}
function prefixSpec(raw){
  const value=String(raw||'');
  if(value.startsWith('>'))return {kind:'command',query:value.slice(1).trim()};
  if(value.startsWith('@'))return {kind:'symbol',query:value.slice(1).trim()};
  if(value.startsWith('#'))return {kind:'text',query:value.slice(1).trim()};
  const lower=value.toLowerCase();
  if(lower.startsWith('git:'))return {kind:'git',query:value.slice(4).trim()};
  if(lower.startsWith('ssh:'))return {kind:'ssh',query:value.slice(4).trim()};
  return {kind:'global',query:value.trim()};
}
function commandPrefixItems(query){
  const commands=globalThis.TaskMenuCommandPalette?.commandList?.()||[];
  return commands.filter(item=>localMatches({name:item.label||'',path:item.keywords||'',detail:item.shortcut||'',kind:'command'},query)).slice(0,50).map(item=>({
    id:'command:'+String(item.id||item.label),kind:'command',name:String(item.label||'Command'),path:String(item.shortcut||''),detail:String(item.keywords||''),disabled:Boolean(item.disabled),run:item.run
  }));
}
function prefixItems(spec){
  if(spec.kind==='command')return commandPrefixItems(spec.query);
  if(spec.kind==='symbol')return [{id:'symbols',kind:'symbol',name:'Symbols · '+(spec.query||'all project symbols'),path:'@'+spec.query,detail:'Open Project Symbols search',run:()=>globalThis.TaskMenuProjectSymbols?.open?.({query:spec.query})}];
  if(spec.kind==='text')return [{id:'text-search',kind:'text',name:'Search in Files · '+(spec.query||'project'),path:'#'+spec.query,detail:'Open project text search',run:()=>globalThis.TaskMenuProjectSearch?.open?.({query:spec.query})}];
  if(spec.kind==='git')return [{id:'git-search',kind:'git',name:'Git branch/commit search · '+(spec.query||'all'),path:'git:'+spec.query,detail:'Open Git Graph with search filter',run:()=>globalThis.TaskMenuGitFiles?.openGraphSearch?.(spec.query)}];
  if(spec.kind==='ssh')return localQuickOpenItems().filter(item=>item.kind==='ssh'&&localMatches(item,spec.query)).slice(0,50);
  return null;
}
function fuzzyNameIndexes(name,query){
  const hay=String(name||'').toLowerCase();
  const needle=String(query||'').toLowerCase().replace(/[*?%\s]+/g,'');
  if(!needle)return [];
  const indexes=[];let from=0;
  for(const ch of needle){
    const found=hay.indexOf(ch,from);
    if(found<0)return [];
    indexes.push(found);from=found+1;
  }
  return indexes;
}
function renderHighlightedName(node,name,query){
  const indexes=fuzzyNameIndexes(name,query);
  if(!indexes.length){node.textContent=name;return;}
  const marked=new Set(indexes);let start=0;
  for(let i=0;i<name.length;){
    const yes=marked.has(i);let end=i+1;
    while(end<name.length&&marked.has(end)===yes)end++;
    const part=document.createElement(yes?'span':'span');
    if(yes)part.className='quick-open-match';
    part.textContent=name.slice(i,end);node.append(part);i=end;
  }
}
function render(){
  results.replaceChildren();
  if(!items.length){setEmpty(input.value.trim()?'No matching files, tabs, terminals, databases or connections':'Recent files, open tabs, terminals, databases and saved connections');return;}
  const query=prefixSpec(input.value).query;
  items.forEach((item,index)=>{
    const button=document.createElement('button');button.className='quick-open-result'+(index===selected?' selected':'');button.type='button';button.dataset.path=item.path||'';button.disabled=Boolean(item.disabled);
    const name=document.createElement('span');name.className='quick-open-result-name';
    renderHighlightedName(name,item.name||String(item.path||''),query);
    const full=document.createElement('span');full.className='quick-open-result-path';full.textContent=[item.kind&&('['+item.kind+']'),item.path,item.detail].filter(Boolean).join(' · ');full.title=full.textContent;
    button.append(name,full);button.onclick=()=>choose(index);button.onmousemove=()=>{if(!item.disabled)select(index);};results.append(button);
  });
}
function select(index){
  if(!items.length)return;
  const direction=index>=selected?1:-1;let next=(index+items.length)%items.length;
  for(let step=0;step<items.length;step++){
    if(!items[next]?.disabled){selected=next;break;}
    next=(next+direction+items.length)%items.length;
  }
  [...results.querySelectorAll('.quick-open-result')].forEach((node,i)=>node.classList.toggle('selected',i===selected));
  results.querySelector('.quick-open-result.selected')?.scrollIntoView({block:'nearest'});
}
function choose(index=selected){
  const item=items[index];if(!item||item.disabled)return;
  close();
  Promise.resolve(item.run?.()).catch(app.showError);
}
function cancelPending(){
  clearTimeout(timer);timer=null;
  clearTimeout(refreshTimer);refreshTimer=null;
  if(controller){controller.abort();controller=null;}
}
function close(){
  cancelPending();seq++;backdrop.classList.remove('visible');items=[];selected=0;results.replaceChildren();
}
function open(options={}){
  backdrop.classList.add('visible');
  input.value=String(options.query||'');
  items=localQuickOpenItems().slice(0,60);selected=0;render();
  requestAnimationFrame(()=>{input.focus();if(input.value)schedule();});
}
function hasExactFilename(query){
  const wanted=String(query||'').trim().toLowerCase();
  if(!wanted||/[?*%]/.test(wanted))return false;
  return items.some(item=>String(item.name||'').toLowerCase()===wanted);
}
async function search(query,currentSeq,refresh=false){
  const spec=prefixSpec(query),prefixed=prefixItems(spec);
  if(prefixed){
    items=prefixed;selected=0;render();return;
  }
  const local=localQuickOpenItems().filter(item=>localMatches(item,spec.query)).slice(0,30);
  if(controller)controller.abort();
  controller=new AbortController();
  try{
    const suffix=refresh?'&refresh=1':'';
    const data=await app.jsonFetch('/api/project/files/search?q='+encodeURIComponent(spec.query)+'&limit=50'+suffix,{signal:controller.signal});
    if(currentSeq!==seq||!backdrop.classList.contains('visible'))return;
    const files=(Array.isArray(data.results)?data.results.slice(0,50):[]).map(row=>fileItem(row.path,{kind:'file',detail:'Project file'})).filter(Boolean);
    const seen=new Set(local.map(item=>item.id));items=[...local,...files.filter(item=>!seen.has(item.id))].slice(0,60);selected=0;render();
    const shouldRefresh=!refresh&&!/[?*%]/.test(spec.query)&&!hasExactFilename(spec.query)&&(files.length===0||spec.query.includes('.')||spec.query.includes('/'));
    if(shouldRefresh){
      refreshTimer=setTimeout(()=>{
        if(currentSeq===seq&&prefixSpec(input.value).query===spec.query&&backdrop.classList.contains('visible'))search(input.value.trim(),currentSeq,true);
      },450);
    }
  }catch(error){
    if(error?.name==='AbortError')return;
    if(currentSeq!==seq)return;
    setEmpty('Search failed');
    console.warn('Quick Open search failed',error);
  }
}
function schedule(){
  const query=input.value.trim();cancelPending();const currentSeq=++seq;
  if(!query){items=localQuickOpenItems().slice(0,60);selected=0;render();return;}
  timer=setTimeout(()=>search(query,currentSeq),80);
}

input.addEventListener('input',schedule);
input.addEventListener('keydown',event=>{
  if(event.key==='ArrowDown'){event.preventDefault();select(selected+1);}
  else if(event.key==='ArrowUp'){event.preventDefault();select(selected-1);}
  else if(event.key==='Enter'){event.preventDefault();choose();}
  else if(event.key==='Escape'){event.preventDefault();close();}
});
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
function globalQuickOpenShortcut(event){
  const primary=event.ctrlKey||event.metaKey;
  if(!primary||event.shiftKey||event.altKey||event.key.toLowerCase()!=='p')return;
  event.preventDefault();
  event.stopPropagation();
  if(backdrop.classList.contains('visible'))input.focus();else open();
}
window.addEventListener('keydown',globalQuickOpenShortcut,true);
document.addEventListener('keydown',event=>{
  if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();
});

globalThis.TaskMenuQuickOpen={open,close,localQuickOpenItems,prefixSpec};
