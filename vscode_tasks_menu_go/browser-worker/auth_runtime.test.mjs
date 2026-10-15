import test from 'node:test';
import assert from 'node:assert/strict';
import {observeHTTPChallenge,submitHTTPCredentials,dismissHTTPChallenge} from './auth_runtime.mjs';

const testPage={isClosed:()=>false};
function response({url='https://service.example.test/private',status=401,
  type='document',header='Basic realm="members"'}={}){
  return {url:()=>url,status:()=>status,request:()=>({
    resourceType:()=>type,frame:()=>({page:()=>testPage})}),
    headerValue:async()=>header};
}
function contextForTest(data={}){
  return {...data,windows:new Map([['main',{id:'main',page:testPage}]]),activeWindowId:'main'};
}
test('observed HTTPS 401 Basic challenge permits credentials only for actual origin',async()=>{
  let applied=null;
  const session=contextForTest({context:{setHTTPCredentials:async data=>{applied=data}}});
  observeHTTPChallenge(session,response());
  await Promise.resolve();await Promise.resolve();
  assert.equal(session.httpChallenge?.origin,'https://service.example.test');
  await submitHTTPCredentials(session,'alice','example-secret');
  assert.deepEqual(applied,{origin:'https://service.example.test',username:'alice',password:'example-secret'});
  assert.equal(session.httpChallenge,null);
  assert.ok(!JSON.stringify(session).includes('example-secret'),'password cannot be placed in snapshots/state');
});
test('never accept forged origin or credentials without real 401 challenge',async()=>{
  let calls=0;
  const session=contextForTest({context:{setHTTPCredentials:async()=>calls++}});
  await assert.rejects(()=>submitHTTPCredentials(session,'user','password'),/No HTTP authentication challenge/);
  for(const sample of [
    {url:'http://service.example.test/',status:401},
    {url:'https://service.example.test/',status:200},
    {url:'https://service.example.test/',status:401,type:'script'},
    {url:'https://service.example.test/',status:401,header:'Bearer realm="api"'}
  ]){
    observeHTTPChallenge(session,response(sample));
    await Promise.resolve();await Promise.resolve();
    assert.equal(session.httpChallenge,undefined);
  }
  assert.equal(calls,0);
});
test('browser HTTP Basic challenge can be dismissed without retaining input',async()=>{
  const session=contextForTest({httpChallenge:{origin:'https://service.example.test',window_id:'main',kind:'basic'}});
  dismissHTTPChallenge(session);
  assert.equal(session.httpChallenge,null);
});

test('Basic auth challenge in a popup is rejected from an unrelated active window',async()=>{
  let supplied=0;
  const popup={isClosed:()=>false};
  const session=contextForTest({
    activeWindowId:'main',
    context:{setHTTPCredentials:async()=>supplied++},
    windows:new Map([['main',{id:'main',page:testPage}],['popup-1',{id:'popup-1',page:popup}]])
  });
  const result={
    status:()=>401,url:()=> 'https://idp.example.test/private',
    request:()=>({resourceType:()=> 'document',frame:()=>({page:()=>popup})}),
    headerValue:async()=> 'Basic realm="private"'
  };
  observeHTTPChallenge(session,result);
  await Promise.resolve();await Promise.resolve();
  assert.equal(session.httpChallenge.window_id,'popup-1');
  await assert.rejects(()=>submitHTTPCredentials(session,'user','secret'),/Select the challenged browser window/);
  assert.equal(supplied,0);
  session.activeWindowId='popup-1';
  await submitHTTPCredentials(session,'user','secret');
  assert.equal(supplied,1);
});
