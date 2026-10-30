'use strict';
// Test the actual offline HTML's embedded WebCrypto, NOT a duplicate codec.
// The browser app cannot fetch JS from PHP after first trusted pairing.
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const {webcrypto}=require('node:crypto');
const html=fs.readFileSync(path.join(__dirname,'../offline/taskdeck_offline_client.html'),'utf8');
assert.match(html,/<!doctype html>/i);
assert.match(html,/Content-Security-Policy/);
assert.match(html,/connect-src 'none'/);
assert.doesNotMatch(html,/(?:<script\s+src|<link\s+[^>]*href=["']https?:|<iframe)/i,
 'offline bundle must never load executable code from PHP or external hosts');
assert.doesNotMatch(html,/\b(?:fetch\s*\(|XMLHttpRequest|WebSocket|EventSource|serviceWorker|indexedDB|localStorage|sessionStorage)\b/i,
 'offline pairing has no network API or origin-dependent key persistence');
assert.doesNotMatch(html,/remote_control_available\s*[:=]\s*true/i,
 'offline pairing must not claim remote action capabilities');
const match=html.match(/<script>([\s\S]+?)<\/script>/);
assert.ok(match,'no self-contained JS found');
new vm.Script(match[1],{filename:'taskdeck_offline_client.html'});
const elements=new Map();
const dom={
 getElementById(id){
  if(!elements.has(id))elements.set(id,{value:'',textContent:'',disabled:true,files:[],addEventListener(){}});
  return elements.get(id);
 }
};
const sandbox={crypto:webcrypto,TextEncoder,Uint8Array,atob,btoa,document:dom,console};
sandbox.globalThis=sandbox;
const context=vm.createContext(sandbox);
vm.runInContext(match[1],context);
const client=context.TaskDeckOfflineCrypto;
assert.equal(typeof client.makeVault,'function');
assert.equal(typeof client.openVault,'function');
assert.equal(typeof client.agentPin,'function');
const b64=x=>Buffer.from(x).toString('base64');
const agent=await webcrypto.subtle.generateKey({name:'Ed25519'},true,['sign','verify']);
const agentPub=b64(await webcrypto.subtle.exportKey('raw',agent.publicKey));
const identity=await webcrypto.subtle.generateKey({name:'Ed25519'},true,['sign','verify']);
const clientPub=b64(await webcrypto.subtle.exportKey('raw',identity.publicKey));
const pkcs8=new Uint8Array(await webcrypto.subtle.exportKey('pkcs8',identity.privateKey));
const password='correct horse battery staples for this local vault';
const record=await client.makeVault({
 username:'alice',publicKey:clientPub,pkcs8,agentPublic:agentPub,pass:password
});
assert.equal(record.format,'taskdeck-offline-client-v1');
assert.equal(record.kdf.iterations,600000);
assert.equal(record.cipher.name,'AES-256-GCM');
assert.match(record.device_id,/^[a-f0-9]{64}$/);
assert.equal(await client.agentPin(agentPub),record.device_id);
assert.equal(record.client_public_key,clientPub);
assert.equal(record.agent_public_key,agentPub);
assert.equal(JSON.stringify(record).includes(b64(pkcs8)),false,
 'vault leaked unencrypted private PKCS8 bytes');
const same=await client.openVault(record,password);
assert.equal(same.username,'alice');
assert.equal(same.deviceID,record.device_id);
assert.equal(same.clientPublicKey,clientPub);
for(const [label,value] of [
 ['wrong password','wrong vault password'],
 ['empty password',''],
]){
 await assert.rejects(()=>client.openVault(record,value),undefined,label);
}
// The attacker who controls PHP can tamper with the JSON backup, replace
// a pinned Agent public key, change local account, or alter ciphertext.
// AES-GCM AAD must reject every one without exposing the private key.
async function tamper(update){
 const candidate=vm.runInContext('JSON.parse(source)',vm.createContext({source:JSON.stringify(record)}));
 update(candidate);
 await assert.rejects(()=>client.openVault(candidate,password));
}
await tamper(v=>{v.username='admin'});
await tamper(v=>{v.device_id='0'.repeat(64)});
await tamper(v=>{v.client_public_key=agentPub});
await tamper(v=>{v.agent_public_key=clientPub;v.device_id='0'.repeat(64)});
await tamper(v=>{v.cipher.iv=b64(Buffer.alloc(12))});
await tamper(v=>{v.encrypted_private_key=b64(Buffer.alloc(70))});
await tamper(v=>{v.kdf.iterations=1});
await assert.rejects(()=>client.makeVault({username:'alice',publicKey:clientPub,pkcs8,
 agentPublic:agentPub,pass:'short'}));
assert.equal(typeof sandbox.fetch,'undefined','offline client unexpectedly provides network fetch');
console.log('PASS: Offline HTML client is self-contained; no network or origin-dependent key store');
console.log('PASS: Ed25519 public-key matching, Agent Device ID pin, PBKDF2-600k/AES-256-GCM vault roundtrip');
console.log('PASS: wrong password, tampered metadata/ciphertext, weak passphrase fail closed');
