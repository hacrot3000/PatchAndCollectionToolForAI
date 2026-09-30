const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for file preview');

const style=document.createElement('style');
style.textContent=`
.file-preview-backdrop{display:none;position:fixed;inset:0;z-index:2600;background:rgba(0,0,0,.72);align-items:center;justify-content:center;padding:28px}
.file-preview-backdrop.visible{display:flex}
.file-preview-dialog{width:min(1100px,96vw);height:min(820px,90vh);display:flex;flex-direction:column;border:1px solid #4a5362;border-radius:10px;background:#11161d;box-shadow:0 20px 60px rgba(0,0,0,.58);overflow:hidden}
.file-preview-head{display:flex;align-items:center;gap:8px;padding:8px 10px;border-bottom:1px solid #30343b}
.file-preview-title{font:12px ui-monospace,monospace;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.file-preview-meta{font-size:10px;opacity:.65;white-space:nowrap}
.file-preview-close{padding:4px 8px}
.file-preview-body{flex:1;min-height:0;display:flex;align-items:center;justify-content:center;overflow:auto;padding:12px;background:#080a0d}
.file-preview-image{display:block;max-width:100%;max-height:100%;object-fit:contain}
.file-preview-error{padding:14px;color:#ffb4b4}
html[data-taskmenu-theme="light"] .file-preview-dialog{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .file-preview-body{background:#eef1f5}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='file-preview-backdrop';
const dialog=document.createElement('div');dialog.className='file-preview-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label','Image preview');
const head=document.createElement('div');head.className='file-preview-head';
const title=document.createElement('div');title.className='file-preview-title';
const meta=document.createElement('div');meta.className='file-preview-meta';
const closeButton=document.createElement('button');closeButton.className='file-preview-close';closeButton.type='button';closeButton.textContent='Close';
const body=document.createElement('div');body.className='file-preview-body';
head.append(title,meta,closeButton);dialog.append(head,body);backdrop.append(dialog);document.body.append(backdrop);

function formatBytes(value){
  const n=Number(value)||0;
  if(n<1024)return n+' B';
  if(n<1024*1024)return (n/1024).toFixed(1)+' KiB';
  return (n/(1024*1024)).toFixed(1)+' MiB';
}
function close(){
  backdrop.classList.remove('visible');
  body.replaceChildren();
}
function open(info){
  if(!info||info.kind!=='image'||!info.url)return;
  title.textContent=info.path||info.name||'Image';
  title.title=info.path||'';
  meta.textContent=[info.content_type||'image',formatBytes(info.size)].filter(Boolean).join(' · ');
  body.replaceChildren();
  const image=document.createElement('img');image.className='file-preview-image';image.alt=info.name||'Image preview';
  image.onload=()=>{meta.textContent=[info.content_type||'image',formatBytes(info.size),image.naturalWidth+'×'+image.naturalHeight].join(' · ');};
  image.onerror=()=>{body.replaceChildren();const error=document.createElement('div');error.className='file-preview-error';error.textContent='Image preview failed or the file changed.';body.append(error);};
  image.src=info.url;
  body.append(image);
  backdrop.classList.add('visible');
  closeButton.focus();
}
closeButton.onclick=close;
backdrop.onmousedown=event=>{if(event.target===backdrop)close();};
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible')){event.preventDefault();close();}},true);
window.addEventListener('taskmenu:file-image-preview',event=>open(event.detail));

globalThis.TaskMenuFilePreview={open,close};
