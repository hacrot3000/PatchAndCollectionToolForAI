const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for project notes');

// Data belongs to the active TaskDeck project, not browser localStorage.
const endpoint='/api/project-notes';
let notes=[];
let visible=false;
let editingID='';
let lastFocus=null;

const style=document.createElement('style');
style.textContent=`
.task-project-notes-backdrop{position:fixed;inset:0;z-index:19400;display:none;align-items:center;justify-content:center;padding:12px;background:rgba(0,0,0,.68)}
.task-project-notes-backdrop.visible{display:flex}
.task-project-notes-panel{display:flex;flex-direction:column;width:min(980px,98vw);max-height:92dvh;min-height:300px;overflow:hidden;background:#111820;color:#e5ebf2;border:1px solid #465262;border-radius:12px;box-shadow:0 24px 80px rgba(0,0,0,.55)}
.task-project-notes-header{display:flex;align-items:center;gap:10px;padding:12px 15px;border-bottom:1px solid #343d48}
.task-project-notes-heading{min-width:0;flex:1}.task-project-notes-heading strong{display:block;font-size:15px}.task-project-notes-heading small{font-size:11px;opacity:.62}
.task-project-notes-toolbar{display:flex;align-items:center;flex-wrap:wrap;gap:8px;padding:10px 15px;border-bottom:1px solid #343d48}
.task-project-notes-toolbar input{flex:2 1 210px;min-width:100px}
.task-project-notes-toolbar select{flex:1 1 160px;min-width:130px}
.task-project-notes-toolbar input,.task-project-notes-toolbar select,.task-project-notes-editor input,.task-project-notes-editor textarea{box-sizing:border-box;background:#0b1017;color:inherit;border:1px solid #49576a;border-radius:6px;padding:8px 10px;outline-offset:2px}
.task-project-notes-toolbar button,.task-project-notes-editor button,.task-project-notes-row-actions button{cursor:pointer}
.task-project-notes-list{display:grid;align-content:start;gap:9px;overflow-y:auto;min-height:100px;padding:12px 15px}
.task-project-notes-row{display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:start;gap:10px;padding:11px 12px;border:1px solid #35404b;border-radius:8px;background:#19222d}
.task-project-notes-info{min-width:0}.task-project-notes-title{font-weight:700;font-size:13px;overflow-wrap:anywhere}.task-project-notes-category{display:inline-block;margin:0 0 5px;padding:2px 7px;border:1px solid #5b738b;border-radius:16px;color:#b8d8fc;font-size:10px}
.task-project-notes-preview{font-family:ui-monospace,monospace;font-size:11px;color:#b1bcca;white-space:pre-wrap;overflow-wrap:anywhere;max-height:4.5em;overflow:hidden;margin-top:5px}
.task-project-notes-meta{opacity:.55;font-size:10px;margin-top:6px}
.task-project-notes-row-actions{display:flex;gap:6px;justify-content:flex-end;flex-wrap:wrap}.task-project-notes-row-actions button{font-size:11px}
.task-project-notes-empty{padding:28px 12px;text-align:center;opacity:.65}
.task-project-notes-editor{display:grid;gap:10px;padding:12px 15px;overflow:auto;border-bottom:1px solid #3c4654;background:#172330}
.task-project-notes-editor[hidden]{display:none}
.task-project-notes-fields{display:grid;grid-template-columns:minmax(150px,1fr) minmax(0,2fr);gap:9px}
.task-project-notes-editor label{display:grid;gap:5px;font-size:11px;font-weight:600}
.task-project-notes-editor textarea{width:100%;min-height:180px;max-height:45vh;resize:vertical;line-height:1.5;font:12px/1.5 ui-monospace,monospace;white-space:pre;tab-size:2}
.task-project-notes-editor-actions{display:flex;justify-content:flex-end;gap:8px}
.task-project-notes-footer{display:flex;justify-content:space-between;align-items:center;gap:8px;padding:8px 15px;border-top:1px solid #343d48;font-size:11px;color:#a9b9c8}
.task-project-notes-panel button:disabled{opacity:.45;cursor:not-allowed}
html[data-taskmenu-theme="light"] .task-project-notes-panel{background:#fff;color:#253446;border-color:#c7d1dd}
html[data-taskmenu-theme="light"] .task-project-notes-toolbar,html[data-taskmenu-theme="light"] .task-project-notes-header,html[data-taskmenu-theme="light"] .task-project-notes-footer{border-color:#d1d9e3}
html[data-taskmenu-theme="light"] .task-project-notes-row{background:#f6f9fc;border-color:#d0d7e0}
html[data-taskmenu-theme="light"] .task-project-notes-editor{background:#f0f5fa;border-color:#d0d7e0}
html[data-taskmenu-theme="light"] .task-project-notes-toolbar input,html[data-taskmenu-theme="light"] .task-project-notes-toolbar select,html[data-taskmenu-theme="light"] .task-project-notes-editor input,html[data-taskmenu-theme="light"] .task-project-notes-editor textarea{background:#fff;color:#253446;border-color:#aebccd}
html[data-taskmenu-theme="light"] .task-project-notes-preview{color:#526170}
html[data-taskmenu-theme="light"] .task-project-notes-category{color:#286095}
@media(max-width:640px){.task-project-notes-backdrop{padding:4px}.task-project-notes-panel{max-height:calc(100dvh - 8px);width:100%;border-radius:8px}.task-project-notes-fields{grid-template-columns:1fr}.task-project-notes-row{grid-template-columns:1fr}.task-project-notes-row-actions{justify-content:flex-start}}
`;
document.head.append(style);

