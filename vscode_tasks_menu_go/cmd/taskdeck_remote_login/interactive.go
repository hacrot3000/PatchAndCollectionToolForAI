package main

import (
 "bufio"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "io"
 "strconv"
 "strings"
 "unicode"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/remoterelay"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// The trusted native console never forwards command text to a shell.
// Remote operations stay individually typed, audited, RBAC-gated and E2EE.
type remoteConsoleAction interface {
 ReadFile(context.Context,string)(string,error)
 ReadFilePreview(context.Context,string)(string,error)
 ReadFileLines(context.Context,string,int64)(string,error)
 EditLargeFileLine(context.Context,string,int64,string,string)(string,error)
 DeleteLargeFileLine(context.Context,string,int64,string)(string,error)
 InsertLargeFileLine(context.Context,string,int64,string,string)(string,error)
 FindFileLines(context.Context,string,string)(string,error)
 ListDirectory(context.Context,string)(string,error)
 GitStatus(context.Context)(string,error)
 GitStatusFull(context.Context)(string,error)
 GitCommitReview(context.Context)(string,error)
 GitCommit(context.Context,string,string)(string,error)
 GitBranchReview(context.Context)(string,error)
 GitBranchCreate(context.Context,string,string)(string,error)
 GitSwitchReview(context.Context,string)(string,error)
 GitSwitchSameHead(context.Context,string,string)(string,error)
 GitBranchDeleteReview(context.Context,string)(string,error)
 GitBranchDelete(context.Context,string,string)(string,error)
 GitLog(context.Context)(string,error)
 GitBranches(context.Context)(string,error)
 GitDiff(context.Context,string)(string,error)
 GitStagedDiff(context.Context,string)(string,error)
 GitHeadDiff(context.Context,string)(string,error)
 GitStage(context.Context,string,string)(string,error)
 GitUnstage(context.Context,string,string)(string,error)
 ListApprovedTasks(context.Context)(string,error)
 ReplaceFile(context.Context,string,string,string)(string,error)
 ReplaceFileText(context.Context,string,string,string)(string,error)
 RunTask(context.Context,int)(string,error)
 RunTaskText(context.Context,int)(string,error)
 RunTaskReport(context.Context,int)(string,error)
 StartManagedTask(context.Context,int)(string,error)
 StatusManagedTask(context.Context,string)(string,error)
 ReadManagedOutput(context.Context,string,int64)(string,error)
 CancelManagedTask(context.Context,string)(string,error)
}

func validConsolePath(path string)bool {
 if path==""||len(path)>120||path=="."||!utf8.ValidString(path)||
  strings.HasPrefix(path,"/")||strings.Contains(path,"..")||
  strings.ContainsAny(path,"\\\x00\r\n:") {return false}
 for _,r:=range path {
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp) {
   return false
  }
 }
 return true
}

// consolePath resolves client-side "cd" navigation. Never forward "..",
// absolute paths, command syntax or an untrusted working directory to Agent.
// "cd" is a local display preference, NOT a server-side process cwd or shell.
func consolePath(cwd, input string, directory bool)(string,error) {
 input=strings.TrimSpace(input)
 if input==""||len(input)>256||!utf8.ValidString(input)||
  strings.HasPrefix(input,"/")||strings.HasPrefix(input,"--")||
  strings.ContainsAny(input,"\\\x00\r\n:") {
  return "",errors.New("đường dẫn remote không hợp lệ")
 }
 parts:=[]string{}
 if cwd!=""&&cwd!="." {parts=append(parts,strings.Split(cwd,"/")...)}
 for _,component:=range strings.Split(input,"/") {
  switch component {
  case "": return "",errors.New("đường dẫn remote không hợp lệ")
  case ".": continue
  case "..":
   if len(parts)==0{return "",errors.New("không thể thoát khỏi workspace root")}
   parts=parts[:len(parts)-1]
  default:
   if strings.Contains(component,"..") {
    return "",errors.New("đường dẫn remote không hợp lệ")
   }
   parts=append(parts,component)
  }
 }
 resolved:=strings.Join(parts,"/")
 if resolved=="" {
  if directory{return ".",nil}
  return "",errors.New("cần chỉ định tên file")
 }
 if !validConsolePath(resolved) {
  return "",errors.New("đường dẫn remote quá dài hoặc không hợp lệ")
 }
 return resolved,nil
}

// ReadSlice bounds the memory consumed by a pasted/untrusted TTY line.
// ReadString would allocate an arbitrarily long buffer BEFORE its length check.
func consoleLine(in *bufio.Reader,limit int)(string,error) {
 if in==nil||limit<1||limit>4096{return "",errors.New("invalid console input limit")}
 // Read at most limit+1 bytes, without allocating an unbounded line.
 // A too-long line terminates the session; the remainder is never parsed
 // as a second command or a confirmation response.
 data:=make([]byte,0,256)
 for {
  b,err:=in.ReadByte()
  if errors.Is(err,io.EOF){
   if len(data)==0{return "",io.EOF}
   break
  }
  if err!=nil{return "",err}
  if b=='\n'{break}
  if len(data)>=limit{return "",errors.New("console input vượt giới hạn, phiên đã dừng")}
  data=append(data,b)
 }
 return strings.TrimSuffix(string(data),"\r"),nil
}

// The caller always verifies a complete SHA-pinned E2EE preview before
// opening this editor. No local shell/editor executable, file download or
// environment-dependent subprocess is used. Lines are literal text.
func consoleReplacement(in *bufio.Reader,out io.Writer)(string,bool,error){
 fmt.Fprintln(out,"Nhập văn bản UTF-8 (tối đa 4096 byte).")
 fmt.Fprintln(out,"Dòng .save kết thúc và giữ newline cuối, .save-noeol bỏ newline cuối, .cancel hủy.")
 fmt.Fprintln(out,"Nếu cần viết nguyên dòng .save/.cancel, thêm dấu chấm đầu dòng (..save).")
 var lines []string
 used:=0
 for {
  line,err:=consoleLine(in,4096)
  if err!=nil{return "",false,err}
  if line==".cancel"{return "",false,nil}
  if line==".save"||line==".save-noeol"{
   content:=strings.Join(lines,"\n")
   if line==".save"&&len(lines)>0{
    if len(content)>=4096{return "",false,errors.New("nội dung edit vượt quá 4096 byte")}
    content+="\n"
   }
   return content,true,nil
  }
  if strings.HasPrefix(line,".."){line=line[1:]}
  if !utf8.ValidString(line)||strings.ContainsRune(line,'\x00'){
   return "",false,errors.New("nội dung edit phải là UTF-8 và không chứa NUL")
  }
  // Include newline between every entered line; the last newline is
  // chosen explicitly via .save vs .save-noeol.
  required:=len(line)
  if len(lines)>0{required++}
  if used+required>4096{return "",false,errors.New("nội dung edit vượt quá 4096 byte")}
  used+=required
  lines=append(lines,line)
 }
}

// Display authenticated remote text with readable line breaks. Never pass raw
// terminal escapes, carriage returns, C1 controls or bidi direction overrides
// from a remote repository/task into the operator's terminal.
func consoleDisplayText(out io.Writer, value string) {
 for _, r := range value {
  if r=='\n'||r=='\t' {
   fmt.Fprint(out,string(r))
  } else if r<0x20||(r>=0x7f&&r<=0x9f)||
   unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp) {
   fmt.Fprintf(out,"\\u%04x",r)
  } else {
   fmt.Fprint(out,string(r))
  }
 }
 if !strings.HasSuffix(value,"\n"){fmt.Fprintln(out)}
}

// Treat a search/replacement as literal TEXT, never as a regex, shell
// expression, terminal escape or path. Reject invisible control/directional
// characters in operator input to make confirmation unambiguous.
func validConsoleLiteral(s string, allowEmpty bool) bool {
 if !allowEmpty && s=="" {return false}
 if len(s)>4096 || !utf8.ValidString(s) {return false}
 for _,r:=range s {
  if unicode.IsControl(r) || unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp) {return false}
 }
 return true
}

// Visit each REAL source line while preserving its one-based location.
// A trailing newline never creates a writable phantom line.
func forEachConsoleSourceLine(data string,visit func(int,string)) {
 index:=0
 for start:=0;start<len(data); {
  end:=strings.IndexByte(data[start:],'\n')
  stop:=len(data)
  next:=len(data)
  if end>=0 {stop=start+end;next=stop+1}
  line:=data[start:stop]
  if end>=0 && strings.HasSuffix(line,"\r") {line=line[:len(line)-1]}
  index++
  visit(index,line)
  start=next
 }
}

func consoleDisplayNumberedText(out io.Writer,data string) {
 if data=="" {fmt.Fprintln(out,"(file rỗng)");return}
 forEachConsoleSourceLine(data,func(index int,line string){
  fmt.Fprintf(out,"%d: ",index)
  consoleDisplayText(out,line)
 })
}

