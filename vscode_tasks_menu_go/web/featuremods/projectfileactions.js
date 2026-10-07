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
async function projectPreviewInfo(pathValue){
  return app.jsonFetch('/api/project/preview?path='+encodeURIComponent(cleanPath(pathValue)),{cache:'no-store'});
}
async function openProjectImagePreview(pathValue,info=null){
  info=info||await projectPreviewInfo(pathValue);
  if(info?.kind!=='image')throw new Error('Selected file is not a supported image preview.');
  const preview=globalThis.TaskMenuFilePreview;if(!preview?.open)throw new Error('Image Preview unavailable');
  preview.open(info);
}
async function openProjectMarkdownPreview(pathValue,info=null){
  info=info||await projectPreviewInfo(pathValue);
  if(info?.kind!=='markdown')throw new Error('Selected file is not a Markdown file.');
  const preview=globalThis.TaskMenuMarkdownPreview;if(!preview?.open)throw new Error('Markdown Preview unavailable');
  preview.open(info);
}
function downloadProjectFile(pathValue){
  const link=document.createElement('a');link.href='/api/project/download?path='+encodeURIComponent(cleanPath(pathValue));link.download='';link.style.display='none';document.body.append(link);
  try{link.click();}finally{link.remove();}
}
function openProjectTerminalDirectory(pathValue){
  return app.startTerminal(parentPath(pathValue)||'.');
}
async function openProjectHostApplication(pathValue){
  if(app.sharedMode)throw new Error('Host application open is disabled in shared-server mode.');
  const response=await app.fetchWithLease('/api/project/open-host?path='+encodeURIComponent(cleanPath(pathValue)),{method:'POST'});
  if(!response.ok)throw new Error((await response.text()).trim()||('HTTP '+response.status));
  return response.json();
}
function projectCompareSource(pathValue){
  const compare=globalThis.TaskMenuFileCompare;if(!compare?.projectSource)throw new Error('File Compare unavailable');
  pathValue=cleanPath(pathValue);
  const writable=!app.sharedMode||Boolean(app.hasPermission?.('files.write')||app.hasPermission?.('project.admin'));
  return compare.projectSource(pathValue,{label:'Project · '+pathValue,writable});
}
function selectProjectForCompare(pathValue){
  const compare=globalThis.TaskMenuFileCompare;if(!compare?.selectForCompare)throw new Error('File Compare unavailable');
  return compare.selectForCompare(projectCompareSource(pathValue));
}
function compareProjectWithSelected(pathValue){
  const compare=globalThis.TaskMenuFileCompare;if(!compare?.compareWithSelected)throw new Error('File Compare unavailable');
  return compare.compareWithSelected(projectCompareSource(pathValue),{title:'Selected project files'});
}
async function openProjectDiff(pathValue){
  const compare=globalThis.TaskMenuFileCompare;if(!compare?.promptProjectCompare)throw new Error('File Compare unavailable');
  return compare.promptProjectCompare(pathValue);
}
function projectCompareActions(pathValue){
  const compare=globalThis.TaskMenuFileCompare,source=projectCompareSource(pathValue);
  const actions=[{label:'Select for compare',title:'Use this file as Compare A',run:()=>selectProjectForCompare(pathValue)}];
  if(compare?.selection)actions.push({
    label:'Compare with selected file',
    title:'Open File Compare using the previously selected Compare A file and this file',
    disabled:!compare.canCompareWithSelected?.(source),
    run:()=>compareProjectWithSelected(pathValue)
  });
  actions.push({label:'Compare with another project file…',run:()=>openProjectDiff(pathValue)});
  return actions;
}
function openWithButton(host,label,run,options={}){
  const button=document.createElement('button');button.type='button';button.textContent=label;button.disabled=Boolean(options.disabled);
  button.style.cssText='display:block;width:100%;text-align:left;margin:2px 0;padding:7px 9px';
  if(options.title)button.title=options.title;
  button.onclick=()=>Promise.resolve(run()).then(()=>host.close()).catch(app.showError);
  host.body.append(button);return button;
}
async function openProjectWith(pathValue){
  pathValue=cleanPath(pathValue);
  const info=await projectPreviewInfo(pathValue);
  const dialog=document.createElement('dialog');
  dialog.style.cssText='width:min(460px,92vw);background:#171b22;color:inherit;border:1px solid #48515f;border-radius:8px;padding:12px';
  const title=document.createElement('h3');title.textContent='Open With · '+pathValue;title.style.margin='0 0 8px';
  const meta=document.createElement('div');meta.style.cssText='font-size:11px;opacity:.68;margin-bottom:8px';meta.textContent=String(info?.kind||'file')+' · '+String(info?.size||0)+' B';
  const body=document.createElement('div');
  const close=document.createElement('button');close.type='button';close.textContent='Close';close.style.marginTop='8px';close.onclick=()=>dialog.close();
  dialog.body=body;
  const textCapable=info?.kind==='text'||info?.kind==='markdown';
  openWithButton(dialog,'Text Editor',()=>openProjectFile(pathValue),{disabled:!textCapable,title:textCapable?'Open in TaskDeck editor':'File is not detected as UTF-8 text'});
  openWithButton(dialog,'Hex',()=>openProjectHex(pathValue));
  openWithButton(dialog,'Image Preview',()=>openProjectImagePreview(pathValue,info),{disabled:info?.kind!=='image'});
  openWithButton(dialog,'Markdown Preview',()=>openProjectMarkdownPreview(pathValue,info),{disabled:info?.kind!=='markdown'});
  for(const addonPreview of globalThis.TaskMenuAddons?.previewActions?.(pathValue)||[]){
    openWithButton(dialog,'Add-on Preview · '+String(addonPreview.label||'Preview'),addonPreview.run);
  }
  openWithButton(dialog,'Diff…',()=>openProjectDiff(pathValue),{disabled:!textCapable,title:textCapable?'Compare with another project file':'Diff requires a supported text file'});
  openWithButton(dialog,'Download',()=>downloadProjectFile(pathValue));
  openWithButton(dialog,'Open terminal directory',()=>openProjectTerminalDirectory(pathValue));
  if(!app.sharedMode)openWithButton(dialog,'Open via host application',()=>openProjectHostApplication(pathValue));
  dialog.append(title,meta,body,close);document.body.append(dialog);
  dialog.addEventListener('close',()=>dialog.remove(),{once:true});dialog.showModal();
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
async function compareProjectChecksum(pathValue){
  const entered=prompt('Paste expected SHA-256 or MD5 checksum:','');
  if(entered===null)return;
  const expected=String(entered||'').trim().toLowerCase();
  if(!/^[0-9a-f]{32}$/.test(expected)&&!/^[0-9a-f]{64}$/.test(expected)){
    throw new Error('Expected checksum must contain 32 hex characters for MD5 or 64 for SHA-256.');
  }
  const data=await projectIntegrity(pathValue);
  const actual=expected.length===32?String(data?.md5||'').toLowerCase():String(data?.sha256||'').toLowerCase();
  if(actual!==expected)throw new Error('Checksum mismatch. Expected '+expected+' but calculated '+actual+'.');
  alert('Checksum matches for '+cleanPath(pathValue));
}
async function generateProjectManifest(pathValue){
  const data=await app.jsonFetch('/api/project/integrity',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({action:'manifest',path:cleanPath(pathValue)})
  });
  const content=String(data?.content||'');
  const blob=new Blob([content],{type:'text/plain;charset=utf-8'});
  const url=URL.createObjectURL(blob);
  const link=document.createElement('a');
  link.href=url;link.download='SHA256SUMS';link.style.display='none';document.body.append(link);
  try{link.click();}finally{setTimeout(()=>URL.revokeObjectURL(url),1000);link.remove();}
  alert('Generated SHA-256 manifest for '+String(data?.files||0)+' file(s).\nDownloaded as SHA256SUMS.');
}
async function verifyProjectManifest(pathValue){
  const data=await app.jsonFetch('/api/project/integrity',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({action:'verify_manifest',path:cleanPath(pathValue)})
  });
  const summary='Verified '+String(data?.total||0)+' entr'+(Number(data?.total)===1?'y':'ies')
    +' · matched '+String(data?.matched||0)
    +' · missing '+String(data?.missing||0)
    +' · mismatched '+String(data?.mismatched||0)
    +' · invalid '+String(data?.invalid||0);
  if(data?.ok){alert('Manifest verification passed.\n\n'+summary);return;}
  const failures=Array.isArray(data?.failures)?data.failures:[];
  const detail=failures.slice(0,20).map(item=>String(item.status||'error')+' · '+String(item.path||'')).join('\n');
  throw new Error('Manifest verification failed. '+summary+(detail?'\n\n'+detail:'')+(data?.truncated?'\n… additional failures omitted':''));
}

