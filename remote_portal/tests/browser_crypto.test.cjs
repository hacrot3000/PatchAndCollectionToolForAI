'use strict';
// Cross-language compatibility test: fresh browser P-256 ECDH public key
// -> Go Agent signed ephemeral response -> browser Ed25519 verification and
// AES-256-GCM decryption. No Portal secrets or TaskDeck commands are involved.
const assert = require('node:assert/strict');
const {webcrypto} = require('node:crypto');
const {execFileSync} = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const root = path.resolve(__dirname, '../..');
const context = vm.createContext({
  crypto:webcrypto, TextEncoder, TextDecoder, atob, btoa, Uint8Array,
});
const source = fs.readFileSync(path.join(root,'remote_portal/public/remote_crypto.js'),'utf8');
vm.runInContext(source,context,{filename:'remote_crypto.js'});
const browser = context.TaskDeckRemoteCrypto;
assert.ok(browser,'browser crypto module not exported');

(async()=>{
  const session = await browser.newBrowserSession();
  const probeID='1'.repeat(32), clientNonce='2'.repeat(32);
  const request=JSON.stringify({
    browser_key:session.publicKeyBase64,
    probe_id:probeID,
    client_nonce:clientNonce,
  });
  const response=execFileSync('go',['run','./cmd/taskdeck_crypto_fixture'],{
    cwd:path.join(root,'vscode_tasks_menu_go'),
    input:request,
    encoding:'utf8',
    timeout:90000,
  });
  const payload=JSON.parse(response);
  const deviceID=payload.device_id;
  const hello=payload.crypto;
  assert.equal(await browser.verifyAgentReady(session,deviceID,probeID,clientNonce,hello),true);
  console.log('PASS: browser independently decrypts Go Agent authenticated AES-GCM proof');

  const other=await browser.newBrowserSession();
  await assert.rejects(
    browser.verifyAgentReady(other,deviceID,probeID,clientNonce,hello),
    /sign|encrypt|decrypt|authenticated/i
  );
  console.log('PASS: different browser ephemeral key cannot reuse Agent handshake');

  await assert.rejects(
    browser.verifyAgentReady(session,deviceID,probeID,'3'.repeat(32),hello),
    /sign|encrypt|decrypt|authenticated/i
  );
  console.log('PASS: browser nonce substitution rejected');

  await assert.rejects(
    browser.verifyAgentReady(session,'0'.repeat(64),probeID,clientNonce,hello),
    /Device ID/i
  );
  console.log('PASS: device identity substitution rejected');

  const tampered=Object.assign({},hello,{encrypted_ready:Buffer.from(
    Buffer.from(hello.encrypted_ready,'base64').map((v,i,a)=>i===a.length-1?(v^1):v)
  ).toString('base64')});
  await assert.rejects(
    browser.verifyAgentReady(session,deviceID,probeID,clientNonce,tampered),
    /authentication failed/i
  );
  console.log('PASS: encrypted frame tampering rejected');

  const forged=Object.assign({},hello,{signature:Buffer.alloc(64).toString('base64')});
  await assert.rejects(
    browser.verifyAgentReady(session,deviceID,probeID,clientNonce,forged),
    /sign/i
  );
  console.log('PASS: forged Agent signature rejected');
})().catch(e=>{console.error(e);process.exitCode=1;});
