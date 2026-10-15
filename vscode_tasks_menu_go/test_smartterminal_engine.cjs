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

const aliases=e.parseAliases([
 "alias gb='git branch'","alias gc='gc-safe'","alias ll='ls -alF'",
 "alias ga='git add'","alias gis='git status'","alias gí='git status'",
 "alias df='df -h | grep -v loop'"
]);
assert.equal(aliases.gc,'gc-safe');
assert.equal(aliases.gb,'git branch');
assert.equal(aliases.ll,'ls -alF');
assert.equal(aliases.gí,'git status');
assert.equal(aliases.df,undefined,'complex pipeline aliases are not treated as direct commands');
assert.equal(e.pathContext('ga Cli',aliases).command,'git add');
assert.equal(e.pathContext('ll Cli',aliases).command,'ls');
assert.equal(e.rankPaths('ga Cli',[{name:'Client',dir:true}],10,aliases)[0].command,'ga Client/');
assert.equal(e.branchContext('gc tes',aliases).verb,'checkout');
assert.equal(e.rankBranches('gc tes',['testing','ss20-dev-hp64'],aliases)[0].command,'gc testing');
assert.equal(e.rankBranches('git merge ss20',['ss20-dev-hp64','testing'],aliases)[0].command,'git merge ss20-dev-hp64');
assert.equal(e.rankBranches('gb -d ss20',['ss20-dev-hp64','testing'],aliases)[0].command,'gb -d ss20-dev-hp64');
assert.equal(e.branchContext('gis tes',aliases),null,'git status aliases do not accept branch-name completion');
assert.equal(e.branchContext('gb testing',aliases).mode,'safe-list','git branch must never accidentally create a branch through completion');
assert.equal(e.branchContext('gc ',aliases).prefix,'');
assert.equal(e.rankBranches('gc ',['testing'],aliases)[0].command,'gc testing');

// Command names come from PATH or shell index, never a hardcoded dictionary.
assert.ok(!e.examples.includes('telnet'),'telnet must be discovered from PATH');
assert.equal(e.commandPrefix('tel'),'tel');
assert.equal(e.commandPrefix('telnet 10.9.1.20'),null,'argument text is not a command lookup');
assert.equal(e.commandPrefix('Client/tel'),null,'paths are not executable-name lookups');
assert.equal(e.commandPrefix('\\tel'),null,'reserved shell escapes are not executable names');
assert.equal(e.rankExecutables('tel',['telnet','telemetry','ls'])[0]?.command,'telnet','shortest matching executable should lead');
assert.deepEqual(e.rankExecutables('tel',['telnet']).map(x=>x.command),['telnet']);
assert.equal(e.rankExecutables('TEL',['telnet'])[0]?.command,'telnet');
assert.equal(e.suffix('tel',{command:'telnet'}),'net');
assert.equal(e.rank('teln',{commands:['telnet']})[0]?.command,'telnet');


// Bare branch commands and aliases should show repo branches before the user
// types an operand, without requiring a trailing whitespace character.
const repoNames=['testing','ss20-dev-hp64','stable-release'];
for(const command of ['git branch','git branch ','gb','gb ']){
 const rows=e.rankBranches(command,repoNames,aliases);
 assert.equal(rows.length,3,command+' must suggest actual refs');
 assert.ok(rows.every(item=>item.command.includes(' --list ')),command+' must not create a branch');
 assert.equal(rows[0].source,'branch');
}
for(const command of ['git checkout','git checkout ','gc','gc ','git switch','git merge']){
 const ctx=e.branchContext(command,aliases);
 assert.ok(ctx,command+' branch mode missing');
 const rows=e.rankBranches(command,repoNames,aliases);
 assert.ok(rows.length>0,command+' should show branch suggestions');
 assert.ok(rows[0].command.endsWith('testing'),command+' should insert a separating space');
}
assert.equal(e.rankBranches('cp ar',repoNames,aliases).length,0);
assert.equal(e.rankBranches('gb -d ss20',repoNames,aliases)[0]?.command,'gb -d ss20-dev-hp64');
assert.equal(e.rankBranches('git branch --list tes',repoNames,aliases)[0]?.command,'git branch --list testing');
assert.equal(e.rankPaths('cp ar',[{name:'artifacts',dir:true}])[0]?.command,'cp artifacts/');

assert.deepEqual(e.rankBranchOptions('gb',aliases).map(x=>x.command),
 ['gb -a','gb -vv','gb --list','gb --show-current']);
