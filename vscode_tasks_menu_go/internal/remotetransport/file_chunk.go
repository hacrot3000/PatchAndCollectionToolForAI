package remotetransport

import (
 "encoding/base64"
 "encoding/json"
)

// A larger read-only response has its OWN AEAD schema. The original
// 512-byte action/request decoder remains unchanged; Relay only sees an
// opaque ciphertext, never the path, full-file SHA or file bytes.
// Both the untrusted Relay and TaskDeck host independently enforce this
// bounded count. The host must NEVER infer authorization from Relay quota.
const ScopedFileTransferProofDomain="taskdeck-e2ee-scoped-file-transfer-v1"
const MaxScopedFileTransferRounds=64
const RemoteFileChunkMaxBytes=4096
const RemoteChunkMaxPlaintext=6144
const MaxEncryptedAppFrame=RemoteChunkMaxPlaintext+9+16
const remoteFileChunkResultType="taskdeck-e2ee-file-chunk-result-v1"

func validRemoteFileChunk(w FileWindow)bool{
 if w.Offset<0||w.Size<0||w.Size>RemoteFileWindowMaxSize||
  w.Offset>w.Size||!validFileWindowDigest(w.FileSHA256)||
  len(w.Data)>RemoteFileChunkMaxBytes{return false}
 expected:=w.Size-w.Offset
 if expected>RemoteFileChunkMaxBytes{expected=RemoteFileChunkMaxBytes}
 return int64(len(w.Data))==expected
}

func SealRemoteFileChunkResult(ch *Channel,id string,w FileWindow)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteFileChunk(w){return nil,ErrInvalidRemoteAction}
 raw,err:=json.Marshal(fileWindowWire{
  Type:remoteFileChunkResultType,RequestID:id,
  OK:true,RemoteControlAvailable:false,
  Offset:w.Offset,Size:w.Size,FileSHA256:w.FileSHA256,
  ContentBase64:base64.StdEncoding.EncodeToString(w.Data),
 })
 if err!=nil||len(raw)>RemoteChunkMaxPlaintext{return nil,ErrInvalidRemoteAction}
 sealed,err:=ch.Seal(raw)
 if err!=nil||len(sealed)>MaxEncryptedAppFrame{return nil,ErrInvalidRemoteAction}
 return sealed,nil
}

func OpenRemoteFileChunkResult(ch *Channel,frame []byte,id string,offset int64)(FileWindow,error){
 var none FileWindow
 if ch==nil||!ch.Established()||!validActionID(id)||
  offset<0||offset>RemoteFileWindowMaxSize||
  len(frame)<25||len(frame)>MaxEncryptedAppFrame{return none,ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return none,ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 raw,err:=decodeActionObjectBounded(clear,RemoteChunkMaxPlaintext,
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
  typ!=remoteFileChunkResultType||gotID!=id||!ok||available||
  gotOffset!=offset||len(encoded)>base64.StdEncoding.EncodedLen(RemoteFileChunkMaxBytes){
  ch.Close();return none,ErrInvalidRemoteAction
 }
 bytes,err:=base64.StdEncoding.Strict().DecodeString(encoded)
 if err!=nil||base64.StdEncoding.EncodeToString(bytes)!=encoded{
  ch.Close();return none,ErrInvalidRemoteAction
 }
 window:=FileWindow{Offset:gotOffset,Size:size,FileSHA256:hash,Data:bytes}
 if !validRemoteFileChunk(window){ch.Close();return none,ErrInvalidRemoteAction}
 return window,nil
}
