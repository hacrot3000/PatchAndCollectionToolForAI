// Offline, read-only TaskDeck Help: shortcuts reflect handlers in featuremods.
const taskdeckHelpTopics=[
  {title:'General shortcuts',items:[
    ['Ctrl/Cmd + Shift + P','Open the Command Palette to find tools and actions.'],
    ['Ctrl/Cmd + P','Quick Open: find files, tabs, and resources.'],
    ['Ctrl/Cmd + Shift + F','Find and replace text across the project.'],
    ['Ctrl/Cmd + Shift + Backquote','Open a new local terminal.'],
    ['Ctrl + Alt + W','Close the selected TaskDeck tab; a safer Firefox alternative to Ctrl+W.'],
    ['Ctrl/Cmd + W','Close a TaskDeck tab only if the browser forwards the shortcut; Firefox may close the browser tab instead.'],
    ['Esc','Close a menu/dialog; close an eligible Diff, viewer, Patch Tool or never-edited file tab.']
  ]},
  {title:'File Editor',items:[
    ['Ctrl/Cmd + S','Save the active editor file.'],
    ['Ctrl/Cmd + F','Find in the file; Enter/Shift+Enter moves between matches.'],
    ['Ctrl/Cmd + H','Find and replace in the file.'],
    ['Ctrl/Cmd + G','Go to a line number.'],
    ['Ctrl/Cmd + Shift + O','Show the file outline and symbols.'],
    ['Ctrl/Cmd + Shift + T','Reopen the most recently closed file editor.']
  ]},
  {title:'Terminal',items:[
    ['Ctrl + C','Interrupt the command in the focused terminal.'],
    ['Ctrl + F','Find text in terminal output.'],
    ['Tab / ↑ / ↓','Complete or browse shell command history, depending on the shell.']
  ]},
  {title:'Quick start',items:[
    ['Tabs & Split','Right-click a tab for its actions, or a gap in the tab bar to reopen a tab or create a terminal. Split groups share a border color; the active tab is brighter.'],
    ['Diff','Compare two files or branches, choose View diff/context and toggle unimportant changes. Escape closes a never-edited File Diff.'],
    ['Git','Use the Git panel to review changes, stage/commit/push and compare branches. Review diffs before committing.'],
    ['FTP/SFTP & Database','Select a connection profile and verify source/target before transfers or data edits.'],
    ['Patch Tool','Prepare a patch or collection, watch the queue, then inspect results and handoff.']
  ]}
];
const style=document.createElement('style');
style.textContent=[
'.taskdeck-help-backdrop{position:fixed;inset:0;z-index:18000;display:none;align-items:flex-start;justify-content:center;padding:clamp(10px,4vh,40px) 12px;background:rgba(0,0,0,.64)}',
'.taskdeck-help-backdrop.visible{display:flex}',
'.taskdeck-help-dialog{width:min(800px,100%);max-height:91vh;display:flex;flex-direction:column;min-height:0;overflow:hidden;background:#151c26;color:#edf3fb;border:1px solid #56677e;border-radius:10px;box-shadow:0 18px 55px #0009}',
'.taskdeck-help-head{display:flex;gap:10px;align-items:center;padding:11px 14px;border-bottom:1px solid #3c4a5d}',
'.taskdeck-help-head h2{font-size:16px;flex:1;margin:0}.taskdeck-help-head button{font-size:16px;padding:3px 10px;border-radius:5px;border:1px solid #61738e;background:#27364a;color:#f5f9ff}',
'.taskdeck-help-intro{font-size:12px;line-height:1.5;margin:8px 14px;opacity:.84}',
'.taskdeck-help-search{box-sizing:border-box;width:calc(100% - 28px);margin:0 14px 7px;padding:7px 9px;border:1px solid #697b94;border-radius:6px;background:#0d1520;color:#f5faff;font-size:12px}',
'.taskdeck-help-scroll{overflow:auto;flex:1;min-height:0;padding:0 14px 16px}',
'.taskdeck-help-section{margin:11px 0 14px}.taskdeck-help-section h3{margin:0 0 7px;font-size:12px;color:#c3dfff}',
'.taskdeck-help-grid{display:grid;grid-template-columns:minmax(120px,190px) minmax(0,1fr);border:1px solid #3e4a5d;border-radius:6px;overflow:hidden}',
'.taskdeck-help-key,.taskdeck-help-description{font-size:12px;line-height:1.5;padding:7px 9px;border-bottom:1px solid #344052}',
'.taskdeck-help-key{background:#202c3a;color:#e4f0ff;overflow-wrap:anywhere}.taskdeck-help-key kbd{font:600 11px/1.5 ui-monospace,monospace}',
'.taskdeck-help-description{color:#e1eaf4}.taskdeck-help-key[hidden],.taskdeck-help-description[hidden]{display:none!important}',
'.taskdeck-help-empty{font-size:12px;opacity:.7;padding:12px}',
'html[data-taskmenu-theme="light"] .taskdeck-help-dialog{background:#fff;color:#182738;border-color:#bac5d2}',
'html[data-taskmenu-theme="light"] .taskdeck-help-head{border-color:#d4deea}',
'html[data-taskmenu-theme="light"] .taskdeck-help-head button{background:#eaf1f8;color:#1c344b;border-color:#aebcca}',
'html[data-taskmenu-theme="light"] .taskdeck-help-search{background:#f9fbff;color:#172535;border-color:#b5c1d0}',
'html[data-taskmenu-theme="light"] .taskdeck-help-section h3{color:#275a82}',
'html[data-taskmenu-theme="light"] .taskdeck-help-grid{border-color:#ccd6e0}',
'html[data-taskmenu-theme="light"] .taskdeck-help-key{background:#edf3fa;color:#193850}',
'html[data-taskmenu-theme="light"] .taskdeck-help-description{color:#1b2938}',
'html[data-taskmenu-theme="light"] .taskdeck-help-key,html[data-taskmenu-theme="light"] .taskdeck-help-description{border-color:#dce4eb}',
'@media(max-width:560px){.taskdeck-help-grid{grid-template-columns:minmax(98px,135px) minmax(0,1fr)}.taskdeck-help-key,.taskdeck-help-description{font-size:11px;padding:6px}}'
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
intro.textContent='Quick reference for shortcuts and basic workflows. Ctrl/Cmd means Ctrl on Linux/Windows and Cmd on macOS. Browsers may reserve certain shortcuts.';
const search=document.createElement('input');search.type='search';search.className='taskdeck-help-search';search.placeholder='Search shortcuts or topics…';search.setAttribute('aria-label','Find Help topic');
const scroll=document.createElement('div');scroll.className='taskdeck-help-scroll';
const empty=document.createElement('p');empty.className='taskdeck-help-empty';empty.textContent='No matching Help topics.';empty.hidden=true;
const viewSections=[];
for(const topic of taskdeckHelpTopics){
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
  section.append(title,grid);scroll.append(section);viewSections.push({section,entries});
}
scroll.append(empty);dialog.append(header,intro,search,scroll);backdrop.append(dialog);document.body.append(backdrop);
function filterHelp(){
  const query=search.value.trim().toLocaleLowerCase('vi');let matched=0;
  for(const {section,entries} of viewSections){
    let shown=0;
    for(const entry of entries){
      const visible=!query||entry.needle.includes(query);
      entry.left.hidden=!visible;entry.right.hidden=!visible;
      if(visible)shown++;
    }
    section.hidden=shown===0;matched+=shown;
  }
  empty.hidden=matched>0;
}
let previousFocus=null;
function close(){
  if(!backdrop.classList.contains('visible'))return;
  backdrop.classList.remove('visible');search.value='';filterHelp();
  if(previousFocus?.isConnected)previousFocus.focus();
  previousFocus=null;
}
function open(){
  previousFocus=document.activeElement;backdrop.classList.add('visible');
  search.value='';filterHelp();search.focus();return true;
}
dismiss.onclick=close;
search.oninput=filterHelp;
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
dialog.addEventListener('keydown',event=>{
  if(event.key==='Escape'){event.preventDefault();event.stopPropagation();close();}
  if(event.key==='Tab'){
    if(event.shiftKey&&document.activeElement===search){event.preventDefault();dismiss.focus();}
    if(!event.shiftKey&&document.activeElement===dismiss){event.preventDefault();search.focus();}
  }
});
globalThis.TaskMenuHelp={open,close,get visible(){return backdrop.classList.contains('visible');},topics:taskdeckHelpTopics};
