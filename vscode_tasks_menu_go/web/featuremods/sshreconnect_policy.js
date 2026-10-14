// SSH reconnect eligibility: independent of the task/process life cycle.
(function(){
 const delays=[2000,5000,10000,20000,30000];
 const exitPattern=/^\s*(?:(?:builtin|command)\s+)?(?:exit|logout)(?:\s+\d+)?\s*$/i;
 const fatalPattern=/REMOTE HOST IDENTIFICATION HAS CHANGED|Host key verification failed|WARNING: POSSIBLE DNS SPOOFING|Permission denied \((?:publickey|password|keyboard-interactive)|Too many authentication failures|Bad owner or permissions|sign_and_send_pubkey: signing failed|no matching host key type found|no matching key exchange method found/i;
 function isSSH(meta){
  if(!meta||String(meta.target_type||'').toLowerCase()!=='ssh'||
   typeof meta.target_profile_id!=='string'||!meta.target_profile_id.trim())return false;
  if(meta.kind==='terminal')return true;
  // Single-user historical SSH sessions have Kind="" and TaskID=-1.
  // Never confuse a Patch Tool process using SSH with an SSH terminal.
  return (meta.kind===''||meta.kind==null)&&Number(meta.task_id)===-1&&
   String(meta.label||'').startsWith('SSH · ');
 }
 function explicitExit(command){return exitPattern.test(String(command||''));}
 function fatalError(output){return fatalPattern.test(String(output||''));}
 function eligible(meta,{observedRunning=false,explicitLogout=false,stoppedByUser=false,tail=''}={}){
  if(!isSSH(meta)||!observedRunning||explicitLogout||stoppedByUser)return false;
  // OpenSSH returns 255 on SSH transport errors; exit 0 means normal logout.
  // Other codes may belong to a command and must not cause reconnect loops.
  return meta.status==='exited'&&Number(meta.exit_code)===255&&!fatalError(tail);
 }
 function delay(attempt){return delays[Math.max(0,Math.min(delays.length-1,Number(attempt)||0))];}
 const api={isSSH,explicitExit,fatalError,eligible,delay,maxAttempts:delays.length};
 globalThis.TaskDeckSSHReconnectPolicy=api;
 if(typeof module==='object'&&module?.exports)module.exports=api;
})();
