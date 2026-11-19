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
    this.style = {};
    this.dataset = {};
    const classes = new Set();
    this.classList = {
      add: (...names) => names.forEach(name => classes.add(name)),
      remove: (...names) => names.forEach(name => classes.delete(name)),
      contains: name => classes.has(name),
    };
  }
  append(...nodes) {
    for (const node of nodes) {
      assert.notEqual(node, undefined, 'Cannot append an uninitialized toolbar button');
      this.children.push(node);
    }
  }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  addEventListener() {}
}
const document = {
  head: new FakeElement('head'),
  body: new FakeElement('body'),
  createElement: tag => new FakeElement(tag),
};
const context = {
  TaskMenuApp: {
    jsonFetch() { throw new Error('Bootstrap must not call the provider or other APIs'); },
  },
  document,
  window: { addEventListener() {} },
};
assert.doesNotThrow(
  () => vm.runInNewContext(source, context, { filename: 'agentchat.js', timeout: 2000 }),
  'Agent Chat must load without referencing lexical variables before initialization'
);
assert.equal(typeof context.TaskMenuAgentChat?.open, 'function', 'Agent Chat open action registered');
assert.equal(typeof context.TaskMenuAgentChat?.settings, 'function', 'provider settings action registered');
const [root, settingsOverlay] = document.body.children;
assert.ok(root && settingsOverlay, 'Agent chat and provider settings DOM created');
const [windowBox] = root.children;
const [heading] = windowBox.children;
const labels = heading.children.map(child => child.textContent);
for (const label of ['TaskDeck AI Agent Chat', '+ New session', 'Preview context',
  'Rename', 'Archive', 'Providers / API keys', 'Refresh', '✕']) {
  assert.ok(labels.includes(label), 'Toolbar button missing at runtime: ' + label);
}
assert.equal(new Set(labels).size, labels.length, 'Toolbar buttons not duplicated');
console.log('PASS: Agent Chat bootstraps successfully; all toolbar controls are initialized and mounted');
