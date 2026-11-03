import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { TextEncoder } from "node:util";

const nativeSource=fs.readFileSync(new URL("./native_gui.js",import.meta.url),"utf8");

class Element {
 constructor(id="",dataset={}){
  this.id=id;
  this.dataset=dataset;
  this.listeners=new Map();
  this.children=[];
  this._text="";
  this.value="";
  this.disabled=false;
  this.classList={add(){},remove(){},toggle(){}};
 }
 get textContent(){return this._text;}
 set textContent(value){this._text=String(value);this.children=[];}
 replaceChildren(...nodes){this.children=[...nodes];this._text="";}
 append(node){this.children.push(node);}
 addEventListener(event,handler){
  // Match browser semantics when input has multiple security listeners.
  const previous=this.listeners.get(event);
  this.listeners.set(event,previous?(...args)=>{previous(...args);return handler(...args);}:handler);
 }
 click(){return this.listeners.get("click")?.();}
}

function nativeHarness(responses,options={}){
 const elements=new Map(),actions=new Map();
 const all=(id)=>{
  if(!elements.has(id))elements.set(id,new Element(id));
  return elements.get(id);
 };
 const opNames=["files_list","file_view","file_lines","file_find_lines",
  "file_edit_review","line_write_review","git_status","git_log",
  "git_branches","git_remotes","git_tracking_refs","git_diff","git_staged","git_head","git_review","git_write_review","tasks",
  "task_review","task_status","task_output"];
 for(const name of opNames)actions.set(name,new Element("",{op:name}));
 const panels=["files","git","tasks","console"].map(panel=>new Element("",{panel}));
 const documents=["files","git","tasks","console"].map(x=>all("panel-"+x));
 all("directory").value=".";
 const controlNames=["execute-task","cancel-task","execute-git",
  "execute-file-edit","execute-line-edit","console-run"];
 const controls=controlNames.map(id=>all(id));
 const document={
  getElementById:all,
  createElement:()=>new Element(),
  querySelectorAll(selector){
   if(selector==="[data-panel]"||selector===".nav")return panels;
   if(selector===".panel")return documents;
   if(selector==="[data-op]")return [...actions.values()];
   if(selector.startsWith("[data-op],"))return [...actions.values(),...controls];
   return [];
  }
 };
 const calls=[],pending=[...responses];
 let used=0;
 const fetch=async(path,options)=>{
  const request=JSON.parse(options.body);
  calls.push({path,request});
  let next=pending.shift();
  if(!next)throw new Error("Unexpected E2EE request");
  if(typeof next==="function")next=await next();
  return {ok:true,async json(){return {ok:true,...(typeof next==="string"?{text:next}:next),used:(used+=(request.operation==="git_review"?2:1)),max:6};}};
 };
 const context={document,fetch,TextEncoder,decodeURIComponent,
  location:{hash:"#"+("a".repeat(64)),pathname:"/"},
  history:{replaceState(){}},console,
  confirm(){if(options.confirm===true)return true;throw new Error("Unexpected confirmation");},
  prompt(){throw new Error("Unexpected modal");}};
 vm.runInNewContext(nativeSource,context,{filename:"native_gui.js"});
 return {all,actions,calls};
}

test("directory picker requires explicit authenticated browse for subfolders",async()=>{
 const h=nativeHarness(["D\tsrc\nF\tREADME.md"]);
 await h.actions.get("files_list").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"files_list");
 const entries=h.all("workspace-entries");
 assert.equal(entries.children.length,2);
 await entries.children[0].click();
 assert.equal(h.all("directory").value,"src");
 assert.equal(h.calls.length,1);
 assert.equal(entries.children.length,0);
});

test("file selection only fills preview and Git paths; no implicit command",async()=>{
 const h=nativeHarness(["F\tREADME.md"]);
 await h.actions.get("files_list").click();
 assert.equal(h.all("workspace-entries").children.length,1);
 await h.all("workspace-entries").children[0].click();
 assert.equal(h.all("file").value,"README.md");
 assert.equal(h.all("git-path").value,"README.md");
 assert.equal(h.calls.length,1);
});

test("approved task cards only select an ID; task execution still requires review",async()=>{
 const h=nativeHarness(["1\tHost approved test\n2\tHost approved backup\n"]);
 await h.actions.get("tasks").click();
 const cards=h.all("task-catalog").children;
 assert.equal(cards.length,2);
 await cards[1].click();
 assert.equal(h.all("task-id").value,"2");
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"tasks");
 assert.match(cards[1].textContent,/Host approved backup/);
});

test("malformed or duplicate task catalogue is never clickable",async()=>{
 const h=nativeHarness(["1\tGood\n1\tDuplicate"]);
 await h.actions.get("tasks").click();
 assert.equal(h.all("task-catalog").children.length,0);
 assert.equal(h.all("task-catalog-count").textContent,"Không hợp lệ");
});


test("approved task search filters locally by name or #ID without an E2EE request",async()=>{
 const h=nativeHarness(["1\tBuild Android\n2\tExport Logs\n3\tBuild iOS\n"]);
 await h.actions.get("tasks").click();
 assert.equal(h.all("task-catalog").children.length,3);
 h.all("task-catalog-filter").value="build";
 h.all("task-catalog-filter").listeners.get("input")();
 assert.equal(h.all("task-catalog").children.length,2);
 assert.match(h.all("task-catalog-filter-summary").textContent,/2\/3/);
 assert.equal(h.calls.length,1,"typing a filter must never reach Agent or PHP");
 h.all("task-catalog-filter").value="#2";
 h.all("task-catalog-filter").listeners.get("input")();
 assert.equal(h.all("task-catalog").children.length,1);
 assert.match(h.all("task-catalog").children[0].textContent,/Export Logs/);
 await h.all("task-catalog").children[0].click();
 assert.equal(h.all("task-id").value,"2");
 assert.match(h.all("task-catalog-selection").textContent,/Export Logs/);
 assert.equal(h.calls.length,1,"choosing a filtered task must never run it");
 await h.all("task-catalog-clear").click();
 assert.equal(h.all("task-catalog-filter").value,"");
 assert.equal(h.all("task-catalog").children.length,3);
 assert.equal(h.calls.length,1);
});

test("task search is literal and never becomes a regex, command or HTML",async()=>{
 const h=nativeHarness(["7\tBuild (nightly) <script>\n8\tExport logs\n"]);
 await h.actions.get("tasks").click();
 h.all("task-catalog-filter").value="(nightly)";
 h.all("task-catalog-filter").listeners.get("input")();
 assert.equal(h.all("task-catalog").children.length,1);
 assert.match(h.all("task-catalog").children[0].textContent,/<script>/);
 h.all("task-catalog-filter").value="[";
 h.all("task-catalog-filter").listeners.get("input")();
 assert.equal(h.all("task-catalog").children.length,0);
 assert.match(h.all("task-catalog").textContent,/Không tìm thấy/);
 assert.equal(h.calls.length,1);
});

test("task catalogue refresh invalidates old selections and fails closed on malformed response",async()=>{
 const h=nativeHarness(["7\tPreviously approved\n","7\tOld\n7\tDuplicate"]);
 await h.actions.get("tasks").click();
 await h.all("task-catalog").children[0].click();
 assert.match(h.all("task-catalog-selection").textContent,/Previously approved/);
 await h.actions.get("tasks").click();
 assert.equal(h.all("task-catalog").children.length,0);
 assert.equal(h.all("task-catalog-count").textContent,"Không hợp lệ");
 assert.match(h.all("task-catalog-selection").textContent,/Chưa có danh mục/);
 assert.equal(h.calls.length,2);
});

test("task catalog fetch rejection clears old cards and local selected name",async()=>{
 const h=nativeHarness(["7\tApproved\n",{ok:false,error:"Task access revoked"}]);
 await h.actions.get("tasks").click();
 await h.all("task-catalog").children[0].click();
 await h.actions.get("tasks").click();
 assert.equal(h.all("task-catalog").children.length,0);
 assert.equal(h.all("task-catalog-count").textContent,"Không tải được");
 assert.doesNotMatch(h.all("task-catalog-selection").textContent,/Approved/);
 assert.equal(h.calls.length,2);
});

test("task filter does not grant manual Task IDs or bypass review and execute",async()=>{
 const h=nativeHarness(["7\tApproved task\n",{text:"Host review",confirm:"RUN TASK 7"},"task completed"]);
 await h.actions.get("tasks").click();
 h.all("task-id").value="999";
 h.all("task-id").listeners.get("input")();
 assert.match(h.all("task-catalog-selection").textContent,/không có trong danh mục/);
 assert.equal(h.calls.length,1);
 await h.all("task-catalog").children[0].click();
 h.all("task-catalog-filter").value="unknown";
 h.all("task-catalog-filter").listeners.get("input")();
 assert.equal(h.calls.length,1);
 h.actions.get("task_review").dataset.mode="run";
 await h.actions.get("task_review").click();
 assert.equal(h.calls[1].request.operation,"task_review");
 assert.equal(h.calls[1].request.task_id,7);
 h.all("approval-entry").value="RUN TASK 7";
 await h.all("execute-task").click();
 assert.equal(h.calls[2].request.operation,"task_execute");
 assert.equal(h.calls[2].request.task_id,7);
});

test("closing remote GUI scrubs locally searched task names and selections",async()=>{
 const h=nativeHarness(["7\tPrivate operation\n",{text:"Closing"}],{confirm:true});
 await h.actions.get("tasks").click();
 h.all("task-catalog-filter").value="Private";
 h.all("task-catalog-filter").listeners.get("input")();
 await h.all("task-catalog").children[0].click();
 await h.all("close").click();
 assert.equal(h.all("task-catalog").children.length,0);
 assert.equal(h.all("task-catalog-filter").value,"");
 assert.doesNotMatch(h.all("task-catalog-selection").textContent,/Private/);
 assert.equal(h.all("task-catalog-clear").disabled,true);
 assert.equal(h.calls.length,2);
});

test("Git status cards reject dangerous paths, renames and quoted path syntax",async()=>{
 const status="## main\n M src/ok.txt\nM  docs/changed.md\n?? ../private\nR  src/old.txt -> src/new.txt\n M \"quoted.txt\"\n";
 const h=nativeHarness([status]);
 await h.actions.get("git_status").click();
 const entries=h.all("git-status-files").children;
 assert.equal(entries.length,2);
 await entries[0].click();
 assert.equal(h.all("git-path").value,"src/ok.txt");
 assert.equal(h.all("file").value,"src/ok.txt");
 assert.equal(h.calls.length,1);
});

test("final E2EE read remains visible when quota is exhausted",async()=>{
 const h=nativeHarness(["result1","result2","result3","result4","result5","trusted final"]);
 for(let i=0;i<6;i++)await h.actions.get("git_log").click();
 assert.equal(h.calls.length,6);
 assert.equal(h.all("output").textContent,"trusted final");
 assert.match(h.all("status").textContent,/Hết lượt E2EE/);
});

test("Git branch cards select safe taskdeck slugs without switching or deleting",async()=>{
 const sha="a".repeat(40);
 const data="* main "+sha+" primary\n  taskdeck/feature-1 "+sha+" supported\n* taskdeck/live "+sha+" current\n  unsafe/name "+sha+" x\n";
 const h=nativeHarness([data]);
 await h.actions.get("git_branches").click();
 const entries=h.all("git-branch-entries").children;
 assert.equal(entries.length,2);
 await entries[0].click();
 assert.equal(h.all("git-branch").value,"feature-1");
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_branches");
});

test("Files editor requires review and exact one-use confirmation before E2EE CAS write",async()=>{
 const confirm="EDIT src/config.txt abcdef012345";
 const h=nativeHarness([{text:"old text\n",confirm},"saved"]);
 h.all("file").value="src/config.txt";
 await h.actions.get("file_edit_review").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.all("file-edit-text").value,"old text\n");
 h.all("file-edit-text").value="new text\n";
 h.all("file-edit-confirm").value=confirm;
 await h.all("execute-file-edit").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].request.operation,"file_edit_execute");
 assert.equal(h.calls[1].request.content,"new text\n");
 assert.equal(h.calls[1].request.confirmation,confirm);
});

test("Files editor refuses incorrect review confirmation before any write",async()=>{
 const h=nativeHarness([{text:"old",confirm:"EDIT sample.txt secret"}]);
 h.all("file").value="sample.txt";
 await h.actions.get("file_edit_review").click();
 h.all("file-edit-text").value="changed";
 h.all("file-edit-confirm").value="wrong";
 await h.all("execute-file-edit").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"file_edit_review");
});

test("Large-file line edit pins requested path, index, exact confirmation and text",async()=>{
 const confirm="EDIT LINE 12 111111111111";
 const h=nativeHarness([{text:"FILE_SHA256 ...",confirm,line:"old"},"line saved"]);
 h.all("file").value="src/large.txt";
 h.all("line").value="12";
 h.actions.get("line_write_review").dataset.mode="edit";
 await h.actions.get("line_write_review").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.all("line-edit-text").value,"old");
 h.all("line-edit-text").value="new";
 h.all("line-edit-confirm").value=confirm;
 await h.all("execute-line-edit").click();
 assert.equal(h.calls.length,2);
 const req=h.calls[1].request;
 assert.deepEqual({operation:req.operation,path:req.path,offset:req.offset,mode:req.mode,content:req.content},
  {operation:"line_write_execute",path:"src/large.txt",offset:12,mode:"edit",content:"new"});
 assert.equal(req.confirmation,confirm);
});

test("Git stage button never mutates before separate review and exact confirmation",async()=>{
 const confirm="STAGE src/main.go";
 const h=nativeHarness([{text:"reviewed file",confirm},"staged"]);
 h.all("git-path").value="src/main.go";
 h.actions.get("git_write_review").dataset.mode="stage";
 await h.actions.get("git_write_review").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_write_review");
 h.all("git-confirm-entry").value=confirm;
 await h.all("execute-git").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].request.operation,"git_write_execute");
 assert.equal(h.calls[1].request.mode,"stage");
 assert.equal(h.calls[1].request.path,"src/main.go");
 assert.equal(h.calls[1].request.confirmation,confirm);
});

test("Task execution requires reviewed approval; clicking catalog item never runs task",async()=>{
 const h=nativeHarness([
  "7\tHost approved test\n",
  {text:"Reviewed host task",confirm:"RUN TASK 7"},
  "task done"
 ]);
 await h.actions.get("tasks").click();
 await h.all("task-catalog").children[0].click();
 assert.equal(h.calls.length,1);
 h.actions.get("task_review").dataset.mode="run";
 await h.actions.get("task_review").click();
 assert.equal(h.calls.length,2);
 h.all("approval-entry").value="RUN TASK 7";
 await h.all("execute-task").click();
 assert.equal(h.calls.length,3);
 assert.equal(h.calls[2].request.operation,"task_execute");
 assert.equal(h.calls[2].request.task_id,7);
});

function taskWindow(handle,offset,next,total,state,eof,data){
 return "TASK_HANDLE "+handle+"\n"+
  "OUTPUT_OFFSET "+offset+"\n"+
  "OUTPUT_NEXT "+next+"\n"+
  "OUTPUT_TOTAL "+total+"\n"+
  "TASK_STATE "+state+"\n"+
  "OUTPUT_EOF "+(eof?"yes":"no")+"\n"+
  "-- TASK OUTPUT WINDOW --\n"+data;
}

test("managed task output pages append only contiguous authenticated offsets",async()=>{
 const handle="a".repeat(32);
 const first=taskWindow(handle,0,5,10,"running",false,"hello");
 const second=taskWindow(handle,5,10,10,"exited",true,"world");
 const h=nativeHarness([first,second]);
 h.all("task-handle").value=handle;
 h.all("task-offset").value="0";
 await h.actions.get("task_output").click();
 assert.equal(h.all("task-transcript").textContent,"hello");
 assert.equal(h.all("task-offset").value,"5");
 assert.equal(h.all("task-read-next").disabled,false);
 await h.all("task-read-next").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].request.operation,"task_output");
 assert.equal(h.calls[1].request.handle,handle);
 assert.equal(h.calls[1].request.offset,5);
 assert.equal(h.all("task-transcript").textContent,"helloworld");
 assert.equal(h.all("task-read-next").disabled,true);
 assert.match(h.all("task-output-progress").textContent,/EOF/);
});

test("output window with mismatched offsets is not displayed or paged",async()=>{
 const handle="b".repeat(32);
 const h=nativeHarness([taskWindow(handle,0,5,5,"exited",true,"bad")]);
 h.all("task-handle").value=handle;
 h.all("task-offset").value="0";
 await h.actions.get("task_output").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.all("task-read-next").disabled,true);
 assert.match(h.all("status").textContent,/Từ chối trang output/);
 assert.doesNotMatch(h.all("task-transcript").textContent,/bad/);
});

test("output next refuses changed task handles without new network operation",async()=>{
 const handle="c".repeat(32);
 const h=nativeHarness([taskWindow(handle,0,3,4,"running",false,"abc")]);
 h.all("task-handle").value=handle;
 h.all("task-offset").value="0";
 await h.actions.get("task_output").click();
 h.all("task-handle").value="d".repeat(32);
 await h.all("task-read-next").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.all("task-read-next").disabled,true);
 assert.match(h.all("status").textContent,/Task đã thay đổi/);
});

test("output does not concatenate noncontiguous pages from a different handle",async()=>{
 const first="e".repeat(32);
 const second="f".repeat(32);
 const h=nativeHarness([
  taskWindow(first,0,3,4,"running",false,"abc"),
  taskWindow(second,0,2,2,"exited",true,"xy")
 ]);
 h.all("task-handle").value=first;
 h.all("task-offset").value="0";
 await h.actions.get("task_output").click();
 h.all("task-handle").value=second;
 h.all("task-offset").value="0";
 await h.actions.get("task_output").click();
 assert.equal(h.all("task-transcript").textContent,"xy");
 assert.equal(h.all("task-read-next").disabled,true);
 assert.equal(h.calls.length,2);
});

test("task output text is escaped and retained only within bounded four pages",async()=>{
 const handle="1".repeat(32);
 const windows=[];
 let offset=0;
 for(let i=0;i<5;i++){
  const data=i===0?"\u001b[31mX":String.fromCharCode(65+i);
  const next=offset+new TextEncoder().encode(data).length;
  windows.push(taskWindow(handle,offset,next,30,"running",false,data));
  offset=next;
 }
 const h=nativeHarness(windows);
 h.all("task-handle").value=handle;
 h.all("task-offset").value="0";
 await h.actions.get("task_output").click();
 assert.match(h.all("task-transcript").textContent,/\\u001b/);
 for(let i=1;i<5;i++)await h.all("task-read-next").click();
 assert.doesNotMatch(h.all("task-transcript").textContent,/\\u001b/);
 assert.equal(h.calls.length,5);
 assert.equal(h.all("task-read-next").disabled,false);
});

function sourcePage(sha,start,next,total,rows){
 return "FILE_SHA256 "+sha+"\n"+
  "START_LINE "+start+"\n"+
  "NEXT_LINE "+next+"\n"+
  "TOTAL_LINES "+total+"\n"+
  "-- COMPLETE SOURCE LINES --\n"+rows;
}

test("source pages concatenate only contiguous matching full-file SHA",async()=>{
 const sha="a".repeat(64);
 const h=nativeHarness([
  sourcePage(sha,1,3,4,"1: foo\n2: bar\n"),
  sourcePage(sha,3,0,4,"3: baz\n4: done\n")
 ]);
 h.all("file").value="src/app.go";
 h.all("line").value="1";
 await h.actions.get("file_lines").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.all("file-read-next").disabled,false);
 assert.equal(h.all("line").value,"3");
 await h.all("file-read-next").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].request.operation,"file_lines");
 assert.equal(h.calls[1].request.path,"src/app.go");
 assert.equal(h.calls[1].request.offset,3);
 assert.equal(h.all("file-page-transcript").textContent,"1: foo\n2: bar\n3: baz\n4: done\n");
 assert.equal(h.all("file-read-next").disabled,true);
 assert.match(h.all("file-page-progress").textContent,/EOF/);
});

test("source paging denies a changed full-file SHA and discards stale page content",async()=>{
 const h=nativeHarness([
  sourcePage("a".repeat(64),1,2,2,"1: safe\n"),
  sourcePage("b".repeat(64),2,0,2,"2: modified\n")
 ]);
 h.all("file").value="src/data.txt";
 h.all("line").value="1";
 await h.actions.get("file_lines").click();
 await h.all("file-read-next").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.all("file-read-next").disabled,true);
 assert.doesNotMatch(h.all("file-page-transcript").textContent,/safe|modified/);
 assert.match(h.all("status").textContent,/Từ chối trang file/);
});

test("malformed source pages do not produce a next cursor",async()=>{
 const sha="c".repeat(64);
 const h=nativeHarness([sourcePage(sha,1,4,4,"1: line\n")]);
 h.all("file").value="notes.txt";
 h.all("line").value="1";
 await h.actions.get("file_lines").click();
 assert.equal(h.all("file-read-next").disabled,true);
 assert.match(h.all("status").textContent,/Từ chối trang file/);
});

test("file changed before next-page click never emits another network request",async()=>{
 const sha="d".repeat(64);
 const h=nativeHarness([sourcePage(sha,1,2,3,"1: hi\n")]);
 h.all("file").value="one.txt";
 h.all("line").value="1";
 await h.actions.get("file_lines").click();
 h.all("file").value="another.txt";
 await h.all("file-read-next").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.all("file-read-next").disabled,true);
 assert.match(h.all("status").textContent,/File đã thay đổi/);
});

