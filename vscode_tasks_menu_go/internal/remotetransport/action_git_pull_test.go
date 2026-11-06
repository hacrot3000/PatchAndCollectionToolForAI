package remotetransport

import (
 "bytes"
 "context"
 "errors"
 "strings"
 "testing"
)

const gitPullBefore="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const gitPullAfter="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestRemoteGitPullE2EEAndNativeClient(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  req,err:=OpenRemoteAction(agent,frame)
  if err!=nil||req.Operation!=RemoteActionGitPull||req.TaskID!=0||
   req.Choice!=0||req.Text!=""{
   return nil,ErrInvalidRemoteAction
  }
  if bytes.Contains(frame,[]byte(RemoteActionGitPull)){
   t.Fatal("Relay saw Git pull plaintext")
  }
  return SealRemoteGitPullResult(agent,req.RequestID,RemoteGitPullResult{
   Before:gitPullBefore,After:gitPullAfter,Updated:true,
  })
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 result,err:=native.GitPull(context.Background())
 if err!=nil||!result.Updated||result.Before!=gitPullBefore||result.After!=gitPullAfter{
  t.Fatalf("native verified result %+v: %v",result,err)
 }
}

func TestRemoteGitPullResultRejectsMalformedRecords(t *testing.T){
 base:=`{"type":"taskdeck-e2ee-git-pull-result-v1","request_id":"0123456789abcdef0123456789abcdef","before":"`+
  gitPullBefore+`","after":"`+gitPullAfter+`","updated":true,"remote_control_available":false}`
 cases:=map[string]string{
  "wrong id":strings.Replace(base,testActionID,strings.Repeat("f",32),1),
  "wrong type":strings.Replace(base,RemoteGitPullResultProtocol,"other",1),
  "fabricated control":strings.Replace(base,`"remote_control_available":false`,`"remote_control_available":true`,1),
  "extra Git arguments":strings.Replace(base,`}`,`,"branch":"attack"}`,1),
  "null flag":strings.Replace(base,`"updated":true`,`"updated":null`,1),
  "wrong updated":strings.Replace(base,`"updated":true`,`"updated":false`,1),
  "uppercase sha":strings.Replace(base,gitPullBefore,strings.ToUpper(gitPullBefore),1),
  "duplicate":strings.Replace(base,`}`,`,"updated":true}`,1),
 }
 for name,clear:=range cases{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=agent.Seal([]byte(clear))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitPullResult(client,frame,testActionID);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("invalid result accepted: %v",err)
   }
   if client.Established(){t.Fatal("invalid Git response did not close E2EE")}
  })
 }
}

func TestRemoteGitPullCodecRefusesInconsistentResult(t *testing.T){
 _,agent,_:=participants(t)
 for _,value:=range []RemoteGitPullResult{
  {Before:gitPullBefore,After:gitPullBefore,Updated:true},
  {Before:gitPullBefore,After:gitPullAfter,Updated:false},
  {Before:strings.Repeat("a",64),After:gitPullAfter,Updated:true},
 }{
  if _,err:=SealRemoteGitPullResult(agent,testActionID,value);err==nil{
   t.Fatalf("inconsistent Git outcome sealed: %+v",value)
  }
 }
}
