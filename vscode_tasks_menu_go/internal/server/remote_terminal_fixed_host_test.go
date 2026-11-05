package server

import (
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestRemoteTerminalFixedHostPlanRequiresExplicitOptInAndOwner(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("fixed Unix PTY shell only")}
 root:=t.TempDir()
 t.Setenv(remoteTerminalHostOptIn,"")
 if spec,err:=remoteTerminalPlan(root,"user-1","project-1");
  err==nil||spec.Command!=""{t.Fatal("host-disabled remote terminal was admitted")}
 t.Setenv(remoteTerminalHostOptIn,"1")
 for _,tc:=range []struct{user,project string}{
  {"","project-1"},{"user-1",""},{"",""},
 }{
  if spec,err:=remoteTerminalPlan(root,tc.user,tc.project);
   err==nil||spec.Command!=""{t.Fatal("unowned PTY execution was admitted")}
 }
}

func TestRemoteTerminalFixedHostPlanNeverInheritsAgentShellOrCredentials(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("fixed Unix PTY shell only")}
 root:=t.TempDir()
 t.Setenv(remoteTerminalHostOptIn,"1")
 t.Setenv("SHELL","/tmp/evil-command")
 t.Setenv("BASH_ENV","/tmp/evil-bash-init")
 t.Setenv("ENV","/tmp/evil-shell-init")
 t.Setenv("LD_PRELOAD","/tmp/evil-preload.so")
 t.Setenv("GIT_ASKPASS","/tmp/evil-askpass")
 t.Setenv("SSH_AUTH_SOCK","/tmp/evil-ssh-agent")
 t.Setenv("HOME","/tmp/evil-home")
 t.Setenv("PATH","/tmp/evil-bin")
 spec,err:=remoteTerminalPlan(root,"owner-user","owner-project")
 if err!=nil{t.Fatal(err)}
 if spec.TaskID!=0||spec.SessionKind!=tasks.SessionKindTerminal||
  spec.TargetType!="local"||spec.TargetProfileID!=""||
  spec.OwnerUserID!="owner-user"||spec.ProjectID!="owner-project"||
  spec.Cwd!=root||len(spec.Args)!=1||spec.Args[0]!="-i"{
  t.Fatalf("wrong owner-bound execution plan: %+v",spec)
 }
 if spec.Command=="/tmp/evil-command"||!filepath.IsAbs(spec.Command){
  t.Fatal("Agent SHELL variable escaped host pin")
 }
 cmdInfo,err:=os.Stat(spec.Command)
 if err!=nil||!cmdInfo.Mode().IsRegular()||cmdInfo.Mode().Perm()&0o111==0{
  t.Fatal("host-pinned executable not regular runnable file")
 }
 env:=strings.Join(spec.Env,"\n")
 for _,leak:=range []string{"LD_PRELOAD","BASH_ENV","SSH_AUTH_SOCK","GIT_ASKPASS",
  "/tmp/evil-home","/tmp/evil-bin","/tmp/evil-preload"}{
  if strings.Contains(env,leak){t.Fatal("remote terminal inherited unsafe environment: "+leak)}
 }
 if len(spec.Env)!=4||!strings.Contains(env,"HOME="+root)||
  !strings.Contains(env,"TERM=xterm")||spec.ProtocolCommands||spec.ProtocolEvents{
  t.Fatalf("terminal execution inherited host integration/protocol: %+v",spec)
 }
}

func TestRemoteTerminalFixedHostPlanRefusesLinkedWorkspaceAndRelativeDirectory(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("fixed Unix PTY shell only")}
 t.Setenv(remoteTerminalHostOptIn,"1")
 root:=t.TempDir()
 alias:=filepath.Join(t.TempDir(),"project-link")
 if err:=os.Symlink(root,alias);err!=nil{t.Fatal(err)}
 for _,bad:=range []string{".","",alias,filepath.Join(root,"missing")}{
  if spec,err:=remoteTerminalPlan(bad,"owner","project");err==nil||spec.Command!=""{
   t.Fatalf("noncanonical/symlinked project workspace admitted: %q",bad)
  }
 }
}
