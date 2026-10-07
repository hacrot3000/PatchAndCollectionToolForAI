const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for editor completion');
if(!globalThis.cm6?.autocompletion||!globalThis.cm6?.snippetCompletion)throw new Error('Vendored CodeMirror completion hooks unavailable');

const words=value=>String(value||'').trim().split(/\s+/).filter(Boolean);
const snippet=(label,detail,template,type='keyword')=>({label,detail,template,type});
const profile=(keywords='',snippets=[],syntax={})=>({keywords:words(keywords),snippets,syntax});

const profiles={
  cpp:profile(
    'alignas alignof and and_eq asm auto bitand bitor bool break case catch char class compl concept const consteval constexpr constinit const_cast continue co_await co_return co_yield decltype default delete do double dynamic_cast else enum explicit export extern false float for friend goto if inline int long mutable namespace new noexcept not not_eq nullptr operator or or_eq private protected public register reinterpret_cast requires return short signed sizeof static static_assert static_cast struct switch template this thread_local throw true try typedef typeid typename union unsigned using virtual void volatile wchar_t while xor xor_eq',
    [
      snippet('if','if block','if (${condition}) {\n\t${}\n}'),
      snippet('for','for loop','for (${init}; ${condition}; ${step}) {\n\t${}\n}'),
      snippet('class','class declaration','class ${Name} {\npublic:\n\t${Name}();\n\t${}\n};'),
      snippet('function','function definition','${returnType} ${name}(${params}) {\n\t${}\n}'),
      snippet('#include','include header','#include <${header}>\n${}')
    ],
    {lineComments:['//'],blockComments:[['/*','*/']]}
  ),
  go:profile(
    'break default func interface select case defer go map struct chan else goto package switch const fallthrough if range type continue for import return var',
    [
      snippet('func','function','func ${name}(${params}) ${result} {\n\t${}\n}'),
      snippet('if','if block','if ${condition} {\n\t${}\n}'),
      snippet('iferr','error guard','if err != nil {\n\treturn ${}\n}'),
      snippet('for','for loop','for ${condition} {\n\t${}\n}'),
      snippet('range','range loop','for ${key}, ${value} := range ${collection} {\n\t${}\n}'),
      snippet('struct','struct type','type ${Name} struct {\n\t${}\n}')
    ],
    {lineComments:['//'],blockComments:[['/*','*/']]}
  ),
  python:profile(
    'and as assert async await break class continue def del elif else except False finally for from global if import in is lambda None nonlocal not or pass raise return True try while with yield match case',
    [
      snippet('def','function','def ${name}(${params}):\n    ${}'),
      snippet('class','class','class ${Name}:\n    def __init__(self, ${params}):\n        ${}'),
      snippet('if','if block','if ${condition}:\n    ${}'),
      snippet('for','for loop','for ${item} in ${items}:\n    ${}'),
      snippet('try','try/except','try:\n    ${}\nexcept ${Exception} as ${exc}:\n    ${}')
    ],
    {lineComments:['#'],tripleQuotes:['"""',"'''"]}
  ),
  javascript:profile(
    'as async await break case catch class const continue debugger default delete do else export extends false finally for from function get if import in instanceof let new null of return set static super switch this throw true try typeof undefined var void while with yield',
    [
      snippet('function','function','function ${name}(${params}) {\n\t${}\n}'),
      snippet('async function','async function','async function ${name}(${params}) {\n\t${}\n}'),
      snippet('if','if block','if (${condition}) {\n\t${}\n}'),
      snippet('forof','for of loop','for (const ${item} of ${items}) {\n\t${}\n}'),
      snippet('class','class','class ${Name} {\n\tconstructor(${params}) {\n\t\t${}\n\t}\n}')
    ],
    {lineComments:['//'],blockComments:[['/*','*/']],templateQuote:'`'}
  ),
  typescript:profile(
    'abstract any as asserts async await bigint boolean break case catch class const constructor continue debugger declare default delete do else enum export extends false finally for from function get global if implements import in infer instanceof interface is keyof let module namespace never new null number object of override private protected public readonly require return set static string super switch symbol this throw true try type typeof undefined unique unknown var void while with yield',
    [
      snippet('interface','interface','interface ${Name} {\n\t${}\n}'),
      snippet('type','type alias','type ${Name} = ${type}'),
      snippet('function','typed function','function ${name}(${params}): ${returnType} {\n\t${}\n}'),
      snippet('class','class','class ${Name} {\n\tconstructor(${params}) {\n\t\t${}\n\t}\n}')
    ],
    {lineComments:['//'],blockComments:[['/*','*/']],templateQuote:'`'}
  ),
  json:profile('true false null',[
    snippet('object','object','{\n  "${key}": ${}\n}'),
    snippet('array','array','[\n  ${}\n]')
  ]),
  yaml:profile('true false null yes no on off',[
    snippet('mapping','mapping','${key}: ${value}'),
    snippet('list','list','- ${item}\n- ${}')
  ],{lineComments:['#']}),
  markdown:profile('',[
    snippet('#','heading','# ${title}\n\n${}'),
    snippet('link','link','[${text}](${url})'),
    snippet('code','fenced code','```${language}\n${}\n```'),
    snippet('table','table','| ${Column 1} | ${Column 2} |\n| --- | --- |\n| ${} | |')
  ]),
  html:profile('html head body title meta link script style div span main section article header footer nav form input button label table thead tbody tr th td ul ol li',[
    snippet('div','div element','<div>\n\t${}\n</div>'),
    snippet('script','script element','<script>\n\t${}\n</script>'),
    snippet('html','HTML document','<!doctype html>\n<html>\n<head>\n\t<meta charset="utf-8">\n\t<title>${title}</title>\n</head>\n<body>\n\t${}\n</body>\n</html>')
  ],{blockComments:[['<!--','-->']]}),
  css:profile('align-items background border bottom color display flex flex-direction font font-family font-size font-weight gap grid height justify-content left margin max-height max-width min-height min-width opacity overflow padding position right top transform transition width z-index',[
    snippet('display:flex','flex container','display: flex;\nalign-items: ${center};\njustify-content: ${center};'),
    snippet('@media','media query','@media (${condition}) {\n\t${}\n}')
  ],{blockComments:[['/*','*/']]}),
  dockerfile:profile('FROM RUN CMD LABEL MAINTAINER EXPOSE ENV ADD COPY ENTRYPOINT VOLUME USER WORKDIR ARG ONBUILD STOPSIGNAL HEALTHCHECK SHELL',[
    snippet('FROM','base image','FROM ${image}:${tag}'),
    snippet('RUN','run command','RUN ${}'),
    snippet('COPY','copy files','COPY ${source} ${destination}'),
    snippet('ENTRYPOINT','entrypoint','ENTRYPOINT ["${command}"]')
  ],{lineComments:['#']}),
  makefile:profile('ifdef ifndef ifeq ifneq else endif include sinclude define endef override export unexport private vpath undefine',[
    snippet('target','target','${target}: ${dependencies}\n\t${}'),
    snippet('variable','variable','${NAME} := ${value}')
  ],{lineComments:['#']}),
  toml:profile('true false inf nan',[
    snippet('table','table','[${table}]\n${key} = ${value}\n${}'),
    snippet('array-table','array table','[[${table}]]\n${key} = ${value}\n${}')
  ],{lineComments:['#']}),
  powershell:profile(
    'begin break catch class continue data define do dynamicparam else elseif end enum exit filter finally for foreach from function hidden if in inlinescript parallel param process return sequence static switch throw trap try until using var while workflow',
    [
      snippet('function','function','function ${Name} {\n    param(\n        ${}\n    )\n}'),
      snippet('if','if block','if (${condition}) {\n    ${}\n}'),
      snippet('foreach','foreach','foreach ($${item} in $${items}) {\n    ${}\n}')
    ],
    {lineComments:['#'],blockComments:[['<#','#>']]}
  ),
  kotlin:profile(
    'as break class continue do else false for fun if in interface is null object package return super this throw true try typealias typeof val var when while by catch constructor delegate dynamic field file finally get import init param property receiver set where actual abstract annotation companion const crossinline data enum expect external final infix inline inner internal lateinit noinline open operator out override private protected public reified sealed suspend tailrec vararg',
    [
      snippet('fun','function','fun ${name}(${params}): ${Type} {\n    ${}\n}'),
      snippet('class','class','class ${Name}(${params}) {\n    ${}\n}'),
      snippet('when','when expression','when (${value}) {\n    ${case} -> ${}\n    else -> ${}\n}')
    ],
    {lineComments:['//'],blockComments:[['/*','*/']]}
  ),
  csharp:profile(
    'abstract as base bool break byte case catch char checked class const continue decimal default delegate do double else enum event explicit extern false finally fixed float for foreach goto if implicit in int interface internal is lock long namespace new null object operator out override params private protected public readonly record ref return sbyte sealed short sizeof stackalloc static string struct switch this throw true try typeof uint ulong unchecked unsafe ushort using virtual void volatile while async await dynamic get init partial remove set value var when where yield',
    [
      snippet('class','class','public class ${Name}\n{\n    ${}\n}'),
      snippet('method','method','public ${ReturnType} ${Name}(${params})\n{\n    ${}\n}'),
      snippet('property','property','public ${Type} ${Name} { get; set; }')
    ],
    {lineComments:['//'],blockComments:[['/*','*/']]}
  ),
  dart:profile(
    'abstract as assert async await break case catch class const continue covariant default deferred do dynamic else enum export extends extension external factory false final finally for Function get hide if implements import in interface is late library mixin new null on operator part required rethrow return set show static super switch sync this throw true try typedef var void while with yield',
    [
      snippet('class','class','class ${Name} {\n  ${}\n}'),
      snippet('function','function','${ReturnType} ${name}(${params}) {\n  ${}\n}'),
      snippet('Future','async function','Future<${Type}> ${name}(${params}) async {\n  ${}\n}')
    ],
    {lineComments:['//'],blockComments:[['/*','*/']]}
  ),
  protobuf:profile('syntax import weak public package option repeated optional required oneof map reserved to max enum message service rpc returns stream extend extensions group',[
    snippet('message','message','message ${Name} {\n  ${}\n}'),
    snippet('enum','enum','enum ${Name} {\n  ${NAME_UNSPECIFIED} = 0;\n  ${}\n}'),
    snippet('service','service','service ${Name} {\n  rpc ${Method}(${Request}) returns (${Response});\n  ${}\n}')
  ],{lineComments:['//'],blockComments:[['/*','*/']]}),
  graphql:profile('query mutation subscription fragment on schema scalar type interface union enum input directive extend implements repeatable true false null',[
    snippet('type','type','type ${Name} {\n  ${}\n}'),
    snippet('query','query','query ${Name} {\n  ${}\n}'),
    snippet('mutation','mutation','mutation ${Name} {\n  ${}\n}')
  ],{lineComments:['#']}),
  actionscript:profile(
    'as break case catch class const continue default delete do dynamic each else extends false final finally for function get if implements import in include instanceof interface internal is namespace native new null override package private protected public return set static super switch this throw true try typeof use var void while with',
    [
      snippet('class','class','package ${packageName} {\n\tpublic class ${Name} {\n\t\t${}\n\t}\n}'),
      snippet('function','function','public function ${name}(${params}):${Type} {\n\t${}\n}'),
      snippet('import','import','import ${packageName}.${Type};')
    ],
    {lineComments:['//'],blockComments:[['/*','*/']]}
  ),
  nginx:profile('http server location upstream events stream map geo limit_except if include listen server_name root alias index proxy_pass fastcgi_pass uwsgi_pass scgi_pass return rewrite set try_files error_page access_log error_log gzip ssl',[
    snippet('server','server block','server {\n    listen ${80};\n    server_name ${name};\n    ${}\n}'),
    snippet('location','location block','location ${/} {\n    ${}\n}')
  ],{lineComments:['#']}),
  apache:profile('VirtualHost Directory Location Files IfModule Listen ServerName ServerAlias DocumentRoot DirectoryIndex Allow Deny Require RewriteEngine RewriteCond RewriteRule ProxyPass ProxyPassReverse ErrorLog CustomLog Options AllowOverride',[
    snippet('<VirtualHost>','virtual host','<VirtualHost *:${80}>\n    ServerName ${name}\n    DocumentRoot ${path}\n    ${}\n</VirtualHost>')
  ],{lineComments:['#']}),
  config:profile('true false yes no on off null none',[
    snippet('key','key/value','${KEY}=${value}'),
    snippet('section','section','[${section}]\n${key}=${value}\n${}')
  ],{lineComments:['#',';','!']}),
  xml:profile('',[
    snippet('element','element','<${tag}>${}</${tag}>'),
    snippet('element block','element block','<${tag}>\n  ${}\n</${tag}>')
  ],{blockComments:[['<!--','-->']]}),
  java:profile(
    'abstract assert boolean break byte case catch char class const continue default do double else enum exports extends final finally float for goto if implements import instanceof int interface long module native new non-sealed null open opens package permits private protected provides public record requires return sealed short static strictfp super switch synchronized this throw throws to transient transitive true try uses var void volatile while with yield',
    [
      snippet('class','class','public class ${Name} {\n    ${}\n}'),
      snippet('method','method','public ${ReturnType} ${name}(${params}) {\n    ${}\n}'),
      snippet('main','main method','public static void main(String[] args) {\n    ${}\n}')
    ],
    {lineComments:['//'],blockComments:[['/*','*/']]}
  ),
  php:profile(
    'abstract and array as break callable case catch class clone const continue declare default die do echo else elseif empty enddeclare endfor endforeach endif endswitch endwhile enum eval exit extends final finally fn for foreach function global goto if implements include include_once instanceof insteadof interface isset list match namespace new or print private protected public readonly require require_once return static switch throw trait try unset use var while xor yield',
    [
      snippet('function','function','function ${name}(${params}): ${Type}\n{\n    ${}\n}'),
      snippet('class','class','class ${Name}\n{\n    ${}\n}'),
      snippet('foreach','foreach','foreach ($${items} as $${item}) {\n    ${}\n}')
    ],
    {lineComments:['//','#'],blockComments:[['/*','*/']]}
  ),
  sql:profile('SELECT FROM WHERE JOIN LEFT RIGHT INNER OUTER ON GROUP BY ORDER HAVING LIMIT OFFSET INSERT INTO VALUES UPDATE SET DELETE CREATE ALTER DROP TABLE VIEW INDEX DISTINCT AS AND OR NOT NULL IS IN EXISTS CASE WHEN THEN ELSE END UNION ALL WITH',[
    snippet('SELECT','select query','SELECT ${columns}\nFROM ${table}\nWHERE ${}'),
    snippet('INSERT','insert','INSERT INTO ${table} (${columns})\nVALUES (${values});'),
    snippet('UPDATE','update','UPDATE ${table}\nSET ${column} = ${value}\nWHERE ${};')
  ],{lineComments:['--'],blockComments:[['/*','*/']]}),
  rust:profile(
    'as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while',
    [
      snippet('fn','function','fn ${name}(${params}) -> ${Type} {\n    ${}\n}'),
      snippet('struct','struct','struct ${Name} {\n    ${}\n}'),
      snippet('impl','impl','impl ${Name} {\n    ${}\n}')
    ],
    {lineComments:['//'],blockComments:[['/*','*/']]}
  ),
  cmake:profile('if elseif else endif foreach endforeach while endwhile function endfunction macro endmacro return break continue set unset option include add_executable add_library target_link_libraries target_include_directories find_package project cmake_minimum_required',[
    snippet('function','function','function(${name} ${args})\n  ${}\nendfunction()'),
    snippet('if','if block','if(${condition})\n  ${}\nendif()')
  ],{lineComments:['#']}),
  shell:profile('if then else elif fi for while until do done case esac in function select time coproc readonly local export declare typeset unset shift break continue return',[
    snippet('if','if block','if ${condition}; then\n  ${}\nfi'),
    snippet('for','for loop','for ${item} in ${items}; do\n  ${}\ndone'),
    snippet('function','function','${name}() {\n  ${}\n}')
  ],{lineComments:['#']}),
  nim:profile(
    'addr and as asm bind block break case cast concept const continue converter defer discard distinct div do elif else end enum except export finally for from func if import in include interface is isnot iterator let macro method mixin mod nil not notin object of or out proc ptr raise ref return shl shr static template try tuple type using var when while xor yield',
    [
      snippet('proc','procedure','proc ${name}(${params}): ${Type} =\n  ${}'),
      snippet('func','function','func ${name}(${params}): ${Type} =\n  ${}'),
      snippet('type','object type','type\n  ${Name} = object\n    ${}')
    ],
    {lineComments:['#'],blockComments:[['#[',']#']]}
  ),
  lua:profile('and break do else elseif end false for function goto if in local nil not or repeat return then true until while',[
    snippet('function','function','function ${name}(${params})\n  ${}\nend'),
    snippet('local function','local function','local function ${name}(${params})\n  ${}\nend'),
    snippet('if','if block','if ${condition} then\n  ${}\nend'),
    snippet('for','for loop','for ${item} in ${iterator} do\n  ${}\nend')
  ],{lineComments:['--'],blockComments:[['--[[',']]']]})
};

