package remotetransport

import (
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteFilesRootStrictEnvelope(t *testing.T){
 c,a,_:=participants(t)
 value:=RemoteFilesRoot{Items:[]string{"D:docs","F:main.go"},More:true}
 frame,err:=SealRemoteFilesRoot(a,testActionID,value)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteFilesRoot(c,frame,testActionID)
 if err!=nil||len(got.Items)!=2||got.Items[0]!="D:docs"||!got.More{
  t.Fatalf("roundtrip: %+v %v",got,err)
 }
 c,_,_ = participants(t)
 if _,err:=SealRemoteFilesRoot(c,testActionID,RemoteFilesRoot{Items:[]string{},More:true});
  !errors.Is(err,ErrInvalidRemoteAction){t.Fatal("empty continuation page accepted")}
 for _,invalid:=range [][]string{
  {"F:.secret"},{"F:../secret"},{"F:hello world"},{"F:ab","D:ab"},
  {"F:zz","F:aa"},{"F:bad/name"},{"D:bad\\name"},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteFilesRoot(c,testActionID,RemoteFilesRoot{Items:invalid});
   !errors.Is(err,ErrInvalidRemoteAction){t.Fatalf("accepted %v: %v",invalid,err)}
 }
 c,_,_ = participants(t)
 many:=make([]string,9)
 for i:=range many{many[i]="F:"+string(rune('a'+i))}
 if _,err:=SealRemoteFilesRoot(c,testActionID,RemoteFilesRoot{Items:many});
  !errors.Is(err,ErrInvalidRemoteAction){t.Fatal("oversized result accepted")}
}

func TestRemoteFilesRootNativeNoArguments(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(agent,frame)
  if err!=nil||request.Operation!=RemoteActionFilesRoot||
   request.TaskID!=0||request.Choice!=0||request.Text!=""||
   request.Message!=""||request.FileIndex!=0||request.Snapshot!=""||
   request.TerminalInput!=""{return nil,ErrInvalidRemoteAction}
  return SealRemoteFilesRoot(agent,request.RequestID,
   RemoteFilesRoot{Items:[]string{"F:README.md"}})
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 result,err:=native.FilesRoot(context.Background())
 if err!=nil||len(result.Items)!=1||result.Items[0]!="F:README.md"{
  t.Fatalf("E2EE names: %+v %v",result,err)
 }
 for _,value:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionFilesRoot,TaskID:1},
  {RequestID:testActionID,Operation:RemoteActionFilesRoot,Text:"README.md"},
  {RequestID:testActionID,Operation:RemoteActionFilesRoot,Message:"../../secret"},
  {RequestID:testActionID,Operation:RemoteActionFilesRoot,FileIndex:1,Snapshot:strings.Repeat("a",32)},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,value);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("unexpected remote path/selector acceptance: %+v",value)
  }
 }
}

func TestRemoteFilesRootNextNoArguments(t *testing.T){
 c,a,_:=participants(t)
 exchanged:=0
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(a,frame)
  if err!=nil||request.Operation!=RemoteActionFilesRootNext||
   request.TaskID!=0||request.Choice!=0||request.Text!=""||
   request.Message!=""||request.Snapshot!=""||request.FileIndex!=0||
   request.TerminalInput!=""||request.Cols!=0||request.Rows!=0{
   return nil,ErrInvalidRemoteAction
  }
  exchanged++
  return SealRemoteFilesRoot(a,request.RequestID,RemoteFilesRoot{
   Items:[]string{"F:second.go"},More:false,
  })
 })
 native,err:=NewPingClient(c,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 result,err:=native.FilesRootNext(context.Background())
 if err!=nil||exchanged!=1||len(result.Items)!=1||result.Items[0]!="F:second.go"{
  t.Fatalf("typed Files next reply invalid: %+v %v",result,err)
 }
 for _,request:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionFilesRootNext,TaskID:2},
  {RequestID:testActionID,Operation:RemoteActionFilesRootNext,Text:"../"},
  {RequestID:testActionID,Operation:RemoteActionFilesRootNext,FileIndex:2},
  {RequestID:testActionID,Operation:RemoteActionFilesRootNext,Snapshot:"secret"},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,request);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("accepted client pagination offset/path: %+v",request)
  }
 }
}
