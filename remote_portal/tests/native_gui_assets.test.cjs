'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');

const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
function extract(name){
 const rx=new RegExp('const '+name+' = `([\\s\\S]*?)`');
 const matched=raw.match(rx);
 assert.ok(matched,'missing native embedded '+name);
 return matched[1];
}
const js=extract('trustedGUIJavaScript');
const limits=fs.readFileSync('vscode_tasks_menu_go/internal/remotelimits/limits.go','utf8');
const relay=fs.readFileSync('vscode_tasks_menu_go/internal/remoterelay/app_mailbox.go','utf8');
const native=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui.go','utf8');
const readQuota=limits.match(/const MaxReadOnlyAppRounds\s*=\s*(\d+)/);
assert.ok(readQuota,'shared bounded E2EE quota is missing');
const maxRounds=Number(readQuota[1]);
assert.ok(maxRounds>4 && maxRounds<=16,'read-only app round quota is excessive or not expanded');
assert.match(relay,/MaxAppPingRounds=remotelimits\.MaxReadOnlyAppRounds/);
assert.match(native,/trustedGUIMaxActions = remotelimits\.MaxReadOnlyAppRounds/);
assert.match(js,new RegExp('budget='+maxRounds+';'));
assert.match(js,new RegExp('budget>'+maxRounds+'\\)'));

const html=extract('trustedGUIHTML');
const css=extract('trustedGUICSS');
new vm.Script(js,{filename:'native-remote-gui.js'});
assert.match(html,/\/app\.js/);
assert.match(html,/\/style\.css/);
assert.match(html,/id="git"/);
assert.match(html,/id="project"/);
assert.match(html,/id="tasks-preview"/);
assert.match(html,/id="task-preview-labels"/);
assert.match(html,/id="task-filter"/);
for(const id of ['task-sort','task-coverage','task-progress','task-next-help','task-filter-empty']){
  assert.match(html,new RegExp('id="'+id+'"'));
}
assert.match(js,/function renderTaskCoverage\(\)/);
assert.match(js,/taskSort\.addEventListener\('change'/);
assert.match(js,/pagehide/);
assert.match(js,/Phiên đã đóng khi rời trang/);
assert.match(html,/id="task-fetch-next"/);
for(const id of ['task-fetch-all','task-stop-all','task-load-status']){
 assert.match(html,new RegExp('id="'+id+'"'));
}
assert.match(js,/async function loadRemainingTaskPages\(\)/);
assert.match(js,/taskFetchAll\.addEventListener\('click'/);
assert.match(js,/taskStopAll\.addEventListener\('click'/);
assert.match(js,/i<16&&!bulkStopRequested/);

assert.match(js,/\/api\/tasks-preview-next/);
assert.match(js,/cachedTaskLabels\.push\(/);
assert.match(js,/function filteredTaskLabels\(\)/);
assert.match(js,/observedEpoch!==actionEpoch/);
assert.match(js,/observedStatus!==sessionStatusEpoch/);
assert.match(js,/observedStatus===sessionStatusEpoch/);
assert.match(js,/setInterval\(\(\)=>\{/);
assert.match(js,/15000/);
assert.match(js,/taskFilter\.addEventListener\('input'/);
assert.match(js,/\/api\/tasks-preview/);
assert.match(js,/taskLabels\.replaceChildren/);
assert.match(html,/id="project-overview"/);
assert.match(html,/id="project-name"/);
assert.match(js,/\/api\/project-info/);
assert.match(html,/id="ping"/);
for(const id of ['quick-command','quick-run','quick-hint']){
  assert.match(html,new RegExp('id="'+id+'"'));
}
assert.match(html,/Không phải Terminal/);
assert.ok(html.indexOf('id="quick-command"')<html.indexOf('id="panel-overview"'),
  'the quick palette must stay available outside all changing GUI tabs');
assert.match(js,/function runQuickAction\(\)/);
assert.match(js,/Object\.freeze\(\{/);
assert.match(js,/const targetViews=Object\.freeze/);
assert.match(js,/Object\.prototype\.hasOwnProperty\.call\(routes,command\)/);
assert.match(js,/quickCommand\.value=''/);
assert.match(js,/quickCommand\.addEventListener\('keydown'/);
for(const view of ['overview','project','tasks','git']){
  assert.match(html,new RegExp('id="nav-'+view+'"'));
  assert.match(html,new RegExp('id="panel-'+view+'"'));
}
assert.match(js,/function showView\(next\)/);
assert.match(js,/aria-current/);
assert.match(js,/function clearPrivateViews\(/);
assert.match(js,/if\(closed\)clearPrivateViews\(\)/);
assert.match(html,/id="features"/);
assert.match(html,/id="feature-summary"/);
assert.match(js,/\/api\/features/);
assert.match(js,/featuresKnown/);
assert.match(html,/id="disconnect"/);
assert.match(html,/id="local-token"/);
assert.match(html,/id="activate"/);
assert.match(js,/activation\.hidden/);
assert.match(js,/candidate=String\(localToken\.value/);
assert.match(html,/id="git-files"/);
for(const id of ['git-filter','git-kind','git-count','git-empty','git-delta-summary','git-delta-list']){
 assert.match(html,new RegExp('id="'+id+'"'));
}
assert.match(js,/function renderGitRows\(\)/);
assert.match(js,/gitFilter\.addEventListener\('input'/);
assert.match(js,/gitKind\.addEventListener\('change'/);
assert.match(js,/gitRows=\[\]/);
assert.match(js,/gitDeltaList\.replaceChildren\(\)/);
assert.match(js,/gitSnapshotBranch/);
assert.match(html,/không đồng bộ upstream/);
assert.match(html,/untracked/);
assert.match(js,/history\.replaceState/);
assert.match(js,/location\.hash/);
assert.match(js,/X-TaskDeck-Local-Capability/);
assert.match(js,/textContent/);
assert.match(js,/replaceChildren/);
assert.match(js,/same-origin/);
assert.match(js,/credentials:'omit'/);
assert.doesNotMatch(js,/innerHTML\s*=|eval\s*\(|localStorage\s*\.|document\.write\s*\(/);
assert.doesNotMatch(js,/https?:\/\//,'client must never fetch code from PHP');
assert.doesNotMatch(html,/<script[^>]+src="https?:/);
assert.doesNotMatch(html,/<iframe/);
assert.ok(css.length>800,'responsive GUI stylesheet missing');
console.log('PASS: bundled trusted native GUI JavaScript syntax, local-only assets and safe rendering');
