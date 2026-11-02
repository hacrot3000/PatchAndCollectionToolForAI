/* Bundled with trusted native executable, never fetched from PHP Portal. */
"use strict";
(() => {
 const key=decodeURIComponent(location.hash.slice(1));
 history.replaceState(null,"",location.pathname);
 const $=(id)=>document.getElementById(id);
 const titles={files:["Files","Duyệt workspace và xem văn bản qua Agent E2EE."],git:["Git","Git metadata và diff qua quyền host."],tasks:["Tasks","Chạy task phê duyệt và theo dõi có kiểm soát."]};
 let used=0,max=6,busy=false,review=null,gitReview=null,fileReview=null,lineReview=null;
 function notify(status,text){$("status").textContent=status;$("output").textContent=text;}
 function safeWorkspacePath(path){
  return path==="."||(typeof path==="string"&&path.length<=120&&
   path.split("/").every(x=>x&&x!=="."&&x!==".."&&!x.startsWith(".")&&
    !/[\u0000-\u001f\u007f-\u009f\\:]/u.test(x)));
 }
 function renderExplorer(directory,listing){
  const target=$("workspace-entries");
  target.replaceChildren();
  $("explorer-location").textContent=directory;
  if(!safeWorkspacePath(directory)||typeof listing!=="string"||
   new TextEncoder().encode(listing).length>192){
   target.textContent="Danh sách không hợp lệ hoặc thư mục chưa chuẩn hóa. Nhập đường dẫn tương đối, rồi duyệt lại.";
   return;
  }
  if(!listing){
   target.textContent="Thư mục trống hoặc không có tệp được phép hiển thị.";return;
  }
  const rows=listing.split("\n");
  const seen=new Set();
  if(rows.length>64){
   target.textContent="Danh sách vượt giới hạn Agent.";return;
  }
  const entries=[];
  for(const row of rows){
   const match=/^([DF])\t([^\t\r\n]+)$/u.exec(row);
   if(!match){entries.length=0;break;}
   const kind=match[1],name=match[2];
   const path=directory==="."?name:directory+"/"+name;
   if(!name||name.trim()!==name||name.startsWith(".")||name===".."||
    new TextEncoder().encode(name).length>80||
    /[\/\\\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(name)||
    !safeWorkspacePath(path)||seen.has(name)){
    entries.length=0;break;
   }
   seen.add(name);entries.push({kind,name,path});
  }
  if(entries.length!==rows.length){
   target.textContent="Danh sách Agent không nguyên vẹn; từ chối tạo liên kết.";
   return;
  }
  for(const item of entries){
   const button=document.createElement("button");
   button.type="button";button.dataset.kind=item.kind;
   button.textContent=(item.kind==="D"?"▸ ":"▤ ")+item.name;
   button.title=item.kind==="D"?"Chọn thư mục; nhấn Duyệt để xác thực tiếp":"Chọn tệp cho Files và Git";
   button.addEventListener("click",()=>{
    if(busy)return;
    if($("directory").value.trim()!==directory){
     notify("Danh sách cũ","Đường dẫn thư mục đã thay đổi; nhấn Duyệt để lấy danh sách mới.");return;
    }
    if(item.kind==="D"){
     $("directory").value=item.path;
     $("explorer-location").textContent="Đã chọn "+item.path+" · cần Duyệt";
     target.textContent="Nhấn Duyệt thư mục để xác thực danh sách thư mục con.";
    }else{
     $("file").value=item.path;
     $("git-path").value=item.path;
     $("line").value="1";
     $("status").textContent="Đã chọn file · nhấn Preview, Git Diff hoặc Review";
    }
   });
   target.append(button);
  }
 }
 $("folder-up").addEventListener("click",()=>{
  if(busy)return;
  const current=$("directory").value.trim();
  if(!safeWorkspacePath(current)){$("directory").value=".";return;}
  const at=current.lastIndexOf("/");
  $("directory").value=current==="."?".":at<0?".":current.slice(0,at);
  $("explorer-location").textContent="Đã chọn thư mục cha · cần Duyệt";
  $("workspace-entries").textContent="Nhấn Duyệt thư mục để xác thực danh sách mới.";
 });

 function renderTaskCatalog(report){
  const target=$("task-catalog");
  target.replaceChildren();
  $("task-catalog-count").textContent="Đang kiểm tra";
  if(typeof report!=="string"||new TextEncoder().encode(report).length>4096){
   target.textContent="Danh mục TaskDeck quá lớn hoặc không hợp lệ.";
   $("task-catalog-count").textContent="Không hợp lệ";
   return;
  }
  if(!report){
   target.textContent="Host hiện không phê duyệt task nào.";
   $("task-catalog-count").textContent="0 task";
   return;
  }
  const rows=report.endsWith("\n")?report.slice(0,-1).split("\n"):report.split("\n");
  const seen=new Set(),items=[];
  if(rows.length>64){
   target.textContent="Danh mục vượt giới hạn 64 tác vụ.";
   $("task-catalog-count").textContent="Không hợp lệ";
   return;
  }
  for(const line of rows){
   const parts=/^([1-9][0-9]*)\t([^\t\r\n]+)$/u.exec(line);
   if(!parts){items.length=0;break;}
   const id=Number(parts[1]),name=parts[2];
   if(!Number.isSafeInteger(id)||id>2147483647||
    String(id)!==parts[1]||
    new TextEncoder().encode(name).length>120||
    name.trim()!==name||
    /[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(name)||
    seen.has(id)){
    items.length=0;break;
   }
   seen.add(id);items.push({id,name});
  }
  if(items.length!==rows.length){
   target.textContent="Danh mục TaskDeck không nguyên vẹn. Không thể chọn hoặc chạy tác vụ.";
   $("task-catalog-count").textContent="Không hợp lệ";
   return;
  }
  $("task-catalog-count").textContent=items.length+" task";
  for(const item of items){
   const button=document.createElement("button");
   button.type="button";button.className="task-card";
   button.textContent="#"+item.id+" · "+item.name;
   button.addEventListener("click",()=>{
    if(busy)return;
    $("task-id").value=String(item.id);
    $("status").textContent="Đã chọn Task #"+item.id+" · cần Review";
    review=null;
    $("approval").classList.add("hidden");
   });
   target.append(button);
  }
 }
 function quota(){
  $("quota").textContent=used+" / "+max+" E2EE actions";
  if(used>=max)$("status").textContent="Hết lượt E2EE · đăng nhập lại để tiếp tục";
 }
 function setBusy(next){
  busy=next;
  document.querySelectorAll("[data-op],#execute-task,#cancel-task,#execute-git,#execute-file-edit,#execute-line-edit").forEach(b=>{
   b.disabled=next||used>=max||(["git_write_review","file_edit_review","line_write_review","git_review"].includes(b.dataset.op)&&used+2>max);
  });
 }
 async function call(input,closing=false){
  if(busy||(!closing&&used>=max))return null;
  setBusy(true);$("status").textContent="Đang xác thực Agent…";
  try{
   const response=await fetch(closing?"/api/close":"/api/action",{
    method:"POST",mode:"same-origin",credentials:"omit",cache:"no-store",referrerPolicy:"no-referrer",
    headers:{"Content-Type":"application/json","X-TaskDeck-Local-Key":key},
    body:JSON.stringify(closing?{}:{operation:"",path:"",task_id:0,mode:"",confirmation:"",handle:"",offset:0,...input})
   });
   const data=await response.json();
   if(data.used!==undefined){used=data.used;max=data.max;}
   if(!response.ok||!data.ok){notify("Bị từ chối",data.error||"Không có quyền thực thi.");quota();return null;}
   if(!closing)notify("Agent đã xác thực",data.text||"Hoàn tất");
   quota();return data;
  }catch(_){
   notify("Mất kết nối","Không thử lại thao tác ghi. Đóng phiên và đăng nhập lại native client.");
   return null;
  }finally{setBusy(false);}
 }
 document.querySelectorAll("[data-panel]").forEach(btn=>btn.addEventListener("click",()=>{
  if(busy)return;
  const panel=btn.dataset.panel;
  document.querySelectorAll(".nav").forEach(b=>b.classList.toggle("active",b===btn));
  document.querySelectorAll(".panel").forEach(p=>p.classList.toggle("hidden",p.id!=="panel-"+panel));
  $("title").textContent=titles[panel][0];$("subtitle").textContent=titles[panel][1];
 }));
 document.querySelectorAll("[data-op]").forEach(btn=>btn.addEventListener("click",async()=>{
  const operation=btn.dataset.op;const input={operation};
  if(operation==="files_list")input.path=$("directory").value.trim();
  if(operation==="file_view"||operation==="file_lines"||operation==="file_find_lines"){
   input.path=$("file").value.trim();
   if(operation==="file_lines")input.offset=Number($("line").value);
   if(operation==="file_find_lines"){
    const literal=$("find-literal").value;
    // Only a bounded exact literal reaches the trusted local GUI handler.
    // No regexp/shell evaluation and no untrusted HTML interpolation.
    if(!literal||new TextEncoder().encode(literal).length>96||
     literal.trim()!==literal||/[\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]/u.test(literal)){
     notify("Chuỗi tìm không hợp lệ","Nhập literal UTF-8 1–96 byte, không có ký tự điều khiển hoặc định hướng.");
     return;
    }
    input.literal=literal;
   }
  }
  if(["git_diff","git_staged","git_head","git_review"].includes(operation))input.path=$("git-path").value.trim();
  if(operation==="line_write_review"){
   lineReview=null;
   $("line-editor").classList.add("hidden");
   input.path=$("file").value.trim();
   input.mode=btn.dataset.mode;
   input.offset=Number($("line").value);
   if(!Number.isSafeInteger(input.offset)||input.offset<1||input.offset>2097152){
    notify("Số dòng không hợp lệ","Chọn dòng hiện có từ 1 đến 2097152.");return;
   }
  }
  if(operation==="file_edit_review"){
   fileReview=null;
   $("file-editor").classList.add("hidden");
   input.path=$("file").value.trim();
  }
  if(operation==="git_write_review"){
   gitReview=null;$("git-approval").classList.add("hidden");
   input.mode=btn.dataset.mode;
   if(input.mode==="stage"||input.mode==="unstage")input.path=$("git-path").value.trim();
   if(input.mode.startsWith("branch-"))input.path=$("git-branch").value.trim();
  }
  if(operation==="task_review"){
   input.task_id=Number($("task-id").value);input.mode=btn.dataset.mode;
   $("approval").classList.add("hidden");review=null;
  }
  if(operation==="task_status"||operation==="task_output"){
   input.handle=$("task-handle").value.trim();
   if(operation==="task_output")input.offset=Number($("task-offset").value);
  }
  const result=await call(input);
  if(result&&operation==="files_list"){renderExplorer(input.path,result.text||"");}
  if(result&&operation==="tasks"){renderTaskCatalog(result.text||"");}
  if(result&&operation==="line_write_review"&&result.confirm){
   lineReview={path:input.path,mode:input.mode,offset:input.offset,confirm:result.confirm,original:result.line||""};
   $("line-edit-title").textContent="Đã xác thực file và dòng "+input.offset+" · "+input.mode;
   $("line-edit-source").textContent="Nội dung hiện tại: "+JSON.stringify(lineReview.original);
   $("line-edit-text").disabled=input.mode==="delete";
   $("line-edit-text").value=input.mode==="edit"?lineReview.original:"";
   $("line-edit-confirm").value="";
   $("line-edit-approval").textContent="Xem đầy đủ trang file ở khung kết quả rồi nhập: "+result.confirm;
   $("line-editor").classList.remove("hidden");
  }
  if(result&&operation==="file_edit_review"&&result.confirm){
   fileReview={path:input.path,confirm:result.confirm,original:result.text||""};
   $("file-edit-text").value=fileReview.original;
   $("file-edit-confirm").value="";
   $("file-edit-approval").textContent="Đọc và xác nhận bằng chuỗi: "+result.confirm;
   $("file-editor").classList.remove("hidden");
  }
  if(result&&operation==="git_write_review"&&result.confirm){
   gitReview={mode:input.mode,path:input.path||"",confirm:result.confirm};
   $("git-confirm-text").textContent="Đọc đầy đủ preview ở khung kết quả, sau đó nhập chính xác: "+result.confirm;
   $("git-confirm-entry").value="";
   $("git-commit-message").value="";
   $("git-message-label").classList.toggle("hidden",input.mode!=="commit");
   $("git-approval").classList.remove("hidden");
  }
  if(result&&operation==="task_review"&&result.confirm){
   review={id:input.task_id,mode:input.mode,confirm:result.confirm};
   $("approval-text").textContent="Host đã phê duyệt. Nhập chính xác: "+result.confirm;
   $("approval-entry").value="";$("approval").classList.remove("hidden");
  }
  if(result&&operation==="task_output"){
   const match=result.text.match(/^OUTPUT_NEXT ([0-9]+)$/m);
   if(match)$("task-offset").value=match[1];
  }
 }));
 $("execute-task").addEventListener("click",async()=>{
  if(!review)return;
  const pending=review;review=null;$("approval").classList.add("hidden");
  if($("approval-entry").value!==pending.confirm){
   notify("Đã hủy","Chuỗi xác nhận sai. Không thực thi task.");return;
  }
  const result=await call({operation:"task_execute",task_id:pending.id,mode:pending.mode,confirmation:pending.confirm});
  if(result&&pending.mode==="start"){
   const match=result.text.match(/^TASK_HANDLE ([0-9a-f]{32})$/m);
   if(match)$("task-handle").value=match[1];
  }
 });
 $("execute-line-edit").addEventListener("click",async()=>{
  if(!lineReview||busy)return;
  const pending=lineReview;
  lineReview=null;
  $("line-editor").classList.add("hidden");
  const content=pending.mode==="delete"?"":$("line-edit-text").value;
  if($("line-edit-confirm").value!==pending.confirm){
   notify("Đã hủy","Chuỗi xác nhận không chính xác. Không gửi lệnh sửa dòng.");return;
  }
  if(pending.mode==="edit"&&content===pending.original){
   notify("Không thay đổi","Nội dung dòng mới giống hệt dòng đã review.");return;
  }
  if(new TextEncoder().encode(content).length>512||
   /[\u0000-\u0008\u000a-\u001f\u007f-\u009f]/u.test(content)||
   /[\p{Cf}\p{Zl}\p{Zp}]/u.test(content)){
   notify("Dòng không hợp lệ","Chỉ nhập một dòng UTF-8 ≤512 byte; tab được phép.");return;
  }
  await call({operation:"line_write_execute",path:pending.path,mode:pending.mode,
   offset:pending.offset,confirmation:pending.confirm,content});
 });
 $("execute-file-edit").addEventListener("click",async()=>{
  if(!fileReview||busy)return;
  const pending=fileReview;
  fileReview=null;
  $("file-editor").classList.add("hidden");
  const content=$("file-edit-text").value;
  if($("file-edit-confirm").value!==pending.confirm){
   notify("Đã hủy","Không gửi lệnh sửa file: chuỗi xác nhận sai.");return;
  }
  if(content===pending.original){
   notify("Không có thay đổi","File chưa thay đổi; không gửi thao tác ghi.");return;
  }
  if(new TextEncoder().encode(content).length>4096||content.includes("\u0000")){
   notify("Vượt giới hạn","Chỉ cho phép tối đa 4096 byte UTF-8, không có NUL.");return;
  }
  await call({operation:"file_edit_execute",path:pending.path,
   confirmation:pending.confirm,content});
 });
 $("execute-git").addEventListener("click",async()=>{
  if(!gitReview||busy)return;
  const pending=gitReview;
  gitReview=null;$("git-approval").classList.add("hidden");
  if($("git-confirm-entry").value!==pending.confirm){
   notify("Đã hủy","Không gửi lệnh ghi Git: chuỗi xác nhận không khớp.");return;
  }
  const message=pending.mode==="commit"?$("git-commit-message").value:"";
  if(pending.mode==="commit"&&(!/^[\x20-\x7e]{3,96}$/.test(message)||message.trim()!==message)){
   notify("Không hợp lệ","Commit message phải là ASCII in được, 3–96 ký tự, không có khoảng trắng đầu/cuối.");return;
  }
  await call({operation:"git_write_execute",mode:pending.mode,
   path:pending.path,confirmation:pending.confirm,message});
 });
 $("cancel-task").addEventListener("click",async()=>{
  const handle=$("task-handle").value.trim();
  if(!/^[0-9a-f]{32}$/.test(handle)){notify("Lỗi dữ liệu","Handle không hợp lệ.");return;}
  const words="CANCEL TASK "+handle.slice(0,12);
  if(prompt("Để yêu cầu dừng task, nhập: "+words)!==words){
   notify("Đã hủy","Không gửi yêu cầu dừng.");return;
  }
  await call({operation:"task_cancel",handle,confirmation:words});
 });
 $("close").addEventListener("click",async()=>{
  if(confirm("Đóng phiên native E2EE và hủy thao tác đang chạy?")){
   await call({},true);$("close").disabled=true;
   notify("Đã đóng phiên","Đóng tab này. Cần đăng nhập lại để tiếp tục.");
  }
 });
 if(!/^[a-f0-9]{64}$/.test(key)){
  setBusy(true);notify("Không có khóa phiên","Chỉ mở GUI qua native client đã xác thực.");
 }else{
  notify("GUI sẵn sàng","Chọn Files, Git hoặc Tasks. PHP/Relay không cung cấp giao diện này.");
 }
})();
