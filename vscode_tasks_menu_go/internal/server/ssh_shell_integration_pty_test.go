package server

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/sshclient"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
	"github.com/creack/pty"
)

// Simulate the command that sshd passes to the remote login shell using a
// local PTY; no test SSH server or remote user configuration is required.
func TestSSHBashIntegrationDetectsIdleCommandAndExitStatus(t *testing.T) {
	if runtime.GOOS != "linux" { t.Skip("PTY test requires Linux") }
	if _,err:=exec.LookPath("bash");err!=nil { t.Skip("Bash is unavailable") }
	profile:=sshprofile.Profile{ID:"ssh-test",Name:"SSH test",Host:"example.com",Username:"tester",AuthMethod:sshprofile.AuthAgent}
	command,err:=sshclient.BuildInstrumentedInteractiveCommand("/usr/bin/ssh",profile,"",taskDeckBashIntegrationRC)
	if err!=nil { t.Fatal(err) }
	script:=command.Args[len(command.Args)-1]
	home:=t.TempDir()
	cmd:=exec.Command("sh","-c",script)
	cmd.Env=append(os.Environ(),"HOME="+home,"SHELL=/bin/bash","TERM=xterm")
	terminal,err:=pty.StartWithSize(cmd,&pty.Winsize{Rows:24,Cols:100})
	if err!=nil { t.Skipf("could not allocate PTY: %v",err) }
	defer func(){_ = cmd.Process.Kill();_ = terminal.Close();_ = cmd.Wait()}()
	var mu sync.Mutex
	var output bytes.Buffer
	go func(){
		buf:=make([]byte,4096)
		for {
			n,readErr:=terminal.Read(buf)
			if n>0 {mu.Lock();_,_=output.Write(buf[:n]);mu.Unlock()}
			if readErr!=nil {return}
		}
	}()
	snapshot:=func()string {mu.Lock();defer mu.Unlock();return output.String()}
	wait:=func(marker string){
		t.Helper()
		deadline:=time.Now().Add(5*time.Second)
		for time.Now().Before(deadline){
			if strings.Contains(snapshot(),marker) {return}
			time.Sleep(20*time.Millisecond)
		}
		t.Fatalf("missing SSH shell integration marker %q: %q",marker,snapshot())
	}
	wait("\x1b]133;A\x07")
	if strings.Contains(snapshot(),"\x1b]133;C\x07") {
		t.Fatalf("idle SSH shell emitted running marker: %q",snapshot())
	}
	if _,err:=terminal.Write([]byte("\n"));err!=nil {t.Fatal(err)}
	time.Sleep(140*time.Millisecond)
	if strings.Contains(snapshot(),"\x1b]133;C\x07") {
		t.Fatalf("empty SSH prompt incorrectly started a command: %q",snapshot())
	}
	if _,err:=terminal.Write([]byte("false\n"));err!=nil {t.Fatal(err)}
	wait("\x1b]133;C\x07")
	wait("\x1b]133;D;1\x07")
	if _,err:=terminal.Write([]byte("true\n"));err!=nil {t.Fatal(err)}
	wait("\x1b]133;D;0\x07")
	entries,err:=os.ReadDir(home)
	if err!=nil {t.Fatal(err)}
	if len(entries)!=0 {t.Fatalf("temporary integration unexpectedly wrote remote home files: %+v",entries)}
}
