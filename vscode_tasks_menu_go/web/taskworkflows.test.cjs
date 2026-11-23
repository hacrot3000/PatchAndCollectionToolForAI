const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const root = path.resolve(__dirname, 'featuremods');
const workflows = fs.readFileSync(path.join(root, 'taskworkflows.js'), 'utf8');
const menus = fs.readFileSync(path.join(root, 'menus.js'), 'utf8');

for (const contract of [
  '/api/agent/task-manifests/validate',
  '/api/agent/task-manifests/runs',
  '/api/agent/task-manifests/runs/providers',
  '/api/agent/context-groups',
  'context_group_id',
  'confirm_external:true',
  'waiting_review',
  'concurrency_slot',
  'dependency',
  'confirm_no_side_effect:true',
  'Stop workflow',
  'Approve task / unlock dependents',
  'Reject task',
]) assert.ok(workflows.includes(contract), `workflow UI missing ${contract}`);
assert.ok(menus.includes('Task Manifest Run All'), 'menu entry missing for workflow UI');
assert.ok(menus.includes("import('/featuremods/taskworkflows.js')"), 'workflow UI is not loaded lazily from its own module');
console.log('task workflow UI contract tests passed');
