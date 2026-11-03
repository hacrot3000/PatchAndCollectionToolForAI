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
const html=extract('trustedGUIHTML');
const css=extract('trustedGUICSS');
new vm.Script(js,{filename:'native-remote-gui.js'});
assert.match(html,/\/app\.js/);
assert.match(html,/\/style\.css/);
assert.match(html,/id="git"/);
assert.match(html,/id="ping"/);
assert.match(html,/id="disconnect"/);
assert.match(html,/id="git-files"/);
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
