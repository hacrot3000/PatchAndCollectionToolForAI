const assert=require('node:assert/strict');
const e=require('./web/featuremods/smartterminal_engine.js');
const items=e.rank('sup',{
 current:[{command:'echo supervisor',finishedAt:4}],
 project:[{command:'supervisorctl status',finishedAt:15}],
 history:['supervisorctl restart all'],
 commands:['supervisorctl']});
assert.equal(items[0].command,'echo supervisor');
assert.equal(items[1].command,'supervisorctl status');
assert.equal(items[1].source,'project');
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
