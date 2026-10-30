'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const {webcrypto}=require('node:crypto');
const html=fs.readFileSync(path.join(__dirname,'../offline/taskdeck_offline_client.html'),'utf8');
const source=html.match(/<script>([\s\S]+?)<\/script>/)?.[1];
assert.ok(source);
const nodes=new Map();
function el(id){
 if(!nodes.has(id)){
  const callbacks={};
  nodes.set(id,{value:'',textContent:'',disabled:false,files:[],checked:false,
   className:'',callbacks,addEventListener(kind,cb){callbacks[kind]=cb;}});
 }
 return nodes.get(id);
}
const doc={getElementById:el};
const sandbox={document:doc,console,crypto:webcrypto,TextEncoder,TextDecoder,
 Uint8Array,URL,atob,btoa,setTimeout,clearTimeout,AbortController};
sandbox.globalThis=sandbox;
vm.runInNewContext(source,sandbox,{filename:'taskdeck-offline.html'});
async function fire(id,type='click'){
 const cb=el(id).callbacks[type];
 assert.equal(typeof cb,'function','missing '+id+'.'+type);
 return cb();
}
const b64=x=>Buffer.from(x).toString('base64');
(async()=>{
 const client=sandbox.TaskDeckOfflineCrypto;
 const agent=await webcrypto.subtle.generateKey({name:'Ed25519'},true,['sign','verify']);
 const identity=await webcrypto.subtle.generateKey({name:'Ed25519'},true,['sign','verify']);
 const pkcs8=new Uint8Array(await webcrypto.subtle.exportKey('pkcs8',identity.privateKey));
 const pass='the-safe-offline-vault-passphrase';
 const record=await client.makeVault({
  username:'alice',publicKey:b64(await webcrypto.subtle.exportKey('raw',identity.publicKey)),
  pkcs8,agentPublic:b64(await webcrypto.subtle.exportKey('raw',agent.publicKey)),pass
 });
 const blob=JSON.stringify(record);
 let resumeRead;
 const fileA={size:blob.length,text:()=>new Promise(resolve=>{resumeRead=()=>resolve(blob)})};
 const fileB={size:blob.length,text:async()=>blob};
 el('vault-file').files=[fileA];
 await fire('vault-file','change');
 el('unlock-password').value=pass;
 const first=fire('unlock');
 assert.equal(el('unlock').disabled,true,'vault unlock not serialized');
 assert.equal(typeof resumeRead,'function');
 el('relay-origin').value='https://relay.example.com';
 await fire('prepare-probe');
 assert.match(el('status').textContent,/đang|mở vault/i);
 assert.equal(el('probe-nonce').textContent,'Chưa chuẩn bị');
 await fire('run-login');
 assert.match(el('status').textContent,/mở vault|đang/i);
 el('vault-file').files=[fileB];
 await fire('vault-file','change');
 resumeRead();
 await first;
 assert.equal(el('unlock').disabled,false);
 assert.match(el('status').textContent,/Không thể mở kho khóa/);
 await fire('prepare-probe');
 assert.equal(el('probe-nonce').textContent,'Chưa chuẩn bị');
 el('unlock-password').value=pass;
 await fire('unlock');
 assert.match(el('status').textContent,/Đã xác minh khóa client/);
 assert.equal(el('unlock-password').value,'','unlock password persisted');
 const pending=fire('prepare-probe');
 await fire('prepare-probe');
 await pending;
 const nonce=el('probe-nonce').textContent;
 const publicKey=el('probe-key').textContent;
 assert.match(nonce,/^[a-f0-9]{32}$/);
 assert.match(publicKey,/^[A-Za-z0-9+/]{86,88}={0,2}$/);
 assert.equal(el('prepare-probe').disabled,false);
 el('relay-origin').value='https://other-relay.example.com';
 await fire('relay-origin','input');
 assert.equal(el('probe-nonce').textContent,'Chưa chuẩn bị');
 assert.equal(el('probe-key').textContent,'Chưa chuẩn bị');
 await fire('vault-file','change');
 await fire('prepare-probe');
 assert.equal(el('probe-nonce').textContent,'Chưa chuẩn bị');
 console.log('PASS: vault change invalidates identity synchronously; concurrent unlock, ECDH and login fail closed');
 console.log('PASS: only current selected file may become trusted after PBKDF2, and relay changes discard ticket inputs');
})().catch(e=>{console.error(e);process.exitCode=1});
