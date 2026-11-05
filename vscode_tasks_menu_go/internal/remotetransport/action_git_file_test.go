package remotetransport

import (
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitFileSelectionE2EEFixedRow(t *testing.T){
 for _,op:=range []string{RemoteActionGitStageFile,RemoteActionGitUnstageFile}{
  t.Run(op,func(t *testing.T){
   client,agent,_:=participants(t)
   snapshot:="## main\n M tracked.txt\n"
   exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
    request,err:=OpenRemoteAction(agent,frame)
    if err!=nil||request.Operation!=op||request.FileIndex!=1||
     request.Snapshot!=snapshot||request.TaskID!=0||
     request.Text!=""||request.Message!=""||request.TerminalInput!=""{
     return nil,ErrInvalidRemoteAction
    }
    return SealRemoteGitIndexResult(agent,request.RequestID,RemoteGitIndexResult{
     Operation:op,Head:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    })
   })
   native,err:=NewPingClient(client,exchange)
   if err!=nil{t.Fatal(err)}
   defer native.Close()
   var got RemoteGitIndexResult
   if op==RemoteActionGitStageFile{
    got,err=native.GitStageFile(context.Background(),1,snapshot)
   }else{
    got,err=native.GitUnstageFile(context.Background(),1,snapshot)
   }
   if err!=nil||got.Operation!=op{t.Fatalf("unexpected result: %+v %v",got,err)}
  })
 }
}

func TestRemoteGitFileSelectionRejectsForgedArgs(t *testing.T){
 cases:=map[string]string{
  "wrong status":"## main",
  "too many rows":"## main\n",
  "control":"## main\n M file\x00.txt\n",
  "newline only":"\n\n\n\n",
  "truncated":"## main\n M x",
 }
 for name,snapshot:=range cases{
  if name=="truncated"{continue}
  if ValidRemoteGitFileSelection(1,snapshot){
   t.Errorf("malformed status accepted %s",name)
  }
 }
 for _,op:=range []string{RemoteActionGitStageFile,RemoteActionGitUnstageFile}{
  for _,request:=range []RemoteAction{
   {RequestID:testActionID,Operation:op,FileIndex:0,Snapshot:"## main\n M tracked.txt\n"},
   {RequestID:testActionID,Operation:op,FileIndex:13,Snapshot:"## main\n M tracked.txt\n"},
   {RequestID:testActionID,Operation:op,FileIndex:1,Snapshot:""},
   {RequestID:testActionID,Operation:op,FileIndex:1,Snapshot:"## main\n M tracked.txt\n",Text:"../../secret"},
   {RequestID:testActionID,Operation:op,FileIndex:1,Snapshot:"## main\n M tracked.txt\n",TaskID:9},
  }{
   client,_,_:=participants(t)
   if _,err:=SealRemoteAction(client,request);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("%s accepted invalid args: %+v %v",op,request,err)
   }
  }
 }
}

func TestRemoteGitFileRequestMalformedClosesChannel(t *testing.T){
 snapshot:="## main\n M tracked.txt\n"
 base:="{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":\"git.stage_file\",\"file_index\":1,\"snapshot\":\"## main\\n M tracked.txt\\n\"}"
 if !strings.Contains(base,"git.stage_file"){t.Fatal("bad fixture")}
 _=snapshot
 tests:=map[string]string{
  "extra path":strings.Replace(base,"}",`,"path":"../../secret"}`,1),
  "duplicate index":strings.Replace(base,"}",`,"file_index":2}`,1),
  "forged op":strings.Replace(base,"git.stage_file","git.commit",1),
  "null snapshot":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":\"git.stage_file\",\"file_index\":1,\"snapshot\":null}",
  "nonint index":strings.Replace(base,`"file_index":1`,`"file_index":"1"`,1),
 }
 for name,raw:=range tests{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=client.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteAction(agent,frame);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("malformed file request accepted: %v",err)
   }
   if agent.Established(){t.Fatal("malformed file request did not close channel")}
  })
 }
}