function element(tag,className='',label=''){
  const node=document.createElement(tag);
  if(className)node.className=className;
  if(label)node.textContent=label;
  return node;
}
function makeButton(label,action){
  const button=element('button','',label);
  button.type='button';
  button.onclick=()=>{try{Promise.resolve(action()).catch(showError);}catch(error){showError(error);}};
  return button;
}
function showError(error){if(app.showError)app.showError(error);else console.error(error);}
function canRead(){return !app.sharedMode||Boolean(app.hasPermission?.('settings.read')||app.hasPermission?.('project.admin'));}
function canWrite(){return !app.sharedMode||Boolean(app.hasPermission?.('settings.write')||app.hasPermission?.('project.admin'));}
const backdrop=element('div','task-project-notes-backdrop');
const panel=element('section','task-project-notes-panel');
panel.setAttribute('role','dialog');
panel.setAttribute('aria-modal','true');
panel.setAttribute('aria-label','Project notes');
const header=element('div','task-project-notes-header');
const heading=element('div','task-project-notes-heading');
heading.append(element('strong','','Project notes'),element('small','','Project-specific prompts, templates and reminders'));
const closeButton=makeButton('×',close);
closeButton.title='Close project notes';
closeButton.setAttribute('aria-label','Close project notes');
header.append(heading,closeButton);
const toolbar=element('div','task-project-notes-toolbar');
const search=element('input');
search.type='search';search.placeholder='Search notes or prompt content';search.setAttribute('aria-label','Search project notes');
const categories=element('select');categories.setAttribute('aria-label','Filter by category');
const addButton=makeButton('+ New note',()=>edit(null));
toolbar.append(search,categories,addButton);
const editor=element('form','task-project-notes-editor');
editor.hidden=true;
const fields=element('div','task-project-notes-fields');
function field(labelText,control){
  const wrapper=element('label');
  wrapper.append(element('span','',labelText),control);
  return wrapper;
}
const categoryInput=element('input');categoryInput.type='text';categoryInput.maxLength=80;categoryInput.placeholder='Prompt / Project note / Other';
const titleInput=element('input');titleInput.type='text';titleInput.maxLength=180;titleInput.required=true;titleInput.placeholder='A short descriptive title';
fields.append(field('Category (optional)',categoryInput),field('Title',titleInput));
const contentInput=element('textarea');contentInput.maxLength=131072;contentInput.required=true;contentInput.placeholder='Write your prompt or project notes here…';
contentInput.spellcheck=false;
const editorActions=element('div','task-project-notes-editor-actions');
const cancelButton=makeButton('Cancel',cancelEdit);
const saveButton=element('button','','Save note');saveButton.type='submit';
editorActions.append(cancelButton,saveButton);
editor.append(fields,field('Content (copied exactly as written)',contentInput),editorActions);
const list=element('div','task-project-notes-list');
const footer=element('div','task-project-notes-footer');
const count=element('span');
const status=element('span');status.setAttribute('role','status');status.setAttribute('aria-live','polite');
footer.append(count,status);
panel.append(header,toolbar,editor,list,footer);backdrop.append(panel);document.body.append(backdrop);