function findReport(matches,termSha="b".repeat(64),fileSha="a".repeat(64),total=30){
 const lines=matches.map(([i,text])=>i+": "+text+"\n").join("");
 return "FILE_SHA256 "+fileSha+"\n"+
  "TERM_SHA256 "+termSha+"\n"+
  "TOTAL_LINES "+total+"\n"+
  "MATCHES "+matches.length+"\n"+
  "-- COMPLETE MATCHING LINES --\n"+lines;
}
test("authenticated literal find renders safe selectable source-line buttons without automatic read",async()=>{
 const h=nativeHarness([findReport([[2,"matching text"],[9,"another matching line"]])]);
 h.all("file").value="src/code.go";
 h.all("find-literal").value="matching";
 await h.actions.get("file_find_lines").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"file_find_lines");
 assert.equal(h.calls[0].request.literal,"matching");
 const matches=h.all("find-entries").children;
 assert.equal(matches.length,2);
 await matches[1].click();
 assert.equal(h.all("line").value,"9");
 assert.equal(h.calls.length,1);
 assert.match(h.all("find-summary").textContent,/2 dòng khớp/);
});
test("search results refuse stale file or literal without host actions",async()=>{
 const h=nativeHarness([findReport([[4,"match source"]])]);
 h.all("file").value="src/data.txt";
 h.all("find-literal").value="match";
 await h.actions.get("file_find_lines").click();
 h.all("file").value="src/other.txt";
 await h.all("find-entries").children[0].click();
 assert.equal(h.calls.length,1);
 assert.match(h.all("status").textContent,/Kết quả cũ/);
 assert.notEqual(h.all("line").value,"4");
});
test("tampered search counts and unsorted matching source lines are never clickable",async()=>{
 for(const report of [
  findReport([[2,"match"]]).replace("MATCHES 1","MATCHES 2"),
  findReport([[5,"match"],[3,"match"]]),
  findReport([[1,"match"]]).replace("1: match","1: absent"),
  findReport([[1,"match"]]).replace("FILE_SHA256 ","FILE_SHA256 invalid"),
 ]){
  const h=nativeHarness([report]);
  h.all("file").value="src/secure.txt";
  h.all("find-literal").value="match";
  await h.actions.get("file_find_lines").click();
  assert.equal(h.all("find-entries").children.length,0);
  assert.equal(h.all("find-summary").textContent,"Không hợp lệ");
  assert.equal(h.calls.length,1);
 }
});
test("literal find with no matches renders an empty selector and exact host SHA",async()=>{
 const h=nativeHarness([findReport([])]);
 h.all("file").value="src/empty.txt";
 h.all("find-literal").value="needle";
 await h.actions.get("file_find_lines").click();
 assert.equal(h.all("find-entries").children.length,0);
 assert.match(h.all("find-summary").textContent,/0 kết quả/);
 assert.equal(h.calls.length,1);
});

test("read-only console maps only fixed Git and task metadata commands to E2EE actions",async()=>{
 const h=nativeHarness(["## main\n M readme.md\n","1\tApproved task\n","HEAD aabb"]);
 for(const [cmd,operation] of [
  ["git status","git_status"],
  ["tasks list","tasks"],
  ["git log","git_log"]
 ]){
  h.all("console-command").value=cmd;
  await h.all("console-run").click();
  assert.equal(h.calls[h.calls.length-1].request.operation,operation);
 }
 assert.equal(h.calls.length,3);
 assert.match(h.all("console-transcript").textContent,/HEAD aabb/);
 assert.equal(h.all("console-count").textContent,"3 kết quả gần nhất");
});

test("read-only console allows only canonical bounded Files and managed-task read arguments",async()=>{
 const handle="a".repeat(32);
 const h=nativeHarness(["hello","D\tsrc\n","state","window"]);
 for(const [command,request] of [
  ["files view src/readme.txt",{operation:"file_view",path:"src/readme.txt"}],
  ["files list .",{operation:"files_list",path:"."}],
  ["tasks status "+handle,{operation:"task_status",handle}],
  ["tasks output "+handle+" 42",{operation:"task_output",handle,offset:42}]
 ]){
  h.all("console-command").value=command;
  await h.all("console-run").click();
  const last=h.calls.at(-1).request;
  for(const [key,value] of Object.entries(request))assert.equal(last[key],value);
 }
 assert.equal(h.calls.length,4);
});

test("read-only console denies shell, git write, traversal, injection and malformed handles without E2EE",async()=>{
 const h=nativeHarness([]);
 for(const command of [
  "git push","git checkout main","git status; rm -rf /","git stage README.md",
  "terminal bash","shell ls","files view ../secret","files list /etc",
  "files view src/file;id","files view src/file | cat","files view .git/config",
  "files list src/../private","files list -option",
  "files view README.md\nhelp","tasks output "+"a".repeat(32)+" 0001",
  "tasks output "+"a".repeat(32)+" 1073741825",
  "tasks status "+"A".repeat(32),"tasks run 1","tasks cancel "+"a".repeat(32),
  " files view src/a","files view src/a "
 ]){
  h.all("console-command").value=command;
  await h.all("console-run").click();
  assert.equal(h.calls.length,0,command);
  assert.match(h.all("status").textContent,/Lệnh bị từ chối/);
 }
});

test("console help is native-only and bounded output retains last four escaped responses",async()=>{
 const h=nativeHarness(["first","\u001b[31mred","third","fourth","fifth"]);
 h.all("console-command").value="help";
 await h.all("console-run").click();
 assert.equal(h.calls.length,0);
 assert.match(h.all("console-transcript").textContent,/Không có shell/);
 for(const cmd of ["git status","git status","git status","git status","git status"]){
  h.all("console-command").value=cmd;
  await h.all("console-run").click();
 }
 assert.equal(h.calls.length,5);
 assert.doesNotMatch(h.all("console-transcript").textContent,/first/);
 assert.match(h.all("console-transcript").textContent,/\\u001b/);
 assert.match(h.all("console-transcript").textContent,/fifth/);
 assert.equal(h.all("console-count").textContent,"4 kết quả gần nhất");
});

test("console uses finite E2EE quota and never emits hidden seventh read",async()=>{
 const h=nativeHarness(["a","b","c","d","e","f"]);
 for(let i=0;i<7;i++){
  h.all("console-command").value="git status";
  await h.all("console-run").click();
 }
 assert.equal(h.calls.length,6);
 assert.equal(h.all("console-run").disabled,false,"local commands must remain usable after quota");
 assert.match(h.all("status").textContent,/Hết lượt E2EE/);
});

test("read-only console Enter submits exactly one typed action and refuses repeated key events",async()=>{
 const h=nativeHarness(["read only"]);
 h.all("console-command").value="git status";
 const listener=h.all("console-command").listeners.get("keydown");
 assert.equal(typeof listener,"function");
 let prevented=0;
 await listener({key:"Enter",repeat:true,preventDefault(){prevented++;}});
 assert.equal(h.calls.length,0);
 await listener({key:"Enter",repeat:false,preventDefault(){prevented++;}});
 // Enter calls the button handler; await the async handler's microtask.
 await Promise.resolve();
 await Promise.resolve();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_status");
 assert.equal(prevented,1);
});

test("console maps bounded Git diffs, source lines and literal search to native typed E2EE reads",async()=>{
 const h=nativeHarness(["diff","staged","head","lines","matches"]);
 for(const [command,expected] of [
  ["git diff src/file.go",{operation:"git_diff",path:"src/file.go"}],
  ["git staged docs/README.md",{operation:"git_staged",path:"docs/README.md"}],
  ["git head src/file.go",{operation:"git_head",path:"src/file.go"}],
  ["files lines src/file.go 42",{operation:"file_lines",path:"src/file.go",offset:42}],
  ["files find src/file.go hello world",{operation:"file_find_lines",path:"src/file.go",literal:"hello world"}]
 ]){
  h.all("console-command").value=command;
  await h.all("console-run").click();
  const req=h.calls.at(-1).request;
  for(const [key,value] of Object.entries(expected))assert.equal(req[key],value,command);
 }
 assert.equal(h.calls.length,5);
 assert.equal(h.all("console-count").textContent,"4 kết quả gần nhất");
});

test("console rejects arbitrary terminal syntax and unsafe structured read fields locally",async()=>{
 const h=nativeHarness([]);
 for(const command of [
  "git diff ../passwd","git diff /etc/shadow","git staged .git/config",
  "git head -foo","git diff src/file|id","git diff src/file;ls",
  "files lines src/main.go 0","files lines src/main.go 2097153",
  "files lines src/main.go 001","files lines src/other.go 1;ls",
  "files find src/main.go hello|cat","files find src/main.go hello;id",
  "files find src/main.go hello\u202Eworld",
  "files find src/main.go  leading","files find src/main.go trailing ",
  "git push origin main","git merge main","git rebase main",
  "files view src/file two.txt"
 ]){
  h.all("console-command").value=command;
  await h.all("console-run").click();
  assert.equal(h.calls.length,0,command);
  assert.match(h.all("status").textContent,/Lệnh bị từ chối/);
 }
});

test("console arrow-key history uses only local bounded commands and never dispatches",async()=>{
 const h=nativeHarness(["status","branches","tasks"]);
 const commands=["git status","git branches","tasks list"];
 for(const cmd of commands){
  h.all("console-command").value=cmd;
  await h.all("console-run").click();
 }
 const listener=h.all("console-command").listeners.get("keydown");
 let prevent=0;
 function key(k){listener({key:k,repeat:false,preventDefault(){prevent++;}});}
 key("ArrowUp");assert.equal(h.all("console-command").value,"tasks list");
 key("ArrowUp");assert.equal(h.all("console-command").value,"git branches");
 key("ArrowUp");assert.equal(h.all("console-command").value,"git status");
 key("ArrowUp");assert.equal(h.all("console-command").value,"git status");
 key("ArrowDown");assert.equal(h.all("console-command").value,"git branches");
 key("ArrowDown");assert.equal(h.all("console-command").value,"tasks list");
 key("ArrowDown");assert.equal(h.all("console-command").value,"");
 assert.equal(h.calls.length,3);
 assert.equal(prevent,7);
});

test("console clear erases only local text, not an E2EE action or host workspace",async()=>{
 const h=nativeHarness(["git metadata"]);
 h.all("console-command").value="git status";
 await h.all("console-run").click();
 assert.match(h.all("console-transcript").textContent,/git metadata/);
 h.all("console-command").value="clear";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.doesNotMatch(h.all("console-transcript").textContent,/git metadata/);
 assert.equal(h.all("console-count").textContent,"0 kết quả");
});

test("Git log cards render verified commit subjects with no checkout or new request",async()=>{
 const h=nativeHarness(["a1b2c3d Fix a build\nb2c3d4e Secure remote host\n"]);
 await h.actions.get("git_log").click();
 const cards=h.all("git-log-entries").children;
 assert.equal(cards.length,2);
 assert.equal(h.all("git-log-count").textContent,"2 commit");
 await cards[1].click();
 assert.match(h.all("git-log-selection").textContent,/b2c3d4e · Secure remote host/);
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_log");
});

test("local-only Git log filter never consumes E2EE quota or sends a remote operation",async()=>{
 const h=nativeHarness(["aaaa111 Fix Linux\nbbbb222 Improve macOS\ncccc333 Improve security\n"]);
 await h.actions.get("git_log").click();
 h.all("git-log-filter").value="macOS";
 h.all("git-log-filter").listeners.get("input")();
 assert.equal(h.all("git-log-entries").children.length,1);
 assert.match(h.all("git-log-entries").children[0].textContent,/bbbb222/);
 h.all("git-log-filter").value="cccc";
 h.all("git-log-filter").listeners.get("input")();
 assert.equal(h.all("git-log-entries").children.length,1);
 assert.match(h.all("git-log-entries").children[0].textContent,/cccc333/);
 assert.equal(h.calls.length,1);
});

test("malformed, oversized or control-character Git logs never create selectable cards",async()=>{
 const bad=[
  "a1b2c3 hello\nnot-a-sha\n",
  "a1b2c3 fine\na1b2c3 duplicate\n",
  "a1b2c3 \u001b[31mred\n",
  "a1b2c3 injected\u202Efilename\n",
  "a1b2c3 trailing \n",
  "a1b2c3 no\n\n",
  "a1b2c3 "+("x".repeat(200))+"\n",
  ("a1b2c3 fine\n").repeat(31),
  "0".repeat(4100)
 ];
 for(const report of bad){
  const h=nativeHarness([report]);
  await h.actions.get("git_log").click();
  assert.equal(h.all("git-log-entries").children.length,0);
  assert.equal(h.all("git-log-count").textContent,"Không hợp lệ");
  assert.equal(h.calls.length,1);
 }
});
test("Git log filter with bidi/control characters creates no fake clickable commit",async()=>{
 const h=nativeHarness(["abcdef1 Normal commit\n"]);
 await h.actions.get("git_log").click();
 h.all("git-log-filter").value="\u202Ehidden";
 h.all("git-log-filter").listeners.get("input")();
 assert.equal(h.all("git-log-entries").children.length,0);
 assert.equal(h.calls.length,1);
});

test("E2EE Git diff is displayed inside Git panel using only text and bound path",async()=>{
 const source="diff --git a/docs/notice.md b/docs/notice.md\n+new line\n";
 const h=nativeHarness([source]);
 h.all("git-path").value="docs/notice.md";
 await h.actions.get("git_diff").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_diff");
 assert.equal(h.calls[0].request.path,"docs/notice.md");
 assert.match(h.all("git-diff-kind").textContent,/Working-tree diff/);
 assert.match(h.all("git-diff-target").textContent,/docs\/notice.md/);
 assert.equal(h.all("git-diff-text").textContent,source);
});
test("switching Git source invalidates stale in-panel preview without a network call",async()=>{
 const h=nativeHarness(["old diff"]);
 h.all("git-path").value="src/original.go";
 await h.actions.get("git_diff").click();
 h.all("git-path").value="src/other.go";
 h.all("git-path").listeners.get("input")();
 assert.equal(h.calls.length,1);
 assert.doesNotMatch(h.all("git-diff-text").textContent,/old diff/);
 assert.match(h.all("git-diff-kind").textContent,/Đường dẫn đã đổi/);
});
test("Git staged and HEAD differences share strict preview rendering",async()=>{
 const h=nativeHarness(["staged diff","head diff"]);
 h.all("git-path").value="src/main.go";
 await h.actions.get("git_staged").click();
 assert.match(h.all("git-diff-kind").textContent,/Staged/);
 assert.equal(h.all("git-diff-text").textContent,"staged diff");
 await h.actions.get("git_head").click();
 assert.match(h.all("git-diff-kind").textContent,/HEAD/);
 assert.equal(h.all("git-diff-text").textContent,"head diff");
 assert.equal(h.calls.length,2);
});
test("oversized and control-laden Git content never becomes HTML or partial diff",async()=>{
 const h=nativeHarness(["A".repeat(4097),"diff \u001b[31mRED"]);
 h.all("git-path").value="README.md";
 await h.actions.get("git_diff").click();
 assert.equal(h.all("git-diff-kind").textContent,"Không hợp lệ");
 assert.doesNotMatch(h.all("git-diff-text").textContent,/AAAAAA/);
 await h.actions.get("git_diff").click();
 assert.match(h.all("git-diff-text").textContent,/\\u001b/);
 assert.doesNotMatch(h.all("git-diff-text").textContent,/\u001b/);
 assert.equal(h.calls.length,2);
});

test("Git diff replies cannot repopulate stale preview after path changes mid-E2EE request",async()=>{
 const h=nativeHarness(["diff --git a/src/old.go b/src/old.go\n+old content"]);
 h.all("git-path").value="src/old.go";
 const pending=h.actions.get("git_diff").click();
 // The text field remains editable while fetch is pending; a late result
 // must not impersonate the newly selected path.
 h.all("git-path").value="src/new.go";
 await pending;
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.path,"src/old.go");
 assert.equal(h.all("git-diff-kind").textContent,"Không hợp lệ");
 assert.doesNotMatch(h.all("git-diff-text").textContent,/old content/);
});

test("successful authenticated GUI close revokes UI access and clears all local plaintext",async()=>{
 const h=nativeHarness([{ok:true}],{confirm:true});
 for(const id of ["output","file-page-transcript","task-transcript","git-diff-text",
   "git-log-selection","console-transcript","file-edit-text",
   "git-commit-message","line-edit-text","task-handle"]){
   h.all(id).textContent="SESSION-SECRET";
   h.all(id).value="SESSION-SECRET";
 }
 await h.all("close").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].path,"/api/close");
 assert.deepEqual(Object.keys(h.calls[0].request),[]);
 for(const id of ["output","file-page-transcript","task-transcript",
  "git-diff-text","git-log-selection","console-transcript"]){
  assert.doesNotMatch(h.all(id).textContent,/SESSION-SECRET/,id);
 }
 for(const id of ["file-edit-text","git-commit-message","line-edit-text","task-handle"]){
  assert.equal(h.all(id).value,"",id);
 }
 assert.equal(h.all("console-run").disabled,true);
 assert.equal(h.all("explorer-location").textContent,"Đã xóa dữ liệu của phiên khỏi giao diện cục bộ.");
 assert.equal(h.all("git-diff-target").textContent,"Đã xóa dữ liệu của phiên khỏi giao diện cục bộ.");
 assert.equal(h.all("close").disabled,true);
 assert.equal(h.all("status").textContent,"Đã đóng GUI native");
});
test("failed native close acknowledgement never reports an Agent session as closed",async()=>{
 const h=nativeHarness([{ok:false,error:"Agent connection unavailable"}],{confirm:true});
 h.all("task-transcript").textContent="PRIVATE";
 h.all("console-command").value="git status";
 await h.all("close").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].path,"/api/close");
 assert.equal(h.all("status").textContent,"Chưa xác nhận đóng GUI");
 assert.doesNotMatch(h.all("task-transcript").textContent,/PRIVATE/);
 assert.equal(h.all("console-command").value,"");
 assert.equal(h.all("console-run").disabled,true);
 await h.actions.get("git_status").click();
 assert.equal(h.calls.length,1);
});
test("late in-flight authenticated output cannot repopulate GUI after local close",async()=>{
 let resolveLate;
 const first=()=>new Promise(resolve=>{resolveLate=resolve});
 const h=nativeHarness([first,{ok:true}],{confirm:true});
 const pending=h.actions.get("git_status").click();
 assert.equal(typeof resolveLate,"function");
 await h.all("close").click();
 resolveLate("SECRET-WITHIN-PENDING-RESPONSE");
 await pending;
 assert.equal(h.calls.length,2);
 assert.equal(h.all("status").textContent,"Đã đóng GUI native");
 assert.doesNotMatch(h.all("output").textContent,/SECRET-WITHIN-PENDING-RESPONSE/);
 assert.equal(h.all("console-run").disabled,true);
});

test("late E2EE network failure cannot overwrite acknowledged local close state",async()=>{
 let rejectLate;
 const deferred=()=>new Promise((_,reject)=>{rejectLate=reject});
 const h=nativeHarness([deferred,{ok:true}],{confirm:true});
 const pending=h.actions.get("git_status").click();
 assert.equal(typeof rejectLate,"function");
 await h.all("close").click();
 rejectLate(new Error("Relay network disconnected after close"));
 await pending;
 assert.equal(h.all("status").textContent,"Đã đóng GUI native");
 assert.equal(h.all("console-run").disabled,true);
 assert.equal(h.calls.length,2);
});


