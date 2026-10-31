package remotetransport

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "unicode/utf8"
)

// Separate authenticated, 4KiB read-only result types. These NEVER grant
// write, shell or file-transfer authority. A compromised Relay sees only AEAD.
const RemoteGitDiffMaxBytes=4096
const RemoteGitDiffMaxPlaintext=6144
const MaxEncryptedGitDiffFrame=RemoteGitDiffMaxPlaintext+9+16
const remoteGitDiffResultType="taskdeck-e2ee-git-diff-result-v1"
const RemoteFilePreviewMaxBytes=4096
const remoteFilePreviewResultType="taskdeck-e2ee-file-preview-result-v1"

func SealRemoteGitDiffResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitDiffResultType)
}

func OpenRemoteGitDiffResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteGitDiffResultType)
}

// Preview reads only a COMPLETE <=4KiB visible UTF-8 file. It is not a
// binary window/download and does not transition into file-transfer mode.
func SealRemoteFilePreviewResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteFilePreviewResultType)
}
func OpenRemoteFilePreviewResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteFilePreviewResultType)
}

func validRemoteBoundedTextType(typ string)bool{
 return typ==remoteGitDiffResultType||typ==remoteFilePreviewResultType
}

func sealRemoteBoundedText4096(ch *Channel,id string,content []byte,typ string)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteBoundedTextType(typ)||
  len(content)>RemoteGitDiffMaxBytes||!utf8.Valid(content){
  return nil,ErrInvalidRemoteAction
 }
 sum:=sha256.Sum256(content)
 clear,err:=json.Marshal(remoteFileReadResultWire{
  Type:typ,RequestID:id,OK:true,
  RemoteControlAvailable:false,Content:string(content),
  SHA256:hex.EncodeToString(sum[:]),
 })
 if err!=nil||len(clear)>RemoteGitDiffMaxPlaintext{return nil,ErrInvalidRemoteAction}
 frame,err:=ch.Seal(clear)
 if err!=nil||len(frame)>MaxEncryptedGitDiffFrame{return nil,ErrInvalidRemoteAction}
 return frame,nil
}

func openRemoteBoundedText4096(ch *Channel,frame []byte,id,expectedType string)(string,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteBoundedTextType(expectedType)||
  len(frame)<25||len(frame)>MaxEncryptedGitDiffFrame{return "",ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return "",ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 raw,err:=decodeActionObjectBounded(clear,RemoteGitDiffMaxPlaintext,
  "type","request_id","ok","remote_control_available","content","sha256")
 if err!=nil{ch.Close();return "",ErrInvalidRemoteAction}
 var typ,receivedID,content,hash string
 var ok,available bool
 if json.Unmarshal(raw["type"],&typ)!=nil||
  json.Unmarshal(raw["request_id"],&receivedID)!=nil||
  json.Unmarshal(raw["content"],&content)!=nil||
  json.Unmarshal(raw["sha256"],&hash)!=nil||
  json.Unmarshal(raw["ok"],&ok)!=nil||
  json.Unmarshal(raw["remote_control_available"],&available)!=nil||
  typ!=expectedType||receivedID!=id||!ok||available||
  len(content)>RemoteGitDiffMaxBytes||!utf8.ValidString(content){
  ch.Close();return "",ErrInvalidRemoteAction
 }
 sum:=sha256.Sum256([]byte(content))
 if hash!=hex.EncodeToString(sum[:]){ch.Close();return "",ErrInvalidRemoteAction}
 return content,nil
}