function close(){
  visible=false;backdrop.classList.remove('visible');cancelEdit();
  lastFocus?.focus?.();lastFocus=null;
}
function cancelEdit(){
  editingID='';editor.hidden=true;editor.reset();status.textContent='';
}
function edit(note){
  if(!canWrite())throw new Error('Project note write permission is required');
  editingID=String(note?.id||'');
  categoryInput.value=String(note?.category||'');
  titleInput.value=String(note?.title||'');
  contentInput.value=String(note?.content||'');
  editor.hidden=false;status.textContent=editingID?'Editing note':'Creating new note';
  titleInput.focus();
}
function fillCategories(){
  const selected=categories.value;
  categories.replaceChildren();
  const all=element('option','','All categories');all.value='';categories.append(all);
  const values=[...new Set(notes.map(item=>String(item.category||'').trim()).filter(Boolean))];
  values.sort((a,b)=>a.localeCompare(b));
  for(const value of values){const option=element('option','',value);option.value=value;categories.append(option);}
  if(values.includes(selected))categories.value=selected;
}
function matchingNotes(){
  const query=search.value.trim().toLocaleLowerCase();
  const category=categories.value;
  return notes.filter(note=>{
    if(category&&note.category!==category)return false;
    return !query||[note.title,note.category,note.content].some(text=>String(text||'').toLocaleLowerCase().includes(query));
  });
}
function render(){
  list.replaceChildren();
  addButton.disabled=!canWrite();
  const filtered=matchingNotes();
  count.textContent=filtered.length+' / '+notes.length+' notes';
  if(!filtered.length){
    list.append(element('div','task-project-notes-empty',notes.length?'No matching notes.':'No notes in this project. Create a prompt template or project reminder.'));
    return;
  }
  for(const note of filtered){
    const row=element('article','task-project-notes-row');
    const info=element('div','task-project-notes-info');
    if(note.category)info.append(element('span','task-project-notes-category',String(note.category)));
    info.append(element('div','task-project-notes-title',String(note.title||'')));
    info.append(element('div','task-project-notes-preview',String(note.content||'')));
    const meta=element('div','task-project-notes-meta');
    const at=Date.parse(String(note.updated_at||''));
    meta.textContent=Number.isFinite(at)?'Updated '+new Date(at).toLocaleString():'';
    info.append(meta);
    const actions=element('div','task-project-notes-row-actions');
    if(canWrite())actions.append(makeButton('Edit',()=>edit(note)));
    actions.append(makeButton('Copy',()=>copyNote(note)));
    if(canWrite()){
      const remove=makeButton('Delete',()=>deleteNote(note));
      remove.classList.add('task-connection-danger');
      actions.append(remove);
    }
    row.append(info,actions);list.append(row);
  }
}
async function refresh(){
  if(!canRead())throw new Error('Project note read permission is required');
  const response=await app.jsonFetch(endpoint,{cache:'no-store'});
  notes=Array.isArray(response?.notes)?response.notes:[];
  fillCategories();render();return notes;
}
async function saveNote(event){
  event.preventDefault();
  if(!canWrite())throw new Error('Project note write permission is required');
  const title=titleInput.value.trim(),content=contentInput.value;
  if(!title||!content.trim())throw new Error('Title and content are required');
  const id=editingID,body={category:categoryInput.value.trim(),title,content};
  if(id)body.id=id;
  saveButton.disabled=true;
  try{
    await app.jsonFetch(endpoint,{method:id?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
    await refresh();cancelEdit();status.textContent=id?'Note updated':'Note created';
  }finally{saveButton.disabled=false;}
}
async function deleteNote(note){
  if(!canWrite())throw new Error('Project note write permission is required');
  if(!confirm('Delete note "'+String(note.title||'')+'"? This cannot be undone.'))return;
  await app.jsonFetch(endpoint+'?id='+encodeURIComponent(String(note.id||'')),{method:'DELETE'});
  if(editingID===note.id)cancelEdit();
  await refresh();status.textContent='Note deleted';
}
async function copyNote(note){
  const content=String(note?.content||'');
  if(navigator.clipboard?.writeText){
    await navigator.clipboard.writeText(content);
  }else{
    const helper=element('textarea');
    helper.value=content;helper.style.position='fixed';helper.style.opacity='0';
    document.body.append(helper);helper.focus();helper.select();
    let success=false;
    try{success=document.execCommand('copy');}finally{helper.remove();}
    if(!success)throw new Error('Clipboard access is unavailable in this browser');
  }
  status.textContent='Copied content: '+String(note?.title||'');
}
async function open(){
  if(!canRead())throw new Error('Project note read permission is required');
  if(!visible)lastFocus=document.activeElement;
  visible=true;backdrop.classList.add('visible');
  try{await refresh();search.focus();}
  catch(error){if(visible)status.textContent='Could not load notes';throw error;}
}
search.addEventListener('input',render);
categories.addEventListener('change',render);
editor.addEventListener('submit',event=>{saveNote(event).catch(showError);});
backdrop.addEventListener('pointerdown',event=>{if(event.target===backdrop)close();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&visible){event.preventDefault();close();}});
globalThis.TaskMenuProjectNotes={open,close,refresh,get items(){return notes;}};
