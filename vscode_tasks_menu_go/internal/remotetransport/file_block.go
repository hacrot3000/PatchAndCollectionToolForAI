package remotetransport

import (
 "encoding/base64"
 "encoding/json"
)

// 32KiB is the maximum plaintext FILE payload, not a generic Relay frame.
// The 64-chunk in-process host and Relay budgets bound a whole download to
// 2MiB; no interactive shell or variable remote method is introduced.
const remoteFileBlockResultType="taskdeck-e2ee-file-block-result-v1"

func validRemoteFileBlock(w FileWindow) bool {
 if w.Offset<0||w.Size<0||w.Size>RemoteFileWindowMaxSize||
  w.Offset>w.Size||!validFileWindowDigest(w.FileSHA256)||
  len(w.Data)>RemoteFileBlockMaxBytes{return false}
 n:=w.Size-w.Offset
 if n>RemoteFileBlockMaxBytes{n=RemoteFileBlockMaxBytes}
 return int64(len(w.Data))==n
}

func SealRemoteFileBlockResult(ch *Channel,id string,w FileWindow)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!validRemoteFileBlock(w){
  return nil,ErrInvalidRemoteAction
 }
 raw,err:=json.Marshal(fileWindowWire{
  Type:remoteFileBlockResultType,RequestID:id,
  OK:true,RemoteControlAvailable:false,
  Offset:w.Offset,Size:w.Size,FileSHA256:w.FileSHA256,
  ContentBase64:base64.StdEncoding.EncodeToString(w.Data),
 })
 if err!=nil||len(raw)>RemoteBlockMaxPlaintext{return nil,ErrInvalidRemoteAction}
 sealed,err:=ch.sealFileBlock(raw)
 if err!=nil||len(sealed)>MaxEncryptedAppFrame{return nil,ErrInvalidRemoteAction}
 return sealed,nil
}

func OpenRemoteFileBlockResult(ch *Channel,frame []byte,id string,offset int64)(FileWindow,error){
 var none FileWindow
 if ch==nil||!ch.Established()||!validActionID(id)||offset<0||
  offset>RemoteFileWindowMaxSize||len(frame)<25||len(frame)>MaxEncryptedAppFrame{
  return none,ErrInvalidRemoteAction
 }
 clear,err:=ch.openFileBlock(frame)
 if err!=nil{return none,ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 raw,err:=decodeActionObjectBounded(clear,RemoteBlockMaxPlaintext,
  "type","request_id","ok","remote_control_available",
  "offset","size","file_sha256","content_b64")
 if err!=nil{ch.Close();return none,ErrInvalidRemoteAction}
 var typ,gotID,hash,encoded string
 var gotOffset,size int64
 var ok,available bool
 if json.Unmarshal(raw["type"],&typ)!=nil||
  json.Unmarshal(raw["request_id"],&gotID)!=nil||
  json.Unmarshal(raw["ok"],&ok)!=nil||
  json.Unmarshal(raw["remote_control_available"],&available)!=nil||
  json.Unmarshal(raw["offset"],&gotOffset)!=nil||
  json.Unmarshal(raw["size"],&size)!=nil||
  json.Unmarshal(raw["file_sha256"],&hash)!=nil||
  json.Unmarshal(raw["content_b64"],&encoded)!=nil||
  typ!=remoteFileBlockResultType||gotID!=id||!ok||available||
  gotOffset!=offset||
  len(encoded)>base64.StdEncoding.EncodedLen(RemoteFileBlockMaxBytes){
  ch.Close();return none,ErrInvalidRemoteAction
 }
 data,err:=base64.StdEncoding.Strict().DecodeString(encoded)
 if err!=nil||base64.StdEncoding.EncodeToString(data)!=encoded{
  ch.Close();return none,ErrInvalidRemoteAction
 }
 result:=FileWindow{Offset:gotOffset,Size:size,FileSHA256:hash,Data:data}
 if !validRemoteFileBlock(result){ch.Close();return none,ErrInvalidRemoteAction}
 return result,nil
}