test("console cwd is local-only, bounded and prepended to typed Files/Git reads",async()=>{
 const h=nativeHarness(["D\tlocal\n","diff","file"]);
 async function command(value){
  h.all("console-command").value=value;
  await h.all("console-run").click();
 }
 await command("pwd");
 assert.equal(h.calls.length,0);
 assert.match(h.all("console-transcript").textContent,/\$ pwd\n\//);
 await command("cd src");
 assert.equal(h.calls.length,0);
 assert.match(h.all("console-location").textContent,/Workspace: \/src/);
 await command("cd lib");
 assert.equal(h.calls.length,0);
 await command("pwd");
 assert.match(h.all("console-transcript").textContent,/\/src\/lib/);
 await command("ls");
 assert.equal(h.calls.at(-1).request.operation,"files_list");
 assert.equal(h.calls.at(-1).request.path,"src/lib");
 await command("git review nested.go");
 assert.equal(h.calls.at(-1).request.operation,"git_review");
 assert.equal(h.calls.at(-1).request.path,"src/lib/nested.go");
 await command("files view README.md");
 assert.equal(h.calls.at(-1).request.path,"src/lib/README.md");
 await command("cd ..");
 await command("cd ..");
 await command("cd ..");
 assert.match(h.all("console-location").textContent,/Workspace: \//);
 assert.equal(h.calls.length,3,"pwd and cd must remain local only");
});

test("console never resolves paths outside project root, including after cd",async()=>{
 const h=nativeHarness([]);
 for(const cmd of ["cd ..","cd src","cd .."]){
  h.all("console-command").value=cmd;
  await h.all("console-run").click();
 }
 assert.match(h.all("console-location").textContent,/Workspace: \//);
 for(const cmd of ["cd ../../escape","cd /etc","cd .git","cd -rf",
  "cd src/../escape","cd src ; id","git review ../secret",
  "git review -p","git review .git/config"]){
  h.all("console-command").value=cmd;
  await h.all("console-run").click();
  assert.equal(h.calls.length,0,cmd);
  assert.match(h.all("status").textContent,/Lệnh bị từ chối/);
 }
 h.all("console-command").value="cd "+("a".repeat(119));
 await h.all("console-run").click();
 h.all("console-command").value="cd nested";
 await h.all("console-run").click();
 assert.match(h.all("status").textContent,/Đường dẫn bị từ chối/);
 assert.equal(h.calls.length,0);
});

test("two-round Git review refuses partial E2EE quota and never issues an extra host action",async()=>{
 const h=nativeHarness(["a","b","c","d","e","final single-round diff"]);
 for(let i=0;i<5;i++){
  h.all("console-command").value="git status";
  await h.all("console-run").click();
 }
 h.all("console-command").value="git review src/main.go";
 await h.all("console-run").click();
 assert.equal(h.calls.length,5);
 assert.match(h.all("status").textContent,/Hết lượt E2EE/);
 h.all("console-command").value="git diff src/main.go";
 await h.all("console-run").click();
 assert.equal(h.calls.length,6,"only one E2EE round should remain");
});

test("two-round Git review consumes host-reported two actions",async()=>{
 const h=nativeHarness(["-- WORKTREE / UNSTAGED --\nmodified"]);
 h.all("console-command").value="git review src/main.go";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_review");
 assert.equal(h.calls[0].request.path,"src/main.go");
 assert.match(h.all("quota").textContent,/2 \/ 6/);
 assert.match(h.all("console-transcript").textContent,/WORKTREE \/ UNSTAGED/);
});

test("local pwd/clear/help still work after E2EE quota exhaustion",async()=>{
 const h=nativeHarness(["1","2","3","4","5","6"]);
 for(let i=0;i<6;i++){
  h.all("console-command").value="git status";
  await h.all("console-run").click();
 }
 for(const cmd of ["pwd","help","clear","cd src","pwd"]){
  h.all("console-command").value=cmd;
  await h.all("console-run").click();
 }
 assert.equal(h.calls.length,6,"local navigation/history must not consume quota");
 assert.match(h.all("console-location").textContent,/Workspace: \/src/);
 h.all("console-command").value="ls";
 await h.all("console-run").click();
 assert.equal(h.calls.length,6,"network is prohibited after quota exhaustion");
});


test("oversized two-way Git review never silently shows truncated console diff",async()=>{
 const source="DIFF-BEGIN\n"+"a".repeat(5000)+"\nDIFF-END";
 const h=nativeHarness([source]);
 h.all("console-command").value="git review src/file.go";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.all("output").textContent,source);
 assert.match(h.all("console-transcript").textContent,/không hiển thị bản cắt cụt/);
 assert.doesNotMatch(h.all("console-transcript").textContent,/DIFF-BEGIN/);
 assert.doesNotMatch(h.all("console-transcript").textContent,/DIFF-END/);
});


test("Git reviewed stage refuses stale visible path even without DOM input event",async()=>{
 const confirm="STAGE src/first.go";
 const h=nativeHarness([{text:"review first",confirm}]);
 h.all("git-path").value="src/first.go";
 h.actions.get("git_write_review").dataset.mode="stage";
 await h.actions.get("git_write_review").click();
 h.all("git-path").value="src/second.go";
 h.all("git-confirm-entry").value=confirm;
 await h.all("execute-git").click();
 assert.equal(h.calls.length,1,"stale Git review must never dispatch execute");
 assert.match(h.all("status").textContent,/Review hết hiệu lực/);
});

test("Git branch create review is dropped when branch slug changes",async()=>{
 const confirm="CREATE BRANCH taskdeck/first";
 const h=nativeHarness([{text:"branch review",confirm}]);
 h.all("git-branch").value="first";
 h.actions.get("git_write_review").dataset.mode="branch-create";
 await h.actions.get("git_write_review").click();
 h.all("git-branch").value="second";
 h.all("git-branch").listeners.get("input")();
 h.all("git-confirm-entry").value=confirm;
 await h.all("execute-git").click();
 assert.equal(h.calls.length,1);
 assert.match(h.all("status").textContent,/Review mới/);
});

test("File, line and task writes require exactly their reviewed visible selectors",async()=>{
 const file=nativeHarness([{text:"old",confirm:"EDIT one.txt abc"}]);
 file.all("file").value="one.txt";
 await file.actions.get("file_edit_review").click();
 file.all("file-edit-text").value="new";
 file.all("file-edit-confirm").value="EDIT one.txt abc";
 file.all("file").value="two.txt";
 await file.all("execute-file-edit").click();
 assert.equal(file.calls.length,1);
 assert.match(file.all("status").textContent,/Review hết hiệu lực/);

 const line=nativeHarness([{text:"review",line:"old",confirm:"EDIT LINE 4 abc"}]);
 line.all("file").value="src/large.txt";
 line.all("line").value="4";
 line.actions.get("line_write_review").dataset.mode="edit";
 await line.actions.get("line_write_review").click();
 line.all("line-edit-text").value="changed";
 line.all("line-edit-confirm").value="EDIT LINE 4 abc";
 line.all("line").value="5";
 await line.all("execute-line-edit").click();
 assert.equal(line.calls.length,1);
 assert.match(line.all("status").textContent,/Review hết hiệu lực/);

 const task=nativeHarness([{text:"host approved",confirm:"RUN TASK 7"}]);
 task.all("task-id").value="7";
 task.actions.get("task_review").dataset.mode="run";
 await task.actions.get("task_review").click();
 task.all("approval-entry").value="RUN TASK 7";
 task.all("task-id").value="8";
 await task.all("execute-task").click();
 assert.equal(task.calls.length,1);
 assert.match(task.all("status").textContent,/Review hết hiệu lực/);
});

test("Late authenticated write review is refused if target changed during request",async()=>{
 const h=nativeHarness([{text:"review",confirm:"STAGE src/original.go"}]);
 h.all("git-path").value="src/original.go";
 h.actions.get("git_write_review").dataset.mode="stage";
 const pending=h.actions.get("git_write_review").click();
 h.all("git-path").value="src/another.go";
 await pending;
 h.all("git-confirm-entry").value="STAGE src/original.go";
 await h.all("execute-git").click();
 assert.equal(h.calls.length,1);
 assert.match(h.all("status").textContent,/Review hết hiệu lực/);
});


test("Git path input invalidates diff preview and pending write review together",async()=>{
 const h=nativeHarness(["authenticated diff",{text:"reviewed",confirm:"STAGE docs/first.md"}]);
 h.all("git-path").value="docs/first.md";
 await h.actions.get("git_diff").click();
 h.actions.get("git_write_review").dataset.mode="stage";
 await h.actions.get("git_write_review").click();
 h.all("git-path").value="docs/second.md";
 h.all("git-path").listeners.get("input")();
 assert.match(h.all("git-diff-kind").textContent,/Đường dẫn đã đổi/);
 assert.match(h.all("status").textContent,/Review mới/);
 h.all("git-confirm-entry").value="STAGE docs/first.md";
 await h.all("execute-git").click();
 assert.equal(h.calls.length,2,"changed path must not emit a third E2EE write");
});

test("Git status type selector filters authenticated staged, unstaged and untracked files locally",async()=>{
 const data="## main\nM  src/staged.go\n M src/dirty.go\nMM src/both.go\n?? docs/new.md\n";
 const h=nativeHarness([data]);
 await h.actions.get("git_status").click();
 assert.equal(h.all("git-status-files").children.length,4);
 assert.match(h.all("git-status-count").textContent,/2 staged · 2 unstaged · 1 untracked/);
 h.all("git-status-scope").value="staged";
 h.all("git-status-scope").listeners.get("input")();
 assert.equal(h.all("git-status-files").children.length,2);
 assert.match(h.all("git-status-files").children[0].textContent,/staged.go/);
 h.all("git-status-scope").value="unstaged";
 h.all("git-status-scope").listeners.get("input")();
 assert.equal(h.all("git-status-files").children.length,2);
 assert.match(h.all("git-status-files").children[0].textContent,/dirty.go/);
 h.all("git-status-scope").value="untracked";
 h.all("git-status-scope").listeners.get("input")();
 assert.equal(h.all("git-status-files").children.length,1);
 assert.match(h.all("git-status-files").children[0].textContent,/new.md/);
 assert.equal(h.calls.length,1,"local status filtering must never use an E2EE round");
});

test("Git filename filter never sends traffic and only selects a reviewed path",async()=>{
 const h=nativeHarness([" M src/apple.go\n M docs/banana.md\n"]);
 await h.actions.get("git_status").click();
 h.all("git-status-filter").value="BANANA";
 h.all("git-status-filter").listeners.get("input")();
 const rows=h.all("git-status-files").children;
 assert.equal(rows.length,1);
 await rows[0].click();
 assert.equal(h.all("git-path").value,"docs/banana.md");
 assert.equal(h.all("file").value,"docs/banana.md");
 assert.equal(h.calls.length,1);
 h.all("git-status-filter").value="no-match";
 h.all("git-status-filter").listeners.get("input")();
 assert.equal(h.all("git-status-files").children.length,0);
 assert.match(h.all("git-status-files").textContent,/Không có file phù hợp/);
 assert.equal(h.calls.length,1);
});

test("Git status filters fail closed on unsafe values and exclude injected paths",async()=>{
 const data=" M src/ok.go\n M src/evil\u202Egnp\n M ../secrets\n!! ignored.txt\n M .git/config\n";
 const h=nativeHarness([data]);
 await h.actions.get("git_status").click();
 assert.equal(h.all("git-status-files").children.length,1);
 h.all("git-status-filter").value="\u202Evil";
 h.all("git-status-filter").listeners.get("input")();
 assert.equal(h.all("git-status-files").children.length,0);
 assert.equal(h.all("git-status-count").textContent,"Lọc không hợp lệ");
 h.all("git-status-filter").value="";
 h.all("git-status-scope").value="write";
 h.all("git-status-scope").listeners.get("input")();
 assert.equal(h.all("git-status-files").children.length,0);
 assert.equal(h.calls.length,1);
});

test("Git status refresh clears former cards before a rejected new response",async()=>{
 const h=nativeHarness([" M src/old.go\n", "x".repeat(4097)]);
 await h.actions.get("git_status").click();
 assert.equal(h.all("git-status-files").children.length,1);
 await h.actions.get("git_status").click();
 assert.equal(h.all("git-status-files").children.length,0);
 assert.equal(h.all("git-status-count").textContent,"Không hợp lệ");
 assert.doesNotMatch(h.all("git-status-files").textContent,/old.go/);
 assert.equal(h.calls.length,2);
});


test("Console git status renders filtered Git GUI cards from same authenticated E2EE read",async()=>{
 const h=nativeHarness(["## main\n M src/one.go\nM  docs/two.md\n?? src/three.go\n"]);
 h.all("git-status-scope").value="staged";
 h.all("console-command").value="git status";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1,"GUI must not send another E2EE request");
 assert.equal(h.calls[0].request.operation,"git_status");
 assert.equal(h.all("git-status-files").children.length,1);
 assert.match(h.all("git-status-files").children[0].textContent,/docs\/two.md/);
 assert.match(h.all("git-status-count").textContent,/1\/3 file/);
 await h.all("git-status-files").children[0].click();
 assert.equal(h.all("git-path").value,"docs/two.md");
 assert.equal(h.calls.length,1,"card selection must remain local");
});

test("Console Git log and branches update read-only GUI without extra authenticated actions",async()=>{
 const h=nativeHarness([
  "aabbcc Authenticated Git log entry\n",
  "* taskdeck/dev 0123456789abcdef0123456789abcdef01234567\n"
 ]);
 h.all("console-command").value="git log";
 await h.all("console-run").click();
 assert.equal(h.all("git-log-entries").children.length,1);
 assert.match(h.all("git-log-count").textContent,/1 commit/);
 h.all("console-command").value="git branches";
 await h.all("console-run").click();
 assert.equal(h.all("git-branch-entries").children.length,1);
 assert.match(h.all("git-branch-count").textContent,/1 nhánh/);
 assert.equal(h.calls.length,2);
});

test("rejected or malformed Console Git refresh never retains old authenticated cards",async()=>{
 const h=nativeHarness([
  " M src/old.go\n",
  {ok:false,error:"Permission revoked"},
  " M src/new.go\n",
  "x".repeat(4097)
 ]);
 h.all("console-command").value="git status";
 await h.all("console-run").click();
 assert.equal(h.all("git-status-files").children.length,1);
 await h.all("console-run").click();
 assert.equal(h.all("git-status-files").children.length,0);
 assert.equal(h.all("git-status-count").textContent,"Không tải được");
 await h.all("console-run").click();
 assert.equal(h.all("git-status-files").children.length,1);
 await h.all("console-run").click();
 assert.equal(h.all("git-status-files").children.length,0);
 assert.equal(h.all("git-status-count").textContent,"Không hợp lệ");
 assert.equal(h.calls.length,4);
});



test("Console ls <dir> reuses one authenticated FilesList result for clickable Files explorer",async()=>{
 const h=nativeHarness(["D\tlib\nF\tREADME.md"]);
 h.all("console-command").value="ls src";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"files_list");
 assert.equal(h.calls[0].request.path,"src");
 assert.equal(h.all("directory").value,"src");
 assert.equal(h.all("workspace-entries").children.length,2);
 await h.all("workspace-entries").children[1].click();
 assert.equal(h.all("file").value,"src/README.md");
 assert.equal(h.all("git-path").value,"src/README.md");
 assert.equal(h.calls.length,1,"selecting a file must not send another request");
});

test("Console cat and grep -F are typed, workspace-scoped reads, never arbitrary shell",async()=>{
 const h=nativeHarness(["safe file view","safe literal find"]);
 h.all("console-command").value="cd src";
 await h.all("console-run").click();
 for(const [raw,operation,expected] of [
  ["cat README.md","file_view",{path:"src/README.md"}],
  ["grep -F README.md exact words","file_find_lines",{path:"src/README.md",literal:"exact words"}]
 ]){
  h.all("console-command").value=raw;
  await h.all("console-run").click();
  const request=h.calls.at(-1).request;
  assert.equal(request.operation,operation);
  for(const [key,value] of Object.entries(expected))assert.equal(request[key],value);
 }
 assert.equal(h.calls.length,2);
 for(const raw of ["cat ../secrets","cat /etc/shadow","cat -e",
  "grep README.md hi","grep -E README.md pattern",
  "grep -F ../secret hi","grep -F README.md a|sh",
  "ls ../../tmp","ls /etc","ls .git"]){
  h.all("console-command").value=raw;
  await h.all("console-run").click();
  assert.equal(h.calls.length,2,raw);
  assert.match(h.all("status").textContent,/Lệnh bị từ chối/);
 }
});

test("Console tasks list makes native approved-task catalogue clickable with one read",async()=>{
 const h=nativeHarness(["7\tAllowed task\n8\tAnother approved task"]);
 h.all("console-command").value="tasks list";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"tasks");
 assert.equal(h.all("task-catalog").children.length,2);
 await h.all("task-catalog").children[0].click();
 assert.equal(h.all("task-id").value,"7");
 assert.equal(h.calls.length,1,"task selection must not auto-run");
});

test("Console Git diff/staged/head previews are native text only, no extra E2EE",async()=>{
 const h=nativeHarness(["unstaged diff","staged diff","head diff"]);
 for(const [command,label] of [
  ["git diff src/main.go","Working-tree diff"],
  ["git staged src/main.go","Staged diff"],
  ["git head src/main.go","HEAD diff"]
 ]){
  h.all("console-command").value=command;
  await h.all("console-run").click();
  assert.equal(h.all("git-path").value,"src/main.go");
  assert.equal(h.all("git-diff-kind").textContent,label);
  assert.match(h.all("git-diff-text").textContent,/diff/);
 }
 assert.equal(h.calls.length,3,"each Console read must cost one E2EE action");
});

test("Console Files and Tasks refreshes fail closed after an Agent rejection",async()=>{
 const h=nativeHarness([
  "F\told.txt",{ok:false,error:"Files RBAC revoked"},
  "1\tOld task",{ok:false,error:"Task RBAC revoked"}
 ]);
 h.all("console-command").value="ls";
 await h.all("console-run").click();
 assert.equal(h.all("workspace-entries").children.length,1);
 await h.all("console-run").click();
 assert.equal(h.all("workspace-entries").children.length,0);
 assert.equal(h.all("explorer-location").textContent,"Không tải được");
 h.all("console-command").value="tasks list";
 await h.all("console-run").click();
 assert.equal(h.all("task-catalog").children.length,1);
 await h.all("console-run").click();
 assert.equal(h.all("task-catalog").children.length,0);
 assert.equal(h.all("task-catalog-count").textContent,"Không tải được");
 assert.equal(h.calls.length,4);
});

test("Console FilesList response cannot bind clickable cards after Files directory changes",async()=>{
 let complete;
 const later=()=>new Promise(resolve=>{complete=resolve});
 const h=nativeHarness([later]);
 h.all("console-command").value="ls src";
 const inFlight=h.all("console-run").click();
 assert.equal(typeof complete,"function");
 h.all("directory").value="other";
 complete("F\ttrusted.txt");
 await inFlight;
 assert.equal(h.calls.length,1);
 assert.equal(h.all("directory").value,"other");
 assert.equal(h.all("workspace-entries").children.length,0);
 assert.equal(h.all("explorer-location").textContent,"Danh sách đã hết hiệu lực");
});

test("Console Git diff response cannot overwrite selection changed while Agent read was pending",async()=>{
 let complete;
 const later=()=>new Promise(resolve=>{complete=resolve});
 const h=nativeHarness([later]);
 h.all("console-command").value="git diff src/first.go";
 const inFlight=h.all("console-run").click();
 h.all("git-path").value="src/other.go";
 complete("old and stale diff");
 await inFlight;
 assert.equal(h.all("git-path").value,"src/other.go");
 assert.equal(h.all("git-diff-kind").textContent,"Đường dẫn đã đổi");
 assert.doesNotMatch(h.all("git-diff-text").textContent,/stale diff/);
 assert.equal(h.calls.length,1);
});



test("Console Git stage review never executes without a separate confirmation click",async()=>{
 const confirm="STAGE src/main.go";
 const h=nativeHarness([{text:"trusted preview",confirm},"host staged"]);
 h.all("console-command").value="git review-stage src/main.go";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_write_review");
 assert.equal(h.calls[0].request.mode,"stage");
 assert.equal(h.calls[0].request.path,"src/main.go");
 assert.equal(h.all("git-path").value,"src/main.go");
 assert.match(h.all("status").textContent,/Git review sẵn sàng/);
 assert.match(h.all("git-confirm-text").textContent,/STAGE src\/main.go/);
 // No hidden automatic write; only the exact typed confirmation may proceed.
 h.all("git-confirm-entry").value=confirm;
 await h.all("execute-git").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].request.operation,"git_write_execute");
 assert.equal(h.calls[1].request.path,"src/main.go");
 assert.equal(h.calls[1].request.confirmation,confirm);
 await h.all("execute-git").click();
 assert.equal(h.calls.length,2,"approval token is one-use on GUI too");
});

test("Console Git write review target and mode are pinned before human execute",async()=>{
 const confirm="UNSTAGE docs/a.md";
 const h=nativeHarness([{text:"staged review",confirm}]);
 h.all("console-command").value="git review-unstage docs/a.md";
 await h.all("console-run").click();
 assert.equal(h.calls[0].request.mode,"unstage");
 h.all("git-path").value="docs/other.md";
 h.all("git-confirm-entry").value=confirm;
 await h.all("execute-git").click();
 assert.equal(h.calls.length,1,"write must be refused if visible Git target changed");
 assert.match(h.all("status").textContent,/Review hết hiệu lực/);
});

test("Console reviewed Git branch remains restricted to taskdeck slug and separately confirmed",async()=>{
 const h=nativeHarness([{text:"branch from HEAD",confirm:"CREATE BRANCH taskdeck/feature-a abcdef012345"},
                        {text:"commit staged",confirm:"COMMIT abcdef012345"}]);
 for(const [command,mode,path] of [
  ["git review-create-branch feature-a","branch-create","feature-a"],
  ["git review-commit","commit",""]
 ]){
  h.all("console-command").value=command;
  await h.all("console-run").click();
  assert.equal(h.calls.at(-1).request.operation,"git_write_review");
  assert.equal(h.calls.at(-1).request.mode,mode);
  assert.equal(h.calls.at(-1).request.path,path);
 }
 assert.equal(h.calls.length,2,"no implicit branch create or commit");
 assert.equal(h.all("git-branch").value,"feature-a");
 assert.match(h.all("git-confirm-text").textContent,/COMMIT abcdef012345/);
});

test("Console Git review rejects dangerous refs and shell syntax without network calls",async()=>{
 const h=nativeHarness([]);
 for(const raw of [
  "git stage src/main.go","git commit -m hello","git push origin main",
  "git review-stage ../secret","git review-stage /etc/passwd",
  "git review-stage .git/config","git review-unstage -x",
  "git review-unstage src/a|id","git review-stage src/a;id",
  "git review-stage src/a b","git review-create-branch x",
  "git review-switch-branch ../main","git review-delete-branch -test",
  "git review-create-branch x","git review-create-branch a-",
  "git review-create-branch a_b","git review-commit --amend",
  "git review-stage src/a\nrm -rf /"
 ]){
  h.all("console-command").value=raw;
  await h.all("console-run").click();
  assert.equal(h.calls.length,0,raw);
  assert.match(h.all("status").textContent,/Lệnh bị từ chối/,raw);
 }
});

test("Console reviewed Git action reserves two remaining E2EE rounds",async()=>{
 const h=nativeHarness(["a","b","c","d","e","single read"]);
 for(let i=0;i<5;i++){
  h.all("console-command").value="git status";
  await h.all("console-run").click();
 }
 h.all("console-command").value="git review-stage src/a.go";
 await h.all("console-run").click();
 assert.equal(h.calls.length,5);
 assert.match(h.all("status").textContent,/Hết lượt E2EE/);
 h.all("console-command").value="git status";
 await h.all("console-run").click();
 assert.equal(h.calls.length,6);
});

test("Late Console Git write review cannot be accepted if target changed mid-flight",async()=>{
 let done;
 const promise=()=>new Promise(resolve=>{done=resolve});
 const h=nativeHarness([promise]);
 h.all("console-command").value="git review-stage src/a.go";
 const pending=h.all("console-run").click();
 assert.equal(typeof done,"function");
 h.all("git-path").value="src/b.go";
 done({text:"old review",confirm:"STAGE src/a.go"});
 await pending;
 assert.equal(h.calls.length,1);
 assert.match(h.all("status").textContent,/Review hết hiệu lực/);
 h.all("git-confirm-entry").value="STAGE src/a.go";
 await h.all("execute-git").click();
 assert.equal(h.calls.length,1);
});

test("Console reviewed Git write is refused when confirmation mismatch",async()=>{
 const h=nativeHarness([{text:"trusted",confirm:"STAGE src/a.go"}]);
 h.all("console-command").value="git review-stage src/a.go";
 await h.all("console-run").click();
 h.all("git-confirm-entry").value="STAGE another.go";
 await h.all("execute-git").click();
 assert.equal(h.calls.length,1);
 assert.match(h.all("status").textContent,/Đã hủy/);
});



test("Console task review-run only prepares host-approved review; execution needs separate click",async()=>{
 const h=nativeHarness([{text:"Approved task #7",confirm:"RUN TASK 7"},"done"]);
 h.all("console-command").value="tasks review-run 7";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"task_review");
 assert.equal(h.calls[0].request.mode,"run");
 assert.equal(h.calls[0].request.task_id,7);
 assert.equal(h.all("task-id").value,"7");
 assert.match(h.all("status").textContent,/Task review sẵn sàng/);
 assert.match(h.all("approval-text").textContent,/RUN TASK 7/);
 h.all("approval-entry").value="RUN TASK 7";
 await h.all("execute-task").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].request.operation,"task_execute");
 assert.equal(h.calls[1].request.mode,"run");
 assert.equal(h.calls[1].request.task_id,7);
 await h.all("execute-task").click();
 assert.equal(h.calls.length,2,"one-use UI review must not execute twice");
});

test("Console task review-start requires exact confirmation and never auto-starts",async()=>{
 const h=nativeHarness([{text:"Host-approved task #12",confirm:"START TASK 12"}]);
 h.all("console-command").value="tasks review-start 12";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"task_review");
 assert.equal(h.calls[0].request.mode,"start");
 h.all("approval-entry").value="RUN TASK 12";
 await h.all("execute-task").click();
 assert.equal(h.calls.length,1);
 assert.match(h.all("status").textContent,/Đã hủy/);
});

