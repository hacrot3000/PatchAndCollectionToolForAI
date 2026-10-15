// Pure defensive parser. AI replies and terminal output are untrusted.
(function(root){
'use strict';
function safeCommand(raw){
 if(typeof raw!=='string'||raw.length<1||raw.length>500)return null;
 const command=raw.trim();
 if(!command||/[\r\n\x00-\x1f\x7f]/.test(command))return null;
 // A single literal command per choice: no interpolation, pipes, chaining,
 // redirects or multi-line payloads.
 if(/[;&|<>\x60\\$]/.test(command))return null;
 if(/(^|\s)(?:sudo|su|doas)\s/.test(command))return null;
 if(/\b(?:password|api[_-]?key|secret|token)\s*[:=]/i.test(command))return null;
 if(/^\s*(?:eval|exec)\s/.test(command))return null;
 return command;
}
function riskFloor(command){
 const c=command.toLowerCase();
 if(/^(?:rm|shred|mkfs|dd|wipefs)\s/.test(c)||
    /^git\s+reset\s+--hard\b/.test(c)||
    /^git\s+clean\s+-[a-z]*f/.test(c))return 'critical';
 if(/^git\s+push\s+.*--force/.test(c)||
    /^git\s+branch\s+-d\b/i.test(command)&&/\s-D\b/.test(command)||
    /^git\s+rebase\s+/.test(c))return 'high';
 if(/^git\s+remote\s+(?:remove|set-url)\b/.test(c))return 'medium';
 if(/^(?:pwd|whoami|date|uname|ls|stat|df|du|head|tail)\b/.test(c)||
  /^git\s+(?:status|diff|log|show|branch\s+-(?:a|v|r)|remote\s+-v)\b/.test(c))
  return 'read-only';
 // Unknown commands can modify state, regardless of what the model claims.
 return 'medium';
}
const risks=['read-only','low','medium','high','critical'];
function maxRisk(modelRisk,command){
 const supplied=risks.includes(modelRisk)?modelRisk:'high';
 const floor=riskFloor(command);
 return risks[Math.max(risks.indexOf(supplied),risks.indexOf(floor))];
}
function parseAnswer(input){
 let text=String(input||'').trim();
 const fenced=/^\x60{3}(?:json)?\s*\n([\s\S]*?)\n\x60{3}$/i.exec(text);
 if(fenced)text=fenced[1].trim();
 if(text.length>20000)throw Error('Response too large');
 let json;
 try{json=JSON.parse(text);}catch{throw Error('AI response is not valid JSON; no commands can be executed');}
 if(!json||typeof json!=='object'||Array.isArray(json)||!Array.isArray(json.commands))
  throw Error('Expected JSON object containing commands array');
 if(json.commands.length>8)throw Error('Too many suggested commands (max 8)');
 const errors=[],commands=[];
 for(let i=0;i<json.commands.length;i++){
  const item=json.commands[i];
  if(!item||typeof item!=='object'||Array.isArray(item)){errors.push(i+1);continue;}
  const command=safeCommand(item.command);
  if(!command){errors.push(i+1);continue;}
  if(commands.some(old=>old.command===command))continue;
  commands.push({command,purpose:String(item.purpose||item.reason||'Review proposed command').slice(0,500),
   risk:maxRisk(item.risk,command),
   confirmation_required:true,index:i});
 }
 return {summary:String(json.summary||'').slice(0,3000),commands,
  rejected:errors,format:'taskdeck-terminal-actions/v1'};
}
function canExecute(view,state,app){
 if(!view?.term||!view?.meta?.id||view.closed||view.tabReadOnly||!view.canControl||
  app?.browserLeaseLost||view.meta.status!=='running'||view.ws?.readyState!==1)return false;
 if(app?.sharedMode&&app.hasPermission&&!app.hasPermission('terminal.view_own'))return false;
 if(!state?.seenPrompt||state.executing||state.commandRow==null)return false;
 const buffer=view.term.buffer?.active;
 if(!buffer)return false;
 const row=Number(buffer.baseY||0)+Number(buffer.cursorY||0);
 if(row!==state.commandRow)return false;
 const line=buffer.getLine(row)?.translateToString?.(true)||'';
 const col=Number(state.commandCol||0);
 if(!Number.isFinite(col)||col<0||col>line.length)return false;
 return !line.slice(col).trim();
}
root.TaskDeckAITerminalActionsCore={parseAnswer,safeCommand,canExecute};
if(typeof module==='object'&&module.exports)module.exports=root.TaskDeckAITerminalActionsCore;
})(globalThis);