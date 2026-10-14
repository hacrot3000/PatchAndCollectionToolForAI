const assert=require('node:assert/strict');
const e=require('./web/featuremods/smartterminal_engine.js');
const items=e.rank('sup',{
 current:[{command:'echo supervisor',finishedAt:4}],
 project:[{command:'supervisorctl status',finishedAt:15}],
 history:['supervisorctl restart all'],
 commands:['supervisorctl']});
assert.equal(items[0].command,'supervisorctl status');
assert.equal(items[0].source,'project');
assert.equal(items[1].command,'supervisorctl restart all');
assert.equal(e.rank('ls',{current:[{command:'ls -la',finishedAt:2},{command:'ls -lh',finishedAt:5}]} )[0].command,'ls -lh');
assert.equal(e.suffix('ls',{command:'ls -la'}),' -la');
assert.equal(e.suffix('sup',{command:'supervisorctl status'}),'ervisorctl status');
assert.equal(e.suffix('status',{command:'supervisorctl status'}),'');
assert.equal(e.rank('git p',{current:[],project:[],history:[],commands:[]})[0].command,'git pull');
assert.ok(!e.rank('pass',{current:['password=letmein'],project:[],history:[],commands:[]}).some(x=>x.command.includes('letmein')));
assert.ok(!e.rank('a',{current:['a\nrm -rf /'],project:[],history:[],commands:[]}).some(x=>x.command.includes('\n')));
const duplicate=e.rank('ls',{current:['ls -la'],project:['ls -la'],history:['ls -la']});
assert.equal(duplicate.filter(x=>x.command==='ls -la').length,1);
console.log('Smart terminal ranking PASS');

const unrelated=e.rank('ls',{
 current:[{command:'ga tools',finishedAt:99999},{command:'echo false',finishedAt:99998}],
 project:[{command:'make tools',finishedAt:99997}],
 history:['ls -al','ls -l'],
 commands:['lsof','lsblk']
});
assert.equal(unrelated[0].command,'ls -al','real ls prefix must outrank unrelated recently used commands');
assert.ok(!unrelated.some(x=>x.command==='ga tools'),'ls must not match the suffix inside tools');
assert.ok(!unrelated.some(x=>x.command==='echo false'),'ls must not match arbitrary two-character infixes');
assert.equal(e.relevance('ls','ga tools'),-1);
assert.ok(e.relevance('ls','echo ls -lah')>0,'standalone command token in the middle is valid');
assert.ok(e.relevance('status','supervisorctl status')>0,'standalone command token at the end is valid');
assert.ok(e.relevance('foo','prefixfoobar')>0,'three-letter infix matching remains supported');
assert.equal(e.rank('ls',{current:[{command:'ls -lah',finishedAt:5}],project:['ls -lh']})[0].command,'ls -lah','current terminal wins for equally strong matches');
