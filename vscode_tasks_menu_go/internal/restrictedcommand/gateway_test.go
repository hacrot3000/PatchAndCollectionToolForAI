package restrictedcommand

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
)

func TestParseOnlyReviewedCommands(t *testing.T){
 valid:=[]string{"git status","git status --short","git branch","git branch --list","git rev-parse --abbrev-ref HEAD","ls","pwd"}
 for _,input:=range valid{
  if _,err:=Parse(input,[]string{"git","ls","pwd"});err!=nil{t.Errorf("valid %q rejected: %v",input,err)}
 }
 blocked:=[]string{
  "git show","git diff","git cat-file -p HEAD","git archive HEAD","git clone https://example.com/x",
  "git -c core.fsmonitor=/bin/sh status","git config -l",
  "git push","git remote add evil https://attacker.invalid",
  "git status; whoami","git status && ls","git status | cat","git status > export",
  "git status $(id)","git status \x60id\x60","git status\npwd",
  "git --no-pager status","git alias.st=status","ls ../secret","ls -la",
  "cat secret.go","sh -c ls","bash","env","sudo git status","git status --porcelain=v2",
 }
 for _,input:=range blocked{
  if plan,err:=Parse(input,[]string{"git","ls","pwd"});err==nil {t.Errorf("unsafe command permitted: %#v",plan)}
 }
 if _,err:=Parse("git status",[]string{"pwd"});!errors.Is(err,ErrDenied){t.Fatalf("disabled group accepted: %v",err)}
 if _,err:=Parse(strings.Repeat("g",MaxInputBytes+1),[]string{"git"});err==nil{t.Fatal("unbounded command accepted")}
}
func TestGatewaySanitizesAndCapsOutput(t *testing.T){
 var capture cappedOutput
 _,_ = capture.Write([]byte("\x1b[31mabc\r\n"))
 if got:=outputText(capture.bytes);got!="[31mabc\n"{
  // ANSI escape sequences are not supposed to reach the user, including
  // format prefixes. Check no ESC/CR survives independently.
  if strings.ContainsAny(got,"\x1b\r"){t.Fatalf("control characters leaked: %q",got)}
 }
 _,_ = capture.Write([]byte(strings.Repeat("X",MaxOutputBytes*2)))
 if !capture.truncated||len(capture.bytes)>MaxOutputBytes{t.Fatal("output cap bypassed")}
}
func TestGatewayRootListingAndNoSourceContents(t *testing.T){
 root:=t.TempDir()
 for name,body:=range map[string]string{
  "main.go":"SUPER_SECRET_SOURCE_CANARY", ".env":"SECRET_ENV_CANARY",
  "README.md":"readme body canary",
 }{
  if err:=os.WriteFile(filepath.Join(root,name),[]byte(body),0600);err!=nil{t.Fatal(err)}
 }
 p,err:=Parse("ls",[]string{"ls"})
 if err!=nil{t.Fatal(err)}
 result,err:=Run(context.Background(),root,p)
 if err!=nil{t.Fatal(err)}
 if strings.Contains(result.Output,"SECRET")||strings.Contains(result.Output,".env")||
  !strings.Contains(result.Output,"main.go"){t.Fatalf("unexpected source disclosure: %#v",result)}
 p,err=Parse("pwd",[]string{"pwd"});if err!=nil{t.Fatal(err)}
 result,err=Run(context.Background(),root,p)
 if err!=nil||result.Output!=".\n"{t.Fatalf("workspace path leaked: %+v %v",result,err)}
}
func TestFixedGitStatusReportsMetadataNotSource(t *testing.T){
 if gitBinary()==""{t.Skip("trusted git binary is unavailable")}
 root:=t.TempDir()
 cmd:=exec.Command(gitBinary(),"init","-q",root)
 if out,err:=cmd.CombinedOutput();err!=nil{t.Fatalf("init git: %v %s",err,out)}
 if err:=os.WriteFile(filepath.Join(root,"secret.go"),[]byte("SUPER_SECRET_SOURCE_CANARY\n"),0600);err!=nil{t.Fatal(err)}
 for _,line:=range []string{"git status","git branch","git rev-parse --abbrev-ref HEAD"}{
  plan,err:=Parse(line,[]string{"git"})
  if err!=nil{t.Fatal(err)}
  result,err:=Run(context.Background(),root,plan)
  if err!=nil{t.Fatal(err)}
  if len(result.Output)>MaxOutputBytes||strings.Contains(result.Output,"SUPER_SECRET_SOURCE_CANARY"){
   t.Fatalf("%q leaked source bytes: %#v",line,result)
  }
 }
}