test("Console task review invalidates a changed Task ID before execute",async()=>{
 const h=nativeHarness([{text:"approved",confirm:"RUN TASK 9"}]);
 h.all("console-command").value="tasks review-run 9";
 await h.all("console-run").click();
 h.all("task-id").value="10";
 h.all("approval-entry").value="RUN TASK 9";
 await h.all("execute-task").click();
 assert.equal(h.calls.length,1);
 assert.match(h.all("status").textContent,/Review hết hiệu lực/);
});

test("Late Console task review is dropped after Task selector change",async()=>{
 let deliver;
 const later=()=>new Promise(resolve=>{deliver=resolve});
 const h=nativeHarness([later]);
 h.all("console-command").value="tasks review-run 21";
 const inFlight=h.all("console-run").click();
 assert.equal(typeof deliver,"function");
 h.all("task-id").value="22";
 deliver({text:"approved old task",confirm:"RUN TASK 21"});
 await inFlight;
 assert.equal(h.calls.length,1);
 assert.match(h.all("status").textContent,/Review hết hiệu lực/);
 h.all("approval-entry").value="RUN TASK 21";
 await h.all("execute-task").click();
 assert.equal(h.calls.length,1);
});

test("Console task review validates finite positive canonical numeric IDs before network",async()=>{
 const h=nativeHarness([]);
 for(const input of ["tasks run 7","tasks start 7","tasks review-run 0",
  "tasks review-start 0007","tasks review-run -1","tasks review-run 2147483648",
  "tasks review-run 7;id","tasks review-start 7 | sh",
  "tasks review-run 1\nid","tasks review-start 1.0",
  "tasks review-run 999999999999999999999999999",
  "tasks review-execute 2"]){
  h.all("console-command").value=input;
  await h.all("console-run").click();
  assert.equal(h.calls.length,0,input);
  assert.match(h.all("status").textContent,/Lệnh bị từ chối/,input);
 }
});

test("Console task review reserves quota for a separately confirmed execute",async()=>{
 const h=nativeHarness(["a","b","c","d","e","final"]);
 for(let i=0;i<5;i++){
  h.all("console-command").value="git status";
  await h.all("console-run").click();
 }
 h.all("console-command").value="tasks review-run 3";
 await h.all("console-run").click();
 assert.equal(h.calls.length,5,"do not spend the final E2EE round on an unusable review");
 assert.match(h.all("status").textContent,/Hết lượt E2EE/);
 h.all("console-command").value="git status";
 await h.all("console-run").click();
 assert.equal(h.calls.length,6,"ordinary one-round read remains available");
});

test("Console tasks output populates verified Task GUI and pages without extra E2EE",async()=>{
 const handle="a".repeat(32);
 const h=nativeHarness([
  taskWindow(handle,0,5,10,"running",false,"hello"),
  taskWindow(handle,5,10,10,"exited",true,"world")
 ]);
 h.all("console-command").value="tasks output "+handle+" 0";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"task_output");
 assert.equal(h.calls[0].request.handle,handle);
 assert.equal(h.calls[0].request.offset,0);
 assert.equal(h.all("task-handle").value,handle);
 assert.equal(h.all("task-transcript").textContent,"hello");
 assert.equal(h.all("task-offset").value,"5");
 assert.equal(h.all("task-read-next").disabled,false);
 await h.all("task-read-next").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.all("task-transcript").textContent,"helloworld");
 assert.match(h.all("task-output-progress").textContent,/EOF/);
});

test("Console task output rejects forged data or revoked permission without stale GUI output",async()=>{
 const handle="b".repeat(32);
 const h=nativeHarness([
  taskWindow(handle,0,5,7,"running",false,"hello"),
  {ok:false,error:"Task RBAC revoked"},
  "TASK_HANDLE "+handle+"\nOUTPUT_OFFSET 1\nOUTPUT_NEXT 2\nOUTPUT_TOTAL 2\nTASK_STATE exited\nOUTPUT_EOF yes\n-- TASK OUTPUT WINDOW --\nx"
 ]);
 h.all("console-command").value="tasks output "+handle+" 0";
 await h.all("console-run").click();
 assert.equal(h.all("task-transcript").textContent,"hello");
 await h.all("console-run").click();
 assert.doesNotMatch(h.all("task-transcript").textContent,/hello/);
 assert.equal(h.all("task-read-next").disabled,true);
 await h.all("console-run").click();
 assert.match(h.all("status").textContent,/Từ chối trang output/);
 assert.equal(h.all("task-read-next").disabled,true);
 assert.equal(h.calls.length,3);
});

test("Late Console task output cannot overwrite a changed handle or cursor",async()=>{
 let resolve;
 const pending=()=>new Promise(r=>{resolve=r});
 const handle="c".repeat(32);
 const h=nativeHarness([pending]);
 h.all("console-command").value="tasks output "+handle+" 0";
 const request=h.all("console-run").click();
 assert.equal(typeof resolve,"function");
 h.all("task-handle").value="d".repeat(32);
 h.all("task-offset").value="5";
 resolve(taskWindow(handle,0,5,5,"exited",true,"hello"));
 await request;
 assert.equal(h.calls.length,1);
 assert.match(h.all("status").textContent,/Output hết hiệu lực/);
 assert.equal(h.all("task-read-next").disabled,true);
 assert.doesNotMatch(h.all("task-transcript").textContent,/hello/);
});

test("Console tasks status sets only the requested safe handle and never starts a task",async()=>{
 const handle="e".repeat(32);
 const h=nativeHarness(["TASK_STATE running\n"]);
 h.all("console-command").value="tasks status "+handle;
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"task_status");
 assert.equal(h.calls[0].request.handle,handle);
 assert.equal(h.all("task-handle").value,handle);
 assert.match(h.all("output").textContent,/running/);
});

test("Git mutation clears old authenticated cards before uncertain E2EE reply",async()=>{
 let deliver;
 const pending=()=>new Promise(resolve=>{deliver=resolve});
 const sha="a".repeat(40);
 const h=nativeHarness([
  " M src/main.go\n",
  "aabbcc Last change\n",
  "  taskdeck/feature-a "+sha+"\n",
  {text:"reviewed Git file",confirm:"STAGE src/main.go"},
  pending
 ]);
 await h.actions.get("git_status").click();
 await h.actions.get("git_log").click();
 await h.actions.get("git_branches").click();
 assert.equal(h.all("git-status-files").children.length,1);
 assert.equal(h.all("git-log-entries").children.length,1);
 assert.equal(h.all("git-branch-entries").children.length,1);
 h.all("git-path").value="src/main.go";
 h.actions.get("git_write_review").dataset.mode="stage";
 await h.actions.get("git_write_review").click();
 h.all("git-confirm-entry").value="STAGE src/main.go";
 const inflight=h.all("execute-git").click();
 assert.equal(typeof deliver,"function");
 assert.equal(h.calls.length,5);
 assert.equal(h.calls[4].request.operation,"git_write_execute");
 for(const id of ["git-status-files","git-log-entries","git-branch-entries","workspace-entries"])
  assert.equal(h.all(id).children.length,0,id);
 assert.equal(h.all("git-status-count").textContent,"Cần tải lại");
 assert.equal(h.all("git-log-count").textContent,"Cần tải lại");
 assert.equal(h.all("git-branch-count").textContent,"Cần tải lại");
 assert.match(h.all("git-diff-kind").textContent,/Cần tải lại/);
 deliver({ok:false,error:"Uncertain commit outcome"});
 await inflight;
 assert.equal(h.all("git-status-files").children.length,0);
 assert.equal(h.all("git-status-count").textContent,"Cần tải lại");
 assert.equal(h.calls.length,5,"no hidden status refresh or retry");
});

test("A rejected review confirmation does not invalidate fresh Git snapshots",async()=>{
 const h=nativeHarness([
  " M src/file.go\n",
  {text:"reviewed",confirm:"STAGE src/file.go"}
 ]);
 await h.actions.get("git_status").click();
 h.all("git-path").value="src/file.go";
 h.actions.get("git_write_review").dataset.mode="stage";
 await h.actions.get("git_write_review").click();
 h.all("git-confirm-entry").value="WRONG";
 await h.all("execute-git").click();
 assert.equal(h.calls.length,2,"no network write on incorrect confirmation");
 assert.equal(h.all("git-status-files").children.length,1,"read view is still valid");
});

test("File and managed-task writes invalidate Git and workspace previews before RPC",async()=>{
 for(const kind of ["file","task"]){
  let deliver;
  const pending=()=>new Promise(resolve=>{deliver=resolve});
  const responses=kind==="file"?
   [" M src/config.txt\n",{text:"old text",confirm:"EDIT src/config.txt 123456789abc"},pending]:
   [" M src/config.txt\n",{text:"Host-approved task",confirm:"RUN TASK 7"},pending];
  const h=nativeHarness(responses);
  await h.actions.get("git_status").click();
  assert.equal(h.all("git-status-files").children.length,1);
  if(kind==="file"){
   h.all("file").value="src/config.txt";
   await h.actions.get("file_edit_review").click();
   h.all("file-edit-text").value="new text";
   h.all("file-edit-confirm").value="EDIT src/config.txt 123456789abc";
  }else{
   h.all("task-id").value="7";
   h.actions.get("task_review").dataset.mode="run";
   await h.actions.get("task_review").click();
   h.all("approval-entry").value="RUN TASK 7";
  }
  const action=h.all(kind==="file"?"execute-file-edit":"execute-task").click();
  assert.equal(typeof deliver,"function");
  assert.equal(h.calls.length,3);
  assert.equal(h.all("git-status-files").children.length,0);
  assert.equal(h.all("git-status-count").textContent,"Cần tải lại");
  assert.equal(h.all("workspace-entries").children.length,0);
  deliver({ok:false,error:"Lost response"});
  await action;
  assert.equal(h.all("git-status-files").children.length,0);
 }
});

test("Later Git status requires fresh E2EE read, not cached cards after mutation",async()=>{
 const h=nativeHarness([
  " M src/main.go\n",
  {text:"review",confirm:"STAGE src/main.go"},
  "staged",
  "M  src/main.go\n"
 ]);
 await h.actions.get("git_status").click();
 h.all("git-path").value="src/main.go";
 h.actions.get("git_write_review").dataset.mode="stage";
 await h.actions.get("git_write_review").click();
 h.all("git-confirm-entry").value="STAGE src/main.go";
 await h.all("execute-git").click();
 assert.equal(h.all("git-status-files").children.length,0);
 await h.actions.get("git_status").click();
 assert.equal(h.calls.length,4);
 assert.equal(h.all("git-status-files").children.length,1);
 assert.match(h.all("git-status-files").children[0].textContent,/M  · src\/main.go/);
});

test("two-way Git diff selector switches verified staged and unstaged text locally",async()=>{
 const review="-- WORKTREE / UNSTAGED --\nworktree-old\n"+
  "-- INDEX / STAGED --\nindex-new\n";
 const h=nativeHarness([review]);
 h.all("git-path").value="src/main.go";
 await h.actions.get("git_review").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_review");
 assert.equal(h.all("git-review-side").disabled,false);
 assert.equal(h.all("git-review-side").value,"all");
 assert.match(h.all("git-diff-text").textContent,/worktree-old/);
 assert.match(h.all("git-diff-text").textContent,/index-new/);
 const change=h.all("git-review-side").listeners.get("input");
 h.all("git-review-side").value="worktree";change();
 assert.match(h.all("git-diff-text").textContent,/worktree-old/);
 assert.doesNotMatch(h.all("git-diff-text").textContent,/index-new/);
 assert.match(h.all("git-diff-target").textContent,/không phải review đầy đủ/);
 h.all("git-review-side").value="index";change();
 assert.match(h.all("git-diff-text").textContent,/index-new/);
 assert.doesNotMatch(h.all("git-diff-text").textContent,/worktree-old/);
 h.all("git-review-side").value="all";change();
 assert.match(h.all("git-diff-text").textContent,/worktree-old/);
 assert.match(h.all("git-diff-text").textContent,/index-new/);
 assert.equal(h.calls.length,1,"switching local view must not consume E2EE or mutate Git");
 assert.equal(h.all("output").textContent,review,"the main authenticated review remains complete");
});

test("ambiguous Git review boundaries fall back to complete text without hiding data",async()=>{
 const full="-- WORKTREE / UNSTAGED --\nline\n"+
  "-- INDEX / STAGED --\nindex\n-- INDEX / STAGED --\nmalicious";
 const h=nativeHarness([full]);
 h.all("git-path").value="src/main.go";
 await h.actions.get("git_review").click();
 h.all("git-review-side").value="index";
 h.all("git-review-side").listeners.get("input")();
 assert.equal(h.all("git-review-side").value,"all");
 assert.equal(h.all("git-diff-text").textContent,full);
 assert.match(h.all("git-diff-target").textContent,/Không thể tách diff an toàn/);
 assert.equal(h.calls.length,1);
});

test("malformed two-way result cannot be filtered to accidentally hide suspicious lines",async()=>{
 const full="-- WORKTREE / UNSTAGED --\nhead only";
 const h=nativeHarness([full]);
 h.all("git-path").value="a.go";
 await h.actions.get("git_review").click();
 h.all("git-review-side").value="worktree";
 h.all("git-review-side").listeners.get("input")();
 assert.equal(h.all("git-review-side").value,"all");
 assert.equal(h.all("git-diff-text").textContent,full);
 assert.equal(h.calls.length,1);
});

test("single-side Git diffs disable local two-way selector",async()=>{
 const h=nativeHarness(["only worktree changes"]);
 h.all("git-path").value="src/main.go";
 await h.actions.get("git_diff").click();
 assert.equal(h.all("git-review-side").disabled,true);
 assert.equal(h.all("git-review-side").value,"all");
 assert.match(h.all("git-diff-text").textContent,/only worktree changes/);
});

test("Git diff local filter never reuses a snapshot for another filename",async()=>{
 const full="-- WORKTREE / UNSTAGED --\nfirst file\n-- INDEX / STAGED --\nsecond section";
 const h=nativeHarness([full]);
 h.all("git-path").value="src/first.go";
 await h.actions.get("git_review").click();
 h.all("git-path").value="src/second.go";
 h.all("git-review-side").value="worktree";
 h.all("git-review-side").listeners.get("input")();
 assert.equal(h.all("git-review-side").disabled,true);
 assert.match(h.all("git-diff-kind").textContent,/Đường dẫn đã đổi/);
 assert.doesNotMatch(h.all("git-diff-text").textContent,/first file/);
 assert.equal(h.calls.length,1);
});

test("local close erases cached two-way Git preview and disables filter",async()=>{
 const full="-- WORKTREE / UNSTAGED --\nworktree-private\n-- INDEX / STAGED --\nindex-private";
 const h=nativeHarness([full,{ok:true,text:"closed"}],{confirm:true});
 h.all("git-path").value="src/private.go";
 await h.actions.get("git_review").click();
 assert.match(h.all("git-diff-text").textContent,/worktree-private/);
 await h.all("close").click();
 assert.equal(h.all("git-review-side").disabled,true);
 assert.equal(h.all("git-review-side").value,"all");
 assert.doesNotMatch(h.all("git-diff-text").textContent,/worktree-private/);
 assert.equal(h.calls.length,2);
});



test("Console git review reuses two E2EE rounds for full native Git pane and local split",async()=>{
 const both="-- WORKTREE / UNSTAGED --\nold line\n-- INDEX / STAGED --\nnew index line";
 const h=nativeHarness([both]);
 h.all("console-command").value="git review src/code.go";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_review");
 assert.equal(h.calls[0].request.path,"src/code.go");
 assert.match(h.all("quota").textContent,/2 \/ 6/);
 assert.equal(h.all("git-path").value,"src/code.go");
 assert.equal(h.all("git-review-side").disabled,false);
 assert.equal(h.all("git-diff-kind").textContent,"Two-way review");
 assert.equal(h.all("git-diff-text").textContent,both);
 h.all("git-review-side").value="index";
 h.all("git-review-side").listeners.get("input")();
 assert.match(h.all("git-diff-text").textContent,/new index line/);
 assert.doesNotMatch(h.all("git-diff-text").textContent,/old line/);
 assert.equal(h.all("output").textContent,both,"full authenticated reply remains available");
 assert.equal(h.calls.length,1,"local filter cannot spend E2EE quota");
});

test("Complete two-way Git preview accepts two independent 4KiB authenticated chunks",async()=>{
 const worktree="A".repeat(4050),index="B".repeat(4050);
 const both="-- WORKTREE / UNSTAGED --\n"+worktree+"\n-- INDEX / STAGED --\n"+index;
 const h=nativeHarness([both]);
 h.all("git-path").value="src/main.go";
 await h.actions.get("git_review").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.all("git-review-side").disabled,false);
 assert.equal(h.all("git-diff-kind").textContent,"Two-way review");
 assert.equal(h.all("git-diff-text").textContent,both);
 assert.equal(h.all("output").textContent,both);
 h.all("git-review-side").value="worktree";
 h.all("git-review-side").listeners.get("input")();
 assert.equal(h.all("git-diff-text").textContent,worktree);
 assert.equal(h.calls.length,1);
});

test("Oversize combined Git review fails closed without truncated or selectable preview",async()=>{
 const oversized="-- WORKTREE / UNSTAGED --\n"+"X".repeat(2*4096+80)+
  "\n-- INDEX / STAGED --\nend";
 const h=nativeHarness([oversized]);
 h.all("console-command").value="git review src/large.go";
 await h.all("console-run").click();
 assert.equal(h.all("git-review-side").disabled,true);
 assert.equal(h.all("git-diff-kind").textContent,"Không hợp lệ");
 assert.doesNotMatch(h.all("git-diff-text").textContent,/XXX/);
 assert.equal(h.calls.length,1);
 assert.equal(h.all("output").textContent,oversized,"bounded view denial does not alter authenticated result");
});

test("Late Console git review response cannot overwrite another file's GUI preview",async()=>{
 let resolve;
 const later=()=>new Promise(r=>{resolve=r});
 const h=nativeHarness([later]);
 h.all("console-command").value="git review src/first.go";
 const pending=h.all("console-run").click();
 h.all("git-path").value="src/second.go";
 resolve("-- WORKTREE / UNSTAGED --\nprivate\n-- INDEX / STAGED --\nprivate");
 await pending;
 assert.equal(h.calls.length,1);
 assert.equal(h.all("git-path").value,"src/second.go");
 assert.equal(h.all("git-diff-kind").textContent,"Đường dẫn đã đổi");
 assert.equal(h.all("git-review-side").disabled,true);
 assert.doesNotMatch(h.all("git-diff-text").textContent,/private/);
});

test("Console files lines displays verified source page with native next cursor",async()=>{
 const sha="a".repeat(64);
 const h=nativeHarness([
  sourcePage(sha,1,3,4,"1: hello\n2: world\n"),
  sourcePage(sha,3,0,4,"3: next\n4: EOF\n")
 ]);
 h.all("console-command").value="files lines src/notes.txt 1";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"file_lines");
 assert.equal(h.calls[0].request.path,"src/notes.txt");
 assert.equal(h.calls[0].request.offset,1);
 assert.equal(h.all("file").value,"src/notes.txt");
 assert.equal(h.all("line").value,"3");
 assert.equal(h.all("file-page-transcript").textContent,"1: hello\n2: world\n");
 assert.equal(h.all("file-read-next").disabled,false);
 await h.all("file-read-next").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.all("file-page-transcript").textContent,"1: hello\n2: world\n3: next\n4: EOF\n");
 assert.equal(h.all("file-read-next").disabled,true);
});

test("Console grep -F reuses authenticated result for clickable literal file line cards",async()=>{
 const h=nativeHarness([findReport([[2,"has word"],[9,"another word"]])]);
 h.all("console-command").value="grep -F src/main.go word";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"file_find_lines");
 assert.equal(h.calls[0].request.path,"src/main.go");
 assert.equal(h.calls[0].request.literal,"word");
 assert.equal(h.all("file").value,"src/main.go");
 assert.equal(h.all("find-literal").value,"word");
 assert.equal(h.all("find-entries").children.length,2);
 await h.all("find-entries").children[1].click();
 assert.equal(h.all("line").value,"9");
 assert.equal(h.calls.length,1,"a click only selects a verified line");
});

test("Console File Lines late response must not overwrite changed file or line",async()=>{
 let resolve;
 const later=()=>new Promise(r=>{resolve=r});
 const h=nativeHarness([later]);
 h.all("console-command").value="files lines src/first.go 1";
 const pending=h.all("console-run").click();
 h.all("file").value="src/second.go";
 h.all("line").value="2";
 resolve(sourcePage("a".repeat(64),1,0,1,"1: private\n"));
 await pending;
 assert.equal(h.calls.length,1);
 assert.equal(h.all("file").value,"src/second.go");
 assert.equal(h.all("file-read-next").disabled,true);
 assert.doesNotMatch(h.all("file-page-transcript").textContent,/private/);
 assert.match(h.all("status").textContent,/Trang file hết hiệu lực/);
});

test("Console grep late response never creates cards for changed literal",async()=>{
 let resolve;
 const later=()=>new Promise(r=>{resolve=r});
 const h=nativeHarness([later]);
 h.all("console-command").value="files find src/a.go secret";
 const pending=h.all("console-run").click();
 h.all("find-literal").value="different";
 resolve(findReport([[3,"secret text"]]));
 await pending;
 assert.equal(h.calls.length,1);
 assert.equal(h.all("find-entries").children.length,0);
 assert.equal(h.all("find-summary").textContent,"Kết quả đã hết hiệu lực");
});

test("Console Files line and find failed E2EE replies clear old GUI data",async()=>{
 const sha="a".repeat(64);
 const h=nativeHarness([
  sourcePage(sha,1,0,1,"1: old\n"),
  {ok:false,error:"Files read revoked"},
  findReport([[2,"word"]]),
  {ok:false,error:"Search read revoked"}
 ]);
 h.all("console-command").value="files lines src/a.go 1";
 await h.all("console-run").click();
 assert.match(h.all("file-page-transcript").textContent,/old/);
 await h.all("console-run").click();
 assert.equal(h.all("file-read-next").disabled,true);
 assert.doesNotMatch(h.all("file-page-transcript").textContent,/old/);
 h.all("console-command").value="files find src/a.go word";
 await h.all("console-run").click();
 assert.equal(h.all("find-entries").children.length,1);
 await h.all("console-run").click();
 assert.equal(h.all("find-entries").children.length,0);
 assert.equal(h.all("find-summary").textContent,"Không tải được");
 assert.equal(h.calls.length,4);
});



