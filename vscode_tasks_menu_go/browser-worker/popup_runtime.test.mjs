import test from 'node:test';
import assert from 'node:assert/strict';
import {EventEmitter} from 'node:events';
import {createWindowSession,registerWindow,activeWindow,selectWindow,
  closeWindow,windowDescriptors,waitForActionOrDialog,resolveDialog,disposeWindowSession,
  settlePopupNavigation,blockedDocumentHost} from './popup_runtime.mjs';

class FakePage extends EventEmitter {
  constructor(title,url,opener=null){super();this.name=title;this.urlValue=url;this.owner=opener;this.closed=false;}
  isClosed(){return this.closed;}
  url(){return this.urlValue;}
  title(){return Promise.resolve(this.name);}
  opener(){return Promise.resolve(this.owner);}
  async close(){this.closed=true;this.emit('close');}
}
function setup(){
  const session=createWindowSession({},'unit-session');
  const main=new FakePage('App','https://app.example.test/');
  registerWindow(session,main,{main:true});
  return {session,main};
}
test('normal popup opens, is selected, returns to opener on close',async()=>{
  const {session,main}=setup();
  const popup=new FakePage('Sign in','https://idp.example.test/',main);
  const entry=registerWindow(session,popup);
  assert.equal(activeWindow(session).id,entry.id);
  assert.equal(session.windows.size,2);
  let windows=await windowDescriptors(session,url=>url);
  assert.equal(windows.length,2);
  assert.equal(windows[1].active,true);
  selectWindow(session,'main');
  assert.equal(activeWindow(session).page,main);
  selectWindow(session,entry.id);
  await closeWindow(session,entry.id);
  assert.equal(session.activeWindowId,'main');
  assert.equal(activeWindow(session).page,main);
  assert.equal(session.windowEvent.type,'closed');
  assert.equal(session.windowEvent.returned_to,'main');
  await disposeWindowSession(session);
});
test('nested OAuth-style popups preserve nearest available opener',async()=>{
  const {session,main}=setup();
  const first=new FakePage('First','https://idp.example.test/a',main);
  const one=registerWindow(session,first);
  const second=new FakePage('Second','https://idp.example.test/b',first);
  const two=registerWindow(session,second);
  await new Promise(resolve=>setImmediate(resolve));
  assert.equal(session.windows.get(two.id).opener_id,one.id);
  await second.close();
  assert.equal(session.activeWindowId,one.id);
  await first.close();
  assert.equal(session.activeWindowId,'main');
  await disposeWindowSession(session);
});
test('alert exposes dialog immediately while click is blocked, responds without deadlock',async()=>{
  const {session,main}=setup();
  let dialogAccepted=false;
  const task=waitForActionOrDialog(session,async()=>{
    await new Promise(resolve=>setImmediate(resolve));
    await new Promise(resolve=>{
      const dialog={
        type:()=> 'alert',message:()=> 'Proceed?',
        accept:async()=>{dialogAccepted=true;resolve();},
        dismiss:async()=>resolve()
      };
      main.emit('dialog',dialog);
    });
  });
  await task;
  assert.equal(session.dialog.type,'alert');
  assert.equal(session.dialog.message,'Proceed?');
  assert.ok(session.pendingAction);
  await resolveDialog(session,{dialog_id:session.dialog.id,accept:true});
  assert.equal(dialogAccepted,true);
  assert.equal(session.dialog,null);
  assert.equal(session.pendingAction,null);
  await disposeWindowSession(session);
});
test('confirm Cancel and prompt text are delivered only to browser dialog, not snapshot',async()=>{
  const {session,main}=setup();
  const values=[];
  for(const kind of ['confirm','prompt']){
    const command=waitForActionOrDialog(session,()=>new Promise(resolve=>{
      main.emit('dialog',{
        type:()=>kind,message:()=> 'Browser question',
        accept:async(value)=>{values.push(kind+':accept:'+String(value||''));resolve();},
        dismiss:async()=>{values.push(kind+':cancel');resolve();}
      });
    }));
    await command;
    const id=session.dialog.id;
    assert.ok(!JSON.stringify(session.dialog).includes('sample-password'));
    await resolveDialog(session,{dialog_id:id,accept:kind==='prompt',prompt_text:'sample-password'});
    assert.equal(session.dialog,null);
  }
  assert.deepEqual(values,['confirm:cancel','prompt:accept:sample-password']);
  await disposeWindowSession(session);
});
test('popup cap rejects seventh window without leaking extra contexts',async()=>{
  const {session}=setup();let rejected=0;
  for(let i=0;i<7;i++){
    const p=new FakePage('Popup '+i,'https://app.example.test/x');
    if(!registerWindow(session,p))rejected++;
  }
  assert.equal(session.windows.size,6);
  assert.equal(rejected,2);
  await disposeWindowSession(session);
});