// Replace only the bytes of an EXISTING line, preserving every other byte
// including LF vs CRLF and any trailing newline. The host still re-reads
// the complete file and uses its own SHA-256 CAS + mutation lock on commit.
func consoleReplaceLine(data string,index int,newLine string)(string,string,error) {
 if index<1||len(data)>4096||!utf8.ValidString(data)||
  !validConsoleLiteral(newLine,true) {
  return "","",errors.New("invalid authenticated line edit")
 }
 lineNumber:=0
 for start:=0;start<len(data); {
  stop:=len(data)
  next:=len(data)
  if rel:=strings.IndexByte(data[start:],'\n');rel>=0 {
   stop=start+rel
   next=stop+1
  }
  contentEnd:=stop
  if next>stop&&contentEnd>start&&data[contentEnd-1]=='\r' {
   contentEnd--
  }
  lineNumber++
  if lineNumber==index {
   original:=data[start:contentEnd]
   result:=data[:start]+newLine+data[contentEnd:]
   if len(result)>4096 {
    return "","",errors.New("line edit exceeds 4096 bytes")
   }
   if result==data {
    return "","",errors.New("no line changes")
   }
   return original,result,nil
  }
  start=next
 }
 return "","",errors.New("line number outside existing file")
}

// A catalogue is a host-authorized, complete 4KiB E2EE snapshot of
// "canonical-decimal-id<TAB>label<LF>" entries. Fail closed on any
// ambiguous/malformed record, including duplicates, terminal controls
// and embedded line breaks. This is ONLY an operator preview: the host
// rechecks the full digest-approved TaskDeck task before executing it.
func consoleApprovedTaskLabel(catalogue string,requested int)(string,error){
 denied:=errors.New("Danh mục task host không hợp lệ hoặc task chưa được phê duyệt")
 if requested<1||requested>0x7fffffff||len(catalogue)==0||
  len(catalogue)>4096||!utf8.ValidString(catalogue)||
  !strings.HasSuffix(catalogue,"\n"){return "",denied}
 seen:=make(map[int]bool)
 label:=""
 for _,line:=range strings.Split(strings.TrimSuffix(catalogue,"\n"),"\n"){
  fields:=strings.Split(line,"\t")
  if len(fields)!=2||fields[0]==""||fields[1]==""||len(fields[1])>120{
   return "",denied
  }
  id,e:=strconv.Atoi(fields[0])
  if e!=nil||id<1||id>0x7fffffff||
   strconv.Itoa(id)!=fields[0]||seen[id]{return "",denied}
  seen[id]=true
  for _,c:=range fields[1] {
   if unicode.IsControl(c)||unicode.In(c,unicode.Cf,unicode.Zl,unicode.Zp){
    return "",denied
   }
  }
  if id==requested{label=fields[1]}
 }
 if label==""{return "",denied}
 return label,nil
}

// Parse the authenticated host line-page envelope before printing any
// content, and only ever combine pages from the same whole-file SHA.
func consoleLinePageSHA(page string,start int64)(string,error){
 invalid:=errors.New("Trang file không hợp lệ; không hiển thị nội dung")
 if start<1||len(page)>4096||!utf8.ValidString(page){return "",invalid}
 chunks:=strings.SplitN(page,"\n",6)
 if len(chunks)!=6||!strings.HasPrefix(chunks[0],"FILE_SHA256 ")||
  !strings.HasPrefix(chunks[1],"START_LINE ")||
  !strings.HasPrefix(chunks[2],"NEXT_LINE ")||
  !strings.HasPrefix(chunks[3],"TOTAL_LINES ")||
  chunks[4]!="-- COMPLETE SOURCE LINES --"{return "",invalid}
 sha:=strings.TrimPrefix(chunks[0],"FILE_SHA256 ")
 if len(sha)!=64{return "",invalid}
 for _,ch:=range sha{
  if !((ch>='0'&&ch<='9')||(ch>='a'&&ch<='f')){return "",invalid}
 }
 getNumber:=func(header,prefix string)(int64,bool){
  value:=strings.TrimPrefix(header,prefix)
  n,e:=strconv.ParseInt(value,10,64)
  return n,e==nil&&n>=0&&strconv.FormatInt(n,10)==value
 }
 first,ok:=getNumber(chunks[1],"START_LINE ")
 if !ok||first!=start{return "",invalid}
 next,ok:=getNumber(chunks[2],"NEXT_LINE ")
 if !ok{return "",invalid}
 total,ok:=getNumber(chunks[3],"TOTAL_LINES ")
 if !ok||total<first||total>2<<20{return "",invalid}
 if !strings.HasSuffix(chunks[5],"\n"){return "",invalid}
 rows:=strings.Split(strings.TrimSuffix(chunks[5],"\n"),"\n")
 if len(rows)==0{return "",invalid}
 for i,row:=range rows {
  expected:=strconv.FormatInt(first+int64(i),10)+": "
  if !strings.HasPrefix(row,expected){return "",invalid}
 }
 last:=first+int64(len(rows))-1
 if last>total||(next!=0&&next!=last+1)||
  (next==0&&last!=total)||(next!=0&&last>=total){return "",invalid}
 return sha,nil
}

// Verify the COMPLETE host search response before exposing any source
// lines in the terminal. The searched term itself is never echoed from PHP.
func consoleFindLinesMetadata(report,term string)(string,error){
 denied:=errors.New("Kết quả tìm kiếm không nguyên vẹn hoặc sai truy vấn")
 if len(report)>4096||!utf8.ValidString(report)||
  !remotetransport.ValidRemoteFindLiteral(term){return "",denied}
 chunks:=strings.SplitN(report,"\n",6)
 if len(chunks)!=6||!strings.HasPrefix(chunks[0],"FILE_SHA256 ")||
  !strings.HasPrefix(chunks[1],"TERM_SHA256 ")||
  !strings.HasPrefix(chunks[2],"TOTAL_LINES ")||
  !strings.HasPrefix(chunks[3],"MATCHES ")||
  chunks[4]!="-- COMPLETE MATCHING LINES --"{return "",denied}
 fileSHA:=strings.TrimPrefix(chunks[0],"FILE_SHA256 ")
 if len(fileSHA)!=64{return "",denied}
 for _,ch:=range fileSHA{
  if !((ch>='0'&&ch<='9')||(ch>='a'&&ch<='f')){return "",denied}
 }
 wanted:=sha256.Sum256([]byte(term))
 if strings.TrimPrefix(chunks[1],"TERM_SHA256 ")!=hex.EncodeToString(wanted[:]){
  return "",denied
 }
 parseCanonical:=func(v string)(int64,bool){
  n,e:=strconv.ParseInt(v,10,64)
  return n,e==nil&&n>=0&&n<=2<<20&&strconv.FormatInt(n,10)==v
 }
 total,ok:=parseCanonical(strings.TrimPrefix(chunks[2],"TOTAL_LINES "))
 if !ok{return "",denied}
 matches,ok:=parseCanonical(strings.TrimPrefix(chunks[3],"MATCHES "))
 if !ok||matches>total{return "",denied}
 if matches==0 {
  if chunks[5]!=""{return "",denied}
  return fileSHA,nil
 }
 if !strings.HasSuffix(chunks[5],"\n"){return "",denied}
 lines:=strings.Split(strings.TrimSuffix(chunks[5],"\n"),"\n")
 if int64(len(lines))!=matches{return "",denied}
 var prev int64
 for _,line:=range lines{
  pos:=strings.Index(line,": ")
  if pos<=0{return "",denied}
  number,ok:=parseCanonical(line[:pos])
  if !ok||number<=prev||number>total||
   !strings.Contains(line[pos+2:],term){return "",denied}
  prev=number
 }
 return fileSHA,nil
}

// Check the entire host E2EE snapshot before prompting a destructive ref edit.
// A preview with missing, spoofed or inconsistent HEAD/index/branch fields
// must never reach a mutation request.
func consoleSafeBranchDeletePreview(reviewed,slug string)(string,error){
 denied:=errors.New("Git branch delete review không nguyên vẹn")
 if !remotetransport.ValidRemoteTaskDeckBranchSlug(slug)||len(reviewed)>1024{return "",denied}
 parts:=strings.Split(reviewed,"\n")
 if len(parts)!=7||parts[6]!=""||
  !strings.HasPrefix(parts[0],"HEAD ")||
  !strings.HasPrefix(parts[1],"BRANCH refs/heads/")||
  parts[2]!="TARGET refs/heads/taskdeck/"+slug||
  !strings.HasPrefix(parts[3],"TARGET_HEAD ")||
  !strings.HasPrefix(parts[4],"INDEX_SHA256 ")||
  parts[5]!="CLEAN yes"||
  parts[1]=="BRANCH refs/heads/taskdeck/"+slug{return "",denied}
 oid:=strings.TrimPrefix(parts[0],"HEAD ")
 idx:=strings.TrimPrefix(parts[4],"INDEX_SHA256 ")
 if (len(oid)!=40&&len(oid)!=64)||len(idx)!=64||
  parts[3]!="TARGET_HEAD "+oid{return "",denied}
 for _,v:=range []string{oid,idx}{
  for _,ch:=range v{
   if !((ch>='0'&&ch<='9')||(ch>='a'&&ch<='f')){return "",denied}
  }
 }
 return oid,nil
}

