package remotetransport

import (
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
)

// One verified binary window of a specific small file version. No raw file
// path, credentials or file handles are serialized in this result.
// The full-file digest enables the trusted client to reject spliced windows
// read from different versions, even when using a new E2EE login.
const RemoteFileWindowMaxBytes=128
const RemoteFileWindowMaxSize=int64(2<<20)
const remoteFileWindowResultType="taskdeck-e2ee-file-window-result-v1"

type FileWindow struct {
 Offset int64
 Size int64
 FileSHA256 string
 Data []byte
}

type fileWindowWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 OK bool `json:"ok"`
 RemoteControlAvailable bool `json:"remote_control_available"`
 Offset int64 `json:"offset"`
 Size int64 `json:"size"`
 FileSHA256 string `json:"file_sha256"`
 ContentBase64 string `json:"content_b64"`
}

func validFileWindowDigest(value string)bool{
 if len(value)!=sha256.Size*2{return false}
 if _,err:=hex.DecodeString(value);err!=nil{return false}
 for _,ch:=range value{
  if (ch<'0'||ch>'9')&&(ch<'a'||ch>'f'){return false}
 }
 return true
}
func validFileWindow(offset,size int64,sha string,data []byte)bool{
 if offset<0||size<0||size>RemoteFileWindowMaxSize||offset>size||
  !validFileWindowDigest(sha)||len(data)>RemoteFileWindowMaxBytes{
  return false
 }
 // A successful window must contain EVERY byte from the requested offset
 // up to EOF or this protocol's 128-byte bound. Accepting a short non-EOF
 // response makes a malformed Agent reply appear complete to callers of
 // ReadFileWindow, even though the resumable downloader rejects it later.
 // Verify this at both the sealing AND opening boundary.
 expected:=size-offset
 if expected>RemoteFileWindowMaxBytes{expected=RemoteFileWindowMaxBytes}
 return int64(len(data))==expected
}

func SealRemoteFileWindowResult(ch *Channel,id string,window FileWindow)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validFileWindow(window.Offset,window.Size,window.FileSHA256,window.Data){
  return nil,ErrInvalidRemoteAction
 }
 raw,err:=json.Marshal(fileWindowWire{
  Type:remoteFileWindowResultType,RequestID:id,OK:true,
  RemoteControlAvailable:false,Offset:window.Offset,Size:window.Size,
  FileSHA256:window.FileSHA256,
  ContentBase64:base64.StdEncoding.EncodeToString(window.Data),
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}

func OpenRemoteFileWindowResult(ch *Channel,frame []byte,wantID string,wantOffset int64)(FileWindow,error){
 var zero FileWindow
 if ch==nil||!ch.Established()||!validActionID(wantID)||
  wantOffset<0||wantOffset>RemoteFileWindowMaxSize{return zero,ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return zero,ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 fields,err:=decodeActionObject(clear,
  "type","request_id","ok","remote_control_available",
  "offset","size","file_sha256","content_b64")
 if err!=nil{ch.Close();return zero,ErrInvalidRemoteAction}
 var typ,id,hash,encoded string
 var offset,size int64
 var ok,available bool
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&id)!=nil||
  json.Unmarshal(fields["ok"],&ok)!=nil||
  json.Unmarshal(fields["remote_control_available"],&available)!=nil||
  json.Unmarshal(fields["offset"],&offset)!=nil||
  json.Unmarshal(fields["size"],&size)!=nil||
  json.Unmarshal(fields["file_sha256"],&hash)!=nil||
  json.Unmarshal(fields["content_b64"],&encoded)!=nil||
  typ!=remoteFileWindowResultType||id!=wantID||!ok||available||
  offset!=wantOffset||len(encoded)>base64.StdEncoding.EncodedLen(RemoteFileWindowMaxBytes){
  ch.Close();return zero,ErrInvalidRemoteAction
 }
 data,err:=base64.StdEncoding.Strict().DecodeString(encoded)
 if err!=nil||base64.StdEncoding.EncodeToString(data)!=encoded||
  !validFileWindow(offset,size,hash,data){
  ch.Close();return zero,ErrInvalidRemoteAction
 }
 return FileWindow{Offset:offset,Size:size,FileSHA256:hash,Data:data},nil
}
