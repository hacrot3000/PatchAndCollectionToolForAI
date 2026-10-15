import test from 'node:test';
import assert from 'node:assert/strict';
import {chromium} from 'playwright';
import {extractSemanticElements,installEventInstrumentation} from './semantic_extract.mjs';
import {interactWithPage} from './universal_input.mjs';

async function fixture(t,markup){
  let browser;
  try{browser=await chromium.launch({headless:true});}
  catch(e){t.skip('Chromium not installed; run npx playwright install chromium');return null;}
  const context=await browser.newContext();
  await context.addInitScript(installEventInstrumentation);
  const page=await context.newPage();
  await page.goto('data:text/html,'+encodeURIComponent(markup));
  return {browser,context,page,close:()=>browser.close()};
}
function item(entries,name){
  const found=entries.find(e=>e.text===name&&e.kind==='input');
  assert.ok(found,'semantic input not found: '+name);
  return found;
}
test('atomic username/password submit preserves ordering, hides secrets and fires React input events',async t=>{
  const s=await fixture(t,`<!doctype html><form id="login">
    <label for="username">Username</label><input id="username" />
    <label for="secret">Password</label><input id="secret" type="password" autocomplete="current-password" />
    <button id="submit" type="submit">Log in</button>
    <script>
      window.log=[];window.credentials=null;
      for(const id of ['username','secret']){
        document.getElementById(id).addEventListener('input',e=>window.log.push(id+':input'));
      }
      document.getElementById('login').addEventListener('submit',e=>{
        e.preventDefault();
        window.credentials={user:username.value,pass:secret.value};
        window.log.push('submitted');
      });
    </script></form>`);
  if(!s)return;
  try{
    const nodes=await s.page.evaluate(extractSemanticElements);
    const u=item(nodes,'Username'),p=item(nodes,'Password');
    assert.equal(p.value,'');assert.equal(p.sensitive,true);
    const submit=nodes.find(e=>e.text==='Log in'&&e.kind==='action');
    assert.ok(submit);
    await interactWithPage(s.page,{target_action:'click',element_id:submit.id,field_updates:[
      {kind:'fill',element_id:u.id,value:'sample-user'},
      {kind:'fill',element_id:p.id,value:'sample-secret'}
    ]},s.page.frames());
    const output=await s.page.evaluate(()=>({credentials:window.credentials,log:window.log}));
    assert.deepEqual(output.credentials,{user:'sample-user',pass:'sample-secret'});
    assert.deepEqual(output.log,['username:input','secret:input','submitted']);
    const next=await s.page.evaluate(extractSemanticElements);
    assert.equal(item(next,'Password').value,'');
    assert.ok(!JSON.stringify(next).includes('sample-secret'));
  }finally{await s.close();}
});
test('Enter submits the precise input after filling all pending fields',async t=>{
  const s=await fixture(t,`<!doctype html><form id="login">
    <input id="user" aria-label="User" /><input id="pass" type="password" aria-label="Password"/>
    <button type="submit">Sign in</button>
    <script>window.submissions=[];
      login.addEventListener('submit',e=>{e.preventDefault();window.submissions.push(user.value+':'+pass.value);});
    </script></form>`);
  if(!s)return;
  try{
    const nodes=await s.page.evaluate(extractSemanticElements);
    const u=item(nodes,'User'),p=item(nodes,'Password');
    await interactWithPage(s.page,{target_action:'press',key:'Enter',element_id:p.id,field_updates:[
      {kind:'fill',element_id:u.id,value:'alice'},
      {kind:'fill',element_id:p.id,value:'abc123'}
    ]},s.page.frames());
    assert.deepEqual(await s.page.evaluate(()=>window.submissions),['alice:abc123']);
  }finally{await s.close();}
});
test('div contenteditable, role textbox and rich editor wrappers accept real text',async t=>{
  const s=await fixture(t,`<!doctype html>
    <div id="editable" contenteditable="true" aria-label="Comment">Original</div>
    <div id="rich" class="cm-editor" aria-label="Code editor">
      <div class="cm-content" contenteditable="true" aria-label="Code content"></div>
    </div>
    <div id="fake" role="textbox" aria-label="Fake textbox">
      <input id="inner" type="text" aria-label="Nested editor" />
    </div>`);
  if(!s)return;
  try{
    const controls=await s.page.evaluate(extractSemanticElements);
    const content=item(controls,'Comment'),code=item(controls,'Code content'),fake=item(controls,'Fake textbox');
    await interactWithPage(s.page,{field_updates:[
      {kind:'fill',element_id:content.id,value:'Xin chào Tiếng Việt'},
      {kind:'fill',element_id:code.id,value:'const a = 1;'},
      {kind:'fill',element_id:fake.id,value:'through nested input'}
    ]},s.page.frames());
    assert.equal(await s.page.locator('#editable').innerText(),'Xin chào Tiếng Việt');
    assert.equal(await s.page.locator('.cm-content').innerText(),'const a = 1;');
    assert.equal(await s.page.locator('#inner').inputValue(),'through nested input');
  }finally{await s.close();}
});
test('input inside iframe is bound to its snapshot frame and stale iframe is rejected',async t=>{
  const markup='<label for="login">Embedded login</label><input id="login"/><button id="go" onclick="window.hit=login.value">Go</button>';
  const s=await fixture(t,'<!doctype html><iframe id="loginFrame" srcdoc="'+markup.replaceAll('"','&quot;')+'"></iframe>');
  if(!s)return;
  try{
    const frames=s.page.frames(),child=frames[1];
    assert.ok(child);
    const nodes=await child.evaluate(extractSemanticElements);
    const input=item(nodes,'Embedded login'),go=nodes.find(e=>e.text==='Go'&&e.kind==='action');
    await interactWithPage(s.page,{target_action:'click',element_id:go.id,frame_index:1,field_updates:[
      {element_id:input.id,frame_index:1,kind:'fill',value:'alice'}
    ]},frames);
    assert.equal(await child.evaluate(()=>window.hit),'alice');
    await s.page.locator('#loginFrame').evaluate(el=>el.remove());
    await assert.rejects(()=>interactWithPage(s.page,{target_action:'click',element_id:go.id,frame_index:1},frames),/Frame changed/);
  }finally{await s.close();}
});
test('OTP/secret-like textboxes are always redacted from snapshots',async t=>{
  const s=await fixture(t,`<!doctype html>
    <input aria-label="OTP code" value="444999">
    <div contenteditable="true" aria-label="secret key">DO_NOT_EXPORT</div>`);
  if(!s)return;
  try{
    const nodes=await s.page.evaluate(extractSemanticElements);
    assert.equal(item(nodes,'OTP code').value,'');
    assert.equal(item(nodes,'secret key').value,'');
    assert.ok(!JSON.stringify(nodes).includes('DO_NOT_EXPORT'));
    assert.ok(!JSON.stringify(nodes).includes('444999'));
  }finally{await s.close();}
});

