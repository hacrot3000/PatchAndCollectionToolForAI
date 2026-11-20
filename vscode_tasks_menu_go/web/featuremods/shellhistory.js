const app=globalThis.TaskMenuApp;
const integration=globalThis.TaskDeckShellIntegration;
if(!app||!integration)throw new Error('Shell integration unavailable for command history');

const installed=new WeakSet();
const buttons=new WeakMap();
const loaded=new Set();
const loading=new Map();
const notes=new Map();
const saveTimers=new Map();
let activeView=null;

const style=document.createElement('style');
style.textContent=`
.shell-history-backdrop{position:fixed;inset:0;z-index:18200;display:none;align-items:center;justify-content:center;background:rgba(0,0,0,.62);padding:18px}
.shell-history-backdrop.visible{display:flex}
.shell-history-dialog{width:min(900px,96vw);max-height:88vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #4a5362;border-radius:10px;overflow:hidden;box-shadow:0 22px 70px rgba(0,0,0,.6)}
.shell-history-head,.shell-history-foot{display:flex;align-items:center;gap:8px;padding:10px 12px;border-bottom:1px solid #30343b}
.shell-history-foot{border-top:1px solid #30343b;border-bottom:0;justify-content:flex-end}
.shell-history-head strong{flex:1}
.shell-history-list{overflow:auto;padding:10px;display:grid;gap:8px}
.shell-history-empty{opacity:.65;padding:20px;text-align:center}
.shell-history-note,.shell-history-bookmark{border-left:3px solid #d2a84a;background:#211d13;padding:7px 9px;border-radius:4px;white-space:pre-wrap;word-break:break-word;font-size:10px}
.shell-history-bookmark{margin-top:6px}
.shell-history-row{border:1px solid #30343b;border-radius:8px;background:#151b23;padding:9px}
.shell-history-meta{display:flex;gap:8px;align-items:center;font-size:10px;opacity:.72;flex-wrap:wrap}
.shell-history-status{font-weight:800}.shell-history-status.ok{color:#89d185}.shell-history-status.fail{color:#ff8b8b}
.shell-history-command{font:12px/1.45 ui-monospace,monospace;margin-top:5px;white-space:pre-wrap;word-break:break-word}
.shell-history-output{font:10px/1.4 ui-monospace,monospace;margin-top:6px;white-space:pre-wrap;word-break:break-word;max-height:120px;overflow:auto;background:#090d12;border-radius:5px;padding:6px;opacity:.85}
.shell-history-actions{display:flex;gap:6px;margin-top:7px;flex-wrap:wrap}.shell-history-actions button{padding:4px 7px;font-size:10px}
.shell-history-button{white-space:nowrap}
html[data-taskmenu-theme='light'] .shell-history-dialog{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme='light'] .shell-history-row{background:#f8fafc;border-color:#d0d7de}
html[data-taskmenu-theme='light'] .shell-history-output{background:#f3f4f6}
html[data-taskmenu-theme='light'] .shell-history-note,html[data-taskmenu-theme='light'] .shell-history-bookmark{background:#fff8df}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='shell-history-backdrop';
const dialog=document.createElement('div');dialog.className='shell-history-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
const head=document.createElement('div');head.className='shell-history-head';
const title=document.createElement('strong');title.textContent='Terminal command history';
const cwd=document.createElement('span');cwd.style.opacity='.65';cwd.style.fontSize='10px';
const close=document.createElement('button');close.textContent='×';close.title='Close';
head.append(title,cwd,close);
const list=document.createElement('div');list.className='shell-history-list';
const foot=document.createElement('div');foot.className='shell-history-foot';
const sessionNote=document.createElement('button');sessionNote.textContent='Session note…';
const exportMarkdown=document.createElement('button');exportMarkdown.textContent='Export Markdown';
const exportText=document.createElement('button');exportText.textContent='Export text';
const clear=document.createElement('button');clear.textContent='Clear history';
const done=document.createElement('button');done.textContent='Close';
foot.append(sessionNote,exportMarkdown,exportText,clear,done);dialog.append(head,list,foot);backdrop.append(dialog);document.body.append(backdrop);

function copyText(value){
  value=String(value??'');
  if(navigator.clipboard&&window.isSecureContext)return navigator.clipboard.writeText(value);
  const area=document.createElement('textarea');area.value=value;area.setAttribute('readonly','');area.style.position='fixed';area.style.left='-9999px';document.body.append(area);area.select();
  try{if(!document.execCommand('copy'))throw new Error('Copy failed');}finally{area.remove();}
  return Promise.resolve();
}
function historyID(view){return String(view?.meta?.id||'');}
function commandKey(item){return [Number(item?.startedAt||0),Number(item?.finishedAt||0),String(item?.command||''),String(item?.cwd||'')].join('\u001f');}
function decodeCommand(raw){
  raw=raw&&typeof raw==='object'?raw:{};
  const exitRaw=raw.exit_code??raw.exitCode;
  return {
    command:String(raw.command||''),output:String(raw.output||''),
    exitCode:exitRaw===null||exitRaw===undefined?null:Number(exitRaw),
    cwd:String(raw.cwd||''),remote:Boolean(raw.remote),
    startedAt:Number(raw.started_at??raw.startedAt??0),finishedAt:Number(raw.finished_at??raw.finishedAt??0),
    bookmarkLine:Number(raw.bookmark_line??raw.bookmarkLine??0),
    bookmarkText:String(raw.bookmark_text??raw.bookmarkText??'')
  };
}
function encodeCommand(item){
  return {
    command:String(item?.command||''),output:String(item?.output||''),
    exit_code:item?.exitCode===null||item?.exitCode===undefined?null:Number(item.exitCode),
    cwd:String(item?.cwd||''),remote:Boolean(item?.remote),
    started_at:Number(item?.startedAt||0),finished_at:Number(item?.finishedAt||0),
    bookmark_line:Number(item?.bookmarkLine||0),bookmark_text:String(item?.bookmarkText||'')
  };
}
function mergeCommands(persisted,live){
  const out=[],seen=new Set();
  for(const item of [...persisted,...live]){
    const value=decodeCommand(item),key=commandKey(value);
    if(seen.has(key))continue;seen.add(key);out.push(value);
  }
  return out.slice(-Number(integration.maxCommands||200));
}
async function loadHistory(view){
  const id=historyID(view);if(!id)return;
  if(loaded.has(id))return;
  if(loading.has(id))return loading.get(id);
  const promise=(async()=>{
    const data=await app.jsonFetch('/api/terminal-history?session_id='+encodeURIComponent(id));
    const persisted=Array.isArray(data?.commands)?data.commands.map(decodeCommand):[];
    const merged=mergeCommands(persisted,integration.getCommands(id));
    integration.replaceCommands(id,merged);
    notes.set(id,String(data?.note||''));
    loaded.add(id);updateButton(view);
    if(activeView===view&&backdrop.classList.contains('visible'))render();
    if(merged.length!==persisted.length)scheduleSave(view,100);
  })().catch(error=>console.warn('Cannot load terminal command history',error)).finally(()=>loading.delete(id));
  loading.set(id,promise);return promise;
}
async function persistHistory(view){
  const id=historyID(view);if(!id||!loaded.has(id))return;
  const commands=integration.getCommands(id).map(encodeCommand);
  await app.jsonFetch('/api/terminal-history?session_id='+encodeURIComponent(id),{
    method:'PUT',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({commands,note:notes.get(id)||''})
  });
}
function scheduleSave(view,delay=350){
  const id=historyID(view);if(!id)return;
  const old=saveTimers.get(id);if(old)clearTimeout(old);
  saveTimers.set(id,setTimeout(()=>{saveTimers.delete(id);persistHistory(view).catch(error=>console.warn('Cannot save terminal command history',error));},delay));
}
function closeDialog(){backdrop.classList.remove('visible');activeView=null;}
close.onclick=closeDialog;done.onclick=closeDialog;backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)closeDialog();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))closeDialog();});

function durationText(item){const ms=Math.max(0,Number(item.finishedAt||0)-Number(item.startedAt||0));return ms<1000?ms+' ms':(ms/1000).toFixed(ms<10000?1:0)+' s';}
function exportFilename(view,extension){
  const raw=String(view?.meta?.title||view?.meta?.label||'terminal-session').trim()||'terminal-session';
  return raw.replace(/[^A-Za-z0-9._-]+/g,'_').replace(/^_+|_+$/g,'').slice(0,80)+'-'+new Date().toISOString().replace(/[:.]/g,'-')+'.'+extension;
}
function downloadTextFile(name,text,type){
  const blob=new Blob([text],{type:type+';charset=utf-8'}),url=URL.createObjectURL(blob),a=document.createElement('a');
  a.href=url;a.download=name;document.body.append(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(url),0);
}
function markdownFence(value){
  const runs=String(value||'').match(/`+/g)||[];let size=3;
  for(const run of runs)size=Math.max(size,run.length+1);
  return '`'.repeat(size);
}
function sessionExport(view,markdown){
  const id=historyID(view),commands=integration.getCommands(id),note=notes.get(id)||'',titleText=String(view?.meta?.title||view?.meta?.label||'Terminal session');
  const sessionCwd=integration.getState(id)?.cwd||view?.meta?.cwd||'';
  if(markdown){
    const out=['# '+titleText,'','- Session ID: `'+id.replace(/`/g,'')+'`','- CWD: `'+String(sessionCwd).replace(/`/g,'')+'`','- Exported: '+new Date().toISOString()];
    if(note)out.push('','## Session note','',note);
    commands.forEach((item,index)=>{
      out.push('','## '+(index+1)+'. Command','', '- Exit: '+(item.exitCode==null?'?':item.exitCode),'- Duration: '+durationText(item),'- CWD: `'+String(item.cwd||'').replace(/`/g,'')+'`');
      const commandFence=markdownFence(item.command);out.push('',commandFence+'sh',String(item.command||''),commandFence);
      if(item.output){const outputFence=markdownFence(item.output);out.push('','### Output','',outputFence+'text',String(item.output),outputFence);}
      if(item.bookmarkLine>0)out.push('','> Bookmark: output line '+item.bookmarkLine+(item.bookmarkText?' — '+item.bookmarkText:''));
    });
    return out.join('\n')+'\n';
  }
  const out=['Terminal session: '+titleText,'Session ID: '+id,'CWD: '+sessionCwd,'Exported: '+new Date().toISOString()];
  if(note)out.push('','SESSION NOTE',note);
  commands.forEach((item,index)=>{
    out.push('','='.repeat(72),'COMMAND '+(index+1),'Exit: '+(item.exitCode==null?'?':item.exitCode),'Duration: '+durationText(item),'CWD: '+String(item.cwd||''),'',String(item.command||''));
    if(item.output)out.push('','OUTPUT',String(item.output));
    if(item.bookmarkLine>0)out.push('','BOOKMARK: output line '+item.bookmarkLine+(item.bookmarkText?' — '+item.bookmarkText:''));
  });
  return out.join('\n')+'\n';
}
function editSessionNote(view){
  const id=historyID(view),current=notes.get(id)||'';
  const value=window.prompt('Session note. Leave empty to clear:',current);if(value===null)return;
  notes.set(id,String(value));scheduleSave(view,0);render();
}
function bookmarkOutputLine(view,item){
  const lines=String(item?.output||'').split('\n');if(!item?.output||!lines.length)throw new Error('This command has no recorded output');
  const raw=window.prompt('Bookmark output line (1-'+lines.length+'). Enter 0 to clear:',String(item.bookmarkLine||1));if(raw===null)return;
  const line=Number.parseInt(String(raw).trim(),10);
  if(line===0){item.bookmarkLine=0;item.bookmarkText='';scheduleSave(view,0);render();return;}
  if(!Number.isInteger(line)||line<1||line>lines.length)throw new Error('Bookmark line must be between 1 and '+lines.length);
  item.bookmarkLine=line;item.bookmarkText=lines[line-1].slice(0,4096);scheduleSave(view,0);render();
}
function rerun(view,item){
  const command=String(item?.command||'').trim();if(!command)throw new Error('Command is empty');
  if(!view?.canControl||view?.tabReadOnly)throw new Error('Terminal is read-only');
  if(!view.ws||view.ws.readyState!==WebSocket.OPEN)throw new Error('Terminal connection is not open');
  if(/[\r\n]/.test(command)&&!window.confirm('This command contains multiple lines. Run it again in the terminal?'))return false;
  view.ws.send(command+'\r');view.term.focus();return true;
}
function render(){
  list.replaceChildren();if(!activeView)return;
  const state=integration.getState(activeView.meta.id);cwd.textContent=state?.cwd||activeView.meta?.cwd||'';
  const note=notes.get(historyID(activeView))||'';
  if(note){const noteEl=document.createElement('div');noteEl.className='shell-history-note';noteEl.textContent='Session note: '+note;list.append(noteEl);}
  const commands=integration.getCommands(activeView.meta.id).slice().reverse();
  if(!commands.length){const empty=document.createElement('div');empty.className='shell-history-empty';empty.textContent='No OSC 133 command records yet for this terminal.';list.append(empty);return;}
  for(const item of commands.slice(0,100)){
    const row=document.createElement('div');row.className='shell-history-row';
    const meta=document.createElement('div');meta.className='shell-history-meta';
    const status=document.createElement('span');status.className='shell-history-status '+(item.exitCode===0?'ok':item.exitCode==null?'':'fail');status.textContent=item.exitCode==null?'exit ?':'exit '+item.exitCode;
    const duration=document.createElement('span');duration.textContent=durationText(item);
    const path=document.createElement('span');path.textContent=item.cwd||'';meta.append(status,duration,path);
    const command=document.createElement('div');command.className='shell-history-command';command.textContent=item.command||'(blank command)';
    row.append(meta,command);
    if(item.output){const output=document.createElement('div');output.className='shell-history-output';output.textContent=item.output;row.append(output);}
    if(item.bookmarkLine>0){const bookmark=document.createElement('div');bookmark.className='shell-history-bookmark';bookmark.textContent='🔖 Output line '+item.bookmarkLine+(item.bookmarkText?' — '+item.bookmarkText:'');row.append(bookmark);}
    const actions=document.createElement('div');actions.className='shell-history-actions';
    const copyCommand=document.createElement('button');copyCommand.textContent='Copy command';copyCommand.onclick=()=>copyText(item.command).catch(app.showError);
    const copyOutput=document.createElement('button');copyOutput.textContent='Copy output';copyOutput.disabled=!item.output;copyOutput.onclick=()=>copyText(item.output).catch(app.showError);
    const bookmark=document.createElement('button');bookmark.textContent=item.bookmarkLine>0?'Change bookmark…':'Bookmark output line…';bookmark.disabled=!item.output||!loaded.has(historyID(activeView));bookmark.onclick=()=>{try{bookmarkOutputLine(activeView,item);}catch(error){app.showError(error);}};
    const run=document.createElement('button');run.textContent='Rerun';run.disabled=!activeView.canControl||activeView.tabReadOnly;run.onclick=()=>{try{if(rerun(activeView,item))closeDialog();}catch(error){app.showError(error);}};
    actions.append(copyCommand,copyOutput,bookmark,run);row.append(actions);list.append(row);
  }
}
function open(view){activeView=view;loadHistory(view).finally(()=>{if(activeView===view)render();});render();backdrop.classList.add('visible');close.focus();}
function updateButton(view){const button=buttons.get(view);if(!button)return;const count=integration.getCommands(view.meta.id).length;button.textContent=count?'Commands ('+count+')':'Commands';button.title='Show OSC 133 command history';}
function install(view){
  if(!view?.pane||installed.has(view))return;installed.add(view);
  const head=view.pane.querySelector('.pane-head');if(!head)return;
  const button=document.createElement('button');button.className='shell-history-button';button.textContent='Commands';button.title='Show OSC 133 command history';button.onclick=()=>open(view);
  const copy=view.copy||head.querySelector('.copy-console');if(copy)head.insertBefore(button,copy);else head.append(button);buttons.set(view,button);updateButton(view);loadHistory(view);
}
function installAll(){for(const view of app.views.values())install(view);}
installAll();
window.addEventListener('taskmenu:session',event=>install(event.detail?.view));
window.addEventListener('taskmenu:shell-integration',event=>{const view=event.detail?.view;if(view){install(view);updateButton(view);if(event.detail?.type==='command-finished')loadHistory(view).then(()=>scheduleSave(view));if(activeView===view&&backdrop.classList.contains('visible'))render();}});
sessionNote.onclick=()=>{const view=activeView;if(!view)return;loadHistory(view).then(()=>{if(activeView===view)editSessionNote(view);});};
exportMarkdown.onclick=()=>{const view=activeView;if(!view)return;loadHistory(view).then(()=>downloadTextFile(exportFilename(view,'md'),sessionExport(view,true),'text/markdown'));};
exportText.onclick=()=>{const view=activeView;if(!view)return;loadHistory(view).then(()=>downloadTextFile(exportFilename(view,'txt'),sessionExport(view,false),'text/plain'));};
clear.onclick=()=>{const view=activeView;if(!view)return;if(!window.confirm('Clear recorded command history for this terminal? The session note is kept.'))return;loadHistory(view).then(()=>{const state=integration.getState(view.meta.id);if(state)state.commands.splice(0);updateButton(view);if(activeView===view)render();scheduleSave(view,0);});};

globalThis.TaskDeckShellHistory={open,rerun};
