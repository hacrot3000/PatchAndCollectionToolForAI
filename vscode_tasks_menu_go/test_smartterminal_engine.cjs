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


// Path completion is context-aware but never executes the command.
const ctx=e.pathContext('cp Cli');
assert.deepEqual([ctx.command,ctx.dir,ctx.prefix],['cp','','Cli']);
assert.deepEqual([e.pathContext('cp Client/as').dir,e.pathContext('cp Client/as').prefix],['Client/','as']);
assert.equal(e.rankPaths('cp Cli',[{name:'Client',dir:true}])[0].command,'cp Client/');
assert.equal(e.rankPaths('cp Client/as',[{name:'assets',dir:true}])[0].command,'cp Client/assets/');
assert.equal(e.rankPaths('cd Cli',[{name:'Client',dir:true},{name:'Cli.log',dir:false}]).length,1,
 'cd should suggest directories only');
assert.equal(e.rankPaths('cp My',[{name:'My Folder',dir:true}])[0].command,'cp My\\ Folder/');
assert.equal(e.rankPaths('cp Client/My\\ Fi',[{name:'My File.txt',dir:false}])[0].command,'cp Client/My\\ File.txt');
assert.equal(e.rankPaths('cp cli',[{name:'Client',dir:true}])[0].command,'cp Client/',
 'case-insensitive completion should preserve filesystem casing');
assert.equal(e.rankPaths('cp Fi',[{name:'Fi$danger',dir:false}])[0].command,'cp Fi\\$danger',
 'shell metacharacters in filenames must be escaped');
assert.equal(e.pathContext('git add Cli').command,'git add');
assert.equal(e.pathContext('sudo cp Cli').command,'cp');
assert.equal(e.pathContext('cp -r Cli').command,'cp');
assert.equal(e.pathContext('git checkout Cli'),null,'Git refs are not filesystem paths');
assert.equal(e.pathContext('echo Cli'),null,'unrelated commands do not trigger directory enumeration');
assert.equal(e.pathContext('cp Cli; rm'),null,'no path completion without an operand');
assert.equal(e.pathContext('cp Client/$HOME'),null,'no shell variable interpolation');
assert.equal(e.pathContext('cp Client/*.md'),null,'do not resolve globs');
assert.equal(e.pathContext('cp Client/as\\'),null,'unfinished escape is not a safe completion target');
