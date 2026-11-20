// Run with: node vscode_tasks_menu_go/web/agentchat_bootstrap_test.cjs
// Execute the actual Agent Chat module in a minimal DOM, catching TDZ/runtime
// errors that node --check (syntax only) and static Go string tests cannot catch.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'featuremods/agentchat.js'), 'utf8');
class FakeElement {
  constructor(tag) {
    this.tagName = tag;
    this.children = [];
    this.attributes = {};
    this.className = '';
    this.textContent = '';
    this.type = '';
    this.value = '';
    this.disabled = false;
    this.style = { setProperty() {} };
    this.dataset = {};
    this.clientWidth = 0;
    this.offsetWidth = 40;
    this.listeners = {};
    this.appendCount = 0;
    this.replaceCount = 0;
    this.removeCount = 0;
    const classes = new Set();
    this.classList = {
      add: (...names) => names.forEach(name => classes.add(name)),
      remove: (...names) => names.forEach(name => classes.delete(name)),
      contains: name => classes.has(name),
      toggle: (name, force) => {
        const add = force === undefined ? !classes.has(name) : Boolean(force);
        if (add) classes.add(name); else classes.delete(name);
        return add;
      },
    };
  }
  append(...nodes) {
    this.appendCount += nodes.length;
    for (const node of nodes) {
      assert.notEqual(node, undefined, 'Cannot append an uninitialized toolbar button');
      if (node.parentElement) node.parentElement.children = node.parentElement.children.filter(child => child !== node);
      node.parentElement = this;
      this.children.push(node);
    }
  }
  replaceChildren(...nodes) { this.replaceCount++; for (const child of this.children) child.parentElement=null; this.children = []; this.append(...nodes); }
  remove() {
    const p=this.parentElement;
    if (!p) return;
    const idx=p.children.indexOf(this);
    if(idx>=0){p.children.splice(idx,1);p.removeCount++;}
    this.parentElement=null;
  }
  replaceWith(node) {
    const p=this.parentElement;
    assert.ok(p,'Cannot replace a detached element');
    const idx=p.children.indexOf(this);
    assert.ok(idx>=0,'Cannot find element in parent');
    p.children[idx]=node;node.parentElement=p;this.parentElement=null;p.replaceCount++;
  }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  addEventListener(type, listener) { (this.listeners[type] ||= []).push(listener); }
  querySelector(selector) { return selector === 'button' ? this.children.find(child => child.tagName === 'button') || null : null; }
  focus() {}
  click() {
    const event = { target: this, stopPropagation() {} };
    this.onclick?.(event);
    for (const listener of this.listeners.click || []) listener(event);
  }
}
const section = new FakeElement('section');
const tabs = new FakeElement('div');
const panes = new FakeElement('div');
const document = {
  head: new FakeElement('head'),
  body: new FakeElement('body'),
  createElement: tag => new FakeElement(tag),
  querySelector: selector => selector === 'main>section' ? section : selector === '#tabs' ? tabs : selector === '#panes' ? panes : null,
};
let resizeObserver;
const context = {
  TaskMenuApp: {
    jsonFetch() { throw new Error('Bootstrap must not call the provider or other APIs'); },
  },
  document,
  Event: class Event { constructor(type) { this.type = type; } },
  ResizeObserver: class ResizeObserver {
    constructor(callback) { this.callback = callback; resizeObserver = this; }
    observe() {}
  },
  window: { innerWidth: 1200, innerHeight: 800, addEventListener() {}, dispatchEvent() {} },
  setTimeout() { return 1; },
  clearTimeout() {},
};
assert.doesNotThrow(
  () => vm.runInNewContext(source, context, { filename: 'agentchat.js', timeout: 2000 }),
  'Agent Chat must load without referencing lexical variables before initialization'
);
assert.equal(typeof context.TaskMenuAgentChat?.open, 'function', 'Agent Chat open action registered');
assert.equal(typeof context.TaskMenuAgentChat?.settings, 'function', 'provider settings action registered');
const root = section.children.find(child => child.className === 'agentchat-overlay');
const [settingsOverlay] = document.body.children;
assert.ok(root && settingsOverlay, 'Agent chat and provider settings DOM created');
const [windowBox] = root.children;
const [heading, viewTabs, , debugPanel] = windowBox.children;
assert.equal(windowBox.attributes.role, 'region', 'Main Agent Chat remains non-modal');
assert.equal(windowBox.attributes['aria-modal'], undefined, 'Main Agent Chat must not create a modal backdrop');
assert.ok(viewTabs.children.some(child => child.textContent === 'AI Debug'), 'AI Debug view tab missing');
assert.equal(debugPanel.className, 'agentchat-debug-panel');
const debugButtons = (() => {const scan = node => [ ...(node.tagName==='button'?[node]:[]),...node.children.flatMap(scan) ];return scan(debugPanel);})();
assert.equal(debugButtons.length,1,'AI Debug must expose exactly one action');
assert.equal(debugButtons[0].textContent,'Copy log','one action must be Copy log');
const find = (node, predicate) => predicate(node) ? node : node.children.map(child => find(child, predicate)).find(Boolean);
const sessionsButton = find(windowBox, node => node.textContent === 'Saved conversations ▾');
const sessionsList = find(windowBox, node => node.id === 'agentchat-saved-sessions');
assert.ok(sessionsButton && sessionsList, 'Saved conversations menu is present');
assert.equal(sessionsButton.parentElement.parentElement, viewTabs, 'Saved conversations shares the conversation/debug tab row');
assert.equal(sessionsList.classList.contains('open'), false, 'Saved conversations start closed');
sessionsButton.onclick();
assert.equal(sessionsList.classList.contains('open'), true, 'Saved conversations open on click');
assert.equal(sessionsButton.attributes['aria-expanded'], 'true');
sessionsButton.onclick();
assert.equal(sessionsList.classList.contains('open'), false, 'Saved conversations close on second click');
assert.equal(sessionsButton.attributes['aria-expanded'], 'false');
const collectLabels = node => [node.textContent, ...node.children.flatMap(collectLabels)];
const labels = collectLabels(heading);
const moreButton = heading.children.find(child => child.textContent === '⋯');
const moreMenu = heading.children.find(child => child.className === 'agentchat-more-menu');
const toolbarActions = heading.children.find(child => child.className === 'agentchat-toolbar-actions');
section.clientWidth = 1000; section.clientHeight = 700; heading.clientWidth = 800;
resizeObserver.callback();
const countAfterResize = toolbarActions.appendCount;
for (let i = 0; i < 50; i++) resizeObserver.callback();
assert.equal(toolbarActions.appendCount, countAfterResize, 'Repeated identical resize notifications must not rearrange actions');
moreButton.click();
assert.equal(moreMenu.classList.contains('open'), true, 'One click opens More actions');
assert.equal(moreButton.attributes['aria-expanded'], 'true');
moreButton.click();
assert.equal(moreMenu.classList.contains('open'), false, 'Second click closes More actions');
assert.equal(moreButton.attributes['aria-expanded'], 'false');
for (const label of ['AI Agent Chat', 'Bottom', 'Auto Hide', '⋯', 'Hide', '+ New session', 'Preview context',
  'Rename', 'Archive', 'Providers / API keys', 'Refresh', '+ Text file', 'Auto-send results']) {
  assert.ok(labels.includes(label), 'Agent Chat control missing at runtime: ' + label);
}
for (const label of ['Bottom', 'Auto Hide', '⋯', 'Hide', '+ New session', 'Preview context',
  'Rename', 'Archive', 'Providers / API keys', 'Refresh', '+ Text file', 'Auto-send results']) {
  assert.equal(labels.filter(value => value === label).length, 1, 'Control duplicated: ' + label);
}
assert.equal(context.TaskMenuAgentChat.dockPosition, 'right');
context.TaskMenuAgentChat.setDockPosition('bottom');
assert.equal(context.TaskMenuAgentChat.dockPosition, 'bottom');
assert.ok(section.classList.contains('agent-dock-bottom'), 'Bottom dock layout applied');
context.TaskMenuAgentChat.setAutoHide(true);
assert.equal(context.TaskMenuAgentChat.autoHide, true);
console.log('PASS: Agent Chat bootstraps and dock position/auto-hide state transitions work');

