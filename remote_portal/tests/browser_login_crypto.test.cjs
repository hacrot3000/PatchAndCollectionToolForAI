'use strict';
// Exact WebCrypto -> Go encrypted TaskDeck-login wire-format interoperability.
// This test uses a dummy credential constant, NEVER a TaskDeck account or API.
const assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const {webcrypto}=require('node:crypto');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const readline=require('node:readline');

const root=path.resolve(__dirname,'../..');
const ctx=vm.createContext({crypto:webcrypto,TextEncoder,TextDecoder,atob,btoa,Uint8Array});
vm.runInContext(fs.readFileSync(path.join(root,'remote_portal/tests/fixtures/remote_login_crypto.js'),'utf8'),ctx);
const cipher=ctx.TaskDeckRemoteCrypto;

async function oneLogin(password,expected,alter=false) {
  const browser=await cipher.newBrowserSession();
  assert.equal(browser.privateKey.extractable,false);
  const probe='1'.repeat(32), nonce='2'.repeat(32);
  const child=spawn('go',['run','./cmd/taskdeck_login_crypto_fixture'],{
    cwd:path.join(root,'vscode_tasks_menu_go'),
    stdio:['pipe','pipe','pipe'],
  });
  let stderr='',isExited=false;
  child.stderr.setEncoding('utf8');
  child.stderr.on('data',s=>{stderr+=s;});
  const reader=readline.createInterface({input:child.stdout});
  const buffered=[],waiting=[];
  reader.on('line',line=>{
    const onLine=waiting.shift();
    if(onLine)onLine(line);else buffered.push(line);
  });
  const next=()=>new Promise((resolve,reject)=>{
    if(buffered.length){resolve(buffered.shift());return;}
    const timeout=setTimeout(()=>reject(new Error('Go login handshake timeout: '+stderr)),45000);
    waiting.push(line=>{clearTimeout(timeout);resolve(line);});
  });
  const exit=new Promise(resolve=>child.on('exit',code=>{isExited=true;resolve(code);}));
  const timer=setTimeout(()=>child.kill('SIGKILL'),85000);
  try {
    child.stdin.write(JSON.stringify({browser_key:browser.publicKeyBase64,
      probe_id:probe,client_nonce:nonce})+'\n');
    const hello=JSON.parse(await next());
    assert.equal(await cipher.verifyAgentReady(browser,hello.device_id,probe,nonce,hello.crypto),true);
    let encrypted=await cipher.sealRemoteLogin(browser,'alice',password);
    assert.equal(typeof encrypted,'string');
    assert.ok(!Buffer.from(encrypted,'base64').includes(Buffer.from(password)));
    await assert.rejects(cipher.sealRemoteLogin(browser,'alice',password),/fresh verified/);
    if(alter) {
      const corrupted=Buffer.from(encrypted,'base64');
      corrupted[corrupted.length-1]^=0x7f;
      encrypted=corrupted.toString('base64');
    }
    child.stdin.write(JSON.stringify({type:'e2ee_login',ciphertext:encrypted})+'\n');
    child.stdin.end();
    if (alter) {
      const code=await exit;
      assert.notEqual(code,0,'Go accepted tampered browser password frame');
      assert.ok(!stderr.includes(password),'Go leaked decrypted password to stderr');
      return;
    }
    const reply=JSON.parse(await next());
    assert.equal(reply.type,'e2ee_login_result');
    assert.equal(await cipher.verifyRemoteLoginResult(browser,reply.ciphertext),expected);
    await assert.rejects(cipher.verifyRemoteLoginResult(browser,reply.ciphertext),/No pending/);
    assert.equal(await exit,0,'Go fixture rejected valid encrypted login record: '+stderr);
  } finally {
    clearTimeout(timer);
    reader.close();
    if(!isExited)child.kill('SIGKILL');
  }
}

(async()=>{
  await oneLogin('test-only-e2ee-password',true);
  await oneLogin('different-dummy-password',false);
  await oneLogin('test-only-e2ee-password',false,true);
  console.log('PASS: Browser->Go encrypted credentials and Go->Browser boolean response');
  console.log('PASS: wrong login, tampered ciphertext, E2EE replay and JS session reuse rejected');
})().catch(e=>{console.error(e);process.exitCode=1;});
