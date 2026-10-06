const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for add-on registry');

const actions=new Map();
const panels=new Map();
const contextCommands=[];
const previewHandlers=[];
const manifests=new Map();
let loadPromise=null;

const style=document.createElement('style');
style.textContent=`
.addon-panel-backdrop{display:none;position:fixed;inset:0;z-index:17820;background:rgba(0,0,0,.5);align-items:flex-start;justify-content:center;padding:6vh 16px}.addon-panel-backdrop.visible{display:flex}
.addon-panel-dialog{width:min(900px,96vw);max-height:86vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 60px rgba(0,0,0,.58);overflow:hidden}.addon-panel-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #303843}.addon-panel-head strong{flex:1}.addon-panel-body{overflow:auto;padding:10px}.addon-panel-content{white-space:pre-wrap;overflow-wrap:anywhere;font:11px/1.45 ui-monospace,monospace}.addon-panel-data{margin-top:10px;white-space:pre-wrap;overflow:auto;background:#0b0f14;border:1px solid #303843;border-radius:6px;padding:7px;font:10px/1.4 ui-monospace,monospace}.addon-panel-error{color:#ff929d}.addon-panel-actions{display:flex;gap:6px;padding:8px 10px;border-top:1px solid #303843}
html[data-taskmenu-theme="light"] .addon-panel-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .addon-panel-data{background:#f6f8fa;border-color:#d0d7de}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='addon-panel-backdrop';
const dialog=document.createElement('div');dialog.className='addon-panel-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','TaskDeck add-on panel');
const head=document.createElement('div');head.className='addon-panel-head';
const title=document.createElement('strong');title.textContent='ADD-ON';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';
head.append(title,closeButton);
const body=document.createElement('div');body.className='addon-panel-body';
const footer=document.createElement('div');footer.className='addon-panel-actions';
const rerunButton=document.createElement('button');rerunButton.type='button';rerunButton.textContent='Run again';
const footerClose=document.createElement('button');footerClose.type='button';footerClose.textContent='Close';
footer.append(rerunButton,footerClose);dialog.append(head,body,footer);backdrop.append(dialog);document.body.append(backdrop);
let panelInvocation=null;

function permissionAllowed(permission){
  return !app.sharedMode||Boolean(app.hasPermission?.(permission)||app.hasPermission?.('project.admin'));
}
function normalizeID(value){return String(value||'').trim();}
function actionKey(addonID,actionID){return normalizeID(addonID)+':'+normalizeID(actionID);}
function extensionOf(pathValue){
  const value=String(pathValue||'').toLowerCase();
  const name=value.slice(value.lastIndexOf('/')+1);
  const dot=name.lastIndexOf('.');
  return dot>=0?name.slice(dot):'';
}
function permissionsAllowed(values){return (Array.isArray(values)?values:[]).every(permissionAllowed);}
function registerAction(addonID,spec){
  addonID=normalizeID(addonID);const id=normalizeID(spec?.id);
  if(!addonID||!id||typeof spec?.run!=='function')throw new Error('Add-on action requires addon id, action id and run callback');
  const item={addon_id:addonID,id,label:String(spec.label||id),keywords:String(spec.keywords||''),permissions:Array.isArray(spec.permissions)?spec.permissions.map(String):[],background:Boolean(spec.background),run:spec.run};
  actions.set(actionKey(addonID,id),item);return item;
}
function registerBackgroundJob(addonID,spec){
  if(typeof spec?.run!=='function')throw new Error('Background add-on job requires run callback');
  const original=spec.run;
  return registerAction(addonID,{...spec,background:true,run:async context=>{
    const controller=new AbortController();
    const operation=globalThis.TaskMenuOperationCenter?.begin?.({
      source:'addon',title:'Add-on · '+String(spec.label||spec.id||'Background job'),detail:String(addonID),
      cancel:()=>controller.abort()
    });
    try{
      const result=await original({...context,signal:controller.signal});
      operation?.complete?.({detail:String(result?.message||'Completed')});
      return result;
    }catch(error){
      operation?.fail?.(error);
      throw error;
    }
  }});
}
function registerPanel(addonID,spec){
  addonID=normalizeID(addonID);const id=normalizeID(spec?.id);
  if(!addonID||!id)throw new Error('Add-on panel requires addon id and panel id');
  const item={addon_id:addonID,id,title:String(spec.title||id),action_id:normalizeID(spec.action_id),render:typeof spec.render==='function'?spec.render:null};
  panels.set(actionKey(addonID,id),item);return item;
}
function registerContextMenuCommand(addonID,spec){
  addonID=normalizeID(addonID);const id=normalizeID(spec?.id);
  if(!addonID||!id)throw new Error('Add-on context command requires addon id and command id');
  const item={addon_id:addonID,id,label:String(spec.label||id),action_id:normalizeID(spec.action_id),scopes:Array.isArray(spec.scopes)?spec.scopes.map(String):[],extensions:Array.isArray(spec.extensions)?spec.extensions.map(value=>String(value).toLowerCase()):[],run:typeof spec.run==='function'?spec.run:null};
  contextCommands.push(item);return item;
}
function registerFilePreviewHandler(addonID,spec){
  addonID=normalizeID(addonID);const id=normalizeID(spec?.id);
  if(!addonID||!id)throw new Error('Add-on preview requires addon id and preview id');
  const item={addon_id:addonID,id,label:String(spec.label||id),action_id:normalizeID(spec.action_id),extensions:Array.isArray(spec.extensions)?spec.extensions.map(value=>String(value).toLowerCase()):[],open:typeof spec.open==='function'?spec.open:null};
  previewHandlers.push(item);return item;
}
function remoteAction(addon,action){
  return registerAction(addon.id,{
    ...action,
    run:context=>invokeRemote(addon.id,action.id,context,{background:Boolean(action.background)})
  });
}
function installManifest(addon){
  manifests.set(String(addon.id),addon);
  for(const action of Array.isArray(addon.actions)?addon.actions:[])remoteAction(addon,action);
  for(const panel of Array.isArray(addon.panels)?addon.panels:[])registerPanel(addon.id,panel);
  for(const item of Array.isArray(addon.context_menus)?addon.context_menus:[])registerContextMenuCommand(addon.id,item);
  for(const item of Array.isArray(addon.file_previews)?addon.file_previews:[])registerFilePreviewHandler(addon.id,item);
}
async function load(){
  if(loadPromise)return loadPromise;
  loadPromise=(async()=>{
    const data=await app.jsonFetch('/api/addons',{cache:'no-store'});
    for(const addon of Array.isArray(data?.addons)?data.addons:[])installManifest(addon);
    window.dispatchEvent(new CustomEvent('taskmenu:addons-loaded',{detail:{count:manifests.size}}));
    return [...manifests.values()];
  })().catch(error=>{loadPromise=null;console.warn('TaskDeck add-on registry load failed',error);return [];});
  return loadPromise;
}
async function invokeRemote(addonID,actionID,context={},options={}){
  const key=actionKey(addonID,actionID),registered=actions.get(key);
  if(registered&&!permissionsAllowed(registered.permissions))throw new Error('Permission denied for add-on action '+registered.label);
  const controller=new AbortController();
  const background=Boolean(options.background||registered?.background);
  const operation=background?globalThis.TaskMenuOperationCenter?.begin?.({
    source:'addon',title:'Add-on · '+String(registered?.label||actionID),detail:String(addonID),
    cancel:()=>controller.abort()
  }):null;
  try{
    const result=await app.jsonFetch('/api/addons/'+encodeURIComponent(addonID),{
      method:'POST',headers:{'Content-Type':'application/json'},signal:controller.signal,
      body:JSON.stringify({action_id:actionID,context:context&&typeof context==='object'?context:{}})
    });
    operation?.complete?.({detail:String(result?.message||'Completed')});
    return result;
  }catch(error){
    operation?.fail?.(error);
    throw error;
  }
}
async function invoke(addonID,actionID,context={}){
  await load();
  const item=actions.get(actionKey(addonID,actionID));
  if(!item)throw new Error('Add-on action not found: '+addonID+':'+actionID);
  if(!permissionsAllowed(item.permissions))throw new Error('Permission denied for add-on action '+item.label);
  return item.run(context);
}
function renderPanelResult(panel,result,error=null){
  body.replaceChildren();
  if(error){
    const node=document.createElement('div');node.className='addon-panel-error';node.textContent=String(error?.message||error);body.append(node);return;
  }
  if(panel.render){panel.render(body,result);return;}
  const content=document.createElement('div');content.className='addon-panel-content';content.textContent=String(result?.content||result?.message||'Completed');body.append(content);
  if(result?.data&&typeof result.data==='object'){
    const pre=document.createElement('pre');pre.className='addon-panel-data';pre.textContent=JSON.stringify(result.data,null,2);body.append(pre);
  }
}
async function openPanel(addonID,panelID,context={}){
  await load();
  const panel=panels.get(actionKey(addonID,panelID));
  if(!panel)throw new Error('Add-on panel not found: '+addonID+':'+panelID);
  title.textContent=panel.title;body.textContent='Loading…';backdrop.classList.add('visible');
  panelInvocation={addonID,panelID,context};
  try{
    const result=panel.action_id?await invoke(addonID,panel.action_id,context):{};
    renderPanelResult(panel,result);
  }catch(error){renderPanelResult(panel,null,error);}
}
function closePanel(){backdrop.classList.remove('visible');body.replaceChildren();panelInvocation=null;}
function commandItems(){
  const out=[];
  for(const item of actions.values()){
    out.push({
      id:'addon:'+item.addon_id+':'+item.id,label:'Add-on: '+item.label,
      keywords:'addon plugin '+item.addon_id+' '+item.keywords,
      disabled:!permissionsAllowed(item.permissions),
      run:()=>invoke(item.addon_id,item.id,{source:'command-palette'})
    });
  }
  for(const panel of panels.values()){
    out.push({
      id:'addon-panel:'+panel.addon_id+':'+panel.id,label:'Add-on Panel: '+panel.title,
      keywords:'addon plugin panel '+panel.addon_id,
      run:()=>openPanel(panel.addon_id,panel.id,{source:'command-palette'})
    });
  }
  return out;
}
function contextMenuActions(pathValue,type='file'){
  const ext=extensionOf(pathValue),scope=type==='dir'?'directory':'file',out=[];
  for(const item of contextCommands){
    if(item.scopes.length&&!item.scopes.includes(scope)&&!item.scopes.includes('project'))continue;
    if(item.extensions.length&&!item.extensions.includes(ext))continue;
    const action=actions.get(actionKey(item.addon_id,item.action_id));
    out.push({
      label:'Add-on · '+item.label,
      disabled:action?!permissionsAllowed(action.permissions):false,
      run:()=>item.run?item.run({path:pathValue,type}):invoke(item.addon_id,item.action_id,{path:pathValue,type,source:'context-menu'})
    });
  }
  return out;
}
function previewActions(pathValue){
  const ext=extensionOf(pathValue),out=[];
  for(const item of previewHandlers){
    if(item.extensions.length&&!item.extensions.includes(ext))continue;
    out.push({
      label:item.label,
      run:()=>item.open?item.open({path:pathValue}):openAddonPreview(item,pathValue)
    });
  }
  return out;
}
async function openAddonPreview(item,pathValue){
  const result=await invoke(item.addon_id,item.action_id,{path:pathValue,source:'file-preview'});
  title.textContent=item.label+' · '+String(pathValue||'');
  renderPanelResult({render:null},result);backdrop.classList.add('visible');
}
function addonSummary(){return [...manifests.values()].map(addon=>({id:addon.id,name:addon.name,description:addon.description||'',actions:(addon.actions||[]).length,panels:(addon.panels||[]).length}));}

closeButton.onclick=closePanel;footerClose.onclick=closePanel;
rerunButton.onclick=()=>{const current=panelInvocation;if(current)openPanel(current.addonID,current.panelID,current.context).catch(app.showError);};
backdrop.onmousedown=event=>{if(event.target===backdrop)closePanel();};
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))closePanel();});

globalThis.TaskMenuAddons={
  load,invoke,openPanel,registerAction,registerPanel,registerContextMenuCommand,registerFilePreviewHandler,registerBackgroundJob,
  commands:commandItems,contextMenuActions,previewActions,get manifests(){return addonSummary();}
};
load();