(async () => {
  let saved;
  context.window.confirm = () => true;
  context.TaskMenuApp.jsonFetch = async (url, options) => {
    assert.equal(url, '/api/agent/providers');
    saved = JSON.parse(options.body);
    return { provider: saved.provider };
  };
  vm.runInNewContext(`
    agentProviders=[{id:'p',name:'Provider',api_type:'responses',base_url:'https://example.com/v1',models:[]}];
    settingsPicker.value='p';
    providerEditor.value=JSON.stringify({...agentProviders[0], name:'Unsaved edit'});
  `, context);
  await vm.runInNewContext("allowAgentProviderIP('p','https://example.com/v1','100.64.0.19')", context);
  assert.deepEqual(saved.provider.trusted_shared_ips, ['100.64.0.19']);
  assert.equal(saved.provider.name, 'Provider', 'Allowing an IP does not persist unrelated unsaved edits');
  assert.equal(vm.runInNewContext('JSON.parse(providerEditor.value).name', context), 'Unsaved edit');
  assert.equal(vm.runInNewContext('JSON.parse(providerEditor.value).trusted_shared_ips[0]', context), '100.64.0.19');
  console.log('PASS: Allow this IP saves only the trusted address and preserves editor drafts');

  // Native status polling replaces transcript DOM. The expanded code review
  // and independent console window must remain open across that refresh.
  const patchSession = {
    id:'test-session',status:'waiting_tool',revision:9,
    history:[
      {type:'user',content:'tạo file helloworld.php trên thư mục gốc'},
      {type:'function_call',call_id:'read-1',name:'taskdeck_collect_workspace',arguments:'{"actions":[{"type":"ls","path":"."}]}'},
      {type:'function_call_output',call_id:'read-1',content:'{"results":[{"entries":["helloworld.php"]}]}'},
      {type:'function_call',call_id:'patch-1',name:'taskdeck_submit_patch',arguments:'{"name":"Create root helloworld.php","patch_python":"print(1)"}'}
    ],
    tool_calls:[{call_id:'patch-1',name:'taskdeck_submit_patch',status:'running',patch_session_id:'native-1'}]
  };
  vm.runInNewContext('displayAgentSession('+JSON.stringify(patchSession)+')',context);
  const patchDetail=find(windowBox,node=>node.dataset?.agentDetailKey==='test-session:patch-1:patch-code');
  assert.ok(patchDetail,'Patch code review must be present');
  patchDetail.open=true;
  patchDetail.listeners.toggle[0]();
  vm.runInNewContext('displayAgentSession('+JSON.stringify(patchSession)+')',context);
  const reopened=find(windowBox,node=>node.dataset?.agentDetailKey==='test-session:patch-1:patch-code');
  assert.equal(reopened.open,true,'Patch review must stay expanded during native status refresh');
  const collectLabel=find(windowBox,node=>node.textContent==='Collection · inspected');
  assert.ok(collectLabel,'Collection must not be displayed as BLOCKED after successful read');
  assert.equal(Boolean(find(windowBox,node=>node.textContent==='Unrecognized function; execution blocked.')),false);
  context.TaskMenuApp.jsonFetch=async(url)=>{
    assert.ok(url.endsWith('/tools/patch-1/execution'),'wrong console endpoint');
    return {execution:{status:'running',stdout_tail:'Patch output remains readable'}};
  };
  vm.runInNewContext("agentActive='test-session'",context);
  await vm.runInNewContext("showAgentExecution('test-session','patch-1')",context);
  const consoleView=find(windowBox,node=>node.className==='agentchat-execution-panel');
  assert.ok(consoleView?.classList.contains('visible'),'View console must open a persistent panel');
  vm.runInNewContext('displayAgentSession('+JSON.stringify({...patchSession,revision:10})+')',context);
  assert.equal(consoleView.classList.contains('visible'),true,'Native status polling must not close the console');
  const consoleText=find(windowBox,node=>node.className==='agentchat-execution-content');
  assert.ok(consoleText.textContent.includes('Patch output remains readable'));
  console.log('PASS: Collection presentation, stable Patch review and persistent native console across refresh');

  // A sequence of identical server snapshots (including revision-only
  // changes) must not rebuild a single bubble, tab, code review or console.
  const transcript=find(windowBox,node=>node.className==='agentchat-transcript');
  const originalUser=transcript.children[0];
  const originalPatch=find(windowBox,node=>node.dataset?.agentDetailKey==='test-session:patch-1:patch-code');
  const baselineReplace=transcript.replaceCount;
  const baselineAppend=transcript.appendCount;
  const baselineNodes=[...transcript.children];
  for(let i=0;i<60;i++){
    vm.runInNewContext('displayAgentSession('+JSON.stringify({...patchSession,revision:100+i})+')',context);
  }
  assert.equal(transcript.replaceCount,baselineReplace,'unchanged snapshots must never clear transcript');
  assert.equal(transcript.appendCount,baselineAppend,'unchanged snapshots must not append duplicate rows');
  assert.deepEqual(transcript.children,baselineNodes,'every existing bubble must retain DOM identity');
  assert.equal(find(windowBox,node=>node.dataset?.agentDetailKey==='test-session:patch-1:patch-code'),originalPatch,
    'expanded code review node must survive all unchanged polls');

  // One new assistant message should append exactly one node and preserve all
  // previous nodes and their state. A native Patch status updates only its card.
  const extended={...patchSession,revision:200,history:[
    ...patchSession.history,{type:'assistant',content:'helloworld.php created'}
  ]};
  vm.runInNewContext('displayAgentSession('+JSON.stringify(extended)+')',context);
  assert.equal(transcript.replaceCount,baselineReplace,'new message must not recreate transcript');
  assert.equal(transcript.appendCount,baselineAppend+1,'new message must append just one row');
  assert.equal(transcript.children[0],originalUser,'old user message must remain the same element');
  assert.equal(transcript.children.at(-1).textContent,'helloworld.php created');
  const patchDone={...extended,revision:201,tool_calls:[
    {...patchSession.tool_calls[0],status:'done',result:'{"status":"PASS"}'}
  ]};
  const stableAssistant=transcript.children.at(-1);
  vm.runInNewContext('displayAgentSession('+JSON.stringify(patchDone)+')',context);
  assert.equal(transcript.children[0],originalUser,'native status must not repaint old user messages');
  assert.equal(transcript.children.at(-1),stableAssistant,'native status must not repaint assistant answer');
  assert.equal(transcript.replaceCount,baselineReplace+1,'native status must replace exactly its own tool card');
  assert.equal(find(windowBox,node=>node.dataset?.agentDetailKey==='test-session:patch-1:patch-code').open,true,
    'card replacement must preserve expanded code review');

  // An actual change in the last history item should replace only the tail,
  // not any preceding source or tool rows.
  const edited={...patchDone,revision:202,history:[
    ...patchSession.history,{type:'assistant',content:'helloworld.php verified'}
  ]};
  vm.runInNewContext('displayAgentSession('+JSON.stringify(edited)+')',context);
  assert.equal(transcript.children[0],originalUser,'changing last answer must not redraw earlier history');
  assert.equal(transcript.children.at(-1).textContent,'helloworld.php verified');
  assert.equal(transcript.removeCount,1,'only changed tail row should be removed');

  // The tab and conversation list should preserve keyed DOM nodes as their
  // labels or active state change.
  vm.runInNewContext("agentSessions=[{id:'test-session',title:'First',status:'waiting_tool'}];agentActive='test-session';agentTabs=['test-session'];updateAgentSessionRows();renderAgentTabs();",context);
  const tabbar=find(windowBox,node=>node.className==='agentchat-modelbar'&&node.children.some(c=>c.className.startsWith('agentchat-tab')));
  const savedTab=tabbar.children[0],savedListButton=sessionsList.children[0];
  const tabReplace=tabbar.replaceCount, listReplace=sessionsList.replaceCount;
  for(let i=0;i<30;i++){
    vm.runInNewContext("agentSessions[0].status='done';updateAgentSessionRows();renderAgentTabs();",context);
  }
  assert.equal(tabbar.children[0],savedTab,'tab DOM must be retained across status refreshes');
  assert.equal(sessionsList.children[0],savedListButton,'session DOM must be retained across status refreshes');
  assert.equal(tabbar.replaceCount,tabReplace,'tab bar must not be cleared');
  assert.equal(sessionsList.replaceCount,listReplace,'session list must not be cleared');

  // A no-op /sync response must not fetch /sessions/:id, even when the user
  // explicitly checks status. A changed revision needs just one detail fetch.
  let requests=[];
  vm.runInNewContext("agentActive='test-session'",context);
  context.TaskMenuApp.jsonFetch=async url=>{
    requests.push(url);
    if(url.endsWith('/sync'))return {revision:202,tool_calls:patchDone.tool_calls};
    if(url.endsWith('/sessions/test-session'))return {session:edited,patch_tool_status:'enabled'};
    throw new Error('Unexpected API request: '+url);
  };
  await vm.runInNewContext("syncAgentPatch('test-session',{quiet:true})",context);
  assert.deepEqual(requests,['/api/agent/sessions/test-session/sync'],
    'unchanged Patch sync must do only one API call');
  assert.equal(transcript.children[0],originalUser,'no-op sync may not redraw transcript');
  requests=[];
  context.TaskMenuApp.jsonFetch=async url=>{
    requests.push(url);
    if(url.endsWith('/sync'))return {revision:203,tool_calls:patchDone.tool_calls};
    if(url.endsWith('/sessions/test-session'))return {session:{...edited,revision:203},patch_tool_status:'enabled'};
    throw new Error('Unexpected API request: '+url);
  };
  await vm.runInNewContext("syncAgentPatch('test-session',{quiet:true})",context);
  assert.deepEqual(requests,['/api/agent/sessions/test-session/sync','/api/agent/sessions/test-session'],
    'changed sync must fetch exactly one session detail');
  assert.equal(transcript.children[0],originalUser,'changed revision without new content may not repaint');
  console.log('PASS: append-only transcript, single-card status updates and revision-gated polling');


})().catch(error => { console.error(error); process.exitCode = 1; });
