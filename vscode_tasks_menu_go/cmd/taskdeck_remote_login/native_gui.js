/* Bundled with trusted native executable, never fetched from PHP Portal. */
"use strict";
(() => {
 const key=decodeURIComponent(location.hash.slice(1));
 history.replaceState(null,"",location.pathname);
 const $=(id)=>document.getElementById(id);
 const titles={files:["Files","Duyệt workspace và xem văn bản qua Agent E2EE."],git:["Git","Git metadata và diff qua quyền host."],tasks:["Tasks","Chạy task phê duyệt và theo dõi có kiểm soát."]};
 let used=0,max=6,busy=false,review=null;
 function notify(status,text){$("status").textContent=status;$("output").textContent=text;}
 function quota(){
  $("quota").textContent=used+" / "+max+" E2EE actions";
  if(used>=max)notify("Hết hạn mức","Đã sử dụng hết lượt E2EE. Đóng GUI và đăng nhập lại.");
 }
 function setBusy(next){
  busy=next;
  document.querySelectorAll("[data-op],#execute-task,#cancel-task").forEach(b=>b.disabled=next||used>=max);
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
     literal.trim()!==literal||/[\\u0000-\\u001f\\u007f-\\u009f\\u202a-\\u202e\\u2066-\\u2069]/u.test(literal)){
     notify("Chuỗi tìm không hợp lệ","Nhập literal UTF-8 1–96 byte, không có ký tự điều khiển hoặc định hướng.");
     return;
    }
    input.literal=literal;
   }
  }
  if(["git_diff","git_staged","git_head"].includes(operation))input.path=$("git-path").value.trim();
  if(operation==="task_review"){
   input.task_id=Number($("task-id").value);input.mode=btn.dataset.mode;
   $("approval").classList.add("hidden");review=null;
  }
  if(operation==="task_status"||operation==="task_output"){
   input.handle=$("task-handle").value.trim();
   if(operation==="task_output")input.offset=Number($("task-offset").value);
  }
  const result=await call(input);
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
