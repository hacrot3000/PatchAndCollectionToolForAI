// Local, read-only Help. Keep each feature in one category, searchable globally.
// Never render help content as HTML: all descriptions use textContent.
const taskdeckHelpTopics=[
 {id:'getting-started',title:'Quick start',items:[
  ['Help & navigation','Open Settings → Help, choose Help in the Command Palette, or type \\help at an idle terminal prompt inside TaskDeck web. This is not a command in an external Ubuntu/Bash terminal. Search finds entries across every category.'],
  ['Tabs & Split','Right-click a tab for its actions, or a gap in the tab bar to reopen a tab or create a terminal. Drag/split tabs into groups; hover a busy terminal tab to see the running command.'],
  ['Connections','Open the Connections panel to choose a saved SSH, database or FTP/SFTP profile. Verify destination and permission before changing remote data.'],
  ['Operation Center','Review running, completed and failed operations, including transfers, Git operations and Patch Tool jobs.'],
  ['Safety','Review changes and backups before destructive Git, database, patch, and remote-file operations. Auto-reconnect does not re-run interrupted commands.']
 ]},
 {id:'shortcuts',title:'General shortcuts',items:[
  ['Ctrl/Cmd + Shift + P','Open the Command Palette to find tools and actions.'],
  ['Ctrl/Cmd + P','Quick Open: find files, tabs, and resources.'],
  ['Ctrl/Cmd + Shift + B','Tasks: Run Build Task. Open a searchable popup filtered to tasks in the build group.'],
  ['Ctrl + Alt + R','Tasks: Run Task. Open the full task picker with filtering, arrow keys, Enter and Esc.'],
  ['Ctrl + Alt + Shift + T','Tasks: Run Test Task. Open a popup filtered to test-group tasks (avoids the Linux Ctrl+Alt+T shortcut).'],
  ['Tasks popup','Use the Tasks… button near the header or Command Palette → Tasks: Run Task / Run Build Task / Run Test Task / Rerun Last Task. Tasks run with existing permissions and input prompts.'],
  ['Ctrl/Cmd + Shift + F','Find and replace text across the project.'],
  ['Ctrl/Cmd + Backquote','Focus an idle local terminal. If none is idle and connected, create a new terminal tab. Busy commands and SSH/task tabs are not reused.'],
  ['Ctrl/Cmd + Shift + Backquote','Open a new local terminal.'],
  ['Ctrl + Alt + W','Close the selected TaskDeck tab; a safer Firefox alternative to Ctrl+W.'],
  ['Ctrl/Cmd + W','Close a TaskDeck tab only if the browser forwards the shortcut; Firefox may close the browser tab instead.'],
  ['Esc','Close a menu/dialog; close an eligible Diff, viewer, Patch Tool or never-edited file tab.']
 ]},
 {id:'editor',title:'File Editor',items:[
  ['Ctrl/Cmd + S','Save the active editor file.'],
  ['Ctrl/Cmd + F','Find in the file; Enter/Shift+Enter moves between matches.'],
  ['Ctrl/Cmd + H','Find and replace in the file.'],
  ['Ctrl/Cmd + G','Go to a line number.'],
  ['Ctrl/Cmd + Shift + O','Show the file outline and symbols.'],
  ['Ctrl/Cmd + Shift + T','Reopen the most recently closed file editor.'],
  ['Quick Open & Explorer','Search files, open tabs, or navigate the folder tree. Use file preview or Markdown preview before editing.'],
  ['Markdown Preview','For .md/.markdown files in the editor, click Preview beside Reload and Find to render the current text, including unsaved edits. In Explorer, right-click a Markdown file and choose Preview Markdown to view its saved contents.'],
  ['Autocomplete & language tools','Use editor language keywords, project symbols and Language Server actions when configured; availability varies by language.'],
  ['Project Search','Search across files and preview matching locations before applying replacements.']
 ]},
 {id:'terminal',title:'Terminal',items:[
  ['Ctrl + C','Interrupt the command in the focused terminal.'],
  ['Ctrl + F','Find text in terminal output.'],
  ['Tab / ↑ / ↓','Complete or browse shell command history, depending on the shell.'],
  ['\\help','Open this Help popup at an idle writable shell prompt in a TaskDeck Terminal/SSH tab. Also accepts a category or phrase (e.g. \\help database). In a standalone Bash terminal, \\help is interpreted as the Bash help builtin instead.'],
  ['\\ai <question>','Ask the configured AI provider for advice. The answer is advisory and never executes a shell command. Provider consent and permissions apply.'],
  ['\\do reload command','Refresh shell command completions from the current supported shell.'],
  ['\\do reload history','Refresh indexed shell history for terminal suggestions.'],
  ['Smart Terminal','Shell completion suggestions and AI commands are TaskDeck overlays; ordinary shell input and interactive applications still use the normal PTY.'],
  ['AI Terminal Troubleshooter','Select text in a terminal and right-click Ask AI about selection. Only a command actually entered in this browser page and verified as the latest command of the active terminal can trigger the nonzero-exit AI help prompt; old errors replayed after a reload never trigger it. Clicking Yes · Ask AI on the error prompt or Ask AI in the wizard sends the selected/visible excerpt without a second confirmation dialog (provider consent still must be enabled in Settings). AI returns JSON repair actions; each requires its own explicit Run confirmation on the verified idle terminal. After successful Copy / Copy JSON or approved Run, the wizard animates into a dock in the original terminal pane (split-safe). Click ✦ AI Smart Terminal to restore the same result without contacting AI again, or × to close it; manual Minimize is also available. No automatic execution or terminal-group broadcast.'],
  ['Filesystem autocomplete','At an idle local terminal prompt, typing ls, cp ar or cp artifacts/ suggests items from the actual current directory (including subfolders). Use Tab to insert the highlighted completion; Enter still controls command execution. No shell command is run to enumerate files.'],
  ['Approximate autocomplete','Terminal suggestions from Git branches, filenames, executable commands and command history now accept contiguous middle-of-name matches (gc multi → gc ss20-dev-multi-wave). Missing text is previewed in a dim color; it is never sent to Bash unless you explicitly accept with Tab or a click. Typing a nonmatching next character immediately hides the preview.'],
  ['Git command guides','At an idle terminal prompt, type git remote set-url, git branch, git pull, git push, git reset or similar Git subcommands. The popup shows common operations, required parameter syntax and concise guidance. Warning text identifies risky commands. Press Tab or click to insert only a safe incomplete command prefix; placeholders like <url> are help text and never typed into Bash automatically. No command executes until Enter. Additional command families can be registered independently.'],
  ['Autocomplete Trace diagnostics','Open Autocomplete Trace… from the TaskDeck header or Command Palette, choose a local terminal from the dropdown (legacy single-user terminals are supported), and Start trace. Type cp a_py without Enter, then reopen Trace, use Snapshot and Probe now, Stop, and Download JSON. If the prompt is not recognized, type cp a_py in the optional API probe field. The trace is off by default, in-memory only, and never uploads shell input; review any paths or filenames before sharing.'],
  ['PATH executable suggestions','Executable names (e.g. telnet) are discovered from the actual PATH directories of the local TaskDeck host, not a hardcoded list. Type tel at an idle Bash prompt, select telnet and press Tab to insert net; nothing runs until Enter. Bash compgen still supplements shell-specific commands and aliases.'],
  ['Exit Telnet','While connected, press Ctrl + ] to reach the telnet> prompt, then type quit and Enter. The Bash exit command and ^]q are not Telnet escape commands. Telnet is an interactive program, so shell completions are suspended at telnet>.'],
  ['Context-aware suggestions','At an idle local terminal prompt, cp ar suggests cp artifacts/; bare cp, ls and aliases such as ll and ga suggest actual entries from the live terminal CWD. Completing a nested path continues in its next directory. Tab inserts the chosen text; no command executes automatically.'],
  ['Git branch & shell aliases','Typing gb or git branch suggests actual local Git refs plus inspection switches. Branch-name suggestions use --list (e.g. gb --list testing), avoiding accidental branch creation. gc/git checkout/git switch suggest checkout targets; develop/new_SNV is supported. No suggestion executes automatically.'],
  ['Alias fallback without OSC','When Bash OSC markers are absent, local TaskDeck may read simple alias declarations from .bashrc and .bash_aliases without executing them. The active shell alias -p inventory takes precedence when available. Complex aliases and changed runtime-only definitions may require \\do reload aliases from a verified shell prompt.'],
  ['Silent terminal suggestions','Opening or activating a terminal never sends hidden alias -p, compgen or TDIDX scripts into the PTY. TaskDeck reads cached commands and read-only local alias/PATH metadata instead. Only an explicit \\do reload aliases or \\do reload command asks the active shell for an inventory.'],
  ['Refresh alias index','Run \\do reload aliases to refresh the current shell alias map after modifying aliases or shell startup scripts. SSH, interactive programs and unsafe alias expansions are not treated as local filesystem lookups.'],
  ['Sessions & split tabs','Terminals, tasks and SSH sessions retain their own logs; a running process may survive a browser refresh via session restore.'],
  ['Console actions','Right-click or use pane actions for copying output, finding text, clearing scrollback, renaming or inspecting a running session.']
 ]},
 {id:'ssh',title:'SSH & connections',items:[
  ['SSH profiles','Open saved SSH sessions from Connections. TaskDeck uses OpenSSH and does not disable SSH host-key verification.'],
  ['Auto reconnect','An unexpected SSH transport failure (exit 255) can trigger retries when Auto reconnect is enabled. An intentional exit or exit code 0 never reconnects automatically.'],
  ['Reconnect now / Cancel retries','Control retries from the SSH tab. Automatic retries are limited; authentication and host-key errors require user action.'],
  ['SSH command safety','Reconnect opens a new login: remote shell variables, working directory and interrupted commands are not replayed.'],
  ['Tunnel Graph','Inspect relevant SSH/DB tunnel relationships and connection information.'],
  ['Remote Workspace','Use authorized SSH-backed workspace and remote-file tools to inspect a remote project.']
 ]},
 {id:'database',title:'Database & SQL results',items:[
  ['Database connections','Open MySQL, SQLite, MongoDB or Redis profiles supported by the available runtime; select the right catalog before running queries.'],
  ['Multiple SQL statements','Running multiple SELECT statements creates Result 1, Result 2 and so on, each associated with its own statement.'],
  ['Result tab right-click','Right-click a result tab to copy its rows or export the selected result or all successful results.'],
  ['Export all results · one ZIP','Export successful result tabs together in query-results.zip with result-01.csv, result-02.csv, etc. Choose CSV, TXT or JSON.'],
  ['Export all results · separate ZIPs','Choose Separate ZIP for each result tab to generate result-01.zip, result-02.zip, etc. Host exports choose the destination once; browser downloads may need permission.'],
  ['Include SQL statement','Optionally place each SELECT as SQL comments at the start of CSV/TXT, or as the sql property in a valid JSON export.'],
  ['Export limits','Exports use loaded rows, including active filters/order and Max rows; they do not re-run SQL. Failed or unfinished tabs are excluded.'],
  ['Row selection & copying','Select all visible rows, selected rows, or the current result; copy TXT, CSV or JSON with optional headers.'],
  ['Filter / Order / Edit','Use result-grid actions to filter or order loaded rows. Editable grids require explicit Apply changes; check the active connection first.'],
  ['Schema Diff','Compare database structures using Database: Schema Diff before applying schema changes.']
 ]},
 {id:'git',title:'Git',items:[
  ['Git Quick Actions','Select a repository to inspect status, stage/commit/push, fetch/pull, stash, branches and recovery actions.'],
  ['Branch Compare','Compare two branch snapshots using a folder-style tree, hide identical entries and open changed files in Diff tabs.'],
  ['Clone branch…','In Branches click Clone branch… (or Clone on a branch row). Select a local or fetched remote-tracking source, enter a new local branch name, and create a branch at that exact source commit. Does not switch or push.'],
  ['Copy branch name','Click the branch name in Branches to copy its name to the clipboard. This only copies text; it does not create a branch or copy files.'],
  ['Merge / conflicts','Use the Git merge/conflict wizard to review decisions and file differences before accepting changes.'],
  ['Git Ignore Wizard','Review suggestions for ignored files or folders and confirm the .gitignore changes.'],
  ['Push failures','Use Git recovery hints to diagnose remotes, timeouts and oversized objects; review history before force operations.'],
  ['Worktrees & history','Use Branches, Worktrees, Log/Graph and File History for targeted inspection.']
 ]},
 {id:'compare',title:'Diff & compare',items:[
  ['Diff','Compare two files or branches, choose View diff/context and toggle unimportant changes. Escape closes a never-edited File Diff.'],
  ['Context around changes','View diff context limits surrounding lines around changed blocks; expand collapsed areas when needed.'],
  ['Inline changes','Review token-level differences, syntax highlighting, copy actions and shared horizontal scrolling.'],
  ['Folder Compare','Compare local or supported remote folders, filter identical entries, inspect one-sided files and open per-file diffs.'],
  ['Multi-source comparison','Open eligible files from editor, Git or file-transfer locations and compare their contents.']
 ]},
 {id:'transfer',title:'FTP / SFTP & files',items:[
  ['Profiles & dual panes','Choose FTP/SFTP profiles and navigate local/remote paths. Check the active remote before transferring.'],
  ['Transfer queue','Track uploads/downloads, scanning, concurrency, progress and cancellation through the queue.'],
  ['Large / growing files','File transfers show progress. For log files changing during transfer, prefer supported append/consistency checks rather than assuming every growth is safe to append.'],
  ['Compressed upload','For many small files, Compress + upload + extract may be offered when the remote supports extraction and collision/safety checks pass.'],
  ['Overwrite & conflicts','Review file-name collisions and server errors before overwriting or extracting archives.']
 ]},
 {id:'patch',title:'Patch Tool & Collection',items:[
  ['Patch Tool','Prepare a patch or collection, watch the queue, then inspect results and handoff.'],
  ['Upload a patch','Upload a supported patch or collection package instead of scanning only host folders.'],
  ['Status colors','Successful patch operations use green; successful collections use light yellow. Failures keep their error/handoff information.'],
  ['Multi-user mutation lock','Shared-server mutation locks protect concurrent writes. Review ownership and permissions rather than bypassing an active lock.'],
  ['Queue & handoff','Review current run, queued operations, validation errors and handoff before closing the result.']
 ]},
 {id:'ai',title:'AI Assistant',items:[
  ['Smart Terminal & AI','Use AI advice from a terminal or its panel. Advice is separate from automatic shell execution.'],
  ['Providers & privacy','Configure providers and API credentials in Settings; check consent and context before sending a request.'],
  ['\\ai','Type \\ai with a question at a supported idle prompt. AI output is advisory and requires configured access.'],
  ['Request Debug','Inspect temporary AI request diagnostics when troubleshooting; do not share secrets in debug reports.']
 ]},
 {id:'settings',title:'Settings & administration',items:[
  ['Help & keyboard commands','Open Settings → Help or use \\help in TaskDeck terminal. Ctrl/Cmd+Shift+P opens the Command Palette.'],
  ['Appearance','Change visual theme and editor/terminal appearance using Settings.'],
  ['Backup / Restore','Export/import TaskDeck configuration with the Settings Backup / Restore tools.'],
  ['Shared-server permissions','Users see only authorized sessions and actions; admin approval and connection permissions apply.'],
  ['Project Health','Open the health dashboard and Operation Center to review current workspace status.'],
  ['Self-update','Use built-in update checks and candidate validation; the running TaskDeck stays usable if activation fails.']
 ]}
];

