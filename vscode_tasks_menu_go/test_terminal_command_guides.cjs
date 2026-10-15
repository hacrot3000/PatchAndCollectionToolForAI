const test=require('node:test');
const assert=require('node:assert/strict');
const guides=require('./web/featuremods/terminal_command_guides.js');
const find=(input,command)=>guides.suggest(input,30).find(row=>row.command===command);
test('Git remote set-url offers meaningful alternative workflows and hints',()=>{
 const names=guides.suggest('git remote set-url',12);
 for(const command of [
  'git remote -v',
  'git remote add ',
  'git remote set-url origin ',
  'git remote set-url ',
  'git remote set-url --add origin ',
  'git remote set-url --delete origin ',
  'git remote set-url --push origin ',
  'git remote set-url --add --push origin ',
  'git remote set-url --delete --push origin ',
  'git remote remove '
 ]){
  // Listing remote -v comes from the parent 'git remote' context.
  if(command==='git remote -v')continue;
  assert.ok(names.some(row=>row.command===command),command);
 }
 const item=find('git remote set-url','git remote set-url origin ');
 assert.equal(item.source,'guide');
 assert.match(item.syntax,/<new-url>/);
 assert.match(item.help,/origin/);
 assert.equal(item.command.includes('<'),false,'help placeholders must never be sent to Bash');
 assert.ok(find('git remote','git remote -v'));
 assert.ok(find('git remote','git remote rename '));
 assert.ok(find('git remote se','git remote set-url origin '));
});
test('remote parameter selection can continue with a user-entered remote name',()=>{
 const chosen=find('git remote set-url upstream','git remote set-url upstream ');
 assert.ok(chosen);
 assert.match(chosen.syntax,/<new-url>/);
 assert.deepEqual(guides.suggest('git remote set-url upstream https:',10),[]);
 assert.deepEqual(guides.suggest('git remote add upstream',10),[]);
});
test('common Git workflows are grouped and all have concise explanations',()=>{
 const samples=[
  ['git','git status'],['git status','git status --short'],
  ['git branch','git switch -c '],['git switch','git switch -c '],
  ['git add','git add -p'],['git commit','git commit -m '],
  ['git fetch','git fetch --all --prune'],
  ['git pull','git pull --ff-only'],['git push','git push -u origin '],
  ['git diff','git diff --staged'],['git log','git log --graph --oneline --decorate --all'],
  ['git merge','git merge --abort'],['git rebase','git rebase --abort'],
  ['git restore','git restore --staged '],['git reset','git reset --hard HEAD'],
  ['git stash','git stash list'],['git tag','git tag --list'],
  ['git clean','git clean -nd'],['git worktree','git worktree add ']
 ];
 for(const [typed,command] of samples)assert.ok(find(typed,command),typed+' -> '+command);
 for(const section of ['git','git remote','git branch','git push','git reset']){
  for(const row of guides.suggest(section,12)){
   assert.ok(row.syntax&&row.label&&row.help);
   assert.equal(row.source,'guide');
   assert.ok(!/[\r\n]/.test(row.command));
  }
 }
 assert.equal(find('git reset','git reset --hard HEAD').risk,'danger');
 assert.equal(find('git remote','git remote remove ').risk,'caution');
});
test('guide suggestions never include literal placeholders or execute commands',()=>{
 for(const command of ['git','git remote','git branch','git reset','git push','git rebase']){
  const rows=guides.suggest(command,12);
  assert.ok(rows.length>0);
  for(const row of rows){
   assert.ok(!/[<>\r\n\x00]/.test(row.command),row.command);
   assert.ok(!row.command.endsWith('\n')&&!row.command.endsWith('\r'));
   assert.ok(['normal','caution','danger'].includes(row.risk));
  }
 }
});
test('suppress guides when user is typing a real argument or shell construct',()=>{
 for(const value of [
  'git remote set-url origin https://example.com/repo.git',
  'git commit -m "message"',
  'git status; rm -rf /',
  'git status | cat',
  'git remote add upstream git@example.com:repo.git',
  'echo git remote set-url',
  'gc multi',
  'git remote set-url $URL',
  'git remote set-url \nmalicious',
  'git push --force-with-lease origin branch'
 ])assert.deepEqual(guides.suggest(value,10),[],value);
});
test('contextual command provider registry permits future tools without changing the renderer',()=>{
 assert.ok(guides.providerIds().includes('git'));
 assert.equal(guides.register({id:'git',sections:{}}),false);
 const future={id:'tool-test',sections:{base:[['tool-test list','tool-test list','Liệt kê','Read only']]}};
 assert.equal(guides.register(future),true);
 assert.equal(guides.suggest('tool-test')[0].command,'tool-test list');
 assert.equal(guides.register(future),false);
 assert.deepEqual(guides.suggest('random-unknown'),[]);
});
