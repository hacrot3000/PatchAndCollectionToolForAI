package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestRemoteBinaryWindowStrictAEADRoundTrip(t *testing.T){
 client,agent,_:=participants(t)
 const id="0123456789abcdef0123456789abcdef"
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionProjectFileWindow,
  Path:"visible/blob.bin",Offset:128,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(request,[]byte("visible/blob.bin")){
  t.Fatal("Relay could inspect plaintext file path")
 }
 got,err:=OpenRemoteAction(agent,request)
 if err!=nil||got.Operation!=RemoteActionProjectFileWindow||
  got.Path!="visible/blob.bin"||got.Offset!=128{t.Fatalf("bad action: %+v %v",got,err)}
 data:=make([]byte,128)
 for i:=range data{data[i]=byte(i)}
 sha:=strings.Repeat("a",64)
 reply,err:=SealRemoteFileWindowResult(agent,id,FileWindow{
  Offset:128,Size:256,FileSHA256:sha,Data:data,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(reply,data)||bytes.Contains(reply,[]byte("content_b64")){
  t.Fatal("Relay could inspect binary file window")
 }
 window,err:=OpenRemoteFileWindowResult(client,reply,id,128)
 if err!=nil||window.Offset!=128||window.Size!=256||
  window.FileSHA256!=sha||!bytes.Equal(window.Data,data){t.Fatalf("bad E2EE binary window: %+v %v",window,err)}
}

func TestRemoteBinaryWindowRejectsInvalidPathsOffsetsAndAttackerFields(t *testing.T){
 for _,v:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionProjectFileWindow,Path:"../secret",Offset:0},
  {RequestID:testActionID,Operation:RemoteActionProjectFileWindow,Path:".secret",Offset:-1},
  {RequestID:testActionID,Operation:RemoteActionProjectFileWindow,Path:"blob",Offset:-1},
  {RequestID:testActionID,Operation:RemoteActionProjectFileWindow,Path:"blob",Offset:(2<<20)+1},
  {RequestID:testActionID,Operation:RemoteActionProjectFileWindow,Path:"",Offset:0},
  {RequestID:testActionID,Operation:RemoteActionProjectFileWindow,Path:"visible",TaskID:12},
 }{
  client,_,_:=participants(t)
  if result,err:=SealRemoteAction(client,v);err==nil||len(result)>0{
   t.Fatalf("invalid binary read accepted %+v",v)
  }
 }
 for _,bad:=range []string{
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.window","path":"x"}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.window","path":"x","offset":0,"command":"id"}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.window","path":"x","offset":0,"offset":128}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.window","path":"x","offset":0.0}`,
 }{
  client,agent,_:=participants(t)
  frame,err:=client.Seal([]byte(bad))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteAction(agent,frame);err==nil||agent.Established(){
   t.Fatalf("malformed binary window schema accepted %q",bad)
  }
 }
}

func TestRemoteBinaryWindowResultFailClosed(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 sha:=strings.Repeat("a",64)
 for _,w:=range []FileWindow{
  {Offset:0,Size:129,FileSHA256:sha,Data:make([]byte,129)},
  {Offset:2,Size:1,FileSHA256:sha,Data:nil},
  {Offset:0,Size:1,FileSHA256:sha,Data:nil},
  {Offset:0,Size:(2<<20)+1,FileSHA256:sha,Data:[]byte("x")},
  {Offset:0,Size:1,FileSHA256:"NOT-A-SHA",Data:[]byte("x")},
 }{
  _,agent,_:=participants(t)
  if data,err:=SealRemoteFileWindowResult(agent,id,w);err==nil||len(data)!=0{
   t.Fatalf("unsafe result accepted: %+v",w)
  }
 }
 t.Run("wrong offset",func(t *testing.T){
  client,agent,_:=participants(t)
  reply,err:=SealRemoteFileWindowResult(agent,id,FileWindow{
   Offset:1,Size:5,FileSHA256:sha,Data:[]byte("ab"),
  })
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteFileWindowResult(client,reply,id,2);err==nil||client.Established(){
   t.Fatal("host result with unexpected byte offset accepted")
  }
 })
 t.Run("forged capability",func(t *testing.T){
  client,agent,_:=participants(t)
  raw:=`{"type":"taskdeck-e2ee-file-window-result-v1","request_id":"0123456789abcdef0123456789abcdef","ok":true,"remote_control_available":true,"offset":0,"size":0,"file_sha256":"`+sha+`","content_b64":"","cookie":"forged"}`
  reply,err:=agent.Seal([]byte(raw));if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteFileWindowResult(client,reply,id,0);err==nil||client.Established(){
   t.Fatal("forged server capability survived strict result codec")
  }
 })
}
