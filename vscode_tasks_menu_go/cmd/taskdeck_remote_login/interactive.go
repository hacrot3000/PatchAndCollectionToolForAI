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
)

// The trusted native console never forwards command text to a shell.
// Remote operations stay individually typed, audited, RBAC-gated and E2EE.
type remoteConsoleAction interface {
 ReadFile(context.Context,string)(string,error)
 ReadFilePreview(context.Context,string)(string,error)
 ListDirectory(context.Context,string)(string,error)
 GitStatus(context.Context)(string,error)
 GitStatusFull(context.Context)(string,error)
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

func consoleHelp()string{
 return "Lệnh E2EE: help | pwd | cd [thư-mục] | ls [đường-dẫn] | view <file> | lines <file> | find <file> | edit <file> | edit-line <file> | edit-range <file> | move-lines <file> | insert-line <file> | delete-line <file> | replace <file> | replace-all <file> | git status | git log | git branches | git diff <file> | git diff --staged <file> | git diff --head <file> | git review <file> | git stage <file> | git unstage <file> | terminal tasks | terminal task <ID> | exit\n"+
  "cd/pwd chỉ thay đường dẫn tương đối trên client; không gọi shell, không tốn lượt E2EE.\n"+
  "Tên file/thư mục có dấu cách có thể nhập trực tiếp, không cần đặt trong nháy.\n"+
  "find tìm literal trong file đã xem đầy đủ; replace-all thay tất cả và cần gõ YES <số-vị-trí>.\n"+
  "lines in số dòng; edit-line sửa một dòng, giữ nguyên CRLF và cần xác nhận YES <số-dòng>.\n"+
  "edit-range thay một vùng dòng nhiều dòng; giữ nguyên phần ngoài vùng, hiển thị toàn bộ nội dung và xác nhận RANGE A-B.\n"+
  "move-lines di chuyển vùng dòng trong một file kết thúc newline; giữ nguyên từng byte và xác nhận MOVE A-B BEFORE N.\n"+
  "insert-line chèn trước dòng N (hoặc N=cuối+1); delete-line xóa dòng N. Xem lại toàn file và gõ INSERT/DELETE N.\n"+
  "replace tìm đúng một chuỗi trên một dòng, chỉ ghi sau khi kiểm tra SHA-256 và gõ YES.\n"+
  "Giới hạn mỗi phiên: 6 thao tác; view/edit đầy đủ tối đa 4KiB, git diff tối đa 4KiB.\n"+
  "git review <file> dùng hai lượt E2EE để xem diff chưa stage và đã stage; không thay đổi Git index hoặc worktree.\n"+
  "git diff --head <file> xem diff giữa HEAD và working tree trong một lượt E2EE (không ghi index).\n"+
  "git stage <file> chỉ stage toàn bộ file <=4KiB sau khi xem trước, xác nhận STAGE <file> và đối chiếu SHA-256 trên host.\n"+
  "git unstage <file> hủy toàn bộ phần đã stage của file, giữ nguyên working tree; yêu cầu diff staged và UNSTAGE <file>.\n"+
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
  case line=="terminal tasks":
   result,err=remote.ListApprovedTasks(ctx)
   used++
  case strings.HasPrefix(line,"terminal task "):
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
   result,err=remote.RunTaskText(ctx,n)
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
