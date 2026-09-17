const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for upload');

const style=document.createElement('style');
style.textContent=`
#upload-workspace{white-space:nowrap}
.upload-drop-overlay{position:fixed;inset:0;z-index:9999;display:none;align-items:center;justify-content:center;background:rgba(5,6,7,.82);font-size:24px;font-weight:600;border:3px dashed #5a88b4;pointer-events:none}
.upload-drop-overlay.visible{display:flex}
`;
document.head.append(style);

const dirKey=()=>`vscode-tasks-menu:upload-dir:${app.taskData.workspace}`;
function lastDir(){try{return localStorage.getItem(dirKey())||'.';}catch{return '.';}}
function saveDir(value){try{localStorage.setItem(dirKey(),value);}catch{}}

let input=null,button=null,overlay=null,dragDepth=0;
function installUploadUI(){
  if(button?.isConnected)return;
  const header=document.querySelector('header');if(!header)return;
  input=document.createElement('input');input.type='file';input.multiple=true;input.hidden=true;input.onchange=()=>{const files=[...(input.files||[])];input.value='';if(files.length)chooseDestinationAndUpload(files);};document.body.append(input);
  button=document.createElement('button');button.id='upload-workspace';button.textContent='Upload';button.title='Upload file vào workspace';button.onclick=()=>input.click();
  const reload=document.querySelector('#reload');if(reload)header.insertBefore(button,reload);else header.append(button);
  overlay=document.createElement('div');overlay.className='upload-drop-overlay';overlay.textContent='Drop files to upload into workspace';document.body.append(overlay);
}

async function chooseDestinationAndUpload(files){
  const value=window.prompt('Thư mục đích tương đối trong workspace:',lastDir());if(value===null)return;
  const dir=(value.trim()||'.');saveDir(dir);
  button.disabled=true;const old=button.textContent;button.textContent=`Upload 0/${files.length}`;
  try{
    for(let i=0;i<files.length;i++){
      button.textContent=`Upload ${i+1}/${files.length}`;
      await uploadOne(files[i],dir,false);
    }
    button.textContent='Uploaded';setTimeout(()=>{if(button?.isConnected)button.textContent=old;},1200);
  }finally{button.disabled=false;if(button.textContent.startsWith('Upload '))button.textContent=old;}
}

async function uploadOne(file,dir,overwrite){
  const form=new FormData();form.append('dir',dir);form.append('file',file,file.name);
  const url='/api/files/upload'+(overwrite?'?overwrite=1':'');
  const response=await fetch(url,{method:'POST',body:form,cache:'no-store'});
  if(response.status===409&&!overwrite){
    const message=(await response.text()).trim();
    if(message.includes('already exists')&&window.confirm(`${file.name} đã tồn tại. Ghi đè?`))return uploadOne(file,dir,true);
    throw new Error(message||'Upload conflict');
  }
  if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
  return response.json();
}

function hasFiles(event){return [...(event.dataTransfer?.types||[])].includes('Files');}
document.addEventListener('dragenter',event=>{if(!hasFiles(event))return;event.preventDefault();dragDepth++;overlay?.classList.add('visible');});
document.addEventListener('dragover',event=>{if(!hasFiles(event))return;event.preventDefault();if(event.dataTransfer)event.dataTransfer.dropEffect='copy';});
document.addEventListener('dragleave',event=>{if(!hasFiles(event))return;dragDepth=Math.max(0,dragDepth-1);if(!dragDepth)overlay?.classList.remove('visible');});
document.addEventListener('drop',event=>{
  if(!hasFiles(event))return;event.preventDefault();dragDepth=0;overlay?.classList.remove('visible');const files=[...(event.dataTransfer?.files||[])].filter(file=>file.size>=0);if(files.length)chooseDestinationAndUpload(files).catch(app.showError);
});

installUploadUI();