function projectArchiveKind(pathValue){
  const lower=cleanPath(pathValue).toLowerCase();
  return lower.endsWith('.zip')||lower.endsWith('.tar.gz')||lower.endsWith('.tgz');
}
async function previewProjectArchive(pathValue){
  const data=await app.jsonFetch('/api/project/archive/preview?path='+encodeURIComponent(cleanPath(pathValue)),{cache:'no-store'});
  const entries=Array.isArray(data?.entries)?data.entries:[];
  const dialog=document.createElement('dialog');
  dialog.style.cssText='width:min(900px,92vw);max-height:82vh;background:#171b22;color:inherit;border:1px solid #48515f;border-radius:8px;padding:12px';
  const title=document.createElement('h3');title.textContent='Archive preview · '+cleanPath(pathValue);
  const summary=document.createElement('div');summary.textContent=String(data?.format||'archive')+' · '+String(data?.files||0)+' file(s) · '+String(data?.dirs||0)+' folder(s) · '+String(data?.bytes||0)+' bytes'+(data?.truncated?' · preview truncated':'');
  summary.style.marginBottom='8px';
  const pre=document.createElement('pre');pre.style.cssText='max-height:58vh;overflow:auto;white-space:pre-wrap';
  pre.textContent=entries.map(item=>(item.type==='directory'?'[DIR] ':'      ')+String(item.path||'')+(item.type==='file'?' · '+String(item.size||0)+' B':'')).join('\n')||'(empty archive)';
  const close=document.createElement('button');close.type='button';close.textContent='Close';close.onclick=()=>dialog.close();
  dialog.append(title,summary,pre,close);document.body.append(dialog);
  dialog.addEventListener('close',()=>dialog.remove(),{once:true});dialog.showModal();
}

