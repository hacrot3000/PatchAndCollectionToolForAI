// Read-only local Structured View. Never modifies the editor source or dirty state.
const structuredKinds={
 json:new Set(['json','json5','jsonc','ipynb','prefab','scene','fire','anim','meta']),
 xml:new Set(['xml','svg','plist','storyboard','xib','entitlements','stringsdict','xcworkspacedata','xcsettings','fairy','iml']),
 csv:new Set(['csv']),tsv:new Set(['tsv'])
};
const maxStructuredBytes=1024*1024,maxStructuredRows=500,maxStructuredNodes=2500;
const structuredStyle=document.createElement('style');
structuredStyle.textContent=[
 '.editor-body.structured-hidden{display:none!important}',
 '.editor-structured-preview{display:flex;flex:1;min-height:0;min-width:0;flex-direction:column;background:#0d1117}',
 '.editor-structured-preview[hidden]{display:none!important}',
 '.editor-structured-toolbar{padding:5px 10px;display:flex;align-items:center;gap:7px;border-bottom:1px solid #30363d;font:11px ui-monospace,monospace}',
 '.editor-structured-toolbar .message{flex:1;min-width:120px;opacity:.73}',
 '.editor-structured-toolbar button{font-size:11px;padding:3px 8px}',
 '.editor-structured-content{flex:1;min-height:0;overflow:auto;padding:10px;font:12px/1.55 ui-monospace,SFMono-Regular,Consolas,monospace;tab-size:2}',
 '.editor-structured-content pre{margin:0;white-space:pre;tab-size:2;font:inherit;width:max-content;min-width:100%}',
 '.editor-structured-content table{border-collapse:collapse;font:inherit;white-space:pre}',
 '.editor-structured-content th,.editor-structured-content td{border:1px solid #30363d;padding:3px 9px;text-align:left;max-width:540px;overflow:auto}',
 '.editor-structured-content th{position:sticky;top:0;background:#222b37;z-index:1}',
 '.editor-structured-content details{padding-left:15px;border-left:1px solid #354051}',
 '.editor-structured-content summary{cursor:pointer;white-space:pre}',
 '.editor-structured-content .structured-leaf{padding-left:18px;white-space:pre-wrap;overflow-wrap:anywhere}',
 '.editor-structured-content .structured-node{color:#bdd8fb}',
 '.editor-structured-content .structured-value{color:#b7d8a6}',
 'html[data-taskmenu-theme="light"] .editor-structured-preview{background:#fff}',
 'html[data-taskmenu-theme="light"] .editor-structured-content th{background:#f0f3f7}',
 'html[data-taskmenu-theme="light"] .editor-structured-content td,html[data-taskmenu-theme="light"] .editor-structured-content th{border-color:#c8d1dd}',
 'html[data-taskmenu-theme="light"] .editor-structured-content .structured-node{color:#174e89}',
 'html[data-taskmenu-theme="light"] .editor-structured-content .structured-value{color:#285c23}'
].join('\n');
document.head.append(structuredStyle);
function kindForPath(pathValue){
 const ext=String(pathValue||'').split(/[\\/]/).pop().toLowerCase().split('.').pop();
 for(const [kind,extensions] of Object.entries(structuredKinds))if(extensions.has(ext))return kind;
 return '';
}
function modeOptions(kind){return kind==='csv'||kind==='tsv'?['raw','pretty','table']:['raw','pretty','tree'];}
function formatJson(source){
 // Validate, but do NOT reserialize parsed numbers: preserve large integers and duplicate keys.
 JSON.parse(source);
 let quoted=false,escaped=false,depth=0,out='';
 const indent=()=> '  '.repeat(depth);
 for(let i=0;i<source.length;i++){
  const ch=source[i];
  if(quoted){
   out+=ch;
   if(escaped)escaped=false;
   else if(ch==='\\')escaped=true;
   else if(ch==='"')quoted=false;
   continue;
  }
  if(ch==='"'){out+=ch;quoted=true;continue;}
  if(/\s/.test(ch))continue;
  if(ch==='{'||ch==='['){
   let j=i+1;while(/\s/.test(source[j]||'')&&j<source.length)j++;
   if(source[j]===(ch==='{'?'}':']')){out+=ch+source[j];i=j;}
   else{depth++;out+=ch+'\n'+indent();}
  }else if(ch==='}'||ch===']'){depth=Math.max(0,depth-1);out+='\n'+indent()+ch;}
  else if(ch===',')out+=',\n'+indent();
  else if(ch===':')out+=': ';
  else out+=ch;
 }
 return out;
}
function parseDelimited(source,delimiter){
 const rows=[],row=[];let cell='',quoted=false,closed=false;
 for(let i=0;i<source.length;i++){
  const ch=source[i];
  if(quoted){
   if(ch==='"'){if(source[i+1]==='"'){cell+='"';i++;}else{quoted=false;closed=true;}}
   else cell+=ch;
   continue;
  }
  if(ch==='"'&&cell===''&&!closed){quoted=true;continue;}
  if(ch===delimiter){row.push(cell);cell='';closed=false;continue;}
  if(ch==='\r'||ch==='\n'){
   if(ch==='\r'&&source[i+1]==='\n')i++;
   row.push(cell);rows.push(row.splice(0));cell='';closed=false;continue;
  }
  if(closed&&ch!==' '&&ch!=='\t')throw Error('Unexpected characters after closing quote');
  if(closed)continue;
  if(ch==='"')throw Error('Unexpected quote inside unquoted field');
  cell+=ch;
 }
 if(quoted)throw Error('Unterminated quoted field');
 if(cell!==''||row.length||closed||!rows.length){row.push(cell);rows.push(row);}
 return rows;
}
function formatDelimited(rows){
 const columns=Math.min(64,Math.max(0,...rows.map(r=>r.length)));
 const show=v=>String(v??'').replaceAll('\r\n','↵').replaceAll('\n','↵').replaceAll('\r','↵');
 const widths=Array.from({length:columns},(_,c)=>Math.min(38,Math.max(1,...rows.slice(0,maxStructuredRows).map(r=>show(r[c]).length))));
 return rows.slice(0,maxStructuredRows).map(row=>row.slice(0,columns).map((v,c)=>show(v).slice(0,200).padEnd(widths[c])).join(' | ')).join('\n')+(rows.length>maxStructuredRows?'\n… '+(rows.length-maxStructuredRows)+' rows omitted':'');
}
function parseXml(source){
 if(/<!\s*(?:DOCTYPE|ENTITY)\b/i.test(source))throw Error('DTD/ENTITY declarations are not supported in Structured View. Use Raw.');
 const doc=new DOMParser().parseFromString(source,'application/xml');
 const err=doc.getElementsByTagName('parsererror')[0];
 if(err)throw Error('Invalid XML: '+err.textContent.slice(0,300));
 return doc;
}
function formatXml(source){
 const doc=parseXml(source),serialize=new XMLSerializer();
 const escapeAttr=v=>String(v).replaceAll('&','&amp;').replaceAll('"','&quot;').replaceAll('<','&lt;');
 const format=(node,depth)=>{
  const pad='  '.repeat(depth);
  if(node.nodeType!==1)return pad+serialize.serializeToString(node);
  const children=[...node.childNodes];
  // Mixed content and xml:space="preserve" must stay byte-insensitive in view.
  if(node.getAttribute('xml:space')==='preserve'||children.some(n=>n.nodeType===3&&n.textContent.trim()))return pad+serialize.serializeToString(node);
  const attrs=[...node.attributes].map(a=>' '+a.name+'="'+escapeAttr(a.value)+'"').join('');
  const open='<'+node.nodeName+attrs;
  const visible=children.filter(n=>n.nodeType!==3||n.textContent.trim());
  return !visible.length?pad+open+'/>':pad+open+'>\n'+visible.map(n=>format(n,depth+1)).join('\n')+'\n'+pad+'</'+node.nodeName+'>';
 };
 const declaration=source.match(/^\s*<\?xml[\s\S]*?\?>/i)?.[0]?.trim();
 return [declaration,...[...doc.childNodes].filter(n=>[1,7,8].includes(n.nodeType)).map(n=>format(n,0))].filter(Boolean).join('\n');
}
function nodeLabel(key,value){
 const prefix=key===null?'':String(key)+': ';
 if(Array.isArray(value))return prefix+'Array ('+value.length+')';
 if(value!==null&&typeof value==='object')return prefix+'Object ('+Object.keys(value).length+')';
 return prefix+JSON.stringify(value);
}
function renderTree(root,parsed){
 let remaining=maxStructuredNodes;
 const make=(parent,key,value,depth)=>{
  if(--remaining<0){const e=document.createElement('div');e.textContent='… tree limit reached';parent.append(e);return;}
  if(depth>40){const e=document.createElement('div');e.textContent='… depth limit reached';parent.append(e);return;}
  if(value===null||typeof value!=='object'){
   const e=document.createElement('div');e.className='structured-leaf structured-value';e.textContent=nodeLabel(key,value);parent.append(e);return;
  }
  const details=document.createElement('details');details.open=depth===0;
  const summary=document.createElement('summary');summary.className='structured-node';summary.textContent=nodeLabel(key,value);
  details.append(summary);parent.append(details);
  let populated=false;
  const expand=()=>{
   if(populated)return;populated=true;
   const entries=Array.isArray(value)?value.map((v,i)=>[i,v]):Object.entries(value);
   for(const [name,child] of entries.slice(0,maxStructuredNodes))make(details,name,child,depth+1);
   if(entries.length>maxStructuredNodes){const el=document.createElement('div');el.textContent='… remaining children omitted';details.append(el);}
  };
  details.addEventListener('toggle',()=>{if(details.open)expand();});
  if(details.open)expand();
 };
 make(root,null,parsed,0);
}
function xmlTree(doc){
 // Generate the hierarchy without inserting untrusted XML as HTML.
 const convert=node=>{
  const result={},children=[...node.childNodes].filter(n=>n.nodeType===1);
  if(node.attributes?.length)result['@attributes']=Object.fromEntries([...node.attributes].map(a=>[a.name,a.value]));
  const text=[...node.childNodes].filter(n=>n.nodeType===3).map(n=>n.textContent).join('').trim();
  if(text)result['#text']=text;
  for(const child of children){
   const data=convert(child);
   if(Object.prototype.hasOwnProperty.call(result,child.nodeName)){
    if(!Array.isArray(result[child.nodeName]))result[child.nodeName]=[result[child.nodeName]];
    result[child.nodeName].push(data);
   }else result[child.nodeName]=data;
  }
  return result;
 };
 return {[doc.documentElement.nodeName]:convert(doc.documentElement)};
}
function renderTable(host,rows){
 const table=document.createElement('table'),head=document.createElement('thead'),body=document.createElement('tbody'),heading=document.createElement('tr'),first=rows[0]||[];
 for(const value of first.slice(0,64)){const th=document.createElement('th');th.textContent=value;heading.append(th);}
 head.append(heading);
 for(const row of rows.slice(1,maxStructuredRows+1)){
  const tr=document.createElement('tr');
  for(let col=0;col<Math.min(64,Math.max(first.length,row.length));col++){
   const td=document.createElement('td');td.textContent=row[col]??'';tr.append(td);
  }
  body.append(tr);
 }
 table.append(head,body);host.append(table);
 if(rows.length>maxStructuredRows+1){
  const p=document.createElement('p');p.textContent=(rows.length-maxStructuredRows-1)+' more rows omitted';host.append(p);
 }
}
function setMode(view,requested){
 const kind=kindForPath(view?.file?.path);
 if(!view||view.closed||!kind)return false;
 const mode=modeOptions(kind).includes(requested)?requested:'raw';
 if(mode==='raw'){
  view.structuredMode='raw';view.body.classList.remove('structured-hidden');
  if(view.structuredPreview)view.structuredPreview.hidden=true;
  view.structuredSelect.value='raw';view.cm.requestMeasure?.();return true;
 }
 if(view.file.large_file)throw Error('Structured View is unavailable for large files');
 const source=view.cm.state.doc.toString();
 if(new TextEncoder().encode(source).length>maxStructuredBytes)throw Error('Structured View is limited to 1 MiB; use Raw for larger documents');
 // Parse before creating or replacing preview, so invalid documents leave Raw unaffected.
 let formatted='',parsed=null,rows=null;
 if(kind==='json'){
  if(mode==='pretty')formatted=formatJson(source);
  else parsed=JSON.parse(source);
 }else if(kind==='xml'){
  if(mode==='pretty')formatted=formatXml(source);
  else parsed=xmlTree(parseXml(source));
 }else{
  rows=parseDelimited(source,kind==='csv'?',':'\t');
  if(mode==='pretty')formatted=formatDelimited(rows);
 }
 if(!view.structuredPreview){
  const shell=document.createElement('div');shell.className='editor-structured-preview';shell.hidden=true;
  const toolbar=document.createElement('div');toolbar.className='editor-structured-toolbar';
  const status=document.createElement('span');status.className='message';
  const refresh=document.createElement('button');refresh.type='button';refresh.textContent='Refresh';refresh.title='Update from current editor buffer';
  const copy=document.createElement('button');copy.type='button';copy.textContent='Copy view';copy.title='Copy rendered view, not source';
  const content=document.createElement('div');content.className='editor-structured-content';
  toolbar.append(status,refresh,copy);shell.append(toolbar,content);view.pane.append(shell);
  view.structuredPreview=shell;view.structuredContent=content;view.structuredStatus=status;
  refresh.onclick=()=>{try{setMode(view,view.structuredMode);}catch(e){view.structuredStatus.textContent=e.message;}};
  copy.onclick=()=>{navigator.clipboard?.writeText(view.structuredText||view.structuredContent.textContent).catch(e=>{view.structuredStatus.textContent='Copy failed: '+e.message;});};
 }
 const container=view.structuredContent;
 container.replaceChildren();
 if(mode==='pretty'){
  const pre=document.createElement('pre');pre.textContent=formatted;container.append(pre);
 }else if(mode==='table')renderTable(container,rows);
 else renderTree(container,parsed);
 view.structuredText=mode==='pretty'?formatted:container.textContent;
 view.structuredMode=mode;view.structuredSelect.value=mode;
 view.structuredStatus.textContent='View only · '+kind.toUpperCase()+' · current editor buffer';
 view.body.classList.add('structured-hidden');view.structuredPreview.hidden=false;
 return true;
}
function markStale(view){
 if(view?.structuredMode&&view.structuredMode!=='raw'&&view.structuredStatus){
  view.structuredStatus.textContent='Source updated · press Refresh to update preview';
 }
}
globalThis.TaskMenuStructuredView={kindForPath,modeOptions,formatJson,parseDelimited,formatDelimited,parseXml,formatXml,setMode,markStale};
