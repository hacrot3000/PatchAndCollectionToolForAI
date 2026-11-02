/* Bundled with trusted native executable, never fetched from PHP Portal. */
"use strict";
(() => {
 const key=decodeURIComponent(location.hash.slice(1));
 history.replaceState(null,"",location.pathname);
 const $=(id)=>document.getElementById(id);
 const titles={files:["Files","Duyệt workspace và xem văn bản qua Agent E2EE."],git:["Git","Git metadata và diff qua quyền host."],tasks:["Tasks","Chạy task phê duyệt và theo dõi có kiểm soát."],console:["Read-only Console","Lệnh cố định: chỉ đọc qua native E2EE Agent, không shell."]};
 let used=0,max=6,busy=false,review=null,gitReview=null,fileReview=null,lineReview=null,taskWindow=null,filePage=null,consoleRecords=[],consoleHistory=[],consoleHistoryCursor=0,gitLogSnapshot=[],gitStatusSnapshot=[],sessionEnding=false,consoleCwd=".";
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
     clearGitDiffPreview("Đã chọn file khác");
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
  $("git-diff-kind").textContent=message;
  $("git-diff-target").textContent="Chọn file rồi yêu cầu một bản diff mới từ Agent.";
  $("git-diff-text").textContent="Không có bản xem trước diff hiện hành.";
 }
 function renderGitDiffPreview(report,operation,path){
  const labels={
   git_diff:"Working-tree diff",
   git_staged:"Staged diff",
   git_head:"HEAD diff",
   git_review:"Two-way review"
  };
  if(!Object.hasOwn(labels,operation)||typeof report!=="string"||
   new TextEncoder().encode(report).length>4096||
   !safeWorkspacePath(path)||path==="."||$("git-path").value.trim()!==path){
   clearGitDiffPreview("Không hợp lệ");
   notify("Từ chối Git diff","Kết quả quá lớn hoặc đường dẫn không hợp lệ, không hiển thị một phần.");
   return;
  }
  const safe=report.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/gu,
   c=>"\\u"+c.codePointAt(0).toString(16).padStart(4,"0"));
  $("git-diff-kind").textContent=labels[operation];
  $("git-diff-target").textContent="File đã yêu cầu: "+path+
   " · kết quả chỉ đọc, đã xác thực qua Agent E2EE";
  $("git-diff-text").textContent=safe||"Không có thay đổi hoặc diff trống.";
 }
 $("git-path").addEventListener("input",()=>clearGitDiffPreview("Đường dẫn đã đổi"));
 function showGitStatusCards(){
  const target=$("git-status-files");
  target.replaceChildren();
  const query=$("git-status-filter").value.toLowerCase().trim();
  const scope=$("git-status-scope").value||"all";
  if(new TextEncoder().encode(query).length>96||
   /[\u0000-\u001f\u007f-\u009f\p{Cf}\p{Zl}\p{Zp}]/u.test(query)||
   !["all","staged","unstaged","untracked"].includes(scope)){
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
   untracked:gitStatusSnapshot.filter(e=>e.untracked).length
  };
  $("git-status-count").textContent=matching.length+"/"+gitStatusSnapshot.length+
   " file · "+counts.staged+" staged · "+counts.unstaged+" unstaged · "+
   counts.untracked+" untracked";
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
   button.addEventListener("click",()=>{
    if(busy||sessionEnding)return;
    // No implicit git write or E2EE action on choosing a status card.
    $("git-path").value=entry.path;
    $("file").value=entry.path;
    invalidateChangedReviews();
    clearGitDiffPreview("Đã chọn file khác");
    gitReview=null;
    $("git-approval").classList.add("hidden");
    $("status").textContent="Đã chọn Git file · cần xem Diff hoặc review thao tác ghi";
   });
   target.append(button);
  }
 }
 ["git-status-filter","git-status-scope"].forEach(id=>
  $(id).addEventListener("input",showGitStatusCards));
 function renderGitStatus(report){
  const target=$("git-status-files");
  target.replaceChildren();gitStatusSnapshot=[];
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
    untracked:status==="??"});
  }
  gitStatusSnapshot=entries;
  showGitStatusCards();
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
  return true;
 }
 // This grammar maps to existing fixed typed read-only operations only.
 // No executable, argv, shell syntax, HTTP method, Git ref or write request
 // can be supplied by this console. The trusted native handler and host
 // still authenticate/authorize every message independently.
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
  const exact={
   "git status":{operation:"git_status"},
   "git log":{operation:"git_log"},
   "git branches":{operation:"git_branches"},
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
  $("console-transcript").textContent=consoleRecords.join("\n\n");
  $("console-count").textContent=consoleRecords.length+" kết quả gần nhất";
 }
 function quota(){
  $("quota").textContent=used+" / "+max+" E2EE actions";
  if(used>=max)$("status").textContent="Hết lượt E2EE · đăng nhập lại để tiếp tục";
 }
 function setBusy(next){
  busy=next;
  document.querySelectorAll("[data-op],#execute-task,#cancel-task,#execute-git,#execute-file-edit,#execute-line-edit,#task-read-next,#file-read-next,#console-run").forEach(b=>{
   b.disabled=sessionEnding||next||(used>=max&&b.id!=="console-run")||
    (b.id==="task-read-next"&&(!taskWindow||taskWindow.eof))||
    (b.id==="file-read-next"&&(!filePage||filePage.next===0))||
    (["git_write_review","file_edit_review","line_write_review","git_review"].includes(b.dataset.op)&&used+2>max);
  });
 }
 async function call(input,closing=false){
  if(!closing&&(busy||used>=max||sessionEnding))return null;
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
  if(operation==="git_status"){
   gitStatusSnapshot=[];
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
  if(result&&operation==="git_status"){renderGitStatus(result.text||"");}
  if(result&&operation==="git_log"){renderGitLog(result.text||"");}
  if(result&&["git_diff","git_staged","git_head","git_review"].includes(operation)){
   renderGitDiffPreview(result.text||"",operation,input.path);
  }
  if(result&&operation==="git_branches"){renderGitBranches(result.text||"");}
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
  if(result&&operation==="git_write_review"&&result.confirm&&
   (!gitTargetMatches(input.mode,input.path||""))){
   notify("Review hết hiệu lực","Đích Git đã đổi trong lúc xác thực; cần Review lại.");
  }else if(result&&operation==="git_write_review"&&result.confirm){
   gitReview={mode:input.mode,path:input.path||"",confirm:result.confirm};
   $("git-confirm-text").textContent="Đọc đầy đủ preview ở khung kết quả, sau đó nhập chính xác: "+result.confirm;
   $("git-confirm-entry").value="";
   $("git-commit-message").value="";
   $("git-message-label").classList.toggle("hidden",input.mode!=="commit");
   $("git-approval").classList.remove("hidden");
  }
  if(result&&operation==="task_review"&&result.confirm&&
   (Number($("task-id").value)!==input.task_id)){
   notify("Review hết hiệu lực","Đích Task đã đổi trong lúc xác thực; cần Review lại.");
  }else if(result&&operation==="task_review"&&result.confirm){
   review={id:input.task_id,mode:input.mode,confirm:result.confirm};
   $("approval-text").textContent="Host đã phê duyệt. Nhập chính xác: "+result.confirm;
   $("approval-entry").value="";$("approval").classList.remove("hidden");
  }
  if(result&&operation==="task_output"){
   acceptTaskWindow(result.text||"",input.handle,input.offset);
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
  // A console read changes review context, so never leave a write approval
  // visible when opening its output. Native Go also drops pending reviews
  // before dispatching any different typed action.
  gitReview=null;$("git-approval").classList.add("hidden");
  fileReview=null;$("file-editor").classList.add("hidden");
  lineReview=null;$("line-editor").classList.add("hidden");
  review=null;$("approval").classList.add("hidden");
  if(parsed.clear){
   consoleRecords=[];
   $("console-transcript").textContent="Đã xóa kết quả hiển thị trong phiên hiện tại.";
   $("console-count").textContent="0 kết quả";
   notify("Đã xóa lịch sử hiển thị","Không gửi yêu cầu mạng và không thay đổi dữ liệu trên host.");
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
    "pwd | cd <dir> | cd .. | ls [dir] | clear | "+
    "tasks status <32 lowercase hex> | tasks output <32 lowercase hex> <offset>. "+
    "Không có shell, PTY, Git write hoặc script.";
   consoleWrite("help",help);
   notify("Hướng dẫn read-only",help);
   return;
  }
  if(used>=max||(parsed.operation==="git_review"&&used+2>max)){
   notify("Hết lượt E2EE",parsed.operation==="git_review"?
    "Git review 2 chiều cần hai lượt xác thực còn trống. Đăng nhập lại để tiếp tục.":
    "Đã hết lượt E2EE; chỉ còn lệnh cục bộ help, clear, pwd, cd.");
   return;
  }
  const selections={directory:$("directory").value,gitPath:$("git-path").value};
  if(parsed.operation==="files_list"){
   $("workspace-entries").textContent="Đang xác thực danh sách thư mục qua Console…";
   $("explorer-location").textContent="Đang xác thực "+parsed.path;
  }else if(parsed.operation==="tasks"){
   $("task-catalog").textContent="Đang xác thực danh sách TaskDeck qua Console…";
   $("task-catalog-count").textContent="Đang cập nhật";
  }else if(["git_diff","git_staged","git_head"].includes(parsed.operation)){
   clearGitDiffPreview("Đang xác thực");
  }
  // Clear old Git cards before fetching a new authenticated snapshot.
  // On transport failure the GUI must not present stale status as fresh.
  if(parsed.operation==="git_status"){
   gitStatusSnapshot=[];
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
  }
  const result=await call(parsed);
  if(!result){
   if(!sessionEnding){
    if(parsed.operation==="git_status")$("git-status-count").textContent="Không tải được";
    if(parsed.operation==="git_log")$("git-log-count").textContent="Không tải được";
    if(parsed.operation==="git_branches")$("git-branch-count").textContent="Không tải được";
    if(parsed.operation==="files_list"){
     $("workspace-entries").textContent="Không lấy được danh sách thư mục từ Agent.";
     $("explorer-location").textContent="Không tải được";
    }
    if(parsed.operation==="tasks"){
     $("task-catalog").textContent="Không lấy được danh sách task từ Agent.";
     $("task-catalog-count").textContent="Không tải được";
    }
    if(["git_diff","git_staged","git_head"].includes(parsed.operation))
     clearGitDiffPreview("Không tải được");
   }
   return;
  }
  // Reuse the SAME authenticated Agent response to update read-only Git
  // cards. No second E2EE request, no browser-to-PHP input and no Git write.
  if(parsed.operation==="git_status")renderGitStatus(result.text||"");
  else if(parsed.operation==="git_log")renderGitLog(result.text||"");
  else if(parsed.operation==="git_branches")renderGitBranches(result.text||"");
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
  else if(["git_diff","git_staged","git_head"].includes(parsed.operation)){
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
 $("console-command").addEventListener("keydown",event=>{
  if(busy||event.repeat)return;
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
  const handle=$("task-handle").value.trim();
  if(!/^[0-9a-f]{32}$/.test(handle)){notify("Lỗi dữ liệu","Handle không hợp lệ.");return;}
  const words="CANCEL TASK "+handle.slice(0,12);
  if(prompt("Để yêu cầu dừng task, nhập: "+words)!==words){
   notify("Đã hủy","Không gửi yêu cầu dừng.");return;
  }
  await call({operation:"task_cancel",handle,confirmation:words});
 });
 function clearPrivateGUIAfterClose(){
  // Purge all in-page authenticated outputs and pending approvals, including
  // values not reflected in the visible output panel. Browser/JS memory
  // cannot revoke the Agent; only the authenticated /api/close may do that.
  consoleRecords=[];consoleHistory=[];consoleHistoryCursor=0;gitLogSnapshot=[];gitStatusSnapshot=[];consoleCwd=".";
  filePage=null;taskWindow=null;
  review=null;gitReview=null;fileReview=null;lineReview=null;
  for(const id of ["approval","git-approval","file-editor","line-editor"]){
   $(id).classList.add("hidden");
  }
  for(const id of ["explorer-location","git-diff-target","output","workspace-entries","file-page-transcript",
   "find-entries","git-status-files","git-log-entries",
   "git-log-selection","git-diff-text","git-branch-entries",
   "task-catalog","task-transcript","console-transcript","console-location",
   "git-confirm-text","approval-text","file-edit-approval","line-edit-approval",
   "line-edit-source","line-edit-title"]){
   $(id).textContent="Đã xóa dữ liệu của phiên khỏi giao diện cục bộ.";
  }
  for(const id of ["directory","file","find-literal","file-edit-text",
   "file-edit-confirm","line-edit-text","line-edit-confirm",
   "git-path","git-branch","git-log-filter","git-status-filter","git-confirm-entry",
   "git-commit-message","task-handle","approval-entry",
   "console-command"]){
   $(id).value="";
  }
  $("line").value="1";$("task-id").value="1";$("task-offset").value="0";
  for(const id of ["file-read-next","task-read-next"])$(id).disabled=true;
  $("console-count").textContent="0 kết quả";
  $("git-log-count").textContent="0 commit";
  $("git-status-scope").value="all";
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
