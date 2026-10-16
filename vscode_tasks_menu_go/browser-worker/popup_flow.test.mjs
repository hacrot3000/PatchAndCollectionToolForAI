import test from 'node:test';
import assert from 'node:assert/strict';
import {chromium} from 'playwright';
import {createWindowSession,registerWindow,activeWindow,waitForActionOrDialog,resolveDialog,settlePopupNavigation} from './popup_runtime.mjs';

async function fixture(t){
  let browser;
  try{browser=await chromium.launch({headless:true});}
  catch{t.skip('Chromium binary unavailable; install with npx playwright install chromium');return null;}
  const context=await browser.newContext();
  const session=createWindowSession(context,'e2e-popup');
  const main=await context.newPage();
  registerWindow(session,main,{main:true});
  context.on('page',opened=>registerWindow(session,opened));
  return {browser,context,session,main,close:()=>browser.close()};
}
test('OAuth-style popup sends postMessage to real opener and automatically closes',async t=>{
  const f=await fixture(t);if(!f)return;
  try{
    await f.main.setContent(`<!doctype html>
      <button id="login" onclick="window.open('about:blank','provider','width=500,height=450')">Login with provider</button>
      <div id="status">Signed out</div>
      <script>
      window.addEventListener('message',event=>{
        if(event.data?.kind==='test-login-done')document.getElementById('status').textContent='Signed in';
      });
      </script>`);
    const opened=f.context.waitForEvent('page');
    await f.main.locator('#login').click();
    const popup=await opened;
    await popup.waitForLoadState('domcontentloaded').catch(()=>{});
    assert.equal(activeWindow(f.session).page,popup);
    // Mock provider callback; no real Google credentials or cookie scraping.
    await popup.evaluate(()=>{
      window.opener.postMessage({kind:'test-login-done'},'*');
      window.close();
    });
    await f.main.locator('#status').getByText('Signed in').waitFor();
    assert.equal(f.session.activeWindowId,'main');
    assert.equal(f.session.windows.size,1);
  }finally{await f.close();}
});
test('confirm dialog is returned to UI without freezing the triggering click',async t=>{
  const f=await fixture(t);if(!f)return;
  try{
    await f.main.setContent(`<!doctype html>
      <button id="confirm" onclick="window.result=confirm('Sign in?')">Authorize</button>`);
    const action=await waitForActionOrDialog(f.session,()=>f.main.locator('#confirm').click());
    assert.equal(action,undefined);
    assert.equal(f.session.dialog?.type,'confirm');
    await resolveDialog(f.session,{dialog_id:f.session.dialog.id,accept:true});
    assert.equal(await f.main.evaluate(()=>window.result),true);
  }finally{await f.close();}
});
test('prompt response is submitted to website JavaScript then cleared from worker dialog',async t=>{
  const f=await fixture(t);if(!f)return;
  try{
    await f.main.setContent(`<!doctype html>
      <button id="challenge" onclick="window.answer=prompt('Code?')">Enter code</button>`);
    await waitForActionOrDialog(f.session,()=>f.main.locator('#challenge').click());
    assert.equal(f.session.dialog?.type,'prompt');
    await resolveDialog(f.session,{dialog_id:f.session.dialog.id,accept:true,prompt_text:'123456'});
    assert.equal(await f.main.evaluate(()=>window.answer),'123456');
    assert.equal(f.session.dialog,null);
  }finally{await f.close();}
});

test('OAuth-like popup transitions from initial about:blank to provider page',async t=>{
  const f=await fixture(t);if(!f)return;
  try{
    await f.main.setContent(`<!doctype html><button id="google" onclick="
      const child=window.open('about:blank','oauth_popup','width=500,height=650');
      setTimeout(()=>{if(!child.closed)child.location.href='data:text/html,%3Ctitle%3EProvider%3C/title%3E%3Cp%3ELogin%3C/p%3E';},110);
    ">Login with Google</button>`);
    const opened=f.context.waitForEvent('page');
    await f.main.locator('#google').click();
    const popup=await opened;
    const result=await settlePopupNavigation(f.session,2000);
    assert.equal(result.pending,false);
    assert.equal(activeWindow(f.session).page,popup);
    assert.notEqual(popup.url(),'about:blank');
    assert.match(popup.url(),/^data:text\/html/);
  }finally{await f.close();}
});
