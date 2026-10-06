const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for project file actions');

const style=document.createElement('style');
style.textContent=`
.project-file-context{display:none;position:fixed;z-index:17000;min-width:220px;max-width:min(360px,92vw);max-height:min(620px,86vh);overflow:auto;padding:5px;border:1px solid #48515f;border-radius:8px;background:#171b22;box-shadow:0 14px 38px rgba(0,0,0,.45)}
.project-file-context.open{display:block}
.project-file-context-title{padding:5px 8px;font:10px ui-monospace,monospace;font-weight:700;opacity:.62;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.project-file-context-separator{height:1px;margin:4px 3px;background:#303844}
.project-file-context button{display:block;width:100%;text-align:left;border:0;background:transparent;color:inherit;padding:6px 8px;border-radius:4px;font-size:12px}
.project-file-context button:hover:not(:disabled){background:#2b3440}.project-file-context button.danger{color:#ff9a9a}.project-file-context button:disabled{opacity:.4}
html[data-taskmenu-theme="light"] .project-file-context{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .project-file-context button:hover:not(:disabled){background:#edf2f7}
`;
document.head.append(style);

const menu=document.createElement('div');menu.className='project-file-context';menu.setAttribute('role','menu');document.body.append(menu);

function cleanPath(value){return String(value||'').trim().replace(/^\.\//,'').replace(/\\/g,'/');}
function parentPath(value){value=cleanPath(value);const i=value.lastIndexOf('/');return i<0?'':value.slice(0,i);}
async function copyText(value){
  const text=String(value??'');
  if(navigator.clipboard&&window.isSecureContext){try{await navigator.clipboard.writeText(text);return;}catch{}}
  const area=document.createElement('textarea');area.value=text;area.setAttribute('readonly','');area.style.position='fixed';area.style.left='-9999px';document.body.append(area);area.select();try{document.execCommand('copy');}finally{area.remove();}
}
function openProjectFile(pathValue){
  window.dispatchEvent(new CustomEvent('taskmenu:project-file-open-request',{detail:{path:pathValue,source:'project-file-context'}}));
}
function openProjectHex(pathValue){
  window.dispatchEvent(new CustomEvent('taskmenu:project-hex-open-request',{detail:{path:pathValue,source:'project-file-context'}}));
}
async function revealProjectPath(pathValue){
  const explorer=globalThis.TaskMenuExplorer;if(!explorer?.reveal)throw new Error('Project Explorer unavailable');
  explorer.open?.();await explorer.reveal(pathValue);
}
async function openContainingFolder(pathValue){
  const explorer=globalThis.TaskMenuExplorer;if(!explorer)throw new Error('Project Explorer unavailable');
  explorer.open?.();
  if(explorer.openContainingFolder)return explorer.openContainingFolder(pathValue);
  const parent=parentPath(pathValue);return explorer.reveal?.(parent||pathValue);
}
function searchScope(pathValue,type){
  const scope=type==='dir'?cleanPath(pathValue):parentPath(pathValue);
  const search=globalThis.TaskMenuProjectSearch;if(!search?.open)throw new Error('Project Search unavailable');
  search.open({scope});
}
async function gitFileView(pathValue,mode){
  const git=globalThis.TaskMenuGitFiles;if(!git?.openWorkspaceFileView)throw new Error('Git file view unavailable');
  return git.openWorkspaceFileView(pathValue,mode);
}
async function projectIntegrity(pathValue){
  return app.jsonFetch('/api/project/integrity?path='+encodeURIComponent(cleanPath(pathValue)),{cache:'no-store'});
}
async function copyProjectChecksum(pathValue,algorithm){
  const data=await projectIntegrity(pathValue);
  const value=String(algorithm==='md5'?data?.md5:data?.sha256||'');
  if(!value)throw new Error('Checksum is unavailable');
  await copyText(value);
  alert((algorithm==='md5'?'MD5':'SHA-256')+' copied to clipboard for '+cleanPath(pathValue)+'\n\n'+value);
  return value;
}
function standardActions(pathValue,type='file'){
  pathValue=cleanPath(pathValue);type=type==='dir'?'dir':'file';
  const actions=[];
  if(type==='file')actions.push(
    {label:'Open',run:()=>openProjectFile(pathValue)},
    {label:'Open as Hex',run:()=>openProjectHex(pathValue)}
  );
  actions.push(
    {label:'Reveal in Explorer',run:()=>revealProjectPath(pathValue)},
    {label:'Open containing folder',run:()=>openContainingFolder(pathValue)},
    {label:type==='dir'?'Search / Replace in folder…':'Search / Replace in containing folder…',run:()=>searchScope(pathValue,type)}
  );
  if(type==='file')actions.push(
    {label:'Compare with another file…',run:()=>{
      const compare=globalThis.TaskMenuFileCompare;if(!compare?.promptProjectCompare)throw new Error('File Compare unavailable');
      return compare.promptProjectCompare(pathValue);
    }},
    {separator:true},
    {label:'SHA-256 checksum',run:()=>copyProjectChecksum(pathValue,'sha256')},
    {label:'MD5 checksum (compatibility)',run:()=>copyProjectChecksum(pathValue,'md5')},
    {separator:true},
    {label:'Git History',run:()=>gitFileView(pathValue,'history')},
    {label:'Git Blame',run:()=>gitFileView(pathValue,'blame')}
  );
  actions.push({separator:true},{label:'Copy path',run:()=>copyText(pathValue)});
  return actions;
}
function appendAction(action){
  if(action?.separator){const sep=document.createElement('div');sep.className='project-file-context-separator';menu.append(sep);return;}
  const button=document.createElement('button');button.type='button';button.textContent=action.label||'Action';button.title=action.title||button.textContent;button.disabled=Boolean(action.disabled);if(action.danger)button.classList.add('danger');
  button.onclick=event=>{event.preventDefault();event.stopPropagation();closeMenu();Promise.resolve(action.run?.()).catch(app.showError);};
  menu.append(button);
}
function clamp(x,y){
  menu.style.left=Math.max(4,x)+'px';menu.style.top=Math.max(4,y)+'px';
  requestAnimationFrame(()=>{if(!menu.classList.contains('open'))return;const rect=menu.getBoundingClientRect();menu.style.left=Math.max(4,Math.min(x,window.innerWidth-rect.width-4))+'px';menu.style.top=Math.max(4,Math.min(y,window.innerHeight-rect.height-4))+'px';});
}
function openMenu({path:pathValue,type='file',x=0,y=0,before=[],after=[],standard=true,title=''}={}){
  closeMenu();pathValue=cleanPath(pathValue);menu.replaceChildren();
  const heading=document.createElement('div');heading.className='project-file-context-title';heading.textContent=title||pathValue||'Project file actions';heading.title=heading.textContent;menu.append(heading);
  for(const action of before||[])appendAction(action);
  if(before?.length&&standard)appendAction({separator:true});
  if(standard)for(const action of standardActions(pathValue,type))appendAction(action);
  if(after?.length){appendAction({separator:true});for(const action of after)appendAction(action);}
  menu.classList.add('open');clamp(x,y);
}
function closeMenu(){menu.classList.remove('open');menu.replaceChildren();}
document.addEventListener('pointerdown',event=>{if(menu.classList.contains('open')&&!menu.contains(event.target))closeMenu();},true);
document.addEventListener('keydown',event=>{if(event.key==='Escape')closeMenu();});
window.addEventListener('blur',closeMenu);window.addEventListener('resize',closeMenu);

globalThis.TaskMenuProjectFileActions={standardActions,openMenu,closeMenu,copyText,parentPath};
