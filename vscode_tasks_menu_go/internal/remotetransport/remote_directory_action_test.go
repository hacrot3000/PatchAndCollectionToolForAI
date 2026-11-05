package remotetransport

import (
 "strings"
 "testing"
)

func TestRemoteDirectoryListTypedE2EEAuthenticatedRoundTrip(t *testing.T){
 for _,path:=range []string{".","src","src/util"}{
  t.Run(path,func(t *testing.T){
   browser,agent,_:=participants(t)
   request,err:=SealRemoteAction(browser,RemoteAction{
    RequestID:testActionID,Operation:RemoteActionProjectDirectoryList,Path:path,
   })
   if err!=nil{t.Fatal(err)}
   action,err:=OpenRemoteAction(agent,request)
   if err!=nil||action.Path!=path||action.Operation!=RemoteActionProjectDirectoryList{
    t.Fatalf("unverified directory action: %+v %v",action,err)
   }
   result,err:=SealRemoteDirectoryListResult(agent,testActionID,[]byte("D\tsub\nF\tx.txt"))
   if err!=nil{t.Fatal(err)}
   names,err:=OpenRemoteDirectoryListResult(browser,result,testActionID)
   if err!=nil||names!="D\tsub\nF\tx.txt"{t.Fatalf("E2EE directory names altered: %q %v",names,err)}
  })
 }
}

func TestRemoteDirectoryListRejectsMalformedOrEscapingRequests(t *testing.T){
 for _,path:=range []string{"","/etc","../../","dir/../evil","C:/Windows","src\\dir"}{
  client,_,_:=participants(t)
  frame,err:=SealRemoteAction(client,RemoteAction{
   RequestID:testActionID,Operation:RemoteActionProjectDirectoryList,Path:path,
  })
  if err==nil||len(frame)>0{t.Fatalf("unsafe directory path accepted %q",path)}
 }
 for _,clear:=range []string{
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.dir.list","path":"." ,"command":"id"}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.dir.list","path":"src","path":"../../etc"}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.dir.list","path":123}`,
 }{
  client,agent,_:=participants(t)
  frame,err:=client.Seal([]byte(clear))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteAction(agent,frame);err==nil||agent.Established(){
   t.Fatal("malformed authenticated directory request was not denied")
  }
 }
}

func TestRemoteDirectoryListResponseNotReusableAsFileReadOrBearer(t *testing.T){
 client,agent,_:=participants(t)
 sealed,err:=SealRemoteDirectoryListResult(agent,testActionID,[]byte("F\tname.txt"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteFileReadResult(client,sealed,testActionID);err==nil||
  client.Established(){t.Fatal("list result upgraded into file-read capability")}
 client2,agent2,_:=participants(t)
 malformed:=`{"type":"taskdeck-e2ee-directory-list-result-v1","request_id":"0123456789abcdef0123456789abcdef","ok":true,"remote_control_available":false,"content":"x","sha256":"`+strings.Repeat("0",64)+`","token":"forged"}`
 sealed,err=agent2.Seal([]byte(malformed))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteDirectoryListResult(client2,sealed,testActionID);err==nil||
  client2.Established(){t.Fatal("forged bearer survived strict directory reply decoding")}
}
