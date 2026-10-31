package remotetransport

import (
 "bytes"
 "encoding/base64"
 "strings"
 "testing"
)

func TestAuthenticatedBulkFileChunkStrictAeadRoundTrip(t *testing.T){
 client,agent,_:=participants(t)
 const id="0123456789abcdef0123456789abcdef"
 req,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionProjectFileChunk,
  Path:"public/file.bin",Offset:4096,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(req,[]byte("public/file.bin")){t.Fatal("Relay saw plaintext path")}
 action,err:=OpenRemoteAction(agent,req)
 if err!=nil||action.Path!="public/file.bin"||action.Offset!=4096||
  action.Operation!=RemoteActionProjectFileChunk{t.Fatalf("wrong bulk action: %+v %v",action,err)}
 bytes4096:=make([]byte,RemoteFileChunkMaxBytes)
 for i:=range bytes4096{bytes4096[i]=byte(i%251)}
 sha:=strings.Repeat("a",64)
 encoded,err:=SealRemoteFileChunkResult(agent,id,FileWindow{
  Offset:4096,Size:8192,FileSHA256:sha,Data:bytes4096,
 })
 if err!=nil{t.Fatal(err)}
 if len(encoded)<=MaxEncryptedPingFrame||len(encoded)>MaxEncryptedAppFrame{
  t.Fatalf("bulk ciphertext size not bounded: %d",len(encoded))
 }
 if bytes.Contains(encoded,bytes4096){t.Fatal("Relay saw clear binary bytes")}
 got,err:=OpenRemoteFileChunkResult(client,encoded,id,4096)
 if err!=nil||got.Offset!=4096||got.Size!=8192||
  got.FileSHA256!=sha||!bytes.Equal(got.Data,bytes4096){
  t.Fatalf("unexpected bulk chunk: size=%d %v",len(got.Data),err)
 }
}

func TestBulkFileChunkRejectsForgedTypeDataAndOffset(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 sha:=strings.Repeat("a",64)
 for _,w:=range []FileWindow{
  {Offset:0,Size:5000,FileSHA256:sha,Data:make([]byte,4095)},
  {Offset:0,Size:5000,FileSHA256:sha,Data:make([]byte,4097)},
  {Offset:2,Size:1,FileSHA256:sha,Data:nil},
  {Offset:0,Size:(2<<20)+1,FileSHA256:sha,Data:[]byte{1}},
  {Offset:0,Size:1,FileSHA256:"FORGED",Data:[]byte{1}},
 }{
  _,agent,_:=participants(t)
  frame,err:=SealRemoteFileChunkResult(agent,id,w)
  if err==nil||len(frame)>0{t.Fatalf("unsafe bulk result sealed: %+v",w)}
 }
 t.Run("wrong offset",func(t *testing.T){
  client,agent,_:=participants(t)
  sealed,err:=SealRemoteFileChunkResult(agent,id,FileWindow{
   Offset:128,Size:256,FileSHA256:sha,Data:make([]byte,128),
  })
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteFileChunkResult(client,sealed,id,0);err==nil||client.Established(){
   t.Fatal("wrong-offset bulk result accepted")
  }
 })
 t.Run("old type confusion",func(t *testing.T){
  client,agent,_:=participants(t)
  sealed,err:=SealRemoteFileChunkResult(agent,id,FileWindow{
   Offset:0,Size:100,FileSHA256:sha,Data:make([]byte,100),
  })
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteFileWindowResult(client,sealed,id,0);err==nil||client.Established(){
   t.Fatal("bulk result accepted as legacy 128-byte window")
  }
 })
 t.Run("forged cookie",func(t *testing.T){
  client,agent,_:=participants(t)
  raw:=`{"type":"taskdeck-e2ee-file-chunk-result-v1","request_id":"0123456789abcdef0123456789abcdef","ok":true,"remote_control_available":false,"offset":0,"size":0,"file_sha256":"`+sha+`","content_b64":"","cookie":"danger"}`
  ciphertext,err:=agent.Seal([]byte(raw))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteFileChunkResult(client,ciphertext,id,0);err==nil||client.Established(){
   t.Fatal("forged result field accepted")
  }
 })
 t.Run("invalid base64",func(t *testing.T){
  client,agent,_:=participants(t)
  raw:=`{"type":"taskdeck-e2ee-file-chunk-result-v1","request_id":"0123456789abcdef0123456789abcdef","ok":true,"remote_control_available":false,"offset":0,"size":1,"file_sha256":"`+sha+`","content_b64":"`+base64.StdEncoding.EncodeToString([]byte{1})+`=="}`
  ciphertext,err:=agent.Seal([]byte(raw))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteFileChunkResult(client,ciphertext,id,0);err==nil||client.Established(){
   t.Fatal("noncanonical base64 accepted")
  }
 })
}

func TestBulkFileChunkRejectsNoncanonicalRequest(t *testing.T){
 for _,path:=range []string{"","../etc","/etc/passwd","a\\b"}{
  client,_,_:=participants(t)
  req,err:=SealRemoteAction(client,RemoteAction{
   RequestID:testActionID,Operation:RemoteActionProjectFileChunk,
   Path:path,Offset:0,
  })
  if err==nil||len(req)>0{t.Fatalf("invalid bulk path accepted %q",path)}
 }
 for _,raw:=range []string{
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.chunk","path":"x","offset":0,"shell":"id"}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.chunk","path":"x","offset":0,"offset":123}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.chunk","path":"x","offset":1.0}`,
 }{
  client,agent,_:=participants(t)
  frame,err:=client.Seal([]byte(raw))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteAction(agent,frame);err==nil||agent.Established(){
   t.Fatal("malformed bulk request admitted")
  }
 }
}
