'use strict';
// Exercise the REAL offline Firefox pairing UI click handlers. The HTML must
// not export a host-pinned client identity until the operator explicitly
// compares the Agent Device ID against host CLI output out-of-band.
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const {webcrypto}=require('node:crypto');
const html=fs.readFileSync(path.join(__dirname,'../offline/taskdeck_offline_client.html'),'utf8');
const js=html.match(/<script>([\s\S]+?)<\/script>/)?.[1];
assert.ok(js,'missing self-contained offline client JS');
assert.match(html,/id="agent-verified" type="checkbox"/);
const nodes=new Map();
const doc={getElementById(id){
 if(!nodes.has(id)){
  const callbacks={};
  nodes.set(id,{value:'',checked:false,disabled:false,textContent:'',className:'',
   addEventListener(type,fn){callbacks[type]=fn;},callbacks});
 }
 return nodes.get(id);
}};
const n=doc.getElementById.bind(doc);
n('export').disabled=true;
const sandbox={document:doc,crypto:webcrypto,TextEncoder,TextDecoder,Uint8Array,URL,
 atob,btoa,console};
sandbox.globalThis=sandbox;
vm.runInNewContext(js,sandbox,{filename:'offline.html'});
const click=async(id,type='click')=>{
 const fn=n(id).callbacks[type];
 assert.equal(typeof fn,'function','missing '+type+' listener for '+id);
 return await fn();
};
(async()=>{
 n('username').value='alice';
 await click('create');
 assert.match(n('client-key').textContent,/^[a-zA-Z0-9+/]{43}=$/);
 assert.equal(n('export').disabled,true,'export before Agent pin calculation');
 const agent=await webcrypto.subtle.generateKey({name:'Ed25519'},true,['sign','verify']);
 const agentPub=btoa(String.fromCharCode(...new Uint8Array(await webcrypto.subtle.exportKey('raw',agent.publicKey))));
 n('agent-key').value=agentPub;
 await click('check-agent');
 assert.match(n('agent-fingerprint').textContent,/^[a-f0-9]{64}$/);
 assert.equal(n('export').disabled,true,'export without out-of-band confirmation');
 n('agent-verified').checked=true;
 await click('agent-verified','change');
 assert.equal(n('export').disabled,false,'correct confirmed host pin must allow encrypted backup');
 n('username').value='different-user';
 await click('username','input');
 assert.equal(n('export').disabled,true,'changed username must not silently reuse prior client approval');
 n('username').value='alice';
 await click('username','input');
 assert.equal(n('export').disabled,false,'reverting username should restore correctly bound key');
 n('agent-key').value='attacker-controlled-new-key';
 await click('agent-key','input');
 assert.equal(n('agent-verified').checked,false,'changed Agent key reused prior out-of-band confirmation');
 assert.equal(n('export').disabled,true,'changed Agent key reused prior host pin');
 n('agent-key').value=agentPub;
 await click('check-agent');
 assert.equal(n('export').disabled,true,'re-entered key must be confirmed again');
 console.log('PASS: out-of-band Device ID confirmation required, username/client key and Agent pin locked');
})().catch(e=>{console.error(e);process.exitCode=1});
