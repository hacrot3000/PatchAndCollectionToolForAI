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
 addEventListener(event,handler){this.listeners.set(event,handler);}
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
  "git_branches","git_diff","git_staged","git_head","git_review","git_write_review","tasks",
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
 const h=nativeHarness(["a","b","c","d","e"]);
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
