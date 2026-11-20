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
    for (const node of nodes) {
      assert.notEqual(node, undefined, 'Cannot append an uninitialized toolbar button');
      if (node.parentElement) node.parentElement.children = node.parentElement.children.filter(child => child !== node);
      node.parentElement = this;
      this.children.push(node);
    }
  }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  addEventListener() {}
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
const context = {
  TaskMenuApp: {
    jsonFetch() { throw new Error('Bootstrap must not call the provider or other APIs'); },
  },
  document,
  Event: class Event { constructor(type) { this.type = type; } },
  window: { innerWidth: 1200, innerHeight: 800, addEventListener() {}, dispatchEvent() {} },
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
for (const label of ['AI Agent Chat', '🐞', 'Bottom', 'Auto Hide', '⋯', 'Hide', '+ New session', 'Preview context',
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
