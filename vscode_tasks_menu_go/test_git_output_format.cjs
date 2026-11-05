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


test('Keybase backspace redraws retain only the final fractional-percentage frame',()=>{
  const label='Preparing and encrypting: ';
  const frames=[
    '(0.00%) 0/6 objects... ',
    '(16.67%) 1/6 objects... ',
    '(33.33%) 2/6 objects... ',
    '(100.00%) 6/6 objects... done.'
  ];
  let raw=label+frames[0];
  for(let i=1;i<frames.length;i++)raw+='\b'.repeat(frames[i-1].length)+frames[i];
  raw+='\nIndexing hashes: (0.00%) 0/6 objects... \rIndexing hashes: (100.00%) 6/6 objects... done.\n';
  assert.equal(format(raw),
    'Preparing and encrypting: (100.00%) 6/6 objects... done.\n'+
    'Indexing hashes: (100.00%) 6/6 objects... done.\n');
  assert.doesNotMatch(format(raw),/[\x00-\x08\x0b-\x1f\x7f]/);
});

test('Keybase progress with independent completed transfers and final ref is preserved',()=>{
  const lines=[
    'Initializing Keybase... done.',
    'Syncing with Keybase... done.',
    'Indexing CRCs: (0.00%) 0/6 objects...',
    'Indexing CRCs: (83.33%) 5/6 objects...',
    'Indexing CRCs: (100.00%) 6/6 objects... done.',
    'Syncing encrypted data to Keybase: (0.00%) 0.00/102.89 KB...',
    'Syncing encrypted data to Keybase: (100.00%) 102.89/102.89 KB... done.',
    'Syncing encrypted data to Keybase: (0.00%) 0.00/10.67 KB...',
    'Syncing encrypted data to Keybase: (100.00%) 10.67/10.67 KB... done.',
    'To keybase://team/568studio.vuahaitac/battle',
    '  57ca481..d39876e  dev -> dev',
    ''
  ];
  const output=format(lines.join('\n'));
  assert.equal(output,[
    lines[0],lines[1],lines[4],lines[6],lines[8],lines[9],lines[10],''
  ].join('\n'));
});

test('shorter overwritten CR frames honor erase-to-end, and backspaces do not print',()=>{
  assert.equal(format('Working 100000%\rDone\x1b[K\n'), 'Done\n');
  assert.equal(format('Error: oops\x1b[2K\rReady\n'),'Ready\n');
  assert.equal(format('abcde\x1b[3DXYZ\n'),'abXYZ\n');
  assert.equal(format('echo \x1b]0;title\x07ok\n'),'echo ok\n');
  assert.equal(format('aaaa\b\bZ\n'),'aaaZa\n');
});

test('warnings and errors between progress stages are never swallowed',()=>{
  const raw='Uploading: (0.00%) 0/2...\nUploading: (50.00%) 1/2...\n'+
    'warning: keybase temporary retry\nUploading: (100.00%) 2/2... done.\n'+
    'fatal: simulated failure\n';
  assert.equal(format(raw),
    'Uploading: (50.00%) 1/2...\nwarning: keybase temporary retry\n'+
    'Uploading: (100.00%) 2/2... done.\nfatal: simulated failure\n');
});
