'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const {webcrypto}=require('node:crypto');
const source=fs.readFileSync(path.join(__dirname,'../public/remote_login.js'),'utf8');
const device='a'.repeat(64);
let submitHandler,mode='success';
const status={textContent:''};
const user={value:'alice'};
const pass={value:'private-sensitive-password'};
const button={disabled:false};
const form={
  dataset:{},
  elements:{namedItem:n=>n==='taskdeck_username'?user:n==='taskdeck_password'?pass:null},
  closest:()=>({querySelector:()=>({dataset:{deviceId:device}})}),
  querySelector:selector=>selector==='.remote-login-probe-result'?status:button
};
const doc={
 getElementById:()=>({dataset:{relayOrigin:'https://relay.example.com',csrf:'csrf-test'}}),
 addEventListener:(type,handler)=>{if(type==='submit')submitHandler=handler;}
};
class WS {
 constructor(uri){
  assert.equal(uri,'wss://relay.example.com/v1/browser');
  setImmediate(()=>this.onopen?.());
 }
 send(payload){
  const msg=JSON.parse(payload);
  const serialized=JSON.stringify(msg);
  assert.ok(!serialized.includes('private-sensitive-password'),'TaskDeck password leaked to Relay');
  assert.ok(!serialized.includes('alice'),'TaskDeck username leaked to Relay');
  if(msg.type==='e2ee_login'){
    assert.equal(msg.ciphertext,'opaque-aes-gcm-ciphertext');
    setImmediate(()=>this.onmessage?.({data:JSON.stringify({
      type:'e2ee_login_result',
      remote_control_available:false,
      ciphertext:mode==='forged'?'forged-reply':'opaque-agent-reply'
    })}));
  } else {
    assert.equal(msg.ticket,'t'.repeat(130));
    assert.match(msg.client_nonce,/^[a-f0-9]{32}$/);
    assert.equal(msg.browser_key,'mock-public-key');
    setImmediate(()=>this.onmessage?.({data:JSON.stringify({
      ticket_valid:true,agent_reachable:true,remote_control_available:false,
      device_id:device,agent_proof:{probe_id:'1'.repeat(32),crypto:{signature:'mock'}}
    })}));
  }
 }
 close(){}
}
const context={
 document:doc,URL,URLSearchParams,crypto:webcrypto,WebSocket:WS,
 setImmediate,setTimeout,clearTimeout,
 TaskDeckRemoteCrypto:{
  newBrowserSession:async()=>({publicKeyBase64:'mock-public-key'}),
  verifyAgentReady:async()=>true,
  sealRemoteLogin:async(session,username,password)=>{
    assert.equal(username,'alice');
    assert.equal(password,'private-sensitive-password');
    assert.equal(pass.value,'','password retained in DOM after submit');
    return 'opaque-aes-gcm-ciphertext';
  },
  verifyRemoteLoginResult:async(session,text)=>{
    if(text!=='opaque-agent-reply')throw Error('Invalid encrypted Agent login reply');
    return mode==='success';
  }
 },
 fetch:async(uri,options)=>{
  assert.equal(uri,'/api/remote/ticket');
  assert.equal(options.method,'POST');
  const data=new URLSearchParams(options.body);
  assert.equal(data.get('device_id'),device);
  assert.match(data.get('client_nonce'),/^[a-f0-9]{32}$/);
  assert.equal(data.get('browser_key'),'mock-public-key');
  assert.equal(data.get('csrf'),'csrf-test');
  assert.equal(data.has('username'),false);
  assert.equal(data.has('password'),false);
  assert.ok(!options.body.toString().includes('private-sensitive-password'));
  return {ok:true,json:async()=>({ticket:'t'.repeat(130)})};
 }
};
vm.runInNewContext(source,context,{filename:'remote_login.js'});
assert.equal(typeof submitHandler,'function');
async function run(label,expected){
 mode=label;
 pass.value='private-sensitive-password';
 user.value='alice';
 status.textContent='';
 let prevented=false;
 submitHandler({target:{closest:()=>form},preventDefault(){prevented=true;}});
 assert.equal(prevented,true);
 const deadline=Date.now()+2500;
 while(Date.now()<deadline){
  if(!button.disabled && status.textContent && !status.textContent.startsWith('Đang')){
   assert.match(status.textContent,expected);
   assert.equal(pass.value,'');
   assert.equal(form.dataset.running,undefined);
   console.log('PASS: browser E2EE login UI '+label+' has no cleartext credential transport');
   return;
  }
  await new Promise(resolve=>setTimeout(resolve,10));
 }
 throw Error('TaskDeck login UI did not finish '+label+': '+status.textContent);
}
(async()=>{
 await run('success',/Xác minh mật khẩu TaskDeck thành công/);
 await run('invalid',/Thông tin đăng nhập TaskDeck không hợp lệ/);
 await run('forged',/Invalid encrypted Agent login reply/);
})().catch(e=>{console.error(e);process.exitCode=1;});
