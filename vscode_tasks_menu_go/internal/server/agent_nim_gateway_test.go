package server

import (
 "encoding/json"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentNimGatewayBuildRegistration(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("local Nim gateway requires Unix")}
 home:=t.TempDir()
 t.Setenv("HOME",home)
 root,err:=filepath.EvalSymlinks(t.TempDir());if err!=nil{t.Fatal(err)}
 identity,err:=patchtool.EnsureProjectIdentity(root);if err!=nil{t.Fatal(err)}
 script:=[]byte("#!/bin/bash\nexit 0\n")
 if err=os.WriteFile(filepath.Join(root,"nim-agent.sh"),script,0700);err!=nil{t.Fatal(err)}
 sha:=agentchat.BodyDigest(script)
 s:=&Server{Workspace:root}
 args:=func(argv []string,pin bool)agentPatchArguments{
  var m map[string]any
  if err:=json.Unmarshal([]byte(testAgentCommandManifest(identity,argv)),&m);err!=nil{t.Fatal(err)}
  if pin{m["preflight"]=map[string]any{"files":[]any{map[string]any{"path":"nim-agent.sh","sha256":sha,"exists":true}}}}
  b,err:=json.Marshal(m);if err!=nil{t.Fatal(err)}
  return agentPatchArguments{Name:"nim",PatchManifest:string(b)}
 }
 valid:=args([]string{"bash","nim-agent.sh","nim-test","tests/test_i0.nim","server-probe","-d:mwfVerboseDiagnostics"},true)
 if err=s.validateAgentNimGatewayProposal(valid,identity.ID);err==nil{t.Fatal("unregistered gateway accepted")}
 cfg:=filepath.Join(home,".config","taskdeck","nim-gateway.json")
 if err=os.MkdirAll(filepath.Dir(cfg),0700);err!=nil{t.Fatal(err)}
 b,_:=json.Marshal(agentNimGatewayRegistration{ProjectRoot:root,ProjectID:identity.ID,Script:"nim-agent.sh",SHA256:sha})
 if err=os.WriteFile(cfg,b,0600);err!=nil{t.Fatal(err)}
 if err=s.validateAgentNimGatewayProposal(valid,identity.ID);err!=nil{t.Fatal(err)}
 if err=s.validateAgentNimGatewayProposal(args([]string{"bash","nim-agent.sh","nim-build","release"},true),identity.ID);err!=nil{t.Fatal(err)}
 if err=s.validateAgentNimGatewayProposal(args([]string{"bash","scripts/check.sh"},false),identity.ID);err!=nil{t.Fatal("generic script contract changed",err)}
 if err=s.validateAgentNimGatewayProposal(args([]string{"bash","nim-agent.sh","nim-test","test.nim"},false),identity.ID);err==nil{t.Fatal("missing SHA pin accepted")}
 for _,argv:=range [][]string{
  {"bash","nim-agent.sh","nim-build","release-copy"},
  {"bash","nim-agent.sh","compile_release"},
  {"bash","nim-agent.sh","nim-delete","test.nim","sha"},
  {"bash","nim-agent.sh","nim-write","test.nim"},
  {"bash","nim-agent.sh","nim-cache-clean","run"},
  {"bash","nim-agent.sh","nim-build-result",strings.Repeat("a",32)},
  {"bash","nim-agent.sh","nim-test","../outside.nim"},
  {"bash","nim-agent.sh","nim-test","test.nim","--out:/tmp/evil"},
  {"bash","nim-agent.sh","nim-test","test.nim","-d:bad=1"},
  {"sh","nim-agent.sh","nim-test","test.nim"},
  {"./nim-agent.sh","nim-test","test.nim"},
 }{
  if err=s.validateAgentNimGatewayProposal(args(argv,true),identity.ID);err==nil{t.Fatalf("unsafe gateway action accepted: %v",argv)}
 }
 if !strings.Contains(s.agentNimGatewayInstructions(identity.ID),"nim-build-result"){t.Fatal("registered gateway not advertised")}
 if err=s.validateAgentNimGatewayProposal(valid,"foreign");err==nil{t.Fatal("foreign identity accepted")}
 if err=os.WriteFile(filepath.Join(root,"nim-agent.sh"),[]byte("changed"),0700);err!=nil{t.Fatal(err)}
 if err=s.validateAgentNimGatewayProposal(valid,identity.ID);err==nil{t.Fatal("changed gateway accepted")}
 if s.agentNimGatewayInstructions(identity.ID)!=""{t.Fatal("changed gateway advertised")}
}
