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

function nativeHarness(responses){
 const elements=new Map(),actions=new Map();
 const all=(id)=>{
  if(!elements.has(id))elements.set(id,new Element(id));
  return elements.get(id);
 };
 const opNames=["files_list","file_view","file_lines","file_find_lines",
  "file_edit_review","line_write_review","git_status","git_log",
  "git_branches","git_diff","git_review","git_write_review","tasks",
  "task_review","task_status","task_output"];
 for(const name of opNames)actions.set(name,new Element("",{op:name}));
 const panels=["files","git","tasks"].map(panel=>new Element("",{panel}));
 const documents=["files","git","tasks"].map(x=>all("panel-"+x));
 all("directory").value=".";
 const controlNames=["execute-task","cancel-task","execute-git",
  "execute-file-edit","execute-line-edit"];
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
  const next=pending.shift();
  if(!next)throw new Error("Unexpected E2EE request");
  return {ok:true,async json(){return {ok:true,...(typeof next==="string"?{text:next}:next),used:++used,max:6};}};
 };
 const context={document,fetch,TextEncoder,decodeURIComponent,
  location:{hash:"#"+("a".repeat(64)),pathname:"/"},
  history:{replaceState(){}},console,
  confirm(){throw new Error("Unexpected confirmation");},
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
