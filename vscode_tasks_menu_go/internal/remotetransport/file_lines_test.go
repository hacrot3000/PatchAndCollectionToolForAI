package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestRemoteFileLinesAuthenticatedE2EEAndResultTypeIsolation(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,host,_:=participants(t)
 req,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionProjectFileLines,
  Path:"src/file with spaces.go",Offset:201,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(req,[]byte("project.file.lines"))||
  bytes.Contains(req,[]byte("src/file with spaces.go")){
  t.Fatal("Relay saw source path or operation")
 }
 got,err:=OpenRemoteAction(host,req)
 if err!=nil||got.Operation!=RemoteActionProjectFileLines||
  got.Path!="src/file with spaces.go"||got.Offset!=201{
  t.Fatalf("wrong typed E2EE line request: %+v %v",got,err)
 }
 body:=[]byte("FILE_SHA256 "+strings.Repeat("a",64)+
  "\nSTART_LINE 201\nNEXT_LINE 0\nTOTAL_LINES 201\n-- COMPLETE SOURCE LINES --\n201: value\n")
 reply,err:=SealRemoteFileLinesResult(host,id,body)
 if err!=nil{t.Fatal(err)}
 value,err:=OpenRemoteFileLinesResult(client,reply,id)
 if err!=nil||value!=string(body){t.Fatalf("wrong line result: %q %v",value,err)}
 client,host,_=participants(t)
 reply,err=SealRemoteFileLinesResult(host,id,body)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteFilePreviewResult(client,reply,id);err==nil||client.Established(){
  t.Fatal("line page was accepted as complete-file edit preview")
 }
}

func TestRemoteFileLinesRejectsInvalidRangeAndForeignFields(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 for _,action:=range []RemoteAction{
  {Operation:RemoteActionProjectFileLines,Path:"src/a.go",Offset:0},
  {Operation:RemoteActionProjectFileLines,Path:"src/a.go",Offset:-1},
  {Operation:RemoteActionProjectFileLines,Path:"src/a.go",Offset:2<<20+1},
  {Operation:RemoteActionProjectFileLines,Path:"../private",Offset:1},
  {Operation:RemoteActionProjectFileLines,Path:".git/config",Offset:1},
  {Operation:RemoteActionProjectFileLines,Path:"src/a.go",Offset:1,Content:"spoof"},
  {Operation:RemoteActionProjectFileLines,Path:"src/a.go",Offset:1,ExpectedSHA256:strings.Repeat("a",64)},
  {Operation:RemoteActionProjectFileLines,Path:"src/a.go",Offset:1,TaskID:5},
 }{
  client,_,_:=participants(t)
  action.RequestID=id
  frame,err:=SealRemoteAction(client,action)
  if err==nil||len(frame)!=0{
   t.Fatalf("accepted invalid E2EE line-page action: %+v",action)
  }
 }
 client,host,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.lines","path":"a.txt","offset":1,"command":"sh"}`)
 frame,err:=client.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(host,frame);err==nil||host.Established(){
  t.Fatal("forged arbitrary command escaped fixed schema")
 }
}

func TestRemoteFileLinesNeverTruncatesOversizedResult(t *testing.T){
 client,host,_:=participants(t)
 const id="0123456789abcdef0123456789abcdef"
 frame,err:=SealRemoteFileLinesResult(host,id,[]byte(strings.Repeat("a",RemoteGitDiffMaxBytes+1)))
 if err==nil||len(frame)!=0{t.Fatal("silent E2EE line-page truncation")}
 _=client
}
