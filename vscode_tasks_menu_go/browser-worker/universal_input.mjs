// Universal input adapter. Runs only in the Chromium worker, on the REAL
// website. Never inject event-dispatching code into TaskDeck's frontend.
const idPattern=/^td-sem-[0-9a-z]+$/;
function invalid(message,status=400) {const e=new Error(message);e.status=status;throw e;}
function locatorFor(page,id,frameIndex=0,frameRefs=page.frames()) {
  if(!idPattern.test(String(id||'')))invalid('Invalid semantic control ID');
  if(!Number.isSafeInteger(frameIndex)||frameIndex<0||frameIndex>7)invalid('Invalid browser frame index');
  const frame=frameRefs[frameIndex];
  // Reject changing iframe order/identity after a snapshot, preventing a stale
  // button in an old frame from targeting an unrelated newly inserted iframe.
  if(!frame||!page.frames().includes(frame))invalid('Frame changed; refresh the page',409);
  return frame.locator('[data-taskdeck-semantic-id="'+id+'"]');
}
async function uniqueControl(page,id,frameIndex=0,frameRefs=page.frames()) {
  const loc=locatorFor(page,id,frameIndex,frameRefs);
  if(await loc.count()!==1)invalid('Control changed or no longer exists; refresh the page',409);
  if(!await loc.isVisible())invalid('Control is not visible anymore; refresh the page',409);
  return loc;
}
async function editableFallback(page,loc,value) {
  // Custom wrappers can relay keyboard input to a nested or hidden textarea
  // (Monaco, CodeMirror). Do not press keys if focus escaped the clicked widget:
  // Ctrl+A could otherwise select the entire document.
  const nested=loc.locator('input:not([type="hidden"]),textarea,[contenteditable="true"],[contenteditable="plaintext-only"]');
  let target=loc;
  for(let i=0,n=Math.min(await nested.count(),12);i<n;i++) {
    const item=nested.nth(i);
    if(await item.isVisible()){target=item;break;}
  }
  await target.click({timeout:6000});
  const focus=await loc.evaluate(el=>{
    const doc=el.ownerDocument,active=doc.activeElement;
    const inside=el===active||el.contains(active)||Boolean(el.shadowRoot?.contains(active));
    const editable=Boolean(active?.isContentEditable||['INPUT','TEXTAREA'].includes(active?.tagName));
    return {inside,editable};
  });
  if(!focus.inside||!focus.editable)invalid('This custom textbox did not expose an editable focus target',422);
  await page.keyboard.press('ControlOrMeta+A');
  if(value.length>1600)await page.keyboard.insertText(value);
  else await page.keyboard.type(value,{delay:0});
}
export async function fillControl(page,loc,value) {
  value=String(value??'');
  if(value.length>32768)invalid('Input exceeds maximum length',413);
  const info=await loc.evaluate(el=>({
    native:['INPUT','TEXTAREA'].includes(el.tagName),
    contenteditable:el.isContentEditable,
    sensitive:el.getAttribute('type')==='password'||el.getAttribute('autocomplete')==='current-password'
  }));
  if(info.native||info.contenteditable){
    // fill fires the same input notification React and typical JS form
    // validators listen to; do NOT set element.value in page.evaluate().
    await loc.fill(value,{timeout:7000});
    return;
  }
  // Some framework textboxes use role=textbox on an outer noneditable DIV,
  // while focus is actually transferred to an inner editor/input.
  await editableFallback(page,loc,value);
}
export async function applyFieldChanges(page,changes,frameRefs=page.frames()) {
  if(!Array.isArray(changes)||changes.length>64)invalid('Too many fields in one interaction');
  // Prevalidate all IDs and types before making the first edit.
  const planned=[];
  const used=new Set();
  for(const item of changes) {
    if(!item||typeof item!=='object')invalid('Invalid field update');
    const key=String(item.frame_index||0)+'/'+String(item.element_id||'');
    if(used.has(key))invalid('Duplicate field update');
    used.add(key);
    if(!['fill','check','select'].includes(item.kind))invalid('Unsupported field update');
    if(typeof item.value!=='string'||item.value.length>32768)invalid('Invalid field value');
    const loc=await uniqueControl(page,item.element_id,item.frame_index||0,frameRefs);
    planned.push({kind:item.kind,value:item.value,loc,checked:item.checked===true});
  }
  for(const item of planned) {
    if(item.kind==='fill')await fillControl(page,item.loc,item.value);
    else if(item.kind==='check')await item.loc.setChecked(item.checked,{timeout:7000});
    else await item.loc.selectOption(item.value,{timeout:7000});
  }
}
// A single serialized transaction performs all pending changes, THEN clicks
// Login/Submit or presses Enter. This eliminates blur/change versus click races.
export async function interactWithPage(page,input,frameRefs=page.frames()) {
  const changes=input.field_updates||[];
  const targetAction=String(input.target_action||'');
  if(!['','click','press','check','select','scroll'].includes(targetAction))invalid('Unsupported target action');
  const target=targetAction?await uniqueControl(page,input.element_id,input.frame_index||0,frameRefs):null;
  // If a framework rerenders the submit button when a field changes, the
  // existing ID is no longer valid. Reject rather than click the wrong button.
  await applyFieldChanges(page,changes,frameRefs);
  if(!target)return;
  if(await target.count()!==1||!await target.isVisible())invalid('Target changed while editing; retry after refresh',409);
  switch(targetAction) {
    case 'click':await target.click({timeout:7000});break;
    case 'press':
      if(!/^(Enter|Tab|Escape|Arrow(Up|Down|Left|Right))$/.test(String(input.key||'')))invalid('Unsupported key');
      await target.press(input.key,{timeout:7000});break;
    case 'check':await target.setChecked(input.checked===true,{timeout:7000});break;
    case 'select':await target.selectOption(String(input.value||''),{timeout:7000});break;
    case 'scroll':await target.scrollIntoViewIfNeeded({timeout:7000});break;
  }
}
