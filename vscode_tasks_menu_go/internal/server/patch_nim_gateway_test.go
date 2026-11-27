package server

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestPatchNimGatewaySetup(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("Unix gateway")}
 t.Setenv("HOME",t.TempDir())
 root,err:=filepath.EvalSymlinks(t.TempDir())
 if err!=nil{t.Fatal(err)}
 s:=&Server{Workspace:root}
 call:=func(method,url string,payload map[string]any,want int)patchNimGatewayView{
  t.Helper()
  body,_:=json.Marshal(payload)
  req:=httptest.NewRequest(method,url,bytes.NewReader(body))
  req.Header.Set("Content-Type","application/json")
  rec:=httptest.NewRecorder()
  s.patchNimGateway(rec,req)
  if rec.Code!=want{t.Fatalf("%s %s: status %d want %d: %s",method,url,rec.Code,want,rec.Body.String())}
  var v patchNimGatewayView
  if want==200{if err:=json.Unmarshal(rec.Body.Bytes(),&v);err!=nil{t.Fatal(err)}}
  return v
 }
 endpoint:="/api/patch/nim-gateway"
 v:=call("GET",endpoint,nil,200)
 if v.Registered||len(v.Candidates)!=0||v.ProjectID!=""{t.Fatalf("unexpected initial state %+v",v)}
 cfg,_:=patchNimRegistrationPath()
 for _,p:=range []string{cfg,filepath.Join(root,".python_patch_tool.json")}{
  if _,err:=os.Stat(p);!os.IsNotExist(err){t.Fatalf("GET mutated %s",p)}
 }
 script:="#!/bin/bash\nexit 0\n"
 base:=map[string]any{"action":"install","project_root":root,"script":"nim-agent.sh","content":script,"confirm":true}
 v=call("POST",endpoint,base,200)
 if v.Registered||len(v.Candidates)!=1||v.Candidates[0].SHA256!=agentchat.BodyDigest([]byte(script)){t.Fatalf("install should discover but not authorize: %+v",v)}
 call("POST",endpoint,base,409)
 nested:="nim-gateway-v4/nim-agent.sh"
 if err:=os.MkdirAll(filepath.Join(root,"nim-gateway-v4"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,nested),[]byte(script),0700);err!=nil{t.Fatal(err)}
 v=call("GET",endpoint,nil,200)
 if len(v.Candidates)!=2{t.Fatalf("both locations should be discovered: %+v",v)}
 call("GET",endpoint+"?script=../nim-agent.sh",nil,400)
 registration:=map[string]any{"action":"register","project_root":root,"script":nested,"sha256":v.Candidates[0].SHA256,"confirm":false}
 call("POST",endpoint,registration,400)
 registration["confirm"]=true
 registration["project_root"]=root+"-other"
 call("POST",endpoint,registration,400)
 registration["project_root"]=root
 registration["sha256"]=strings.Repeat("0",64)
 call("POST",endpoint,registration,409)
 registration["sha256"]=agentchat.BodyDigest([]byte(script))
 v=call("POST",endpoint,registration,200)
 if !v.Registered||v.RegisteredScript!=nested||v.ProjectID==""||!strings.Contains(v.Prompt,nested)||!strings.Contains(v.Prompt,v.ProjectID)||!strings.Contains(v.Prompt,"nim-build-result"){t.Fatalf("registration or prompt invalid: %+v",v)}
 identity,err:=patchtool.ReadProjectIdentity(root)
 if err!=nil||identity.ID!=v.ProjectID{t.Fatal("project identity mismatch",err)}
 if st,err:=os.Stat(cfg);err!=nil||st.Mode().Perm()!=0600{t.Fatal("registration must be private",err)}
 // The complete tool example must parse in the actual agent manifest parser.
 marker:="Example taskdeck_submit_patch arguments"
 start:=strings.Index(v.Prompt,marker)
 if start<0{t.Fatal("missing example")}
 example:=v.Prompt[start:]
 begin,end:=strings.Index(example,"{"),strings.LastIndex(example,"}")
 if begin<0||end<=begin{t.Fatal("invalid example")}
 args,err:=parseAgentPatchArguments(example[begin:end+1])
 if err!=nil{t.Fatal(err)}
 if _,err:=agentPatchPackage(args,identity);err!=nil{t.Fatal(err)}
 if err:=s.validateAgentNimGatewayProposal(args,identity.ID);err!=nil{t.Fatal(err)}
 // No automatic re-authorization after script edits.
 if err:=os.WriteFile(filepath.Join(root,nested),[]byte(script+"# changed\n"),0700);err!=nil{t.Fatal(err)}
 v=call("GET",endpoint,nil,200)
 if v.Registered||v.Prompt!=""{t.Fatal("stale registration advertised")}
 other,err:=filepath.EvalSymlinks(t.TempDir());if err!=nil{t.Fatal(err)}
 if _,err:=patchtool.EnsureProjectIdentity(other);err!=nil{t.Fatal(err)}
 s.Workspace=other
 call("POST",endpoint,map[string]any{"action":"revoke","project_root":other,"confirm":true},409)
 s.Workspace=root
 v=call("POST",endpoint,map[string]any{"action":"revoke","project_root":root,"confirm":true},200)
 if v.Registered||v.RegistrationPresent{t.Fatal("revoke did not clear registration")}
 if _,err:=os.Stat(filepath.Join(root,nested));err!=nil{t.Fatal("revoke removed gateway script",err)}
 s.Config.SharedServerEnabled=true
 call("GET",endpoint,nil,403)
 call("POST",endpoint,base,403)
}
