package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestAuthenticated32KiBFileBlockAeadAndWholeFileBoundary(t *testing.T){
 client,agent,_:=participants(t)
 id:="1234567890abcdef1234567890abcdef"
 req,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionProjectFileBlock,
  Path:"visible/binary.bin",Offset:32*1024,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(req,[]byte("visible/binary.bin")){t.Fatal("Relay saw file path")}
 action,err:=OpenRemoteAction(agent,req)
 if err!=nil||action.Operation!=RemoteActionProjectFileBlock||
  action.Path!="visible/binary.bin"||action.Offset!=32768{t.Fatalf("wrong block operation %+v %v",action,err)}
 payload:=make([]byte,RemoteFileBlockMaxBytes)
 for i:=range payload{payload[i]=byte(i%251)}
 sha:=strings.Repeat("a",64)
 sealed,err:=SealRemoteFileBlockResult(agent,id,FileWindow{
  Offset:32768,Size:2<<20,FileSHA256:sha,Data:payload,
 })
 if err!=nil{t.Fatal(err)}
 if len(sealed)<=MaxEncryptedChunkFrame||len(sealed)>MaxEncryptedAppFrame{
  t.Fatalf("incorrect distinct block envelope length %d",len(sealed))
 }
 if bytes.Contains(sealed,payload){t.Fatal("Relay observed E2EE block plaintext")}
 got,err:=OpenRemoteFileBlockResult(client,sealed,id,32768)
 if err!=nil||got.Size!=2<<20||got.Offset!=32768||
  got.FileSHA256!=sha||!bytes.Equal(got.Data,payload){
  t.Fatalf("authenticated 32KiB block changed: length=%d err=%v",len(got.Data),err)
 }
}

func TestAuthenticatedFileBlockBadLengthsAndTypeConfusion(t *testing.T){
 sha:=strings.Repeat("a",64)
 id:="1234567890abcdef1234567890abcdef"
 for _,w:=range []FileWindow{
  {Offset:0,Size:2<<20,FileSHA256:sha,Data:make([]byte,32767)},
  {Offset:0,Size:2<<20,FileSHA256:sha,Data:make([]byte,32769)},
  {Offset:1,Size:0,FileSHA256:sha,Data:nil},
  {Offset:0,Size:(2<<20)+1,FileSHA256:sha,Data:make([]byte,32768)},
  {Offset:0,Size:5,FileSHA256:sha,Data:make([]byte,4)},
  {Offset:0,Size:1,FileSHA256:"INVALID",Data:[]byte{1}},
 }{
  _,agent,_:=participants(t)
  sealed,err:=SealRemoteFileBlockResult(agent,id,w)
  if err==nil||len(sealed)>0{t.Fatal("inconsistent E2EE block accepted")}
 }
 t.Run("wrong offset",func(t *testing.T){
  client,agent,_:=participants(t)
  frame,err:=SealRemoteFileBlockResult(agent,id,FileWindow{
   Offset:0,Size:1,FileSHA256:sha,Data:[]byte{1},
  })
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteFileBlockResult(client,frame,id,32768);err==nil||client.Established(){
   t.Fatal("authenticated wrong-offset data accepted")
  }
 })
 t.Run("strict distinct result type",func(t *testing.T){
  client,agent,_:=participants(t)
  sealed,err:=SealRemoteFileBlockResult(agent,id,FileWindow{
   Offset:0,Size:1,FileSHA256:sha,Data:[]byte{1},
  })
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteFileChunkResult(client,sealed,id,0);err==nil||client.Established(){
   t.Fatal("32KiB block mislabeled as 4KiB chunk")
  }
 })
 t.Run("unknown capabilities",func(t *testing.T){
  client,agent,_:=participants(t)
  raw:=`{"type":"taskdeck-e2ee-file-block-result-v1","request_id":"1234567890abcdef1234567890abcdef","ok":true,"remote_control_available":false,"offset":0,"size":0,"file_sha256":"`+sha+`","content_b64":"","command":"id"}`
  frame,err:=agent.Seal([]byte(raw));if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteFileBlockResult(client,frame,id,0);err==nil||client.Established(){
   t.Fatal("forged command embedded in file block was admitted")
  }
 })
}

func TestAuthenticatedFileBlockRejectsUntrustedSourcePaths(t *testing.T){
 id:="1234567890abcdef1234567890abcdef"
 for _,path:=range []string{"","../secret","/etc/passwd","a\\b"}{
  client,_,_:=participants(t)
  sealed,err:=SealRemoteAction(client,RemoteAction{
   RequestID:id,Operation:RemoteActionProjectFileBlock,
   Path:path,Offset:0,
  })
  if err==nil||len(sealed)!=0{t.Fatalf("invalid 32KiB source path accepted %q",path)}
 }
}
