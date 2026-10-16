import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const launcher=fileURLToPath(new URL('./start-configured.mjs',import.meta.url));
test('configured worker launcher has valid Node ESM syntax',()=>{
 const p=spawnSync(process.execPath,['--check',launcher],{encoding:'utf8'});
 assert.equal(p.status,0,p.stderr);
});
test('configured worker refuses public config permissions',t=>{
 if(process.platform==='win32'){t.skip('POSIX permission contract');return}
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'taskdeck-semantic-'));
 t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
 const config=path.join(dir,'worker.json');
 const secret='s'.repeat(64);
 fs.writeFileSync(config,JSON.stringify({socket:path.join(dir,'worker.sock'),token:secret,allowed_hosts:['example.com']}),{mode:0o644});
 const p=spawnSync(process.execPath,[launcher,config],{encoding:'utf8',timeout:5000});
 assert.notEqual(p.status,0);
 assert.match(p.stderr,/private regular file/);
 assert.ok(!p.stderr.includes(secret),'worker secret appeared in logs');
});
test('launcher never hardcodes or prints the worker token',()=>{
 const src=fs.readFileSync(launcher,'utf8');
 assert.match(src,/TASKDECK_BROWSER_WORKER_TOKEN=data\.token/);
 assert.ok(!src.includes('console.log(data.token)'));
 assert.ok(!src.includes('process.stdout.write(data.token)'));
});

test('missing worker.json has actionable wizard instructions without ENOENT stack',t=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'taskdeck-browser-missing-'));
 t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
 const file=path.join(dir,'missing-worker.json');
 const proc=spawnSync(process.execPath,[launcher,file],{encoding:'utf8',timeout:5000});
 assert.notEqual(proc.status,0);
 assert.match(proc.stderr,/Setup wizard/);
 assert.match(proc.stderr,/worker\.json/);
 assert.doesNotMatch(proc.stderr,/node:fs:|ENOENT:/);
});