test("Console files next uses only last verified SHA-pinned source cursor",async()=>{
 const sha="a".repeat(64);
 const h=nativeHarness([
  sourcePage(sha,1,3,4,"1: first\n2: second\n"),
  sourcePage(sha,3,0,4,"3: third\n4: fourth\n")
 ]);
 h.all("console-command").value="files lines src/readme.md 1";
 await h.all("console-run").click();
 assert.equal(h.all("line").value,"3");
 h.all("console-command").value="files next";
 await h.all("console-run").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].request.operation,"file_lines");
 assert.equal(h.calls[1].request.path,"src/readme.md");
 assert.equal(h.calls[1].request.offset,3);
 assert.equal(h.all("file-page-transcript").textContent,
  "1: first\n2: second\n3: third\n4: fourth\n");
 await h.all("console-run").click();
 assert.equal(h.calls.length,2,"EOF must forbid a third network request");
});

test("Console tasks next reuses authenticated 2KiB task cursor, never starts process",async()=>{
 const handle="a".repeat(32);
 const h=nativeHarness([
  taskWindow(handle,0,5,10,"running",false,"hello"),
  taskWindow(handle,5,10,10,"exited",true,"world")
 ]);
 h.all("console-command").value="tasks output "+handle+" 0";
 await h.all("console-run").click();
 assert.equal(h.all("task-offset").value,"5");
 h.all("console-command").value="tasks next";
 await h.all("console-run").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].request.operation,"task_output");
 assert.equal(h.calls[1].request.handle,handle);
 assert.equal(h.calls[1].request.offset,5);
 assert.equal(h.all("task-transcript").textContent,"helloworld");
 assert.match(h.all("task-output-progress").textContent,/EOF/);
 await h.all("console-run").click();
 assert.equal(h.calls.length,2,"EOF forbids another E2EE read");
});

test("Console page-next aliases refuse changed file, line, handle, or task offset",async()=>{
 const sha="a".repeat(64),handle="b".repeat(32);
 const h=nativeHarness([
  sourcePage(sha,1,2,3,"1: safe\n"),
  taskWindow(handle,0,2,4,"running",false,"hi")
 ]);
 h.all("console-command").value="files lines src/a.go 1";
 await h.all("console-run").click();
 h.all("console-command").value="files next";
 h.all("line").value="3";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1,"edited source cursor must not be usable");
 h.all("line").value="2";
 h.all("file").value="other.go";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1,"changed file must not reuse a cursor");
 h.all("console-command").value="tasks output "+handle+" 0";
 await h.all("console-run").click();
 assert.equal(h.calls.length,2);
 h.all("console-command").value="tasks next";
 h.all("task-offset").value="3";
 await h.all("console-run").click();
 assert.equal(h.calls.length,2,"edited output cursor must be denied");
 h.all("task-offset").value="2";
 h.all("task-handle").value="c".repeat(32);
 await h.all("console-run").click();
 assert.equal(h.calls.length,2,"different handle must not read another task");
});

test("Console next aliases have no network access without verified page state",async()=>{
 const h=nativeHarness([]);
 for(const cmd of ["files next","tasks next","files next 2",
  "tasks next "+("a".repeat(32)),"files next ; id","tasks next | sh"]){
  h.all("console-command").value=cmd;
  await h.all("console-run").click();
  assert.equal(h.calls.length,0,cmd);
  assert.match(h.all("status").textContent,/Lệnh bị từ chối/,cmd);
 }
});

test("Console source next invalid SHA discards old data and cannot reuse cursor",async()=>{
 const sha="a".repeat(64);
 const h=nativeHarness([
  sourcePage(sha,1,2,2,"1: first\n"),
  sourcePage("b".repeat(64),2,0,2,"2: altered\n")
 ]);
 h.all("console-command").value="files lines src/a.go 1";
 await h.all("console-run").click();
 h.all("console-command").value="files next";
 await h.all("console-run").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.all("file-read-next").disabled,true);
 assert.match(h.all("status").textContent,/Từ chối trang file/);
 assert.doesNotMatch(h.all("file-page-transcript").textContent,/first|altered/);
 await h.all("console-run").click();
 assert.equal(h.calls.length,2,"tampered SHA must disable future next");
});

test("Console tasks next rejection invalidates cursor and displays no prior output",async()=>{
 const handle="d".repeat(32);
 const h=nativeHarness([
  taskWindow(handle,0,2,4,"running",false,"hi"),
  {ok:false,error:"Task grant revoked"}
 ]);
 h.all("console-command").value="tasks output "+handle+" 0";
 await h.all("console-run").click();
 h.all("console-command").value="tasks next";
 await h.all("console-run").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.all("task-read-next").disabled,true);
 assert.doesNotMatch(h.all("task-transcript").textContent,/hi/);
 await h.all("console-run").click();
 assert.equal(h.calls.length,2,"revoked task cannot be advanced with old cursor");
});



test("Git diff literal search is case-insensitive and never spends another E2EE round",async()=>{
 const review="-- WORKTREE / UNSTAGED --\n+Alpha line\n-context\n"+
  "-- INDEX / STAGED --\n+alpha staged\n";
 const h=nativeHarness([review]);
 h.all("git-path").value="src/a.go";
 await h.actions.get("git_review").click();
 assert.equal(h.calls.length,1);
 h.all("git-diff-find").value="alpha";
 h.all("git-diff-find").listeners.get("input")();
 const rows=h.all("git-diff-find-entries").children;
 assert.equal(rows.length,2);
 assert.match(h.all("git-diff-find-count").textContent,/2 dòng khớp/);
 await rows[0].click();
 assert.match(h.all("git-diff-selected").textContent,/Alpha line/);
 assert.match(h.all("git-diff-selected").textContent,/chỉ xem, không stage/);
 assert.equal(h.calls.length,1);
 assert.equal(h.all("output").textContent,review);
});

test("Git diff literal filter respects staged and unstaged view without network",async()=>{
 const review="-- WORKTREE / UNSTAGED --\n+needle unstaged\n"+
  "-- INDEX / STAGED --\n+needle index\n";
 const h=nativeHarness([review]);
 h.all("git-path").value="src/a.go";
 await h.actions.get("git_review").click();
 h.all("git-diff-find").value="needle";
 h.all("git-diff-find").listeners.get("input")();
 assert.equal(h.all("git-diff-find-entries").children.length,2);
 h.all("git-review-side").value="index";
 h.all("git-review-side").listeners.get("input")();
 assert.equal(h.all("git-diff-find-entries").children.length,1);
 assert.match(h.all("git-diff-find-entries").children[0].textContent,/needle index/);
 h.all("git-review-side").value="worktree";
 h.all("git-review-side").listeners.get("input")();
 assert.equal(h.all("git-diff-find-entries").children.length,1);
 assert.match(h.all("git-diff-find-entries").children[0].textContent,/needle unstaged/);
 assert.equal(h.calls.length,1);
});

test("Git diff find rejects control and bidirectional Unicode queries without E2EE calls",async()=>{
 const h=nativeHarness(["+safe change\n"]);
 h.all("git-path").value="src/a.go";
 await h.actions.get("git_diff").click();
 for(const term of [" safe","safe ","x\nx","x\u202Ex","a".repeat(97)]){
  h.all("git-diff-find").value=term;
  h.all("git-diff-find").listeners.get("input")();
  assert.equal(h.all("git-diff-find-entries").children.length,0);
  assert.match(h.all("git-diff-find-count").textContent,/không hợp lệ/);
 }
 assert.equal(h.calls.length,1);
});

test("Git diff search escape is text-only and bounded to 32 display rows",async()=>{
 const diff=Array.from({length:40},(_,i)=>"+needle <img src=x onerror=bad"+i+">\n").join("");
 const h=nativeHarness([diff]);
 h.all("git-path").value="src/a.go";
 await h.actions.get("git_diff").click();
 h.all("git-diff-find").value="needle";
 h.all("git-diff-find").listeners.get("input")();
 assert.equal(h.all("git-diff-find-entries").children.length,32);
 assert.match(h.all("git-diff-find-count").textContent,/40 dòng khớp/);
 assert.match(h.all("git-diff-find-count").textContent,/32 dòng\/trang/);
 assert.match(h.all("git-diff-find-entries").children[0].textContent,/<img/);
 assert.equal(h.calls.length,1);
});

test("Git diff search refuses stale result when path or literal changes",async()=>{
 const h=nativeHarness(["+needle safe"]);
 h.all("git-path").value="src/a.go";
 await h.actions.get("git_diff").click();
 h.all("git-diff-find").value="needle";
 h.all("git-diff-find").listeners.get("input")();
 const row=h.all("git-diff-find-entries").children[0];
 h.all("git-diff-find").value="other";
 await row.click();
 assert.match(h.all("git-diff-selected").textContent,/Lựa chọn cũ/);
 h.all("git-path").value="src/b.go";
 await row.click();
 assert.match(h.all("git-diff-selected").textContent,/Lựa chọn cũ/);
 assert.equal(h.calls.length,1);
});

test("Old Git search card cannot bind to new authenticated diff at the same path",async()=>{
 const h=nativeHarness(["+needle OLD", "+needle NEW"]);
 h.all("git-path").value="src/a.go";
 await h.actions.get("git_diff").click();
 h.all("git-diff-find").value="needle";
 h.all("git-diff-find").listeners.get("input")();
 const stale=h.all("git-diff-find-entries").children[0];
 await h.actions.get("git_diff").click();
 assert.equal(h.all("git-diff-find-entries").children.length,0);
 h.all("git-diff-find").value="needle";
 h.all("git-diff-find").listeners.get("input")();
 assert.match(h.all("git-diff-find-entries").children[0].textContent,/NEW/);
 await stale.click();
 assert.match(h.all("git-diff-selected").textContent,/Lựa chọn cũ/);
 assert.equal(h.calls.length,2);
});

test("Local close scrubs all Git diff search query, rows and selected sensitive text",async()=>{
 const h=nativeHarness(["+needle confidential",{ok:true,text:"closed"}],{confirm:true});
 h.all("git-path").value="src/private.go";
 await h.actions.get("git_diff").click();
 h.all("git-diff-find").value="needle";
 h.all("git-diff-find").listeners.get("input")();
 await h.all("git-diff-find-entries").children[0].click();
 assert.match(h.all("git-diff-selected").textContent,/confidential/);
 await h.all("close").click();
 assert.equal(h.all("git-diff-find").value,"");
 assert.equal(h.all("git-diff-find-entries").children.length,0);
 assert.doesNotMatch(h.all("git-diff-selected").textContent,/confidential/);
 assert.equal(h.calls.length,2);
});



test("Console Tab completes only hard-coded read-only command prefix without network",async()=>{
 const h=nativeHarness([]);
 const input=h.all("console-command");
 let prevented=0;
 input.value="git statu";
 input.listeners.get("keydown")({key:"Tab",repeat:false,preventDefault(){prevented++}});
 assert.equal(prevented,1);
 assert.equal(input.value,"git status");
 assert.equal(h.calls.length,0);
 assert.match(h.all("status").textContent,/cục bộ/);
});

test("Console Tab autocompletes verified cursor and review-only verbs but never executes",async()=>{
 const h=nativeHarness([]);
 const input=h.all("console-command");
 for(const [typed,expected] of [["files nex","files next"],
   ["tasks nex","tasks next"],
   ["git review-comm","git review-commit"],
   ["tasks review-r","tasks review-run "],
   ["git review-switch-b","git review-switch-branch "]]){
  input.value=typed;
  input.listeners.get("keydown")({key:"Tab",repeat:false,preventDefault(){}});
  assert.equal(input.value,expected);
  assert.equal(h.calls.length,0);
 }
});

test("Ambiguous console Tab completion only extends common static prefix",async()=>{
 const h=nativeHarness([]);
 const input=h.all("console-command");
 input.value="git rev";
 input.listeners.get("keydown")({key:"Tab",repeat:false,preventDefault(){}});
 assert.equal(input.value,"git review");
 input.listeners.get("keydown")({key:"Tab",repeat:false,preventDefault(){}});
 assert.equal(input.value,"git review","when ambiguous no guessed Git write mode");
 assert.match(h.all("status").textContent,/Cú pháp có sẵn/);
 assert.equal(h.calls.length,0);
});

test("Console Tab never completes arbitrary Git shell syntax or unsafe control text",async()=>{
 const h=nativeHarness([]);
 const input=h.all("console-command");
 for(const raw of ["git push","sh","bash -c","git status;rm -rf /",
   "git diff \nrm -rf /","git rev\u202Eiew"]){
  input.value=raw;
  input.listeners.get("keydown")({key:"Tab",repeat:false,preventDefault(){}});
  assert.equal(input.value,raw);
  assert.equal(h.calls.length,0);
 }
});

test("Console Tab completion never sends E2EE and stays usable after quota exhaustion",async()=>{
 const h=nativeHarness(["1","2","3","4","5","6"]);
 const input=h.all("console-command");
 for(let i=0;i<6;i++){
  input.value="git status";
  await h.all("console-run").click();
 }
 assert.equal(h.calls.length,6);
 input.value="git branch";
 input.listeners.get("keydown")({key:"Tab",repeat:false,preventDefault(){}});
 assert.equal(input.value,"git branches");
 assert.equal(h.calls.length,6);
 assert.equal(h.all("quota").textContent.includes("6 / 6"),true);
});


test("Git diff search pages all 70 local matches without dropping tail or E2EE",async()=>{
 const diff=Array.from({length:70},(_,i)=>"+needle part "+i+"\n").join("");
 const h=nativeHarness([diff]);
 h.all("git-path").value="src/readme.txt";
 await h.actions.get("git_diff").click();
 h.all("git-diff-find").value="needle";
 h.all("git-diff-find").listeners.get("input")();
 assert.equal(h.all("git-diff-find-entries").children.length,32);
 assert.equal(h.all("git-diff-page").textContent,"Trang 1/3");
 assert.equal(h.all("git-diff-prev").disabled,true);
 assert.equal(h.all("git-diff-next").disabled,false);
 await h.all("git-diff-next").click();
 assert.equal(h.all("git-diff-page").textContent,"Trang 2/3");
 assert.equal(h.all("git-diff-find-entries").children.length,32);
 assert.match(h.all("git-diff-find-entries").children[0].textContent,/part 32/);
 await h.all("git-diff-next").click();
 assert.equal(h.all("git-diff-page").textContent,"Trang 3/3");
 assert.equal(h.all("git-diff-next").disabled,true);
 assert.equal(h.all("git-diff-find-entries").children.length,6);
 assert.match(h.all("git-diff-find-entries").children[5].textContent,/part 69/);
 await h.all("git-diff-prev").click();
 assert.equal(h.all("git-diff-page").textContent,"Trang 2/3");
 assert.equal(h.calls.length,1);
 assert.equal(h.all("output").textContent,diff);
});

test("Changing Git diff find query, view or file resets local page safely",async()=>{
 const first="-- WORKTREE / UNSTAGED --\n"+
  Array.from({length:35},(_,i)=>"+needle un "+i+"\n").join("")+
  "-- INDEX / STAGED --\n"+
  Array.from({length:36},(_,i)=>"+needle idx "+i+"\n").join("");
 const h=nativeHarness([first]);
 h.all("git-path").value="src/a.go";
 await h.actions.get("git_review").click();
 h.all("git-diff-find").value="needle";
 h.all("git-diff-find").listeners.get("input")();
 await h.all("git-diff-next").click();
 assert.equal(h.all("git-diff-page").textContent,"Trang 2/3");
 h.all("git-review-side").value="index";
 h.all("git-review-side").listeners.get("input")();
 assert.equal(h.all("git-diff-page").textContent,"Trang 1/2");
 assert.match(h.all("git-diff-find-entries").children[0].textContent,/idx 0/);
 await h.all("git-diff-next").click();
 h.all("git-diff-find").value="idx";
 h.all("git-diff-find").listeners.get("input")();
 assert.equal(h.all("git-diff-page").textContent,"Trang 1/2");
 h.all("git-path").value="src/b.go";
 h.all("git-path").listeners.get("input")();
 assert.equal(h.all("git-diff-page").textContent,"Trang 0/0");
 assert.equal(h.all("git-diff-next").disabled,true);
 assert.equal(h.calls.length,1);
});

test("Git diff pagination refuses old row from previous page even on unchanged snapshot",async()=>{
 const diff=Array.from({length:40},(_,i)=>"+needle row "+i+"\n").join("");
 const h=nativeHarness([diff]);
 h.all("git-path").value="src/a.go";
 await h.actions.get("git_diff").click();
 h.all("git-diff-find").value="needle";
 h.all("git-diff-find").listeners.get("input")();
 const stale=h.all("git-diff-find-entries").children[0];
 await h.all("git-diff-next").click();
 await stale.click();
 assert.match(h.all("git-diff-selected").textContent,/Lựa chọn cũ/);
 await h.all("git-diff-prev").click();
 await stale.click();
 assert.match(h.all("git-diff-selected").textContent,/Lựa chọn cũ/);
 await h.all("git-diff-next").click();
 await h.all("git-diff-find-entries").children[0].click();
 assert.match(h.all("git-diff-selected").textContent,/row 32/);
 assert.equal(h.calls.length,1);
});

test("Git diff pagination is disabled on unsafe query and after native close",async()=>{
 const diff=Array.from({length:40},(_,i)=>"+needle secret "+i+"\n").join("");
 const h=nativeHarness([diff,{ok:true,text:"closed"}],{confirm:true});
 h.all("git-path").value="src/secret.txt";
 await h.actions.get("git_diff").click();
 h.all("git-diff-find").value="needle";
 h.all("git-diff-find").listeners.get("input")();
 assert.equal(h.all("git-diff-next").disabled,false);
 h.all("git-diff-find").value="needle\u202E";
 h.all("git-diff-find").listeners.get("input")();
 assert.equal(h.all("git-diff-next").disabled,true);
 assert.equal(h.all("git-diff-page").textContent,"Trang 0/0");
 h.all("git-diff-find").value="needle";
 h.all("git-diff-find").listeners.get("input")();
 await h.all("git-diff-next").click();
 await h.all("close").click();
 assert.equal(h.all("git-diff-next").disabled,true);
 assert.equal(h.all("git-diff-prev").disabled,true);
 assert.equal(h.all("git-diff-find-entries").children.length,0);
 assert.equal(h.all("git-diff-find").value,"");
 assert.doesNotMatch(h.all("git-diff-page").textContent,/secret/);
 assert.equal(h.calls.length,2);
});


test("Git multi-file queue navigates 3 authenticated paths locally and requires explicit diff",async()=>{
 const h=nativeHarness([" M src/a.go\nM  src/b.go\n?? docs/new.txt\n","+selected diff"]);
 await h.actions.get("git_status").click();
 for(const path of ["src/a.go","src/b.go","docs/new.txt"]){
  h.all("git-path").value=path;
  await h.all("git-queue-add").click();
 }
 assert.equal(h.all("git-review-queue-summary").textContent.includes("3/3"),true);
 assert.equal(h.all("git-path").value,"docs/new.txt");
 assert.equal(h.calls.length,1,"queue selection must not send another E2EE request");
 assert.equal(h.all("git-queue-next").disabled,true);
 await h.all("git-queue-prev").click();
 assert.equal(h.all("git-path").value,"src/b.go");
 await h.all("git-queue-prev").click();
 assert.equal(h.all("git-path").value,"src/a.go");
 assert.equal(h.all("git-queue-prev").disabled,true);
 await h.all("git-queue-next").click();
 assert.equal(h.all("git-path").value,"src/b.go");
 assert.equal(h.all("file").value,"src/b.go");
 assert.equal(h.calls.length,1);
 await h.actions.get("git_diff").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].request.operation,"git_diff");
 assert.equal(h.calls[1].request.path,"src/b.go");
 assert.equal(h.all("output").textContent,"+selected diff");
});

test("Git multi-file queue refuses unverified paths, duplicate and more than 8",async()=>{
 const status=Array.from({length:9},(_,i)=>" M src/file"+i+".txt\n").join("");
 const h=nativeHarness([status]);
 await h.actions.get("git_status").click();
 h.all("git-path").value="private/.key";
 await h.all("git-queue-add").click();
 assert.equal(h.all("git-review-queue-summary").textContent.startsWith("0/0"),true);
 for(let i=0;i<8;i++){
  h.all("git-path").value="src/file"+i+".txt";
  await h.all("git-queue-add").click();
 }
 assert.equal(h.all("git-review-queue-summary").textContent.includes("8/8"),true);
 h.all("git-path").value="src/file8.txt";
 await h.all("git-queue-add").click();
 assert.match(h.all("status").textContent,/tối đa 8/);
 assert.doesNotMatch(h.all("git-review-queue-list").textContent,/file8.txt/);
 h.all("git-path").value="src/file0.txt";
 await h.all("git-queue-add").click();
 assert.match(h.all("git-review-queue-summary").textContent,/1\/8/);
 assert.equal(h.all("git-path").value,"src/file0.txt");
 assert.equal(h.calls.length,1);
});

test("New Git status invalidates multi-file review queue and detached old status cards",async()=>{
 const h=nativeHarness([" M src/old.go\n"," M src/fresh.go\n"]);
 await h.actions.get("git_status").click();
 const stale=h.all("git-status-files").children[0];
 await stale.click();
 await h.all("git-queue-add").click();
 assert.match(h.all("git-review-queue-list").textContent,/src\/old.go/);
 await h.actions.get("git_status").click();
 assert.match(h.all("git-review-queue-summary").textContent,/0\/0/);
 assert.doesNotMatch(h.all("git-review-queue-list").textContent,/old.go/);
 await stale.click();
 assert.match(h.all("status").textContent,/Git Status cũ/);
 assert.equal(h.all("git-path").value,"src/old.go","stale button may not change selected path");
 await h.all("git-queue-add").click();
 assert.match(h.all("status").textContent,/Chỉ thêm file/);
 assert.equal(h.calls.length,2);
});

