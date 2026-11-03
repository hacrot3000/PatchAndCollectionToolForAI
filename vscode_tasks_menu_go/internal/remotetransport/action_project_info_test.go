package remotetransport

import (
 "bytes"
 "errors"
 "strings"
 "testing"
)

func TestRemoteProjectInfoE2EEConfidentialityAndReplay(t *testing.T){
 client,agent,_:=participants(t)
 req,err:=SealRemoteAction(client,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionProjectInfo,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(req,[]byte("project.info")){
  t.Fatal("PHP/Relay can read TaskDeck project action")
 }
 action,err:=OpenRemoteAction(agent,req)
 if err!=nil || action.Operation!=RemoteActionProjectInfo{t.Fatalf("unexpected request %+v %v",action,err)}
 metadata:=RemoteProjectInfo{
  ProjectName:"Dev Team",WorkspaceName:"Workspace",HostOS:"linux",
 }
 frame,err:=SealRemoteActionProjectInfoResult(agent,testActionID,metadata)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(frame,[]byte("Dev Team")) ||
  bytes.Contains(frame,[]byte("Workspace")){
  t.Fatal("TaskDeck project metadata exposed to compromised PHP")
 }
 opened,err:=OpenRemoteActionProjectInfoResult(client,frame,testActionID)
 if err!=nil || opened!=metadata{t.Fatalf("project metadata mismatch %+v %v",opened,err)}
 if _,err:=OpenRemoteActionProjectInfoResult(client,frame,testActionID);err==nil{
  t.Fatal("E2EE project reply replay was accepted")
 }
 if client.Established(){t.Fatal("replayed project info did not poison E2EE channel")}
}

func TestRemoteProjectInfoMalformedResponsePoisonsChannel(t *testing.T){
 cases:=map[string]string{
  "extra shell capability": `{"type":"taskdeck-e2ee-project-info-result-v1","request_id":"0123456789abcdef0123456789abcdef","project_name":"Dev","workspace_name":"project","host_os":"linux","remote_control_available":false,"command":"rm -rf /"}`,
  "extra full path": `{"type":"taskdeck-e2ee-project-info-result-v1","request_id":"0123456789abcdef0123456789abcdef","project_name":"Dev","workspace_name":"project","host_os":"linux","remote_control_available":false,"absolute_path":"/private/workspace"}`,
  "duplicate field": `{"type":"taskdeck-e2ee-project-info-result-v1","request_id":"0123456789abcdef0123456789abcdef","project_name":"Dev","project_name":"Hacked","workspace_name":"project","host_os":"linux","remote_control_available":false}`,
  "wrong ID": `{"type":"taskdeck-e2ee-project-info-result-v1","request_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","project_name":"Dev","workspace_name":"project","host_os":"linux","remote_control_available":false}`,
  "wrong platform": `{"type":"taskdeck-e2ee-project-info-result-v1","request_id":"0123456789abcdef0123456789abcdef","project_name":"Dev","workspace_name":"project","host_os":"root shell","remote_control_available":false}`,
  "control character": `{"type":"taskdeck-e2ee-project-info-result-v1","request_id":"0123456789abcdef0123456789abcdef","project_name":"Dev\u0000Secret","workspace_name":"project","host_os":"linux","remote_control_available":false}`,
  "privilege escalation": `{"type":"taskdeck-e2ee-project-info-result-v1","request_id":"0123456789abcdef0123456789abcdef","project_name":"Dev","workspace_name":"project","host_os":"linux","remote_control_available":true}`,
  "oversize": `{"type":"taskdeck-e2ee-project-info-result-v1","request_id":"0123456789abcdef0123456789abcdef","project_name":"`+strings.Repeat("x",97)+`","workspace_name":"project","host_os":"linux","remote_control_available":false}`,
 }
 for name,clear:=range cases{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   encrypted,err:=agent.Seal([]byte(clear));if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteActionProjectInfoResult(client,encrypted,testActionID);
    !errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("accepted forged E2EE project result: %v",err)
   }
   if client.Established(){t.Fatal("malformed project result left E2EE session active")}
  })
 }
}

func TestRemoteProjectInfoCannotSmuggleLongOrControlDisplayName(t *testing.T){
 _,agent,_:=participants(t)
 for _,name:=range []string{"",strings.Repeat("x",97),"bad\x00name","with\nnewline","hidden\u202epath"}{
  if _,err:=SealRemoteActionProjectInfoResult(agent,testActionID,RemoteProjectInfo{
   ProjectName:name,WorkspaceName:"project",HostOS:"linux",
  });err==nil{t.Fatalf("untrusted project display name accepted: %q",name)}
 }
 if _,err:=SealRemoteAction(agent,RemoteAction{
  RequestID:testActionID,Operation:"project.execute",
 });err==nil{t.Fatal("project execute was inadvertently permitted")}
}
