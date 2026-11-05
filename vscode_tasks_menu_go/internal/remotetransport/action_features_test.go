package remotetransport

import (
 "bytes"
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteFeaturesE2EEOnlySnapshotAndReplay(t *testing.T){
 client,agent,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionSessionFeatures,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(request,[]byte("session.features")) {
  t.Fatal("PHP/Relay observed encrypted feature inspection")
 }
 action,err:=OpenRemoteAction(agent,request)
 if err!=nil||action.Operation!=RemoteActionSessionFeatures {
  t.Fatalf("wrong authenticated read-only action: %+v %v",action,err)
 }
 snapshot:=RemoteFeatures{ProjectInfo:true,TasksPreview:false,GitStatus:true}
 sealed,err:=SealRemoteActionFeaturesResult(agent,action.RequestID,snapshot)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(sealed,[]byte(RemoteFeaturesResultProtocol)){
  t.Fatal("PHP/Relay saw feature list plaintext")
 }
 opened,err:=OpenRemoteActionFeaturesResult(client,sealed,testActionID)
 if err!=nil || opened!=snapshot {t.Fatalf("bad authenticated flags: %+v %v",opened,err)}
 if _,err:=OpenRemoteActionFeaturesResult(client,sealed,testActionID);err==nil {
  t.Fatal("replayed feature snapshot accepted")
 }
 if client.Established(){t.Fatal("replayed features did not poison E2EE")}
}

func TestRemoteFeaturesRejectsMaliciousWireFields(t *testing.T){
 basic:=`{"type":"taskdeck-e2ee-session-features-result-v1","request_id":"0123456789abcdef0123456789abcdef","session_ping":true,"project_info":true,"tasks_preview":false,"git_status":true,"remote_control_available":false`
 cases:=map[string]string{
  "remote capability forced":strings.Replace(basic+`}`,`"remote_control_available":false`,`"remote_control_available":true`,1),
  "unexpected command":basic+`,"command":"git push --force"}`,
  "task execution":basic+`,"tasks_run":true}`,
  "unknown key":basic+`,"terminal_exec":true}`,
  "duplicate key":basic+`,"git_status":false}`,
  "wrong ID":strings.Replace(basic+`}`,testActionID,strings.Repeat("a",32),1),
  "string instead of boolean":strings.Replace(basic+`}`,`"tasks_preview":false`,`"tasks_preview":"true"`,1),
  "null bool":strings.Replace(basic+`}`,`"project_info":true`,`"project_info":null`,1),
  "missing field":strings.Replace(basic+`}`,`,"git_status":true`,"",1),
 }
 for name,plaintext:=range cases{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=agent.Seal([]byte(plaintext))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteActionFeaturesResult(client,frame,testActionID);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("malformed or escalated features accepted: %v",err)
   }
   if client.Established(){t.Fatal("malicious feature payload did not poison AEAD")}
  })
 }
}

func TestRemoteFeaturesClientFixedAction(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(agent,frame)
  if err!=nil || request.Operation!=RemoteActionSessionFeatures {
   return nil,ErrRemoteActionDenied
  }
  return SealRemoteActionFeaturesResult(agent,request.RequestID,RemoteFeatures{GitStatus:true})
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 info,err:=native.Features(context.Background())
 if err!=nil || !info.GitStatus || info.ProjectInfo || info.TasksPreview{
  t.Fatalf("native features verified wrong availability: %+v %v",info,err)
 }
}

func TestRemoteFeaturesIndependentGitAndTaskFlags(t *testing.T){
 cases:=map[string]RemoteFeatures{
  "neither opt-in":{GitStatus:true},
  "only Git pull":{GitStatus:true,GitPull:true},
  "only Git commit":{GitStatus:true,GitCommit:true},
  "only long task":{GitStatus:true,TaskLong:true},
  "pull and task":{GitStatus:true,TaskLong:true,GitPull:true},
  "commit and task":{GitStatus:true,TaskLong:true,GitCommit:true},
  "commit and pull":{GitStatus:true,GitPull:true,GitCommit:true},
  "all opt-ins":{GitStatus:true,TaskLong:true,GitPull:true,GitCommit:true},
 }
 for name,want:=range cases{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   sealed,err:=SealRemoteActionFeaturesResult(agent,testActionID,want)
   if err!=nil{t.Fatal(err)}
   got,err:=OpenRemoteActionFeaturesResult(client,sealed,testActionID)
   if err!=nil||got!=want{
    t.Fatalf("optional feature flags mismatched: got=%+v want=%+v: %v",got,want,err)
   }
  })
 }
}