assert.equal(e.rankBranches('gc develop/ne',['develop/new_SNV'],aliases)[0]?.command,'gc develop/new_SNV');
assert.equal(e.rankPaths('cp',[{name:'artifacts',dir:true}])[0]?.command,'cp artifacts/');
assert.equal(e.rankPaths('ls',[{name:'artifacts',dir:true}])[0]?.command,'ls artifacts/');
assert.equal(e.rankPaths('ll',[{name:'artifacts',dir:true}],10,aliases)[0]?.command,'ll artifacts/');
assert.equal(e.rankPaths('ga',[{name:'artifacts',dir:true}],10,aliases)[0]?.command,'ga artifacts/');


// Autocomplete popup follows the editable cursor row, never overlapping it.
// Place above when the full menu fits; otherwise flip below; when neither
// side can hold it, shrink on the larger side with scrollable results.
const above=e.popupPlacement(500,18,240,800);
assert.equal(above.side,'above');
assert.equal(above.top+above.maxHeight<=500-6,true);
assert.equal(above.maxHeight,240);
const below=e.popupPlacement(42,18,240,800);
assert.equal(below.side,'below');
assert.ok(below.top>=42+18+6);
assert.equal(below.maxHeight,240);
const screenshot=e.popupPlacement(258,16,260,311);
assert.equal(screenshot.side,'above','short Firefox viewport must not cover cursor row');
assert.equal(screenshot.top,6);
assert.equal(screenshot.maxHeight,246);
assert.ok(screenshot.top+screenshot.maxHeight<=258-6);
const constrainedBelow=e.popupPlacement(120,16,260,280);
assert.equal(constrainedBelow.side,'below');
assert.equal(constrainedBelow.maxHeight,132);
assert.ok(constrainedBelow.top+constrainedBelow.maxHeight<=280-6);
const tiny=e.popupPlacement(20,16,260,60);
assert.equal(tiny,null,'do not display an unreadable clipped list over the cursor');
assert.equal(e.popupPlacement(100,0,260,800),null,'invalid geometry must not position the popup');


// Approximate matching is shared across branch, filesystem, PATH and history.
// It must be CONTIGUOUS: 'multia' must not skip the 'w' in multi-wave.
const fuzzyAliases=e.parseAliases(["alias gc='gc-safe'"]);
const multiBranch=e.rankBranches('gc multi',['ss20-dev-multi-wave','testing'],fuzzyAliases);
assert.equal(multiBranch[0]?.command,'gc ss20-dev-multi-wave');
assert.equal(e.rankBranches('gc multia',['ss20-dev-multi-wave'],fuzzyAliases).length,0);
assert.deepEqual(e.inlinePreview('gc multi',multiBranch[0]),{
 kind:'replace',start:3,before:'ss20-dev-',matched:'multi',after:'-wave'
});
const fuzzyPaths=e.rankPaths('cp python',[
 {name:'a_python_patch',dir:true},{name:'python3',dir:false},{name:'python-helpers',dir:true}
]);
assert.equal(fuzzyPaths[0]?.command,'cp python-helpers/');
assert.ok(fuzzyPaths.some(x=>x.command==='cp a_python_patch/'));
assert.equal(e.rankPaths('cp pythona',[{name:'a_python_patch',dir:true}]).length,0);
assert.deepEqual(e.inlinePreview('cp python',{command:'cp a_python_patch/'}),{
 kind:'replace',start:3,before:'a_',matched:'python',after:'_patch/'
});
assert.deepEqual(e.inlinePreview('cp artifacts/script',{command:'cp artifacts/patch_script'}),{
 kind:'replace',start:13,before:'patch_',matched:'script',after:''
});
assert.equal(e.rankExecutables('grep',['egrep','pgrep','grep','zgrep']).length,3);
assert.equal(e.rankExecutables('grepx',['egrep','pgrep']).length,0);
assert.equal(e.rank('gc multi',{history:['gc ss20-dev-multi-wave']})[0]?.command,'gc ss20-dev-multi-wave');
assert.equal(e.rank('gc multia',{history:['gc ss20-dev-multi-wave']}).length,0);
assert.equal(e.nameMatch('multi','ss20-dev-multi-wave')?.index,9);
assert.equal(e.nameMatch('multia','ss20-dev-multi-wave'),null);
assert.equal(e.nameMatch('ls','tools'),null,'two-letter arbitrary infix remains excluded');
assert.equal(e.nameMatch('py','a_python_patch')?.score,2,'word-boundary two-letter match is valid');