func consoleHelp()string{
 return "Lệnh E2EE: help | pwd | cd [thư-mục] | ls [đường-dẫn] | view <file> | view-lines <file> <start-line> | view-next | lines <file> | find <file> | find-lines <file> :: <literal> | edit <file> | edit-line <file> | edit-large-line <file> <line> | delete-large-line <file> <line> | insert-large-line <file> <line> | edit-range <file> | move-lines <file> | insert-line <file> | delete-line <file> | replace <file> | replace-all <file> | git status | git log | git branches | git diff <file> | git diff --staged <file> | git diff --head <file> | git review <file> | git stage <file> | git unstage <file> | git commit-review | git commit | git branch-review | git branch-create <slug> | git switch taskdeck/<slug> | git branch-delete taskdeck/<slug> | terminal tasks | terminal task <ID> | terminal task-report <ID> | terminal task-start <ID> | terminal task-output <handle> <offset> | terminal task-status <handle> | terminal task-cancel <handle> | exit\n"+
  "cd/pwd chỉ thay đường dẫn tương đối trên client; không gọi shell, không tốn lượt E2EE.\n"+
  "Tên file/thư mục có dấu cách có thể nhập trực tiếp, không cần đặt trong nháy.\n"+
  "find tìm literal trong file đã xem đầy đủ; replace-all thay tất cả và cần gõ YES <số-vị-trí>.\n"+
  "lines in số dòng; edit-line sửa một dòng, giữ nguyên CRLF và cần xác nhận YES <số-dòng>.\n"+
  "edit-large-line sửa, delete-large-line xóa và insert-large-line chèn TRƯỚC một dòng có thật của file UTF-8 <=2MiB. Mỗi lệnh cần SHA-256 toàn file, 2 lượt E2EE và 3 cờ host.\n"+
  "edit-range thay một vùng dòng nhiều dòng; giữ nguyên phần ngoài vùng, hiển thị toàn bộ nội dung và xác nhận RANGE A-B.\n"+
  "move-lines di chuyển vùng dòng trong một file kết thúc newline; giữ nguyên từng byte và xác nhận MOVE A-B BEFORE N.\n"+
  "insert-line chèn trước dòng N (hoặc N=cuối+1); delete-line xóa dòng N. Xem lại toàn file và gõ INSERT/DELETE N.\n"+
  "replace tìm đúng một chuỗi trên một dòng, chỉ ghi sau khi kiểm tra SHA-256 và gõ YES.\n"+
  "Giới hạn mỗi phiên: 6 thao tác; view/edit đầy đủ tối đa 4KiB, git diff tối đa 4KiB.\n"+
  "view-lines đọc từng trang dòng từ file tối đa 2MiB, <=4KiB mỗi E2EE; chỉ xem, không chỉnh sửa/tải file, phát hiện đổi SHA giữa các trang.\n"+
  "view-next xem trang tiếp theo từ đường dẫn và NEXT_LINE đã được xác thực; không phải shell hoặc file download.\n"+
  "find-lines tìm literal trong file UTF-8 <=2MiB, chỉ trả tất cả dòng khớp nếu vừa đủ 4KiB; từ chối kết quả bị cắt.\n"+
  "git review <file> dùng hai lượt E2EE để xem diff chưa stage và đã stage; không thay đổi Git index hoặc worktree.\n"+
  "git diff --head <file> xem diff giữa HEAD và working tree trong một lượt E2EE (không ghi index).\n"+
  "git stage <file> chỉ stage toàn bộ file <=4KiB sau khi xem trước, xác nhận STAGE <file> và đối chiếu SHA-256 trên host.\n"+
  "git unstage <file> hủy toàn bộ phần đã stage của file, giữ nguyên working tree; yêu cầu diff staged và UNSTAGE <file>.\n"+
  "git commit-review xem HEAD/branch, SHA-256 của Git index và toàn bộ staged diff <=4KiB; tuyệt đối không commit.\n"+
  "git commit thực hiện sau review toàn bộ index, nhập message, xác nhận COMMIT <12-hex-HEAD>; host kiểm tra hash và quyền lần cuối.\n"+
  "git branch-create <slug> tạo refs/heads/taskdeck/<slug> từ HEAD đã review; không checkout, không thay đổi worktree hoặc index.\n"+
  "git switch taskdeck/<slug> chỉ đổi symbolic HEAD nếu branch trỏ CÙNG commit, index/worktree sạch; không checkout commit khác.\n"+
  "git branch-delete taskdeck/<slug> chỉ xóa nhánh taskdeck/ dư thừa cùng HEAD, khác nhánh hiện tại; xác nhận hai bước, không sửa worktree/index.\n"+
  "terminal task-start <ID> tạo task host quản lý tối đa 30 giây; status/cancel qua mã E2EE, cờ riêng trên host.\n"+
  "terminal task-output <handle> <offset> đọc tối đa 2048 byte; dùng OUTPUT_NEXT làm offset kế tiếp, OUTPUT_EOF=yes chỉ khi task kết thúc và không thiếu byte. Đọc trước task-status kết thúc vì status cuối giải phóng mã task.\n"+
  "terminal task <ID> chạy task host phê duyệt; terminal task-report <ID> có thêm mã thoát host; cả hai xác nhận RUN TASK <ID> trước khi chạy, cần 2 lượt E2EE.\n"+
  "Terminal trả về đầy đủ kết quả tối đa 4KiB của task đã duyệt (không cắt đầu log). Terminal PTY/shell tùy ý CHƯA được bật. task phải được host phê duyệt và có TasksRun.\n"
}