test("Any uncertain Git mutation erases queued files before host response",async()=>{
 let resolve;
 const pending=()=>new Promise(r=>{resolve=r});
 const h=nativeHarness([" M src/main.go\n",{text:"review",confirm:"STAGE src/main.go"},pending]);
 await h.actions.get("git_status").click();
 h.all("git-path").value="src/main.go";
 await h.all("git-queue-add").click();
 assert.match(h.all("git-review-queue-list").textContent,/main.go/);
 h.actions.get("git_write_review").dataset.mode="stage";
 await h.actions.get("git_write_review").click();
 h.all("git-confirm-entry").value="STAGE src/main.go";
 const inflight=h.all("execute-git").click();
 assert.equal(typeof resolve,"function");
 assert.match(h.all("git-review-queue-summary").textContent,/0\/0/);
 assert.doesNotMatch(h.all("git-review-queue-list").textContent,/main.go/);
 assert.equal(h.all("git-queue-next").disabled,true);
 resolve({ok:false,error:"Lost response"});
 await inflight;
 assert.equal(h.calls.length,3);
});

test("Closing native session clears multi-file Git review queue without remote writes",async()=>{
 const h=nativeHarness([" M src/secret.go\n",{ok:true,text:"closed"}],{confirm:true});
 await h.actions.get("git_status").click();
 h.all("git-path").value="src/secret.go";
 await h.all("git-queue-add").click();
 assert.match(h.all("git-review-queue-list").textContent,/secret.go/);
 await h.all("close").click();
 assert.doesNotMatch(h.all("git-review-queue-list").textContent,/secret.go/);
 assert.equal(h.all("git-queue-add").disabled,true);
 assert.equal(h.all("git-queue-prev").disabled,true);
 assert.equal(h.all("git-queue-next").disabled,true);
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].path,"/api/close");
});


test("Detached Git status card cannot select a file after local filter redraw",async()=>{
 const h=nativeHarness([" M src/a.go\n M src/b.go\n"]);
 await h.actions.get("git_status").click();
 const stale=h.all("git-status-files").children[0];
 h.all("git-status-filter").value="b.go";
 h.all("git-status-filter").listeners.get("input")();
 assert.equal(h.all("git-status-files").children.length,1);
 await stale.click();
 assert.match(h.all("status").textContent,/Git Status cũ/);
 assert.equal(h.all("git-path").value,"");
 await h.all("git-status-files").children[0].click();
 assert.equal(h.all("git-path").value,"src/b.go");
 assert.equal(h.calls.length,1);
});


test("Filtered Git queue batch selection is local, bounded and preserves status order",async()=>{
 const h=nativeHarness([" M src/a.go\n M src/b.go\nM  docs/c.md\n"]);
 await h.actions.get("git_status").click();
 h.all("git-status-filter").value="src/";
 h.all("git-status-filter").listeners.get("input")();
 await h.all("git-queue-add-visible").click();
 assert.match(h.all("git-review-queue-summary").textContent,/1\/2/);
 assert.match(h.all("git-review-queue-list").textContent,/src\/a.go/);
 assert.match(h.all("git-review-queue-list").textContent,/src\/b.go/);
 assert.doesNotMatch(h.all("git-review-queue-list").textContent,/docs\/c.md/);
 h.all("git-status-filter").value="";
 h.all("git-status-filter").listeners.get("input")();
 await h.all("git-queue-add-visible").click();
 assert.match(h.all("git-review-queue-summary").textContent,/1\/3/);
 assert.equal(h.all("git-path").value,"src/a.go");
 assert.match(h.all("git-review-queue-list").textContent,/docs\/c.md/);
 assert.equal(h.calls.length,1,"batch selection must not consume E2EE or stage any file");
});

test("Git queue add-visible rejects overflow atomically and invalid filter",async()=>{
 const status=Array.from({length:9},(_,i)=>" M src/file"+i+".go\n").join("");
 const h=nativeHarness([status]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 assert.match(h.all("status").textContent,/Không thêm một phần/);
 assert.match(h.all("git-review-queue-summary").textContent,/0\/0/);
 assert.equal(h.calls.length,1);
 h.all("git-status-filter").value="file0";
 h.all("git-status-filter").listeners.get("input")();
 await h.all("git-queue-add-visible").click();
 assert.match(h.all("git-review-queue-summary").textContent,/1\/1/);
 h.all("git-status-filter").value="x\u202E";
 h.all("git-status-filter").listeners.get("input")();
 await h.all("git-queue-add-visible").click();
 assert.match(h.all("status").textContent,/không hợp lệ/);
 assert.match(h.all("git-review-queue-summary").textContent,/1\/1/);
 assert.equal(h.calls.length,1);
});

test("Git queue counts only authenticated well-formed two-way review, never one-sided diff",async()=>{
 const review="-- WORKTREE / UNSTAGED --\n+new\n-- INDEX / STAGED --\n+staged\n";
 const h=nativeHarness([" M src/a.go\n M src/b.go\n","+one-side",review]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 await h.actions.get("git_diff").click();
 assert.match(h.all("git-review-queue-summary").textContent,/0\/2/);
 assert.match(h.all("git-review-queue-list").textContent,/chưa tải review 2 chiều/);
 await h.actions.get("git_review").click();
 assert.match(h.all("git-review-queue-summary").textContent,/đã tải review 2 chiều 1\/2/);
 assert.match(h.all("git-review-queue-list").textContent,/src\/a.go · đã tải review 2 chiều/);
 assert.match(h.all("git-review-queue-list").textContent,/src\/b.go · chưa tải review 2 chiều/);
 assert.equal(h.calls.length,3);
});

test("Git queue next-unreviewed selects next file without any auto read or approval",async()=>{
 const review="-- WORKTREE / UNSTAGED --\n+change\n-- INDEX / STAGED --\n+staged";
 const h=nativeHarness([" M src/a.go\n M src/b.go\n M src/c.go\n",review]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 assert.equal(h.all("git-path").value,"src/a.go");
 await h.actions.get("git_review").click();
 assert.equal(h.all("git-queue-next-unread").disabled,false);
 await h.all("git-queue-next-unread").click();
 assert.equal(h.all("git-path").value,"src/b.go");
 assert.equal(h.all("git-review-queue-summary").textContent.startsWith("File 2/3"),true);
 await h.all("git-queue-next-unread").click();
 assert.equal(h.all("git-path").value,"src/c.go");
 await h.all("git-queue-next-unread").click();
 assert.equal(h.all("git-path").value,"src/b.go","wrap past reviewed file to next incomplete file");
 assert.equal(h.calls.length,2,"all queue navigation must stay local");
 assert.equal(h.all("git-approval").classList!==undefined,true);
});

test("Malformed Git two-way reply and oversized output cannot mark queued review loaded",async()=>{
 const oversized="x".repeat(8300);
 const h=nativeHarness([" M src/a.go\n","partial review",oversized]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 await h.actions.get("git_review").click();
 assert.match(h.all("git-review-queue-summary").textContent,/0\/1/);
 await h.actions.get("git_review").click();
 assert.match(h.all("git-review-queue-summary").textContent,/0\/1/);
 assert.match(h.all("git-diff-kind").textContent,/Không hợp lệ/);
 assert.equal(h.calls.length,3);
});

test("Git queue review progress is cleared on refreshed status and native close",async()=>{
 const review="-- WORKTREE / UNSTAGED --\n+text\n-- INDEX / STAGED --\n";
 const h=nativeHarness([" M src/a.go\n",review," M src/a.go\n",review,{ok:true,text:"closed"}],{confirm:true});
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 await h.actions.get("git_review").click();
 assert.match(h.all("git-review-queue-summary").textContent,/1\/1/);
 await h.actions.get("git_status").click();
 assert.match(h.all("git-review-queue-summary").textContent,/0\/0/);
 await h.all("git-queue-add-visible").click();
 assert.match(h.all("git-review-queue-summary").textContent,/0\/1/);
 await h.actions.get("git_review").click();
 assert.match(h.all("git-review-queue-summary").textContent,/1\/1/);
 await h.all("close").click();
 assert.doesNotMatch(h.all("git-review-queue-list").textContent,/src\/a.go/);
 assert.equal(h.all("git-queue-next-unread").disabled,true);
 assert.equal(h.all("git-queue-add-visible").disabled,true);
 assert.equal(h.calls.length,5);
});


test("Authenticated Git diff hunk navigator reads separate patches without another E2EE",async()=>{
 const diff="diff --git a/src/main.go b/src/main.go\n"+
  "@@ -1,2 +1,2 @@ old\n-old\n+new\n context\n"+
  "@@ -10,1 +10,2 @@ next\n+new function\n";
 const h=nativeHarness([diff]);
 h.all("git-path").value="src/main.go";
 await h.actions.get("git_diff").click();
 assert.equal(h.calls.length,1);
 assert.match(h.all("git-hunk-count").textContent,/2 hunk/);
 assert.equal(h.all("git-hunk-entries").children.length,2);
 await h.all("git-hunk-entries").children[0].click();
 assert.match(h.all("git-hunk-snippet").textContent,/\+new/);
 assert.doesNotMatch(h.all("git-hunk-snippet").textContent,/new function/);
 await h.all("git-hunk-entries").children[1].click();
 assert.match(h.all("git-hunk-snippet").textContent,/new function/);
 assert.doesNotMatch(h.all("git-hunk-snippet").textContent,/\-old/);
 assert.equal(h.calls.length,1,"local hunk selection never sends RPC");
});

test("Git hunk navigation pages all 18 headers without silently losing the tail",async()=>{
 const diff=Array.from({length:18},(_,i)=>"@@ -"+(i+1)+",1 +"+(i+1)+",1 @@\n-old"+i+"\n+new"+i+"\n").join("");
 const h=nativeHarness([diff]);
 h.all("git-path").value="src/main.go";
 await h.actions.get("git_diff").click();
 const previous=h.all("git-hunk-entries").children[0];
 assert.equal(h.all("git-hunk-entries").children.length,16);
 assert.equal(h.all("git-hunk-page").textContent,"Trang 1/2");
 await h.all("git-hunk-next").click();
 assert.equal(h.all("git-hunk-entries").children.length,2);
 assert.equal(h.all("git-hunk-page").textContent,"Trang 2/2");
 assert.equal(h.all("git-hunk-next").disabled,true);
 await previous.click();
 assert.match(h.all("git-hunk-selected").textContent,/Hunk cũ/);
 await h.all("git-hunk-entries").children[1].click();
 assert.match(h.all("git-hunk-snippet").textContent,/\+new17/);
 await h.all("git-hunk-prev").click();
 assert.equal(h.all("git-hunk-page").textContent,"Trang 1/2");
 assert.equal(h.calls.length,1);
});

test("Git hunk cards are invalidated when two-way review changes staged side",async()=>{
 const data="-- WORKTREE / UNSTAGED --\n@@ -1,1 +1,1 @@\n-a\n+b\n"+
  "\n-- INDEX / STAGED --\n@@ -4,1 +4,1 @@\n-c\n+d\n";
 const h=nativeHarness([data]);
 h.all("git-path").value="src/main.go";
 await h.actions.get("git_review").click();
 const old=h.all("git-hunk-entries").children[0];
 assert.equal(h.all("git-hunk-entries").children.length,2);
 h.all("git-review-side").value="index";
 h.all("git-review-side").listeners.get("input")();
 assert.equal(h.all("git-hunk-entries").children.length,1);
 await old.click();
 assert.match(h.all("git-hunk-selected").textContent,/Hunk cũ/);
 await h.all("git-hunk-entries").children[0].click();
 assert.match(h.all("git-hunk-snippet").textContent,/\+d/);
 assert.doesNotMatch(h.all("git-hunk-snippet").textContent,/\+b/);
 assert.equal(h.calls.length,1,"two-way review is one GUI action");
});

test("Git hunk parser excludes malformed lines and escapes untrusted text only",async()=>{
 const diff="@@ malformed header @@\n"+
  "@@ -1,1 +1,1 @@ safe\n"+
  "-old\n+<svg onload=attack>\n"+
  "@@ -foo +bar @@\n";
 const h=nativeHarness([diff]);
 h.all("git-path").value="src/main.go";
 await h.actions.get("git_diff").click();
 assert.equal(h.all("git-hunk-entries").children.length,1);
 await h.all("git-hunk-entries").children[0].click();
 assert.match(h.all("git-hunk-snippet").textContent,/<svg onload=attack>/);
 assert.equal(h.all("git-hunk-snippet").children.length,0,"only textContent, never nodes");
 assert.equal(h.calls.length,1);
});

test("Git hunk selection fails closed after refresh to another authenticated diff at same path",async()=>{
 const first="@@ -1,1 +1,1 @@\n-a\n+b\n";
 const second="@@ -4,1 +4,1 @@\n-x\n+y\n";
 const h=nativeHarness([first,second]);
 h.all("git-path").value="src/main.go";
 await h.actions.get("git_diff").click();
 const old=h.all("git-hunk-entries").children[0];
 await h.actions.get("git_diff").click();
 await old.click();
 assert.match(h.all("git-hunk-selected").textContent,/Hunk cũ/);
 assert.doesNotMatch(h.all("git-hunk-snippet").textContent,/\+b/);
 await h.all("git-hunk-entries").children[0].click();
 assert.match(h.all("git-hunk-snippet").textContent,/\+y/);
 assert.equal(h.calls.length,2);
});

test("Closing native session purges authenticated hunk rows, patch and navigation",async()=>{
 const h=nativeHarness(["@@ -1,1 +1,1 @@\n-private\n+secret\n",{ok:true,text:"closed"}],{confirm:true});
 h.all("git-path").value="src/main.go";
 await h.actions.get("git_diff").click();
 await h.all("git-hunk-entries").children[0].click();
 assert.match(h.all("git-hunk-snippet").textContent,/secret/);
 await h.all("close").click();
 assert.doesNotMatch(h.all("git-hunk-snippet").textContent,/secret/);
 assert.equal(h.all("git-hunk-entries").children.length,0);
 assert.equal(h.all("git-hunk-prev").disabled,true);
 assert.equal(h.all("git-hunk-next").disabled,true);
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].path,"/api/close");
});


test("Git status reports locally tracked ahead/behind and conflict count from one E2EE read",async()=>{
 const report="## feature/remote...origin/feature/remote [ahead 2, behind 3]\n"+
  "UU src/merge.go\nAA docs/shared.md\n M src/clean.go\n";
 const h=nativeHarness([report]);
 await h.actions.get("git_status").click();
 assert.match(h.all("git-upstream-summary").textContent,/ahead 2 \/ behind 3/);
 assert.match(h.all("git-upstream-summary").textContent,/chưa fetch từ remote/);
 assert.match(h.all("git-status-count").textContent,/2 conflict/);
 assert.equal(h.all("git-next-conflict").disabled,false);
 h.all("git-status-scope").value="conflict";
 h.all("git-status-scope").listeners.get("input")();
 assert.equal(h.all("git-status-files").children.length,2);
 assert.equal(h.calls.length,1);
});

test("Git conflict-next cycles only verified unmerged paths; no implicit diff or Git write",async()=>{
 const h=nativeHarness(["## main\nUD src/ud.go\nDU src/du.go\n"+
  "DD src/dd.go\n M src/other.go\n"]);
 await h.actions.get("git_status").click();
 await h.all("git-next-conflict").click();
 assert.equal(h.all("git-path").value,"src/ud.go");
 await h.all("git-next-conflict").click();
 assert.equal(h.all("git-path").value,"src/du.go");
 await h.all("git-next-conflict").click();
 assert.equal(h.all("git-path").value,"src/dd.go");
 await h.all("git-next-conflict").click();
 assert.equal(h.all("git-path").value,"src/ud.go");
 assert.equal(h.all("file").value,"src/ud.go");
 assert.equal(h.calls.length,1);
 assert.match(h.all("status").textContent,/bấm Diff\/Review/);
});

test("Git conflict filter supports queue-only bulk selection without network",async()=>{
 const h=nativeHarness(["## main\nUU src/a.go\nAA src/b.go\n"+
  " M src/normal.go\n"]);
 await h.actions.get("git_status").click();
 h.all("git-status-scope").value="conflict";
 h.all("git-status-scope").listeners.get("input")();
 await h.all("git-queue-add-visible").click();
 assert.match(h.all("git-review-queue-summary").textContent,/1\/2/);
 assert.match(h.all("git-review-queue-list").textContent,/src\/a.go/);
 assert.match(h.all("git-review-queue-list").textContent,/src\/b.go/);
 assert.doesNotMatch(h.all("git-review-queue-list").textContent,/src\/normal.go/);
 assert.equal(h.calls.length,1);
});

test("Git Status never invents a zero ahead/behind for absent, invalid or gone upstream",async()=>{
 const cases=[
  ["## main\n","Upstream chưa được xác định"],
  ["## HEAD (no branch)\n","Detached HEAD"],
  ["## main...origin/main [gone]\n","upstream mất"],
  ["## main...origin/main [ahead 1000000001]\n","Upstream chưa được xác định"],
  ["## main...origin/main \u202E[ahead 2]\n","Upstream chưa được xác định"],
  ["## main...origin/main [ahead 2, behind 3] extra\n","Upstream chưa được xác định"],
  ["## main...origin/main\n","ahead 0 / behind 0"],
  ["## main...origin/main [behind 5]\n","ahead 0 / behind 5"],
  ["## main...origin/main [ahead 7]\n","ahead 7 / behind 0"]
 ];
 for(const [status,expected] of cases){
  const h=nativeHarness([status]);
  await h.actions.get("git_status").click();
  assert.match(h.all("git-upstream-summary").textContent,new RegExp(expected),status);
  assert.equal(h.calls.length,1);
 }
});

test("Git conflict and tracking UI clear on refresh and failed Agent status",async()=>{
 const status="## main...origin/main [ahead 1]\nUU src/conflict.go\n";
 const h=nativeHarness([status,{ok:false,error:"Agent revoked"}]);
 await h.actions.get("git_status").click();
 assert.equal(h.all("git-next-conflict").disabled,false);
 await h.actions.get("git_status").click();
 assert.equal(h.all("git-next-conflict").disabled,true);
 assert.match(h.all("git-upstream-summary").textContent,/Không tải được/);
 assert.equal(h.all("git-status-files").children.length,0);
 await h.all("git-next-conflict").click();
 assert.equal(h.all("git-path").value,"");
 assert.equal(h.calls.length,2);
});

test("Git conflict and tracking UI clear before uncertain host mutation",async()=>{
 let resolve;
 const pending=()=>new Promise(r=>{resolve=r});
 const h=nativeHarness([
  "## main...origin/main [ahead 4]\nUU src/merge.go\n",
  {text:"review",confirm:"STAGE src/merge.go"},pending
 ]);
 await h.actions.get("git_status").click();
 h.all("git-path").value="src/merge.go";
 h.actions.get("git_write_review").dataset.mode="stage";
 await h.actions.get("git_write_review").click();
 h.all("git-confirm-entry").value="STAGE src/merge.go";
 const running=h.all("execute-git").click();
 assert.equal(typeof resolve,"function");
 assert.equal(h.all("git-next-conflict").disabled,true);
 assert.match(h.all("git-upstream-summary").textContent,/Tracking cũ/);
 resolve({ok:false,error:"Lost response"});
 await running;
 assert.equal(h.calls.length,3);
});

test("Closing native GUI scrubs upstream branch names and disables conflict navigation",async()=>{
 const h=nativeHarness(["## secret/test...origin/private [ahead 8]\nUU src/merge.go\n",
  {ok:true,text:"closed"}],{confirm:true});
 await h.actions.get("git_status").click();
 assert.match(h.all("git-upstream-summary").textContent,/secret\/test/);
 await h.all("close").click();
 assert.doesNotMatch(h.all("git-upstream-summary").textContent,/secret|private/);
 assert.equal(h.all("git-next-conflict").disabled,true);
 assert.equal(h.calls.length,2);
});


test("Console git tracking and git conflicts use only previously authenticated status",async()=>{
 const h=nativeHarness(["## main...origin/main [ahead 1, behind 2]\nUU src/a.go\n M src/b.go\n"]);
 h.all("console-command").value="git tracking";
 await h.all("console-run").click();
 assert.match(h.all("status").textContent,/Chưa xác thực/);
 assert.equal(h.calls.length,0);
 await h.actions.get("git_status").click();
 h.all("console-command").value="git tracking";
 await h.all("console-run").click();
 assert.match(h.all("console-transcript").textContent,/ahead 1 \/ behind 2/);
 h.all("console-command").value="git conflicts";
 await h.all("console-run").click();
 assert.match(h.all("console-transcript").textContent,/UU src\/a.go/);
 assert.doesNotMatch(h.all("console-transcript").textContent,/src\/b.go/);
 assert.equal(h.calls.length,1,"cached command never sends another E2EE action");
});

test("Console git local commands cannot reuse tracking after status refresh denial",async()=>{
 const h=nativeHarness(["## main...origin/main [ahead 5]\nUU src/a.go\n",
   {ok:false,error:"TaskDeck principal revoked"}]);
 await h.actions.get("git_status").click();
 await h.actions.get("git_status").click();
 h.all("console-command").value="git conflicts";
 await h.all("console-run").click();
 assert.match(h.all("status").textContent,/Chưa xác thực/);
 assert.doesNotMatch(h.all("console-transcript").textContent,/src\/a.go/);
 h.all("console-command").value="git tracking";
 await h.all("console-run").click();
 assert.match(h.all("status").textContent,/Chưa xác thực/);
 assert.equal(h.calls.length,2);
});

test("Console git status local command completions cannot execute unsafe Git remote verbs",async()=>{
 const h=nativeHarness([]);
 const input=h.all("console-command"),key=input.listeners.get("keydown");
 let prevented=0;
 input.value="git track";
 key({key:"Tab",repeat:false,preventDefault(){prevented++;}});
 assert.equal(input.value,"git tracking");
 input.value="git conflict";
 key({key:"Tab",repeat:false,preventDefault(){prevented++;}});
 assert.equal(input.value,"git conflicts");
 input.value="git fetch";
 key({key:"Tab",repeat:false,preventDefault(){prevented++;}});
 assert.equal(input.value,"git fetch");
 assert.equal(h.calls.length,0);
 assert.equal(prevented,3);
});

test("Oversized authenticated Git Status does not populate Console tracking/conflict cache",async()=>{
 const h=nativeHarness(["## main...origin/main [ahead 1]\n"+
  "UU src/a.go\n"+"x".repeat(4200)]);
 await h.actions.get("git_status").click();
 h.all("console-command").value="git conflicts";
 await h.all("console-run").click();
 assert.match(h.all("status").textContent,/Chưa xác thực/);
 assert.equal(h.all("git-next-conflict").disabled,true);
 assert.equal(h.calls.length,1);
});


test("Trusted Console transcript literal search filters authenticated records locally",async()=>{
 const h=nativeHarness([]);
 for(const command of ["pwd","cd src","pwd"]){
  h.all("console-command").value=command;
  await h.all("console-run").click();
 }
 assert.equal(h.calls.length,0,"pwd/cd stay local");
 assert.equal(h.all("console-count").textContent,"3 kết quả gần nhất");
 h.all("console-transcript-search").value="Thư mục cục bộ";
 h.all("console-transcript-search").listeners.get("input")();
 assert.match(h.all("console-search-count").textContent,/1\/3/);
 assert.match(h.all("console-transcript").textContent,/cd src/);
 assert.doesNotMatch(h.all("console-transcript").textContent,/\$ pwd/);
 h.all("console-transcript-search").value="";
 h.all("console-transcript-search").listeners.get("input")();
 assert.match(h.all("console-search-count").textContent,/3\/3/);
 assert.equal(h.calls.length,0);
});

test("Console search remains scoped to last four bounded records and never requests Agent",async()=>{
 const h=nativeHarness([]);
 for(const command of ["cd src0","cd ..","cd src1","cd ..","cd src4"]){
  h.all("console-command").value=command;
  await h.all("console-run").click();
 }
 h.all("console-transcript-search").value="src0";
 h.all("console-transcript-search").listeners.get("input")();
 assert.match(h.all("console-search-count").textContent,/0\/4/);
 assert.doesNotMatch(h.all("console-transcript").textContent,/src0/);
 h.all("console-transcript-search").value="src4";
 h.all("console-transcript-search").listeners.get("input")();
 assert.match(h.all("console-search-count").textContent,/1\/4/);
 assert.match(h.all("console-transcript").textContent,/src4/);
 assert.equal(h.calls.length,0);
});

test("Console search rejects Bidi, control, and excessive UTF-8 literals without RPC",async()=>{
 const h=nativeHarness([]);
 h.all("console-command").value="pwd";await h.all("console-run").click();
 for(const input of ["\u202Eevil","\n", "a".repeat(97)," leading "]){
  h.all("console-transcript-search").value=input;
  h.all("console-transcript-search").listeners.get("input")();
  assert.equal(h.all("console-search-count").textContent,"Bộ lọc không hợp lệ");
  assert.doesNotMatch(h.all("console-transcript").textContent,/\$ pwd/);
 }
 assert.equal(h.calls.length,0);
});

test("Console transcript filter updates after new verified response and stays read-only",async()=>{
 const h=nativeHarness(["authenticated foobar remote output"]);
 h.all("console-transcript-search").value="foobar";
 h.all("console-transcript-search").listeners.get("input")();
 assert.match(h.all("console-search-count").textContent,/0\/0/);
 h.all("console-command").value="git log";
 await h.all("console-run").click();
 assert.match(h.all("console-search-count").textContent,/1\/1/);
 assert.match(h.all("console-transcript").textContent,/foobar/);
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_log");
});

test("Console clear and native close wipe transcript search and cached sensitive output",async()=>{
 const h=nativeHarness(["secret log record",{ok:true,text:"closed"}],{confirm:true});
 h.all("console-command").value="git log";await h.all("console-run").click();
 h.all("console-transcript-search").value="secret";
 h.all("console-transcript-search").listeners.get("input")();
 assert.match(h.all("console-transcript").textContent,/secret/);
 h.all("console-command").value="clear";await h.all("console-run").click();
 assert.equal(h.all("console-transcript-search").value,"");
 assert.doesNotMatch(h.all("console-transcript").textContent,/secret/);
 assert.match(h.all("console-search-count").textContent,/0\/0/);
 h.all("console-transcript-search").value="another query";
 await h.all("close").click();
 assert.equal(h.all("console-transcript-search").value,"");
 assert.doesNotMatch(h.all("console-transcript").textContent,/secret/);
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].path,"/api/close");
});


