/* Bundled with trusted native executable, never fetched from PHP Portal. */
"use strict";
(() => {
 const key=decodeURIComponent(location.hash.slice(1));
 history.replaceState(null,"",location.pathname);
 const $=(id)=>document.getElementById(id);
 const titles={files:["Files","Duyệt workspace và xem văn bản qua Agent E2EE."],git:["Git","Git metadata và diff qua quyền host."],tasks:["Tasks","Chạy task phê duyệt và theo dõi có kiểm soát."],console:["Console · Read & Review","Lệnh đọc và Git/Task review có xác thực; thao tác ghi/chạy cần xác nhận riêng, không shell."]};
 let used=0,max=6,busy=false,review=null,gitReview=null,fileReview=null,lineReview=null,taskWindow=null,filePage=null,consoleRecords=[],consoleHistory=[],consoleHistoryCursor=0,gitLogSnapshot=[],gitStatusSnapshot=[],gitDiffSnapshot=null,sessionEnding=false,consoleCwd=".",gitReviewQueue=[],gitReviewQueueIndex=-1,gitReviewQueueSource=null,gitStatusRenderGeneration=0,gitReviewQueueLoaded=new Set(),gitStatusAuthenticated=false;
 let gitDiffFindPage=0,gitDiffFindContext=null,gitDiffFindGeneration=0;
 let gitHunkPage=0,gitHunkContext=null,gitHunkGeneration=0;
 let gitConflictPreviewSnapshot=null,gitConflictGeneration=0;
 let consoleHistoryGeneration=0;
 let gitConflictSelectedIndex=-1,gitConflictDraft=null;
 let managedTaskReceipt=null; // local, authenticated, handle-bound; never persisted
 function notify(status,text){$("status").textContent=status;$("output").textContent=text;}
 function clearManagedTaskReceipt(message){
  managedTaskReceipt=null;
  $("task-verify-stop").disabled=true;
  $("task-lifecycle").textContent=message||"Chưa xác minh trạng thái task.";
 }
 function presentManagedTaskReceipt(report,handle,operation){
  if(typeof report!=="string"||new TextEncoder().encode(report).length>16384||
   !/^[0-9a-f]{32}$/u.test(handle)||$("task-handle").value.trim()!==handle){
   clearManagedTaskReceipt("Handle hoặc bản ghi task không hợp lệ. Cần đọc lại từ host.");
   return false;
  }
  const m=/^TASK_HANDLE ([0-9a-f]{32})\nTASK_STATE (running|cancel_requested|exited|stopped)\nTASK_EXIT_CODE (-1|0|[1-9][0-9]{0,2})\n-- TASK OUTPUT --\n/u.exec(report);
  if(!m||m[1]!==handle){
   clearManagedTaskReceipt("Bản ghi trạng thái không khớp task handle đã yêu cầu.");
   return false;
  }
  const state=m[2],exit=Number(m[3]);
  if(exit>255||(["running","cancel_requested"].includes(state)&&exit!==-1)||
   (state==="exited"&&exit<0)||
   (operation==="task_execute"&&state!=="running")||
   (operation==="task_status"&&state==="cancel_requested")||
   (operation==="task_cancel"&&state==="running")){
   clearManagedTaskReceipt("Trạng thái task sai quy ước Agent; không xác nhận đã dừng.");
   return false;
  }
  managedTaskReceipt={handle,state,exit};
  if(state==="cancel_requested"){
   $("task-lifecycle").textContent="Host đã nhận YÊU CẦU HỦY, chưa xác nhận STOPPED. Bấm Xác minh đã dừng để đọc trạng thái qua E2EE.";
  }else if(state==="running"){
   $("task-lifecycle").textContent="Host xác nhận task đang CHẠY tại thời điểm đọc. Chưa có xác nhận dừng.";
  }else{
   $("task-lifecycle").textContent="Host xác nhận task "+(state==="stopped"?"ĐÃ DỪNG":"ĐÃ KẾT THÚC")+
    " · exit code "+String(exit)+". Đây là ảnh chụp E2EE, không tự cập nhật.";
  }
  setBusy(false); // refresh all controls using the authenticated task budget
  return true;
 }
 function presentRunTaskReceipt(report){
  const m=typeof report==="string"?
    /^TASK_STATUS exited\nTASK_EXIT_CODE (0|[1-9][0-9]{0,2})\n-- COMPLETE TASK OUTPUT --\n/u.exec(report):null;
  if(!m||Number(m[1])>255){
   clearManagedTaskReceipt("Kết quả Run Task thiếu trạng thái hoặc exit code hợp lệ.");
   return false;
  }
  // A synchronous run is separate from any existing managed task.
  // Never erase that task's handle or emergency Cancel budget.
  if(!managedTaskReceipt||!["running","cancel_requested"].includes(managedTaskReceipt.state)){
   clearManagedTaskReceipt("Host kết thúc Run Task với exit code "+m[1]+
    ". Không có managed handle; xem bản ghi trong AUTHENTICATED RESULT.");
  }
  return true;
 }
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
     clearGitDiffPreview("Đã chọn file khác");
     clearGitConflictMarkers("Đã chọn file khác");
     $("line").value="1";
     invalidateChangedReviews();
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
    invalidateChangedReviews();
    $("status").textContent="Đã chọn Task #"+item.id+" · cần Review";
    review=null;
    $("approval").classList.add("hidden");
   });
   target.append(button);
  }
 }

 // UI review-target pinning complements host one-use tokens, SHA-CAS,
 // E2EE and authorization. Never silently write to an old visible target.
 function gitTargetMatches(mode,path){
  if(mode==="commit")return true;
  return (mode.startsWith("branch-")?$("git-branch").value.trim():
   $("git-path").value.trim())===path;
 }
 function invalidateChangedReviews(){
  let invalid=false;
  if(fileReview&&$("file").value.trim()!==fileReview.path){
   fileReview=null;$("file-editor").classList.add("hidden");invalid=true;
  }
  if(lineReview&&($("file").value.trim()!==lineReview.path||
   Number($("line").value)!==lineReview.offset)){
   lineReview=null;$("line-editor").classList.add("hidden");invalid=true;
  }
  if(gitReview&&!gitTargetMatches(gitReview.mode,gitReview.path)){
   gitReview=null;$("git-approval").classList.add("hidden");invalid=true;
  }
  if(review&&Number($("task-id").value)!==review.id){
   review=null;$("approval").classList.add("hidden");invalid=true;
  }
  if(invalid)$("status").textContent="Đích thao tác đã đổi · cần Review mới";
 }
 ["file","line","git-path","git-branch","task-id"].forEach(id=>
  $(id).addEventListener("input",invalidateChangedReviews));
 function clearGitDiffPreview(message="Chưa tải"){
  gitDiffSnapshot=null;
  $("git-review-side").value="all";
  $("git-review-side").disabled=true;
  $("git-diff-kind").textContent=message;
  $("git-diff-target").textContent="Chọn file rồi yêu cầu một bản diff mới từ Agent.";
  $("git-diff-text").textContent="Không có bản xem trước diff hiện hành.";
  $("git-diff-find").value="";
  $("git-diff-find-entries").replaceChildren();
  $("git-diff-find-count").textContent="Chưa tìm";
  $("git-diff-selected").textContent="Chưa chọn dòng khớp";
  gitDiffFindPage=0;gitDiffFindContext=null;
  $("git-diff-page").textContent="Trang 0/0";
  $("git-diff-prev").disabled=true;
  $("git-diff-next").disabled=true;
  gitHunkContext=null;gitHunkPage=0;gitHunkGeneration++;
  $("git-hunk-entries").replaceChildren();
  $("git-hunk-count").textContent="Chưa tải";
  $("git-hunk-page").textContent="Trang 0/0";
  $("git-hunk-selected").textContent="Chưa chọn khối thay đổi";
  $("git-hunk-snippet").textContent="Chưa có nội dung hunk đã xác thực.";
  $("git-hunk-prev").disabled=true;$("git-hunk-next").disabled=true;
 }
 function showGitDiffMatches(display,path,operation,mode){
  const target=$("git-diff-find-entries"),snapshot=gitDiffSnapshot;
  const query=$("git-diff-find").value;
  const generation=++gitDiffFindGeneration;
  target.replaceChildren();
  $("git-diff-selected").textContent="Chưa chọn dòng khớp";
  $("git-diff-prev").disabled=true;$("git-diff-next").disabled=true;
  if(!query){
   gitDiffFindPage=0;gitDiffFindContext=null;
   $("git-diff-find-count").textContent="Nhập từ khóa để tìm cục bộ";
   $("git-diff-page").textContent="Trang 0/0";return;
  }
  if(query!==query.trim()||new TextEncoder().encode(query).length>96||
   /[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(query)){
   gitDiffFindPage=0;gitDiffFindContext=null;
   $("git-diff-find-count").textContent="Từ khóa không hợp lệ";
   $("git-diff-page").textContent="Trang 0/0";return;
  }
  if(!gitDiffFindContext||gitDiffFindContext.snapshot!==snapshot||
   gitDiffFindContext.path!==path||gitDiffFindContext.operation!==operation||
   gitDiffFindContext.mode!==mode||gitDiffFindContext.query!==query){
   gitDiffFindPage=0;
   gitDiffFindContext={snapshot,path,operation,mode,query};
  }
  const lines=display.split("\n"),needle=query.toLowerCase(),matches=[];
  for(let i=0;i<lines.length;i++){
   if(lines[i].toLowerCase().includes(needle))matches.push({number:i+1,body:lines[i]});
  }
  const totalPages=Math.ceil(matches.length/32);
  gitDiffFindPage=Math.min(gitDiffFindPage,Math.max(0,totalPages-1));
  const offset=gitDiffFindPage*32;
  $("git-diff-page").textContent="Trang "+(totalPages?gitDiffFindPage+1:0)+"/"+totalPages;
  $("git-diff-prev").disabled=gitDiffFindPage===0;
  $("git-diff-next").disabled=gitDiffFindPage+1>=totalPages;
  $("git-diff-find-count").textContent=matches.length+" dòng khớp trong phần đang xem"+
   (matches.length>32?" · 32 dòng/trang":"")+" · không gửi Agent";
  for(const match of matches.slice(offset,offset+32)){
   const btn=document.createElement("button");
   btn.type="button";btn.className="git-status-card";
   const safe=match.body.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/gu,
    c=>"\\u"+c.codePointAt(0).toString(16).padStart(4,"0"));
   btn.textContent=match.number+": "+(safe.length>160?safe.slice(0,160)+"…":safe);
   const context=gitDiffFindContext;
   btn.addEventListener("click",()=>{
    if(busy||sessionEnding||gitDiffFindGeneration!==generation||gitDiffFindContext!==context||
     !gitDiffSnapshot||gitDiffSnapshot!==snapshot||
     $("git-path").value.trim()!==path||$("git-review-side").value!==mode||
     $("git-diff-find").value!==query){
     $("git-diff-selected").textContent="Lựa chọn cũ; tìm lại trên diff mới.";
     return;
    }
    $("git-diff-selected").textContent="Dòng "+match.number+" (chỉ xem, không stage): "+safe;
   });
   target.append(btn);
  }
 }
 function navigateGitDiffMatches(delta){
  const context=gitDiffFindContext;
  if(busy||sessionEnding||!context||!gitDiffSnapshot||
   context.snapshot!==gitDiffSnapshot||$("git-path").value.trim()!==context.path||
   $("git-review-side").value!==context.mode||$("git-diff-find").value!==context.query||
   (delta<0&&$("git-diff-prev").disabled)||(delta>0&&$("git-diff-next").disabled)){
   return;
  }
  gitDiffFindPage+=delta;
  showGitDiffSnapshot();
 }
 $("git-diff-prev").addEventListener("click",()=>navigateGitDiffMatches(-1));
 $("git-diff-next").addEventListener("click",()=>navigateGitDiffMatches(1));
 $("git-diff-find").addEventListener("input",()=>{
  if(!gitDiffSnapshot||sessionEnding){
   $("git-diff-find-entries").replaceChildren();
   $("git-diff-find-count").textContent="Cần tải diff từ Agent";
   $("git-diff-selected").textContent="Chưa chọn dòng khớp";
   return;
  }
  showGitDiffSnapshot();
 });
 function showGitDiffSnapshot(){
  if(!gitDiffSnapshot||sessionEnding)return;
  const {operation,path,report}=gitDiffSnapshot;
  if($("git-path").value.trim()!==path){
   clearGitDiffPreview("Đường dẫn đã đổi");return;
  }
  const labels={git_diff:"Working-tree diff",git_staged:"Staged diff",
   git_head:"HEAD diff",git_review:"Two-way review"};
  const mode=$("git-review-side").value;
  let display=report,description="Toàn bộ bản xem trước đã xác thực";
  if(operation==="git_review"&&mode!=="all"){
   // The host concatenates two independently authenticated reads with these
   // fixed delimiters. Never silently drop lines if parsing is ambiguous.
   const head="-- WORKTREE / UNSTAGED --\\n".replace("\\n","\n");
   const boundary="\n-- INDEX / STAGED --\n";
   const at=report.indexOf(boundary,head.length);
   if(!report.startsWith(head)||at<0||
    report.indexOf(boundary,at+boundary.length)!==-1||
    report.indexOf(head,head.length)!==-1){
    $("git-review-side").value="all";
    display=report;
    description="Không thể tách diff an toàn; đang hiển thị toàn bộ";
   }else if(mode==="worktree"){
    display=report.slice(head.length,at);
    description="Chỉ hiển thị phần WORKTREE / UNSTAGED (không phải review đầy đủ)";
   }else if(mode==="index"){
    display=report.slice(at+boundary.length);
    description="Chỉ hiển thị phần INDEX / STAGED (không phải review đầy đủ)";
   }else{
    $("git-review-side").value="all";
    description="Bộ lọc không hợp lệ; đang hiển thị toàn bộ";
   }
  }
  const safe=display.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/gu,
   c=>"\\u"+c.codePointAt(0).toString(16).padStart(4,"0"));
  $("git-diff-kind").textContent=labels[operation];
  $("git-diff-target").textContent="File đã yêu cầu: "+path+" · "+description+
   " · kết quả chỉ đọc qua Agent E2EE; bản đầy đủ ở AUTHENTICATED RESULT";
  $("git-diff-text").textContent=safe||"Không có thay đổi trong phần đang hiển thị.";
  showGitDiffMatches(display,path,operation,$("git-review-side").value);
  showGitHunkNavigator(display,path,operation,$("git-review-side").value);
 }
 $("git-review-side").addEventListener("input",showGitDiffSnapshot);
 function showGitHunkNavigator(display,path,operation,mode){
  const snapshot=gitDiffSnapshot,target=$("git-hunk-entries");
  const generation=++gitHunkGeneration;
  target.replaceChildren();
  $("git-hunk-selected").textContent="Chưa chọn khối thay đổi";
  $("git-hunk-snippet").textContent="Chọn một hunk để đọc đoạn diff cục bộ.";
  $("git-hunk-prev").disabled=true;$("git-hunk-next").disabled=true;
  if(!snapshot||sessionEnding||$("git-path").value.trim()!==path)return;
  if(!gitHunkContext||gitHunkContext.snapshot!==snapshot||
   gitHunkContext.path!==path||gitHunkContext.operation!==operation||
   gitHunkContext.mode!==mode){
   gitHunkPage=0;
   gitHunkContext={snapshot,path,operation,mode};
  }
  const lines=display.split("\n"),hunks=[];
  const header=/^@@ -([0-9]{1,7})(?:,([0-9]{1,7}))? \+([0-9]{1,7})(?:,([0-9]{1,7}))? @@(?: [^\r\n]*)?$/u;
  for(let i=0;i<lines.length;i++){
   const match=header.exec(lines[i]);
   if(!match||/[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(lines[i]))continue;
   if(hunks.length){
    const last=hunks[hunks.length-1];
    if(last.end===lines.length)last.end=i;
   }
   hunks.push({start:i,end:lines.length,header:lines[i]});
  }
  // Count all matching hunks rather than silently clipping at 16.
  const pages=Math.ceil(hunks.length/16);
  gitHunkPage=Math.min(gitHunkPage,Math.max(0,pages-1));
  $("git-hunk-count").textContent=hunks.length+" hunk trong diff đang xem · chỉ đọc tại máy";
  $("git-hunk-page").textContent="Trang "+(pages?gitHunkPage+1:0)+"/"+pages;
  $("git-hunk-prev").disabled=gitHunkPage===0;
  $("git-hunk-next").disabled=gitHunkPage+1>=pages;
  if(!hunks.length){
   target.textContent="Không có hunk định dạng unified diff trong kết quả đã xác thực.";
   return;
  }
  for(let j=gitHunkPage*16;j<Math.min(hunks.length,(gitHunkPage+1)*16);j++){
   const hunk=hunks[j],button=document.createElement("button");
   button.type="button";button.className="git-status-card";
   const escaped=hunk.header.replace(/[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/gu,
    c=>"\\u"+c.codePointAt(0).toString(16).padStart(4,"0"));
   button.textContent="Hunk "+(j+1)+"/"+hunks.length+" · "+
    (escaped.length>150?escaped.slice(0,150)+"…":escaped);
   const context=gitHunkContext;
   button.addEventListener("click",()=>{
    if(busy||sessionEnding||gitHunkGeneration!==generation||
     gitHunkContext!==context||gitDiffSnapshot!==snapshot||
     $("git-path").value.trim()!==path||$("git-review-side").value!==mode){
     $("git-hunk-selected").textContent="Hunk cũ; cần chọn lại từ diff đã xác thực.";
     return;
    }
    let end=hunk.end;
    for(let k=hunk.start+1;k<end;k++){
     if(lines[k].startsWith("diff --git ")||
      lines[k]==="-- WORKTREE / UNSTAGED --"||
      lines[k]==="-- INDEX / STAGED --"){end=k;break;}
    }
    const fragment=lines.slice(hunk.start,end).join("\n");
    $("git-hunk-selected").textContent="Hunk "+(j+1)+"/"+hunks.length+" · chỉ xem, không stage hunk";
    $("git-hunk-snippet").textContent=fragment.replace(
     /[\u0000-\u0008\u000b-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/gu,
     c=>"\\u"+c.codePointAt(0).toString(16).padStart(4,"0"));
   });
   target.append(button);
  }
 }
 function navigateGitHunks(delta){
  const ctx=gitHunkContext;
  if(busy||sessionEnding||!ctx||!gitDiffSnapshot||
   ctx.snapshot!==gitDiffSnapshot||$("git-path").value.trim()!==ctx.path||
   $("git-review-side").value!==ctx.mode||
   (delta<0&&$("git-hunk-prev").disabled)||(delta>0&&$("git-hunk-next").disabled))return;
  gitHunkPage+=delta;
  showGitDiffSnapshot();
 }
 $("git-hunk-prev").addEventListener("click",()=>navigateGitHunks(-1));
 $("git-hunk-next").addEventListener("click",()=>navigateGitHunks(1));

 function renderGitDiffPreview(report,operation,path){
  const labels={
   git_diff:"Working-tree diff",
   git_staged:"Staged diff",
   git_head:"HEAD diff",
   git_review:"Two-way review"
  };
  // A two-way review is two separate fully bounded 4 KiB Agent reads
  // plus two fixed section delimiters. Reject anything larger, never cut.
  const maxPreviewBytes=operation==="git_review"?2*4096+64:4096;
  if(!Object.hasOwn(labels,operation)||typeof report!=="string"||
   new TextEncoder().encode(report).length>maxPreviewBytes||
   !safeWorkspacePath(path)||path==="."||$("git-path").value.trim()!==path){
   clearGitDiffPreview("Không hợp lệ");
   $("status").textContent="Từ chối Git diff · kết quả quá lớn hoặc đường dẫn không hợp lệ; không hiển thị một phần";
   return;
  }
  gitDiffSnapshot={report,operation,path};
  // A successful authenticated two-way result is recorded only as "loaded",
  // never as human approval or proof that the host repository stayed unchanged.
  const head="-- WORKTREE / UNSTAGED --\n";
  const separator="\n-- INDEX / STAGED --\n";
  const splitAt=report.indexOf(separator,head.length);
  const completeTwoWay=operation==="git_review"&&report.startsWith(head)&&
   splitAt>=0&&report.indexOf(separator,splitAt+separator.length)<0&&
   report.indexOf(head,head.length)<0;
  if(completeTwoWay&&gitReviewQueueSource===gitStatusSnapshot&&
   gitReviewQueue.includes(path)){
   gitReviewQueueLoaded.add(path);
   showGitReviewQueue();
  }
  $("git-review-side").value="all";
  $("git-review-side").disabled=operation!=="git_review";
  showGitDiffSnapshot();
 }
 $("git-path").addEventListener("input",()=>clearGitDiffPreview("Đường dẫn đã đổi"));
 function invalidateAfterPossibleHostMutation(reason){
  // The Agent may commit a mutation before the native response is lost.
  // These cards were authenticated only for a previous repository state.
  // Never allow a historical status/branch/diff card to guide the next write.
  gitStatusSnapshot=[];gitStatusAuthenticated=false;gitLogSnapshot=[];resetGitReviewQueue();
  $("git-status-files").textContent="Dữ liệu Git cũ đã hết hiệu lực; bấm Status để xác thực lại.";
  $("git-status-count").textContent="Cần tải lại";
  $("git-upstream-summary").textContent="Tracking cũ · cần Status mới";
  $("git-next-conflict").disabled=true;
  clearGitConflictMarkers("Host có thể đã thay đổi Git Status");
  $("git-log-entries").textContent="Commit log có thể đã thay đổi; bấm Log để xác thực lại.";
  $("git-log-selection").textContent="Chưa chọn commit";
  $("git-log-count").textContent="Cần tải lại";
  $("git-branch-entries").textContent="Danh sách nhánh có thể đã thay đổi; bấm Branches để xác thực lại.";
  $("git-branch-count").textContent="Cần tải lại";
  $("git-remotes-entries").textContent="Có thể đã thay đổi · yêu cầu Remotes để xác thực lại.";
  $("git-remotes-count").textContent="Cần tải lại";
  $("git-tracking-entries").textContent="Cần tải lại local tracking refs sau thay đổi repository.";
  $("git-tracking-count").textContent="Cần tải lại";
  clearGitDiffPreview("Cần tải lại sau "+reason);
  gitReview=null;$("git-approval").classList.add("hidden");
  // These buttons come from a previous host FilesList snapshot. A task or
  // branch switch may also change the workspace contents.
  $("workspace-entries").textContent="Workspace có thể đã thay đổi; bấm Duyệt để xác thực lại.";
  $("explorer-location").textContent="Cần duyệt lại";
  $("find-entries").textContent="Kết quả tìm trước đó có thể đã thay đổi.";
  $("find-summary").textContent="Cần tìm lại";
  clearSourcePages();
 }


 // Queue operations are navigation/review shortcuts only, never Git writes.
 // Every action obtains a NEW host SHA-CAS review and then requires
 // a distinct human confirmation through the normal Execute Git flow.
 function queuedGitWriteTarget(mode){
  if(!["stage","unstage"].includes(mode)||sessionEnding||
   !gitStatusAuthenticated||gitReviewQueueSource!==gitStatusSnapshot||
   gitReviewQueueIndex<0||gitReviewQueueIndex>=gitReviewQueue.length||
   gitReviewQueue.length>8)return null;
  const path=gitReviewQueue[gitReviewQueueIndex];
  if($("git-path").value!==path||!gitReviewQueueLoaded.has(path))return null;
  const entry=gitStatusSnapshot.find(e=>e.path===path);
  if(!entry||entry.conflict||entry.untracked||
   !(mode==="stage"?entry.unstaged:entry.staged))return null;
  return {path,source:gitStatusSnapshot,queue:gitReviewQueue,entry};
 }
 async function reviewQueuedGitWrite(mode){
  if(busy||sessionEnding||used+2>max)return;
  const target=queuedGitWriteTarget(mode);
  if(!target){
   $("status").textContent="Chưa đủ điều kiện: tải Git Status, chọn file trong queue và Review 2 chiều trước.";
   return;
  }
  gitReview=null;$("git-approval").classList.add("hidden");
  fileReview=null;$("file-editor").classList.add("hidden");
  lineReview=null;$("line-editor").classList.add("hidden");
  review=null;$("approval").classList.add("hidden");
  const input={operation:"git_write_review",mode,path:target.path};
  const result=await call(input);
  if(sessionEnding)return;
  if(!result)return;
  // The selected file, authenticated status and queue provenance must
  // still match at the moment the host review arrives.
  if(target.source!==gitStatusSnapshot||target.queue!==gitReviewQueue||
   queuedGitWriteTarget(mode)?.path!==target.path){
   gitReview=null;$("git-approval").classList.add("hidden");
   $("status").textContent="Review cũ: hàng đợi/file Git đã đổi trong lúc Agent phản hồi.";
   return;
  }
  if(acceptGitWriteReview(result,input)){
   $("status").textContent="Host đã review "+mode.toUpperCase()+
    " file từ queue · đọc kết quả rồi nhập xác nhận riêng để ghi.";
  }
 }
 $("git-queue-review-stage").addEventListener("click",()=>reviewQueuedGitWrite("stage"));
 $("git-queue-review-unstage").addEventListener("click",()=>reviewQueuedGitWrite("unstage"));

 function showGitReviewQueue(){
  const valid=gitReviewQueueSource===gitStatusSnapshot&&
   gitReviewQueue.length>0&&gitReviewQueueIndex>=0&&
   gitReviewQueueIndex<gitReviewQueue.length;
  const current=valid?gitReviewQueue[gitReviewQueueIndex]:null;
  const loaded=valid?gitReviewQueue.filter(path=>gitReviewQueueLoaded.has(path)).length:0;
  $("git-review-queue-list").textContent=valid?
   gitReviewQueue.map((path,i)=>(i===gitReviewQueueIndex?"● ":"○ ")+path+
    (gitReviewQueueLoaded.has(path)?" · đã tải review 2 chiều":" · chưa tải review 2 chiều")).join("\n"):
   "Chưa có file trong danh sách review.";
  $("git-review-queue-summary").textContent=valid?
   "File "+(gitReviewQueueIndex+1)+"/"+gitReviewQueue.length+
   " · đã tải review 2 chiều "+loaded+"/"+gitReviewQueue.length:
   "0/0 file · chỉ chọn từ Git Status đã xác thực";
  $("git-queue-prev").disabled=!valid||gitReviewQueueIndex===0;
  $("git-queue-next").disabled=!valid||gitReviewQueueIndex===gitReviewQueue.length-1;
  $("git-queue-next-unread").disabled=!valid||
   !gitReviewQueue.some((path,i)=>i!==gitReviewQueueIndex&&!gitReviewQueueLoaded.has(path));
  $("git-queue-clear").disabled=!valid;
  $("git-queue-review-stage").disabled=busy||used+2>max||!queuedGitWriteTarget("stage");
  $("git-queue-review-unstage").disabled=busy||used+2>max||!queuedGitWriteTarget("unstage");
 }
 $("git-path").addEventListener("input",showGitReviewQueue);
 function resetGitReviewQueue(){
  gitReviewQueue=[];gitReviewQueueIndex=-1;gitReviewQueueSource=null;
  gitReviewQueueLoaded.clear();
  showGitReviewQueue();
 }
 function selectGitReviewQueueFile(){
  if(busy||sessionEnding||gitReviewQueueSource!==gitStatusSnapshot||
   gitReviewQueueIndex<0||gitReviewQueueIndex>=gitReviewQueue.length){
   resetGitReviewQueue();return;
  }
  const path=gitReviewQueue[gitReviewQueueIndex];
  if(!gitStatusSnapshot.some(entry=>entry.path===path)){
   resetGitReviewQueue();
   $("status").textContent="Git status đã đổi; cần xác thực lại.";
   return;
  }
  $("git-path").value=path;$("file").value=path;
  invalidateChangedReviews();
  clearGitDiffPreview("Đã chọn file từ danh sách review");
  clearGitConflictMarkers("Đã chọn file từ danh sách review");
  gitReview=null;$("git-approval").classList.add("hidden");
  showGitReviewQueue();
  $("status").textContent="Đã chọn "+path+" · bấm Diff/Review 2 chiều; không tự gửi Agent";
 }
 $("git-queue-add").addEventListener("click",()=>{
  if(busy||sessionEnding)return;
  const path=$("git-path").value.trim();
  if(!path||!gitStatusSnapshot.some(entry=>entry.path===path)){
   $("status").textContent="Chỉ thêm file từ Git Status đã xác thực.";return;
  }
  if(gitReviewQueueSource!==gitStatusSnapshot)resetGitReviewQueue();
  if(gitReviewQueue.includes(path)){
   gitReviewQueueIndex=gitReviewQueue.indexOf(path);
   selectGitReviewQueueFile();return;
  }
  if(gitReviewQueue.length>=8){
   $("status").textContent="Danh sách review tối đa 8 file. Xóa danh sách để tạo đợt khác.";
   return;
  }
  gitReviewQueue.push(path);
  gitReviewQueueSource=gitStatusSnapshot;
  gitReviewQueueIndex=gitReviewQueue.length-1;
  selectGitReviewQueueFile();
 });
 $("git-queue-prev").addEventListener("click",()=>{
  if(busy||sessionEnding||$("git-queue-prev").disabled)return;
  gitReviewQueueIndex--;selectGitReviewQueueFile();
 });
 $("git-queue-next").addEventListener("click",()=>{
  if(busy||sessionEnding||$("git-queue-next").disabled)return;
  gitReviewQueueIndex++;selectGitReviewQueueFile();
 });
 $("git-queue-clear").addEventListener("click",()=>{
  if(busy||sessionEnding)return;
  resetGitReviewQueue();
  $("status").textContent="Đã xóa danh sách review cục bộ; không thay đổi Git.";
 });
 $("git-queue-next-unread").addEventListener("click",()=>{
  if(busy||sessionEnding||$("git-queue-next-unread").disabled||
   gitReviewQueueSource!==gitStatusSnapshot)return;
  for(let offset=1;offset<gitReviewQueue.length;offset++){
   const index=(gitReviewQueueIndex+offset)%gitReviewQueue.length;
   if(!gitReviewQueueLoaded.has(gitReviewQueue[index])){
    gitReviewQueueIndex=index;
    selectGitReviewQueueFile();
    return;
   }
  }
  showGitReviewQueue();
 });
 $("git-queue-add-visible").addEventListener("click",()=>{
  if(busy||sessionEnding||!gitStatusSnapshot.length)return;
  const query=$("git-status-filter").value.toLowerCase().trim();
  const scope=$("git-status-scope").value||"all";
  if(new TextEncoder().encode(query).length>96||
   /[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(query)||
   !["all","staged","unstaged","untracked","conflict"].includes(scope)){
   $("status").textContent="Bộ lọc Git không hợp lệ; không tạo danh sách review.";
   return;
  }
  const files=gitStatusSnapshot.filter(e=>
   (!query||e.path.toLowerCase().includes(query))&&(scope==="all"||e[scope])).map(e=>e.path);
  const existing=gitReviewQueueSource===gitStatusSnapshot?gitReviewQueue:[];
  const combined=[...existing];
  for(const path of files){if(!combined.includes(path))combined.push(path);}
  if(!files.length||combined.length>8){
   $("status").textContent=files.length?
    "Quá 8 file; hãy lọc Git Status hoặc xóa danh sách trước. Không thêm một phần.":
    "Không có file khớp bộ lọc để thêm vào danh sách.";
   return;
  }
  if(gitReviewQueueSource!==gitStatusSnapshot)resetGitReviewQueue();
  gitReviewQueue=combined;gitReviewQueueSource=gitStatusSnapshot;
  gitReviewQueueIndex=0;
  selectGitReviewQueueFile();
 });


 function renderGitUpstreamTracking(lines){
  const target=$("git-upstream-summary");
  target.textContent="Upstream chưa được xác định · không suy ra ahead/behind";
  // Trust only authenticated, bounded Git status --short --branch text.
  // Ahead/behind describes LOCAL tracking refs, not a fresh network fetch.
  if(!lines.length||!lines[0].startsWith("## "))return;
  const header=lines[0];
  if(header==="## HEAD (no branch)"){
   target.textContent="Detached HEAD · chưa xác định upstream";return;
  }
  const match=/^## ([A-Za-z0-9][A-Za-z0-9._/-]{0,95})\.\.\.([A-Za-z0-9][A-Za-z0-9._/-]{0,95})(?: \[(ahead ([0-9]{1,10})(?:, behind ([0-9]{1,10}))?|behind ([0-9]{1,10})|gone)\])?$/u.exec(header);
  if(!match)return;
  if(match[3]==="gone"){
   target.textContent="Tracking local: "+match[1]+" → "+match[2]+" · upstream mất trong local refs (chưa fetch)";
   return;
  }
  const ahead=match[4]?Number(match[4]):0;
  const behind=match[5]?Number(match[5]):match[6]?Number(match[6]):0;
  if(!Number.isSafeInteger(ahead)||!Number.isSafeInteger(behind)||
   ahead>1000000000||behind>1000000000)return;
  target.textContent="Tracking local: "+match[1]+" → "+match[2]+
   " · ahead "+ahead+" / behind "+behind+" · chưa fetch từ remote";
 }
 const gitConflictCodes=new Set(["DD","AU","UD","UA","DU","AA","UU"]);

 function currentGitConflictEntry(){
  if(!gitStatusAuthenticated||sessionEnding)return null;
  const path=$("git-path").value;
  return gitStatusSnapshot.find(entry=>entry.conflict&&entry.path===path)||null;
 }
 function syncGitConflictReadButton(){
  $("git-conflict-read").disabled=!currentGitConflictEntry()||sessionEnding;
 }
 function clearGitConflictMarkers(message="Chưa đọc file conflict"){
  if(gitConflictDraft&&fileReview&&fileReview.path===gitConflictDraft.snapshot.path){
   fileReview=null;$("file-editor").classList.add("hidden");
  }
  gitConflictGeneration++;
  gitConflictPreviewSnapshot=null;
  gitConflictSelectedIndex=-1;gitConflictDraft=null;
  $("git-conflict-draft-ours").disabled=true;
  $("git-conflict-draft-theirs").disabled=true;
  $("git-conflict-draft-review").disabled=true;
  $("git-conflict-draft-label").textContent="Chưa chọn phía cho bản nháp.";
  $("git-conflict-draft-text").textContent="Bản nháp chỉ xuất hiện sau khi chọn một khối và một phía.";
  $("git-conflict-marker-count").textContent=message;
  $("git-conflict-marker-entries").replaceChildren();
  $("git-conflict-marker-selected").textContent="Chưa chọn khối conflict";
  $("git-conflict-marker-text").textContent="Nội dung đọc qua Agent sẽ hiển thị ở đây sau khi chọn một khối hợp lệ.";
  syncGitConflictReadButton();
 }
 function renderGitConflictMarkers(report,path,source){
  clearGitConflictMarkers("Đang xác minh file conflict");
  if(typeof report!=="string"||new TextEncoder().encode(report).length>4096||
   source!==gitStatusSnapshot||!gitStatusAuthenticated||sessionEnding||
   $("git-path").value!==path||!currentGitConflictEntry()){
   $("git-conflict-marker-count").textContent="Đã từ chối bản xem trước không hợp lệ hoặc đã hết hiệu lực";
   return;
  }
  const lines=report.split("\n"),blocks=[];
  let open=null,separator=-1,invalid=false;
  const header=/^<<<<<<< [^\r\n]{1,80}$/u;
  const footer=/^>>>>>>> [^\r\n]{1,80}$/u;
  const unsafe=/[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u;
  for(let i=0;i<lines.length;i++){
   const raw=lines[i],line=raw.endsWith("\r")?raw.slice(0,-1):raw;
   const begin=line.startsWith("<<<<<<<"),middle=line.startsWith("======="),
    end=line.startsWith(">>>>>>>"),base=line.startsWith("|||||||");
   if(!(begin||middle||end||base))continue;
   if(unsafe.test(line)||base){invalid=true;break;}
   if(begin){
    if(open!==null||!header.test(line)){invalid=true;break;}
    open=i;separator=-1;
   }else if(middle){
    if(line!=="======="||open===null||separator>=0){invalid=true;break;}
    separator=i;
   }else if(end){
    if(!footer.test(line)||open===null||separator<0){invalid=true;break;}
    blocks.push({start:open,separator,end:i,sourceLabel:lines[open],targetLabel:raw});
    open=null;separator=-1;
    if(blocks.length>16){invalid=true;break;}
   }
  }
  if(open!==null||invalid){
   $("git-conflict-marker-count").textContent="Marker thiếu/cấu trúc phức tạp: không tạo khối chọn";
   return;
  }
  if(blocks.length===0){
   $("git-conflict-marker-count").textContent="0 marker chuẩn · file vẫn được Git báo conflict";
   $("git-conflict-marker-entries").textContent=
    "Không có cặp marker trong preview ≤4 KiB. Có thể là conflict nhị phân, xóa file hoặc kiểu conflict khác.";
   return;
  }
  const snapshot={source,path,report,blocks};
  gitConflictPreviewSnapshot=snapshot;
  const generation=gitConflictGeneration;
  $("git-conflict-marker-count").textContent=blocks.length+
   " khối conflict · file "+path+" · chưa phải xác nhận giải quyết";
  for(let n=0;n<blocks.length;n++){
   const block=blocks[n],button=document.createElement("button");
   button.type="button";button.className="git-status-card";
   button.textContent="Conflict "+(n+1)+"/"+blocks.length+
    " · dòng "+(block.start+1)+"–"+(block.end+1);
   button.addEventListener("click",()=>{
    if(busy||sessionEnding||gitConflictGeneration!==generation||
     gitConflictPreviewSnapshot!==snapshot||source!==gitStatusSnapshot||
     !gitStatusAuthenticated||$("git-path").value!==path||
     !currentGitConflictEntry()){
     $("git-conflict-marker-selected").textContent="Khối conflict cũ; cần xem lại file đã xác thực.";
     return;
    }
    const safe=lines.slice(block.start,block.end+1).join("\n").replace(
     /[\u0000-\u0008\u000b-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/gu,
     c=>"\\u"+c.codePointAt(0).toString(16).padStart(4,"0"));
    $("git-conflict-marker-selected").textContent="Khối "+(n+1)+"/"+blocks.length+
     " · dòng "+(block.start+1)+"–"+(block.end+1)+" · chỉ đọc, không tự resolve";
    $("git-conflict-marker-text").textContent=safe;
    if(gitConflictDraft&&fileReview&&fileReview.path===gitConflictDraft.snapshot.path){
     fileReview=null;$("file-editor").classList.add("hidden");
    }
    gitConflictSelectedIndex=n;gitConflictDraft=null;
    $("git-conflict-draft-ours").disabled=false;
    $("git-conflict-draft-theirs").disabled=false;
    $("git-conflict-draft-review").disabled=true;
    $("git-conflict-draft-label").textContent="Khối "+(n+1)+"/"+blocks.length+
     " · chọn OURS hoặc THEIRS cho bản nháp; chưa ghi file.";
    $("git-conflict-draft-text").textContent="Chưa chọn phía cho khối này.";
   });
   $("git-conflict-marker-entries").append(button);
  }
 }
 $("git-conflict-read").addEventListener("click",async()=>{
  if(busy||sessionEnding)return;
  const entry=currentGitConflictEntry();
  if(!entry){clearGitConflictMarkers("Cần Git Status mới và một file có conflict đã chọn");return;}
  const source=gitStatusSnapshot,path=entry.path;
  clearGitConflictMarkers("Đang xác thực đọc file conflict");
  // A read invalidates any unrelated visible one-use host approvals.
  gitReview=null;$("git-approval").classList.add("hidden");
  fileReview=null;$("file-editor").classList.add("hidden");
  lineReview=null;$("line-editor").classList.add("hidden");
  review=null;$("approval").classList.add("hidden");
  const result=await call({operation:"file_view",path});
  if(sessionEnding)return;
  if(!result){
   clearGitConflictMarkers("Không đọc được file conflict · không có dữ liệu cũ");
   return;
  }
  if(source!==gitStatusSnapshot||!source.includes(entry)||$("git-path").value!==path||
   !gitStatusAuthenticated){
   clearGitConflictMarkers("Git Status hoặc file đã đổi; phải đọc lại");
   $("status").textContent="File conflict đã đổi trước khi Agent phản hồi";
   return;
  }
  renderGitConflictMarkers(result.text||"",path,source);
 });

 function draftGitConflictSide(side){
  if(busy||sessionEnding||!["ours","theirs"].includes(side))return;
  const snapshot=gitConflictPreviewSnapshot,index=gitConflictSelectedIndex;
  if(!snapshot||snapshot.source!==gitStatusSnapshot||!gitStatusAuthenticated||
   !currentGitConflictEntry()||snapshot.path!==$("git-path").value||
   index<0||index>=snapshot.blocks.length){
   clearGitConflictMarkers("Conflict cũ · cần tải lại file và chọn khối");
   return;
  }
  const lines=snapshot.report.split("\n"),block=snapshot.blocks[index];
  const chosen=side==="ours"?
   lines.slice(block.start+1,block.separator):
   lines.slice(block.separator+1,block.end);
  const candidate=[
   ...lines.slice(0,block.start),
   ...chosen,
   ...lines.slice(block.end+1)
  ].join("\n");
  if(candidate===snapshot.report||new TextEncoder().encode(candidate).length>4096||
   candidate.includes("\u0000")){
   $("status").textContent="Bản nháp không hợp lệ; không tạo review";
   return;
  }
  if(gitConflictDraft&&fileReview&&fileReview.path===gitConflictDraft.snapshot.path){
   fileReview=null;$("file-editor").classList.add("hidden");
  }
  gitConflictDraft={snapshot,index,side,content:candidate};
  $("git-conflict-draft-label").textContent="BẢN NHÁP · "+side.toUpperCase()+
   " · chỉ thay khối "+(index+1)+"/"+snapshot.blocks.length+
   " · chưa giải quyết các khối còn lại";
  $("git-conflict-draft-text").textContent=candidate.replace(
   /[\u0000-\u0008\u000b-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/gu,
   c=>"\\u"+c.codePointAt(0).toString(16).padStart(4,"0"));
  $("git-conflict-draft-review").disabled=false;
  $("status").textContent="Bản nháp chỉ ở GUI; cần review lại file từ Agent trước khi chuyển vào editor";
 }
 $("git-conflict-draft-ours").addEventListener("click",()=>draftGitConflictSide("ours"));
 $("git-conflict-draft-theirs").addEventListener("click",()=>draftGitConflictSide("theirs"));
 $("git-conflict-draft-review").addEventListener("click",async()=>{
  if(busy||sessionEnding||$("git-conflict-draft-review").disabled)return;
  const draft=gitConflictDraft,snapshot=draft?.snapshot;
  if(!draft||!snapshot||gitConflictPreviewSnapshot!==snapshot||
   snapshot.source!==gitStatusSnapshot||!gitStatusAuthenticated||
   $("git-path").value!==snapshot.path||!currentGitConflictEntry()||
   gitConflictSelectedIndex!==draft.index){
   clearGitConflictMarkers("Bản nháp đã hết hiệu lực; cần đọc file lại");
   return;
  }
  if(used+2>max){
   $("status").textContent="Không đủ 2 lượt E2EE cho review và ghi: cần đăng nhập lại";
   return;
  }
  // Independent host CAS review: refuse to reuse the earlier file_view as a
  // write approval. Human must still inspect and enter exact confirmation.
  gitReview=null;$("git-approval").classList.add("hidden");
  fileReview=null;$("file-editor").classList.add("hidden");
  lineReview=null;$("line-editor").classList.add("hidden");
  review=null;$("approval").classList.add("hidden");
  $("git-conflict-draft-review").disabled=true;
  const result=await call({operation:"file_edit_review",path:snapshot.path});
  if(sessionEnding)return;
  if(!result||gitConflictDraft!==draft||
   gitConflictPreviewSnapshot!==snapshot||
   snapshot.source!==gitStatusSnapshot||!gitStatusAuthenticated||
   $("git-path").value!==snapshot.path||!currentGitConflictEntry()||
   typeof result.text!=="string"||result.text!==snapshot.report||
   typeof result.confirm!=="string"||!result.confirm||
   new TextEncoder().encode(result.text).length>4096){
   fileReview=null;$("file-editor").classList.add("hidden");
   clearGitConflictMarkers("Review không khớp file conflict; phải đọc lại Status/file");
   $("status").textContent="File đã thay đổi hoặc review không hợp lệ: không được dùng bản nháp";
   return;
  }
  $("file").value=snapshot.path;
  fileReview={path:snapshot.path,confirm:result.confirm,original:result.text};
  $("file-edit-text").value=draft.content;
  $("file-edit-confirm").value="";
  $("file-edit-approval").textContent="Bản nháp "+draft.side.toUpperCase()+
   " trên 1 khối; Agent vừa xác nhận nội dung gốc còn khớp. Xem kỹ"+
   " toàn bộ file rồi nhập đúng chuỗi riêng để GHI: "+result.confirm;
  $("file-editor").classList.remove("hidden");
  $("status").textContent="Đã tạo File editor từ review mới. Chưa ghi hoặc stage; xác nhận riêng nếu muốn.";
 });

 $("git-path").addEventListener("input",()=>clearGitConflictMarkers("Đường dẫn Git đã đổi"));

 function showGitStatusCards(){
  const generation=++gitStatusRenderGeneration;
  const target=$("git-status-files");
  target.replaceChildren();
  const query=$("git-status-filter").value.toLowerCase().trim();
  const scope=$("git-status-scope").value||"all";
  if(new TextEncoder().encode(query).length>96||
   /[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(query)||
   !["all","staged","unstaged","untracked","conflict"].includes(scope)){
   target.textContent="Bộ lọc Git không hợp lệ.";
   $("git-status-count").textContent="Lọc không hợp lệ";
   return;
  }
  const matching=gitStatusSnapshot.filter(entry=>
   (!query||entry.path.toLowerCase().includes(query))&&
   (scope==="all"||entry[scope]));
  const counts={
   staged:gitStatusSnapshot.filter(e=>e.staged).length,
   unstaged:gitStatusSnapshot.filter(e=>e.unstaged).length,
   untracked:gitStatusSnapshot.filter(e=>e.untracked).length,
   conflict:gitStatusSnapshot.filter(e=>e.conflict).length
  };
  $("git-status-count").textContent=matching.length+"/"+gitStatusSnapshot.length+
   " file · "+counts.staged+" staged · "+counts.unstaged+" unstaged · "+
   counts.untracked+" untracked · "+counts.conflict+" conflict";
  $("git-next-conflict").disabled=counts.conflict===0||sessionEnding;
  if(!matching.length){
   target.textContent=gitStatusSnapshot.length?
    "Không có file phù hợp với bộ lọc cục bộ.":
    "Không có file an toàn trong Git status hiện tại.";
   return;
  }
  for(const entry of matching){
   const button=document.createElement("button");
   button.type="button";button.className="git-status-card";
   button.textContent=entry.status+" · "+entry.path;
   const source=gitStatusSnapshot;
   button.addEventListener("click",()=>{
    if(busy||sessionEnding)return;
    if(gitStatusRenderGeneration!==generation||gitStatusSnapshot!==source||!source.includes(entry)){
     $("status").textContent="Git Status cũ · cần tải lại trước khi chọn file.";return;
    }
    // No implicit git write or E2EE action on choosing a status card.
    $("git-path").value=entry.path;
    $("file").value=entry.path;
    invalidateChangedReviews();
    clearGitDiffPreview("Đã chọn file khác");
    clearGitConflictMarkers("Đã chọn file khác");
    gitReview=null;
    $("git-approval").classList.add("hidden");
    $("status").textContent="Đã chọn Git file · cần xem Diff hoặc review thao tác ghi";
   });
   target.append(button);
  }
 }
 ["git-status-filter","git-status-scope"].forEach(id=>
  $(id).addEventListener("input",showGitStatusCards));

 $("git-next-conflict").addEventListener("click",()=>{
  if(busy||sessionEnding||$("git-next-conflict").disabled)return;
  const conflicts=gitStatusSnapshot.filter(e=>e.conflict);
  if(!conflicts.length){$("git-next-conflict").disabled=true;return;}
  const selected=$("git-path").value.trim();
  const current=conflicts.findIndex(e=>e.path===selected);
  const next=conflicts[(current+1)%conflicts.length];
  // Only select an authenticated safe path: no network, Git stage,
  // conflict resolution or bypass of host write approvals.
  $("git-path").value=next.path;
  $("file").value=next.path;
  invalidateChangedReviews();
  clearGitDiffPreview("Đã chọn file xung đột khác");
  clearGitConflictMarkers("Đã chọn conflict khác");
  gitReview=null;$("git-approval").classList.add("hidden");
  $("status").textContent="Conflict "+(conflicts.indexOf(next)+1)+"/"+conflicts.length+
   " · "+next.path+" · bấm Diff/Review để xem nội dung qua E2EE";
 });

 function renderGitStatus(report){
  const target=$("git-status-files");
  target.replaceChildren();gitStatusSnapshot=[];gitStatusAuthenticated=false;resetGitReviewQueue();
  clearGitConflictMarkers("Git Status đã được tải lại");
  $("git-upstream-summary").textContent="Tracking chưa xác thực";
  $("git-next-conflict").disabled=true;
  $("git-status-count").textContent="Đang kiểm tra";
  if(typeof report!=="string"||new TextEncoder().encode(report).length>4096){
   target.textContent="Git status vượt giới hạn hoặc không hợp lệ.";
   $("git-status-count").textContent="Không hợp lệ";return;
  }
  const lines=report.split("\n");
  if(lines.length>65){
   target.textContent="Quá nhiều file thay đổi để lập danh sách chọn an toàn.";
   $("git-status-count").textContent="Giới hạn";return;
  }
  const entries=[],seen=new Set();
  for(const line of lines){
   if(!line||line.startsWith("## "))continue;
   const row=/^([ MADRCU?!]{2}) (.+)$/u.exec(line);
   if(!row||row[1]==="  "||row[1]==="!!")continue;
   const path=row[2],status=row[1];
   if(path.length>120||path.includes("..")||path.startsWith("-")||
     /[\p{Cf}\p{Zl}\p{Zp}]/u.test(path)||
    path.startsWith('"')||path.includes('"')||path.includes(" -> ")||
    /[*?\[\]]/u.test(path)||!safeWorkspacePath(path)||
    path.split("/").some(segment=>segment.startsWith("."))||
    seen.has(path))continue;
   seen.add(path);entries.push({path,status,
    staged:status[0]!==" "&&status[0]!=="?",
    unstaged:status[1]!==" "&&status[1]!=="?",
    untracked:status==="??",
    conflict:gitConflictCodes.has(status)});
  }
  gitStatusSnapshot=entries;gitStatusAuthenticated=true;
  renderGitUpstreamTracking(lines);
  showGitStatusCards();
  syncGitConflictReadButton();
 }

 function showGitLogCards(){
  const target=$("git-log-entries");
  target.replaceChildren();
  const query=$("git-log-filter").value.toLowerCase().trim();
  if(query.length>96||/[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(query)){
   target.textContent="Bộ lọc không hợp lệ.";
   return;
  }
  const filtered=gitLogSnapshot.filter(c=>!query||
   c.short.includes(query)||c.subject.toLowerCase().includes(query));
  if(!filtered.length){
   target.textContent="Không có commit phù hợp với bộ lọc cục bộ.";
   return;
  }
  for(const commit of filtered){
   const btn=document.createElement("button");
   btn.type="button";
   btn.className="git-status-card";
   btn.textContent=commit.short+" · "+commit.subject;
   btn.addEventListener("click",()=>{
    if(busy)return;
    gitReview=null;$("git-approval").classList.add("hidden");
    $("git-log-selection").textContent="Đã chọn (chỉ đọc): "+commit.short+" · "+commit.subject;
    $("status").textContent="Git commit chỉ xem · không checkout hoặc gửi lệnh";
   });
   target.append(btn);
  }
 }
 function renderGitLog(report){
  const target=$("git-log-entries");
  target.replaceChildren();
  gitLogSnapshot=[];
  $("git-log-selection").textContent="Chưa chọn commit";
  $("git-log-count").textContent="Đang kiểm tra";
  const reject=()=>{
   target.textContent="Git log không nguyên vẹn hoặc vượt giới hạn, không tạo liên kết.";
   $("git-log-count").textContent="Không hợp lệ";
  };
  if(typeof report!=="string"||new TextEncoder().encode(report).length>4096){reject();return;}
  if(!report){
   $("git-log-count").textContent="0 commit";
   target.textContent="Không có commit để hiển thị.";return;
  }
  const rows=(report.endsWith("\n")?report.slice(0,-1):report).split("\n");
  if(rows.length>30){reject();return;}
  const seen=new Set(),items=[];
  for(const row of rows){
   const match=/^([0-9a-f]{4,64}) ([^\r\n]+)$/u.exec(row);
   if(!match){reject();return;}
   const short=match[1],subject=match[2];
   if(seen.has(short)||subject.trim()!==subject||
    new TextEncoder().encode(subject).length>160||
    /[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(subject)){
    reject();return;
   }
   seen.add(short);items.push({short,subject});
  }
  gitLogSnapshot=items;
  $("git-log-count").textContent=items.length+" commit";
  showGitLogCards();
 }
 $("git-log-filter").addEventListener("input",()=>showGitLogCards());


 function renderGitTrackingRefs(report){
  const target=$("git-tracking-entries");
  target.replaceChildren();
  $("git-tracking-count").textContent="Không hợp lệ";
  if(typeof report!=="string"||new TextEncoder().encode(report).length>4096||
   !report.startsWith("GIT_TRACKING_V1\n")){
   target.textContent="Đã từ chối định dạng tracking refs không hợp lệ.";
   return false;
  }
  const data=report.slice("GIT_TRACKING_V1\n".length);
  if(data===""){
   $("git-tracking-count").textContent="0 refs cục bộ";
   target.textContent="Chưa có remote-tracking refs trong clone cục bộ. Không thực hiện fetch.";
   return true;
  }
  if(!data.endsWith("\n")){
   target.textContent="Thiếu dấu kết thúc tracking refs.";return false;
  }
  const rows=data.slice(0,-1).split("\n"),items=[],seen=new Set();
  const segment=/^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,62}[A-Za-z0-9])?$/u;
  for(const row of rows){
   const match=/^([^ \r\n]+) ([0-9a-f]{40}|[0-9a-f]{64})$/u.exec(row);
   if(!match||rows.length>32||match[1].length>160||
    match[1].endsWith("/HEAD")||seen.has(match[1])){
    target.textContent="Tracking refs quá nhiều, không an toàn hoặc bị trùng.";
    return false;
   }
   const parts=match[1].split("/");
   if(parts.length<2||parts.length>7||
    parts.some(p=>!segment.test(p)||p.includes("..")||p.endsWith(".lock"))){
    target.textContent="Đường dẫn tracking refs bị từ chối.";
    return false;
   }
   seen.add(match[1]);items.push({name:match[1],oid:match[2]});
  }
  for(const item of items){
   const row=document.createElement("div");row.className="git-status-card";
   row.textContent="● "+item.name+" · "+item.oid.slice(0,12)+
    " · local snapshot (không fetch)";
   target.append(row);
  }
  $("git-tracking-count").textContent=items.length+" refs cục bộ · có thể cũ";
  return true;
 }

 function renderGitRemotes(report){
  const target=$("git-remotes-entries");
  target.replaceChildren();
  $("git-remotes-count").textContent="Không hợp lệ";
  // Only the versioned, name-only authenticated envelope is accepted.
  // URL, credentials and arbitrary Git configuration must never become UI.
  if(typeof report!=="string"||new TextEncoder().encode(report).length>1100||
   !report.startsWith("GIT_REMOTES_V1\n")){
   target.textContent="Từ chối Git remote metadata không hợp lệ.";
   return false;
  }
  const data=report.slice("GIT_REMOTES_V1\n".length);
  if(data===""){
   $("git-remotes-count").textContent="0 remote";
   target.textContent="Chưa cấu hình Git remote trên host.";
   return true;
  }
  if(!data.endsWith("\n")){
   target.textContent="Thiếu dấu kết thúc danh sách Git remote.";
   return false;
  }
  const names=data.slice(0,-1).split("\n");
  const seen=new Set();
  if(names.length>16||names.some(name=>
   !/^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,62}[A-Za-z0-9])?$/u.test(name)||
   name.includes("..")||seen.has(name)||!seen.add(name))){
   target.textContent="Remote names có ký tự không an toàn hoặc bị trùng.";
   return false;
  }
  for(const name of names){
   const row=document.createElement("div");
   row.className="git-status-card";
   row.textContent="● "+name+" · chỉ tên, không chứa URL";
   target.append(row);
  }
  $("git-remotes-count").textContent=names.length+" remote · offline";
  return true;
 }
 function renderGitBranches(report){
  const target=$("git-branch-entries");
  target.replaceChildren();
  $("git-branch-count").textContent="Đang kiểm tra";
  if(typeof report!=="string"||new TextEncoder().encode(report).length>4096){
   target.textContent="Danh sách nhánh vượt giới hạn hoặc không hợp lệ.";
   $("git-branch-count").textContent="Không hợp lệ";return;
  }
  const rows=report.split("\n");
  const entries=[],seen=new Set();
  if(rows.length>65){
   target.textContent="Có quá nhiều nhánh để duyệt an toàn.";
   $("git-branch-count").textContent="Giới hạn";return;
  }
  for(const line of rows){
   const match=/^([ *+]) taskdeck\/([a-z][a-z0-9-]{1,31}) +([0-9a-f]{40}|[0-9a-f]{64})(?: .*)?$/u.exec(line);
   if(!match)continue;
   const slug=match[2];
   if(slug.endsWith("-")||seen.has(slug))continue;
   seen.add(slug);
   entries.push({slug,current:match[1]==="*",elsewhere:match[1]==="+"});
  }
  $("git-branch-count").textContent=entries.length+" nhánh";
  if(!entries.length){
   target.textContent="Không có nhánh taskdeck/ hợp lệ trong kết quả đã xác thực.";
   return;
  }
  for(const entry of entries){
   const button=document.createElement("button");
   button.type="button";button.className="git-status-card";
   button.textContent=(entry.current?"● ":entry.elsewhere?"⊕ ":"⑂ ")+
    "taskdeck/"+entry.slug+(entry.current?" · hiện tại":entry.elsewhere?" · worktree khác":"");
   button.addEventListener("click",()=>{
    if(busy)return;
    $("git-branch").value=entry.slug;
    invalidateChangedReviews();
    gitReview=null;
    $("git-approval").classList.add("hidden");
    $("status").textContent="Đã chọn nhánh taskdeck/"+entry.slug+
     " · cần review thao tác chuyển/xóa trước khi thực hiện";
   });
   target.append(button);
  }
 }



 function renderFindMatches(report,path,literal){
  const target=$("find-entries");
  target.replaceChildren();
  $("find-summary").textContent="Đang xác minh";
  const reject=()=>{
   target.textContent="Kết quả tìm kiếm sai định dạng hoặc vượt giới hạn; không chọn dòng.";
   $("find-summary").textContent="Không hợp lệ";
  };
  if(typeof report!=="string"||!safeWorkspacePath(path)||path==="."||
    !literal||new TextEncoder().encode(report).length>4096){reject();return;}
  const m=/^FILE_SHA256 ([0-9a-f]{64})\nTERM_SHA256 ([0-9a-f]{64})\nTOTAL_LINES (0|[1-9][0-9]*)\nMATCHES (0|[1-9][0-9]*)\n-- COMPLETE MATCHING LINES --\n/u.exec(report);
  if(!m){reject();return;}
  const total=Number(m[3]),count=Number(m[4]);
  const data=report.slice(m[0].length);
  if(!Number.isSafeInteger(total)||!Number.isSafeInteger(count)||
    total>2097152||count>total||count>64){reject();return;}
  if(count===0){
   if(data!==""){reject();return;}
   $("find-summary").textContent="0 kết quả · SHA "+m[1].slice(0,12);
   target.textContent="Không tìm thấy chuỗi trong file tại thời điểm đọc.";
   return;
  }
  if(!data.endsWith("\n")){reject();return;}
  const rows=data.slice(0,-1).split("\n");
  if(rows.length!==count){reject();return;}
  const entries=[];
  let previous=0;
  for(const line of rows){
   const sep=line.indexOf(": ");
   if(sep<1){reject();return;}
   const digits=line.slice(0,sep),body=line.slice(sep+2),number=Number(digits);
   if(!/^[1-9][0-9]*$/u.test(digits)||!Number.isSafeInteger(number)||
     number>total||number<=previous||String(number)!==digits||
     !body.includes(literal)){reject();return;}
   entries.push({number,body});
   previous=number;
  }
  $("find-summary").textContent=count+" dòng khớp · SHA "+m[1].slice(0,12);
  for(const entry of entries){
   const button=document.createElement("button");
   button.type="button";
   button.className="git-status-card";
   const display=entry.body.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/gu,
    ch=>"\\u"+ch.codePointAt(0).toString(16).padStart(4,"0"));
   button.textContent=entry.number+": "+display;
   button.addEventListener("click",()=>{
    if(busy)return;
    if($("file").value.trim()!==path||$("find-literal").value!==literal){
     notify("Kết quả cũ","File hoặc chuỗi tìm đã đổi; hãy tìm lại trước khi chọn dòng.");return;
    }
    $("line").value=String(entry.number);
    $("status").textContent="Đã chọn dòng "+entry.number+" · nhấn Xem theo dòng để đọc lại từ Agent";
   });
   target.append(button);
  }
 }
 function clearSourcePages(){
  filePage=null;
  $("file-page-progress").textContent="Chưa đọc";
  $("file-page-transcript").textContent="Không có trang nguồn hợp lệ.";
  $("file-read-next").disabled=true;
 }
 function acceptSourcePage(report,path,start,continuation){
  const denied=()=>{
   clearSourcePages();
   notify("Từ chối trang file","Trang file không nguyên vẹn hoặc SHA của toàn file thay đổi; cần đọc từ đầu.");
   return false;
  };
  if(typeof report!=="string"||
   new TextEncoder().encode(report).length>4096||
   !safeWorkspacePath(path)||path==="."||start<1||start>2097152)return denied();
  const m=/^FILE_SHA256 ([0-9a-f]{64})\nSTART_LINE (0|[1-9][0-9]*)\nNEXT_LINE (0|[1-9][0-9]*)\nTOTAL_LINES (0|[1-9][0-9]*)\n-- COMPLETE SOURCE LINES --\n/u.exec(report);
  if(!m)return denied();
  const actualStart=Number(m[2]),next=Number(m[3]),total=Number(m[4]);
  const content=report.slice(m[0].length);
  if(!Number.isSafeInteger(actualStart)||!Number.isSafeInteger(next)||
   !Number.isSafeInteger(total)||actualStart!==start||total>2097152||
   total<actualStart||!content.endsWith("\n"))return denied();
  const lines=content.slice(0,-1).split("\n");
  if(lines.length===0||actualStart+lines.length-1>total)return denied();
  for(let i=0;i<lines.length;i++){
   if(!lines[i].startsWith(String(actualStart+i)+": "))return denied();
  }
  const last=actualStart+lines.length-1;
  if((next!==0&&next!==last+1)||(next===0&&last!==total)||
   (next!==0&&last>=total))return denied();
  if(continuation&&(!filePage||filePage.path!==path||
   filePage.next!==start||filePage.sha!==m[1]))return denied();
  // Maintain only four fully verified pages. A changed source SHA must
  // never produce a mixed historical transcript, even for read-only UI.
  const chunks=continuation?filePage.chunks.slice(-3):[];
  const safe=content.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/gu,
   ch=>"\\u"+ch.codePointAt(0).toString(16).padStart(4,"0"));
  chunks.push(safe);
  filePage={path,sha:m[1],next,chunks};
  $("file-page-transcript").textContent=chunks.join("");
  $("file-page-progress").textContent="Lines "+actualStart+"–"+last+" / "+total+
   (next===0?" · EOF":" · tiếp: "+next)+" · SHA "+m[1].slice(0,12);
  if(next!==0)$("line").value=String(next);
  $("file-read-next").disabled=busy||used>=max||next===0;
  return true;
 }
 function clearTaskWindow(){
  taskWindow=null;
  $("task-transcript").textContent="Chưa có cửa sổ output hợp lệ.";
  $("task-output-progress").textContent="Chưa đọc";
  $("task-read-next").disabled=true;
  $("console-task-window").textContent="Chưa có cửa sổ task đã xác thực.";
  $("console-task-progress").textContent="Chưa đọc output";
  $("console-task-next").disabled=true;
 }
 function acceptTaskWindow(report,handle,requestedOffset){
  const denied=()=>{
   clearTaskWindow();
   notify("Từ chối trang output","Dữ liệu cửa sổ task không nguyên vẹn; không tiếp tục tự động hoặc dùng lại cursor.");
   return false;
  };
  if(typeof report!=="string"||new TextEncoder().encode(report).length>4096)return denied();
  const m=/^TASK_HANDLE ([0-9a-f]{32})\nOUTPUT_OFFSET (0|[1-9][0-9]*)\nOUTPUT_NEXT (0|[1-9][0-9]*)\nOUTPUT_TOTAL (0|[1-9][0-9]*)\nTASK_STATE (running|exited|stopped)\nOUTPUT_EOF (yes|no)\n-- TASK OUTPUT WINDOW --\n/u.exec(report);
  if(!m||m[1]!==handle)return denied();
  const offset=Number(m[2]),next=Number(m[3]),total=Number(m[4]);
  const data=report.slice(m[0].length);
  const bytes=new TextEncoder().encode(data).length;
  if(!Number.isSafeInteger(offset)||!Number.isSafeInteger(next)||!Number.isSafeInteger(total)||
   offset!==requestedOffset||offset>next||next>total||total>1073741824||
   bytes>2048||next-offset!==bytes||
   (m[6]==="yes")!==(m[5]!=="running"&&next===total))return denied();
  // Never concatenate windows from different task handles or noncontiguous
  // byte offsets. Keep at most four verified windows in browser memory.
  const append=taskWindow&&taskWindow.handle===handle&&taskWindow.next===offset;
  const chunks=append?taskWindow.chunks.slice(-3):[];
  const safe=data.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/gu,
    ch=>"\\u"+ch.codePointAt(0).toString(16).padStart(4,"0"));
  chunks.push(safe);
  taskWindow={handle,next,eof:m[6]==="yes",chunks};
  $("task-transcript").textContent=chunks.join("");
  $("task-output-progress").textContent=next+" / "+total+" bytes · "+m[5]+
   (taskWindow.eof?" · EOF":" · có thể đọc tiếp");
  $("task-offset").value=String(next);
  $("task-read-next").disabled=busy||used>=max||taskWindow.eof;
  // Same validated E2EE byte window, mirrored in trusted Console UI.
  // No new Agent request and no raw terminal control sequences rendered.
  $("console-task-window").textContent=chunks.join("");
  $("console-task-progress").textContent=$("task-output-progress").textContent;
  $("console-task-next").disabled=busy||used>=max||taskWindow.eof||sessionEnding;
  return true;
 }
 // Console maps fixed typed reads and explicit Git *review* commands.
 // Git writes still require a separate human click with one-use confirmation
 // at the native Git panel; no shell syntax, arbitrary Git argv or PHP authority.

 $("console-task-attach").addEventListener("click",()=>{
  if(busy||sessionEnding||used>=max)return;
  const handle=$("task-handle").value.trim();
  if(!/^[0-9a-f]{32}$/.test(handle)){
   $("status").textContent="Handle không hợp lệ; chọn task đã được host phê duyệt trong tab Tasks.";
   return;
  }
  // Use the exact existing allowlisted Console command grammar.
  $("console-command").value="tasks output "+handle+" 0";
  return $("console-run").click();
 });
 $("console-task-next").addEventListener("click",()=>{
  if(busy||sessionEnding||used>=max||$("console-task-next").disabled||
   !taskWindow||taskWindow.eof||taskWindow.next<0||
   $("task-handle").value.trim()!==taskWindow.handle||
   $("task-offset").value!==String(taskWindow.next)){
   if(!sessionEnding)$("status").textContent="Cursor task không còn hợp lệ; cần đọc lại handle từ đầu.";
   return;
  }
  // "tasks next" binds to the last authenticated handle/byte offset.
  $("console-command").value="tasks next";
  return $("console-run").click();
 });
 $("task-handle").addEventListener("input",()=>{
  if(taskWindow&&$("task-handle").value.trim()!==taskWindow.handle)clearTaskWindow();
  if(managedTaskReceipt&&$("task-handle").value.trim()!==managedTaskReceipt.handle){
   clearManagedTaskReceipt("Task handle đã thay đổi. Cần xác minh lại từ host.");
  }
 });

 function consoleCanonicalPath(path,allowDot=false){
  // Deliberately stricter than the Files UI: no spaces, quotes, Git
  // options or shell characters are accepted by this command grammar.
  if(path===".")return allowDot;
  return safeWorkspacePath(path)&&path.length<=120&&!path.startsWith("-")&&
   path.split("/").every(p=>/^[\p{L}\p{N}_-][\p{L}\p{N}_.-]*$/u.test(p)&&!p.startsWith("-"));
 }
 function consoleResolvePath(input,allowDot=false){
  if(input==="."&&allowDot)return consoleCwd;
  if(!consoleCanonicalPath(input,allowDot))return null;
  const resolved=consoleCwd==="."?input:consoleCwd+"/"+input;
  return consoleCanonicalPath(resolved,allowDot)?resolved:null;
 }
 function consoleNavigate(directory){
  if(directory===".."){
   const slash=consoleCwd.lastIndexOf("/");
   consoleCwd=consoleCwd==="."?".":slash<0?".":consoleCwd.slice(0,slash);
  }else if(directory!=="."){
   const next=consoleResolvePath(directory,false);
   if(!next)return false;
   consoleCwd=next;
  }
  $("console-location").textContent="Workspace: /"+
   (consoleCwd==="."?"":consoleCwd)+" · chỉ đổi vị trí cục bộ; dùng ls kiểm tra host";
  return true;
 }
 function parseSafeReadCommand(raw){
  if(typeof raw!=="string"||raw.length>180||raw!==raw.trim()||!raw)return null;
  if(raw==="help")return {help:true};
  if(raw==="clear")return {clear:true};
  if(raw==="pwd")return {pwd:true};
  if(raw==="git conflicts")return {gitConflicts:true};
  if(raw==="git tracking")return {gitTracking:true};
  // Cursor aliases are strictly local selectors derived from verified
  // E2EE page headers; never trust a user-supplied next byte or filename.
  if(raw==="files next"){
   return filePage&&!sessionEnding&&filePage.next>0&&
    $("file").value.trim()===filePage.path&&
    $("line").value===String(filePage.next)?
    {operation:"file_lines",path:filePage.path,offset:filePage.next}:null;
  }
  if(raw==="tasks next"){
   return taskWindow&&!sessionEnding&&!taskWindow.eof&&
    $("task-handle").value.trim()===taskWindow.handle&&
    $("task-offset").value===String(taskWindow.next)?
    {operation:"task_output",handle:taskWindow.handle,offset:taskWindow.next}:null;
  }
  if(raw==="ls")return {operation:"files_list",path:consoleCwd};
  if(raw.startsWith("ls ")){
   const path=consoleResolvePath(raw.slice(3),true);
   return path?{operation:"files_list",path}:null;
  }
  if(raw==="cd ."||raw==="cd ..")return {navigate:raw.slice(3)};
  if(raw.startsWith("cd ")){
   const path=raw.slice(3);
   return consoleCanonicalPath(path,false)?{navigate:path}:null;
  }
  // Review only: a separate Git panel confirmation is required to WRITE.
  if(raw==="git review-commit")
   return {operation:"git_write_review",mode:"commit",path:""};
  for(const [prefix,mode] of [
   ["git review-stage ","stage"],["git review-unstage ","unstage"]
  ]){
   if(raw.startsWith(prefix)){
    const path=consoleResolvePath(raw.slice(prefix.length),false);
    return path?{operation:"git_write_review",mode,path}:null;
   }
  }
  for(const [prefix,mode] of [
   ["git review-create-branch ","branch-create"],
   ["git review-switch-branch ","branch-switch"],
   ["git review-delete-branch ","branch-delete"]
  ]){
   if(raw.startsWith(prefix)){
    const slug=raw.slice(prefix.length);
    return /^[a-z][a-z0-9-]{1,31}$/.test(slug)&&!slug.endsWith("-")?
     {operation:"git_write_review",mode,path:slug}:null;
   }
  }
  const exact={
   "git status":{operation:"git_status"},
   "git log":{operation:"git_log"},
   "git branches":{operation:"git_branches"},
   "git remotes":{operation:"git_remotes"},
   "git tracking-refs":{operation:"git_tracking_refs"},
   "tasks list":{operation:"tasks"}
  };
  if(Object.hasOwn(exact,raw))return exact[raw];
  for(const [prefix,operation,allowDot] of [
   ["files list ","files_list",true],
   ["files view ","file_view",false],
   ["cat ","file_view",false],
   ["git diff ","git_diff",false],
   ["git staged ","git_staged",false],
   ["git head ","git_head",false],
   ["git review ","git_review",false]
  ]){
   if(raw.startsWith(prefix)){
    const path=raw.slice(prefix.length);
    const resolved=consoleResolvePath(path,allowDot);
    return resolved?{operation,path:resolved}:null;
   }
  }
  const line=/^files lines ([^\s]+) ([1-9][0-9]*)$/u.exec(raw);
  if(line){
   const offset=Number(line[2]);
   const path=consoleResolvePath(line[1],false);
   if(path&&Number.isSafeInteger(offset)&&
    offset<=2097152)return {operation:"file_lines",path,offset};
  }
  const find=/^(?:files find|grep -F) ([^\s]+) ([^\r\n]+)$/u.exec(raw);
  if(find){
   const literal=find[2];
   const path=consoleResolvePath(find[1],false);
   if(path&&literal===literal.trim()&&
    new TextEncoder().encode(literal).length<=96&&
    !/[;&|$<>\x60"'()[\]{}!\\\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(literal))
    return {operation:"file_find_lines",path,literal};
  }
  // TaskDeck reviewed work never maps to shell or automatic execution.
  const taskReview=/^tasks review-(run|start) ([1-9][0-9]*)$/u.exec(raw);
  if(taskReview){
   const taskID=Number(taskReview[2]);
   if(Number.isSafeInteger(taskID)&&taskID<=2147483647)
    return {operation:"task_review",mode:taskReview[1],task_id:taskID};
  }
  const status=/^tasks status ([0-9a-f]{32})$/u.exec(raw);
  if(status)return {operation:"task_status",handle:status[1]};
  const output=/^tasks output ([0-9a-f]{32}) (0|[1-9][0-9]*)$/u.exec(raw);
  if(output){
   const offset=Number(output[2]);
   if(Number.isSafeInteger(offset)&&offset<=1073741824)
    return {operation:"task_output",handle:output[1],offset};
  }
  return null;
 }


 function renderConsoleCommandHistory(){
  // Human-typed commands that passed parseSafeReadCommand, never remote text.
  const target=$("console-history-entries"),generation=++consoleHistoryGeneration;
  target.replaceChildren();
  const query=$("console-history-filter").value;
  if(query!==query.trim()||new TextEncoder().encode(query).length>96||
   /[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(query)){
   $("console-history-count").textContent="Lọc không hợp lệ";
   target.textContent="Từ khóa không hợp lệ; không tạo nút chọn lệnh.";
   return;
  }
  const matching=[...consoleHistory].reverse().filter(command=>
   !query||command.toLowerCase().includes(query.toLowerCase()));
  $("console-history-count").textContent=matching.length+"/"+consoleHistory.length+
   " lệnh gần đây · chỉ tại máy";
  if(!matching.length){
   target.textContent="Không có lệnh phù hợp trong lịch sử RAM hiện tại.";
   return;
  }
  for(const command of matching){
   const button=document.createElement("button");
   button.type="button";button.className="git-status-card";
   button.textContent="$ "+command;
   button.addEventListener("click",()=>{
    if(sessionEnding||busy||generation!==consoleHistoryGeneration||
     !consoleHistory.includes(command)){
     $("status").textContent="Lệnh lịch sử cũ; chọn lại để tránh nhầm lẫn.";
     return;
    }
    // Choosing a command only fills an input, NEVER dispatches it.
    $("console-command").value=command;
    consoleHistoryCursor=consoleHistory.length;
    $("status").textContent="Đã chọn lệnh từ lịch sử · nhấn Enter hoặc Đọc để yêu cầu riêng";
   });
   target.append(button);
  }
 }
 $("console-history-filter").addEventListener("input",renderConsoleCommandHistory);

 function renderConsoleTranscript(){
  // Search only the last four text-only, bounded results already in RAM.
  // Never construct a new remote action from an untrusted search string.
  const query=$("console-transcript-search").value;
  if(query!==query.trim()||new TextEncoder().encode(query).length>96||
   /[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(query)){
   $("console-transcript").textContent="Từ khóa không hợp lệ; chỉ tìm nguyên văn UTF-8 tối đa 96 byte.";
   $("console-search-count").textContent="Bộ lọc không hợp lệ";
   return;
  }
  const matching=query?consoleRecords.filter(record=>
   record.toLowerCase().includes(query.toLowerCase())):consoleRecords;
  $("console-search-count").textContent=matching.length+"/"+consoleRecords.length+
   " kết quả có trong RAM"+(query?" · lọc tại máy":"");
  $("console-transcript").textContent=matching.length?
   matching.join("\n\n"):query?
   "Không có kết quả khớp trong tối đa 4 bản ghi Console gần nhất.":
   "Không có kết quả Console trong phiên này.";
 }
 $("console-transcript-search").addEventListener("input",renderConsoleTranscript);

 function consoleWrite(command,output){
  const safe=String(output).replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/gu,
   c=>"\\u"+c.codePointAt(0).toString(16).padStart(4,"0"));
  // Last four bounded responses only; never retain a full remote transcript,
  // raw terminal escape streams, app credentials or non-text payloads.
  const prefix="$ "+command+"\n";
  // A two-way Git review can be larger than one 4KiB read. Do not
  // silently truncate a potentially security-relevant review in console.
  // The exact authenticated body remains in the separate result pane.
  const chunk=prefix+(safe.length>4500-prefix.length?
   "[Kết quả lớn hơn giới hạn console. Đọc toàn bộ nội dung đã xác thực tại khung kết quả chính; không hiển thị bản cắt cụt.]":safe);
  consoleRecords.push(chunk);
  if(consoleRecords.length>4)consoleRecords.shift();
  renderConsoleTranscript();
  $("console-count").textContent=consoleRecords.length+" kết quả gần nhất";
 }
 function quota(){
  $("quota").textContent=used+" / "+max+" E2EE actions";
  if(used>=max)$("status").textContent="Hết lượt E2EE · đăng nhập lại để tiếp tục";
  showGitReviewQueue(); // quota may change while a reviewed queue is displayed
 }
 function managedTaskBudgetAllows(input){
  if(!input||!managedTaskReceipt||
   !["running","cancel_requested"].includes(managedTaskReceipt.state))return true;
  const remaining=max-used;
  const cost=input.operation==="git_review"?2:1;
  if(managedTaskReceipt.state==="running"&&remaining-cost<2){
   return input.operation==="task_cancel"&&input.handle===managedTaskReceipt.handle;
  }
  if(managedTaskReceipt.state==="cancel_requested"&&remaining-cost<1){
   return input.operation==="task_status"&&input.handle===managedTaskReceipt.handle;
  }
  return true;
 }
 function setBusy(next){
  busy=next;
  document.querySelectorAll("[data-op],#execute-task,#cancel-task,#execute-git,#execute-file-edit,#execute-line-edit,#task-read-next,#file-read-next,#console-run,#git-queue-review-stage,#git-queue-review-unstage").forEach(b=>{
   const input=b.id==="cancel-task"?{operation:"task_cancel",handle:$("task-handle").value.trim()}:
    b.dataset.op?{operation:b.dataset.op,handle:$("task-handle").value.trim()}:
    {operation:"other"};
   const reviewBudget=b.dataset.op==="task_review"?
    (b.dataset.mode==="start"?4:2):0;
   b.disabled=sessionEnding||next||(used>=max&&b.id!=="console-run")||
    (reviewBudget>0&&used+reviewBudget>max)||
    !managedTaskBudgetAllows(input)||
    (b.id==="task-read-next"&&(!taskWindow||taskWindow.eof))||
    (b.id==="file-read-next"&&(!filePage||filePage.next===0))||
    (b.id==="git-queue-review-stage"&&(used+2>max||!queuedGitWriteTarget("stage")))||
    (b.id==="git-queue-review-unstage"&&(used+2>max||!queuedGitWriteTarget("unstage")))||
    (["git_write_review","file_edit_review","line_write_review","git_review"].includes(b.dataset.op)&&used+2>max);
  });
  $("task-verify-stop").disabled=next||sessionEnding||used>=max||
   !managedTaskReceipt||managedTaskReceipt.state!=="cancel_requested"||
   $("task-handle").value.trim()!==managedTaskReceipt.handle;
  showGitReviewQueue(); // refresh queue shortcuts after busy/quota transitions
 }
 async function call(input,closing=false){
  if(!closing&&(busy||used>=max||sessionEnding))return null;
  if(!closing&&input?.operation==="task_execute"&&input.mode==="start"&&used+3>max){
   notify("Không đủ lượt E2EE","Task cần 1 lượt Start, 1 lượt Cancel và 1 lượt xác minh trạng thái. Đăng nhập lại phiên native.");
   return null;
  }
  if(!closing&&!managedTaskBudgetAllows(input)){
   notify("Đã giữ lượt E2EE","Các lượt còn lại dành cho Cancel và xác minh trạng thái task. Không gửi yêu cầu mạng.");
   return null;
  }
  setBusy(true);$("status").textContent="Đang xác thực Agent…";
  try{
   const response=await fetch(closing?"/api/close":"/api/action",{
    method:"POST",mode:"same-origin",credentials:"omit",cache:"no-store",referrerPolicy:"no-referrer",
    headers:{"Content-Type":"application/json","X-TaskDeck-Local-Key":key},
    body:JSON.stringify(closing?{}:{operation:"",path:"",task_id:0,mode:"",confirmation:"",handle:"",offset:0,...input})
   });
   const data=await response.json();
   if(sessionEnding&&!closing)return null; // discard late E2EE data after local close
   if(data.used!==undefined){used=data.used;max=data.max;}
   if(!response.ok||!data.ok){notify("Bị từ chối",data.error||"Không có quyền thực thi.");quota();return null;}
   if(!closing)notify("Agent đã xác thực",data.text||"Hoàn tất");
   quota();return data;
  }catch(_){
   if(sessionEnding&&!closing)return null; // stale rejected read must not overwrite close status
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
 function acceptTaskReview(result,input){
  if(!result||!result.confirm)return false;
  if(Number($("task-id").value)!==input.task_id){
   notify("Review hết hiệu lực","Task ID đã đổi trong lúc xác thực; cần Review lại.");
   return false;
  }
  review={id:input.task_id,mode:input.mode,confirm:result.confirm};
  $("approval-text").textContent="Host đã phê duyệt. Nhập chính xác: "+result.confirm;
  $("approval-entry").value="";
  $("approval").classList.remove("hidden");
  return true;
 }
 function acceptGitWriteReview(result,input){
  if(!result||!result.confirm)return false;
  if(!gitTargetMatches(input.mode,input.path||"")){
   notify("Review hết hiệu lực","Đích Git đã đổi trong lúc xác thực; cần Review lại.");
   return false;
  }
  gitReview={mode:input.mode,path:input.path||"",confirm:result.confirm};
  $("git-confirm-text").textContent="Đọc đầy đủ preview ở khung kết quả, sau đó nhập chính xác: "+result.confirm;
  $("git-confirm-entry").value="";
  $("git-commit-message").value="";
  $("git-message-label").classList.toggle("hidden",input.mode!=="commit");
  $("git-approval").classList.remove("hidden");
  return true;
 }
 document.querySelectorAll("[data-op]").forEach(btn=>btn.addEventListener("click",async()=>{
  const operation=btn.dataset.op;const input={operation};
  // Match native-host review invalidation: a different read/action makes
  // every unrelated pending confirmation stale. Never display stale buttons.
  if(operation!=="git_write_review"){
   gitReview=null;$("git-approval").classList.add("hidden");
  }
  if(operation!=="file_edit_review"){
   fileReview=null;$("file-editor").classList.add("hidden");
  }
  if(operation!=="line_write_review"){
   lineReview=null;$("line-editor").classList.add("hidden");
  }
  if(operation!=="task_review"){
   review=null;$("approval").classList.add("hidden");
  }
  if(operation==="files_list")input.path=$("directory").value.trim();
  if(operation==="git_log"){
   gitLogSnapshot=[];
   $("git-log-entries").textContent="Đang xác thực Git log mới…";
   $("git-log-selection").textContent="Chưa chọn commit";
   $("git-log-count").textContent="Đang cập nhật";
  }
  if(operation==="git_tracking_refs"){
   $("git-tracking-entries").textContent="Đang xác thực tracking refs cục bộ qua E2EE…";
   $("git-tracking-count").textContent="Đang cập nhật";
  }
  if(operation==="git_remotes"){
   $("git-remotes-entries").textContent="Đang xác thực tên remotes từ Agent E2EE…";
   $("git-remotes-count").textContent="Đang cập nhật";
  }
  if(operation==="git_status"){
   gitStatusSnapshot=[];gitStatusAuthenticated=false;resetGitReviewQueue();
   $("git-upstream-summary").textContent="Tracking đang xác thực lại";
   $("git-next-conflict").disabled=true;
   clearGitConflictMarkers("Đang xác thực Git Status mới");
   $("git-status-files").textContent="Đang tải Git status mới…";
   $("git-status-count").textContent="Đang cập nhật";
  }
  if(operation==="file_view"||operation==="file_lines"||operation==="file_find_lines"){
   input.path=$("file").value.trim();
   if(operation==="file_lines")input.offset=Number($("line").value);
   if(operation==="file_find_lines"){
    $("find-entries").textContent="Đang xác thực kết quả tìm kiếm…";
    $("find-summary").textContent="Đang tìm";
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
  if(["git_diff","git_staged","git_head","git_review"].includes(operation)){
   input.path=$("git-path").value.trim();
   clearGitDiffPreview("Đang xác thực");
  }
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
   const reserve=input.mode==="start"?4:2;
   if(used+reserve>max){
    notify("Không đủ lượt E2EE",input.mode==="start"?
     "Chưa đủ 4 lượt cho Review, Start, Cancel và xác minh dừng. Đăng nhập phiên mới.":
     "Chưa đủ 2 lượt cho Review và Run. Đăng nhập phiên mới.");
    return;
   }
  }
  if(operation==="task_status"||operation==="task_output"){
   input.handle=$("task-handle").value.trim();
   if(operation==="task_output")input.offset=Number($("task-offset").value);
  }
  const result=await call(input);
  if(result&&operation==="file_lines"){
   acceptSourcePage(result.text||"",input.path,input.offset,false);
  }
  if(result&&operation==="file_find_lines"){renderFindMatches(result.text||"",input.path,input.literal);}
  if(result&&operation==="files_list"){renderExplorer(input.path,result.text||"");}
  if(result&&operation==="tasks"){renderTaskCatalog(result.text||"");}
  if(!result&&operation==="git_tracking_refs"){
   $("git-tracking-entries").textContent="Không tải được local tracking refs; đã xóa dữ liệu cũ.";
   $("git-tracking-count").textContent="Không tải được";
  }
  if(!result&&operation==="git_remotes"){
   $("git-remotes-entries").textContent="Không tải được danh sách remote; dữ liệu cũ đã xóa.";
   $("git-remotes-count").textContent="Không tải được";
  }
  if(!result&&operation==="git_status"){
   $("git-upstream-summary").textContent="Không tải được Git tracking từ Agent";
   $("git-status-count").textContent="Không tải được";
  }
  if(result&&operation==="git_status"){renderGitStatus(result.text||"");}
  if(result&&operation==="git_log"){renderGitLog(result.text||"");}
  if(result&&["git_diff","git_staged","git_head","git_review"].includes(operation)){
   renderGitDiffPreview(result.text||"",operation,input.path);
  }
  if(result&&operation==="git_branches"){renderGitBranches(result.text||"");}
  if(result&&operation==="git_remotes"){renderGitRemotes(result.text||"");}
  if(result&&operation==="git_tracking_refs"){renderGitTrackingRefs(result.text||"");}
  if(result&&operation==="line_write_review"&&result.confirm&&
   ($("file").value.trim()!==input.path||Number($("line").value)!==input.offset)){
   notify("Review hết hiệu lực","Đích File/dòng đã đổi trong lúc xác thực; cần Review lại.");
  }else if(result&&operation==="line_write_review"&&result.confirm){
   lineReview={path:input.path,mode:input.mode,offset:input.offset,confirm:result.confirm,original:result.line||""};
   $("line-edit-title").textContent="Đã xác thực file và dòng "+input.offset+" · "+input.mode;
   $("line-edit-source").textContent="Nội dung hiện tại: "+JSON.stringify(lineReview.original);
   $("line-edit-text").disabled=input.mode==="delete";
   $("line-edit-text").value=input.mode==="edit"?lineReview.original:"";
   $("line-edit-confirm").value="";
   $("line-edit-approval").textContent="Xem đầy đủ trang file ở khung kết quả rồi nhập: "+result.confirm;
   $("line-editor").classList.remove("hidden");
  }
  if(result&&operation==="file_edit_review"&&result.confirm&&
   ($("file").value.trim()!==input.path)){
   notify("Review hết hiệu lực","Đích File đã đổi trong lúc xác thực; cần Review lại.");
  }else if(result&&operation==="file_edit_review"&&result.confirm){
   fileReview={path:input.path,confirm:result.confirm,original:result.text||""};
   $("file-edit-text").value=fileReview.original;
   $("file-edit-confirm").value="";
   $("file-edit-approval").textContent="Đọc và xác nhận bằng chuỗi: "+result.confirm;
   $("file-editor").classList.remove("hidden");
  }
  if(operation==="git_write_review")acceptGitWriteReview(result,input);
  if(operation==="task_review")acceptTaskReview(result,input);
  if(result&&operation==="task_output"){
   acceptTaskWindow(result.text||"",input.handle,input.offset);
  }
  if(operation==="task_status"){
   if(result)presentManagedTaskReceipt(result.text||"",input.handle,operation);
   else clearManagedTaskReceipt("Không xác minh được trạng thái. Không coi task đã dừng.");
  }
 }));
 $("console-run").addEventListener("click",async()=>{
  if(busy||sessionEnding)return;
  const raw=$("console-command").value;
  const parsed=parseSafeReadCommand(raw);
  if(!parsed){
   notify("Lệnh bị từ chối","Không có lệnh trong allowlist read-only. Gõ help để xem cú pháp.");
   return;
  }
  if(consoleHistory.at(-1)!==raw){
   consoleHistory.push(raw);
   if(consoleHistory.length>16)consoleHistory.shift();
  }
  consoleHistoryCursor=consoleHistory.length;
  renderConsoleCommandHistory();
  // A console read changes review context, so never leave a write approval
  // visible when opening its output. Native Go also drops pending reviews
  // before dispatching any different typed action.
  gitReview=null;$("git-approval").classList.add("hidden");
  fileReview=null;$("file-editor").classList.add("hidden");
  lineReview=null;$("line-editor").classList.add("hidden");
  review=null;$("approval").classList.add("hidden");
  if(parsed.clear){
   consoleRecords=[];consoleHistory=[];consoleHistoryCursor=0;
   $("console-history-filter").value="";
   renderConsoleCommandHistory();
   $("console-transcript-search").value="";
   $("console-search-count").textContent="0/0 kết quả";
   $("console-transcript").textContent="Đã xóa kết quả hiển thị trong phiên hiện tại.";
   $("console-count").textContent="0 kết quả";
   notify("Đã xóa lịch sử hiển thị","Không gửi yêu cầu mạng và không thay đổi dữ liệu trên host.");
   return;
  }
  if(parsed.gitConflicts||parsed.gitTracking){
   if(!gitStatusAuthenticated){
    notify("Chưa xác thực Git Status","Bấm Status hoặc chạy git status trước khi xem tracking/conflict cục bộ.");
    return;
   }
   const output=parsed.gitTracking?$("git-upstream-summary").textContent:
    gitStatusSnapshot.filter(e=>e.conflict).map(e=>e.status+" "+e.path).join("\n")||
     "Không có file conflict hợp lệ trong Git Status đã xác thực.";
   consoleWrite(raw,output);
   notify("Git Status cục bộ đã xác thực","Dữ liệu từ lần Status gần nhất; không gọi Agent hay thực thi lệnh.");
   return;
  }
  if(parsed.pwd){
   const current="/"+(consoleCwd==="."?"":consoleCwd);
   consoleWrite("pwd",current);
   notify("Thư mục cục bộ",current+" (workspace-relative; không truy vấn host).");
   return;
  }
  if(Object.hasOwn(parsed,"navigate")){
   if(!consoleNavigate(parsed.navigate)){
    notify("Đường dẫn bị từ chối","Không thể chuyển ra ngoài workspace.");return;
   }
   consoleWrite(raw,"Thư mục cục bộ: /"+(consoleCwd==="."?"":consoleCwd)+
    " · dùng ls để kiểm tra trên host.");
   notify("Đã đổi thư mục cục bộ","Không gửi lệnh cd tới host.");
   return;
  }
  if(parsed.help){
   const help="Allowlist: git status | git log | git branches | tasks list | "+
    "files list/view <path> | files lines <path> <line> | files find <path> <literal> | "+
    "git diff/staged/head/review <path> | cat <file> | grep -F <file> <literal> | "+
    "git conflicts | git tracking | "+
    "git review-stage/review-unstage <path> | git review-commit | "+
    "git review-create-branch/review-switch-branch/review-delete-branch <slug> | "+
    "pwd | cd <dir> | cd .. | ls [dir] | clear | "+
    "tasks review-run/review-start <id> | tasks status <32 lowercase hex> | "+
    "tasks output <32 lowercase hex> <offset> | files next | tasks next. "+
    "Git write chỉ thực thi sau xác nhận ở tab Git. Không có shell, PTY hoặc script.";
   consoleWrite("help",help);
   notify("Hướng dẫn read-only",help);
   return;
  }
  const requiredReviewRounds=parsed.operation==="task_review"?
   (parsed.mode==="start"?4:2):
   (["git_review","git_write_review"].includes(parsed.operation)?2:1);
  if(used+requiredReviewRounds>max){
   notify("Hết lượt E2EE",requiredReviewRounds>1?
    (requiredReviewRounds===4?
     "Managed Start cần 4 lượt E2EE cho Review, Start, Cancel và Status. Đăng nhập lại.":
     "Review cần đủ hai lượt E2EE, bao gồm thao tác xác nhận riêng. Đăng nhập lại."):
    "Đã hết lượt E2EE; chỉ còn lệnh cục bộ help, clear, pwd, cd.");
   return;
  }
  if(parsed.operation==="task_review"){
   // Select only a reviewed task in GUI; never auto-run it from Console.
   $("task-id").value=String(parsed.task_id);
   invalidateChangedReviews();
   review=null;$("approval").classList.add("hidden");
   $("approval-entry").value="";
  }
  if(parsed.operation==="git_write_review"){
   // Choose the reviewed target on screen, but NEVER dispatch execute here.
   if(["stage","unstage"].includes(parsed.mode)){
    $("git-path").value=parsed.path;
    clearGitDiffPreview("Review Git write mới");
   }else if(parsed.mode.startsWith("branch-")){
    $("git-branch").value=parsed.path;
   }
   invalidateChangedReviews();
   gitReview=null;$("git-approval").classList.add("hidden");
   $("git-confirm-entry").value="";
  }
  if(["git_diff","git_staged","git_head","git_review"].includes(parsed.operation)){
   // Console has already validated the workspace-relative path, but the
   // GUI binds any clickable preview to the selected target as well.
   $("git-path").value=parsed.path;
   clearGitDiffPreview("Đang xác thực diff từ Console…");
  }
  const verifiedFileNext=raw==="files next";
  if(["file_lines","file_find_lines"].includes(parsed.operation)){
   $("file").value=parsed.path;
   if(parsed.operation==="file_lines"){
    $("line").value=String(parsed.offset);
    if(!verifiedFileNext)clearSourcePages();
   }else{
    $("find-literal").value=parsed.literal;
    $("find-entries").textContent="Đang xác thực kết quả từ Console…";
    $("find-summary").textContent="Đang tìm";
   }
   invalidateChangedReviews();
  }
  if(parsed.operation==="task_output"){
   // Pin the task's handle/cursor for the verified reply and reject races.
   $("task-handle").value=parsed.handle;
   $("task-offset").value=String(parsed.offset);
   if(!taskWindow||taskWindow.handle!==parsed.handle||
    taskWindow.next!==parsed.offset)clearTaskWindow();
  }else if(parsed.operation==="task_status"){
   $("task-handle").value=parsed.handle;
  }
  const selections={directory:$("directory").value,gitPath:$("git-path").value,
   filePath:$("file").value,findLiteral:$("find-literal").value,
   sourceLine:$("line").value,taskHandle:$("task-handle").value,
   taskOffset:$("task-offset").value};
  if(parsed.operation==="task_output"){
   $("task-output-progress").textContent="Đang đọc cửa sổ task từ Agent…";
  }
  if(parsed.operation==="files_list"){
   $("workspace-entries").textContent="Đang xác thực danh sách thư mục qua Console…";
   $("explorer-location").textContent="Đang xác thực "+parsed.path;
  }else if(parsed.operation==="tasks"){
   $("task-catalog").textContent="Đang xác thực danh sách TaskDeck qua Console…";
   $("task-catalog-count").textContent="Đang cập nhật";
  }else if(["git_diff","git_staged","git_head","git_review"].includes(parsed.operation)){
   clearGitDiffPreview("Đang xác thực");
  }
  // Clear old Git cards before fetching a new authenticated snapshot.
  // On transport failure the GUI must not present stale status as fresh.
  if(parsed.operation==="git_status"){
   gitStatusSnapshot=[];gitStatusAuthenticated=false;resetGitReviewQueue();
   $("git-upstream-summary").textContent="Tracking đang xác thực lại";
   $("git-next-conflict").disabled=true;
   clearGitConflictMarkers("Đang xác thực Git Status qua Console");
   $("git-status-files").textContent="Đang xác thực Git Status qua Console…";
   $("git-status-count").textContent="Đang cập nhật";
  }else if(parsed.operation==="git_log"){
   gitLogSnapshot=[];
   $("git-log-entries").textContent="Đang xác thực Git Log qua Console…";
   $("git-log-selection").textContent="Chưa chọn commit";
   $("git-log-count").textContent="Đang cập nhật";
  }else if(parsed.operation==="git_branches"){
   $("git-branch-entries").textContent="Đang xác thực Git Branches qua Console…";
   $("git-branch-count").textContent="Đang cập nhật";
  }else if(parsed.operation==="git_tracking_refs"){
   $("git-tracking-entries").textContent="Đang xác thực tracking refs cục bộ qua E2EE…";
   $("git-tracking-count").textContent="Đang cập nhật";
  }else if(parsed.operation==="git_remotes"){
   $("git-remotes-entries").textContent="Đang xác thực tên Git remotes qua E2EE…";
   $("git-remotes-count").textContent="Đang cập nhật";
  }
  const result=await call(parsed);
  if(!result){
   if(!sessionEnding){
    if(parsed.operation==="git_status"){
     $("git-status-count").textContent="Không tải được";
     $("git-upstream-summary").textContent="Không tải được tracking · không dùng số cũ";
    }
    if(parsed.operation==="git_log")$("git-log-count").textContent="Không tải được";
    if(parsed.operation==="git_branches")$("git-branch-count").textContent="Không tải được";
    if(parsed.operation==="git_tracking_refs"){
     $("git-tracking-entries").textContent="Không tải được tracking refs";
     $("git-tracking-count").textContent="Không tải được";
    }
    if(parsed.operation==="git_remotes"){
     $("git-remotes-entries").textContent="Không tải được danh sách remote.";
     $("git-remotes-count").textContent="Không tải được";
    }
    if(parsed.operation==="files_list"){
     $("workspace-entries").textContent="Không lấy được danh sách thư mục từ Agent.";
     $("explorer-location").textContent="Không tải được";
    }
    if(parsed.operation==="tasks"){
     $("task-catalog").textContent="Không lấy được danh sách task từ Agent.";
     $("task-catalog-count").textContent="Không tải được";
    }
    if(parsed.operation==="task_output")clearTaskWindow();
    if(parsed.operation==="file_lines")clearSourcePages();
    if(parsed.operation==="file_find_lines"){
     $("find-entries").textContent="Không tải được tìm kiếm từ Agent.";
     $("find-summary").textContent="Không tải được";
    }
    if(["git_diff","git_staged","git_head","git_review"].includes(parsed.operation))
     clearGitDiffPreview("Không tải được");
   }
   return;
  }
  if(parsed.operation==="task_review"&&acceptTaskReview(result,parsed)){
   $("status").textContent="Task review sẵn sàng · vào tab Tasks, nhập xác nhận và bấm Thực thi";
  }
  if(parsed.operation==="task_status"){
   if(result)presentManagedTaskReceipt(result.text||"",parsed.handle,"task_status");
   else clearManagedTaskReceipt("Không xác minh được trạng thái task qua Console.");
  }
  if(parsed.operation==="git_write_review"&&acceptGitWriteReview(result,parsed)){
   $("status").textContent="Git review sẵn sàng · vào tab Git, đọc kết quả và nhập xác nhận để ghi";
  }
  // Reuse the SAME authenticated Agent response to update read-only Git
  // cards. No second E2EE request, no browser-to-PHP input and no Git write.
  if(parsed.operation==="git_status")renderGitStatus(result.text||"");
  else if(parsed.operation==="git_log")renderGitLog(result.text||"");
  else if(parsed.operation==="git_branches")renderGitBranches(result.text||"");
  else if(parsed.operation==="git_remotes")renderGitRemotes(result.text||"");
  else if(parsed.operation==="git_tracking_refs")renderGitTrackingRefs(result.text||"");
  else if(parsed.operation==="files_list"){
   if($("directory").value!==selections.directory){
    $("workspace-entries").textContent="Vị trí Files đã thay đổi khi đọc; cần xác thực lại.";
    $("explorer-location").textContent="Danh sách đã hết hiệu lực";
   }else{
    // Same authenticated data as Console; no second host read.
    $("directory").value=parsed.path;
    renderExplorer(parsed.path,result.text||"");
   }
  }else if(parsed.operation==="tasks")renderTaskCatalog(result.text||"");
  else if(parsed.operation==="task_output"){
   if($("task-handle").value!==selections.taskHandle||
    $("task-offset").value!==selections.taskOffset){
    clearTaskWindow();
    notify("Output hết hiệu lực","Handle hoặc offset đã đổi trong lúc Agent đọc; cần yêu cầu lại.");
   }else{
    acceptTaskWindow(result.text||"",parsed.handle,parsed.offset);
   }
  }
  else if(parsed.operation==="file_lines"){
   if($("file").value!==selections.filePath||
    $("line").value!==selections.sourceLine){
    clearSourcePages();
    notify("Trang file hết hiệu lực","File hoặc dòng đã đổi trong lúc Agent đọc; cần xem lại.");
   }else{
    acceptSourcePage(result.text||"",parsed.path,parsed.offset,verifiedFileNext);
   }
  }
  else if(parsed.operation==="file_find_lines"){
   if($("file").value!==selections.filePath||
    $("find-literal").value!==selections.findLiteral){
    $("find-entries").textContent="Tệp hoặc từ khóa đã thay đổi; cần tìm lại.";
    $("find-summary").textContent="Kết quả đã hết hiệu lực";
   }else{
    renderFindMatches(result.text||"",parsed.path,parsed.literal);
   }
  }
  else if(["git_diff","git_staged","git_head","git_review"].includes(parsed.operation)){
   if($("git-path").value!==selections.gitPath){
    clearGitDiffPreview("Đường dẫn đã đổi");
   }else{
    $("git-path").value=parsed.path;
    renderGitDiffPreview(result.text||"",parsed.operation,parsed.path);
   }
  }
  // Keep the main authenticated output unmodified and present the exact
  // same text as a bounded, escaped local console record.
  consoleWrite(raw,result.text||"");
 });
 const consoleCommandCompletions=[
  "help","clear","pwd","ls","cd ","files list ","files view ",
  "files lines ","files find ","files next","cat ","grep -F ",
  "git status","git tracking","git conflicts","git log","git branches","git diff ","git staged ",
  "git head ","git review ","git review-stage ","git review-unstage ",
  "git review-commit","git review-create-branch ",
  "git review-switch-branch ","git review-delete-branch ",
  "tasks list","tasks review-run ","tasks review-start ",
  "tasks status ","tasks output ","tasks next"
 ];
 function completeSafeConsoleCommand(){
  const raw=$("console-command").value;
  // Complete only hard-coded verbs, never previously seen untrusted paths,
  // executable names, Git argv, task IDs or a remote-host suggestion.
  if(!raw||raw.length>180||/[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(raw)){
   $("status").textContent="Không thể gợi ý cú pháp không hợp lệ";return;
  }
  const choices=consoleCommandCompletions.filter(x=>x.startsWith(raw));
  if(!choices.length){
   $("status").textContent="Không có lệnh cố định phù hợp; gõ help.";return;
  }
  let prefix=choices[0];
  for(const word of choices.slice(1)){
   let n=0;
   while(n<prefix.length&&n<word.length&&prefix[n]===word[n])n++;
   prefix=prefix.slice(0,n);
  }
  if(prefix.length>raw.length&&prefix.length<=180){
   $("console-command").value=prefix;
   $("status").textContent="Đã hoàn thành cú pháp cục bộ · không gửi yêu cầu Agent";
  }else{
   $("status").textContent="Cú pháp có sẵn: "+choices.slice(0,6).join(", ")+
    (choices.length>6?" · và "+(choices.length-6)+" lệnh khác":"");
  }
 }
 $("console-command").addEventListener("keydown",event=>{
  if(busy||event.repeat)return;
  if(event.key==="Tab"){
   event.preventDefault();
   completeSafeConsoleCommand();return;
  }
  if(event.key==="ArrowUp"&&consoleHistory.length){
   event.preventDefault();
   consoleHistoryCursor=Math.max(0,consoleHistoryCursor-1);
   $("console-command").value=consoleHistory[consoleHistoryCursor];
   return;
  }
  if(event.key==="ArrowDown"&&consoleHistory.length){
   event.preventDefault();
   consoleHistoryCursor=Math.min(consoleHistory.length,consoleHistoryCursor+1);
   $("console-command").value=consoleHistoryCursor===consoleHistory.length?
    "":consoleHistory[consoleHistoryCursor];
   return;
  }
  if(event.key!=="Enter"||sessionEnding)return;
  event.preventDefault();
  $("console-run").click();
 });
 $("execute-task").addEventListener("click",async()=>{
  if(!review||busy)return;
  const pending=review;review=null;$("approval").classList.add("hidden");
  if(Number($("task-id").value)!==pending.id){
   notify("Review hết hiệu lực","Task ID đã đổi; không gửi lệnh chạy task.");return;
  }
  if($("approval-entry").value!==pending.confirm){
   notify("Đã hủy","Chuỗi xác nhận sai. Không thực thi task.");return;
  }
  invalidateAfterPossibleHostMutation("thực thi task");
  const result=await call({operation:"task_execute",task_id:pending.id,mode:pending.mode,confirmation:pending.confirm});
  if(result&&pending.mode==="start"){
   const match=/^TASK_HANDLE ([0-9a-f]{32})\n/u.exec(result.text||"");
   if(match){
    $("task-handle").value=match[1];
    presentManagedTaskReceipt(result.text||"",match[1],"task_execute");
   }else clearManagedTaskReceipt("Không nhận được task handle được xác thực.");
  }else if(result&&pending.mode==="run"){
   presentRunTaskReceipt(result.text||"");
  }else if(!result){
   clearManagedTaskReceipt("Không xác nhận được kết quả task; không tự chạy lại.");
  }
 });
 $("execute-line-edit").addEventListener("click",async()=>{
  if(!lineReview||busy)return;
  const pending=lineReview;
  lineReview=null;
  $("line-editor").classList.add("hidden");
  if($("file").value.trim()!==pending.path||Number($("line").value)!==pending.offset){
   notify("Review hết hiệu lực","File hoặc dòng đã đổi; không gửi lệnh sửa.");return;
  }
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
  invalidateAfterPossibleHostMutation("sửa dòng");
  await call({operation:"line_write_execute",path:pending.path,mode:pending.mode,
   offset:pending.offset,confirmation:pending.confirm,content});
 });
 $("execute-file-edit").addEventListener("click",async()=>{
  if(!fileReview||busy)return;
  const pending=fileReview;
  fileReview=null;
  $("file-editor").classList.add("hidden");
  if($("file").value.trim()!==pending.path){
   notify("Review hết hiệu lực","File đã đổi; không gửi lệnh ghi.");return;
  }
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
  invalidateAfterPossibleHostMutation("sửa file");
  await call({operation:"file_edit_execute",path:pending.path,
   confirmation:pending.confirm,content});
 });
 $("execute-git").addEventListener("click",async()=>{
  if(!gitReview||busy)return;
  const pending=gitReview;
  gitReview=null;$("git-approval").classList.add("hidden");
  if(!gitTargetMatches(pending.mode,pending.path)){
   notify("Review hết hiệu lực","File hoặc nhánh Git đã đổi; không gửi lệnh ghi.");return;
  }
  if($("git-confirm-entry").value!==pending.confirm){
   notify("Đã hủy","Không gửi lệnh ghi Git: chuỗi xác nhận không khớp.");return;
  }
  const message=pending.mode==="commit"?$("git-commit-message").value:"";
  if(pending.mode==="commit"&&(!/^[\x20-\x7e]{3,96}$/.test(message)||message.trim()!==message)){
   notify("Không hợp lệ","Commit message phải là ASCII in được, 3–96 ký tự, không có khoảng trắng đầu/cuối.");return;
  }
  invalidateAfterPossibleHostMutation("ghi Git");
  await call({operation:"git_write_execute",mode:pending.mode,
   path:pending.path,confirmation:pending.confirm,message});
 });
 $("file-read-next").addEventListener("click",async()=>{
  if(busy||!filePage||filePage.next===0||used>=max)return;
  const {path,next}=filePage;
  if($("file").value.trim()!==path){
   clearSourcePages();
   notify("File đã thay đổi","Chọn lại file và đọc từ đầu để tránh nhầm nguồn.");return;
  }
  gitReview=null;$("git-approval").classList.add("hidden");
  fileReview=null;$("file-editor").classList.add("hidden");
  lineReview=null;$("line-editor").classList.add("hidden");
  review=null;$("approval").classList.add("hidden");
  const result=await call({operation:"file_lines",path,offset:next});
  if(result)acceptSourcePage(result.text||"",path,next,true);
 });
 $("task-read-next").addEventListener("click",async()=>{
  if(busy||!taskWindow||taskWindow.eof||used>=max)return;
  const {handle,next}=taskWindow;
  if($("task-handle").value.trim()!==handle){
   clearTaskWindow();
   notify("Task đã thay đổi","Đọc output từ đầu với task handle mới.");return;
  }
  // The next offset is exclusively the last verified E2EE window cursor,
  // not an untrusted external URL, filename, command or user-supplied arg.
  gitReview=null;$("git-approval").classList.add("hidden");
  fileReview=null;$("file-editor").classList.add("hidden");
  lineReview=null;$("line-editor").classList.add("hidden");
  review=null;$("approval").classList.add("hidden");
  const result=await call({operation:"task_output",handle,offset:next});
  if(result)acceptTaskWindow(result.text||"",handle,next);
 });
 $("cancel-task").addEventListener("click",async()=>{
  if(busy||sessionEnding||used>=max)return;
  const handle=$("task-handle").value.trim();
  if(!/^[0-9a-f]{32}$/.test(handle)){notify("Lỗi dữ liệu","Handle không hợp lệ.");return;}
  const words="CANCEL TASK "+handle.slice(0,12);
  if(prompt("Để yêu cầu dừng task, nhập: "+words)!==words){
   notify("Đã hủy","Không gửi yêu cầu dừng.");return;
  }
  clearManagedTaskReceipt("Đang yêu cầu hủy; chưa có xác nhận từ host.");
  invalidateAfterPossibleHostMutation("hủy task");
  const result=await call({operation:"task_cancel",handle,confirmation:words});
  if(result)presentManagedTaskReceipt(result.text||"",handle,"task_cancel");
  else clearManagedTaskReceipt("Yêu cầu hủy chưa xác nhận. Task có thể vẫn đang chạy.");
 });
 $("task-verify-stop").addEventListener("click",async()=>{
  const receipt=managedTaskReceipt;
  if(!receipt||receipt.state!=="cancel_requested"||busy||sessionEnding||
   used>=max||$("task-handle").value.trim()!==receipt.handle)return;
  $("task-verify-stop").disabled=true;
  const result=await call({operation:"task_status",handle:receipt.handle});
  if(result)presentManagedTaskReceipt(result.text||"",receipt.handle,"task_status");
  else clearManagedTaskReceipt("Chưa xác nhận STOPPED. Không tự động retry hoặc chạy lại task.");
 });
 function clearPrivateGUIAfterClose(){
  // Purge all in-page authenticated outputs and pending approvals, including
  // values not reflected in the visible output panel. Browser/JS memory
  // cannot revoke the Agent; only the authenticated /api/close may do that.
  consoleRecords=[];consoleHistory=[];consoleHistoryCursor=0;consoleHistoryGeneration++;gitLogSnapshot=[];gitStatusSnapshot=[];gitStatusAuthenticated=false;gitReviewQueue=[];gitReviewQueueIndex=-1;gitReviewQueueSource=null;gitReviewQueueLoaded.clear();gitDiffSnapshot=null;gitDiffFindContext=null;gitDiffFindPage=0;gitDiffFindGeneration++;gitHunkContext=null;gitHunkPage=0;gitHunkGeneration++;gitConflictPreviewSnapshot=null;gitConflictGeneration++;gitConflictSelectedIndex=-1;gitConflictDraft=null;consoleCwd=".";
  filePage=null;taskWindow=null;managedTaskReceipt=null;
  $("task-verify-stop").disabled=true;
  review=null;gitReview=null;fileReview=null;lineReview=null;
  for(const id of ["approval","git-approval","file-editor","line-editor"]){
   $(id).classList.add("hidden");
  }
  for(const id of ["explorer-location","git-diff-target","output","workspace-entries","file-page-transcript",
   "find-entries","git-status-files","git-log-entries","git-remotes-entries","git-remotes-count","git-tracking-entries","git-tracking-count",
   "git-log-selection","git-diff-text","git-branch-entries","git-review-queue-list","git-review-queue-summary","git-upstream-summary","git-conflict-marker-count","git-conflict-marker-selected","git-conflict-marker-text","git-conflict-marker-entries","git-conflict-draft-text","git-conflict-draft-label",
   "git-diff-find-entries","git-diff-find-count","git-diff-selected","git-diff-page",
   "git-hunk-entries","git-hunk-count","git-hunk-selected","git-hunk-page","git-hunk-snippet",
   "task-catalog","task-lifecycle","task-transcript","console-transcript","console-location","console-search-count","console-history-entries","console-history-count","console-task-window","console-task-progress",
   "git-confirm-text","approval-text","file-edit-approval","line-edit-approval",
   "line-edit-source","line-edit-title"]){
   $(id).textContent="Đã xóa dữ liệu của phiên khỏi giao diện cục bộ.";
  }
  for(const id of ["directory","file","find-literal","file-edit-text",
   "file-edit-confirm","line-edit-text","line-edit-confirm",
   "git-path","git-branch","git-log-filter","git-status-filter","git-diff-find","git-confirm-entry",
   "git-commit-message","task-handle","approval-entry",
   "console-command","console-transcript-search","console-history-filter"]){
   $(id).value="";
  }
  $("line").value="1";$("task-id").value="1";$("task-offset").value="0";
  for(const id of ["file-read-next","task-read-next","git-diff-prev","git-diff-next","git-hunk-prev","git-hunk-next","git-queue-prev","git-queue-next","git-queue-next-unread","git-queue-add-visible","git-queue-clear","git-queue-add","git-queue-review-stage","git-queue-review-unstage","git-next-conflict","git-conflict-read","git-conflict-draft-ours","git-conflict-draft-theirs","git-conflict-draft-review","console-task-attach","console-task-next"])$(id).disabled=true;
  $("console-count").textContent="0 kết quả";
  $("git-log-count").textContent="0 commit";
  $("git-status-scope").value="all";
  $("git-review-side").value="all";$("git-review-side").disabled=true;
  $("git-status-count").textContent="Đã xóa";
  $("git-branch-count").textContent="Đã xóa";
  $("task-catalog-count").textContent="Đã xóa";
  $("find-summary").textContent="Đã xóa";
  $("git-diff-kind").textContent="Đã xóa";
  $("file-page-progress").textContent="Đã xóa";
  $("task-output-progress").textContent="Đã xóa";
 }
 $("close").addEventListener("click",async()=>{
  if(sessionEnding||!confirm("Đóng phiên và xóa dữ liệu đã xem? Thao tác đang thực thi trên host có thể vẫn hoàn tất."))return;
  sessionEnding=true;
  clearPrivateGUIAfterClose();
  $("close").disabled=true;
  setBusy(true);
  const result=await call({},true);
  setBusy(false);
  if(result){
   // /api/close acknowledges the *local native GUI*, not a separate
   // Agent-side revocation receipt. runNativeGUI then unwinds and closes
   // PingClient/WSS; never claim a remote acknowledgement here.
   notify("Đã đóng GUI native",
    "GUI native xác nhận yêu cầu đóng; kết nối E2EE đang kết thúc. Đóng tab này. Không có xác nhận thu hồi riêng từ Agent.");
  }else{
   notify("Chưa xác nhận đóng GUI",
    "Đã xóa dữ liệu và khóa thao tác cục bộ; chưa xác nhận GUI native đã đóng. Thoát tiến trình TaskDeck native để ngắt kết nối host.");
  }
 });
 if(!/^[a-f0-9]{64}$/.test(key)){
  setBusy(true);notify("Không có khóa phiên","Chỉ mở GUI qua native client đã xác thực.");
 }else{
  notify("GUI sẵn sàng","Chọn Files, Git hoặc Tasks. PHP/Relay không cung cấp giao diện này.");
 }
})();
