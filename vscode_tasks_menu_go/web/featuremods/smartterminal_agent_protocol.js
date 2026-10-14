// Capture fixed shell metadata commands without mixing output into the rendered PTY.
(function(root){
'use strict';
function create(marker,maxBytes=1<<20){
 if(!/^TDIDX_[A-Za-z0-9_-]{10,80}$/.test(marker))throw Error('invalid reload marker');
 let buffer='',started=false,done=false;
 function feed(raw){
  if(done)return {completed:true,tail:String(raw||''),body:''};
  buffer+=String(raw||'');
  if(buffer.length>maxBytes){done=true;buffer='';return {completed:true,error:'OS index response exceeded the size limit',body:'',tail:''};}
  if(!started){
   const begin='\n'+marker+'_BEGIN\n';
   const offset=buffer.replace(/\r\n/g,'\n').indexOf(begin);
   if(offset<0)return {completed:false,tail:''};
   buffer=buffer.replace(/\r\n/g,'\n').slice(offset+begin.length);
   started=true;
  }
  const end='\n'+marker+'_END\n',offset=buffer.indexOf(end);
  if(offset<0)return {completed:false,tail:''};
  const body=buffer.slice(0,offset),tail=buffer.slice(offset+end.length);
  done=true;buffer='';
  return {completed:true,body,tail};
 }
 return {feed,get completed(){return done}};
}
function parse(kind,body){
 const out=[],seen=new Set();
 for(const raw of String(body||'').split('\n')){
  let line=raw.replace(/\r/g,'').trim();
  if(kind==='history')line=line.replace(/^\d+\s+/,'');
  if(!line||line.length>1024||/[\x00-\x1f\x7f]/.test(line)||seen.has(line))continue;
  // Avoid persisting obvious credentials (not a substitute for user review).
  if(/(?:password|passwd|api[_-]?key|token|secret)\s*(?:=|:|\s)/i.test(line))continue;
  seen.add(line);out.push(line);
  if(out.length>=4000)break;
 }
 return out;
}
root.TaskDeckAgentIndexProtocol={create,parse};
if(typeof module==='object'&&module.exports)module.exports=root.TaskDeckAgentIndexProtocol;
})(globalThis);
