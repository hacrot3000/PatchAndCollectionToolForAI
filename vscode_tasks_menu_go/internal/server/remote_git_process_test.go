package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteGitEnvironmentNeverInheritsDaemonSecrets(t *testing.T){
 t.Setenv("HOME","/home/daemon/.config/taskdeck/private-signing-keys")
 t.Setenv("PATH","/workspace/attacker-bin:/usr/bin")
 t.Setenv("GIT_CONFIG_COUNT","1")
 t.Setenv("GIT_CONFIG_KEY_0","core.fsmonitor")
 t.Setenv("GIT_CONFIG_VALUE_0","/workspace/attack")
 t.Setenv("GIT_EXEC_PATH","/workspace/malicious")
 t.Setenv("GIT_TRACE","/workspace/private-trace")
 t.Setenv("LD_PRELOAD","/workspace/malicious.so")
 t.Setenv("DYLD_INSERT_LIBRARIES","/workspace/malicious.dylib")
 want:=[]string{
  "PATH=/usr/bin:/bin","HOME=/nonexistent","LANG=C","LC_ALL=C",
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_OPTIONAL_LOCKS=0","GIT_TERMINAL_PROMPT=0",
 }
 got:=isolatedRemoteGitEnvironment()
 if !reflect.DeepEqual(got,want){t.Fatalf("remote Git inherited daemon environment: %q",got)}
}

func TestRemoteEncryptedGitStatusCannotExecuteWorkspacePATHGit(t *testing.T){
 s,principal,client,agent:=remoteDispatchFixture(t)
 trusted,err:=remoteTrustedGitExecutable(s.Workspace)
 if err!=nil {t.Skipf("system Git unavailable: %v",err)}
 init:=exec.Command(trusted,"init","-q",s.Workspace)
 init.Env=isolatedRemoteGitEnvironment()
 if output,err:=init.CombinedOutput();err!=nil{
  t.Fatalf("cannot create Git fixture: %v %s",err,output)
 }
 marker:=filepath.Join(s.Workspace,"malicious-git-was-run")
 attackerBin:=filepath.Join(s.Workspace,"attacker-bin")
 if err:=os.Mkdir(attackerBin,0700);err!=nil{t.Fatal(err)}
 fake:=filepath.Join(attackerBin,"git")
 // Calling this counterfeit Git would prove that a valid signed E2EE
 // request executed a workspace-controlled program using daemon PATH.
 if err:=os.WriteFile(fake,[]byte("#!/bin/sh\ntouch '"+marker+"'\necho PWNED\n"),0700);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",attackerBin+string(os.PathListSeparator)+os.Getenv("PATH"))
 t.Setenv("HOME",s.Workspace)
 t.Setenv("GIT_CONFIG_COUNT","1")
 t.Setenv("GIT_CONFIG_KEY_0","core.fsmonitor")
 t.Setenv("GIT_CONFIG_VALUE_0",fake)
 t.Setenv("GIT_EXEC_PATH",attackerBin)
 sealed,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,
  Operation:remotetransport.RemoteActionGitStatus,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),principal,agent,sealed)
 if err!=nil{t.Fatalf("safe E2EE GitStatus denied: %v",err)}
 result,err:=remotetransport.OpenRemoteGitStatusResult(client,reply,remoteDispatchTestID)
 if err!=nil||result==""||strings.Contains(result,"PWNED"){
  t.Fatalf("remote Git response is invalid or attacker controlled: %q %v",result,err)
 }
 if _,err:=os.Stat(marker);!os.IsNotExist(err){
  t.Fatalf("daemon PATH executable ran: %v",err)
 }
}
