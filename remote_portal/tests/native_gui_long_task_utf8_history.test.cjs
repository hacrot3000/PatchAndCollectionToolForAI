'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const extracted=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(extracted,'trusted native GUI must expose its actual JavaScript');
const anchor='  let inspectButtons=[];';
assert.ok(extracted[1].includes(anchor),'history injection anchor missing');
const js=extracted[1].replace(anchor,`
  globalThis.__exerciseTaskHistory=(output)=>{
    const serial=rememberTaskRun(55,'dài','running',-1,'',false);
    longTaskHistorySerial=serial;
    updateLongTaskHistory(55,'completed',0,false,output);
    return taskRunHistory.find(x=>x.serial===serial);
  };
  globalThis.__appendActiveOutput=(chunk)=>{
    appendLongTaskOutput(chunk);
    return {text:longTaskText,clipped:longTaskClipped};
  };
  globalThis.__resetActiveOutput=()=>{
    longTaskText='';longTaskClipped=false;
  };
  globalThis.__boundedUTF8Tail=(value,maxBytes)=>
    boundedUTF8HistoryTail(value,maxBytes);
  globalThis.__appendTerminalUnicode=(chunk)=>{
    appendTerminalOutput(chunk);
    return {text:terminalTranscript,clipped:terminalLocalClipped,
      shown:terminalOutput.textContent};
  };
  globalThis.__resetTerminalUnicode=()=>{
    terminalTranscript='';terminalLocalClipped=false;
    terminalOutput.textContent='';
  };
  globalThis.__terminalUnicodeState=()=>({
    text:terminalTranscript,clipped:terminalLocalClipped,
    shown:terminalOutput.textContent,
  });
  globalThis.__captureActiveOutputHistory=()=>{
    const serial=rememberTaskRun(55,'dài','running',-1,'',false);
    longTaskHistorySerial=serial;
    updateLongTaskHistory(55,'completed',0,longTaskClipped,longTaskText);
    return taskRunHistory.find(x=>x.serial===serial);
  };
`+anchor);
class Element{
 constructor(id){this.id=id;this.value='';this.textContent='';this.disabled=false;
 this.hidden=false;this.children=[];this.listeners={};this.attributes={};
 this.classList={toggle(){}};}
 addEventListener(type,fn){this.listeners[type]=fn}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
 append(...children){this.children.push(...children)}
 replaceChildren(...nodes){this.children=nodes}
 click(){assert.ok(this.listeners.click,'missing click: '+this.id);this.listeners.click()}
}
const elements=new Map();
const get=id=>{if(!elements.has(id))elements.set(id,new Element(id));return elements.get(id)};
const token='0123456789abcdef'.repeat(4);
const copied=[];
const sandbox={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{confirm(){return true},addEventListener(){}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 navigator:{clipboard:{writeText(value){copied.push(value);return Promise.resolve()}}},
 fetch:async(endpoint)=>{
  assert.equal(endpoint,'/api/session');
  return {ok:true,json:async()=>({ok:true,closed:false,remaining:16,
   expires_in_seconds:300,device_id:'a'.repeat(64),username:'alice'})};
 },
 Date,Number,String,Promise,setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
new vm.Script(js,{filename:'native-utf8-history.js'}).runInContext(sandbox);
const flush=async()=>{for(let n=0;n<70;n++)await Promise.resolve()};
(async()=>{
 await flush();
 // 1024 emoji = 4096 UTF-8 bytes, but only 2048 UTF-16 code units.
 // The history tail is exactly 512 whole emoji, not 1024.
 const emoji='😀';
 let item=sandbox.__exerciseTaskHistory(emoji.repeat(1024));
 assert.equal(item.output,emoji.repeat(512));
 assert.equal(Buffer.byteLength(item.output,'utf8'),2048);
 assert.equal(item.truncated,true);
 let entry=get('task-run-history-list').children[0];
 assert.equal(entry.children[1].textContent,emoji.repeat(512));
 entry.children[0].children[1].click();await flush();
 assert.equal(copied.at(-1),emoji.repeat(512));
 // Old .slice(-2048) started at the LOW surrogate of this emoji.
 // New tail strips that whole rune and preserves the ASCII suffix.
 item=sandbox.__exerciseTaskHistory(emoji+'x'.repeat(2047));
 assert.equal(item.output,'x'.repeat(2047));
 assert.doesNotMatch(item.output,/\uFFFD/);
 assert.equal(Buffer.byteLength(item.output,'utf8'),2047);
 entry=get('task-run-history-list').children[0];
 entry.children[0].children[1].click();await flush();
 assert.equal(copied.at(-1),'x'.repeat(2047));
 // Exactly 2048 UTF-8 bytes must not be marked clipped or mangled.
 item=sandbox.__exerciseTaskHistory(emoji+'x'.repeat(2044));
 assert.equal(item.output,emoji+'x'.repeat(2044));
 assert.equal(Buffer.byteLength(item.output,'utf8'),2048);
 assert.equal(item.truncated,false);
 // Multi-byte Vietnamese input at an exact two-byte boundary.
 item=sandbox.__exerciseTaskHistory('é'.repeat(1025));
 assert.equal(item.output,'é'.repeat(1024));
 assert.equal(item.truncated,true);
 assert.equal(Buffer.byteLength(item.output,'utf8'),2048);
 // A malformed lone UTF-16 surrogate can never be returned as an
 // apparently valid output tail, even in a synthetic internal call.
 let malformed=sandbox.__boundedUTF8Tail('left\uD800right',2048);
 assert.equal(malformed.text,'right');
 assert.equal(malformed.clipped,true);
 malformed=sandbox.__boundedUTF8Tail('left\uDC00right',2048);
 assert.equal(malformed.text,'right');
 assert.equal(malformed.clipped,true);
 malformed=sandbox.__boundedUTF8Tail('left\uD800',2048);
 assert.equal(malformed.text,'');
 assert.equal(malformed.clipped,true);
 // The active panel must preserve the latest *UTF-8 bytes*, not the
 // earliest UTF-16 units (which previously lost recent output).
 let active=sandbox.__appendActiveOutput(emoji.repeat(4096));
 assert.equal(Buffer.byteLength(active.text,'utf8'),16384);
 assert.equal(active.clipped,false);
 active=sandbox.__appendActiveOutput('\nLATEST'+emoji);
 assert.equal(active.clipped,true,'local 16 KiB overflow was not recorded');
 assert.ok(active.text.endsWith('\nLATEST'+emoji),
  'append after 16 KiB silently discarded the newest host diagnostics');
 assert.ok(!active.text.includes('\uFFFD'),'active transcript broke Unicode');
 assert.ok(Buffer.byteLength(active.text,'utf8')<=16384,
  'active transcript exceeded its UTF-8 byte budget');
 // Cross a surrogate boundary: preserve the LAST complete emoji and
 // the exact final 16 KiB rather than splitting the trailing rune.
 sandbox.__resetActiveOutput();
 active=sandbox.__appendActiveOutput('x'.repeat(16383)+emoji);
 assert.equal(active.text,'x'.repeat(16380)+emoji);
 assert.equal(Buffer.byteLength(active.text,'utf8'),16384);
 assert.equal(active.clipped,true);
 const clippedItem=sandbox.__captureActiveOutputHistory();
 assert.equal(clippedItem.truncated,true,
  'active 16 KiB clipping did not reach retained history');
 assert.equal(Buffer.byteLength(clippedItem.output,'utf8'),2048);
 assert.ok(clippedItem.output.endsWith(emoji),
  'retained history lost the final complete emoji');
 // Terminal has its own rolling, short-lived display. Its 8 KiB bound
 // must count actual UTF-8 bytes, NEVER UTF-16 code units or half-surrogates.
 let terminal=sandbox.__appendTerminalUnicode(emoji.repeat(2048));
 assert.equal(terminal.text,emoji.repeat(2048));
 assert.equal(Buffer.byteLength(terminal.text,'utf8'),8192);
 assert.equal(terminal.clipped,false);
 terminal=sandbox.__appendTerminalUnicode('LATEST'+emoji);
 assert.equal(terminal.text,emoji.repeat(2045)+'LATEST'+emoji);
 assert.equal(Buffer.byteLength(terminal.text,'utf8'),8190);
 assert.equal(terminal.clipped,true);
 assert.equal(terminal.shown,terminal.text);
 assert.ok(!terminal.text.includes('\uFFFD'),
  'Terminal GUI split a UTF-16 surrogate during transcript compaction');
 sandbox.__resetTerminalUnicode();
 terminal=sandbox.__appendTerminalUnicode('x'.repeat(8191)+emoji);
 assert.equal(terminal.text,'x'.repeat(8188)+emoji);
 assert.equal(Buffer.byteLength(terminal.text,'utf8'),8192);
 assert.equal(terminal.clipped,true);
 sandbox.__resetTerminalUnicode();
 terminal=sandbox.__appendTerminalUnicode('ế'.repeat(4000));
 assert.ok(Buffer.byteLength(terminal.text,'utf8')<=8192);
 assert.ok(terminal.text.endsWith('ế'));
 assert.ok(!terminal.text.includes('\uFFFD'));
 const detached=get('task-run-history-list').children[0].children[0].children[1];
 get('disconnect').click();await flush();
 const logoutTerminal=sandbox.__terminalUnicodeState();
 assert.equal(logoutTerminal.text,'','Terminal transcript survived logout');
 assert.equal(logoutTerminal.shown,'','Terminal DOM survived logout');
 assert.equal(logoutTerminal.clipped,false,'Terminal clipping flag survived logout');
 const count=copied.length;
 detached.click();await flush();
 assert.equal(copied.length,count,'logout left a detached sensitive copy action active');
 console.log('PASS: UTF-8 bounded task history and Terminal display preserve Unicode and logout');
})().catch(err=>{console.error(err);process.exitCode=1});
