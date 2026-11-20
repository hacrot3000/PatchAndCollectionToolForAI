const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

// Exercise the production filtering function without requiring a browser DOM.
const source = fs.readFileSync(path.join(__dirname, 'web/featuremods/directorycompare.js'), 'utf8');
const start = source.indexOf('function filterDirectoryCompareRows(');
const end = source.indexOf('\nfunction visibleRows(){', start);
assert.ok(start >= 0 && end > start, 'production Directory Compare filter must exist');
const filterRows = new Function(source.slice(start, end) + '\nreturn filterDirectoryCompareRows;')();

const dir = (path, state, left = true, right = true) => ({
  path, kind: 'dir', state,
  left: left ? { kind: 'dir' } : null,
  right: right ? { kind: 'dir' } : null
});
const file = (path, state, left = true, right = true) => ({
  path, kind: 'file', state,
  left: left ? { kind: 'file' } : null,
  right: right ? { kind: 'file' } : null
});
const rows = [
  dir('A', 'changed'), file('A/X.txt', 'changed'),
  dir('B', 'left-only', true, false), file('B/Y.txt', 'left-only', true, false),
  dir('C', 'changed'), file('C/Z.txt', 'important'),
  dir('EmptyOnly', 'right-only', false, true),
  dir('Deep', 'changed'), dir('Deep/Sub', 'changed'), file('Deep/Sub/C.txt', 'changed'),
  dir('Same', 'same'), file('Same/Plain.txt', 'same'),
  dir('Unknown', 'unknown'), file('Unknown/U.txt', 'unknown'),
  dir('Mismatch', 'changed'),
  { path: 'Mismatch/Switch', state: 'type-mismatch', kind: 'dir', left: {kind: 'dir'}, right: {kind: 'file'} },
  dir('Right', 'right-only', false, true), file('Right/new.txt', 'right-only', false, true)
];
function names(status = 'all', query = '', includeIdentical = true, collapsed = []) {
  return filterRows(rows, status, query, includeIdentical, new Set(collapsed)).map(row => row.path);
}

test('Checksum/size changed shows A/X and hides one-sided B/Y and folders without matching files', () => {
  assert.deepEqual(names('changed'), ['A', 'A/X.txt', 'Deep', 'Deep/Sub', 'Deep/Sub/C.txt']);
});
test('All includes files and their parent folders even if present on one side only', () => {
  assert.deepEqual(names(), rows.filter(row => row.path !== 'EmptyOnly').map(row => row.path));
});
test('Left-only and right-only preserve only ancestors of matching file differences', () => {
  assert.deepEqual(names('left-only'), ['B', 'B/Y.txt']);
  assert.deepEqual(names('right-only'), ['Right', 'Right/new.txt']);
});
test('Status propagated to directory does not keep a folder if children fail filter', () => {
  assert.deepEqual(names('important'), ['C', 'C/Z.txt']);
  assert.equal(names('changed').includes('C'), false);
});
test('Nested ancestors retained with search, and collapsing a folder hides children', () => {
  assert.deepEqual(names('changed', 'Deep/Sub'), ['Deep', 'Deep/Sub', 'Deep/Sub/C.txt']);
  assert.deepEqual(names('changed', '', true, ['Deep']), ['A', 'A/X.txt', 'Deep']);
});
test('Type mismatch with a file on one side retains its parent', () => {
  assert.deepEqual(names('type-mismatch'), ['Mismatch', 'Mismatch/Switch']);
});
test('Hide identical preserves relevant changed files while dropping same/unchecked ones', () => {
  assert.deepEqual(names('all', '', false), [
    'A', 'A/X.txt', 'B', 'B/Y.txt', 'C', 'C/Z.txt',
    'Deep', 'Deep/Sub', 'Deep/Sub/C.txt',
    'Mismatch', 'Mismatch/Switch', 'Right', 'Right/new.txt'
  ]);
});
test('No file matches means no empty folder rows, regardless of folder status', () => {
  assert.deepEqual(names('changed', 'Nonexistent.txt'), []);
});
