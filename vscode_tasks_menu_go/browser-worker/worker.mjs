// TaskDeck Semantic Browser worker. Run under a separately sandboxed non-root
// identity/container with network egress policy; never alongside privileged Go.
import http from 'node:http';
import fs from 'node:fs';
import dns from 'node:dns/promises';
import net from 'node:net';
import crypto from 'node:crypto';
import {chromium} from 'playwright';
import {extractSemanticElements} from './semantic_extract.mjs';

const socket=process.env.TASKDECK_BROWSER_WORKER_SOCKET||'';
const token=process.env.TASKDECK_BROWSER_WORKER_TOKEN||'';
const hosts=(process.env.TASKDECK_BROWSER_ALLOWED_HOSTS||'').split(',').map(s=>s.trim().toLowerCase()).filter(Boolean);
const limit=Math.max(1,Math.min(16,Number(process.env.TASKDECK_BROWSER_MAX_SESSIONS)||6));
const idleMs=15*60*1000;
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
  return hosts.some(entry=>entry===host||(entry.startsWith('*.')&&host.endsWith(entry.slice(1))&&host.length>entry.length));
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
  if(!['https:','http:'].includes(u.protocol)||u.username||u.password)return false;
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
function watchPage(page){
  page.on('dialog',dialog=>dialog.dismiss().catch(()=>{}));
  page.on('download',download=>download.cancel().catch(()=>{}));
  page.on('popup',popup=>popup.close().catch(()=>{}));
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
    await context.addInitScript(()=>{
      const original=EventTarget.prototype.addEventListener;
      const listeners=new WeakSet();
      Object.defineProperty(window,'__taskdeckInteractiveListeners',{value:listeners,configurable:false});
      EventTarget.prototype.addEventListener=function(type,...args){
        if(['click','pointerdown','mousedown','touchstart'].includes(String(type).toLowerCase()))listeners.add(this);
        return Reflect.apply(original,this,[type,...args]);
      };
    });
    const page=await context.newPage();watchPage(page);
    const session={id:input.session_id,context,page,revision:0,seen:Date.now(),queue:Promise.resolve()};
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
async function snapshot(session){
  session.revision++;
  const revision=session.revision;
  const elements=await session.page.evaluate(extractSemanticElements,revision);
  return {
    session_id:session.id,revision,url:session.page.url(),
    title:await session.page.title().catch(()=>''),elements,truncated:elements.length>=450
  };
}
async function handleAction(session,input){
  return serialized(session,async()=>{
    const action=String(input.action||'');
    if(action==='navigate'){
      await enforceURL(input.url);
      await session.page.goto(input.url,{waitUntil:'domcontentloaded',timeout:20000});
    }else if(action==='back'){
      await session.page.goBack({waitUntil:'domcontentloaded',timeout:15000});
    }else if(action==='forward'){
      await session.page.goForward({waitUntil:'domcontentloaded',timeout:15000});
    }else if(action==='reload'){
      await session.page.reload({waitUntil:'domcontentloaded',timeout:20000});
    }else if(action!=='snapshot'){
      if(!Number.isSafeInteger(input.revision)||input.revision!==session.revision)failure('Page changed; refresh before interacting',409);
      if(!/^td-sem-[0-9a-z]+$/.test(input.element_id||''))failure('Invalid element ID');
      const locator=session.page.locator('[data-taskdeck-semantic-id="'+input.element_id+'"]');
      if(await locator.count()!==1)failure('Interactive element is no longer unique or available',409);
      switch(action){
        case 'click':await locator.click({timeout:7000});break;
        case 'fill':await locator.fill(String(input.value||'').slice(0,16384),{timeout:7000});break;
        case 'press':await locator.press(String(input.value||'Enter'),{timeout:7000});break;
        case 'check':await locator.setChecked(input.value==='true',{timeout:7000});break;
        case 'select':await locator.selectOption(String(input.value||''),{timeout:7000});break;
        case 'scroll':await locator.scrollIntoViewIfNeeded({timeout:7000});break;
        default:failure('Unsupported action');
      }
    }
    return snapshot(session);
  });
}
async function readJSON(request){
  const chunks=[];let size=0;
  for await(const chunk of request){
    size+=chunk.length;if(size>32768)failure('Request exceeds limit',413);
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
      await serialized(session,()=>session.context.close());
      reply(res,200,{ok:true});return;
    }
    if(req.method==='POST'){
      const input=await readJSON(req);
      reply(res,200,await handleAction(session,input));return;
    }
    failure('Method not allowed',405);
  }catch(err){
    reply(res,err.status||502,{error:String(err.message||err).slice(0,350)});
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
    await serialized(s,()=>s.context.close()).catch(()=>{});
  }
},60000);
cleanup.unref();
async function shutdown(){
  clearInterval(cleanup);
  server.close();
  for(const s of sessions.values())await s.context.close().catch(()=>{});
  sessions.clear();
  await browser?.close().catch(()=>{});
  try{fs.unlinkSync(socket);}catch{}
}
process.on('SIGTERM',()=>{shutdown().finally(()=>process.exit(0));});
process.on('SIGINT',()=>{shutdown().finally(()=>process.exit(0));});