function languageForFile(file){
  return globalThis.TaskMenuEditor?.languageForPath?.(file?.path||'')||null;
}
function completionProfile(file){
  const language=languageForFile(file);
  return language?.completion?profiles[language.completion]||null:null;
}
function currentLexicalMode(context,syntax){
  const start=Math.max(0,context.pos-8192);
  const text=context.state.sliceDoc(start,context.pos);
  let quote='',escaped=false,lineComment=false,blockClose='';
  for(let i=0;i<text.length;i++){
    const ch=text[i],next=text[i+1]||'';
    if(lineComment){
      if(ch==='\n')lineComment=false;
      continue;
    }
    if(blockClose){
      if(text.startsWith(blockClose,i)){i+=blockClose.length-1;blockClose='';}
      continue;
    }
    if(quote){
      if(escaped){escaped=false;continue;}
      if(ch==='\\'){escaped=true;continue;}
      if(ch===quote)quote='';
      continue;
    }
    if(ch==='\n')continue;
    let found=false;
    for(const [open,close] of syntax?.blockComments||[]){
      if(text.startsWith(open,i)){blockClose=close;i+=open.length-1;found=true;break;}
    }
    if(found)continue;
    for(const marker of syntax?.lineComments||[]){
      if(text.startsWith(marker,i)){lineComment=true;i+=marker.length-1;found=true;break;}
    }
    if(found)continue;
    if(ch==='"'||ch==="'"||syntax?.templateQuote===ch){quote=ch;escaped=false;}
  }
  return lineComment||blockClose?'comment':quote?'string':'code';
}
function completionToken(context){
  return context.matchBefore(/[A-Za-z_#$@][A-Za-z0-9_$@#.-]*$/);
}
function makeKeywordOptions(p){
  return p.keywords.map(label=>({label,type:'keyword',detail:'keyword',boost:20}));
}
function makeSnippetOptions(p){
  return p.snippets.map(item=>globalThis.cm6.snippetCompletion(item.template,{
    label:item.label,detail:item.detail,type:item.type||'keyword',boost:35
  }));
}
function tier1Source(file){
  const p=completionProfile(file);
  if(!p)return ()=>null;
  const options=[...makeSnippetOptions(p),...makeKeywordOptions(p)];
  return context=>{
    if(currentLexicalMode(context,p.syntax)!=='code')return null;
    const token=completionToken(context);
    if(!token&&!context.explicit)return null;
    return {
      from:token?token.from:context.pos,
      options,
      validFor:/[A-Za-z_#$@][A-Za-z0-9_$@#.-]*/
    };
  };
}
function completionPosition(context){
  const line=context.state.doc.lineAt(context.pos);
  return {line:line.number-1,character:context.pos-line.from,linePrefix:line.text.slice(0,context.pos-line.from)};
}
function likelyImportContext(language,linePrefix){
  const id=language?.completion||language?.id||'';
  if(id==='cpp')return /#\s*include\s*[<"][^>"]*$/.test(linePrefix);
  if(id==='javascript'||id==='typescript')return /(?:\bfrom\s+|\bimport\s*|\brequire\s*\()\s*["'][^"']*$/.test(linePrefix);
  if(id==='python')return /(?:^|\s)(?:from|import)\s+[A-Za-z0-9_.]*$/.test(linePrefix);
  if(id==='lua')return /\brequire\s*\(?\s*["'][^"']*$/.test(linePrefix);
  if(id==='nim')return /(?:^|\s)(?:import|include|from)\s+[A-Za-z0-9_./-]*$/.test(linePrefix);
  if(id==='actionscript')return /(?:^|\s)import\s+[A-Za-z0-9_.]*$/.test(linePrefix);
  if(id==='go')return /(?:^|\s)import\s+(?:\(\s*)?["'][^"']*$/.test(linePrefix);
  if(id==='php')return /\b(?:require|include)(?:_once)?\s*\(?\s*["'][^"']*$/.test(linePrefix);
  if(id==='dart')return /\bimport\s+["'][^"']*$/.test(linePrefix);
  return false;
}
function completionOptionType(kind){
  kind=String(kind||'').toLowerCase();
  if(['class','interface','struct','object','message'].includes(kind))return 'class';
  if(['function','func','proc','macro','template','iterator'].includes(kind))return 'function';
  if(['method','rpc'].includes(kind))return 'method';
  if(['constant','const','enum'].includes(kind))return 'constant';
  if(['variable','var','field','property'].includes(kind))return 'variable';
  if(['namespace','module','package','service'].includes(kind))return 'namespace';
  if(['type','trait','scalar','union','input'].includes(kind))return 'type';
  return kind||'text';
}
function tier2Source(file){
  const p=completionProfile(file),language=languageForFile(file);
  if(!p||!language||file?.remote_workspace_id)return ()=>null;
  let controller=null;
  return async context=>{
    const mode=currentLexicalMode(context,p.syntax);
    if(mode==='comment')return null;
    const position=completionPosition(context);
    const importContext=likelyImportContext(language,position.linePrefix);
    if(mode!=='code'&&!importContext)return null;
    const token=completionToken(context);
    const prefix=token?.text||'';
    if(!importContext&&!context.explicit&&prefix.length<2)return null;
    controller?.abort();
    controller=new AbortController();
    context.addEventListener('abort',()=>controller?.abort(),{onDocChange:true});
    let data;
    try{
      data=await app.jsonFetch('/api/project/completions',{
        method:'POST',cache:'no-store',signal:controller.signal,
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({
          path:String(file.path||''),
          language:String(language.completion||language.id||''),
          prefix,
          text:context.state.doc.toString(),
          line_prefix:position.linePrefix,
          lexical_mode:mode,
          position:{line:position.line,character:position.character},
          limit:60
        })
      });
    }catch(error){
      if(error?.name==='AbortError')return null;
      console.warn('Project completion unavailable',error);
      return null;
    }
    const items=Array.isArray(data?.items)?data.items:[];
    if(!items.length)return null;
    const pathMode=items.some(item=>item?.source==='project-path');
    let from=token?token.from:context.pos;
    if(pathMode){
      const replace=String(items.find(item=>item?.replace_prefix)?.replace_prefix||'');
      if(replace)from=Math.max(context.state.doc.lineAt(context.pos).from,context.pos-replace.length);
    }
    return {
      from,
      options:items.map(item=>({
        label:String(item.label||item.insert_text||''),
        displayLabel:String(item.label||item.insert_text||''),
        apply:String(item.insert_text||item.label||''),
        type:completionOptionType(item.kind),
        detail:[item.detail,item.path&&item.path!==item.detail?item.path:'',item.source].filter(Boolean).join(' · '),
        boost:Math.max(-99,Math.min(99,Math.round(Number(item.score||0)/100)))
      })).filter(item=>item.label),
      validFor:pathMode?/[A-Za-z0-9_.$@#\/\\-]*/:/[A-Za-z_#$@][A-Za-z0-9_$@#.-]*/
    };
  };
}
function extensionsForFile(file){
  const p=completionProfile(file);
  if(!p)return [];
  return [globalThis.cm6.autocompletion({
    override:[tier1Source(file),tier2Source(file)],
    activateOnTyping:true,
    activateOnTypingDelay:120,
    maxRenderedOptions:80,
    icons:true
  })];
}
function trigger(view){
  if(!view?.cm)return false;
  return globalThis.cm6.startCompletion(view.cm);
}

globalThis.TaskMenuEditorCompletion={
  profiles,
  completionProfile,
  tier1Source,
  tier2Source,
  likelyImportContext,
  extensionsForFile,
  trigger
};
