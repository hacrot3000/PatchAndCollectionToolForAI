// Development-only outbound presence UI. No remote command proxy exists here.
const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Remote settings');
const endpoint='/api/config/remote-access';
const button=document.createElement('button');
button.id='remote-access-settings';
button.type='button';
button.textContent='Remote Access…';
button.hidden=true;
document.body.append(button);
const style=document.createElement('style');
style.textContent=[
'.taskdeck-remote-overlay{position:fixed;inset:0;z-index:6700;background:rgba(0,0,0,.65);display:flex;align-items:center;justify-content:center;padding:12px}',
'.taskdeck-remote-dialog{width:min(620px,96vw);max-height:94dvh;overflow:auto;box-sizing:border-box;background:#1b2330;border:1px solid #53637b;border-radius:10px;color:#f1f5fa;padding:18px}',
'.taskdeck-remote-dialog h2{font-size:18px;margin:0 0 10px}',
'.taskdeck-remote-dialog p{font-size:12px;line-height:1.5}',
'.taskdeck-remote-dialog label{font-size:12px;display:flex;flex-direction:column;gap:6px;margin:12px 0}',
'.taskdeck-remote-dialog .toggle{flex-direction:row;align-items:center}',
'.taskdeck-remote-dialog input[type=url]{width:100%;box-sizing:border-box;background:#0b1521;border:1px solid #607087;color:inherit;border-radius:6px;padding:10px}',
'.taskdeck-remote-dialog code{display:block;overflow-wrap:anywhere;padding:9px;background:#0c1420;font-size:12px;border-radius:5px}',
'.taskdeck-remote-actions{display:flex;flex-wrap:wrap;gap:8px;justify-content:flex-end;margin-top:14px}',
'.taskdeck-remote-actions button{padding:9px 12px}',
'.taskdeck-remote-note{border-left:3px solid #c99542;padding:9px;background:#302518}',
'html[data-taskmenu-theme="light"] .taskdeck-remote-dialog{background:#fff;color:#1d2734}',
'html[data-taskmenu-theme="light"] .taskdeck-remote-dialog code,html[data-taskmenu-theme="light"] .taskdeck-remote-dialog input{background:#f1f5f9;color:#172236}'
].join('\n');
document.head.append(style);
let overlay=null,previousFocus=null;
function element(tag,text){
  const n=document.createElement(tag);
  if(text!==undefined)n.textContent=text;
  return n;
}
function close(){
  if(overlay){overlay.remove();overlay=null;}
  if(previousFocus?.isConnected)previousFocus.focus();
  previousFocus=null;
}
async function open(){
  if(app.sharedMode&&!app.hasPermission?.('project.admin'))throw new Error('Project admin required');
  const setting=await app.jsonFetch(endpoint);
  if(overlay)close();
  previousFocus=document.activeElement;
  overlay=element('div');
  overlay.className='taskdeck-remote-overlay';
  const dialog=element('section');
  dialog.className='taskdeck-remote-dialog';
  dialog.setAttribute('role','dialog');
  dialog.setAttribute('aria-modal','true');
  dialog.setAttribute('aria-label','TaskDeck Remote Access settings');
  const title=element('h2','Remote Access');
  const notice=element('p','Remote control is not implemented yet. Only outbound signed online presence is available in this development phase.');
  notice.className='taskdeck-remote-note';
  const switchLabel=element('label');
  switchLabel.className='toggle';
  const enabled=element('input');
  enabled.type='checkbox';
  enabled.checked=Boolean(setting.enabled);
  switchLabel.append(enabled,document.createTextNode('Enable outbound remote presence'));
  const portalLabel=element('label','Management Portal HTTPS URL');
  const portal=element('input');
  portal.type='url';
  portal.placeholder='https://remote.example.com';
  portal.value=setting.portal_url||'';
  portal.spellcheck=false;
  portal.autocomplete='off';
  portalLabel.append(portal);
  const deviceLabel=element('p','Device ID (public fingerprint)');
  const id=element('code',setting.device_id||'Available after enabling and restarting TaskDeck');
  const code=element('code');
  code.hidden=true;
  const status=element('p');
  status.setAttribute('role','status');
  const actions=element('div');
  actions.className='taskdeck-remote-actions';
  const dismiss=element('button','Close');
  dismiss.type='button';
  dismiss.onclick=close;
  const pair=element('button','Generate pairing code');
  pair.type='button';
  pair.disabled=!setting.enabled;
  const save=element('button','Save settings');
  save.type='button';
  actions.append(dismiss,pair,save);
  pair.onclick=async()=>{
    pair.disabled=true;
    code.hidden=true;code.textContent='';
    status.textContent='Generating a temporary pairing code…';
    try{
      const workspace=app.taskData?.workspace||'';
      const name=workspace.split(/[/\\]/).filter(Boolean).pop()||'TaskDeck';
      const result=await app.jsonFetch(endpoint,{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({action:'enroll',name})
      });
      id.textContent=result.device_id;
      code.textContent=result.pairing_code;
      code.hidden=false;
      status.textContent='Enter the Device ID and one-time code in the Portal within 5 minutes. Keep this code private.';
    }catch(e){status.textContent='Pairing error: '+String(e.message||e);}
    finally{pair.disabled=false;}
  };
  save.onclick=async()=>{
    save.disabled=true;pair.disabled=true;
    status.textContent='Saving…';
    try{
      const result=await app.jsonFetch(endpoint,{
        method:'PUT',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({enabled:enabled.checked,portal_url:portal.value.trim()})
      });
      code.textContent='';code.hidden=true;
      status.textContent=result.restart_required
        ? 'Saved. TaskDeck daemon is reloading. Reopen this dialog to verify the active configuration.'
        : 'Saved.';
    }catch(e){status.textContent='Save failed: '+String(e.message||e);pair.disabled=!setting.enabled;}
    finally{save.disabled=false;}
  };
  dialog.append(title,notice,switchLabel,portalLabel,deviceLabel,id,code,actions,status);
  overlay.append(dialog);
  document.body.append(overlay);
  overlay.addEventListener('pointerdown',ev=>{if(ev.target===overlay)close();});
  overlay.addEventListener('keydown',ev=>{
    if(ev.key==='Escape'){ev.preventDefault();close();}
    if(ev.key==='Tab'){
      const controls=[...dialog.querySelectorAll('button:not([disabled]),input:not([disabled])')];
      if(!controls.length)return;
      const first=controls[0],last=controls[controls.length-1];
      if(ev.shiftKey&&document.activeElement===first){ev.preventDefault();last.focus();}
      else if(!ev.shiftKey&&document.activeElement===last){ev.preventDefault();first.focus();}
    }
  });
  dismiss.focus();
}
button.onclick=()=>open().catch(error=>app.showError(error));
globalThis.TaskMenuRemoteAccess={open};
