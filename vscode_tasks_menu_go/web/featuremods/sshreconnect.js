// Interactive SSH auto-reconnect: a new server session replaces connectivity,
// while the original terminal tab and its error log remain available to inspect.
// Never re-run previously typed shell commands or circumvent SSH host-key policy.
(function(){
const app=globalThis.TaskMenuApp;
const policy=globalThis.TaskDeckSSHReconnectPolicy;
if(!app||!policy)throw Error('SSH reconnect prerequisites are unavailable');
const workspace=String(app.taskData?.workspace||'default');
const preferenceKey='taskdeck:ssh-auto-reconnect:'+workspace;
let autoEnabled=true;
try{autoEnabled=localStorage.getItem(preferenceKey)!=='off';}catch{}
const states=new Map();
const inheritedAttempts=new Map();
const style=document.createElement('style');
style.textContent=[
 '.taskdeck-ssh-reconnect{display:flex;align-items:center;flex-wrap:wrap;gap:5px;padding:4px 6px;font-size:11px;border-top:1px solid #3c5266;color:#cddfeb}',
 '.taskdeck-ssh-reconnect label{display:inline-flex;align-items:center;gap:4px;cursor:pointer}',
 '.taskdeck-ssh-reconnect p{margin:0;flex:1;min-width:130px}',
 '.taskdeck-ssh-reconnect button{padding:3px 7px;font-size:11px}',
 '.taskdeck-ssh-reconnect .taskdeck-ssh-state{font-weight:650}',
 'html[data-taskmenu-theme="light"] .taskdeck-ssh-reconnect{color:#253749;border-color:#b9c9d9}'
].join('\n');document.head.append(style);
const make=(tag,text)=>{const el=document.createElement(tag);if(text!==undefined)el.textContent=text;return el;};
function alive(st){return !st.cancelled&&!st.view.closed&&st.view.tab.isConnected&&app.views.get(st.id)===st.view&&!app.browserLeaseLost;}
function notify(st,message){st.message.textContent=message;st.bar.hidden=false;st.view.tab.title=message;}
function clearTimers(st){
 if(st.timer!==null){clearTimeout(st.timer);st.timer=null;}
 if(st.countdown!==null){clearInterval(st.countdown);st.countdown=null;}
 if(st.stable!==null){clearTimeout(st.stable);st.stable=null;}
}
function cancel(st,message){
 st.cancelled=true;clearTimers(st);
 st.cancel.hidden=true;
 if(message)notify(st,message);
}
function captureInput(st,value){
 let input=String(value||'');
 for(const ch of input){
  if(ch==='\x03'){st.line='';st.explicitLogout=false;continue;}
  if(ch==='\x04'){
   if(!st.line.trim())st.explicitLogout=true;
   continue;
  }
  if(ch==='\x7f'||ch==='\b'){st.line=st.line.slice(0,-1);continue;}
  if(ch==='\r'||ch==='\n'){
   st.explicitLogout=policy.explicitExit(st.line);
   st.line='';continue;
  }
  if(ch==='\x1b')continue; // key/control sequences are not shell commands
  if(ch>=' '&&ch!=='\x7f')st.line=(st.line+ch).slice(-500);
 }
}
function isPermissionOK(){
 return !app.sharedMode||app.hasPermission?.('ssh.use')===true;
}
async function profileFor(st,manual=false){
 // Do not silently auto-prompt for unsaved passwords/passphrases. The manual
 // path must open a visible terminal immediately for interactive login.
 const data=await app.jsonFetch('/api/ssh/profiles');
 const profile=(data.profiles||[]).find(p=>p.id===st.profileID);
 if(!profile){const e=new Error('SSH profile is no longer available');e.noAutoRetry=true;throw e;}
 if(!manual&&((profile.auth_method==='password'&&!profile.has_secret)||
   (profile.auth_method==='private_key'&&profile.has_passphrase&&!profile.has_secret))){
  const e=new Error('SSH credentials need interactive login · click Reconnect now');e.noAutoRetry=true;throw e;
 }
 return profile;
}
async function cleanupCandidate(id,abandon){
 try{
  let meta=await app.jsonFetch('/api/sessions/'+encodeURIComponent(id));
  if(abandon&&meta.status==='running'){
   await app.jsonFetch('/api/sessions/'+encodeURIComponent(id)+'/stop',{method:'POST'});
   for(let n=0;n<10;n++){
    await new Promise(resolve=>setTimeout(resolve,250));
    meta=await app.jsonFetch('/api/sessions/'+encodeURIComponent(id));
    if(meta.status!=='running')break;
   }
  }
  if(meta.status!=='running')await app.jsonFetch('/api/sessions/'+encodeURIComponent(id),{method:'DELETE'});
 }catch(error){console.warn('SSH reconnect candidate cleanup failed',error);}
}
async function waitCandidate(id,st,seconds){
 // Delay activation until fast SSH failures (host key, authentication,
 // connection refused) have been reported. Running state is not proof of
 // successful authentication, so subsequent 255 failures are still watched.
 const duration=Math.max(3000,Math.min(12000,seconds*1000));
 const started=Date.now();
 let current=null;
 while(Date.now()-started<duration){
  await new Promise(resolve=>setTimeout(resolve,1000));
  if(!alive(st))return null;
  current=await app.jsonFetch('/api/sessions/'+encodeURIComponent(id));
  if(current.status!=='running')return current;
 }
 return current;
}
async function attempt(st,manual=false){
 if(!alive(st)||st.busy)return;
 if(!isPermissionOK()){cancel(st,'Automatic SSH reconnect requires ssh.use permission');return;}
 st.busy=true;st.cancel.hidden=false;st.retry.disabled=true;
 let candidate=null,adopted=false;
 try{
  const profile=await profileFor(st,manual);
  if(!alive(st))return;
  notify(st,'Connecting SSH · attempt '+st.attempts+'/'+policy.maxAttempts+' …');
  candidate=await app.jsonFetch('/api/sessions',{
   method:'POST',headers:{'Content-Type':'application/json'},
   body:JSON.stringify({kind:'terminal',ssh_profile_id:st.profileID})
  });
  if(!candidate?.id)throw Error('SSH session creation returned no ID');
  const observed=manual?candidate:await waitCandidate(candidate.id,st,Math.min(Number(profile.connect_timeout_seconds)||8,10));
  if(!alive(st))return;
  if(!observed)throw Error('Could not verify SSH session state');
  if(observed.status==='running'){
   inheritedAttempts.set(candidate.id,st.attempts);
   app.materializeSession(observed,app.active===st.id);
   adopted=true;
   cancel(st,'SSH reconnected in a new tab · original session log preserved');
   st.retry.hidden=false;
   return;
  }
  const code=Number(observed.exit_code);
  if(observed.status==='stopped'||code===0||code!==255){
   cancel(st,'SSH ended with code '+String(observed.exit_code)+' · automatic retries stopped');
   return;
  }
  throw Error('SSH closed with exit code 255');
 }catch(error){
  if(!alive(st))return;
  st.lastError=String(error?.message||error).slice(0,250);
  if(manual||error?.noAutoRetry){
   cancel(st,'SSH reconnect requires manual action: '+st.lastError);
   st.retry.hidden=false;
  }else if(st.attempts>=policy.maxAttempts){
   cancel(st,'SSH retries exhausted ('+st.attempts+'/'+policy.maxAttempts+'). '+st.lastError+' · Retry manually');
  }else schedule(st,'SSH reconnect attempt failed: '+st.lastError);
 }finally{
  if(candidate?.id)await cleanupCandidate(candidate.id,!adopted);
  st.busy=false;st.retry.disabled=false;
 }
}
function schedule(st,message){
 if(!alive(st)||st.timer!==null)return;
 if(!autoEnabled){notify(st,'SSH disconnected · automatic reconnect disabled');return;}
 if(st.attempts>=policy.maxAttempts){
  cancel(st,'SSH reconnect limit reached · Retry manually');return;
 }
 const delay=policy.delay(st.attempts);
 let remaining=Math.ceil(delay/1000);
 notify(st,(message?message+' · ':'')+'Reconnecting in '+remaining+'s');
 st.cancel.hidden=false;
 st.countdown=setInterval(()=>{
  if(!alive(st)){cancel(st);return;}
  remaining=Math.max(0,remaining-1);
  notify(st,'SSH disconnected · reconnect in '+remaining+'s ('+(st.attempts+1)+'/'+policy.maxAttempts+')');
 },1000);
 st.timer=setTimeout(()=>{
  if(st.countdown!==null){clearInterval(st.countdown);st.countdown=null;}
  st.timer=null;
  if(!alive(st))return;
  st.attempts++;
  attempt(st,false).catch(error=>{cancel(st,'Reconnect failed: '+String(error?.message||error));});
 },delay);
}
function onExit(st,meta){
 if(!alive(st)||st.handledExit)return;
 st.handledExit=true;
 if(meta.status==='stopped'||meta.exit_code===0||st.explicitLogout){
  cancel(st,'SSH ended normally or was explicitly closed · auto reconnect skipped');
  st.retry.hidden=false;return;
 }
 if(policy.fatalError(st.tail)){
  cancel(st,'SSH authentication/host-key problem · check credentials or host key before reconnect');
  st.retry.hidden=false;return;
 }
 if(!policy.eligible(meta,{observedRunning:st.observedRunning,explicitLogout:st.explicitLogout,stoppedByUser:st.cancelled,tail:st.tail})){
  notify(st,'SSH closed · exit code '+String(meta.exit_code??'?')+' · no automatic retry');
  st.retry.hidden=false;return;
 }
 if(!autoEnabled){
  notify(st,'SSH disconnected (255) · automatic reconnect disabled');st.retry.hidden=false;return;
 }
 schedule(st,'SSH transport disconnected (255)');
}
function wire(view){
 const meta=view?.meta;
 if(!policy.isSSH(meta))return null;
 let st=states.get(meta.id);
 if(st)return st;
 const bar=make('div');bar.className='taskdeck-ssh-reconnect';
 const toggle=make('input');toggle.type='checkbox';toggle.checked=autoEnabled;
 const label=make('label');label.append(toggle,make('span','Auto reconnect'));
 const message=make('p','SSH reconnect enabled · exit 0 never retries');message.className='taskdeck-ssh-state';
 const retry=make('button','Reconnect now');retry.type='button';retry.hidden=true;
 const stop=make('button','Cancel retries');stop.type='button';stop.hidden=true;
 bar.append(label,message,retry,stop);
 view.pane.insertBefore(bar,view.pane.children[1]||null);
 st={id:meta.id,profileID:meta.target_profile_id,view,bar,message,retry,cancel:stop,
  observedRunning:meta.status==='running',explicitLogout:false,handledExit:false,
  attempts:inheritedAttempts.get(meta.id)||0,busy:false,cancelled:false,tail:'',line:'',
  timer:null,countdown:null,stable:null,lastError:''};
 inheritedAttempts.delete(meta.id);states.set(st.id,st);
 toggle.onchange=()=>{
  autoEnabled=toggle.checked;
  try{localStorage.setItem(preferenceKey,autoEnabled?'on':'off');}catch{}
  for(const item of states.values()){
   const checkbox=item.bar.querySelector('input');
   if(checkbox)checkbox.checked=autoEnabled;
   if(!autoEnabled&&(item.timer!==null||item.busy)){cancel(item,'Automatic SSH reconnect disabled');item.retry.hidden=false;}
  }
 };
 stop.onclick=()=>{cancel(st,'SSH automatic retries cancelled by user');retry.hidden=false;};
 retry.onclick=()=>{
  if(!alive(st)||st.busy)return;
  clearTimers(st);st.cancelled=false;st.handledExit=true;st.attempts=1;
  attempt(st,true).catch(error=>{cancel(st,String(error?.message||error));});
 };
 view.term.onData(data=>captureInput(st,data));
 if(st.observedRunning)st.stable=setTimeout(()=>{if(alive(st)&&st.view.meta?.status==='running')st.attempts=0;},60000);
 if(meta.status==='exited'||meta.status==='stopped'){
  // A page reload showing an old exited session is not a new disconnect.
  retry.hidden=false;notify(st,'SSH session already ended · manual reconnect available');
 }
 return st;
}
window.addEventListener('taskmenu:session',event=>{
 const view=event.detail?.view,meta=event.detail?.meta;
 const st=wire(view);
 if(!st||!meta)return;
 if(meta.status==='running'){
  st.observedRunning=true;
  return;
 }
 if(meta.status==='exited'||meta.status==='stopped')onExit(st,meta);
});
window.addEventListener('taskmenu:shell-integration',event=>{
 const {view,type,state,command}=event.detail||{};
 const st=states.get(view?.meta?.id);
 if(!st)return;
 if(type==='command-output'&&policy.explicitExit(state?.activeCommand))st.explicitLogout=true;
 if(type==='command-finished'&&policy.explicitExit(command?.command))st.explicitLogout=true;
});
window.addEventListener('taskmenu:output',event=>{
 const view=event.detail?.view,st=states.get(view?.meta?.id);
 if(!st)return;
 const data=event.detail?.data;
 let text='';
 if(typeof data==='string')text=data;
 else if(data instanceof ArrayBuffer){try{text=new TextDecoder().decode(data)}catch{}}
 if(text)st.tail=(st.tail+text).slice(-5000);
});
// Release abandoned per-tab controllers when a user closes old SSH tabs.
setInterval(()=>{
 for(const [id,st] of states){
  if(st.view.closed||!st.view.tab.isConnected){
   cancel(st);
   states.delete(id);
  }
 }
},30000);
globalThis.TaskDeckSSHReconnect={
 get enabled(){return autoEnabled;},
 getState(id){
  const s=states.get(String(id));return s?{attempts:s.attempts,explicitLogout:s.explicitLogout,
   observedRunning:s.observedRunning,handledExit:s.handledExit,message:s.message.textContent}:null;
 }
};
})();
