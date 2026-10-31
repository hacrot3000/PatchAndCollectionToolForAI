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

 "bletonfc/vscode_tasks_menu/internal/remoterelay"
)

// RemoteConsoleAction is intentionally a *fixed* native-client interface.
// No input line is passed to a shell or remote process. "terminal task N"
// executes only a host-approved, RBAC-gated TaskDeck task ID through the
// existing local-only task.run adapter. A true PTY has NOT been exposed.
type remoteConsoleAction interface {
 ReadFile(context.Context,string)(string,error)
 ListDirectory(context.Context,string)(string,error)
 GitStatus(context.Context)(string,error)
 GitDiff(context.Context,string)(string,error)
 ReplaceFile(context.Context,string,string,string)(string,error)
 RunTask(context.Context,int)(string,error)
}

func validConsolePath(path string)bool {
 return path!=""&&len(path)<=120&&path!="."&&
  !strings.HasPrefix(path,"/")&&!strings.Contains(path,"..")&&
  !strings.ContainsAny(path,"\\\x00\r\n:")
}

func consoleHelp()string{
 return "Lệnh E2EE: help | ls [đường-dẫn] | view <file> | edit <file> | git status | git diff <file> | terminal task <ID> | exit\n"+
  "Giới hạn mỗi phiên: 6 thao tác; view tối đa 192B, edit tối đa 96B, git diff tối đa 4KiB.\n"+
  "Terminal PTY/shell tùy ý CHƯA được bật. task phải được host phê duyệt và có TasksRun.\n"
}

// The console never accepts a command as a remote shell string. It parses
// exact verbs and arguments, and invokes independently typed E2EE operations.
// Each operation is still audited and reauthorized by TaskDeck on the host.
func runNativeInteractiveConsole(ctx context.Context,remote remoteConsoleAction,
 in *bufio.Reader,out io.Writer)error{
 if ctx==nil||remote==nil||in==nil||out==nil{return errors.New("missing authenticated native console")}
 fmt.Fprint(out,consoleHelp())
 used:=0
 for ctx.Err()==nil {
  if used>=remoterelay.MaxAppPingRounds{
   fmt.Fprintln(out,"Phiên đã dùng hết 6 lượt E2EE; đăng nhập lại bằng ticket mới.")
   return nil
  }
  fmt.Fprintf(out,"TaskDeck E2EE [%d/%d]> ",used,remoterelay.MaxAppPingRounds)
  line,err:=in.ReadString('\n')
  if err!=nil{return err}
  line=strings.TrimSpace(line)
  if len(line)>256{return errors.New("console input exceeds 256 bytes")}
  args:=strings.Fields(line)
  if len(args)==0{continue}
  if len(args)==1&&args[0]=="exit"{return nil}
  if len(args)==1&&args[0]=="help"{fmt.Fprint(out,consoleHelp());continue}
  var result string
  switch {
  case len(args)==1&&args[0]=="ls":
   result,err=remote.ListDirectory(ctx,".")
   used++
  case len(args)==2&&args[0]=="ls"&&validConsolePath(args[1]):
   result,err=remote.ListDirectory(ctx,args[1])
   used++
  case len(args)==2&&args[0]=="view"&&validConsolePath(args[1]):
   result,err=remote.ReadFile(ctx,args[1])
   used++
  case len(args)==2&&args[0]=="edit"&&validConsolePath(args[1]):
   if used+2>remoterelay.MaxAppPingRounds{
    fmt.Fprintln(out,"Edit cần hai lượt để đọc SHA và ghi CAS; hãy đăng nhập phiên mới.")
    continue
   }
   var previous string
   previous,err=remote.ReadFile(ctx,args[1])
   used++
   if err!=nil{break}
   fmt.Fprintf(out,"Nội dung hiện tại: %q\n",previous)
   fmt.Fprint(out,"Nội dung mới (một dòng, tối đa 96 byte): ")
   next,e:=in.ReadString('\n')
   if e!=nil{return e}
   next=strings.TrimSuffix(strings.TrimSuffix(next,"\n"),"\r")
   if len(next)>96||strings.ContainsRune(next,'\x00'){
    fmt.Fprintln(out,"Nội dung không hợp lệ; chưa gửi lệnh ghi.")
    continue
   }
   fmt.Fprint(out,"Xác nhận ghi file đã kiểm tra SHA? Gõ YES: ")
   confirm,e:=in.ReadString('\n')
   if e!=nil{return e}
   if strings.TrimSpace(confirm)!="YES"{
    fmt.Fprintln(out,"Đã hủy; chưa gửi thao tác ghi.")
    continue
   }
   hash:=sha256.Sum256([]byte(previous))
   result,err=remote.ReplaceFile(ctx,args[1],next,hex.EncodeToString(hash[:]))
   used++
  case len(args)==2&&args[0]=="git"&&args[1]=="status":
   result,err=remote.GitStatus(ctx)
   used++
  case len(args)==3&&args[0]=="git"&&args[1]=="diff"&&validConsolePath(args[2]):
   result,err=remote.GitDiff(ctx,args[2])
   used++
  case len(args)==3&&args[0]=="terminal"&&args[1]=="task":
   n,e:=strconv.Atoi(args[2])
   if e!=nil||n<1||n>0x7fffffff{
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
   // A failed AEAD record or revoked TaskDeck principal closes the native
   // client. Do NOT continue sending commands into a poisoned E2EE session.
   return fmt.Errorf("TaskDeck từ chối thao tác hoặc phiên hết hiệu lực: %w",err)
  }
  // Print quoted UTF-8, never execute remote ANSI control or terminal
  // escape bytes returned from potentially malicious workspace files.
  fmt.Fprintf(out,"%q\n",result)
 }
 return ctx.Err()
}
