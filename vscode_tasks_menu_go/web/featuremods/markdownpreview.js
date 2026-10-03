const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Markdown preview');

const style=document.createElement('style');
style.textContent=`
.markdown-preview-backdrop{display:none;position:fixed;inset:0;z-index:2620;background:rgba(0,0,0,.72);align-items:center;justify-content:center;padding:28px}
.markdown-preview-backdrop.visible{display:flex}
.markdown-preview-dialog{width:min(1100px,96vw);height:min(860px,92vh);display:flex;flex-direction:column;border:1px solid #4a5362;border-radius:10px;background:#11161d;box-shadow:0 20px 60px rgba(0,0,0,.58);overflow:hidden}
.markdown-preview-head{display:flex;align-items:center;gap:8px;padding:8px 10px;border-bottom:1px solid #30343b}
.markdown-preview-title{font:12px ui-monospace,monospace;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.markdown-preview-meta{font-size:10px;opacity:.65;white-space:nowrap}
.markdown-preview-body{flex:1;min-height:0;overflow:auto;background:#0d1117}
.markdown-preview-doc{max-width:980px;margin:0 auto;padding:28px 34px 56px;font:15px/1.65 system-ui,sans-serif;color:#dfe5ec;overflow-wrap:anywhere}
.markdown-preview-doc h1,.markdown-preview-doc h2,.markdown-preview-doc h3,.markdown-preview-doc h4,.markdown-preview-doc h5,.markdown-preview-doc h6{line-height:1.25;margin:1.4em 0 .55em;color:#f2f5f8}
.markdown-preview-doc h1,.markdown-preview-doc h2{padding-bottom:.28em;border-bottom:1px solid #303843}.markdown-preview-doc h1{font-size:2em}.markdown-preview-doc h2{font-size:1.55em}.markdown-preview-doc h3{font-size:1.28em}
.markdown-preview-doc p{margin:.75em 0}.markdown-preview-doc ul,.markdown-preview-doc ol{padding-left:2em}.markdown-preview-doc li{margin:.24em 0}
.markdown-preview-doc blockquote{margin:1em 0;padding:.15em 1em;border-left:4px solid #52606d;color:#aeb8c3;background:#111820;white-space:pre-wrap}
.markdown-preview-doc pre{overflow:auto;padding:14px;border:1px solid #303843;border-radius:7px;background:#070a0e;font:13px/1.5 ui-monospace,SFMono-Regular,Consolas,monospace;tab-size:4}
.markdown-preview-doc code{padding:.12em .32em;border-radius:4px;background:#1a212a;font:13px ui-monospace,SFMono-Regular,Consolas,monospace}.markdown-preview-doc pre code{padding:0;background:transparent}
.markdown-preview-doc table{width:100%;border-collapse:collapse;margin:1em 0;display:block;overflow:auto}.markdown-preview-doc th,.markdown-preview-doc td{border:1px solid #35404d;padding:6px 10px;text-align:left}.markdown-preview-doc th{background:#171f28}
.markdown-preview-doc hr{border:0;border-top:1px solid #35404d;margin:1.7em 0}.markdown-preview-doc a{color:#71b7ff}
html[data-taskmenu-theme="light"] .markdown-preview-dialog{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .markdown-preview-body{background:#fff}.markdown-preview-close{padding:4px 8px}
html[data-taskmenu-theme="light"] .markdown-preview-doc{color:#24292f}html[data-taskmenu-theme="light"] .markdown-preview-doc h1,html[data-taskmenu-theme="light"] .markdown-preview-doc h2,html[data-taskmenu-theme="light"] .markdown-preview-doc h3,html[data-taskmenu-theme="light"] .markdown-preview-doc h4,html[data-taskmenu-theme="light"] .markdown-preview-doc h5,html[data-taskmenu-theme="light"] .markdown-preview-doc h6{color:#1f2328}
html[data-taskmenu-theme="light"] .markdown-preview-doc pre{background:#f6f8fa;border-color:#d0d7de}html[data-taskmenu-theme="light"] .markdown-preview-doc code{background:#eff1f3}html[data-taskmenu-theme="light"] .markdown-preview-doc pre code{background:transparent}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='markdown-preview-backdrop';
const dialog=document.createElement('div');dialog.className='markdown-preview-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label','Markdown preview');
const head=document.createElement('div');head.className='markdown-preview-head';
const title=document.createElement('div');title.className='markdown-preview-title';
const meta=document.createElement('div');meta.className='markdown-preview-meta';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.className='markdown-preview-close';closeButton.textContent='Close';
const body=document.createElement('div');body.className='markdown-preview-body';
head.append(title,meta,closeButton);dialog.append(head,body);backdrop.append(dialog);document.body.append(backdrop);

function formatBytes(value){
  const n=Number(value)||0;
  if(n<1024)return n+' B';
  if(n<1024*1024)return (n/1024).toFixed(1)+' KiB';
  return (n/(1024*1024)).toFixed(1)+' MiB';
}

function appendInline(host,text){
  text=String(text||'');
  const pattern=/(`[^`\n]+`|\*\*[^*\n]+\*\*|__[^_\n]+__|\*[^*\n]+\*|_[^_\n]+_|\[[^\]\n]+\]\([^\s)]+\))/g;
  let cursor=0;
  for(const match of text.matchAll(pattern)){
    if(match.index>cursor)host.append(document.createTextNode(text.slice(cursor,match.index)));
    const token=match[0];
    if(token.startsWith('**')||token.startsWith('__')){
      const node=document.createElement('strong');node.textContent=token.slice(2,-2);host.append(node);
    }else if(token.startsWith('*')||token.startsWith('_')){
      const node=document.createElement('em');node.textContent=token.slice(1,-1);host.append(node);
    }else if(token.startsWith('`')){
      const node=document.createElement('code');node.textContent=token.slice(1,-1);host.append(node);
    }else{
      const split=token.lastIndexOf(']('),label=token.slice(1,split),href=token.slice(split+2,-1);
      let url=null;try{url=new URL(href,location.href);}catch{}
      if(url&&['http:','https:','mailto:'].includes(url.protocol)){
        const node=document.createElement('a');node.textContent=label;node.href=url.href;node.target='_blank';node.rel='noopener noreferrer';host.append(node);
      }else host.append(document.createTextNode(label+' ('+href+')'));
    }
    cursor=match.index+token.length;
  }
  if(cursor<text.length)host.append(document.createTextNode(text.slice(cursor)));
}