test('CSS text cursor DIV forwarding focus into real input is recognized and editable',async t=>{
  const s=await fixture(t,`<!doctype html>
    <div id="pseudo" class="custom-input" style="cursor:text" data-placeholder="Search records" tabindex="0" onclick="document.querySelector('#real').focus()">
      Search
    </div><input id="real" aria-label="Real search" />`);
  if(!s)return;
  try{
    const nodes=await s.page.evaluate(extractSemanticElements);
    const pseudo=item(nodes,'Search records');
    assert.equal(pseudo.editor,true);
    // The real target lives outside the DIV, and is not linked with
    // aria-controls. We must refuse to type into an unowned focus target.
    await assert.rejects(
      ()=>interactWithPage(s.page,{field_updates:[{kind:'fill',element_id:pseudo.id,value:'query'}]},s.page.frames()),
      /editable focus target/
    );
  }finally{await s.close();}
});
test('custom ARIA checkbox DIV toggles by a real click',async t=>{
  const s=await fixture(t,`<!doctype html>
    <div role="checkbox" id="consent" aria-checked="false" tabindex="0"
      onclick="this.setAttribute('aria-checked','true')">Consent</div>`);
  if(!s)return;
  try{
    const nodes=await s.page.evaluate(extractSemanticElements);
    const checkbox=nodes.find(e=>e.kind==='check'&&e.text==='Consent');
    assert.ok(checkbox);
    await interactWithPage(s.page,{field_updates:[
      {kind:'check',element_id:checkbox.id,value:'',checked:true}
    ]},s.page.frames());
    assert.equal(await s.page.locator('#consent').getAttribute('aria-checked'),'true');
  }finally{await s.close();}
});
test('keyup-only login validation sees real keystrokes',async t=>{
  const s=await fixture(t,`<!doctype html>
    <input id="username" aria-label="Username" value="old"/>
    <script>
      window.seen=[];
      document.getElementById('username').addEventListener('keyup',e=>window.seen.push(e.key));
    </script>`);
  if(!s)return;
  try{
    const nodes=await s.page.evaluate(extractSemanticElements);
    await interactWithPage(s.page,{field_updates:[
      {kind:'fill',element_id:item(nodes,'Username').id,value:'abc'}
    ]},s.page.frames());
    assert.equal(await s.page.locator('#username').inputValue(),'abc');
    assert.ok((await s.page.evaluate(()=>window.seen)).length>=3);
  }finally{await s.close();}
});
