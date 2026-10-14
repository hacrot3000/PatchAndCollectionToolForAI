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
function suffix(typed,entry){
 const command=String(entry?.command||'');
 return command.startsWith(String(typed||''))?command.slice(String(typed||'').length):'';
}
root.TaskDeckCompletionEngine={rank,relevance,suffix,examples};
if(typeof module==='object'&&module.exports)module.exports=root.TaskDeckCompletionEngine;
})(globalThis);
