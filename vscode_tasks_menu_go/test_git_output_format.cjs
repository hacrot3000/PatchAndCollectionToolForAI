const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const test=require('node:test');
const vm=require('node:vm');

const source=fs.readFileSync(path.join(__dirname,'web/featuremods/gitoutputformat.js'),'utf8');
const sandbox={globalThis:{}};
vm.runInNewContext(source,sandbox,{timeout:1000,filename:'gitoutputformat.js'});
const format=sandbox.globalThis.TaskMenuGitOutputFormat.format;

test('carriage-return progress redraws in place and keeps final push summary',()=>{
  const raw=[
    'Enumerating objects: 63, done.\n',
    'Counting objects:   1% (1/63)\r',
    'Counting objects:  50% (32/63)\r',
    'Counting objects: 100% (63/63), done.\n',
    'Delta compression using up to 16 threads\n',
    'Writing objects:   4% (1/23)\r',
    'Writing objects: 100% (23/23), 4.61 KiB | 1.15 MiB/s, done.\n',
    'To https://github.com/example/repo.git\n',
    '  e47b4e1d..a7c51c64 feature/testing -> feature/testing\n'
  ].join('');
  assert.equal(format(raw),[
    'Enumerating objects: 63, done.',
    'Counting objects: 100% (63/63), done.',
    'Delta compression using up to 16 threads',
    'Writing objects: 100% (23/23), 4.61 KiB | 1.15 MiB/s, done.',
    'To https://github.com/example/repo.git',
    '  e47b4e1d..a7c51c64 feature/testing -> feature/testing',''
  ].join('\n'));
});

test('newline-expanded progress compacts only the same adjacent stage',()=>{
  const raw='Counting objects: 1% (1/3)\nCounting objects: 66% (2/3)\n'+
    'Counting objects: 100% (3/3), done.\nremote: Resolving deltas: 0% (0/4)\n'+
    'remote: Resolving deltas: 100% (4/4), completed\n';
  assert.equal(format(raw),'Counting objects: 100% (3/3), done.\nremote: Resolving deltas: 100% (4/4), completed\n');
});

test('CRLF boundaries, repeated errors and warning lines stay intact',()=>{
  assert.equal(format('warning: test\r\nwarning: test\r\nfatal: broken pipe\r\n'),
    'warning: test\nwarning: test\nfatal: broken pipe\n');
});

test('ANSI CSI styling is removed but status and Unicode are preserved',()=>{
  assert.equal(format('\x1b[31mfatal: mạng không ổn định\x1b[0m\n'),
    'fatal: mạng không ổn định\n');
});

test('single current progress row remains visible while operation runs',()=>{
  assert.equal(format('RUNNING · elapsed 2s\nCounting objects: 1% (1/63)\rCounting objects: 6% (4/63)'),
    'RUNNING · elapsed 2s\nCounting objects: 6% (4/63)');
});
