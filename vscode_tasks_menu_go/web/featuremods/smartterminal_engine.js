// Pure ranked suggestion engine: no network, shell execution or third-party dependency.
(function(root){
'use strict';
const examples=[
 'ls -la','ls -lh','ls -lah','pwd','cd ..','git status','git log --oneline -10',
 'git diff','git branch -avv','git pull --ff-only','git fetch --all --prune',
 'docker ps -a','docker compose ps','docker compose logs --tail=100',
 'systemctl status','journalctl -u','supervisorctl status',
 'ps aux','df -h','du -sh','tail -f','grep -R','find . -type f'
];
const byContext={
 'git':['status','diff','add','commit','pull','push','fetch','log','branch','checkout','switch','restore','show','merge','rebase','stash'],
 'docker':['ps','images','logs','exec','inspect','stats','compose'],
 'docker compose':['ps','logs','up -d','down','restart','build'],
 'systemctl':['status','restart','start','stop','reload','list-units'],
 'supervisorctl':['status','restart','start','stop','reread','update'],
 'kubectl':['get pods','get svc','describe','logs','apply -f','rollout status'],
 'git remote':['-v','add','set-url']
};
const safe=v=>typeof v==='string'&&v.length<=4096&&
 !/[\r\n\x00-\x08\x0b-\x1f\x7f]/.test(v)&&
 !/(?:password|passwd|secret|token|api[_-]?key|authorization)\s*(?:=|:|\s)/i.test(v);
// Short inputs must match a real token or token prefix. For example, "ls"
// must NEVER match the last two letters of "tools".
const tokenBoundary=char=>!char||/[\s"'\`=\/:;|,&(){}[\]._+-]/.test(char);
function relevance(typed,command){
 const a=typed.trim().toLowerCase(),b=command.toLowerCase();
 if(!a||!b||a===b)return -1;
 // Exact command + arguments outranks a command with the same initial letters
 // (e.g. "ls -la" above "lsof").
 if(b.startsWith(a))return tokenBoundary(b[a.length])?6:5;
 // Match an entire argument/verb or a word beginning, not a suffix from
 // inside another word. Even matches in the middle/end of the command count.
 for(let start=b.indexOf(a);start!==-1;start=b.indexOf(a,start+1)){
  if(tokenBoundary(b[start-1]))return tokenBoundary(b[start+a.length])?4:3;
 }
 // Longer inputs can still match meaningful infixes, but they rank below
 // real command/word prefixes, irrespective of how recently they were run.
 if(a.length>=3&&b.includes(a))return 1;
 return -1;
}
function contextual(typed){
 const text=String(typed||'').trim(),keys=Object.keys(byContext).sort((a,b)=>b.length-a.length);
 for(const key of keys){
  if(text===key||text.startsWith(key+' ')){
   return byContext[key].map(s=>key+' '+s);
  }
 }
 return [];
}
// Relevance first; for equally relevant matches: current tab > project > OS history > OS commands > builtins.
function rank(typed,groups,limit=9){
 typed=String(typed||'');
 if(!typed.trim()||typed.length>4096)return [];
 const inputs=[groups?.current,groups?.project,groups?.history,groups?.commands,contextual(typed).concat(examples)];
 const sources=['terminal','project','history','command','builtin'];
 const out=[],seen=new Set();
 for(let source=0;source<inputs.length;source++){
  const items=Array.isArray(inputs[source])?inputs[source]:[];
  for(let n=0;n<Math.min(20000,items.length);n++){
   const item=items[n],value=typeof item==='string'?item:String(item?.command||'');
   if(!safe(value)||seen.has(value))continue;
   const score=relevance(typed,value);
   if(score<0)continue;
   seen.add(value);
   out.push({command:value,source:sources[source],group:source,score,
    recent:typeof item==='object'?Number(item?.finishedAt||item?.finished_at||item?.startedAt||0):0,
    order:n});
  }
 }
 out.sort((a,b)=>b.score-a.score||a.group-b.group||b.recent-a.recent||a.order-b.order||a.command.localeCompare(b.command));
 return out.slice(0,Math.max(1,Math.min(40,Number(limit)||9)));
}
// Shell command-line parsing for *suggestion only*. No glob expansion,
// variable interpolation, shell execution, or filesystem access occurs here.
const pathCommands=new Set(['cp','mv','cd','ls','ll','cat','less','more','head','tail','vim','vi','nano','code','mkdir','rmdir','rm','touch','du','stat','file','chmod','chown','find','tar','rsync','scp','diff','patch','source','.','bash','sh','python','python3','node','grep','rg']);
const directoryCommands=new Set(['cd','rmdir']);
function parseWords(line){
 const words=[];let raw='',decoded='',quote='',start=0,active=false;
 let segment=0;let escaped=false,valid=true;
 const push=()=>{
  if(active)words.push({raw,decoded,start,quote});
  raw='';decoded='';quote='';active=false;
 };
 for(let i=0;i<line.length;i++){
  const ch=line[i];
  if(!active){start=i;active=true;}
  raw+=ch;
  if(escaped){decoded+=ch;escaped=false;continue;}
  if(ch==='\\'&&quote!=="'"){escaped=true;continue;}
  if(ch==="'"){
   if(quote==="'")quote='';
   else if(!quote)quote="'";
   else decoded+=ch;
   continue;
  }
  if(ch==='"'){
   if(quote==='"')quote='';
   else if(!quote)quote='"';
   else decoded+=ch;
   continue;
  }
  if(!quote&&/[\s]/.test(ch)){raw=raw.slice(0,-1);push();continue;}
  if(!quote&&/[;|&<>]/.test(ch)){
   raw=raw.slice(0,-1);push();words.length=0;segment=i+1;active=false;continue;
  }
  decoded+=ch;
 }
 if(escaped)valid=false; // Never suggest while an escape sequence is incomplete.
 const atBoundary=!active;
 if(active&&valid)push();
 return {words,atBoundary,valid,segment};
}
// Alias declarations originate from 'alias -p' in the CURRENT terminal.
// Ignore disabled/commented aliases, control chars, secrets and nontrivial
// expansions. Alias lookup is never sent back to the shell as executable text.
function parseAliases(lines){
 const aliases=Object.create(null);
 for(const value of Array.isArray(lines)?lines.slice(0,400):[]){
  const line=String(value||'');
  const match=/^alias ([^\s=]+)=(['"])(.*)\2$/.exec(line);
  if(!match||!safe(line)||match[1].length>64)continue;
  const name=match[1];
  // Bash 'alias -p' quotes single-quotes using '\''' fragments. Those
  // complex expansions are not reliably comparable and remain history-only.
  const expansion=match[3];
  if(expansion.length>256||/[\r\n\x00-\x1f\x7f]/.test(expansion)||
     /[$\x60;|&<>{}()]/.test(expansion))continue;
  aliases[name]=expansion;
 }
 return aliases;
}
function aliasCommand(verb,aliases){
 let expanded=verb;
 const seen=new Set();
 for(let i=0;i<4;i++){
  if(!aliases||!Object.prototype.hasOwnProperty.call(aliases,expanded)||seen.has(expanded))break;
  seen.add(expanded);
  const expansion=String(aliases[expanded]||'').trim();
  if(!expansion||/[\r\n;$\x60|&<>{}()]/.test(expansion))break;
  expanded=expansion;
 }
 return expanded;
}
function effectiveCommand(verb,aliases){
 const command=aliasCommand(verb,aliases);
 const pieces=command.split(/\s+/).filter(Boolean);
 if(pieces[0]==='sudo'&&pieces.length>1)return pieces.slice(1).join(' ');
 if(pieces[0]==='env'){
  const idx=pieces.findIndex((v,i)=>i>0&&!v.includes('='));
  return idx>0?pieces.slice(idx).join(' '):command;
 }
 return command;
}
function branchContext(typed,aliases){
 typed=String(typed||'');
 if(!typed||typed.length>1024||/[\r\n\x00-\x1f\x7f]/.test(typed))return null;
 const parsed=parseWords(typed),words=parsed.words;
 if(!parsed.valid||!words.length)return null;
 const resolved=effectiveCommand(words[0].decoded,aliases);
 const base=resolved.split(/\s+/).filter(Boolean);
 const isGit=base[0]==='git';
 const verb=isGit?(base[1]||words[1]?.decoded):(base[0]==='gc-safe'?'checkout':'');
 const offset=isGit&&base.length===1?2:1;
 if(!['switch','checkout','merge','rebase','branch'].includes(verb))return null;
 if(words.length<offset)return null;
 // No operand yet must still suggest branches; for branch use --list safely.
 const hasOperand=words.length>offset&&!parsed.atBoundary;
 const final=hasOperand?words[words.length-1]:{raw:'',decoded:'',start:typed.length};
 if(final.decoded.startsWith('-')||/[\/$\x60*?[{}]/.test(final.decoded)||
    final.raw.endsWith("'")||final.raw.endsWith('"'))return null;
 const flags=words.slice(offset,hasOperand?-1:undefined).map(v=>v.decoded);
 const destructive=flags.some(flag=>['-d','-D','--delete','-m','-M','--move','--set-upstream-to'].includes(flag));
 const explicitList=flags.some(flag=>['--list','-l','-a','--all','-r','--remotes'].includes(flag));
 if(verb==='branch'&&flags.some(flag=>flag.startsWith('-')&&!destructive&&!explicitList&&
    !['-v','-vv','--verbose','--sort'].includes(flag)))return null;
 return {prefix:final.decoded,quote:final.raw.startsWith("'")?"'":final.raw.startsWith('"')?'"':'',
  wordStart:final.start,verb,mode:verb==='branch'&&!destructive&&!explicitList?'safe-list':'operand',
  listExplicit:explicitList,hasOperand,needsSpace:!hasOperand&&!/\s$/.test(typed)};
}
function rankBranches(typed,branches,aliases,limit=10){
 const ctx=branchContext(typed,aliases);
 if(!ctx||!Array.isArray(branches))return [];
 const exact=[],folded=[];
 for(const branch of branches.slice(0,500)){
  const name=String(branch||'');
  if(!name||name==='HEAD'||!safe(name)||name.includes(' ')||name.startsWith('-'))continue;
  const starts=name.startsWith(ctx.prefix),fold=name.toLowerCase().startsWith(ctx.prefix.toLowerCase());
  if(!starts&&!fold)continue;
  let candidate;
  if(ctx.mode==='safe-list'){
   // Plain git branch NAME creates a new branch. Always add --list.
   const base=typed.slice(0,ctx.wordStart).trimEnd();
   candidate=base+' --list '+escapedPathSuffix(name,ctx.quote);
  }else if(starts){
   candidate=typed+(ctx.needsSpace?' ':'')+escapedPathSuffix(name.slice(ctx.prefix.length),ctx.quote);
  }else{
   candidate=typed.slice(0,ctx.wordStart)+(ctx.needsSpace?' ':'')+(ctx.quote||'')+
    escapedPathSuffix(name,ctx.quote);
  }
  if(!safe(candidate)||candidate===typed)continue;
  (starts?exact:folded).push({command:candidate,source:'branch',group:-2,score:starts?15:11});
 }
 return exact.concat(folded).slice(0,Math.max(1,Math.min(40,Number(limit)||10)));
}


// List the common safe Git branch inspection flags when the user types the
// bare verb (or its alias). Branch names themselves are supplied by Git.
function rankBranchOptions(typed,aliases){
 const ctx=branchContext(typed,aliases);
 if(!ctx||ctx.verb!=='branch'||ctx.mode!=='safe-list'||ctx.hasOperand||ctx.prefix)return [];
 const base=typed.trimEnd();
 return ['-a','-vv','--list','--show-current'].map(option=>({
  command:base+' '+option,source:'git option',group:-2,score:10
 }));
}

function pathContext(typed,aliases){
 typed=String(typed||'');
 if(!typed||typed.length>1024||/[\r\n\x00-\x1f\x7f]/.test(typed))return null;
 const parsed=parseWords(typed);
 if(!parsed.valid||parsed.words.length<1)return null;
 const {words,atBoundary}=parsed;
 const verb=words[0]?.decoded||'';
 const resolved=effectiveCommand(verb,aliases);
 let command=resolved.split(/\s+/)[0]||verb;
 // Support "sudo cp ..." and "env VAR=value cp ..." without executing them.
 let argStart=1;
 if(resolved.startsWith('git ')){const parts=resolved.split(/\s+/);command='git';argStart=1;}
 if(command==='sudo'){
  const idx=words.findIndex((w,i)=>i>0&&!w.decoded.startsWith('-'));
  if(idx<0)return null;command=words[idx].decoded;argStart=idx+1;
 }else if(command==='env'){
  const idx=words.findIndex((w,i)=>i>0&&!w.decoded.includes('=')&&!w.decoded.startsWith('-'));
  if(idx<0)return null;command=words[idx].decoded;argStart=idx+1;
 }
 if(command==='git'){
  const verbIndex=argStart;
  const aliasSub=resolved.startsWith('git ')?resolved.split(/\s+/)[1]:'';
  const sub=aliasSub||words[verbIndex]?.decoded||'';
  if(!['add','restore','rm','mv','diff','show'].includes(sub))return null;
  command='git '+sub;argStart=verbIndex+(aliasSub?0:1);
 }else if(!pathCommands.has(command))return null;
 // No filename typed yet is a valid completion context (cp, cp -r , ls,
 // ga/git add). Never mistake an option currently being typed for a filename.
 const hasOperand=words.length>argStart&&!atBoundary;
 const word=hasOperand?words[words.length-1]:{raw:'',decoded:'',start:typed.length};
 if(word.decoded.startsWith('-')||/[$\x60*?[{}]/.test(word.decoded))return null;
 // The typed token can contain path separators; request only the immediate
 // containing directory and the final filename prefix.
 const slash=word.decoded.lastIndexOf('/');
 const dir=slash<0?'':word.decoded.slice(0,slash+1),prefix=word.decoded.slice(slash+1);
 // An empty prefix enumerates immediate entries in the current CWD.
 if(dir.length>512||prefix.length>160)return null;
 if(word.raw.endsWith("'")||word.raw.endsWith('"'))return null;
 const quote=word.raw.startsWith("'")?"'":word.raw.startsWith('"')?'"':'';
 const mustDir=directoryCommands.has(command);
 return {dir,prefix,quote,mustDir,command,wordStart:word.start,rawWord:word.raw,needsSpace:!hasOperand&&!/\s$/.test(typed)};
}
function escapedPathSuffix(name,quote){
 if(quote==="'")return name.includes("'")?'':name;
 if(quote==='"')return name.replace(/[\\"$\x60]/g,'\\$&');
 // Escape shell metacharacters including whitespace; never create a second
 // argument or allow command substitution from an untrusted filename.
 return name.replace(/[^A-Za-z0-9_./@%+=:,~-]/g,'\\$&');
}
function rankPaths(typed,entries,limit=12,aliases=null){
 const context=pathContext(typed,aliases);
 if(!context||!Array.isArray(entries))return [];
 const exact=[],folded=[];
 for(const entry of entries.slice(0,150)){
  const name=String(entry?.name||''),isDir=entry?.dir===true;
  if(!name||name.includes('/')||name.includes('\n')||name.includes('\r')||name.includes('\x00')||
     (context.mustDir&&!isDir))continue;
  if(!name.startsWith(context.prefix)){
   if(!name.toLowerCase().startsWith(context.prefix.toLowerCase()))continue;
  }
  const end=isDir?'/':'';
  const remainder=name.slice(context.prefix.length)+end;
  const appended=escapedPathSuffix(remainder,context.quote);
  if(!appended||name===context.prefix&&!end)continue;
  const prefixMatch=name.startsWith(context.prefix);
  const value=prefixMatch?typed+(context.needsSpace?' ':'')+appended:typed.slice(0,context.wordStart)+
   (context.quote||'')+escapedPathSuffix(context.dir+name+end,context.quote);
  if(!safe(value))continue;
  (prefixMatch?exact:folded).push({command:value,source:isDir?'directory':'file',group:-1,score:prefixMatch?12:9,dir:isDir});
 }
 const sort=(a,b)=>Number(b.dir)-Number(a.dir)||a.command.localeCompare(b.command);
 return exact.sort(sort).concat(folded.sort(sort)).slice(0,Math.max(1,Math.min(40,Number(limit)||12)));
}

// Only enumerate executable names when completing the FIRST command word.
// Never treat arguments, paths, quoted strings, expansions or TUI output as
// executable-name lookups. The server returns verified PATH entries.
function commandPrefix(typed){
 const input=String(typed||'');
 if(input.length<2||input.length>96||!/^[A-Za-z_][A-Za-z0-9_.+-]*$/.test(input))return null;
 return input;
}
function rankExecutables(typed,commands,limit=10){
 const prefix=commandPrefix(typed);
 if(!prefix||!Array.isArray(commands))return [];
 const exact=[],folded=[],seen=new Set();
 for(const value of commands.slice(0,400)){
  const name=String(value||'');
  if(!/^[A-Za-z_][A-Za-z0-9_.+-]{0,127}$/.test(name)||seen.has(name)||name===prefix)continue;
  seen.add(name);
  if(name.startsWith(prefix))exact.push(name);
  else if(name.toLowerCase().startsWith(prefix.toLowerCase()))folded.push(name);
 }
 const make=(name,score)=>({command:name,source:'PATH',group:-3,score});
 const closest=(a,b)=>a.length-b.length||a.localeCompare(b);
 return exact.sort(closest).map(x=>make(x,17)).concat(folded.sort(closest).map(x=>make(x,13)))
  .slice(0,Math.max(1,Math.min(30,Number(limit)||10)));
}
function suffix(typed,entry){
 const command=String(entry?.command||'');
 return command.startsWith(String(typed||''))?command.slice(String(typed||'').length):'';
}
root.TaskDeckCompletionEngine={rank,relevance,suffix,examples,pathContext,rankPaths,parseAliases,aliasCommand,effectiveCommand,branchContext,rankBranches,rankBranchOptions,commandPrefix,rankExecutables};
if(typeof module==='object'&&module.exports)module.exports=root.TaskDeckCompletionEngine;
})(globalThis);
