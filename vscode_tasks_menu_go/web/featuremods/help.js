// Offline, read-only TaskDeck Help: shortcuts reflect handlers in featuremods.
const taskdeckHelpTopics=[
  {title:'Phím tắt chung · General',items:[
    ['Ctrl/Cmd + Shift + P','Mở Command Palette để tìm lệnh hoặc công cụ.'],
    ['Ctrl/Cmd + P','Quick Open: tìm file, tab và tài nguyên.'],
    ['Ctrl/Cmd + Shift + F','Tìm và thay thế text trên toàn project.'],
    ['Ctrl/Cmd + Shift + Backquote','Mở terminal local mới.'],
    ['Ctrl + Alt + W','Đóng tab TaskDeck đang chọn, dùng được khi Firefox dành Ctrl+W.'],
    ['Ctrl/Cmd + W','Đóng tab trong tool nếu trình duyệt cho phép; Firefox có thể đóng tab trình duyệt.'],
    ['Esc','Đóng menu/dialog; đóng Diff, viewer, Patch Tool hoặc editor chưa từng sửa.']
  ]},
  {title:'File Editor',items:[
    ['Ctrl/Cmd + S','Lưu file đang sửa.'],
    ['Ctrl/Cmd + F','Tìm trong file; Enter/Shift+Enter chuyển kết quả.'],
    ['Ctrl/Cmd + H','Tìm và thay thế.'],
    ['Ctrl/Cmd + G','Chuyển tới dòng.'],
    ['Ctrl/Cmd + Shift + O','Hiển thị outline/symbol.'],
    ['Ctrl/Cmd + Shift + T','Mở lại file editor vừa đóng.']
  ]},
  {title:'Terminal',items:[
    ['Ctrl + C','Ngắt lệnh đang chạy trong terminal được focus.'],
    ['Ctrl + F','Tìm trong nội dung terminal.'],
    ['Tab / ↑ / ↓','Hoàn thành hoặc duyệt lịch sử lệnh tùy shell.']
  ]},
  {title:'Hướng dẫn nhanh · Quick start',items:[
    ['Tabs & Split','Chuột phải vào tab để xem thao tác; chuột phải khe trống để mở lại tab hoặc tạo terminal. Split group có viền cùng màu, tab active sáng hơn.'],
    ['Diff','So sánh hai file/nhánh, xem View diff/context và bật/tắt unimportant. Esc đóng File Diff chưa từng sửa.'],
    ['Git','Mở Git panel để kiểm tra thay đổi, stage/commit/push và so sánh branch. Luôn xem diff trước commit.'],
    ['FTP/SFTP & Database','Chọn connection profile, kiểm tra nguồn/đích trước upload/download hoặc thao tác dữ liệu.'],
    ['Patch Tool','Chuẩn bị patch/collection, theo dõi Queue và xem kết quả/handoff.']
  ]}
];
const style=document.createElement('style');
style.textContent=[
'.taskdeck-help-backdrop{position:fixed;inset:0;z-index:18000;display:none;align-items:flex-start;justify-content:center;padding:clamp(10px,4vh,40px) 12px;background:rgba(0,0,0,.64)}',
'.taskdeck-help-backdrop.visible{display:flex}',
'.taskdeck-help-dialog{width:min(800px,100%);max-height:91vh;display:flex;flex-direction:column;min-height:0;overflow:hidden;background:#151c26;color:#edf3fb;border:1px solid #56677e;border-radius:10px;box-shadow:0 18px 55px #0009}',
'.taskdeck-help-head{display:flex;gap:10px;align-items:center;padding:11px 14px;border-bottom:1px solid #3c4a5d}',
'.taskdeck-help-head h2{font-size:16px;flex:1;margin:0}.taskdeck-help-head button{font-size:16px;padding:3px 10px;border-radius:5px;border:1px solid #61738e;background:#27364a;color:#f5f9ff}',
'.taskdeck-help-intro{font-size:12px;line-height:1.5;margin:8px 14px;opacity:.84}',
'.taskdeck-help-search{box-sizing:border-box;width:calc(100% - 28px);margin:0 14px 7px;padding:7px 9px;border:1px solid #697b94;border-radius:6px;background:#0d1520;color:#f5faff;font-size:12px}',
'.taskdeck-help-scroll{overflow:auto;flex:1;min-height:0;padding:0 14px 16px}',
'.taskdeck-help-section{margin:11px 0 14px}.taskdeck-help-section h3{margin:0 0 7px;font-size:12px;color:#c3dfff}',
'.taskdeck-help-grid{display:grid;grid-template-columns:minmax(120px,190px) minmax(0,1fr);border:1px solid #3e4a5d;border-radius:6px;overflow:hidden}',
'.taskdeck-help-key,.taskdeck-help-description{font-size:12px;line-height:1.5;padding:7px 9px;border-bottom:1px solid #344052}',
'.taskdeck-help-key{background:#202c3a;color:#e4f0ff;overflow-wrap:anywhere}.taskdeck-help-key kbd{font:600 11px/1.5 ui-monospace,monospace}',
'.taskdeck-help-description{color:#e1eaf4}.taskdeck-help-key[hidden],.taskdeck-help-description[hidden]{display:none!important}',
'.taskdeck-help-empty{font-size:12px;opacity:.7;padding:12px}',
'html[data-taskmenu-theme="light"] .taskdeck-help-dialog{background:#fff;color:#182738;border-color:#bac5d2}',
'html[data-taskmenu-theme="light"] .taskdeck-help-head{border-color:#d4deea}',
'html[data-taskmenu-theme="light"] .taskdeck-help-head button{background:#eaf1f8;color:#1c344b;border-color:#aebcca}',
'html[data-taskmenu-theme="light"] .taskdeck-help-search{background:#f9fbff;color:#172535;border-color:#b5c1d0}',
'html[data-taskmenu-theme="light"] .taskdeck-help-section h3{color:#275a82}',
'html[data-taskmenu-theme="light"] .taskdeck-help-grid{border-color:#ccd6e0}',
'html[data-taskmenu-theme="light"] .taskdeck-help-key{background:#edf3fa;color:#193850}',
'html[data-taskmenu-theme="light"] .taskdeck-help-description{color:#1b2938}',
'html[data-taskmenu-theme="light"] .taskdeck-help-key,html[data-taskmenu-theme="light"] .taskdeck-help-description{border-color:#dce4eb}',
'@media(max-width:560px){.taskdeck-help-grid{grid-template-columns:minmax(98px,135px) minmax(0,1fr)}.taskdeck-help-key,.taskdeck-help-description{font-size:11px;padding:6px}}'
].join('\n');
document.head.append(style);
const backdrop=document.createElement('div');backdrop.className='taskdeck-help-backdrop';
const dialog=document.createElement('section');dialog.className='taskdeck-help-dialog';dialog.tabIndex=-1;
dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-labelledby','taskdeck-help-title');
const header=document.createElement('div');header.className='taskdeck-help-head';
const heading=document.createElement('h2');heading.id='taskdeck-help-title';heading.textContent='Help · TaskDeck';
const dismiss=document.createElement('button');dismiss.type='button';dismiss.textContent='×';dismiss.title='Close Help';dismiss.setAttribute('aria-label','Close Help');
header.append(heading,dismiss);
const intro=document.createElement('p');intro.className='taskdeck-help-intro';
intro.textContent='Tra nhanh phím tắt và hướng dẫn cơ bản. Ctrl/Cmd = Ctrl trên Linux/Windows, Cmd trên macOS. Trình duyệt có thể ưu tiên một số phím tắt.';
const search=document.createElement('input');search.type='search';search.className='taskdeck-help-search';search.placeholder='Tìm phím tắt hoặc hướng dẫn…';search.setAttribute('aria-label','Find Help topic');
const scroll=document.createElement('div');scroll.className='taskdeck-help-scroll';
const empty=document.createElement('p');empty.className='taskdeck-help-empty';empty.textContent='Không tìm thấy mục phù hợp.';empty.hidden=true;
const viewSections=[];
for(const topic of taskdeckHelpTopics){
  const section=document.createElement('section');section.className='taskdeck-help-section';
  const title=document.createElement('h3');title.textContent=topic.title;
  const grid=document.createElement('div');grid.className='taskdeck-help-grid';
  const entries=[];
  for(const [key,description] of topic.items){
    const left=document.createElement('div');left.className='taskdeck-help-key';
    const kbd=document.createElement('kbd');kbd.textContent=key;left.append(kbd);
    const right=document.createElement('div');right.className='taskdeck-help-description';right.textContent=description;
    grid.append(left,right);
    entries.push({left,right,needle:(key+' '+description).toLocaleLowerCase('vi')});
  }
  section.append(title,grid);scroll.append(section);viewSections.push({section,entries});
}
scroll.append(empty);dialog.append(header,intro,search,scroll);backdrop.append(dialog);document.body.append(backdrop);
function filterHelp(){
  const query=search.value.trim().toLocaleLowerCase('vi');let matched=0;
  for(const {section,entries} of viewSections){
    let shown=0;
    for(const entry of entries){
      const visible=!query||entry.needle.includes(query);
      entry.left.hidden=!visible;entry.right.hidden=!visible;
      if(visible)shown++;
    }
    section.hidden=shown===0;matched+=shown;
  }
  empty.hidden=matched>0;
}
let previousFocus=null;
function close(){
  if(!backdrop.classList.contains('visible'))return;
  backdrop.classList.remove('visible');search.value='';filterHelp();
  if(previousFocus?.isConnected)previousFocus.focus();
  previousFocus=null;
}
function open(){
  previousFocus=document.activeElement;backdrop.classList.add('visible');
  search.value='';filterHelp();search.focus();return true;
}
dismiss.onclick=close;
search.oninput=filterHelp;
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
dialog.addEventListener('keydown',event=>{
  if(event.key==='Escape'){event.preventDefault();event.stopPropagation();close();}
  if(event.key==='Tab'){
    if(event.shiftKey&&document.activeElement===search){event.preventDefault();dismiss.focus();}
    if(!event.shiftKey&&document.activeElement===dismiss){event.preventDefault();search.focus();}
  }
});
globalThis.TaskMenuHelp={open,close,get visible(){return backdrop.classList.contains('visible');},topics:taskdeckHelpTopics};
