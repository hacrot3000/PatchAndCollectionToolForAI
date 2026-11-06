package remotetransport

import (
 "context"
 "errors"
 "testing"
)

func TestRemoteFilesDirectoryIndexOnlyStrictE2EE(t *testing.T){
 for _,op:=range []string{RemoteActionFilesDirSelect,RemoteActionFilesDirNext}{
  t.Run(op,func(t *testing.T){
   index:=0
   if op==RemoteActionFilesDirSelect{index=5}
   client,agent,_:=participants(t)
   desired:=RemoteAction{RequestID:testActionID,Operation:op,FileIndex:index}
   sealed,err:=SealRemoteAction(client,desired)
   if err!=nil{t.Fatal(err)}
   request,err:=OpenRemoteAction(agent,sealed)
   if err!=nil||request!=desired{
    t.Fatalf("index-only E2EE action mismatch: %+v %v",request,err)
   }
   for _,malicious:=range []RemoteAction{
    {RequestID:testActionID,Operation:op,FileIndex:129},
    {RequestID:testActionID,Operation:op,FileIndex:1,Snapshot:"../private"},
    {RequestID:testActionID,Operation:op,Text:"../../private"},
    {RequestID:testActionID,Operation:op,Message:"../../private"},
    {RequestID:testActionID,Operation:op,TaskID:3},
    {RequestID:testActionID,Operation:op,Choice:2},
   }{
    // A next-page request has NO index. Select requires a strict 1..128.
    client,_,_:=participants(t)
    if _,err:=SealRemoteAction(client,malicious);
     !errors.Is(err,ErrInvalidRemoteAction){
     t.Fatalf("accepted attacker-supplied directory arguments: %+v",malicious)
    }
   }
   if op==RemoteActionFilesDirSelect{
    client,_,_:=participants(t)
    if _,err:=SealRemoteAction(client,RemoteAction{
     RequestID:testActionID,Operation:op,
    });!errors.Is(err,ErrInvalidRemoteAction){
     t.Fatal("accepted a directory selection without an issued index")
    }
   }
  })
 }
}

func TestRemoteFilesDirectoryTypedNativeClient(t *testing.T){
 for _,op:=range []string{RemoteActionFilesDirSelect,RemoteActionFilesDirNext}{
  t.Run(op,func(t *testing.T){
   client,agent,_:=participants(t)
   exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
    action,err:=OpenRemoteAction(agent,frame)
    if err!=nil||action.Operation!=op||action.Snapshot!=""||
     action.Message!=""||action.TaskID!=0||action.Cols!=0||
     action.Rows!=0||action.Choice!=0||action.Text!=""{
     return nil,ErrInvalidRemoteAction
    }
    if op==RemoteActionFilesDirSelect&&action.FileIndex!=3{
     return nil,ErrInvalidRemoteAction
    }
    if op==RemoteActionFilesDirNext&&action.FileIndex!=0{
     return nil,ErrInvalidRemoteAction
    }
    return SealRemoteFilesRoot(agent,action.RequestID,RemoteFilesRoot{
     Items:[]string{"D:docs","F:guide.txt"},More:false,
    })
   })
   native,err:=NewPingClient(client,exchange)
   if err!=nil{t.Fatal(err)}
   defer native.Close()
   var response RemoteFilesRoot
   if op==RemoteActionFilesDirSelect{response,err=native.FilesDirSelect(context.Background(),3)
   }else{response,err=native.FilesDirNext(context.Background())}
   if err!=nil||len(response.Items)!=2||response.Items[1]!="F:guide.txt"{
    t.Fatalf("typed directory listing E2EE failed: %+v %v",response,err)
   }
  })
 }
}
