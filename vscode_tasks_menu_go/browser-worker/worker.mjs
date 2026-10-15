// TaskDeck Semantic Browser worker. Run under a separately sandboxed non-root
// identity/container with network egress policy; never alongside privileged Go.
import http from 'node:http';
import fs from 'node:fs';
import dns from 'node:dns/promises';
import net from 'node:net';
import crypto from 'node:crypto';
import {chromium} from 'playwright';
import {extractSemanticElements,installEventInstrumentation} from './semantic_extract.mjs';
import {fillControl,interactWithPage} from './universal_input.mjs';
import {createWindowSession,registerWindow,activeWindow,selectWindow,closeWindow,windowDescriptors,waitForActionOrDialog,resolveDialog,disposeWindowSession} from './popup_runtime.mjs';
import {observeHTTPChallenge,submitHTTPCredentials,dismissHTTPChallenge} from './auth_runtime.mjs';

const socket=process.env.TASKDECK_BROWSER_WORKER_SOCKET||'';
const token=process.env.TASKDECK_BROWSER_WORKER_TOKEN||'';
const hosts=(process.env.TASKDECK_BROWSER_ALLOWED_HOSTS||'').split(',').map(s=>s.trim().toLowerCase()).filter(Boolean);
const limit=Math.max(1,Math.min(16,Number(process.env.TASKDECK_BROWSER_MAX_SESSIONS)||6));
const idleMs=15*60*1000;
const allowHTTP=process.env.TASKDECK_BROWSER_ALLOW_HTTP==='1';
const sessions=new Map();
let browser=null;

