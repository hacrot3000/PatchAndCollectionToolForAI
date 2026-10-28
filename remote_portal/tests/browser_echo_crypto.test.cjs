'use strict';
// Browser WebCrypto <-> Go Agent E2EE interoperability using two messages
// in one OS process and one pair of ephemeral keys. No TaskDeck APIs involved.
const assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const {webcrypto}=require('node:crypto');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const readline=require('node:readline');
const root=path.resolve(__dirname,'../..');
const ctx=vm.createContext({crypto:webcrypto,TextEncoder,TextDecoder,atob,btoa,Uint8Array});
vm.runInContext(fs.readFileSync(path.join(root,'remote_portal/public/remote_crypto.js'),'utf8'),ctx);
const crypto=ctx.TaskDeckRemoteCrypto;

async function run() {
 const browser=await crypto.newBrowserSession();
 assert.equal(browser.privateKey.extractable,false);
 const probe='1'.repeat(32),nonce='2'.repeat(32);
 const child=spawn('go',['run','./cmd/taskdeck_crypto_fixture'],{
  cwd:path.join(root,'vscode_tasks_menu_go'),
  stdio:['pipe','pipe','pipe'],
 });
 let stderr='',finished=false;
 child.stderr.setEncoding('utf8');
 child.stderr.on('data',s=>{stderr+=s;});
 const reader=readline.createInterface({input:child.stdout});
 const waiting=[];
 const values=[];
 reader.on('line',line=>{
  const resolve=waiting.shift();
  if(resolve)resolve(line);else values.push(line);
 });
 const nextLine=()=>new Promise((resolve,reject)=>{
  if(values.length){resolve(values.shift());return;}
  const timer=setTimeout(()=>reject(new Error('timed out waiting for Go E2EE reply: '+stderr)),45000);
  waiting.push(line=>{clearTimeout(timer);resolve(line);});
 });
 const exit=new Promise((resolve,reject)=>child.once('exit',(code)=>{
  finished=true;
  if(code===0)resolve();else reject(new Error('Go crypto fixture exited '+code+': '+stderr));
 }));
 const timer=setTimeout(()=>child.kill('SIGKILL'),85000);
 try {
  child.stdin.write(JSON.stringify({browser_key:browser.publicKeyBase64,
   probe_id:probe,client_nonce:nonce})+'\n');
  const response=JSON.parse(await nextLine());
  assert.equal(await crypto.verifyAgentReady(browser,response.device_id,probe,nonce,response.crypto),true);
  const ciphertext=await crypto.sealDiagnosticEcho(browser);
  assert.ok(typeof ciphertext==='string' && ciphertext.length>40);
  // Actual second message is encrypted by WebCrypto and authenticated by Go.
  child.stdin.write(JSON.stringify({type:'e2ee_echo',ciphertext})+'\n');
  child.stdin.end();
  const answer=JSON.parse(await nextLine());
  assert.equal(answer.type,'e2ee_echo_reply');
  assert.equal(await crypto.verifyDiagnosticEcho(browser,answer.ciphertext),true);
  await assert.rejects(crypto.verifyDiagnosticEcho(browser,answer.ciphertext),/No pending/);
  await exit;
  console.log('PASS: WebCrypto->Go P-256/AES-GCM ciphertext and Go->WebCrypto AEAD reply');
  console.log('PASS: echo reply cannot be consumed twice');
 } finally {
  clearTimeout(timer);
  reader.close();
  if(!finished)child.kill('SIGKILL');
 }
}
run().catch(e=>{console.error(e);process.exitCode=1;});
