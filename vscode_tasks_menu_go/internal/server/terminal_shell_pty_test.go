package server

import (
 "bytes"
 "os/exec"
 "runtime"
 "strings"
 "sync"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/tasks"
 "github.com/creack/pty"
)

func TestBashTerminalPTYEmitsPreexecOnlyForCommands(t *testing.T) {
 if runtime.GOOS!="linux" { t.Skip("Linux PTY shell integration test") }
 if _,err:=exec.LookPath("bash");err!=nil {t.Skip("bash unavailable")}
 t.Setenv("HOME",t.TempDir())
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 spec:=tasks.Execution{Command:"bash",Preview:"bash"}
 if err:=configureTerminalShellIntegration(&spec);err!=nil {t.Fatal(err)}
 cmd:=exec.Command("bash","--noprofile",spec.Args[0],spec.Args[1],spec.Args[2])
 term,err:=pty.Start(cmd)
 if err!=nil {t.Skipf("PTY unavailable: %v",err)}
 defer func(){_ = cmd.Process.Kill();_ = term.Close();_ = cmd.Wait()}()
 var mu sync.Mutex
 var output bytes.Buffer
 go func(){
  data:=make([]byte,4096)
  for {
   n,readErr:=term.Read(data)
   if n>0 {mu.Lock();_,_=output.Write(data[:n]);mu.Unlock()}
   if readErr!=nil{return}
  }
 }()
 snapshot:=func()string{mu.Lock();defer mu.Unlock();return output.String()}
 wait:=func(marker string){
  t.Helper()
  deadline:=time.Now().Add(4*time.Second)
  for time.Now().Before(deadline) {
   if strings.Contains(snapshot(),marker){return}
   time.Sleep(20*time.Millisecond)
  }
  t.Fatalf("missing PTY marker %q in %q",marker,snapshot())
 }
 wait("\x1b]133;B\x07")
 if strings.Contains(snapshot(),"\x1b]133;C\x07") {t.Fatalf("idle shell emitted execution marker: %q",snapshot())}
 if _,err:=term.Write([]byte("\n"));err!=nil {t.Fatal(err)}
 time.Sleep(120*time.Millisecond)
 if strings.Contains(snapshot(),"\x1b]133;C\x07") {t.Fatalf("empty Enter emitted execution marker: %q",snapshot())}
 if _,err:=term.Write([]byte("false\n"));err!=nil {t.Fatal(err)}
 wait("\x1b]133;C\x07")
 wait("\x1b]133;D;1\x07")
}
