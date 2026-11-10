//go:build linux || darwin

package server

import (
 "context"
 "encoding/pem"
 "net/http"
 "net/http/httptest"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strconv"
 "strings"
 "sync/atomic"
 "testing"
)

// A repository can define credential.helper as an executable. Even when
// GIT_TERMINAL_PROMPT=0 prevents terminal input, Git may run that helper
// after a 401 Basic challenge from an HTTPS host. The three typed Remote
// network mutations must reject the authentication challenge WITHOUT
// ever executing arbitrary repository-owned helpers or askpass scripts.
func TestRemoteGitNetworkMutationsNeverExecuteRepositoryCredentialHelpers(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only remote Git network transport")}
 t.Setenv("TASKDECK_REMOTE_GIT_PULL","1")
 t.Setenv("TASKDECK_REMOTE_GIT_FETCH","1")
 t.Setenv("TASKDECK_REMOTE_GIT_PUSH","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 gitBin,err:=exec.LookPath("git")
 if err!=nil{t.Fatal(err)}
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command(gitBin,append([]string{"-C",s.Workspace},args...)...)
  output,e:=cmd.CombinedOutput()
  if e!=nil{t.Fatalf("Git fixture %v: %v: %s",args,e,output)}
  return strings.TrimSpace(string(output))
 }
 run("config","user.name","Remote Test")
 run("config","user.email","security@example.invalid")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"tracked.txt"),
  []byte("baseline\n"),0600);err!=nil{t.Fatal(err)}
 run("add","-A")
 run("commit","-qm","baseline")
 branch:=run("symbolic-ref","--short","HEAD")

 var requests atomic.Int64
 remote:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  requests.Add(1)
  w.Header().Set("WWW-Authenticate",`Basic realm="TaskDeck test only"`)
  w.WriteHeader(http.StatusUnauthorized)
 }))
 defer remote.Close()
 run("remote","add","origin",remote.URL+"/repo.git")
 // Trust only this fixture certificate while keeping HTTPS peer
 // verification enabled. Do not disable SSL verification for testing.
 certificate:=pem.EncodeToMemory(&pem.Block{
  Type:"CERTIFICATE",Bytes:remote.Certificate().Raw,
 })
 caFile:=filepath.Join(t.TempDir(),"trusted-fixture-ca.pem")
 if err:=os.WriteFile(caFile,certificate,0600);err!=nil{t.Fatal(err)}
 run("config","http.sslCAInfo",caFile)
 run("config","branch."+branch+".remote","origin")
 run("config","branch."+branch+".merge","refs/heads/"+branch)
 run("update-ref","refs/remotes/origin/"+branch,"HEAD")
 script:=filepath.Join(t.TempDir(),"repo-credential-helper")
 marker:=filepath.Join(t.TempDir(),"helper-executed")
 // This helper MUST NOT be invoked by any authenticated network action.
 helper:="#!/bin/sh\nprintf used > "+strconv.Quote(marker)+
  "\nprintf 'username=tester\\npassword=fixture\\n'\n"
 if err:=os.WriteFile(script,[]byte(helper),0700);err!=nil{t.Fatal(err)}
 run("config","credential.helper",script)
 for _,action:=range []struct{
  name string
  invoke func()error
 }{
  {"pull",func()error{_,e:=s.executeRemoteGitPull(context.Background(),session);return e}},
  {"fetch",func()error{_,e:=s.executeRemoteGitFetch(context.Background(),session);return e}},
  {"push",func()error{_,e:=s.executeRemoteGitPush(context.Background(),session);return e}},
 }{
  t.Run(action.name,func(t *testing.T){
   before:=requests.Load()
   if err:=action.invoke();err==nil{
    t.Fatal("Git network action succeeded against unauthorized HTTPS server")
   }
   if requests.Load()<=before{
    t.Fatal("Git operation was denied before exercising HTTPS 401")
   }
   if _,err:=os.Stat(marker);!os.IsNotExist(err){
    t.Fatalf("repository-defined credential helper executed: %v",err)
   }
  })
 }

 // After the positive HTTPS 401 test, prove both a general repository
 // downgrade and a longer, URL-specific downgrade are refused BEFORE
 // contacting the HTTPS server. Git's URL-specific SSL policy can
 // otherwise override generic http.sslVerify=true.
 for _,name:=range []string{
  "http.sslVerify",
  "http."+remote.URL+"/repo.git/info.sslVerify",
 }{
  t.Run("reject-"+name,func(t *testing.T){
   run("config",name,"false")
   for _,action:=range []struct{
    name string
    invoke func()error
   }{
    {"pull",func()error{_,e:=s.executeRemoteGitPull(context.Background(),session);return e}},
    {"fetch",func()error{_,e:=s.executeRemoteGitFetch(context.Background(),session);return e}},
    {"push",func()error{_,e:=s.executeRemoteGitPush(context.Background(),session);return e}},
   }{
    before:=requests.Load()
    if err:=action.invoke();err==nil{
     t.Fatalf("%s admitted repository TLS verification override",action.name)
    }
    if requests.Load()!=before{
     t.Fatalf("%s contacted server with unsafe TLS config",action.name)
    }
   }
   run("config","--unset",name)
  })
 }

 // Prove that this configuration and script would execute when Git is
 // invoked without the hardened -c credential.helper= override.
 prove:=exec.Command(gitBin,"-C",s.Workspace,"credential","fill")
 prove.Stdin=strings.NewReader("protocol=https\nhost=example.invalid\n\n")
 prove.Env=append(os.Environ(),"GIT_TERMINAL_PROMPT=0")
 if output,err:=prove.CombinedOutput();err!=nil{
  t.Fatalf("credential helper fixture cannot execute: %v %s",err,output)
 }
 if _,err:=os.Stat(marker);err!=nil{
  t.Fatal("fixture never demonstrated repo-controlled helper execution")
 }
}
