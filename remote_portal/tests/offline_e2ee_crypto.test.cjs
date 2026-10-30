'use strict';
// Interop regression for the REAL inline offline Firefox client, using
// independently generated WebCrypto Ed25519 Agent + P-256 ECDH, live AEAD
// frames, fake Portal tampering and strict 3-round stop-and-wait.
const assert=require('node:assert/strict');
const {webcrypto}=require('node:crypto');
const {TextEncoder,TextDecoder}=require('node:util');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const html=fs.readFileSync(path.join(__dirname,'../offline/taskdeck_offline_client.html'),'utf8');
const inline=html.match(/<script>([\s\S]+?)<\/script>/);
assert.ok(inline);
const nodes=new Map();
const dom={getElementById(id){
 if(!nodes.has(id))nodes.set(id,{value:'',textContent:'',files:[],disabled:true,addEventListener(){}});
 return nodes.get(id);
}};
const ctx={document:dom,crypto:webcrypto,TextEncoder,TextDecoder,Uint8Array,URL,
 atob,btoa,console};
ctx.globalThis=ctx;
vm.runInNewContext(inline[1],ctx,{filename:'offline-client.html'});
const lib=ctx.TaskDeckOfflineCrypto;
const subtle=webcrypto.subtle;
const encode=new TextEncoder();
const base64=x=>Buffer.from(x).toString('base64');
const digest=async data=>Buffer.from(await subtle.digest('SHA-256',data)).toString('hex');
const concat=(...args)=>Buffer.concat(args.map(x=>Buffer.from(x)));
const header=seq=>{const b=Buffer.alloc(9);b[0]=1;b.writeBigUInt64BE(BigInt(seq),1);return b};
const nonce=(prefix,seq)=>concat(prefix,header(seq).subarray(1));
;(async()=>{
const pinnedAgent=await subtle.generateKey({name:'Ed25519'},true,['sign','verify']);
const agentPub=Buffer.from(await subtle.exportKey('raw',pinnedAgent.publicKey));
const deviceID=await digest(agentPub);
const pinned={deviceID,agentPublicKey:base64(agentPub)};
const b64sig=async(data)=>base64(await subtle.sign('Ed25519',pinnedAgent.privateKey,data));
const clone=source=>JSON.parse(JSON.stringify(source));
async function fakeAgentReady(probe){
 const eph=await subtle.generateKey({name:'ECDH',namedCurve:'P-256'},true,['deriveBits']);
 const rawEph=new Uint8Array(await subtle.exportKey('raw',eph.publicKey));
 const probeID='c'.repeat(32);
 const transcript=concat(encode.encode('taskdeck-e2ee-p256-v1'),new Uint8Array([0]),
  encode.encode(deviceID),encode.encode(probeID),encode.encode(probe.nonce),
  probe.publicKey,rawEph);
 const shared=await subtle.deriveBits({name:'ECDH',public:await subtle.importKey('raw',probe.publicKey,
  {name:'ECDH',namedCurve:'P-256'},false,[])},eph.privateKey,256);
 const binding=new Uint8Array(await subtle.digest('SHA-256',transcript));
 const hkdf=await subtle.importKey('raw',shared,'HKDF',false,['deriveBits']);
 const material=new Uint8Array(await subtle.deriveBits({name:'HKDF',hash:'SHA-256',
  salt:binding,info:encode.encode('taskdeck-e2ee-p256-v1/aes-gcm')},hkdf,72*8));
 const send=await subtle.importKey('raw',material.slice(36,68),{name:'AES-GCM'},false,['encrypt']);
 const receive=await subtle.importKey('raw',material.slice(0,32),{name:'AES-GCM'},false,['decrypt']);
 const seal=async(direction,seq,text)=>{
  const k=direction===2?send:receive;
  const ivPrefix=direction===2?material.slice(68,72):material.slice(32,36);
  const encrypted=new Uint8Array(await subtle.encrypt({name:'AES-GCM',
   iv:nonce(ivPrefix,seq),additionalData:concat(binding,new Uint8Array([direction]),header(seq)),
   tagLength:128},k,encode.encode(text)));
  return base64(concat(header(seq),encrypted));
 };
 const outer=encode.encode(['taskdeck-browser-attestation-v1',deviceID,probeID,probe.nonce].join('\n'));
 const response={ticket_valid:true,agent_reachable:true,remote_control_available:false,
  device_id:deviceID,agent_proof:{
   probe_id:probeID,client_nonce:probe.nonce,public_key:base64(agentPub),
   signature:await b64sig(outer),crypto:{
    identity:base64(agentPub),ephemeral:base64(rawEph),
    signature:await b64sig(transcript),
    encrypted_ready:await seal(2,1,'taskdeck-e2ee-ready-v1')
   }
  }};
 const openEcho=async(encoded,seq)=>{
  const frame=Buffer.from(encoded,'base64');
  assert.equal(frame.readBigUInt64BE(1),BigInt(seq));
  const plain=new Uint8Array(await subtle.decrypt({name:'AES-GCM',
   iv:nonce(material.slice(32,36),seq),
   additionalData:concat(binding,new Uint8Array([1]),frame.subarray(0,9)),
   tagLength:128},receive,frame.subarray(9)));
  return new TextDecoder().decode(plain);
 };
 return {response,seal,openEcho,binding};
}
 const probe=await lib.makeE2eeProbe();
 assert.match(probe.nonce,/^[a-f0-9]{32}$/);
 assert.equal(probe.publicKey.length,65);
 const agent=await fakeAgentReady(probe);
 for(const [label,mutate] of [
  ['wrong device ID',m=>m.device_id='0'.repeat(64)],
  ['bad Agent outer signature',m=>m.agent_proof.signature=base64(Buffer.alloc(64))],
  ['wrong proof nonce',m=>m.agent_proof.client_nonce='1'.repeat(32)],
  ['swapped Agent identity',m=>m.agent_proof.crypto.identity=base64(Buffer.alloc(32))],
  ['bad ECDH signature',m=>m.agent_proof.crypto.signature=base64(Buffer.alloc(64))],
  ['tampered encrypted ready',m=>m.agent_proof.crypto.encrypted_ready=base64(Buffer.alloc(90))],
  ['Relay claims remote control',m=>m.remote_control_available=true],
 ]){
  const tampered=clone(agent.response);
  mutate(tampered);
  await assert.rejects(()=>lib.verifyE2eeAgent(probe,pinned,tampered),undefined,label);
 }
 const state=await lib.verifyE2eeAgent(probe,pinned,agent.response);
 for(let seq=1;seq<=3;seq++){
  const encrypted=await lib.sealE2eeEcho(state);
  await assert.rejects(()=>lib.sealE2eeEcho(state),undefined,'pipelined echo accepted');
  const clear=await agent.openEcho(encrypted,seq);
  assert.match(clear,/^taskdeck-e2ee-echo-v1:[a-f0-9]{32}$/);
  const reply=await agent.seal(2,seq+1,'taskdeck-e2ee-echo-reply-v1:'+clear.split(':')[1]);
  assert.equal(await lib.verifyE2eeEcho(state,reply),seq);
 }
 await assert.rejects(()=>lib.sealE2eeEcho(state),undefined,'fourth echo accepted');
 const probe2=await lib.makeE2eeProbe();
 const agent2=await fakeAgentReady(probe2);
 const tamperedState=await lib.verifyE2eeAgent(probe2,pinned,agent2.response);
 const request=await lib.sealE2eeEcho(tamperedState);
 const plain=await agent2.openEcho(request,1);
 await assert.rejects(()=>lib.verifyE2eeEcho(tamperedState,
  base64(Buffer.alloc(80))),undefined,'forged AEAD reply accepted');
 const lateValid=await agent2.seal(2,2,'taskdeck-e2ee-echo-reply-v1:'+plain.split(':')[1]);
 await assert.rejects(()=>lib.verifyE2eeEcho(tamperedState,lateValid),
  undefined,'poisoned session revived after forged response');
 for(const bad of [
  'http://relay.example.com','https://relay.example.com/evil',
  'https://php.example.com@relay.example.com',
  'https://relay.example.com/?ticket=abc',
  'javascript:alert(1)'
 ]){
  assert.throws(()=>lib.strictRelayOrigin(bad));
 }
 assert.equal(lib.strictRelayOrigin('https://relay.example.com'),'wss://relay.example.com/v1/offline');

 // A trusted first-pair client proves Ed25519 PRIVATE-key possession only
 // after the Agent has signed the exact ECDH transcript. The PHP-issued
 // ticket and copied public key cannot forge this proof.
 const clientIdentity=await subtle.generateKey({name:'Ed25519'},true,['sign','verify']);
 const publicKey=base64(await subtle.exportKey('raw',clientIdentity.publicKey));
 const signingKey=await subtle.importKey('pkcs8',
  await subtle.exportKey('pkcs8',clientIdentity.privateKey),
  {name:'Ed25519'},false,['sign']);
 const trustedVault={username:'Alice',clientPublicKey:publicKey,signingKey};
 const probe3=await lib.makeE2eeProbe();
 const host3=await fakeAgentReady(probe3);
 const channel=await lib.verifyE2eeAgent(probe3,pinned,host3.response);
 const requestKey=await lib.sealE2eeClientKeyCheck(channel,trustedVault);
 const clearCheck=JSON.parse(await host3.openEcho(requestKey,1));
 assert.deepEqual(Object.keys(clearCheck).sort(),['public_key','signature','type','username']);
 assert.equal(clearCheck.type,'taskdeck-e2ee-client-key-check-v1');
 assert.equal(clearCheck.public_key,publicKey);
 assert.equal(clearCheck.username,'Alice');
 const signature=Buffer.from(clearCheck.signature,'base64');
 assert.equal(signature.length,64);
 const expectedProof=concat(encode.encode('taskdeck-remote-client-login-v1'),
  new Uint8Array([0]),encode.encode('alice'),new Uint8Array([0]),host3.binding);
 assert.deepEqual(Buffer.from(lib.clientProofMessage('Alice',host3.binding)),expectedProof);
 const verifyKey=await subtle.importKey('raw',Buffer.from(publicKey,'base64'),
  {name:'Ed25519'},false,['verify']);
 assert.equal(await subtle.verify('Ed25519',verifyKey,signature,expectedProof),true);
 const copiedPublicNoPrivate=await subtle.generateKey({name:'Ed25519'},true,['sign','verify']);
 const forged=await subtle.sign('Ed25519',copiedPublicNoPrivate.privateKey,expectedProof);
 assert.equal(await subtle.verify('Ed25519',verifyKey,forged,expectedProof),false);
 const positive=await host3.seal(2,2,JSON.stringify({
  type:'taskdeck-e2ee-client-key-status-v1',approved:true
 }));
 assert.equal(await lib.verifyE2eeClientKeyStatus(channel,{
  type:'e2ee_client_key_check_result',ciphertext:positive,remote_control_available:false
 }),true);
 await assert.rejects(()=>lib.sealE2eeClientKeyCheck(channel,trustedVault),
  undefined,'one-shot key preflight reused completed channel');
 const probe4=await lib.makeE2eeProbe();
 const host4=await fakeAgentReady(probe4);
 const channel4=await lib.verifyE2eeAgent(probe4,pinned,host4.response);
 const frame4=await lib.sealE2eeClientKeyCheck(channel4,trustedVault);
 assert.equal(typeof frame4,'string');
 const negative=await host4.seal(2,2,JSON.stringify({
  type:'taskdeck-e2ee-client-key-status-v1',approved:false
 }));
 assert.equal(await lib.verifyE2eeClientKeyStatus(channel4,{
  type:'e2ee_client_key_check_result',ciphertext:negative,remote_control_available:false
 }),false,'Host denial cannot be overridden by PHP admin');
 const probe5=await lib.makeE2eeProbe();
 const host5=await fakeAgentReady(probe5);
 const channel5=await lib.verifyE2eeAgent(probe5,pinned,host5.response);
 await lib.sealE2eeClientKeyCheck(channel5,trustedVault);
 await assert.rejects(()=>lib.verifyE2eeClientKeyStatus(channel5,{
  type:'e2ee_client_key_check_result',ciphertext:base64(Buffer.alloc(75)),
  remote_control_available:false
 }),undefined,'malicious PHP forged client-key status ciphertext');
 await assert.rejects(()=>lib.verifyE2eeClientKeyStatus(channel5,{
  type:'e2ee_client_key_check_result',
  ciphertext:await host5.seal(2,2,JSON.stringify({
   type:'taskdeck-e2ee-client-key-status-v1',approved:true
  })),
  remote_control_available:false
 }),undefined,'poisoned key-status channel revived');
 console.log('PASS: offline Firefox Ed25519 client-key preflight exactly matches Go Host ProofMessage');
 console.log('PASS: host-approved result is authenticated AEAD; forged/denied/replayed statuses fail closed');
 console.log('PASS: real inline offline Firefox crypto interoperates with Agent Ed25519, ECDH/HKDF/AES-GCM');
 console.log('PASS: 3 authenticated stop-and-wait echoes, replay/forgery poisons channel');
 console.log('PASS: forged PHP/Relay signatures, keys, status, nonce, ready ciphertext denied');
})().catch(e=>{console.error(e);process.exitCode=1});
