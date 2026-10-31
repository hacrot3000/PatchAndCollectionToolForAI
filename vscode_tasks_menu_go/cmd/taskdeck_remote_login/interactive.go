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
 GitLog(context.Context)(string,error)
 GitBranches(context.Context)(string,error)
 GitDiff(context.Context,string)(string,error)
 ReplaceFile(context.Context,string,string,string)(string,error)
 ReplaceFileText(context.Context,string,string,string)(string,error)
 RunTask(context.Context,int)(string,error)
}

func validConsolePath(path string)bool {
 return path!=""&&len(path)<=120&&path!="."&&utf8.ValidString(path)&&
  !strings.HasPrefix(path,"/")&&!strings.Contains(path,"..")&&
  !strings.ContainsAny(path,"\\\x00\r\n:")
}

// consolePath resolves client-side "cd" navigation. Never forward "..",
// absolute paths, command syntax or an untrusted working directory to Agent.
// "cd" is a local display preference, NOT a server-side process cwd or shell.
func consolePath(cwd, input string, directory bool)(string,error) {
 input=strings.TrimSpace(input)
 if input==""||len(input)>256||!utf8.ValidString(input)||
  strings.HasPrefix(input,"/")||strings.ContainsAny(input,"\\\x00\r\n:") {
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
   if line==".save"&&len(lines)>0{content+="\n"}
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

func consoleHelp()string{
 return "Lệnh E2EE: help | pwd | cd [thư-mục] | ls [đường-dẫn] | view <file> | edit <file> | git status | git log | git branches | git diff <file> | terminal task <ID> | exit\n"+
  "cd/pwd chỉ thay đường dẫn tương đối trên client; không gọi shell, không tốn lượt E2EE.\n"+
  "Tên file/thư mục có dấu cách có thể nhập trực tiếp, không cần đặt trong nháy.\n"+
  "Giới hạn mỗi phiên: 6 thao tác; view/edit đầy đủ tối đa 4KiB, git diff tối đa 4KiB.\n"+
  "Terminal PTY/shell tùy ý CHƯA được bật. task phải được host phê duyệt và có TasksRun.\n"
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
   fmt.Fprintf(out,"Nội dung hiện tại: %q\n",previous)
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
  case line=="git status":
   result,err=remote.GitStatus(ctx)
   used++
  case line=="git log":
   result,err=remote.GitLog(ctx)
   used++
  case line=="git branches":
   result,err=remote.GitBranches(ctx)
   used++
  case strings.HasPrefix(line,"git diff "):
   target,e=consolePath(cwd,strings.TrimSpace(strings.TrimPrefix(line,"git diff ")),false)
   if e!=nil{fmt.Fprintln(out,e);continue}
   result,err=remote.GitDiff(ctx,target)
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
   result,err=remote.RunTask(ctx,n)
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
  fmt.Fprintf(out,"%q\n",result)
 }
 return ctx.Err()
}