test('sequential dialogs from one website click cannot deadlock the action queue',async()=>{
  const {session,main}=setup();
  let unblock;
  const click=waitForActionOrDialog(session,()=>new Promise(resolve=>{unblock=resolve;
    main.emit('dialog',{
      type:()=> 'confirm',message:()=> 'First?',
      accept:async()=>{main.emit('dialog',{
        type:()=> 'prompt',message:()=> 'Second?',
        accept:async()=>resolve(),dismiss:async()=>resolve()
      });},
      dismiss:async()=>resolve()
    });
  }));
  await click;
  const first=session.dialog.id;
  await resolveDialog(session,{dialog_id:first,accept:true});
  assert.equal(session.dialog?.type,'prompt');
  await resolveDialog(session,{dialog_id:session.dialog.id,accept:true,prompt_text:'example'});
  assert.equal(session.dialog,null);
  assert.equal(session.pendingAction,null);
  await disposeWindowSession(session);
});

test('OAuth popup initial about:blank settles to provider redirect within bounded wait',async()=>{
  const {session,main}=setup();
  const popup=new FakePage('Pending','about:blank',main);
  popup.waitForURL=async(predicate)=>{
    await new Promise(resolve=>setTimeout(resolve,25));
    popup.urlValue='https://accounts.google.com/v3/signin/identifier?state=HIDDEN';
    if(!predicate(new URL(popup.urlValue)))throw new Error('expected HTTPS');
  };
  registerWindow(session,popup);
  const state=await settlePopupNavigation(session,250);
  assert.deepEqual(state,{pending:false});
  assert.equal(activeWindow(session).page,popup);
  assert.match(popup.url(),/^https:\/\/accounts.google.com\//);
});
test('blocked identity provider navigation keeps a safe public host, never OAuth state',async()=>{
  const {session,main}=setup();
  const popup=new FakePage('Pending','about:blank',main);
  popup.waitForURL=async()=>{throw new Error('timeout')};
  const entry=registerWindow(session,popup);
  session.blockedNavigation={window_id:entry.id,host:'accounts.google.com',at:Date.now()};
  const state=await settlePopupNavigation(session,10);
  assert.equal(state.pending,false);
  const oauth='https://accounts.google.com/v3/signin/identifier?state=DO_NOT_ECHO&code=SENSITIVE&login_hint=user%40example.com';
  assert.equal(blockedDocumentHost(oauth),'accounts.google.com');
  assert.equal(blockedDocumentHost('http://127.0.0.1/secret'),'');
  assert.equal(blockedDocumentHost('https://user:pass@accounts.google.com/'),'');
  assert.equal(blockedDocumentHost('javascript:alert(1)'),'');
});
test('provisional popup reports pending then expires rather than waiting forever',async()=>{
  const {session,main}=setup();
  const popup=new FakePage('Pending','about:blank',main);
  popup.waitForURL=async()=>{throw new Error('timeout')};
  const entry=registerWindow(session,popup);
  const pending=await settlePopupNavigation(session,2);
  assert.equal(pending.pending,true);
  entry.opened_at=Date.now()-10001;
  const expired=await settlePopupNavigation(session,2);
  assert.equal(expired.pending,false);
  assert.equal(expired.expired,true);
});