test("Git conflict viewer demands authenticated conflict status and explicit E2EE file read",async()=>{
 const content="<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\n";
 const h=nativeHarness(["UU src/app.go\n",content]);
 await h.all("git-conflict-read").click();
 assert.equal(h.calls.length,0,"unauthenticated request never reaches Agent");
 await h.actions.get("git_status").click();
 assert.equal(h.all("git-conflict-read").disabled,true,"user must select verified conflict path");
 await h.all("git-status-files").children[0].click();
 assert.equal(h.all("git-conflict-read").disabled,false);
 assert.equal(h.calls.length,1,"selecting Git file does not read its contents");
 await h.all("git-conflict-read").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].request.operation,"file_view");
 assert.equal(h.calls[1].request.path,"src/app.go");
 assert.equal(h.all("git-conflict-marker-entries").children.length,1);
 await h.all("git-conflict-marker-entries").children[0].click();
 assert.match(h.all("git-conflict-marker-text").textContent,/ours/);
 assert.match(h.all("git-conflict-marker-text").textContent,/theirs/);
 assert.match(h.all("git-conflict-marker-selected").textContent,/chỉ đọc/);
});

test("Git conflict viewer rejects malformed, nested and diff3 markers atomically",async()=>{
 const cases=[
  "<<<<<<< HEAD\nours\n=======\nno ending\n",
  "<<<<<<< HEAD\n<<<<<<< nested\n=======\nvalue\n>>>>>>> x\n",
  "<<<<<<< HEAD\nours\n||||||| base\n=======\ntheirs\n>>>>>>> x\n",
  "<<<<<<< HEAD\nours\n=======\nagain\n=======\ntheirs\n>>>>>>> x\n"
 ];
 for(const content of cases){
  const h=nativeHarness(["UU src/app.go\n",content]);
  await h.actions.get("git_status").click();
  await h.all("git-status-files").children[0].click();
  await h.all("git-conflict-read").click();
  assert.equal(h.all("git-conflict-marker-entries").children.length,0);
  assert.match(h.all("git-conflict-marker-count").textContent,/Marker thiếu/);
  assert.equal(h.calls.length,2);
 }
});

