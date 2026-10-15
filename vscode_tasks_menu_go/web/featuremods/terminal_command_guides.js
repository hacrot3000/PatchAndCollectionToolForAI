// Pure, declarative command guide registry. Providers can be added later without
// altering autocomplete or terminal input. No commands are ever executed here.
(function(root){
'use strict';
const providers=new Map();
const git={
 id:'git',
 sections:{
  base:[
   ['git status','git status','Kiểm tra trạng thái','Hiển thị staged, unstaged, untracked'],
   ['git diff','git diff','Xem thay đổi','So sánh working tree với index'],
   ['git add ','git add <path>','Stage file','Nhập đường dẫn, hoặc dấu . để stage tất cả'],
   ['git commit -m ','git commit -m "<message>"','Tạo commit','Nhập thông điệp trong dấu nháy'],
   ['git log --oneline -10','git log --oneline -10','Lịch sử gần nhất','10 commit mới nhất'],
   ['git branch -a','git branch -a','Xem branch','Cả local và remote-tracking'],
   ['git remote -v','git remote -v','Xem URL remote','Kiểm tra fetch và push URL'],
   ['git fetch --all --prune','git fetch --all --prune','Fetch và dọn ref','Không merge vào working tree'],
   ['git pull --ff-only','git pull --ff-only','Pull fast-forward','Không tạo merge commit'],
   ['git push','git push','Push branch hiện tại','Sử dụng upstream đã thiết lập']
  ],
  remote:[
   ['git remote -v','git remote -v','Xem các remote','URL dùng để fetch và push'],
   ['git remote add ','git remote add <name> <url>','Thêm remote','Nhập tên remote và URL'],
   ['git remote set-url origin ','git remote set-url origin <new-url>','Đổi URL của origin','Thay URL fetch của remote origin'],
   ['git remote set-url ','git remote set-url <remote> <new-url>','Đổi URL remote khác','Nhập tên remote và URL mới'],
   ['git remote set-url --push origin ','git remote set-url --push origin <url>','Đổi push URL','Giữ nguyên URL fetch'],
   ['git remote set-url --add --push origin ','git remote set-url --add --push origin <url>','Thêm URL push','Giữ URL push cũ'],
   ['git remote set-url --delete --push origin ','git remote set-url --delete --push origin <url>','Xóa một URL push','Nhập đúng URL cần xóa','caution'],
   ['git remote remove ','git remote remove <name>','Xóa remote','Xóa cấu hình remote tại local','caution'],
   ['git remote rename ','git remote rename <old> <new>','Đổi tên remote','Nhập tên cũ và tên mới']
  ],
  status:[
   ['git status','git status','Xem trạng thái','Kiểm tra các file thay đổi'],
   ['git status --short','git status --short','Trạng thái gọn','Dạng hai ký tự'],
   ['git status --branch','git status --branch','Xem trạng thái branch','Kèm thông tin upstream']
  ],
  branch:[
   ['git branch -avv','git branch -avv','Liệt kê các branch','Kèm upstream và commit'],
   ['git switch ','git switch <branch>','Chuyển branch','Nhập tên branch'],
   ['git switch -c ','git switch -c <new-branch>','Tạo và chuyển branch','Tạo branch rồi checkout'],
   ['git branch ','git branch <name>','Tạo branch mới','Không checkout'],
   ['git branch -d ','git branch -d <branch>','Xóa branch đã merge','Git từ chối nếu còn commit chưa merge','caution'],
   ['git branch -D ','git branch -D <branch>','Buộc xóa branch','Có thể mất tham chiếu tới commit','danger']
  ],
  switch:[
   ['git switch ','git switch <branch>','Chuyển branch','Nhập branch đích'],
   ['git switch -c ','git switch -c <name>','Tạo branch','Tạo rồi chuyển'],
   ['git switch -','git switch -','Quay về branch trước','Không cần nhớ tên']
  ],
  checkout:[
   ['git checkout ','git checkout <branch>','Checkout branch','Nên dùng git switch khi chỉ đổi branch'],
   ['git checkout -b ','git checkout -b <name>','Tạo và checkout','Tạo branch mới']
  ],
  add:[
   ['git add ','git add <path>','Stage file','Nhập đường dẫn'],
   ['git add .','git add .','Stage tất cả','Gồm file mới và file sửa'],
   ['git add -p','git add -p','Stage từng hunk','Git sẽ hỏi từng phần']
  ],
  commit:[
   ['git commit -m ','git commit -m "<message>"','Commit với thông điệp','Nhập thông điệp'],
   ['git commit --amend','git commit --amend','Sửa commit cuối','Commit sẽ đổi hash','caution']
  ],
  fetch:[
   ['git fetch origin','git fetch origin','Fetch origin','Không làm thay đổi file'],
   ['git fetch --all --prune','git fetch --all --prune','Fetch tất cả','Dọn remote-tracking ref'],
   ['git fetch --tags','git fetch --tags','Lấy tags','Không tự checkout']
  ],
  pull:[
   ['git pull --ff-only','git pull --ff-only','Pull fast-forward','Không tự merge diverged branch'],
   ['git pull --rebase','git pull --rebase','Pull bằng rebase','Có thể gặp conflict','caution'],
   ['git pull origin ','git pull origin <branch>','Pull branch từ origin','Nhập branch nguồn']
  ],
  push:[
   ['git push','git push','Push branch hiện tại','Dùng upstream đang theo dõi'],
   ['git push -u origin ','git push -u origin <branch>','Push và đặt upstream','Dùng cho branch mới'],
   ['git push --tags','git push --tags','Push tags','Đẩy mọi tag'],
   ['git push --force-with-lease','git push --force-with-lease','Force có kiểm tra','Có thể viết lại lịch sử remote','danger']
  ],
  diff:[
   ['git diff','git diff','Diff chưa staged','Working tree với index'],
   ['git diff --staged','git diff --staged','Diff đã staged','Sẽ nằm trong commit'],
   ['git diff ','git diff <branch>','So sánh branch','Nhập branch hoặc commit'],
   ['git diff --stat','git diff --stat','Thống kê file','Không hiện chi tiết từng dòng']
  ],
  log:[
   ['git log --oneline -10','git log --oneline -10','Lịch sử rút gọn','10 commit mới nhất'],
   ['git log --graph --oneline --decorate --all','git log --graph --oneline --decorate --all','Graph commit','Mọi nhánh và tag'],
   ['git log -- ','git log -- <path>','Lịch sử một file','Nhập đường dẫn']
  ],
  merge:[
   ['git merge --ff-only ','git merge --ff-only <branch>','Merge fast-forward','Dừng nếu không thể fast-forward'],
   ['git merge --no-ff ','git merge --no-ff <branch>','Merge tạo commit','Có thể gặp conflict','caution'],
   ['git merge --abort','git merge --abort','Hủy merge dở dang','Trở về trạng thái trước merge','caution']
  ],
  rebase:[
   ['git rebase ','git rebase <branch>','Rebase lên branch','Viết lại commit local','caution'],
   ['git rebase --continue','git rebase --continue','Tiếp tục rebase','Sau khi sửa conflict'],
   ['git rebase --abort','git rebase --abort','Hủy rebase','Trở về trạng thái cũ','caution']
  ],
  restore:[
   ['git restore ','git restore <path>','Khôi phục file','Mất thay đổi chưa commit','caution'],
   ['git restore --staged ','git restore --staged <path>','Bỏ stage file','Giữ working tree'],
   ['git restore --source=HEAD -- ','git restore --source=HEAD -- <path>','Khôi phục từ HEAD','Ghi đè file','caution']
  ],
  reset:[
   ['git reset','git reset','Bỏ stage','Giữ working tree'],
   ['git reset --soft HEAD~1','git reset --soft HEAD~1','Lùi commit, giữ stage','Viết lại lịch sử','caution'],
   ['git reset --hard HEAD','git reset --hard HEAD','Bỏ mọi thay đổi chưa commit','Có thể mất dữ liệu','danger']
  ],
  stash:[
   ['git stash push -u','git stash push -u','Stash cả file mới','Lưu thay đổi tạm thời'],
   ['git stash list','git stash list','Liệt kê stash','Xem các stash đã lưu'],
   ['git stash pop','git stash pop','Khôi phục stash','Có thể phát sinh conflict','caution'],
   ['git stash apply','git stash apply','Áp dụng stash','Giữ lại stash']
  ],
  tag:[
   ['git tag --list','git tag --list','Xem tag','Liệt kê tag local'],
   ['git tag ','git tag <name>','Tạo tag','Nhập tên tag'],
   ['git tag -a ','git tag -a <name> -m "<message>"','Tạo annotated tag','Thêm -m và nội dung']
  ],
  clean:[
   ['git clean -nd','git clean -nd','Xem trước file sẽ xóa','Dry-run, chưa xóa'],
   ['git clean -fd','git clean -fd','Xóa file untracked','Không hoàn tác bằng Git được','danger']
  ],
  worktree:[
   ['git worktree list','git worktree list','Liệt kê worktree','Đường dẫn và branch'],
   ['git worktree add ','git worktree add <path> <branch>','Tạo worktree','Nhập thư mục và branch'],
   ['git worktree prune','git worktree prune','Dọn worktree cũ','Dọn metadata']
  ],
  show:[
   ['git show --stat','git show --stat','Tóm tắt commit','Thống kê thay đổi'],
   ['git show ','git show <commit>','Xem commit','Nhập ref hoặc SHA']
  ]
 }
};
function register(provider){
 if(!provider||!/^[a-z][a-z0-9-]*$/.test(provider.id)||!provider.sections||providers.has(provider.id))return false;
 providers.set(provider.id,provider);
 return true;
}
register(git);
function wrap(row,index){
 const [command,syntax,label,help,risk='normal']=row;
 return {command,syntax,label,help,risk,source:'guide',group:-4,score:50-index};
}
function suggest(raw,limit=10){
 const typed=String(raw||'');
 // No shell interpolation, pipelines, quoted arguments, redirection or URL
 // editing context. Guides only own simple incomplete command prefixes.
 if(!typed||typed.length>240||/[;|&<>$\x60\\\r\n\x00-\x1f"']/.test(typed))return [];
 const clean=typed.trim().replace(/ +/g,' ');
 const words=clean.split(' '),provider=providers.get(words[0]);
 if(!provider)return [];
 const sections=provider.sections,topic=words[1]||'';
 if(words.length===1)return (sections.base||[]).slice(0,limit).map(wrap);
 if(!Object.prototype.hasOwnProperty.call(sections,topic)){
  if(words.length!==2)return [];
  return Object.keys(sections).filter(name=>name.startsWith(topic))
   .flatMap(name=>(sections[name]||[]).slice(0,2)).slice(0,limit).map(wrap);
 }
 if(words.length>4||(words.length>3&&topic!=='remote'))return [];
 if(topic==='remote'&&words.length===4){
  if(words[2]!=='set-url'||!/^[A-Za-z0-9._-]{1,64}$/.test(words[3]))return [];
  const name=words[3],row=['git remote set-url '+name+' ',
   'git remote set-url '+name+' <new-url>','Đổi URL '+name,'Nhập URL mới cho remote này'];
  return [wrap(row,0)];
 }
 let actions=sections[topic];
 if(words.length>=3){
  const keyword=words.slice(2).join(' ').toLowerCase();
  const lead=provider.id+' '+topic+' '+keyword;
  actions=actions.filter(row=>row[0].toLowerCase().startsWith(lead)||
   (topic==='remote'&&keyword==='set-url'&&['git remote add ','git remote remove '].includes(row[0])));
  // Show direct continuations of what the user typed first, then related
  // workflows that change the operation (e.g. remote add / remove).
  actions=actions.map((row,index)=>({row,index})).sort((a,b)=>
   Number(!a.row[0].toLowerCase().startsWith(lead))-
   Number(!b.row[0].toLowerCase().startsWith(lead))||a.index-b.index).map(item=>item.row);
 }
 return actions.slice(0,Math.max(1,Math.min(12,Number(limit)||10))).map(wrap);
}
root.TaskDeckCommandGuides={register,suggest,providerIds:()=>[...providers.keys()]};
if(typeof module==='object'&&module.exports)module.exports=root.TaskDeckCommandGuides;
})(globalThis);