const style=document.createElement('style');
style.textContent=[
'.taskdeck-help-backdrop{position:fixed;inset:0;z-index:18000;display:none;align-items:flex-start;justify-content:center;padding:clamp(10px,3vh,32px) 12px;background:rgba(0,0,0,.64)}',
'.taskdeck-help-backdrop.visible{display:flex}',
'.taskdeck-help-dialog{width:min(1040px,100%);max-height:94vh;display:flex;flex-direction:column;min-height:0;overflow:hidden;background:#151c26;color:#edf3fb;border:1px solid #56677e;border-radius:10px;box-shadow:0 18px 55px #0009}',
'.taskdeck-help-head{display:flex;gap:10px;align-items:center;padding:11px 14px;border-bottom:1px solid #3c4a5d}',
'.taskdeck-help-head h2{font-size:16px;flex:1;margin:0}.taskdeck-help-head button{font-size:16px;padding:3px 10px;border-radius:5px;border:1px solid #61738e;background:#27364a;color:#f5f9ff}',
'.taskdeck-help-intro{font-size:12px;line-height:1.5;margin:8px 14px;opacity:.84}',
'.taskdeck-help-search{box-sizing:border-box;width:calc(100% - 28px);margin:0 14px 10px;padding:8px 10px;border:1px solid #697b94;border-radius:6px;background:#0d1520;color:#f5faff;font-size:12px}',
'.taskdeck-help-layout{display:grid;grid-template-columns:minmax(170px,230px) minmax(0,1fr);flex:1;min-height:0;border-top:1px solid #344052}',
'.taskdeck-help-nav{overflow-y:auto;min-height:0;padding:9px 7px;border-right:1px solid #344052;display:flex;flex-direction:column;gap:3px}',
'.taskdeck-help-nav button{background:transparent;color:inherit;text-align:left;padding:8px 10px;border:0;border-radius:6px;font-size:12px;cursor:pointer;display:flex;gap:6px;align-items:center}',
'.taskdeck-help-nav button span{flex:1}.taskdeck-help-nav button small{opacity:.64}',
'.taskdeck-help-nav button:hover{background:#26384a}.taskdeck-help-nav button.active{background:#254460;color:#fff;font-weight:700}',
'.taskdeck-help-scroll{overflow:auto;flex:1;min-height:0;padding:4px 16px 16px}',
'.taskdeck-help-section{margin:12px 0 18px}.taskdeck-help-section h3{margin:0 0 9px;font-size:14px;color:#c3dfff}',
'.taskdeck-help-grid{display:grid;grid-template-columns:minmax(135px,205px) minmax(0,1fr);border:1px solid #3e4a5d;border-radius:7px;overflow:hidden}',
'.taskdeck-help-key,.taskdeck-help-description{font-size:12px;line-height:1.55;padding:9px 10px;border-bottom:1px solid #344052}',
'.taskdeck-help-key{background:#202c3a;color:#e4f0ff;overflow-wrap:anywhere}.taskdeck-help-key kbd{font:600 11px/1.5 ui-monospace,monospace}',
'.taskdeck-help-description{color:#e1eaf4}.taskdeck-help-key[hidden],.taskdeck-help-description[hidden]{display:none!important}',
'.taskdeck-help-empty{font-size:12px;opacity:.7;padding:12px}',
'html[data-taskmenu-theme="light"] .taskdeck-help-dialog{background:#fff;color:#182738;border-color:#bac5d2}',
'html[data-taskmenu-theme="light"] .taskdeck-help-head,html[data-taskmenu-theme="light"] .taskdeck-help-layout,html[data-taskmenu-theme="light"] .taskdeck-help-nav{border-color:#d4deea}',
'html[data-taskmenu-theme="light"] .taskdeck-help-head button{background:#eaf1f8;color:#1c344b;border-color:#aebcca}',
'html[data-taskmenu-theme="light"] .taskdeck-help-search{background:#f9fbff;color:#172535;border-color:#b5c1d0}',
'html[data-taskmenu-theme="light"] .taskdeck-help-nav button:hover{background:#edf3fa}',
'html[data-taskmenu-theme="light"] .taskdeck-help-nav button.active{background:#d7e9fa;color:#163f68}',
'html[data-taskmenu-theme="light"] .taskdeck-help-section h3{color:#275a82}',
'html[data-taskmenu-theme="light"] .taskdeck-help-grid{border-color:#ccd6e0}',
'html[data-taskmenu-theme="light"] .taskdeck-help-key{background:#edf3fa;color:#193850}',
'html[data-taskmenu-theme="light"] .taskdeck-help-description{color:#1b2938}',
'html[data-taskmenu-theme="light"] .taskdeck-help-key,html[data-taskmenu-theme="light"] .taskdeck-help-description{border-color:#dce4eb}',
'@media(max-width:640px){.taskdeck-help-dialog{max-height:95vh}.taskdeck-help-layout{grid-template-columns:1fr}.taskdeck-help-nav{flex-direction:row;overflow-x:auto;overflow-y:hidden;flex:0 0 auto;border-right:0;border-bottom:1px solid #344052}.taskdeck-help-nav button{white-space:nowrap;flex:0 0 auto}.taskdeck-help-scroll{padding:4px 9px 12px}.taskdeck-help-grid{grid-template-columns:minmax(112px,38%) minmax(0,1fr)}.taskdeck-help-key,.taskdeck-help-description{padding:7px;font-size:11px}}'
].join('\n');
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='taskdeck-help-backdrop';
const dialog=document.createElement('section');dialog.className='taskdeck-help-dialog';dialog.tabIndex=-1;
dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-labelledby','taskdeck-help-title');
const header=document.createElement('div');header.className='taskdeck-help-head';
const heading=document.createElement('h2');heading.id='taskdeck-help-title';heading.textContent='Help · TaskDeck';
const dismiss=document.createElement('button');dismiss.type='button';dismiss.textContent='×';dismiss.title='Close Help';dismiss.setAttribute('aria-label','Close Help');
header.append(heading,dismiss);
const intro=document.createElement('p');intro.className='taskdeck-help-intro';
intro.textContent='Choose a feature group on the left, or search all topics. Ctrl/Cmd means Ctrl on Linux/Windows and Cmd on macOS; browsers may reserve some shortcuts.';
const search=document.createElement('input');search.type='search';search.className='taskdeck-help-search';search.placeholder='Search all features, shortcuts and commands…';search.setAttribute('aria-label','Find Help topic');
const layout=document.createElement('div');layout.className='taskdeck-help-layout';
const nav=document.createElement('nav');nav.className='taskdeck-help-nav';nav.setAttribute('aria-label','Help feature groups');
const scroll=document.createElement('div');scroll.className='taskdeck-help-scroll';
const empty=document.createElement('p');empty.className='taskdeck-help-empty';empty.textContent='No matching Help topics.';empty.hidden=true;
let selectedTopic='getting-started';
const viewSections=[];
function navButton(topicID,label,count){
 const button=document.createElement('button');button.type='button';
 button.textContent='';
 const name=document.createElement('span');name.textContent=label;
 const tally=document.createElement('small');tally.textContent=String(count);
 button.append(name,tally);
 button.onclick=()=>{selectedTopic=topicID;search.value='';filterHelp();scroll.scrollTop=0;};
 nav.append(button);
 return button;
}
const allButton=navButton('all','All topics',taskdeckHelpTopics.reduce((n,t)=>n+t.items.length,0));
for(const topic of taskdeckHelpTopics){
 const button=navButton(topic.id,topic.title,topic.items.length);
 const section=document.createElement('section');section.className='taskdeck-help-section';
 const title=document.createElement('h3');title.textContent=topic.title;
 const grid=document.createElement('div');grid.className='taskdeck-help-grid';
 const entries=[];
 for(const [key,description] of topic.items){
   const left=document.createElement('div');left.className='taskdeck-help-key';
   const kbd=document.createElement('kbd');kbd.textContent=key;left.append(kbd);
   const right=document.createElement('div');right.className='taskdeck-help-description';right.textContent=description;
   grid.append(left,right);
   entries.push({left,right,needle:(key+' '+description).toLocaleLowerCase('vi')});
 }
 section.append(title,grid);scroll.append(section);
 viewSections.push({topic,button,section,entries});
}
scroll.append(empty);layout.append(nav,scroll);dialog.append(header,intro,search,layout);backdrop.append(dialog);document.body.append(backdrop);
function filterHelp(){
 const query=search.value.trim().toLocaleLowerCase('vi');let matched=0;
 const searchEverywhere=Boolean(query);
 const hasExactPhrase=searchEverywhere&&viewSections.some(({entries})=>entries.some(entry=>entry.needle.includes(query)));
 const tokens=query.split(/\s+/).filter(Boolean);
 for(const {topic,button,section,entries} of viewSections){
   let shown=0;
   for(const entry of entries){
     const visible=searchEverywhere
       ?(hasExactPhrase?entry.needle.includes(query):tokens.every(token=>(topic.title.toLocaleLowerCase('vi')+' '+entry.needle).includes(token)))
       :(selectedTopic==='all'||selectedTopic===topic.id);
     entry.left.hidden=!visible;entry.right.hidden=!visible;
     if(visible)shown++;
   }
   section.hidden=shown===0;
   matched+=shown;
   if(button.classList.contains('active'))button.classList.remove('active');
   if(!searchEverywhere&&selectedTopic===topic.id)button.classList.add('active');
   button.setAttribute('aria-current',!searchEverywhere&&selectedTopic===topic.id?'true':'false');
 }
 if(allButton.classList.contains('active'))allButton.classList.remove('active');
 if(!searchEverywhere&&selectedTopic==='all')allButton.classList.add('active');
 allButton.setAttribute('aria-current',!searchEverywhere&&selectedTopic==='all'?'true':'false');
 empty.hidden=matched>0;
}
let previousFocus=null;
function close(){
 if(!backdrop.classList.contains('visible'))return;
 backdrop.classList.remove('visible');search.value='';selectedTopic='getting-started';filterHelp();
 if(previousFocus?.isConnected)previousFocus.focus();
 previousFocus=null;
}
function open(topic=''){
 previousFocus=backdrop.classList.contains('visible')?previousFocus:document.activeElement;
 backdrop.classList.add('visible');
 const requested=String(topic||'').trim().toLocaleLowerCase('vi');
 search.value='';
 const chosen=taskdeckHelpTopics.find(item=>item.id===requested||item.title.toLocaleLowerCase('vi')===requested);
 if(chosen){selectedTopic=chosen.id;}
 else if(requested){selectedTopic='all';search.value=requested;}
 else selectedTopic='getting-started';
 filterHelp();scroll.scrollTop=0;search.focus();return true;
}
dismiss.onclick=close;
search.oninput=()=>{filterHelp();scroll.scrollTop=0;};
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
dialog.addEventListener('keydown',event=>{
 if(event.key==='Escape'){event.preventDefault();event.stopPropagation();close();}
 if(event.key==='Tab'){
   if(event.shiftKey&&document.activeElement===search){event.preventDefault();dismiss.focus();}
   if(!event.shiftKey&&document.activeElement===dismiss){event.preventDefault();search.focus();}
   const lastGroup=viewSections[viewSections.length-1]?.button;
   if(!event.shiftKey&&document.activeElement===lastGroup){event.preventDefault();dismiss.focus();}
 }
});
filterHelp();
globalThis.TaskMenuHelp={open,close,get visible(){return backdrop.classList.contains('visible');},topics:taskdeckHelpTopics};