test("Git conflict viewer navigates two markers safely and never interprets hostile markup",async()=>{
 const content="prefix\n<<<<<<< HEAD\nold\n=======\n+<img onerror=evil>\n>>>>>>> branch\n"+
  "between\n<<<<<<< HEAD\none\n=======\ntwo\n>>>>>>> next\n";
 const h=nativeHarness(["UU src/app.go\n",content]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 assert.equal(h.all("git-conflict-marker-entries").children.length,2);
 await h.all("git-conflict-marker-entries").children[0].click();
 assert.match(h.all("git-conflict-marker-text").textContent,/<img onerror=evil>/);
 assert.equal(h.all("git-conflict-marker-text").children.length,0,"must never create HTML nodes");
 assert.doesNotMatch(h.all("git-conflict-marker-text").textContent,/two/);
 await h.all("git-conflict-marker-entries").children[1].click();
 assert.match(h.all("git-conflict-marker-text").textContent,/two/);
 assert.doesNotMatch(h.all("git-conflict-marker-text").textContent,/evil/);
 assert.equal(h.calls.length,2,"selecting conflict block is purely local");
});

test("Refreshing Git status clears old conflict markers and detached buttons cannot restore them",async()=>{
 const conflict="<<<<<<< HEAD\nsecret\n=======\nnormal\n>>>>>>> branch\n";
 const h=nativeHarness(["UU src/app.go\n",conflict," M src/app.go\n"]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 const stale=h.all("git-conflict-marker-entries").children[0];
 await stale.click();
 assert.match(h.all("git-conflict-marker-text").textContent,/secret/);
 await h.actions.get("git_status").click();
 assert.equal(h.all("git-conflict-marker-entries").children.length,0);
 assert.doesNotMatch(h.all("git-conflict-marker-text").textContent,/secret/);
 assert.equal(h.all("git-conflict-read").disabled,true);
 await stale.click();
 assert.match(h.all("git-conflict-marker-selected").textContent,/cũ/);
 assert.doesNotMatch(h.all("git-conflict-marker-text").textContent,/secret/);
 assert.equal(h.calls.length,3);
});

test("Late conflict file E2EE response cannot rebind to a different selected file",async()=>{
 let resolve;
 const pending=()=>new Promise(r=>{resolve=r});
 const h=nativeHarness(["UU src/a.go\nUU src/b.go\n",pending]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 const inflight=h.all("git-conflict-read").click();
 assert.equal(typeof resolve,"function");
 h.all("git-path").value="src/b.go";
 resolve("<<<<<<< HEAD\nsecret\n=======\nnew\n>>>>>>> branch\n");
 await inflight;
 assert.equal(h.all("git-conflict-marker-entries").children.length,0);
 assert.doesNotMatch(h.all("git-conflict-marker-text").textContent,/secret/);
 assert.match(h.all("status").textContent,/đã đổi/);
 assert.equal(h.calls.length,2);
});

test("Host write uncertainty invalidates previously authenticated conflict markers",async()=>{
 let deliver;
 const pending=()=>new Promise(r=>{deliver=r});
 const path="src/app.go",confirm="EDIT "+path+" abcdef012345";
 const conflict="<<<<<<< HEAD\nold\n=======\nnew\n>>>>>>> branch\n";
 const h=nativeHarness(["UU "+path+"\n",conflict,{text:conflict,confirm},pending]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 await h.all("git-conflict-marker-entries").children[0].click();
 h.all("file").value=path;
 await h.actions.get("file_edit_review").click();
 h.all("file-edit-text").value="resolved\n";
 h.all("file-edit-confirm").value=confirm;
 const inflight=h.all("execute-file-edit").click();
 assert.equal(typeof deliver,"function");
 assert.equal(h.all("git-conflict-marker-entries").children.length,0);
 assert.equal(h.all("git-conflict-read").disabled,true);
 assert.doesNotMatch(h.all("git-conflict-marker-text").textContent,/old/);
 deliver({ok:false,error:"Uncertain E2EE response"});
 await inflight;
 assert.equal(h.calls.length,4);
});

test("Native close scrubs conflict marker source and disables further reads",async()=>{
 const secret="<<<<<<< HEAD\nprivate token\n=======\nnew\n>>>>>>> branch\n";
 const h=nativeHarness(["UU src/private.go\n",secret,{ok:true,text:"closed"}],{confirm:true});
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 await h.all("git-conflict-marker-entries").children[0].click();
 assert.match(h.all("git-conflict-marker-text").textContent,/private token/);
 await h.all("close").click();
 assert.equal(h.all("git-conflict-read").disabled,true);
 assert.equal(h.all("git-conflict-marker-entries").children.length,0);
 assert.doesNotMatch(h.all("git-conflict-marker-text").textContent,/private token/);
 assert.equal(h.calls[2].path,"/api/close");
});


test("Native Console history picker selects only previously allowed commands without Agent call",async()=>{
 const h=nativeHarness([]);
 for(const command of ["pwd","cd src","pwd"]){
  h.all("console-command").value=command;
  await h.all("console-run").click();
 }
 assert.equal(h.all("console-history-entries").children.length,3);
 assert.match(h.all("console-history-count").textContent,/3\/3/);
 h.all("console-command").value="help";
 await h.all("console-history-entries").children[1].click();
 assert.equal(h.all("console-command").value,"cd src");
 assert.equal(h.calls.length,0,"clicking history never runs E2EE");
 assert.match(h.all("status").textContent,/nhấn Enter/);
});

test("Console history filter searches only RAM and makes old selection buttons stale",async()=>{
 const h=nativeHarness([]);
 for(const command of ["pwd","cd src","help"]){
  h.all("console-command").value=command;
  await h.all("console-run").click();
 }
 const stale=h.all("console-history-entries").children[1];
 h.all("console-history-filter").value="PWD";
 h.all("console-history-filter").listeners.get("input")();
 assert.equal(h.all("console-history-entries").children.length,1);
 assert.match(h.all("console-history-count").textContent,/1\/3/);
 h.all("console-command").value="help";
 await stale.click();
 assert.equal(h.all("console-command").value,"help");
 assert.match(h.all("status").textContent,/cũ/);
 await h.all("console-history-entries").children[0].click();
 assert.equal(h.all("console-command").value,"pwd");
 assert.equal(h.calls.length,0);
});

test("Console history never includes refused shell commands, capped at 16 safe inputs",async()=>{
 const h=nativeHarness([]);
 h.all("console-command").value="git push --force";
 await h.all("console-run").click();
 assert.equal(h.all("console-history-entries").children.length,0);
 for(let i=0;i<20;i++){
  h.all("console-command").value=i%2?"help":"pwd";
  await h.all("console-run").click();
 }
 assert.equal(h.all("console-history-entries").children.length,16);
 assert.match(h.all("console-history-count").textContent,/16\/16/);
 assert.doesNotMatch(h.all("console-history-entries").children[0].textContent,/push/);
 assert.equal(h.calls.length,0);
});

test("Console history rejects unsafe Unicode search and cannot select historical cards",async()=>{
 const h=nativeHarness([]);
 h.all("console-command").value="pwd";await h.all("console-run").click();
 const old=h.all("console-history-entries").children[0];
 for(const query of ["\u202Eevil","\n","a".repeat(97)," leading "]){
  h.all("console-history-filter").value=query;
  h.all("console-history-filter").listeners.get("input")();
  assert.equal(h.all("console-history-entries").children.length,0);
  assert.equal(h.all("console-history-count").textContent,"Lọc không hợp lệ");
 }
 h.all("console-command").value="help";
 await old.click();
 assert.equal(h.all("console-command").value,"help");
 assert.equal(h.calls.length,0);
});

test("Console clear scrubs command history and transcript including detached history buttons",async()=>{
 const h=nativeHarness([]);
 h.all("console-command").value="pwd";await h.all("console-run").click();
 h.all("console-command").value="help";await h.all("console-run").click();
 const stale=h.all("console-history-entries").children[0];
 h.all("console-history-filter").value="help";
 h.all("console-command").value="clear";
 await h.all("console-run").click();
 assert.equal(h.all("console-history-filter").value,"");
 assert.equal(h.all("console-history-entries").children.length,0);
 assert.match(h.all("console-history-count").textContent,/0\/0/);
 assert.equal(h.all("console-count").textContent,"0 kết quả");
 assert.doesNotMatch(h.all("console-transcript").textContent,/\$ help/);
 h.all("console-command").value="";
 await stale.click();
 assert.equal(h.all("console-command").value,"");
 assert.equal(h.calls.length,0);
});

test("Native session close clears command history controls and rejects stale selection",async()=>{
 const h=nativeHarness([{ok:true,text:"closed"}],{confirm:true});
 h.all("console-command").value="pwd";await h.all("console-run").click();
 const stale=h.all("console-history-entries").children[0];
 assert.match(stale.textContent,/pwd/);
 await h.all("close").click();
 assert.equal(h.all("console-history-entries").children.length,0);
 assert.equal(h.all("console-history-filter").value,"");
 await stale.click();
 assert.equal(h.all("console-command").value,"");
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].path,"/api/close");
});


test("Git conflict OURS draft requires separate fresh E2EE file review before File editor",async()=>{
 const path="src/app.go";
 const original="before\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> topic\nafter\n";
 const confirm="EDIT src/app.go abcdef012345";
 const h=nativeHarness(["UU "+path+"\n",original,{text:original,confirm}]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 await h.all("git-conflict-marker-entries").children[0].click();
 await h.all("git-conflict-draft-ours").click();
 assert.equal(h.calls.length,2,"local draft must not invoke Agent");
 assert.equal(h.all("git-conflict-draft-text").textContent,"before\nours\nafter\n");
 assert.equal(h.all("file-edit-text").value,"");
 await h.all("git-conflict-draft-review").click();
 assert.equal(h.calls.length,3);
 assert.equal(h.calls[2].request.operation,"file_edit_review");
 assert.equal(h.calls[2].request.path,path);
 assert.equal(h.all("file").value,path);
 assert.equal(h.all("file-edit-text").value,"before\nours\nafter\n");
 assert.equal(h.all("file-edit-confirm").value,"","fresh review cannot auto-approve write");
 assert.match(h.all("file-edit-approval").textContent,/Agent vừa xác nhận/);
 assert.match(h.all("file-edit-approval").textContent,/abcdef012345/);
});

test("Git conflict THEIRS draft changes only selected hunk and preserves other unresolved hunk",async()=>{
 const original="one\n<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> feature\n"+
  "middle\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> feature\nlast\n";
 const h=nativeHarness(["UU src/app.go\n",original]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 await h.all("git-conflict-marker-entries").children[1].click();
 await h.all("git-conflict-draft-theirs").click();
 const preview=h.all("git-conflict-draft-text").textContent;
 assert.match(preview,/<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> feature/);
 assert.match(preview,/middle\ntheirs2\nlast/);
 assert.doesNotMatch(preview,/ours2/);
 assert.match(h.all("git-conflict-draft-label").textContent,/chưa giải quyết các khối còn lại/);
 assert.equal(h.calls.length,2);
});

test("Git conflict draft refuses stale file version returned from new host CAS review",async()=>{
 const original="<<<<<<< HEAD\nold\n=======\nnew\n>>>>>>> topic\n";
 const changed="<<<<<<< HEAD\nother\n=======\nnew\n>>>>>>> topic\n";
 const h=nativeHarness(["UU src/app.go\n",original,{text:changed,confirm:"EDIT src/app.go abcdef012345"}]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 await h.all("git-conflict-marker-entries").children[0].click();
 await h.all("git-conflict-draft-ours").click();
 await h.all("git-conflict-draft-review").click();
 assert.equal(h.calls.length,3);
 assert.equal(h.all("git-conflict-draft-review").disabled,true);
 assert.equal(h.all("file-edit-text").value,"");
 assert.equal(h.all("git-conflict-marker-entries").children.length,0);
 assert.match(h.all("status").textContent,/đã thay đổi/);
});

test("Late conflict draft review cannot bind to another Git selected file",async()=>{
 let resolve;const delayed=()=>new Promise(r=>{resolve=r});
 const original="<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> topic\n";
 const h=nativeHarness(["UU src/a.go\nUU src/b.go\n",original,delayed]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 await h.all("git-conflict-marker-entries").children[0].click();
 await h.all("git-conflict-draft-ours").click();
 const inflight=h.all("git-conflict-draft-review").click();
 assert.equal(typeof resolve,"function");
 h.all("git-path").value="src/b.go";
 resolve({text:original,confirm:"EDIT src/a.go abcdef012345"});
 await inflight;
 assert.equal(h.all("file-edit-text").value,"");
 assert.equal(h.all("git-conflict-draft-review").disabled,true);
 assert.match(h.all("status").textContent,/không được dùng bản nháp/);
 assert.equal(h.calls.length,3);
});

test("Conflict draft human confirmation must be exact and remains one-use",async()=>{
 const original="<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> topic\n";
 const confirm="EDIT src/app.go abcdef012345";
 const h=nativeHarness(["UU src/app.go\n",original,{text:original,confirm},"saved"]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 await h.all("git-conflict-marker-entries").children[0].click();
 await h.all("git-conflict-draft-theirs").click();
 await h.all("git-conflict-draft-review").click();
 assert.equal(h.calls.length,3);
 h.all("file-edit-confirm").value="incorrect";
 await h.all("execute-file-edit").click();
 assert.equal(h.calls.length,3,"invalid confirmation never writes");
 await h.all("execute-file-edit").click();
 assert.equal(h.calls.length,3,"one-use review cannot be replayed");
 assert.match(h.all("status").textContent,/Đã hủy/);
});

test("Conflict draft accepted write delegates to existing host CAS and invalidates GUI snapshot",async()=>{
 const original="<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> topic\n";
 const confirm="EDIT src/app.go abcdef012345";
 const h=nativeHarness(["UU src/app.go\n",original,{text:original,confirm},"saved"]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 await h.all("git-conflict-marker-entries").children[0].click();
 await h.all("git-conflict-draft-ours").click();
 await h.all("git-conflict-draft-review").click();
 h.all("file-edit-confirm").value=confirm;
 await h.all("execute-file-edit").click();
 assert.equal(h.calls.length,4);
 assert.equal(h.calls[3].request.operation,"file_edit_execute");
 assert.equal(h.calls[3].request.path,"src/app.go");
 assert.equal(h.calls[3].request.content,"ours\n");
 assert.equal(h.calls[3].request.confirmation,confirm);
 assert.equal(h.all("git-conflict-marker-entries").children.length,0);
 assert.equal(h.all("git-conflict-read").disabled,true);
 assert.equal(h.all("git-status-count").textContent,"Cần tải lại");
});

test("Git conflict draft does not start host review unless 2 E2EE rounds remain",async()=>{
 const original="<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> topic\n";
 const h=nativeHarness(["UU src/app.go\n",original,"log1","log2","log3"]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 await h.all("git-conflict-marker-entries").children[0].click();
 await h.all("git-conflict-draft-ours").click();
 for(let i=0;i<3;i++)await h.actions.get("git_log").click();
 assert.equal(h.calls.length,5);
 await h.all("git-conflict-draft-review").click();
 assert.equal(h.calls.length,5);
 assert.match(h.all("status").textContent,/Không đủ 2 lượt E2EE/);
});

test("Native session close scrubs conflict draft and disables draft-to-editor buttons",async()=>{
 const original="<<<<<<< HEAD\nprivate credential\n=======\nreplacement\n>>>>>>> topic\n";
 const h=nativeHarness(["UU src/app.go\n",original,{ok:true,text:"closed"}],{confirm:true});
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 await h.all("git-conflict-marker-entries").children[0].click();
 await h.all("git-conflict-draft-ours").click();
 assert.match(h.all("git-conflict-draft-text").textContent,/private credential/);
 await h.all("close").click();
 assert.doesNotMatch(h.all("git-conflict-draft-text").textContent,/private credential/);
 assert.equal(h.all("git-conflict-draft-ours").disabled,true);
 assert.equal(h.all("git-conflict-draft-theirs").disabled,true);
 assert.equal(h.all("git-conflict-draft-review").disabled,true);
 assert.equal(h.calls.length,3);
 assert.equal(h.calls[2].path,"/api/close");
});


test("Changing conflict draft revokes previous one-use File editor review",async()=>{
 const original="<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> topic\n";
 const confirm="EDIT src/app.go abcdef012345";
 const h=nativeHarness(["UU src/app.go\n",original,{text:original,confirm}]);
 await h.actions.get("git_status").click();
 await h.all("git-status-files").children[0].click();
 await h.all("git-conflict-read").click();
 await h.all("git-conflict-marker-entries").children[0].click();
 await h.all("git-conflict-draft-ours").click();
 await h.all("git-conflict-draft-review").click();
 assert.equal(h.calls.length,3);
 await h.all("git-conflict-draft-theirs").click();
 h.all("file-edit-confirm").value=confirm;
 await h.all("execute-file-edit").click();
 assert.equal(h.calls.length,3,"review for OURS cannot authorize THEIRS");
 assert.match(h.all("git-conflict-draft-text").textContent,/theirs/);
});


test("Console managed task output attaches only with a valid host-managed handle",async()=>{
 const handle="a".repeat(32);
 const reply="TASK_HANDLE "+handle+"\nOUTPUT_OFFSET 0\nOUTPUT_NEXT 4\n"+
  "OUTPUT_TOTAL 8\nTASK_STATE running\nOUTPUT_EOF no\n-- TASK OUTPUT WINDOW --\nabcd";
 const h=nativeHarness([reply]);
 h.all("task-handle").value="not-a-handle";
 await h.all("console-task-attach").click();
 assert.equal(h.calls.length,0);
 assert.match(h.all("status").textContent,/Handle không hợp lệ/);
 h.all("task-handle").value=handle;
 await h.all("console-task-attach").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"task_output");
 assert.equal(h.calls[0].request.handle,handle);
 assert.equal(h.calls[0].request.offset,0);
 assert.equal(h.all("console-task-window").textContent,"abcd");
 assert.match(h.all("console-task-progress").textContent,/4 \/ 8/);
 assert.equal(h.all("console-task-next").disabled,false);
});

test("Console managed task next uses verified cursor and doesn't poll automatically",async()=>{
 const handle="b".repeat(32);
 const first="TASK_HANDLE "+handle+"\nOUTPUT_OFFSET 0\nOUTPUT_NEXT 4\n"+
  "OUTPUT_TOTAL 8\nTASK_STATE running\nOUTPUT_EOF no\n-- TASK OUTPUT WINDOW --\nabcd";
 const second="TASK_HANDLE "+handle+"\nOUTPUT_OFFSET 4\nOUTPUT_NEXT 8\n"+
  "OUTPUT_TOTAL 8\nTASK_STATE exited\nOUTPUT_EOF yes\n-- TASK OUTPUT WINDOW --\nefgh";
 const h=nativeHarness([first,second]);
 h.all("task-handle").value=handle;
 await h.all("console-task-attach").click();
 assert.equal(h.calls.length,1);
 await h.all("console-task-next").click();
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].request.operation,"task_output");
 assert.equal(h.calls[1].request.offset,4);
 assert.equal(h.all("console-task-window").textContent,"abcdefgh");
 assert.equal(h.all("console-task-next").disabled,true);
 await h.all("console-task-next").click();
 assert.equal(h.calls.length,2,"EOF cannot poll host again");
});

test("Console managed task refuses hand-edited cursor without any E2EE request",async()=>{
 const handle="c".repeat(32);
 const first="TASK_HANDLE "+handle+"\nOUTPUT_OFFSET 0\nOUTPUT_NEXT 4\n"+
  "OUTPUT_TOTAL 9\nTASK_STATE running\nOUTPUT_EOF no\n-- TASK OUTPUT WINDOW --\nabcd";
 const h=nativeHarness([first]);
 h.all("task-handle").value=handle;
 await h.all("console-task-attach").click();
 h.all("task-offset").value="999";
 await h.all("console-task-next").click();
 assert.equal(h.calls.length,1);
 assert.match(h.all("status").textContent,/Cursor task không còn hợp lệ/);
});

test("Console managed task rejects forged or incomplete output windows atomically",async()=>{
 const handle="d".repeat(32);
 const invalid="TASK_HANDLE "+handle+"\nOUTPUT_OFFSET 0\nOUTPUT_NEXT 5\n"+
  "OUTPUT_TOTAL 5\nTASK_STATE exited\nOUTPUT_EOF yes\n-- TASK OUTPUT WINDOW --\nabcd";
 const h=nativeHarness([invalid]);
 h.all("task-handle").value=handle;
 await h.all("console-task-attach").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.all("console-task-next").disabled,true);
 assert.doesNotMatch(h.all("console-task-window").textContent,/abcd/);
 assert.match(h.all("status").textContent,/Từ chối trang output/);
});

test("Console task output ignores late response after task handle changes",async()=>{
 const handle="e".repeat(32);
 let resolve;const delayed=()=>new Promise(r=>{resolve=r});
 const reply="TASK_HANDLE "+handle+"\nOUTPUT_OFFSET 0\nOUTPUT_NEXT 4\n"+
  "OUTPUT_TOTAL 4\nTASK_STATE exited\nOUTPUT_EOF yes\n-- TASK OUTPUT WINDOW --\nabcd";
 const h=nativeHarness([delayed]);
 h.all("task-handle").value=handle;
 const pending=h.all("console-task-attach").click();
 assert.equal(typeof resolve,"function");
 h.all("task-handle").value="f".repeat(32);
 h.all("task-handle").listeners.get("input")();
 resolve(reply);
 await pending;
 assert.equal(h.calls.length,1);
 assert.equal(h.all("console-task-next").disabled,true);
 assert.doesNotMatch(h.all("console-task-window").textContent,/abcd/);
 assert.match(h.all("status").textContent,/Output hết hiệu lực/);
});

test("Existing Tasks output read mirrors one authenticated window into Console without extra call",async()=>{
 const handle="1".repeat(32);
 const reply="TASK_HANDLE "+handle+"\nOUTPUT_OFFSET 0\nOUTPUT_NEXT 4\n"+
  "OUTPUT_TOTAL 4\nTASK_STATE exited\nOUTPUT_EOF yes\n-- TASK OUTPUT WINDOW --\ntext";
 const h=nativeHarness([reply]);
 h.all("task-handle").value=handle;
 h.all("task-offset").value="0";
 await h.actions.get("task_output").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.all("console-task-window").textContent,"text");
 assert.equal(h.all("console-task-next").disabled,true);
});

test("Closing trusted Console clears managed task output and locks task output navigation",async()=>{
 const handle="2".repeat(32);
 const reply="TASK_HANDLE "+handle+"\nOUTPUT_OFFSET 0\nOUTPUT_NEXT 6\n"+
  "OUTPUT_TOTAL 6\nTASK_STATE exited\nOUTPUT_EOF yes\n-- TASK OUTPUT WINDOW --\nsecret";
 const h=nativeHarness([reply,{ok:true,text:"closed"}],{confirm:true});
 h.all("task-handle").value=handle;
 await h.all("console-task-attach").click();
 assert.match(h.all("console-task-window").textContent,/secret/);
 await h.all("close").click();
 assert.doesNotMatch(h.all("console-task-window").textContent,/secret/);
 assert.equal(h.all("console-task-next").disabled,true);
 assert.equal(h.all("console-task-attach").disabled,true);
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].path,"/api/close");
});


test("Queue Stage review requires authenticated two-way diff and separate host confirmation",async()=>{
 const path="src/app.go",review="-- WORKTREE / UNSTAGED --\n+modified\n-- INDEX / STAGED --\n";
 const confirm="STAGE src/app.go abcdef012345";
 const h=nativeHarness([" M "+path+"\n",review,{text:"Host SHA-CAS preview",confirm},"Stage applied"]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 assert.equal(h.all("git-queue-review-stage").disabled,true,"not reviewed");
 await h.all("git-queue-review-stage").click();
 assert.equal(h.calls.length,1,"disabled queue cannot cause remote action");
 await h.actions.get("git_review").click();
 assert.equal(h.all("git-queue-review-stage").disabled,false);
 assert.equal(h.all("git-queue-review-unstage").disabled,true);
 await h.all("git-queue-review-stage").click();
 assert.equal(h.calls.length,3);
 assert.equal(h.calls[2].request.operation,"git_write_review");
 assert.equal(h.calls[2].request.mode,"stage");
 assert.equal(h.calls[2].request.path,path);
 assert.equal(h.calls.filter(c=>c.request.operation==="git_write_execute").length,0);
 assert.match(h.all("git-confirm-text").textContent,/STAGE src\/app.go/);
 assert.equal(h.all("git-confirm-entry").value,"");
 h.all("git-confirm-entry").value=confirm;
 await h.all("execute-git").click();
 assert.equal(h.calls.length,4);
 assert.equal(h.calls[3].request.operation,"git_write_execute");
 assert.equal(h.calls[3].request.mode,"stage");
 assert.equal(h.calls[3].request.path,path);
 assert.equal(h.all("git-queue-review-stage").disabled,true,"host write invalidates queue");
 assert.match(h.all("git-status-count").textContent,/Cần tải lại/);
});

test("Queue Unstage review is independently authenticated and does not execute Git",async()=>{
 const path="src/staged.go",review="-- WORKTREE / UNSTAGED --\n\n-- INDEX / STAGED --\n+staged";
 const h=nativeHarness(["M  "+path+"\n",review,{text:"Index review",confirm:"UNSTAGE src/staged.go abcdef012345"}]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 await h.actions.get("git_review").click();
 assert.equal(h.all("git-queue-review-stage").disabled,true);
 assert.equal(h.all("git-queue-review-unstage").disabled,false);
 await h.all("git-queue-review-unstage").click();
 assert.equal(h.calls.length,3);
 assert.equal(h.calls[2].request.operation,"git_write_review");
 assert.equal(h.calls[2].request.mode,"unstage");
 assert.equal(h.calls[2].request.path,path);
 assert.equal(h.calls.filter(c=>c.request.operation==="git_write_execute").length,0);
 assert.equal(h.all("git-confirm-entry").value,"");
});

test("Queue write review never enables on conflict or untracked paths",async()=>{
 const review="-- WORKTREE / UNSTAGED --\n+new\n-- INDEX / STAGED --\n+old";
 const h=nativeHarness(["UU src/conflict.go\n?? src/untracked.go\n",review,review]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 await h.actions.get("git_review").click();
 assert.equal(h.all("git-queue-review-stage").disabled,true);
 assert.equal(h.all("git-queue-review-unstage").disabled,true);
 await h.all("git-queue-next").click();
 await h.actions.get("git_review").click();
 assert.equal(h.all("git-queue-review-stage").disabled,true);
 assert.equal(h.all("git-queue-review-unstage").disabled,true);
 assert.equal(h.calls.length,3);
});

test("Queue write review cannot use a different manually edited Git path",async()=>{
 const review="-- WORKTREE / UNSTAGED --\n+change\n-- INDEX / STAGED --\n";
 const h=nativeHarness([" M src/a.go\n M src/b.go\n",review]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 await h.actions.get("git_review").click();
 assert.equal(h.all("git-queue-review-stage").disabled,false);
 h.all("git-path").value="src/b.go";
 await h.all("git-queue-review-stage").click();
 assert.equal(h.calls.length,2,"not allowed to request review for unexpected path");
});

test("Queue Stage review rejects late authenticated reply after selected target changes",async()=>{
 let resolve;const delayed=()=>new Promise(r=>{resolve=r});
 const review="-- WORKTREE / UNSTAGED --\n+change\n-- INDEX / STAGED --\n";
 const h=nativeHarness([" M src/a.go\n M src/b.go\n",review,delayed]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 await h.actions.get("git_review").click();
 const pending=h.all("git-queue-review-stage").click();
 assert.equal(typeof resolve,"function");
 h.all("git-path").value="src/b.go";
 resolve({text:"review",confirm:"STAGE src/a.go abcdef012345"});
 await pending;
 assert.equal(h.calls.length,3);
 assert.match(h.all("status").textContent,/Review cũ/);
 assert.equal(h.all("git-confirm-entry").value,"");
 await h.all("execute-git").click();
 assert.equal(h.calls.length,3,"late review never produces executable Git approval");
});

test("Queue Stage review requires enough E2EE rounds for review plus potential execute",async()=>{
 const review="-- WORKTREE / UNSTAGED --\n+change\n-- INDEX / STAGED --\n";
 const h=nativeHarness([" M src/a.go\n",review,"first","second"]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 await h.actions.get("git_review").click();
 assert.equal(h.all("git-queue-review-stage").disabled,false);
 await h.actions.get("git_log").click();
 assert.equal(h.all("git-queue-review-stage").disabled,false,"4/6 consumed still permits two-round review");
 await h.actions.get("git_log").click();
 assert.equal(h.all("git-queue-review-stage").disabled,true,"5/6 consumed cannot reserve two rounds");
 await h.all("git-queue-review-stage").click();
 assert.equal(h.calls.length,4);
});

test("Queue write review cannot survive Git status refresh and native close",async()=>{
 const review="-- WORKTREE / UNSTAGED --\n+change\n-- INDEX / STAGED --\n";
 const h=nativeHarness([" M src/a.go\n",review," M src/a.go\n",{ok:true,text:"closed"}],{confirm:true});
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 await h.actions.get("git_review").click();
 assert.equal(h.all("git-queue-review-stage").disabled,false);
 await h.actions.get("git_status").click();
 assert.equal(h.all("git-queue-review-stage").disabled,true);
 assert.equal(h.all("git-queue-review-unstage").disabled,true);
 await h.all("close").click();
 assert.equal(h.all("git-queue-review-stage").disabled,true);
 assert.equal(h.all("git-queue-review-unstage").disabled,true);
 assert.equal(h.calls.length,4);
 assert.equal(h.calls[3].path,"/api/close");
});

test("Queue write review denies malformed Git two-way preview even with matching file",async()=>{
 const h=nativeHarness([" M src/a.go\n","corrupt -- INDEX / STAGED --"]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 await h.actions.get("git_review").click();
 assert.equal(h.all("git-queue-review-stage").disabled,true);
 await h.all("git-queue-review-stage").click();
 assert.equal(h.calls.length,2);
});


test("Queue Git Stage review button disables on typed path changes and rejects whitespace aliases",async()=>{
 const review="-- WORKTREE / UNSTAGED --\n+change\n-- INDEX / STAGED --\n";
 const h=nativeHarness([" M src/a.go\n",review]);
 await h.actions.get("git_status").click();
 await h.all("git-queue-add-visible").click();
 await h.actions.get("git_review").click();
 assert.equal(h.all("git-queue-review-stage").disabled,false);
 h.all("git-path").value=" src/a.go ";
 h.all("git-path").listeners.get("input")();
 assert.equal(h.all("git-queue-review-stage").disabled,true);
 await h.all("git-queue-review-stage").click();
 assert.equal(h.calls.length,2,"whitespace alias must never trigger host Git review");
 h.all("git-path").value="src/a.go";
 h.all("git-path").listeners.get("input")();
 assert.equal(h.all("git-queue-review-stage").disabled,false);
 assert.equal(h.calls.length,2,"restoring selection never auto-reads or stages");
});


test("Git remotes GUI reads names only from E2EE and never requests URLs or Git network",async()=>{
 const h=nativeHarness(["GIT_REMOTES_V1\norigin\nupstream\n"]);
 await h.actions.get("git_remotes").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_remotes");
 assert.equal(h.calls[0].request.path,"");
 assert.equal(h.all("git-remotes-entries").children.length,2);
 assert.match(h.all("git-remotes-entries").children[0].textContent,/origin/);
 assert.match(h.all("git-remotes-count").textContent,/2 remote/);
 assert.doesNotMatch(h.all("git-remotes-entries").children[0].textContent,/https|token|password/);
 assert.equal(h.calls.filter(x=>/fetch|pull|push|ls-remote/.test(x.request.operation)).length,0);
});

test("Git remotes GUI accepts authenticated empty remote-name envelope",async()=>{
 const h=nativeHarness(["GIT_REMOTES_V1\n"]);
 await h.actions.get("git_remotes").click();
 assert.equal(h.all("git-remotes-entries").children.length,0);
 assert.match(h.all("git-remotes-entries").textContent,/Chưa cấu hình/);
 assert.equal(h.all("git-remotes-count").textContent,"0 remote");
});

test("Git remotes GUI refuses credential URLs, bidi, duplicates and forged envelopes",async()=>{
 const invalid=[
  "origin\n",
  "GIT_REMOTES_V1\norigin https://user:secret@example.invalid/repo.git\n",
  "GIT_REMOTES_V1\norigin\norigin\n",
  "GIT_REMOTES_V1\n\u202Eevil\n",
  "GIT_REMOTES_V1\n<script>alert(1)</script>\n",
  "GIT_REMOTES_V1\norigin",
  "GIT_REMOTES_V1\n"+Array.from({length:17},(_,i)=>"remote"+i+"\n").join("")
 ];
 for(const candidate of invalid){
  const h=nativeHarness([candidate]);
  await h.actions.get("git_remotes").click();
  assert.equal(h.all("git-remotes-count").textContent,"Không hợp lệ");
  assert.equal(h.all("git-remotes-entries").children.length,0);
  assert.equal(h.calls.length,1);
 }
});

test("Git remotes Console command uses exactly one typed E2EE read and mirrors safe GUI names",async()=>{
 const h=nativeHarness(["GIT_REMOTES_V1\norigin\n"]);
 h.all("console-command").value="git remotes";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_remotes");
 assert.equal(h.calls[0].request.path,"");
 assert.equal(h.all("git-remotes-entries").children.length,1);
 assert.match(h.all("git-remotes-count").textContent,/1 remote/);
 await h.all("git-remotes-entries").children[0].click();
 assert.equal(h.calls.length,1,"read-only remote-name cards cannot invoke Git actions");
});

test("Failed new Git remotes E2EE read must invalidate previously shown names",async()=>{
 const h=nativeHarness(["GIT_REMOTES_V1\norigin\n",{ok:false,error:"denied"}]);
 await h.actions.get("git_remotes").click();
 assert.equal(h.all("git-remotes-entries").children.length,1);
 await h.actions.get("git_remotes").click();
 assert.equal(h.all("git-remotes-entries").children.length,0);
 assert.equal(h.all("git-remotes-count").textContent,"Không tải được");
 assert.doesNotMatch(h.all("git-remotes-entries").textContent,/origin/);
});

test("Native close scrubs authenticated Git remote-name metadata",async()=>{
 const h=nativeHarness(["GIT_REMOTES_V1\nprivate-backup\n",{ok:true,text:"closed"}],{confirm:true});
 await h.actions.get("git_remotes").click();
 assert.match(h.all("git-remotes-entries").children[0].textContent,/private-backup/);
 await h.all("close").click();
 assert.doesNotMatch(h.all("git-remotes-entries").textContent,/private-backup/);
 assert.equal(h.all("git-remotes-entries").children.length,0);
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].path,"/api/close");
});


test("Git tracking refs shows authenticated local-only branch names and abbreviated SHA",async()=>{
 const oid="a".repeat(40);
 const h=nativeHarness(["GIT_TRACKING_V1\norigin/feature/test "+oid+"\norigin/main "+oid+"\n"]);
 await h.actions.get("git_tracking_refs").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_tracking_refs");
 assert.equal(h.calls[0].request.path,"");
 assert.equal(h.all("git-tracking-entries").children.length,2);
 assert.match(h.all("git-tracking-entries").children[0].textContent,/origin\/feature\/test/);
 assert.match(h.all("git-tracking-entries").children[0].textContent,/aaaaaaaaaaaa/);
 assert.match(h.all("git-tracking-count").textContent,/2 refs cục bộ/);
 await h.all("git-tracking-entries").children[0].click();
 assert.equal(h.calls.length,1,"tracking ref cards cannot execute Git");
});
test("Git tracking refs empty envelope does not invent branches or remote freshness",async()=>{
 const h=nativeHarness(["GIT_TRACKING_V1\n"]);
 await h.actions.get("git_tracking_refs").click();
 assert.equal(h.all("git-tracking-entries").children.length,0);
 assert.match(h.all("git-tracking-entries").textContent,/Chưa có remote-tracking/);
 assert.match(h.all("git-tracking-count").textContent,/0 refs/);
 assert.equal(h.calls.length,1);
});
test("Git tracking refs reject malformed names, missing OIDs and malicious HTML",async()=>{
 const oid="a".repeat(40);
 const invalid=[
  "origin/main "+oid+"\n",
  "GIT_TRACKING_V1\norigin/main\n",
  "GIT_TRACKING_V1\norigin/main "+oid+"\norigin/main "+oid+"\n",
  "GIT_TRACKING_V1\norigin/main "+oid,
  "GIT_TRACKING_V1\norigin/../main "+oid+"\n",
  "GIT_TRACKING_V1\norigin/HEAD "+oid+"\n",
  "GIT_TRACKING_V1\norigin/<img onerror=evil> "+oid+"\n",
  "GIT_TRACKING_V1\norigin/main "+oid.toUpperCase()+"\n",
  "GIT_TRACKING_V1\n"+Array.from({length:33},(_,i)=>"origin/b"+i+" "+oid+"\n").join("")
 ];
 for(const candidate of invalid){
  const h=nativeHarness([candidate]);
  await h.actions.get("git_tracking_refs").click();
  assert.equal(h.all("git-tracking-count").textContent,"Không hợp lệ");
  assert.equal(h.all("git-tracking-entries").children.length,0);
  assert.equal(h.calls.length,1);
 }
});
test("Git tracking refs Console alias mirrors E2EE result without second request",async()=>{
 const h=nativeHarness(["GIT_TRACKING_V1\norigin/main "+"f".repeat(40)+"\n"]);
 h.all("console-command").value="git tracking-refs";
 await h.all("console-run").click();
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0].request.operation,"git_tracking_refs");
 assert.equal(h.all("git-tracking-entries").children.length,1);
 assert.match(h.all("git-tracking-entries").children[0].textContent,/local snapshot/);
});
test("Git tracking ref failed reread clears old authenticated cards",async()=>{
 const h=nativeHarness(["GIT_TRACKING_V1\norigin/main "+"a".repeat(40)+"\n",{ok:false,error:"Denied"}]);
 await h.actions.get("git_tracking_refs").click();
 assert.equal(h.all("git-tracking-entries").children.length,1);
 await h.actions.get("git_tracking_refs").click();
 assert.equal(h.all("git-tracking-entries").children.length,0);
 assert.equal(h.all("git-tracking-count").textContent,"Không tải được");
 assert.doesNotMatch(h.all("git-tracking-entries").textContent,/origin\/main/);
});
test("Git tracking refs cleared by native close and cannot trigger fetch",async()=>{
 const h=nativeHarness(["GIT_TRACKING_V1\nbackup/release "+"f".repeat(40)+"\n",{ok:true,text:"closed"}],{confirm:true});
 await h.actions.get("git_tracking_refs").click();
 assert.match(h.all("git-tracking-entries").children[0].textContent,/backup\/release/);
 await h.all("close").click();
 assert.equal(h.all("git-tracking-entries").children.length,0);
 assert.doesNotMatch(h.all("git-tracking-entries").textContent,/backup\/release/);
 assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].path,"/api/close");
});
