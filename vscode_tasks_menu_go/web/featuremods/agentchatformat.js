// TaskDeck Agent Chat: safe rich Markdown and copyable, syntax-highlighted code.
// Untrusted messages are only rendered via textContent; never use innerHTML.
(() => {
 'use strict';
 const el=(tag,cls,text)=>{
  const n=document.createElement(tag);
  if(cls)n.className=cls;
  if(text!==undefined)n.textContent=String(text);
  return n;
 };
 const css=el('style');
 css.textContent=[
  '.agentchat-bubble:has(.agentchat-code-block){box-sizing:border-box;min-width:0;width:min(100%,710px)}',
  '.agentchat-md-paragraph{margin:0 0 7px;white-space:pre-wrap;overflow-wrap:anywhere}',
  '.agentchat-md-paragraph:last-child{margin-bottom:0}',
  '.agentchat-md-heading{font-weight:700;line-height:1.4;margin:7px 0 5px;white-space:normal}',
  '.agentchat-md-h1{font-size:17px}.agentchat-md-h2{font-size:15px}.agentchat-md-h3{font-size:13px}',
  '.agentchat-md-list{margin:4px 0 8px;padding-left:22px;white-space:normal}',
  '.agentchat-md-list li{margin:2px 0;white-space:pre-wrap;overflow-wrap:anywhere}',
  '.agentchat-md-quote{margin:6px 0 9px;padding:5px 9px;border-left:3px solid #7a9abb;background:#223145;border-radius:0 5px 5px 0;white-space:normal}',
  '.agentchat-md-quote .agentchat-md-paragraph{margin:2px 0}',
  '.agentchat-md-divider{border:0;border-top:1px solid #54657b;margin:10px 0}',
  '.agentchat-inline-code{font:11px/1.45 ui-monospace,monospace;background:#293849;color:#e7c7a5;padding:1px 4px;border:1px solid #4e6176;border-radius:4px;white-space:break-spaces;overflow-wrap:anywhere}',
  '.agentchat-inline-link{color:#91caff;text-decoration:underline;text-underline-offset:2px}',
  '.agentchat-md-table{border-collapse:collapse;display:block;max-width:100%;overflow-x:auto;margin:6px 0 10px;font-size:11px;white-space:normal}',
  '.agentchat-md-table th,.agentchat-md-table td{border:1px solid #52647a;padding:4px 7px;text-align:left;vertical-align:top}',
  '.agentchat-md-table th{background:#293b50}',
  '.agentchat-code-block{max-width:100%;min-width:0;margin:6px 0 10px;border:1px solid #40546b;border-radius:7px;overflow:hidden;background:#101923;white-space:normal}',
  '.agentchat-code-header{display:flex;gap:8px;align-items:center;justify-content:space-between;min-height:28px;padding:3px 8px;background:#243246;border-bottom:1px solid #40546b}',
  '.agentchat-code-language{font:10px ui-monospace,monospace;color:#b9cce2;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}',
  '.agentchat-window button.agentchat-code-copy{display:flex;align-items:center;justify-content:center;min-width:25px;width:25px;height:23px;padding:0!important;font:16px/1 ui-monospace,monospace;background:transparent;border:1px solid #53657b;color:#e1e9f5;border-radius:4px}',
  '.agentchat-window button.agentchat-code-copy:hover{background:#3b516a}',
  '.agentchat-code-pre{margin:0;padding:10px 11px;max-width:100%;overflow-x:auto;overflow-y:auto;max-height:400px;white-space:pre;tab-size:4;font:11px/1.55 ui-monospace,Consolas,monospace}',
  '.agentchat-code-content{white-space:pre;font:inherit;color:#e4edf8}',
  '.agentchat-syntax-keyword{color:#d0b5ff}.agentchat-syntax-string{color:#f0c39f}.agentchat-syntax-comment{color:#92bda3;font-style:italic}',
  '.agentchat-syntax-number,.agentchat-syntax-literal{color:#bde19d}.agentchat-syntax-variable{color:#9dd4f7}.agentchat-syntax-function{color:#f3d58e}.agentchat-syntax-property{color:#9bd3eb}',
  'html[data-taskmenu-theme="light"] .agentchat-md-quote{background:#e8f0fa;border-left-color:#6d8bac}',
  'html[data-taskmenu-theme="light"] .agentchat-inline-code{background:#ecf2fa;color:#815327;border-color:#d0dbe7}',
  'html[data-taskmenu-theme="light"] .agentchat-inline-link{color:#155bb4}',
  'html[data-taskmenu-theme="light"] .agentchat-md-table th{background:#e8f0fa}',
  'html[data-taskmenu-theme="light"] .agentchat-code-block{background:#f6f8fc;border-color:#bdccdb}',
  'html[data-taskmenu-theme="light"] .agentchat-code-header{background:#e6edf7;border-color:#bdccdb}',
  'html[data-taskmenu-theme="light"] .agentchat-code-language{color:#4b5d73}',
  'html[data-taskmenu-theme="light"] .agentchat-window button.agentchat-code-copy{color:#294765;border-color:#b4c8dc}',
  'html[data-taskmenu-theme="light"] .agentchat-code-content{color:#17283a}',
  'html[data-taskmenu-theme="light"] .agentchat-syntax-keyword{color:#7232b2}',
  'html[data-taskmenu-theme="light"] .agentchat-syntax-string{color:#a34f21}',
  'html[data-taskmenu-theme="light"] .agentchat-syntax-comment{color:#3c7754}',
  'html[data-taskmenu-theme="light"] .agentchat-syntax-number,html[data-taskmenu-theme="light"] .agentchat-syntax-literal{color:#527221}',
  'html[data-taskmenu-theme="light"] .agentchat-syntax-variable{color:#1769aa}',
  'html[data-taskmenu-theme="light"] .agentchat-syntax-function{color:#8b601b}',
  'html[data-taskmenu-theme="light"] .agentchat-syntax-property{color:#006b84}'
 ].join('\n');
 document.head.append(css);

 const keywords={
  php:'abstract array as break case catch class const continue default do echo else elseif empty extends false final finally for foreach function global if include include_once instanceof interface isset namespace new null print private protected public require require_once return static switch throw trait true try use while',
  javascript:'async await break case catch class const continue default delete do else export extends false finally for from function if import in instanceof let new null of return static super switch this throw true try typeof undefined var void while yield',
  typescript:'any as async await boolean break case catch class const continue declare default do else enum export extends false finally for from function if implements import in interface let never new null number of private protected public readonly return static string super switch this throw true try type typeof undefined unknown var void while',
  python:'and as assert async await break class continue def del elif else except false finally for from global if import in is lambda none nonlocal not or pass raise return self true try while with yield',
  go:'break case chan const continue default defer else fallthrough for func go goto if import interface map nil package range return select struct switch type var',
  sql:'add all alter and as asc begin between by case cast check column commit constraint create cross database default delete desc distinct drop else end exists false foreign from full group having if in index inner insert into is join key left like limit not null offset on or order outer primary references right rollback select set table then true truncate union unique update values view when where with',
  bash:'alias break case cd continue declare do done echo elif else esac eval exec exit export false fi for function if in local printf read readonly return select set shift source test then time trap true unset until wait while',
  java:'abstract assert boolean break byte case catch char class const continue default do double else enum extends final finally float for if implements import instanceof int interface long native new null package private protected public return short static super switch synchronized this throw throws true try void volatile while',
  kotlin:'as break by catch class constructor continue data do else enum false final finally for fun if import in infix init inline interface internal is lateinit null object open operator out override package private protected public return sealed suspend super this throw true try typealias val var when while',
  swift:'actor any as async await break case catch class continue default defer do else enum extension false fileprivate for func guard if import in init internal is let nil open override private protocol public repeat return self static struct super switch throw throws true try typealias var weak where while',
  css:'auto background block border color content display fixed flex font grid height inherit initial inline margin none padding relative solid sticky unset var width',
  rust:'as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self static struct super trait true type unsafe use where while',
  c:'auto break case char const continue default do double else enum extern float for goto if inline int long register return short signed sizeof static struct switch typedef union unsigned void volatile while',
  cpp:'auto bool break case catch char class const constexpr continue decltype default delete do double else enum explicit export extern false final float for friend if inline int long mutable namespace new nullptr operator override private protected public return short signed sizeof static struct switch template this throw true try typedef typename union unsigned using virtual void volatile while'
 };
 function languageOf(hint,body){
  const aliases={js:'javascript',jsx:'javascript',mjs:'javascript',ts:'typescript',tsx:'typescript',py:'python',sh:'bash',shell:'bash',zsh:'bash',yml:'yaml',htm:'html',cxx:'cpp','c++':'cpp',rs:'rust',golang:'go',txt:'text',plaintext:'text'};
  let language=String(hint||'').trim().toLowerCase().slice(0,32);
  language=aliases[language]||language;
  if(language&&/^[a-z][a-z0-9+#.-]*$/.test(language))return language;
  const s=String(body||'').trimStart();
  if(/^<\?php\b/i.test(s))return 'php';
  if(/^[\[{]/.test(s)){try{JSON.parse(s);return 'json';}catch{}}
  if(/^(?:diff --git|@@ -\d|--- a\/)/.test(s))return 'diff';
  if(/^(?:#!.*\b(?:bash|sh)|\$\s)/.test(s))return 'bash';
  if(/^(?:def |from \w+ import |import \w+$)/m.test(s))return 'python';
  if(/^(?:package \w+|func \w+\()/m.test(s))return 'go';
  if(/^(?:SELECT|INSERT|UPDATE|CREATE TABLE)\b/i.test(s))return 'sql';
  if(/^</.test(s)&&/<[A-Za-z/]/.test(s))return 'html';
  return 'text';
 }
 function highlightCode(container,raw,language){
  const code=String(raw),lang=languageOf(language,code);
  if(code.length>120000){container.append(el('span','',code));return;}
  const words=new Set((keywords[lang]||'').split(' ').filter(Boolean));
  const re=/\/\*[\s\S]*?\*\/|<!--[\s\S]*?-->|\/\/[^\n]*|(?:#|--)[^\n]*|"(?:\\[\s\S]|[^"\\])*"|'(?:\\[\s\S]|[^'\\])*'|\x60(?:\\[\s\S]|[^\x60\\])*\x60|\$[A-Za-z_]\w*|\b\d+(?:\.\d+)?\b|\b[A-Za-z_]\w*\b/g;
  let last=0,m,count=0;
  while((m=re.exec(code))!==null && ++count<=12000){
   if(m.index>last)container.append(el('span','',code.slice(last,m.index)));
   const value=m[0];let kind='';
   if(/^\/\*|^<!--/.test(value)||
     (/^\/\//.test(value)&&!['python','sql','yaml','json'].includes(lang))||
     (/^#/.test(value)&&['python','bash','ruby','yaml','powershell'].includes(lang))||
     (/^--/.test(value)&&lang==='sql'))kind='comment';
   else if(/^[\x60"']/.test(value))kind=lang==='json'&&/^\s*:/.test(code.slice(re.lastIndex))?'property':'string';
   else if(/^\$[A-Za-z_]/.test(value))kind='variable';
   else if(/^\d/.test(value))kind='number';
   else if(words.has(value)||words.has(value.toLowerCase()))kind='keyword';
   else if(/^(?:true|false|null|nil|none|undefined)$/i.test(value))kind='literal';
   else if(/^[A-Za-z_]\w*$/.test(value)&&/^\s*\(/.test(code.slice(re.lastIndex)))kind='function';
   container.append(el('span',kind?'agentchat-syntax-'+kind:'',value));last=re.lastIndex;
  }
  if(last<code.length)container.append(el('span','',code.slice(last)));
 }
 function renderCode(body,hint,onError){
  const code=String(body),lang=languageOf(hint,code),root=el('div','agentchat-code-block');
  const bar=el('div','agentchat-code-header'),label=el('span','agentchat-code-language',lang==='text'?'Text':lang);
  const copy=el('button','agentchat-code-copy','⧉');
  copy.type='button';copy.title='Copy code';copy.setAttribute('aria-label','Copy code block');
  copy.onclick=()=>{
   const api=globalThis.navigator?.clipboard;
   if(!api?.writeText){onError?.(new Error('Clipboard unavailable. Use HTTPS/localhost and allow clipboard access.'));return;}
   Promise.resolve(api.writeText(code)).then(()=>{
    copy.textContent='✓';copy.setAttribute('aria-label','Code copied');
    setTimeout(()=>{if(copy.isConnected!==false){copy.textContent='⧉';copy.setAttribute('aria-label','Copy code block');}},1600);
   }).catch(e=>onError?.(e));
  };
  bar.append(label,copy);
  const pre=el('pre','agentchat-code-pre'),source=el('code','agentchat-code-content');
  highlightCode(source,code,lang);pre.append(source);root.append(bar,pre);return root;
 }
 function inline(root,text,depth=0){
  text=String(text);
  if(depth>=4){root.append(el('span','',text));return;}
  const tokens=/\x60[^\x60\n]+\x60|\[[^\]\n]+\]\((?:https?:\/\/|mailto:)[^\s)]+\)|\*\*[^*\n]+\*\*|__[^_\n]+__|~~[^~\n]+~~|\*[^*\n]+\*|_[^_\n]+_/g;
  let last=0,m;
  while((m=tokens.exec(text))!==null){
   if(m.index>last)root.append(el('span','',text.slice(last,m.index)));
   const token=m[0];
   if(token[0]==='\x60')root.append(el('code','agentchat-inline-code',token.slice(1,-1)));
   else if(token[0]==='['){
    const at=token.indexOf(']('),href=token.slice(at+2,-1);
    const a=el('a','agentchat-inline-link',token.slice(1,at));
    a.href=href;a.target='_blank';a.rel='noopener noreferrer';root.append(a);
   }else{
    const long=token.startsWith('**')||token.startsWith('__')||token.startsWith('~~'),size=long?2:1;
    const n=el(token.startsWith('~~')?'del':long?'strong':'em');
    inline(n,token.slice(size,-size),depth+1);root.append(n);
   }
   last=tokens.lastIndex;
  }
  if(last<text.length)root.append(el('span','',text.slice(last)));
 }
 function blocks(root,lines,onError,depth=0){
  let i=0,paragraph=[];
  const flush=()=>{
   if(!paragraph.length)return;
   const p=el('p','agentchat-md-paragraph');
   paragraph.forEach((line,j)=>{if(j)p.append(el('span','','\n'));inline(p,line);});
   root.append(p);paragraph=[];
  };
  const fence=line=>/^\s{0,3}(\x60{3,}|~{3,})[ \t]*([A-Za-z0-9_+#.-]{0,32})(?:[ \t]+(.*))?$/.exec(line);
  const quote=line=>/^\s{0,3}>\s?(.*)$/.exec(line);
  const list=line=>/^\s{0,3}([-+*]|\d+\.)\s+(.+)$/.exec(line);
  const tableSeparator=line=>/^\s*\|?\s*:?-{3,}:?\s*(?:\|\s*:?-{3,}:?\s*)+\|?\s*$/.test(line);
  const cells=line=>line.trim().replace(/^\|/,'').replace(/\|$/,'').split(/(?<!\\)\|/).map(v=>v.trim().replace(/\\\|/g,'|'));
  while(i<lines.length){
   const line=lines[i],f=fence(line);
   if(f){
    flush();const code=[],char=f[1][0],width=f[1].length;
    const end=new RegExp('^\\s{0,3}'+char+'{'+width+',}\\s*$');
    const tail=f[3]||'',at=tail.lastIndexOf(f[1]);
    if(tail&&at>=0&&tail.slice(at+width).trim()===''){code.push(tail.slice(0,at).trimEnd());i++;}
    else{
     if(tail)code.push(tail);
     i++;while(i<lines.length&&!end.test(lines[i]))code.push(lines[i++]);
     if(i<lines.length)i++;
    }
    root.append(renderCode(code.join('\n'),f[2],onError));continue;
   }
   if(!line.trim()){flush();i++;continue;}
   const heading=/^\s{0,3}(#{1,6})\s+(.+?)\s*#*\s*$/.exec(line);
   if(heading){flush();const h=el('div','agentchat-md-heading agentchat-md-h'+heading[1].length);inline(h,heading[2]);root.append(h);i++;continue;}
   if(/^\s{0,3}(?:-{3,}|\*{3,}|_{3,})\s*$/.test(line)){flush();root.append(el('hr','agentchat-md-divider'));i++;continue;}
   if(quote(line)){
    flush();const parts=[];
    while(i<lines.length&&quote(lines[i]))parts.push(quote(lines[i++])[1]);
    const b=el('blockquote','agentchat-md-quote');
    if(depth<3)blocks(b,parts,onError,depth+1);else b.append(el('span','',parts.join('\n')));
    root.append(b);continue;
   }
   if(list(line)){
    flush();const ordered=/^\d/.test(list(line)[1]),group=el(ordered?'ol':'ul','agentchat-md-list');
    while(i<lines.length&&list(lines[i])&&(/^\d/.test(list(lines[i])[1]))===ordered){
     const li=el('li');inline(li,list(lines[i++])[2]);group.append(li);
    }
    root.append(group);continue;
   }
   if(i+1<lines.length&&line.includes('|')&&tableSeparator(lines[i+1])){
    flush();const table=el('table','agentchat-md-table'),thead=el('thead'),tr=el('tr');
    for(const v of cells(line).slice(0,12)){const th=el('th');inline(th,v);tr.append(th);}
    thead.append(tr);table.append(thead);i+=2;const tbody=el('tbody');let rows=0;
    while(i<lines.length&&lines[i].includes('|')&&lines[i].trim()&&rows++<80){
     const row=el('tr');
     for(const v of cells(lines[i]).slice(0,12)){const td=el('td');inline(td,v);row.append(td);}
     tbody.append(row);i++;
    }
    table.append(tbody);root.append(table);continue;
   }
   paragraph.push(line);i++;
  }
  flush();
 }
 function renderMessage(role,raw,onError){
  const root=el('div','agentchat-bubble '+role),content=String(raw??'');
  if(content.length>160000){root.textContent=content;return root;}
  blocks(root,content.replace(/\r\n?/g,'\n').split('\n'),onError);
  return root;
 }
 globalThis.TaskMenuAgentChatFormat=Object.freeze({renderMessage,renderCode,languageOf});
})();
