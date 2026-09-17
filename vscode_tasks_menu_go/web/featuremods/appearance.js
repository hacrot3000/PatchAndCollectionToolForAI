const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for appearance controls');

const style=document.createElement('style');
style.textContent=`
.appearance-controls{display:flex;align-items:center;gap:4px;white-space:nowrap}.appearance-controls select,.appearance-controls button{padding:5px 7px;font-size:11px}.appearance-font-size{min-width:42px;text-align:center;font:11px ui-monospace,monospace;opacity:.75}
html[data-taskmenu-theme="light"]{color-scheme:light;background:#f5f7fa;color:#202124}
html[data-taskmenu-theme="light"] body{background:#f5f7fa;color:#202124}
html[data-taskmenu-theme="light"] header,html[data-taskmenu-theme="light"] aside,html[data-taskmenu-theme="light"] #tabs,html[data-taskmenu-theme="light"] .pane-head{border-color:#d0d7de}
html[data-taskmenu-theme="light"] button,html[data-taskmenu-theme="light"] select,html[data-taskmenu-theme="light"] input{background:#fff;color:#202124;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .task,html[data-taskmenu-theme="light"] .quick-task-run{background:#f6f8fa}
html[data-taskmenu-theme="light"] .task:hover,html[data-taskmenu-theme="light"] .quick-task-run:hover{background:#eaeef2}
html[data-taskmenu-theme="light"] .terminal{background:#fff}
html[data-taskmenu-theme="light"] .detected-actions,html[data-taskmenu-theme="light"] .console-find,html[data-taskmenu-theme="light"] .quick-task-section,html[data-taskmenu-theme="light"] .history-section,html[data-taskmenu-theme="light"] .task-search-results{background:#f6f8fa;border-color:#d0d7de}
html[data-taskmenu-theme="light"] .tab.active{background:#dde3ea}
html[data-taskmenu-theme="light"] .git-status-pill.clean{background:#e7f5ea;border-color:#7ab487}
html[data-taskmenu-theme="light"] .git-status-pill.dirty{background:#fff3cd;border-color:#c4a75d}
`;
document.head.append(style);

const workspaceKey=name=>'vscode-tasks-menu:'+name+':'+app.taskData.workspace;
const families={default:'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',dejavu:'DejaVu Sans Mono, monospace',liberation:'Liberation Mono, monospace',mono:'monospace'};
function read(name,fallback){try{return localStorage.getItem(workspaceKey(name))||fallback;}catch{return fallback;}}
function write(name,value){try{localStorage.setItem(workspaceKey(name),String(value));}catch(e){console.warn('Cannot persist appearance setting',e);}}

let theme=read('theme','dark');if(theme!=='light')theme='dark';
let fontKey=read('terminal-font','default');if(!families[fontKey])fontKey='default';
let fontSize=Math.max(10,Math.min(24,Number(read('terminal-font-size','13'))||13));

const controls=document.createElement('div');controls.className='appearance-controls';
const themeSelect=document.createElement('select');themeSelect.title='Theme';
for(const [value,label] of [['dark','Dark'],['light','Light']]){const option=document.createElement('option');option.value=value;option.textContent=label;themeSelect.append(option);}themeSelect.value=theme;
const fontSelect=document.createElement('select');fontSelect.title='Terminal font';
for(const [value,label] of [['default','Default Mono'],['dejavu','DejaVu Mono'],['liberation','Liberation Mono'],['mono','Monospace']]){const option=document.createElement('option');option.value=value;option.textContent=label;fontSelect.append(option);}fontSelect.value=fontKey;
const smaller=document.createElement('button');smaller.textContent='A−';smaller.title='Decrease terminal font size';
const sizeLabel=document.createElement('span');sizeLabel.className='appearance-font-size';
const larger=document.createElement('button');larger.textContent='A+';larger.title='Increase terminal font size';
controls.append(themeSelect,fontSelect,smaller,sizeLabel,larger);
document.querySelector('#reload').before(controls);

function terminalTheme(){return theme==='light'?{background:'#ffffff',foreground:'#202124',cursor:'#202124',selectionBackground:'#b6d7ff'}:{background:'#050607',foreground:'#e8eaed',cursor:'#e8eaed'};}
function applyView(view){
  view.term.options.theme=terminalTheme();view.term.options.fontSize=fontSize;view.term.options.fontFamily=families[fontKey];
  try{view.fit.fit();}catch{}
}
function applyAll(){
  document.documentElement.dataset.taskmenuTheme=theme;sizeLabel.textContent=fontSize+'px';
  for(const view of app.views.values())applyView(view);
}
function setTheme(value){theme=value==='light'?'light':'dark';write('theme',theme);applyAll();}
function setFont(value){fontKey=families[value]?value:'default';write('terminal-font',fontKey);applyAll();}
function setFontSize(value){fontSize=Math.max(10,Math.min(24,value));write('terminal-font-size',fontSize);applyAll();}

themeSelect.onchange=()=>setTheme(themeSelect.value);fontSelect.onchange=()=>setFont(fontSelect.value);smaller.onclick=()=>setFontSize(fontSize-1);larger.onclick=()=>setFontSize(fontSize+1);
window.addEventListener('taskmenu:session',event=>{const view=event.detail?.view;if(view)applyView(view);});
applyAll();