// One action per authenticated E2EE round-trip; cd/pwd/help do not use
// network grants. Invalid path input is rejected locally, before E2EE.
func runNativeInteractiveConsole(ctx context.Context,remote remoteConsoleAction,
 in *bufio.Reader,out io.Writer)error{
 if ctx==nil||remote==nil||in==nil||out==nil{return errors.New("missing authenticated native console")}
 fmt.Fprint(out,consoleHelp())
 used:=0
 cwd:="."
 linePageVersions:=make(map[string]string)
 lastPagePath:=""
 var nextPageLine int64
 for ctx.Err()==nil {
  if used>=remoterelay.MaxAppPingRounds{
   fmt.Fprintln(out,"Phiên đã dùng hết 6 lượt E2EE; đăng nhập lại bằng ticket mới.")
   return nil
  }
  fmt.Fprintf(out,"TaskDeck E2EE [%d/%d] %s> ",used,remoterelay.MaxAppPingRounds,cwd)
  line,err:=consoleLine(in,256)
  if errors.Is(err,io.EOF){return nil}
  if err!=nil{return err}
  line=strings.TrimSpace(line)
  if line==""{continue}
  if line=="exit"{return nil}
  if line=="help"{fmt.Fprint(out,consoleHelp());continue}
  if line=="pwd"{fmt.Fprintln(out,cwd);continue}
  if line=="cd"{cwd=".";fmt.Fprintln(out,cwd);continue}
  if strings.HasPrefix(line,"cd "){
   next,e:=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"cd ")),true)
   if e!=nil{fmt.Fprintln(out,e);continue}
   cwd=next
   fmt.Fprintln(out,cwd)
   continue
  }
  var result string
  var target string
  var e error
  switch {
  case line=="ls":
   result,err=remote.ListDirectory(ctx,cwd)
   used++
  case strings.HasPrefix(line,"ls "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"ls ")),true)
   if e!=nil{fmt.Fprintln(out,e);continue}
   result,err=remote.ListDirectory(ctx,target)
   used++
  case line=="view-next" || strings.HasPrefix(line,"view-lines "):
   if line=="view-next" {
    if lastPagePath==""||nextPageLine<1{
     fmt.Fprintln(out,"Không có trang tiếp theo đã xác thực. Hãy dùng view-lines <file> 1.")
     continue
    }
    target=lastPagePath
   }
   raw:=""
   separator:=-1
   if line!="view-next" {
    raw=strings.TrimSpace(strings.TrimPrefix(line,"view-lines "))
    separator=strings.LastIndexByte(raw,' ')
   }
   var first int64
   if line=="view-next" {
    first=nextPageLine
   } else {
    if separator<1{
     fmt.Fprintln(out,"Cú pháp: view-lines <file> <start-line>.")
     continue
    }
    requestedPath:=strings.TrimSpace(raw[:separator])
    number:=raw[separator+1:]
    start,parseErr:=strconv.ParseInt(number,10,64)
    if parseErr!=nil||start<1||start>2<<20||
     strconv.FormatInt(start,10)!=number{
     fmt.Fprintln(out,"Số dòng bắt đầu không hợp lệ.")
     continue
    }
    first=start
    target,e=consolePath(cwd,requestedPath,false)
    if e!=nil{fmt.Fprintln(out,e);continue}
   }
   result,err=remote.ReadFileLines(ctx,target,first)
   used++
   if err!=nil{break}
   hash,parseErr:=consoleLinePageSHA(result,first)
   if parseErr!=nil{
    err=parseErr
    break
   }
   if previous,exists:=linePageVersions[target];exists&&previous!=hash{
    nextPageLine=0
    fmt.Fprintf(out,"File %s đã đổi SHA giữa các trang; mở phiên mới để xem phiên bản mới.\n",target)
    continue
   }
   // NEXT_LINE was verified by consoleLinePageSHA: it is either zero at
   // EOF or exactly one after the last complete numbered source line.
   headers:=strings.SplitN(result,"\n",4)
   if len(headers)!=4||!strings.HasPrefix(headers[2],"NEXT_LINE "){
    err=errors.New("Thiếu NEXT_LINE đã xác thực")
    break
   }
   next,parseErr:=strconv.ParseInt(strings.TrimPrefix(headers[2],"NEXT_LINE "),10,64)
   if parseErr!=nil||next<0||next>2<<20{
    err=errors.New("NEXT_LINE không hợp lệ")
    break
   }
   linePageVersions[target]=hash
   lastPagePath=target
   nextPageLine=next
  case strings.HasPrefix(line,"view "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"view ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   result,err=remote.ReadFilePreview(ctx,target)
   used++
  case strings.HasPrefix(line,"lines "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"lines ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   result,err=remote.ReadFilePreview(ctx,target)
   used++
   if err!=nil{break}
   fmt.Fprintf(out,"Dòng của %s:\n",target)
   consoleDisplayNumberedText(out,result)
   continue
  case strings.HasPrefix(line,"find-lines "):
   raw:=strings.TrimPrefix(line,"find-lines ")
   parts:=strings.SplitN(raw," :: ",2)
   if len(parts)!=2||strings.TrimSpace(parts[0])!=parts[0]||
    !remotetransport.ValidRemoteFindLiteral(parts[1]){
    fmt.Fprintln(out,"Cú pháp: find-lines <file> :: <literal UTF-8 (1–96 bytes)>")
    continue
   }
   target,e=consolePath(cwd,parts[0],false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   hidden:=false
   for _,part:=range strings.Split(target,"/"){
    if strings.HasPrefix(part,"."){hidden=true;break}
   }
   if hidden{
    fmt.Fprintln(out,"Từ chối tìm trong đường dẫn ẩn hoặc metadata Git.")
    continue
   }
   result,err=remote.FindFileLines(ctx,target,parts[1])
   used++
   if err!=nil{break}
   sha,parseErr:=consoleFindLinesMetadata(result,parts[1])
   if parseErr!=nil{err=parseErr;break}
   if previous,exists:=linePageVersions[target];exists&&previous!=sha{
    fmt.Fprintln(out,"File đã đổi SHA; không hiển thị kết quả từ phiên bản khác.")
    continue
   }
   linePageVersions[target]=sha
  case strings.HasPrefix(line,"find "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"find ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   previous,readErr:=remote.ReadFilePreview(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   fmt.Fprint(out,"Chuỗi cần tìm chính xác (một dòng): ")
   term,readErr:=consoleLine(in,4096)
   if readErr!=nil{return readErr}
   if !validConsoleLiteral(term,false){
    fmt.Fprintln(out,"Chuỗi tìm không hợp lệ: cần literal UTF-8 không rỗng, không chứa ký tự điều khiển.")
    continue
   }
   matches:=strings.Count(previous,term)
   fmt.Fprintf(out,"Tìm thấy %d vị trí của %q trong %s.\n",matches,term,target)
   if matches>0{
    for i,sourceLine:=range strings.Split(previous,"\n"){
     if strings.Contains(sourceLine,term) {
      fmt.Fprintf(out,"%d: ",i+1)
      consoleDisplayText(out,sourceLine)
     }
    }
   }
  case strings.HasPrefix(line,"edit "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"edit ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Edit cần hai lượt để đọc SHA và ghi CAS; hãy đăng nhập phiên mới.")
    continue
   }
   var previous string
   previous,err=remote.ReadFilePreview(ctx,target)
   used++
   if err!=nil{break}
   fmt.Fprintln(out,"Nội dung hiện tại:")
   consoleDisplayText(out,previous)
   next,accepted,readErr:=consoleReplacement(in,out)
   if readErr!=nil{return readErr}
   if !accepted{
    fmt.Fprintln(out,"Đã hủy edit; chưa gửi thao tác ghi.")
    continue
   }
   fmt.Fprint(out,"Xác nhận ghi file đã kiểm tra SHA? Gõ YES: ")
   confirm,readErr:=consoleLine(in,16)
   if readErr!=nil{return readErr}
   if confirm!="YES"{
    fmt.Fprintln(out,"Đã hủy; chưa gửi thao tác ghi.")
    continue
   }
   hash:=sha256.Sum256([]byte(previous))
   result,err=remote.ReplaceFileText(ctx,target,next,hex.EncodeToString(hash[:]))
   used++
  case strings.HasPrefix(line,"edit-large-line ") || strings.HasPrefix(line,"delete-large-line ") || strings.HasPrefix(line,"insert-large-line "):
   deleteMode:=strings.HasPrefix(line,"delete-large-line ")
   insertMode:=strings.HasPrefix(line,"insert-large-line ")
   command:="edit-large-line "
   if deleteMode{command="delete-large-line "}
   if insertMode{command="insert-large-line "}
   raw:=strings.TrimSpace(strings.TrimPrefix(line,command))
   space:=strings.LastIndexByte(raw,' ')
   if space<1{
    fmt.Fprintln(out,"Cú pháp: "+strings.TrimSpace(command)+" <file> <line>.")
    continue
   }
   number:=raw[space+1:]
   index,parseErr:=strconv.ParseInt(number,10,64)
   if parseErr!=nil||index<1||index>2<<20||
    strconv.FormatInt(index,10)!=number{
    fmt.Fprintln(out,"Số dòng không hợp lệ.")
    continue
   }
   target,e=consolePath(cwd,strings.TrimSpace(raw[:space]),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Sửa file lớn cần hai lượt E2EE; đăng nhập lại.")
    continue
   }
   page,readErr:=remote.ReadFileLines(ctx,target,index)
   used++
   if readErr!=nil{err=readErr;break}
   fileSHA,parseErr:=consoleLinePageSHA(page,index)
   if parseErr!=nil{err=parseErr;break}
   if previous,exists:=linePageVersions[target];exists&&previous!=fileSHA{
    err=errors.New("File thay đổi giữa các trang; phải mở phiên mới")
    break
   }
   chunks:=strings.SplitN(page,"\n",6)
   if len(chunks)!=6{
    err=errors.New("Trang dòng không nguyên vẹn")
    break
   }
   first:=strings.SplitN(chunks[5],"\n",2)[0]
   prefix:=strconv.FormatInt(index,10)+": "
   if !strings.HasPrefix(first,prefix){
    err=errors.New("Dòng nguồn không khớp")
    break
   }
   original:=strings.TrimSuffix(strings.TrimPrefix(first,prefix),"\r")
   if len(original)>3500{
    err=errors.New("Dòng quá dài để chỉnh sửa an toàn")
    break
   }
   if deleteMode{
    fmt.Fprintf(out,"Dòng %d của %s sẽ bị XÓA (giữ nguyên các dòng còn lại):\n",index,target)
    consoleDisplayText(out,original)
    ack:="DELETE LINE "+strconv.FormatInt(index,10)+" "+fileSHA[:12]
    fmt.Fprintf(out,"Gõ chính xác %q để xóa dòng theo SHA-256 của toàn file: ",ack)
    accepted,readErr:=consoleLine(in,96)
    if readErr!=nil{return readErr}
    if accepted!=ack{
     fmt.Fprintln(out,"Đã hủy; không gửi lệnh ghi.")
     continue
    }
    result,err=remote.DeleteLargeFileLine(ctx,target,index,fileSHA)
   }else if insertMode{
    fmt.Fprintf(out,"Chèn một dòng TRƯỚC dòng %d của %s:\n",index,target)
    consoleDisplayText(out,original)
    fmt.Fprint(out,"Dòng sẽ chèn (UTF-8 tối đa 512 bytes): ")
    newLine,readErr:=consoleLine(in,512)
    if readErr!=nil{return readErr}
    if !remotetransport.ValidRemoteLargeLineText(newLine){
     fmt.Fprintln(out,"Dòng chèn chứa ký tự không hợp lệ.")
     continue
    }
    fmt.Fprintln(out,"Nội dung dòng chèn:")
    consoleDisplayText(out,newLine)
    ack:="INSERT LINE "+strconv.FormatInt(index,10)+" "+fileSHA[:12]
    fmt.Fprintf(out,"Gõ chính xác %q để chèn theo SHA-256 của toàn file: ",ack)
    accepted,readErr:=consoleLine(in,96)
    if readErr!=nil{return readErr}
    if accepted!=ack{
     fmt.Fprintln(out,"Đã hủy; không gửi lệnh ghi.")
     continue
    }
    result,err=remote.InsertLargeFileLine(ctx,target,index,newLine,fileSHA)
   }else{
    fmt.Fprintf(out,"Dòng %d của %s trước khi sửa:\n",index,target)
    consoleDisplayText(out,original)
    fmt.Fprint(out,"Nội dung mới (một dòng UTF-8 tối đa 512 bytes): ")
    newLine,readErr:=consoleLine(in,512)
    if readErr!=nil{return readErr}
    if !remotetransport.ValidRemoteLargeLineText(newLine)||newLine==original{
     fmt.Fprintln(out,"Dòng mới không hợp lệ hoặc không có thay đổi.")
     continue
    }
    fmt.Fprintf(out,"Dòng %d sau khi sửa:\n",index)
    consoleDisplayText(out,newLine)
    ack:="EDIT LINE "+strconv.FormatInt(index,10)+" "+fileSHA[:12]
    fmt.Fprintf(out,"Gõ chính xác %q để ghi theo SHA-256 của toàn file: ",ack)
    accepted,readErr:=consoleLine(in,96)
    if readErr!=nil{return readErr}
    if accepted!=ack{
     fmt.Fprintln(out,"Đã hủy; không gửi lệnh ghi.")
     continue
    }
    result,err=remote.EditLargeFileLine(ctx,target,index,newLine,fileSHA)
   }
   used++
   if err==nil {
    // The write changes the source hash. Never reuse the stale line-page
    // cursor; a later view-lines obtains a fresh authenticated snapshot.
    delete(linePageVersions,target)
    if lastPagePath==target{lastPagePath="";nextPageLine=0}
   }
  case strings.HasPrefix(line,"edit-line "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"edit-line ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Edit-line cần hai lượt (preview và SHA-256 CAS); hãy đăng nhập phiên mới.")
    continue
   }
   previous,readErr:=remote.ReadFilePreview(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   fmt.Fprintf(out,"Dòng của %s:\n",target)
   consoleDisplayNumberedText(out,previous)
   fmt.Fprint(out,"Số dòng cần sửa: ")
   rawIndex,readErr:=consoleLine(in,16)
   if readErr!=nil{return readErr}
   index,parseErr:=strconv.Atoi(rawIndex)
   if parseErr!=nil||index<1 {
    fmt.Fprintln(out,"Số dòng không hợp lệ; file chưa thay đổi.")
    continue
   }
   fmt.Fprint(out,"Nội dung dòng thay thế (một dòng, được phép rỗng): ")
   newLine,readErr:=consoleLine(in,4096)
   if readErr!=nil{return readErr}
   original,next,editErr:=consoleReplaceLine(previous,index,newLine)
   if editErr!=nil {
    fmt.Fprintln(out,"Không thể sửa dòng: ",editErr)
    continue
   }
   fmt.Fprintf(out,"Dòng %d trước:\n",index)
   consoleDisplayText(out,original)
   fmt.Fprintf(out,"Dòng %d sau:\n",index)
   consoleDisplayText(out,newLine)
   fmt.Fprintf(out,"Xác nhận ghi một dòng bằng SHA-256 CAS? Gõ YES %d: ",index)
   confirm,readErr:=consoleLine(in,32)
   if readErr!=nil{return readErr}
   if confirm!=fmt.Sprintf("YES %d",index) {
    fmt.Fprintln(out,"Đã hủy; không gửi lệnh ghi.")
    continue
   }
   hash:=sha256.Sum256([]byte(previous))
   result,err=remote.ReplaceFileText(ctx,target,next,hex.EncodeToString(hash[:]))
   used++
  case strings.HasPrefix(line,"edit-range "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"edit-range ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Edit-range cần hai lượt E2EE (preview và SHA-256 CAS); đăng nhập lại.")
    continue
   }
   previous,readErr:=remote.ReadFilePreview(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   spans,spanErr:=consoleSourceLines(previous)
   if spanErr!=nil{return spanErr}
   fmt.Fprintf(out,"Các dòng của %s:\n",target)
   consoleDisplayNumberedText(out,previous)
   if len(spans)==0{
    fmt.Fprintln(out,"File rỗng chưa có vùng dòng; dùng edit để thay toàn file.")
    continue
   }
   fmt.Fprintf(out,"Dòng bắt đầu (1..%d): ",len(spans))
   rawFirst,readErr:=consoleLine(in,16)
   if readErr!=nil{return readErr}
   fmt.Fprintf(out,"Dòng kết thúc (1..%d): ",len(spans))
   rawLast,readErr:=consoleLine(in,16)
   if readErr!=nil{return readErr}
   first,firstErr:=strconv.Atoi(rawFirst)
   last,lastErr:=strconv.Atoi(rawLast)
   if firstErr!=nil||lastErr!=nil||first<1||last<first||last>len(spans){
    fmt.Fprintln(out,"Vùng dòng không hợp lệ; không gửi lệnh ghi.")
    continue
   }
   fmt.Fprintln(out,"Nhập văn bản cho vùng này. .save-noeol kết thúc sau dòng cuối; .save thêm một dòng trống; .cancel hủy.")
   replacement,accepted,readErr:=consoleReplacement(in,out)
   if readErr!=nil{return readErr}
   if !accepted{
    fmt.Fprintln(out,"Đã hủy edit-range; không gửi lệnh ghi.")
    continue
   }
   next,mutationErr:=consoleReplaceLineRange(previous,first,last,replacement)
   if mutationErr!=nil{
    fmt.Fprintln(out,"Không thể thay vùng dòng: ",mutationErr)
    continue
   }
   fmt.Fprintln(out,"Toàn bộ nội dung dự kiến:")
   consoleDisplayNumberedText(out,next)
   fmt.Fprintf(out,"Xác nhận thay dòng %d..%d bằng SHA-256 CAS? Gõ RANGE %d-%d: ",first,last,first,last)
   confirmation,readErr:=consoleLine(in,32)
   if readErr!=nil{return readErr}
   if confirmation!=fmt.Sprintf("RANGE %d-%d",first,last){
    fmt.Fprintln(out,"Đã hủy edit-range; chưa gửi lệnh ghi.")
    continue
   }
   hash:=sha256.Sum256([]byte(previous))
   result,err=remote.ReplaceFileText(ctx,target,next,hex.EncodeToString(hash[:]))
   used++
  case strings.HasPrefix(line,"move-lines "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"move-lines ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Move-lines cần hai lượt E2EE (preview và SHA-256 CAS); đăng nhập lại.")
    continue
   }
   previous,readErr:=remote.ReadFilePreview(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   spans,spanErr:=consoleSourceLines(previous)
   if spanErr!=nil{return spanErr}
   fmt.Fprintf(out,"Các dòng của %s:\n",target)
   consoleDisplayNumberedText(out,previous)
   if len(spans)==0 || spans[len(spans)-1].ending==""{
    fmt.Fprintln(out,"Move-lines chỉ hỗ trợ file có newline cuối; không gửi lệnh ghi.")
    continue
   }
   fmt.Fprintf(out,"Dòng đầu tiên cần chuyển (1..%d): ",len(spans))
   rawFirst,readErr:=consoleLine(in,16)
   if readErr!=nil{return readErr}
   fmt.Fprintf(out,"Dòng cuối cùng cần chuyển (1..%d): ",len(spans))
   rawLast,readErr:=consoleLine(in,16)
   if readErr!=nil{return readErr}
   fmt.Fprintf(out,"Chèn trước dòng (1..%d; %d = nối cuối): ",len(spans)+1,len(spans)+1)
   rawBefore,readErr:=consoleLine(in,16)
   if readErr!=nil{return readErr}
   first,firstErr:=strconv.Atoi(rawFirst)
   last,lastErr:=strconv.Atoi(rawLast)
   before,beforeErr:=strconv.Atoi(rawBefore)
   if firstErr!=nil||lastErr!=nil||beforeErr!=nil{
    fmt.Fprintln(out,"Số dòng không hợp lệ; không gửi lệnh ghi.")
    continue
   }
   next,moveErr:=consoleMoveLines(previous,first,last,before)
   if moveErr!=nil{
    fmt.Fprintln(out,"Không thể di chuyển dòng: ",moveErr)
    continue
   }
   fmt.Fprintln(out,"Toàn bộ nội dung dự kiến:")
   consoleDisplayNumberedText(out,next)
   expected:=fmt.Sprintf("MOVE %d-%d BEFORE %d",first,last,before)
   fmt.Fprintf(out,"Xác nhận SHA-256 CAS? Gõ chính xác %q: ",expected)
   confirmation,readErr:=consoleLine(in,64)
   if readErr!=nil{return readErr}
   if confirmation!=expected{
    fmt.Fprintln(out,"Đã hủy move-lines; chưa gửi lệnh ghi.")
    continue
   }
   hash:=sha256.Sum256([]byte(previous))
   result,err=remote.ReplaceFileText(ctx,target,next,hex.EncodeToString(hash[:]))
   used++
  case strings.HasPrefix(line,"insert-line "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"insert-line ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Insert-line cần hai lượt E2EE (preview và CAS); đăng nhập lại.")
    continue
   }
   previous,readErr:=remote.ReadFilePreview(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   fmt.Fprintf(out,"Các dòng hiện tại của %s:\n",target)
   consoleDisplayNumberedText(out,previous)
   spans,spanErr:=consoleSourceLines(previous)
   if spanErr!=nil{return spanErr}
   fmt.Fprintf(out,"Chèn trước dòng (1..%d; %d là nối cuối): ",len(spans)+1,len(spans)+1)
   rawIndex,readErr:=consoleLine(in,16)
   if readErr!=nil{return readErr}
   index,parseErr:=strconv.Atoi(rawIndex)
   if parseErr!=nil||index<1||index>len(spans)+1 {
    fmt.Fprintln(out,"Số dòng không hợp lệ; không gửi lệnh ghi.")
    continue
   }
   fmt.Fprint(out,"Nội dung dòng cần chèn (một dòng): ")
   newLine,readErr:=consoleLine(in,4096)
   if readErr!=nil{return readErr}
   next,mutationErr:=consoleInsertLine(previous,index,newLine)
   if mutationErr!=nil{
    fmt.Fprintln(out,"Không thể chèn dòng: ",mutationErr)
    continue
   }
   fmt.Fprintln(out,"Toàn bộ nội dung dự kiến:")
   consoleDisplayNumberedText(out,next)
   fmt.Fprintf(out,"Xác nhận chèn dòng bằng SHA-256 CAS? Gõ INSERT %d: ",index)
   confirmation,readErr:=consoleLine(in,32)
   if readErr!=nil{return readErr}
   if confirmation!=fmt.Sprintf("INSERT %d",index){
    fmt.Fprintln(out,"Đã hủy chèn; chưa gửi lệnh ghi.")
    continue
   }
   hash:=sha256.Sum256([]byte(previous))
   result,err=remote.ReplaceFileText(ctx,target,next,hex.EncodeToString(hash[:]))
   used++
  case strings.HasPrefix(line,"delete-line "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"delete-line ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Delete-line cần hai lượt E2EE (preview và CAS); đăng nhập lại.")
    continue
   }
   previous,readErr:=remote.ReadFilePreview(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   fmt.Fprintf(out,"Các dòng hiện tại của %s:\n",target)
   consoleDisplayNumberedText(out,previous)
   fmt.Fprint(out,"Số dòng cần xóa: ")
   rawIndex,readErr:=consoleLine(in,16)
   if readErr!=nil{return readErr}
   index,parseErr:=strconv.Atoi(rawIndex)
   if parseErr!=nil||index<1{
    fmt.Fprintln(out,"Số dòng không hợp lệ; không gửi lệnh ghi.")
    continue
   }
   removed,next,mutationErr:=consoleDeleteLine(previous,index)
   if mutationErr!=nil{
    fmt.Fprintln(out,"Không thể xóa dòng: ",mutationErr)
    continue
   }
   fmt.Fprintf(out,"Dòng %d sẽ bị xóa:\n",index)
   consoleDisplayText(out,removed)
   fmt.Fprintln(out,"Toàn bộ nội dung dự kiến sau khi xóa:")
   consoleDisplayNumberedText(out,next)
   fmt.Fprintf(out,"Xác nhận XÓA dòng bằng SHA-256 CAS? Gõ DELETE %d: ",index)
   confirmation,readErr:=consoleLine(in,32)
   if readErr!=nil{return readErr}
   if confirmation!=fmt.Sprintf("DELETE %d",index){
    fmt.Fprintln(out,"Đã hủy xóa; chưa gửi lệnh ghi.")
    continue
   }
   hash:=sha256.Sum256([]byte(previous))
   result,err=remote.ReplaceFileText(ctx,target,next,hex.EncodeToString(hash[:]))
   used++
  case strings.HasPrefix(line,"replace "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"replace ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Replace cần hai lượt (preview và ghi CAS); hãy đăng nhập phiên mới.")
    continue
   }
   previous,readErr:=remote.ReadFilePreview(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   fmt.Fprintln(out,"Nội dung hiện tại:")
   consoleDisplayText(out,previous)
   fmt.Fprint(out,"Chuỗi cần tìm chính xác (một dòng, không rỗng): ")
   old,readErr:=consoleLine(in,4096)
   if readErr!=nil{return readErr}
   fmt.Fprint(out,"Chuỗi thay thế (một dòng, được phép rỗng): ")
   replacement,readErr:=consoleLine(in,4096)
   if readErr!=nil{return readErr}
   if !validConsoleLiteral(old,false)||!validConsoleLiteral(replacement,true){
    fmt.Fprintln(out,"Chuỗi tìm/thay phải là UTF-8 không chứa NUL; chuỗi tìm không được rỗng.")
    continue
   }
   if strings.Count(previous,old)!=1{
    fmt.Fprintln(out,"Từ chối: chuỗi tìm phải xuất hiện đúng một lần; file chưa thay đổi.")
    continue
   }
   next:=strings.Replace(previous,old,replacement,1)
   if len(next)>4096{
    fmt.Fprintln(out,"Từ chối: nội dung sau thay thế vượt giới hạn 4096 byte.")
    continue
   }
   fmt.Fprintln(out,"Nội dung dự kiến:")
   consoleDisplayText(out,next)
   fmt.Fprint(out,"Xác nhận thay đúng một chỗ bằng SHA-256 CAS? Gõ YES: ")
   confirm,readErr:=consoleLine(in,16)
   if readErr!=nil{return readErr}
   if confirm!="YES"{
    fmt.Fprintln(out,"Đã hủy; không gửi lệnh ghi.")
    continue
   }
   hash:=sha256.Sum256([]byte(previous))
   result,err=remote.ReplaceFileText(ctx,target,next,hex.EncodeToString(hash[:]))
   used++
  case strings.HasPrefix(line,"replace-all "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"replace-all ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Replace-all cần hai lượt (preview và ghi CAS); hãy đăng nhập phiên mới.")
    continue
   }
   previous,readErr:=remote.ReadFilePreview(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   fmt.Fprintln(out,"Nội dung hiện tại:")
   consoleDisplayText(out,previous)
   fmt.Fprint(out,"Chuỗi cần tìm chính xác (một dòng, không rỗng): ")
   old,readErr:=consoleLine(in,4096)
   if readErr!=nil{return readErr}
   fmt.Fprint(out,"Chuỗi thay thế (một dòng, được phép rỗng): ")
   replacement,readErr:=consoleLine(in,4096)
   if readErr!=nil{return readErr}
   if !validConsoleLiteral(old,false)||!validConsoleLiteral(replacement,true){
    fmt.Fprintln(out,"Chuỗi tìm/thay phải là literal UTF-8 không chứa ký tự điều khiển.")
    continue
   }
   count:=strings.Count(previous,old)
   if count==0{
    fmt.Fprintln(out,"Không tìm thấy chuỗi cần thay; file chưa thay đổi.")
    continue
   }
   next:=strings.ReplaceAll(previous,old,replacement)
   if len(next)>4096||next==previous{
    fmt.Fprintln(out,"Từ chối: kết quả quá 4096 byte hoặc không có thay đổi.")
    continue
   }
   fmt.Fprintf(out,"Sẽ thay %d vị trí trong %s.\nNội dung dự kiến:\n",count,target)
   consoleDisplayText(out,next)
   confirmation:=fmt.Sprintf("YES %d",count)
   fmt.Fprintf(out,"Xác nhận ghi SHA-256 CAS? Gõ chính xác %q: ",confirmation)
   confirm,readErr:=consoleLine(in,32)
   if readErr!=nil{return readErr}
   if confirm!=confirmation{
    fmt.Fprintln(out,"Đã hủy; không gửi lệnh ghi.")
    continue
   }
   hash:=sha256.Sum256([]byte(previous))
   result,err=remote.ReplaceFileText(ctx,target,next,hex.EncodeToString(hash[:]))
   used++
  case strings.HasPrefix(line,"git branch-delete taskdeck/"):
   slug:=strings.TrimPrefix(line,"git branch-delete taskdeck/")
   if !remotetransport.ValidRemoteTaskDeckBranchSlug(slug){
    fmt.Fprintln(out,"Branch không hợp lệ; chỉ nhận refs/heads/taskdeck/<slug>.")
    continue
   }
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Xóa branch cần hai lượt E2EE; đăng nhập lại.")
    continue
   }
   reviewed,readErr:=remote.GitBranchDeleteReview(ctx,slug)
   used++
   if readErr!=nil{err=readErr;break}
   oid,parseErr:=consoleSafeBranchDeletePreview(reviewed,slug)
   if parseErr!=nil{err=parseErr;break}
   fmt.Fprintln(out,"Chỉ xóa ref taskdeck/ cùng HEAD đang không checkout; không sửa commit, index hoặc files.")
   consoleDisplayText(out,reviewed)
   ack:="DELETE BRANCH taskdeck/"+slug+" "+oid[:12]
   fmt.Fprintf(out,"Gõ chính xác %q để xóa branch dư thừa: ",ack)
   confirmation,readErr:=consoleLine(in,120)
   if readErr!=nil{return readErr}
   if confirmation!=ack{
    fmt.Fprintln(out,"Đã hủy; không gửi lệnh xóa branch.")
    continue
   }
   h:=sha256.Sum256([]byte(reviewed))
   result,err=remote.GitBranchDelete(ctx,slug,hex.EncodeToString(h[:]))
   used++
  case strings.HasPrefix(line,"git switch taskdeck/"):
   slug:=strings.TrimPrefix(line,"git switch taskdeck/")
   if !remotetransport.ValidRemoteTaskDeckBranchSlug(slug){
    fmt.Fprintln(out,"Branch đích không hợp lệ; chỉ chấp nhận taskdeck/<slug> chữ thường.")
    continue
   }
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Git switch cần hai lượt E2EE; đăng nhập lại để tránh thiếu review.")
    continue
   }
   reviewed,readErr:=remote.GitSwitchReview(ctx,slug)
   used++
   if readErr!=nil{err=readErr;break}
   chunks:=strings.Split(reviewed,"\n")
   if len(chunks)!=7||chunks[6]!=""||
    !strings.HasPrefix(chunks[0],"HEAD ")||
    !strings.HasPrefix(chunks[1],"BRANCH refs/heads/")||
    chunks[2]!="TARGET refs/heads/taskdeck/"+slug||
    !strings.HasPrefix(chunks[3],"TARGET_HEAD ")||
    !strings.HasPrefix(chunks[4],"INDEX_SHA256 ")||
    chunks[5]!="CLEAN yes"{
    err=errors.New("Git switch review không nguyên vẹn")
    break
   }
   oid:=strings.TrimPrefix(chunks[0],"HEAD ")
   targetOID:=strings.TrimPrefix(chunks[3],"TARGET_HEAD ")
   indexSHA:=strings.TrimPrefix(chunks[4],"INDEX_SHA256 ")
   if (len(oid)!=40&&len(oid)!=64)||oid!=targetOID||
    len(indexSHA)!=64||len(reviewed)>1024{
    err=errors.New("HEAD/target/index review không hợp lệ")
    break
   }
   for _,v:=range []string{oid,indexSHA}{
    for _,c:=range v{
     if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){
      err=errors.New("SHA review không hợp lệ")
      break
     }
    }
   }
   if err!=nil{break}
   if chunks[1]=="BRANCH refs/heads/taskdeck/"+slug{
    fmt.Fprintln(out,"Đang ở đúng branch, không cần switch.")
    continue
   }
   fmt.Fprintln(out,"Git same-HEAD switch: KHÔNG checkout hoặc sửa file; cần index và worktree sạch.")
   consoleDisplayText(out,reviewed)
   ack:="SWITCH taskdeck/"+slug+" "+oid[:12]
   fmt.Fprintf(out,"Gõ chính xác %q để đổi symbolic HEAD: ",ack)
   confirmation,readErr:=consoleLine(in,120)
   if readErr!=nil{return readErr}
   if confirmation!=ack{
    fmt.Fprintln(out,"Đã hủy; không gửi yêu cầu ghi HEAD.")
    continue
   }
   h:=sha256.Sum256([]byte(reviewed))
   result,err=remote.GitSwitchSameHead(ctx,slug,hex.EncodeToString(h[:]))
   used++
  case line=="git branch-review":
   result,err=remote.GitBranchReview(ctx)
   used++
  case strings.HasPrefix(line,"git branch-create "):
   slug:=strings.TrimPrefix(line,"git branch-create ")
   if !remotetransport.ValidRemoteTaskDeckBranchSlug(slug){
    fmt.Fprintln(out,"Tên branch phải là slug ASCII chữ thường/số/gạch nối, 2–32 ký tự, bắt đầu bằng chữ và không kết thúc bằng gạch nối.")
    continue
   }
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Tạo branch cần hai lượt E2EE (review, CAS create); đăng nhập lại.")
    continue
   }
   reviewed,readErr:=remote.GitBranchReview(ctx)
   used++
   if readErr!=nil{err=readErr;break}
   lines:=strings.Split(reviewed,"\n")
   if len(lines)!=3||lines[2]!=""||
    !strings.HasPrefix(lines[0],"HEAD ")||
    !strings.HasPrefix(lines[1],"BRANCH refs/heads/"){
    err=errors.New("Git branch review không nguyên vẹn")
    break
   }
   head:=strings.TrimPrefix(lines[0],"HEAD ")
   if (len(head)!=40&&len(head)!=64)||len(reviewed)>512{
    err=errors.New("HEAD không hợp lệ")
    break
   }
   for _,c:=range head{
    if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){
     err=errors.New("HEAD Git có ký tự không hợp lệ")
     break
    }
   }
   if err!=nil{break}
   fmt.Fprintln(out,"Đang tạo branch mới từ HEAD đã xác thực, KHÔNG checkout:")
   consoleDisplayText(out,reviewed)
   fmt.Fprintf(out,"Branch đích: refs/heads/taskdeck/%s\n",slug)
   confirm:="CREATE BRANCH taskdeck/"+slug+" "+head[:12]
   fmt.Fprintf(out,"Nhập chính xác %q để tạo branch (không thể ghi đè branch có sẵn): ",confirm)
   accepted,readErr:=consoleLine(in,120)
   if readErr!=nil{return readErr}
   if accepted!=confirm{
    fmt.Fprintln(out,"Đã hủy; không gửi lệnh tạo branch tới host.")
    continue
   }
   sha:=sha256.Sum256([]byte(reviewed))
   result,err=remote.GitBranchCreate(ctx,slug,hex.EncodeToString(sha[:]))
   used++
  case line=="git commit":
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Git commit cần hai lượt E2EE; đăng nhập lại để tránh review không đầy đủ.")
    continue
   }
   reviewed,readErr:=remote.GitCommitReview(ctx)
   used++
   if readErr!=nil{err=readErr;break}
   if !strings.HasPrefix(reviewed,"HEAD ")||!strings.Contains(reviewed,"\nBRANCH refs/heads/")||
    !strings.Contains(reviewed,"\nINDEX_SHA256 ")||
    !strings.Contains(reviewed,"\n-- COMPLETE STAGED DIFF --\n"){
    err=errors.New("review Git không hợp lệ; từ chối commit")
    break
   }
   header:=strings.SplitN(reviewed,"\n",2)[0]
   parent:=strings.TrimPrefix(header,"HEAD ")
   if (len(parent)!=40&&len(parent)!=64)||len(reviewed)>remotetransport.RemoteGitDiffMaxBytes{
    err=errors.New("HEAD Git không hợp lệ; từ chối commit")
    break
   }
   for _,c:=range parent{
    if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){
     err=errors.New("HEAD chứa ký tự không hợp lệ")
     break
    }
   }
   if err!=nil{break}
   fmt.Fprintln(out,"Git commit — toàn bộ staged snapshot phải được xem và đồng ý:")
   consoleDisplayText(out,reviewed)
   fmt.Fprint(out,"Nhập commit message (3–96 ký tự ASCII an toàn): ")
   message,readErr:=consoleLine(in,128)
   if readErr!=nil{return readErr}
   if !remotetransport.ValidRemoteCommitMessage(message){
    fmt.Fprintln(out,"Commit message không hợp lệ; không gửi yêu cầu ghi.")
    continue
   }
   ack:="COMMIT "+parent[:12]
   fmt.Fprintf(out,"Xác nhận chính xác %q để cập nhật một branch Git: ",ack)
   confirmation,readErr:=consoleLine(in,80)
   if readErr!=nil{return readErr}
   if confirmation!=ack{
    fmt.Fprintln(out,"Đã hủy; Git branch và index không bị thay đổi.")
    continue
   }
   digest:=sha256.Sum256([]byte(reviewed))
   result,err=remote.GitCommit(ctx,message,hex.EncodeToString(digest[:]))
   used++
  case line=="git commit-review":
   result,err=remote.GitCommitReview(ctx)
   used++
  case line=="git status":
   result,err=remote.GitStatusFull(ctx)
   used++
  case line=="git log":
   result,err=remote.GitLog(ctx)
   used++
  case line=="git branches":
   result,err=remote.GitBranches(ctx)
   used++
  case strings.HasPrefix(line,"git unstage "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"git unstage ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if strings.HasPrefix(target,"-")||strings.ContainsAny(target,"*?[]")||
    target==".git"||strings.HasPrefix(target,".git/")||strings.Contains(target,"/.git/"){
    fmt.Fprintln(out,"Từ chối đường dẫn Git dạng tham số hoặc pattern.")
    continue
   }
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Git unstage cần hai lượt E2EE (staged diff và CAS Git index); đăng nhập lại.")
    continue
   }
   staged,readErr:=remote.GitStagedDiff(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   if staged==""{
    fmt.Fprintln(out,"Không có thay đổi đã stage để hủy; chưa gửi Git index write.")
    continue
   }
   fmt.Fprintf(out,"Git UNSTAGE toàn bộ thay đổi đã stage của %s (working tree không đổi):\n",target)
   consoleDisplayText(out,staged)
   fmt.Fprintf(out,"Gõ chính xác %q để đưa CHỈ file này về HEAD: ","UNSTAGE "+target)
   confirm,readErr:=consoleLine(in,192)
   if readErr!=nil{return readErr}
   if confirm!="UNSTAGE "+target{
    fmt.Fprintln(out,"Đã hủy; chưa gửi Git index write.")
    continue
   }
   digest:=sha256.Sum256([]byte(staged))
   result,err=remote.GitUnstage(ctx,target,hex.EncodeToString(digest[:]))
   used++
  case strings.HasPrefix(line,"git stage "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"git stage ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if strings.HasPrefix(target,"-"){
    fmt.Fprintln(out,"Từ chối đường dẫn Git dạng tham số.")
    continue
   }
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Git stage cần hai lượt E2EE (preview và CAS ghi index); đăng nhập lại.")
    continue
   }
   previous,readErr:=remote.ReadFilePreview(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   fmt.Fprintf(out,"Git STAGE toàn bộ file %s, không chạy clean filter hay hook:\n",target)
   consoleDisplayNumberedText(out,previous)
   fmt.Fprintf(out,"STAGE ghi đè nội dung từng phần đã stage của chính file này. Gõ chính xác %q: ","STAGE "+target)
   confirm,readErr:=consoleLine(in,192)
   if readErr!=nil{return readErr}
   if confirm!="STAGE "+target{
    fmt.Fprintln(out,"Đã hủy; chưa gửi Git index write.")
    continue
   }
   hash:=sha256.Sum256([]byte(previous))
   result,err=remote.GitStage(ctx,target,hex.EncodeToString(hash[:]))
   used++
  case strings.HasPrefix(line,"git review "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"git review ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Git review cần hai lượt E2EE (diff và diff --staged); đăng nhập lại.")
    continue
   }
   // Reuse exact host-gated read-only actions, not a new Git command or
   // arbitrary argv. The host independently checks GitStatus AND FilesRead
   // for EACH action and audits them separately. Do not print a partial
   // review if either authorization/revocation step fails.
   unstaged,readErr:=remote.GitDiff(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   staged,readErr:=remote.GitStagedDiff(ctx,target)
   used++
   if readErr!=nil{err=readErr;break}
   fmt.Fprintf(out,"Git review (read-only) %s:\n",target)
   fmt.Fprintln(out,"-- Chưa stage (working tree vs index) --")
   if unstaged==""{fmt.Fprintln(out,"(không có diff chưa stage)")}else{consoleDisplayText(out,unstaged)}
   fmt.Fprintln(out,"-- Đã stage (index vs HEAD) --")
   if staged==""{fmt.Fprintln(out,"(không có diff đã stage)")}else{consoleDisplayText(out,staged)}
   fmt.Fprintln(out,"Git review chỉ đọc; chưa stage, commit hay push.")
   continue
  case strings.HasPrefix(line,"git diff --head "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"git diff --head ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   result,err=remote.GitHeadDiff(ctx,target)
   used++
  case strings.HasPrefix(line,"git diff --staged "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"git diff --staged ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   result,err=remote.GitStagedDiff(ctx,target)
   used++
  case strings.HasPrefix(line,"git diff "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"git diff ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   result,err=remote.GitDiff(ctx,target)
   used++
  case strings.HasPrefix(line,"terminal task-output "):
   params:=strings.Fields(strings.TrimPrefix(line,"terminal task-output "))
   if len(params)!=2||!remotetransport.ValidManagedTaskHandle(params[0]){
    fmt.Fprintln(out,"Cú pháp: terminal task-output <handle-32-hex> <offset-byte>.")
    continue
   }
   offset,parseErr:=strconv.ParseInt(params[1],10,64)
   if parseErr!=nil||!remotetransport.ValidManagedOutputOffset(offset)||
    strconv.FormatInt(offset,10)!=params[1]{
    fmt.Fprintln(out,"Offset phải là số byte thập phân chuẩn, không âm.")
    continue
   }
   result,err=remote.ReadManagedOutput(ctx,params[0],offset)
   used++
  case strings.HasPrefix(line,"terminal task-status "):
   handle:=strings.TrimSpace(strings.TrimPrefix(line,"terminal task-status "))
   if !remotetransport.ValidManagedTaskHandle(handle){
    fmt.Fprintln(out,"Mã task không hợp lệ; phải gồm 32 ký tự hex viết thường.")
    continue
   }
   result,err=remote.StatusManagedTask(ctx,handle)
   used++
  case strings.HasPrefix(line,"terminal task-cancel "):
   handle:=strings.TrimSpace(strings.TrimPrefix(line,"terminal task-cancel "))
   if !remotetransport.ValidManagedTaskHandle(handle){
    fmt.Fprintln(out,"Mã task không hợp lệ; phải gồm 32 ký tự hex viết thường.")
    continue
   }
   expected:="CANCEL TASK "+handle[:12]
   fmt.Fprintf(out,"Yêu cầu dừng task %s, không dùng PID/shell. Nhập %q: ",handle,expected)
   confirm,readErr:=consoleLine(in,64)
   if readErr!=nil{return readErr}
   if confirm!=expected{
    fmt.Fprintln(out,"Đã hủy; không gửi yêu cầu dừng.")
    continue
   }
   result,err=remote.CancelManagedTask(ctx,handle)
   used++
  case line=="terminal tasks":
   result,err=remote.ListApprovedTasks(ctx)
   used++
  case strings.HasPrefix(line,"terminal task-start "),strings.HasPrefix(line,"terminal task-report "),strings.HasPrefix(line,"terminal task "):
   wantStart:=strings.HasPrefix(line,"terminal task-start ")
   wantReport:=strings.HasPrefix(line,"terminal task-report ")
   args:=strings.Fields(line)
   if len(args)!=3{
    fmt.Fprintln(out,"Task ID không hợp lệ.")
    continue
   }
   n,parseErr:=strconv.Atoi(args[2])
   if parseErr!=nil||n<1||n>0x7fffffff{
    fmt.Fprintln(out,"Task ID không hợp lệ.")
    continue
   }
   if strconv.Itoa(n)!=args[2]{
    fmt.Fprintln(out,"Task ID phải là số thập phân chuẩn.")
    continue
   }
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Terminal task cần hai lượt E2EE (danh mục và chạy); đăng nhập lại.")
    continue
   }
   catalogue,listErr:=remote.ListApprovedTasks(ctx)
   used++
   if listErr!=nil{err=listErr;break}
   taskLabel,labelErr:=consoleApprovedTaskLabel(catalogue,n)
   if labelErr!=nil{
    fmt.Fprintln(out,labelErr)
    continue
   }
   fmt.Fprintf(out,"TaskDeck host đã phê duyệt task #%d:\n",n)
   consoleDisplayText(out,taskLabel)
   confirmText:="RUN TASK "+strconv.Itoa(n)
   if wantStart{confirmText="START TASK "+strconv.Itoa(n)}
   fmt.Fprintf(out,"Xác nhận chạy task này, không phải shell/PTy, bằng %q: ",confirmText)
   confirm,readErr:=consoleLine(in,80)
   if readErr!=nil{return readErr}
   if confirm!=confirmText{
    fmt.Fprintln(out,"Đã hủy; không gửi yêu cầu chạy task tới host.")
    continue
   }
   // Server independently re-resolves the task and re-verifies its
   // exact host approval token, live TaskDeck grant and execution policy.
   if wantStart {
    result,err=remote.StartManagedTask(ctx,n)
   } else if wantReport {
    result,err=remote.RunTaskReport(ctx,n)
   } else {
    result,err=remote.RunTaskText(ctx,n)
   }
   used++
  default:
   fmt.Fprintln(out,"Lệnh không hỗ trợ; không chuyển shell hoặc Git argument tới host. Gõ help.")
   continue
  }
  if err!=nil{
   // A rejected/revoked action may close the AEAD channel. Never retry it.
   return fmt.Errorf("TaskDeck từ chối thao tác hoặc phiên hết hiệu lực: %w",err)
  }
  // Keep terminal escape/control bytes escaped, including in diff/task output.
  consoleDisplayText(out,result)
 }
 return ctx.Err()
}