function tableCells(line){
  let value=String(line||'').trim();
  if(value.startsWith('|'))value=value.slice(1);
  if(value.endsWith('|'))value=value.slice(0,-1);
  return value.split('|').map(cell=>cell.trim());
}
function tableDivider(line){
  const cells=tableCells(line);
  return cells.length>1&&cells.every(cell=>/^:?-{3,}:?$/.test(cell));
}
function beginsBlock(lines,index){
  const line=lines[index]||'';
  if(!line.trim())return true;
  if(/^\s*(```|~~~)/.test(line)||/^#{1,6}\s+/.test(line)||/^\s*>\s?/.test(line)||/^\s*[-+*]\s+/.test(line)||/^\s*\d+[.)]\s+/.test(line)||/^\s*((\*\s*){3,}|(-\s*){3,}|(_\s*){3,})\s*$/.test(line))return true;
  return index+1<lines.length&&line.includes('|')&&tableDivider(lines[index+1]);
}

function renderMarkdown(content){
  const root=document.createElement('article');root.className='markdown-preview-doc';
  const lines=String(content||'').replace(/\r\n?/g,'\n').split('\n');
  let i=0;
  while(i<lines.length){
    const line=lines[i];
    if(!line.trim()){i++;continue;}
    const fence=line.match(/^\s*(```|~~~)\s*([^\s]*)?.*$/);
    if(fence){
      const marker=fence[1],codeLines=[];i++;
      while(i<lines.length&&lines[i].trim()!==marker){codeLines.push(lines[i]);i++;}
      if(i<lines.length)i++;
      const pre=document.createElement('pre'),code=document.createElement('code');code.textContent=codeLines.join('\n');pre.append(code);root.append(pre);continue;
    }
    const heading=line.match(/^(#{1,6})\s+(.*)$/);
    if(heading){const node=document.createElement('h'+heading[1].length);appendInline(node,heading[2].replace(/\s+#+\s*$/,''));root.append(node);i++;continue;}
    if(/^\s*((\*\s*){3,}|(-\s*){3,}|(_\s*){3,})\s*$/.test(line)){root.append(document.createElement('hr'));i++;continue;}
    if(/^\s*>\s?/.test(line)){
      const values=[];while(i<lines.length&&/^\s*>\s?/.test(lines[i])){values.push(lines[i].replace(/^\s*>\s?/,''));i++;}
      const node=document.createElement('blockquote');appendInline(node,values.join('\n'));root.append(node);continue;
    }
    if(/^\s*[-+*]\s+/.test(line)){
      const list=document.createElement('ul');
      while(i<lines.length&&/^\s*[-+*]\s+/.test(lines[i])){const item=document.createElement('li');appendInline(item,lines[i].replace(/^\s*[-+*]\s+/,''));list.append(item);i++;}
      root.append(list);continue;
    }
    if(/^\s*\d+[.)]\s+/.test(line)){
      const list=document.createElement('ol');
      while(i<lines.length&&/^\s*\d+[.)]\s+/.test(lines[i])){const item=document.createElement('li');appendInline(item,lines[i].replace(/^\s*\d+[.)]\s+/,''));list.append(item);i++;}
      root.append(list);continue;
    }
    if(i+1<lines.length&&line.includes('|')&&tableDivider(lines[i+1])){
      const table=document.createElement('table'),thead=document.createElement('thead'),header=document.createElement('tr');
      for(const cell of tableCells(line)){const th=document.createElement('th');appendInline(th,cell);header.append(th);}thead.append(header);table.append(thead);i+=2;
      const tbody=document.createElement('tbody');
      while(i<lines.length&&lines[i].trim()&&lines[i].includes('|')){const row=document.createElement('tr');for(const cell of tableCells(lines[i])){const td=document.createElement('td');appendInline(td,cell);row.append(td);}tbody.append(row);i++;}
      table.append(tbody);root.append(table);continue;
    }
    const paragraph=[line.trim()];i++;
    while(i<lines.length&&!beginsBlock(lines,i)){paragraph.push(lines[i].trim());i++;}
    const node=document.createElement('p');appendInline(node,paragraph.join(' '));root.append(node);
  }
  return root;
}

function close(){
  backdrop.classList.remove('visible');
  body.replaceChildren();
}
function open(info){
  if(!info||info.kind!=='markdown'||typeof info.content!=='string')return;
  title.textContent=info.path||info.name||'Markdown';
  title.title=info.path||'';
  meta.textContent=['Markdown',formatBytes(info.size)].join(' · ');
  body.replaceChildren(renderMarkdown(info.content));
  backdrop.classList.add('visible');
  closeButton.focus();
}
closeButton.onclick=close;
backdrop.onmousedown=event=>{if(event.target===backdrop)close();};
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible')){event.preventDefault();close();}},true);
window.addEventListener('taskmenu:file-markdown-preview',event=>open(event.detail));

globalThis.TaskMenuMarkdownPreview={open,close,renderMarkdown};