function failure(message,status=400){const err=new Error(message);err.status=status;throw err;}
function authorized(request){
  const supplied=request.headers['x-taskdeck-worker-token'];
  if(typeof supplied!=='string'||!token)return false;
  const a=Buffer.from(supplied),b=Buffer.from(token);
  return a.length===b.length&&crypto.timingSafeEqual(a,b);
}
function matchedHost(host){
  return hosts.some(entry=>entry===host||(entry.startsWith('*.')&&host.endsWith(entry.slice(1))&&host.length>entry.length-1));
}
function privateAddress(ip){
  if(net.isIP(ip)===4){
    const [a,b]=ip.split('.').map(Number);
    return a===0||a===10||a===127||a>=224||a===169&&b===254||
      a===172&&b>=16&&b<=31||a===192&&b===168||a===100&&b>=64&&b<=127||
      a===192&&(b===0||b===88)||a===198&&(b===18||b===19||b===51)||a===203&&b===0;
  }
  if(net.isIP(ip)===6){
    const x=ip.toLowerCase();
    return x==='::'||x==='::1'||/^fe[89ab]/.test(x)||x.startsWith('fc')||x.startsWith('fd')||
      x.startsWith('ff')||x.startsWith('::ffff:')||x.startsWith('2001:db8:');
  }
  return true;
}
async function allowedURL(value){
  let u;
  try{u=new URL(value);}catch{return false;}
  if(!(u.protocol==='https:'||(allowHTTP&&u.protocol==='http:'))||u.username||u.password)return false;
  const hostname=u.hostname.toLowerCase().replace(/\.$/,'');
  if(!hostname||!matchedHost(hostname)||hostname==='localhost'||hostname.endsWith('.localhost')||hostname.endsWith('.local'))return false;
  if(net.isIP(hostname))return !privateAddress(hostname);
  try{
    const addresses=await dns.lookup(hostname,{all:true});
    return addresses.length>0&&addresses.every(item=>!privateAddress(item.address));
  }catch{return false;}
}
async function enforceURL(url){if(!(await allowedURL(url)))failure('Target URL is not allowed by browser worker network policy',403);}
async function startBrowser(){
  if(!browser)browser=await chromium.launch({headless:true,timeout:20000});
  return browser;
}
async function buildSession(input){
  if(sessions.size>=limit)failure('Browser session limit reached',429);
  if(!/^[0-9a-f]{48}$/.test(input.session_id||''))failure('Invalid browser session ID');
  if(sessions.has(input.session_id))failure('Browser session already exists',409);
  await enforceURL(input.url);
  const b=await startBrowser();
  const context=await b.newContext({javaScriptEnabled:true,acceptDownloads:false,serviceWorkers:'block'});
  try{
    // Browser-level request interception includes redirects, subresources and frames.
    // A container egress firewall is STILL REQUIRED, especially for WebSockets,
    // DNS rebinding and protocol features outside Playwright routing.
    await context.route('**/*',async route=>{
      try{
        if(await allowedURL(route.request().url()))await route.continue();
        else await route.abort('blockedbyclient');
      }catch{try{await route.abort('blockedbyclient');}catch{}}
    });
    // WebSockets need their own interception; HTTP request routes do NOT
    // cover them. Permit only hosts approved by the same network policy.
    await context.routeWebSocket('**/*',async ws=>{
      try{
        const address=ws.url().replace(/^ws:/,'http:').replace(/^wss:/,'https:');
        if(await allowedURL(address))ws.connectToServer();
        else await ws.close({code:1008,reason:'Blocked by TaskDeck browser policy'});
      }catch{await ws.close({code:1008,reason:'Blocked by TaskDeck browser policy'}).catch(()=>{});}
    });
    await context.addInitScript(installEventInstrumentation);
    const session=createWindowSession(context,input.session_id);
    context.on('response',response=>observeHTTPChallenge(session,response));
    // register before navigation; context receives every popup (window.open,
    // target=_blank, and OAuth windows) in this same browser context.
    const page=await context.newPage();
    registerWindow(session,page,{main:true});
    context.on('page',opened=>{registerWindow(session,opened);});
    sessions.set(session.id,session);
    await page.goto(input.url,{waitUntil:'domcontentloaded',timeout:20000});
    return await snapshot(session);
  }catch(err){
    sessions.delete(input.session_id);
    await context.close().catch(()=>{});
    throw err;
  }
}
function serialized(session,fn){
  const next=session.queue.catch(()=>{}).then(async()=>{
    session.seen=Date.now();
    const value=await fn();
    session.seen=Date.now();return value;
  });
  session.queue=next.catch(()=>{});
  return next;
}
// URL contents are untrusted; OAuth redirects may contain login credentials.
function publicURL(value){
  try{
    const u=new URL(value);
    if(!['http:','https:'].includes(u.protocol))return 'about:blank';
    u.username='';u.password='';
    for(const key of [...u.searchParams.keys()]){
      if(/token|secret|password|passwd|credential|authorization|auth_code|code|session|signature|api_key|access_key|assertion|saml|state|nonce|login_hint|request_uri|code_challenge|code_verifier|ticket|session_state/i.test(key))u.searchParams.set(key,'[redacted]');
    }
    if(/token|secret|credential|code|session|signature|assertion|saml/i.test(u.hash))u.hash='';
    return u.toString();
  }catch{return 'about:blank';}
}
async function snapshot(session){
  const event=session.windowEvent;
  session.windowEvent=null;
  const active=activeWindow(session);
  const page=active.page;
  session.revision++;
  const revision=session.revision,windows=await windowDescriptors(session,publicURL);
  // Pending alert/confirm/prompt blocks website JavaScript. Do not evaluate
  // page DOM or title until the user answers it.
  if(session.dialog){
    return {session_id:session.id,revision,active_window_id:active.id,
      windows,window_event:event,dialog:{...session.dialog},auth_challenge:session.httpChallenge||null,
      url:publicURL(page.url()),title:active.kind==='main'?'Main':'Popup',
      elements:[],truncated:false};
  }
  await page.waitForLoadState('domcontentloaded',{timeout:2000}).catch(()=>{});
  const elements=[],frames=page.frames().slice(0,8);
  session.snapshotFrames=frames;
  for(let index=0;index<frames.length&&elements.length<450;index++){
    try{
      const found=await frames[index].evaluate(extractSemanticElements,revision);
      for(const item of found.slice(0,450-elements.length))elements.push({...item,frame_index:index});
    }catch{} // frame may be detached, redirected or cross-origin in transit
  }
  return {
    session_id:session.id,revision,active_window_id:active.id,windows,
    window_event:event,dialog:null,auth_challenge:session.httpChallenge||null,
    url:publicURL(page.url()),title:await page.title().catch(()=>''),elements,
    truncated:elements.length>=450
  };
}
async function handleAction(session,input){
  const action=String(input.action||'');
  // Dialog resolution MUST bypass the serialized action queue because the
  // Playwright click that opened it may still be awaiting dialog.accept().
  if(action==='dialog_respond'){
    if(typeof input.accept!=='boolean')failure('Choose OK or Cancel');
    await resolveDialog(session,input);
    return serialized(session,()=>snapshot(session));
  }
  return serialized(session,async()=>{
    if(action==='switch_window'){
      if(session.dialog)failure('Answer the current website dialog first',409);
      selectWindow(session,String(input.window_id||''));
      return snapshot(session);
    }
    if(action==='close_window'){
      await closeWindow(session,String(input.window_id||''));
      return snapshot(session);
    }
    if(session.dialog&&action!=='snapshot')failure('Answer the current website dialog first',409);
    if(action==='snapshot')return snapshot(session);
    if(action==='http_auth_cancel'){dismissHTTPChallenge(session);return snapshot(session);}
    if(action==='http_auth'){
      await submitHTTPCredentials(session,input.http_username,input.http_password);
      const current=activeWindow(session);
      await current.page.reload({waitUntil:'domcontentloaded',timeout:20000});
      return snapshot(session);
    }
    activeWindow(session);
    const page=session.page;
    if(action==='navigate'){
      await enforceURL(input.url);
      await page.goto(input.url,{waitUntil:'domcontentloaded',timeout:20000});
    }else if(action==='back'){
      await page.goBack({waitUntil:'domcontentloaded',timeout:15000});
    }else if(action==='forward'){
      await page.goForward({waitUntil:'domcontentloaded',timeout:15000});
    }else if(action==='reload'){
      await page.reload({waitUntil:'domcontentloaded',timeout:20000});
    }else if(action!=='snapshot'){
      if(action==='interact'){
        if(!Number.isSafeInteger(input.revision)||input.revision!==session.revision)failure('Page changed; refresh before interacting',409);
        await waitForActionOrDialog(session,()=>interactWithPage(page,input,session.snapshotFrames||[]));
        return snapshot(session);
      }
      if(!Number.isSafeInteger(input.revision)||input.revision!==session.revision)failure('Page changed; refresh before interacting',409);
      if(!/^td-sem-[0-9a-z]+$/.test(input.element_id||''))failure('Invalid element ID');
      const locator=page.locator('[data-taskdeck-semantic-id="'+input.element_id+'"]');
      if(await locator.count()!==1)failure('Interactive element is no longer unique or available',409);
      await waitForActionOrDialog(session,async()=>{
        switch(action){
          case 'click':await locator.click({timeout:70000});break;
          case 'fill':await fillControl(page,locator,String(input.value||'').slice(0,32768));break;
          case 'fill_press':await locator.fill(String(input.value||'').slice(0,16384),{timeout:7000});await locator.press('Enter',{timeout:70000});break;
          case 'press':await locator.press(String(input.value||'Enter'),{timeout:70000});break;
          case 'check':await locator.setChecked(input.value==='true',{timeout:7000});break;
          case 'select':await locator.selectOption(String(input.value||''),{timeout:7000});break;
          case 'scroll':await locator.scrollIntoViewIfNeeded({timeout:7000});break;
          default:failure('Unsupported action');
        }
      });
    }
    return snapshot(session);
  });
}
async function readJSON(request){
  const chunks=[];let size=0;
  for await(const chunk of request){
    size+=chunk.length;if(size>192*1024)failure('Request exceeds limit',413);
    chunks.push(chunk);
  }
  try{return JSON.parse(Buffer.concat(chunks).toString('utf8'));}catch{failure('Invalid request JSON');}
}
function reply(response,status,body){
  response.writeHead(status,{'content-type':'application/json','cache-control':'no-store'});
  response.end(JSON.stringify(body));
}
const server=http.createServer(async(req,res)=>{
  try{
    if(!authorized(req))failure('Unauthorized worker caller',401);
    const parts=(req.url||'').split('/').filter(Boolean);
    if(parts[0]!=='sessions'||parts.length>2)failure('Not found',404);
    if(req.method==='POST'&&parts.length===1){
      reply(res,200,await buildSession(await readJSON(req)));return;
    }
    if(parts.length!==2||!/^[0-9a-f]{48}$/.test(parts[1]))failure('Not found',404);
    const session=sessions.get(parts[1]);
    if(!session)failure('Browser session not found',404);
    if(req.method==='DELETE'){
      sessions.delete(parts[1]);
      await disposeWindowSession(session);
      await serialized(session,()=>session.context.close());
      reply(res,200,{ok:true});return;
    }
    if(req.method==='POST'){
      const input=await readJSON(req);
      reply(res,200,await handleAction(session,input));return;
    }
    failure('Method not allowed',405);
  }catch(err){
    // Playwright diagnostics can contain typed field values, selectors and
    // URLs. Never return raw framework errors to the user's frontend.
    const safe=Number.isInteger(err?.status);
    reply(res,safe?err.status:502,{error:safe?String(err.message).slice(0,250):
      'Browser operation failed; refresh the page and try again'});
  }
});
if(!socket.startsWith('/')||token.length<24||hosts.length===0){
  throw new Error('Worker requires absolute TASKDECK_BROWSER_WORKER_SOCKET, strong TASKDECK_BROWSER_WORKER_TOKEN and explicit TASKDECK_BROWSER_ALLOWED_HOSTS');
}
if(fs.existsSync(socket)){
  const stat=fs.lstatSync(socket);
  if(!stat.isSocket())throw new Error('Refusing to overwrite non-socket path');
  fs.unlinkSync(socket);
}
server.listen(socket,()=>{fs.chmodSync(socket,0o600);process.stdout.write('Semantic Browser worker ready\n');});
const cleanup=setInterval(async()=>{
  for(const [id,s] of sessions){
    if(Date.now()-s.seen<idleMs)continue;
    sessions.delete(id);
    await disposeWindowSession(s);
    await serialized(s,()=>s.context.close()).catch(()=>{});
  }
},60000);
cleanup.unref();
async function shutdown(){
  clearInterval(cleanup);
  server.close();
  for(const s of sessions.values()){await disposeWindowSession(s);await s.context.close().catch(()=>{});}
  sessions.clear();
  await browser?.close().catch(()=>{});
  try{fs.unlinkSync(socket);}catch{}
}
process.on('SIGTERM',()=>{shutdown().finally(()=>process.exit(0));});
process.on('SIGINT',()=>{shutdown().finally(()=>process.exit(0));});
