import test from 'node:test';
import assert from 'node:assert/strict';
import {chromium} from 'playwright';
import {extractSemanticElements,installEventInstrumentation} from './semantic_extract.mjs';

test('detect div onclick, addEventListener, CSS pointer and React delegated click; use real pointer events',async t=>{
  let browser;
  try{browser=await chromium.launch({headless:true});}
  catch(error){t.skip('Chromium unavailable: run npx playwright install chromium first');return;}
  try{
    const context=await browser.newContext();
    await context.addInitScript(installEventInstrumentation);
    const page=await context.newPage();
    await page.goto('data:text/html,'+encodeURIComponent(`<!doctype html>
      <style>#css{cursor:pointer}</style>
      <div id="inline" onclick="window.hits.inline++">Inline Save</div>
      <div id="listener">Listener Save</div>
      <div id="css">CSS Action</div>
      <div id="react">Synthetic Action</div>
      <button id="native">Native Save</button>
      <input type="password" id="secret" value="SHOULD_NOT_LEAK" />
      <script>
      window.hits={inline:0,listener:0,css:0,react:0};
      document.getElementById('listener').addEventListener('click',()=>window.hits.listener++);
      document.getElementById('css').addEventListener('click',()=>window.hits.css++);
      const react=document.getElementById('react');
      react['__reactProps$test']={onClick:()=>window.hits.react++};
      document.addEventListener('click',e=>{if(e.target===react)react['__reactProps$test'].onClick();});
      </script>`));
    const elements=await page.evaluate(extractSemanticElements,1);
    for(const name of ['Inline Save','Listener Save','CSS Action','Synthetic Action','Native Save']){
      const item=elements.find(e=>e.text===name&&e.kind==='action');
      assert.ok(item,'not recognized: '+name);
      await page.locator('[data-taskdeck-semantic-id="'+item.id+'"]').click();
    }
    const hits=await page.evaluate(()=>window.hits);
    assert.deepEqual(hits,{inline:1,listener:1,css:1,react:1});
    const password=elements.find(e=>e.input_type==='password');
    assert.equal(password.value,'','password value must never leave the worker');
    assert.ok(!JSON.stringify(elements).includes('SHOULD_NOT_LEAK'));
    await context.close();
  }finally{await browser.close();}
});

test('extractor works with dynamically added click targets',async t=>{
  let browser;
  try{browser=await chromium.launch({headless:true});}
  catch{t.skip('Chromium not installed');return;}
  try{
    const context=await browser.newContext();
    await context.addInitScript(installEventInstrumentation);
    const page=await context.newPage();
    await page.goto('data:text/html,'+encodeURIComponent('<body><button id="add">Add</button></body>'));
    await page.evaluate(()=>{
      document.getElementById('add').addEventListener('click',()=>{
        const node=document.createElement('div');
        node.textContent='Async div button';
        node.addEventListener('click',()=>document.body.dataset.hit='yes');
        document.body.append(node);
      });
    });
    await page.locator('#add').click();
    const elements=await page.evaluate(extractSemanticElements,2);
    const dynamic=elements.find(e=>e.text==='Async div button'&&e.kind==='action');
    assert.ok(dynamic,'dynamically registered listener was not discovered');
    await page.locator('[data-taskdeck-semantic-id="'+dynamic.id+'"]').click();
    assert.equal(await page.locator('body').getAttribute('data-hit'),'yes');
    await context.close();
  }finally{await browser.close();}
});