function archiveParentPath(pathValue){
  const value=cleanPath(pathValue);const i=value.lastIndexOf('/');return i<0?'':value.slice(0,i);
}
function archiveBaseName(pathValue){
  const value=cleanPath(pathValue);const name=value.slice(value.lastIndexOf('/')+1);
  return name.replace(/\.tar\.gz$|\.tgz$|\.zip$/i,'')||'archive';
}
async function createProjectArchive(paths,defaultOutput=''){
  paths=(Array.isArray(paths)?paths:[paths]).map(cleanPath).filter(Boolean);
  if(!paths.length)throw new Error('Select at least one project file or folder.');
  const first=paths[0],parent=archiveParentPath(first);
  const suggested=defaultOutput||((parent?parent+'/':'')+archiveBaseName(first)+'.zip');
  const output=prompt('Archive output path (.zip or .tar.gz):',suggested);
  if(output===null)return null;
  const format=String(output).toLowerCase().endsWith('.tar.gz')||String(output).toLowerCase().endsWith('.tgz')?'tar.gz':'zip';
  const response=await app.fetchWithLease('/api/project/archive/create',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({paths,output:cleanPath(output),format})
  });
  if(!response.ok)throw new Error(await response.text()||('HTTP '+response.status));
  const data=await response.json();
  alert('Archive created: '+String(data?.path||output));
  return data;
}
async function extractProjectArchive(pathValue){
  pathValue=cleanPath(pathValue);
  const parent=archiveParentPath(pathValue),suggested=(parent?parent+'/':'')+archiveBaseName(pathValue);
  const destination=prompt('Extract into a NEW project folder:',suggested);
  if(destination===null)return null;
  if(!confirm('Extract archive into new folder?\n\n'+cleanPath(destination)+'\n\nExisting destinations are never overwritten.'))return null;
  const response=await app.fetchWithLease('/api/project/archive/extract',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({path:pathValue,destination:cleanPath(destination)})
  });
  if(!response.ok)throw new Error(await response.text()||('HTTP '+response.status));
  const data=await response.json();
  alert('Archive extracted to '+String(data?.path||destination));
  return data;
}
async function downloadProjectPathsAsZip(paths,name='taskdeck-selection.zip'){
  paths=(Array.isArray(paths)?paths:[paths]).map(cleanPath).filter(Boolean);
  if(!paths.length)throw new Error('Select at least one project file or folder.');
  const response=await app.fetchWithLease('/api/project/archive/download',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({paths,name})
  });
  if(!response.ok)throw new Error(await response.text()||('HTTP '+response.status));
  const blob=await response.blob(),url=URL.createObjectURL(blob),link=document.createElement('a');
  link.href=url;link.download=name||'taskdeck-selection.zip';link.style.display='none';document.body.append(link);
  try{link.click();}finally{setTimeout(()=>URL.revokeObjectURL(url),1000);link.remove();}
}
function standardActions(pathValue,type='file'){
  pathValue=cleanPath(pathValue);type=type==='dir'?'dir':'file';
  const actions=[];
  if(type==='file')actions.push(
    {label:'Open',run:()=>openProjectFile(pathValue)},
    {label:'Open With…',run:()=>openProjectWith(pathValue)}
  );
  if(type==='file'&&projectArchiveKind(pathValue))actions.push(
    {label:'Preview archive…',run:()=>previewProjectArchive(pathValue)},
    {label:'Extract archive…',run:()=>extractProjectArchive(pathValue)}
  );
  if(type==='dir')actions.push(
    {label:'Create archive…',run:()=>createProjectArchive([pathValue])}
  );
  else actions.push({label:'Generate SHA-256 manifest…',run:()=>generateProjectManifest(pathValue)});
  actions.push(
    {label:'Reveal in Explorer',run:()=>revealProjectPath(pathValue)},
    {label:'Open containing folder',run:()=>openContainingFolder(pathValue)},
    {label:type==='dir'?'Search / Replace in folder…':'Search / Replace in containing folder…',run:()=>searchScope(pathValue,type)}
  );
  if(type==='file')actions.push(
    ...projectCompareActions(pathValue),
    {separator:true},
    {label:'SHA-256 checksum',run:()=>copyProjectChecksum(pathValue,'sha256')},
    {label:'MD5 checksum (compatibility)',run:()=>copyProjectChecksum(pathValue,'md5')},
    {label:'Compare checksum…',run:()=>compareProjectChecksum(pathValue)},
    {label:'Verify SHA-256 manifest…',run:()=>verifyProjectManifest(pathValue)},
    {separator:true},
    {label:'Git History',run:()=>gitFileView(pathValue,'history')},
    {label:'Git Blame',run:()=>gitFileView(pathValue,'blame')}
  );
  const addonActions=globalThis.TaskMenuAddons?.contextMenuActions?.(pathValue,type)||[];
  if(addonActions.length)actions.push({separator:true},...addonActions);
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

globalThis.TaskMenuProjectFileActions={projectCompareSource,selectProjectForCompare,compareProjectWithSelected,projectCompareActions,standardActions,openMenu,closeMenu,copyText,parentPath,createProjectArchive,extractProjectArchive,previewProjectArchive,downloadProjectPathsAsZip,projectArchiveKind,openProjectWith,projectPreviewInfo,downloadProjectFile,openProjectHostApplication};
