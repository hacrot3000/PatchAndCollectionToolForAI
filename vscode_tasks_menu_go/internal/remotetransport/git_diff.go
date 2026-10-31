package remotetransport

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "unicode/utf8"
)

// The E2EE Git diff has a distinct authenticated result shape. The Relay
// carries only opaque AEAD ciphertext; no shell, arbitrary Git flag, token
// or repository path is made visible to the compromised PHP Portal.
const RemoteGitDiffMaxBytes=4096
const RemoteGitDiffMaxPlaintext=6144
const MaxEncryptedGitDiffFrame=RemoteGitDiffMaxPlaintext+9+16
const remoteGitDiffResultType="taskdeck-e2ee-git-diff-result-v1"

func SealRemoteGitDiffResult(ch *Channel,id string,content []byte)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  len(content)>RemoteGitDiffMaxBytes||!utf8.Valid(content){
  return nil,ErrInvalidRemoteAction
 }
 sum:=sha256.Sum256(content)
 clear,err:=json.Marshal(remoteFileReadResultWire{
  Type:remoteGitDiffResultType,RequestID:id,OK:true,
  RemoteControlAvailable:false,Content:string(content),
  SHA256:hex.EncodeToString(sum[:]),
 })
 if err!=nil||len(clear)>RemoteGitDiffMaxPlaintext{return nil,ErrInvalidRemoteAction}
 frame,err:=ch.Seal(clear)
 if err!=nil||len(frame)>MaxEncryptedGitDiffFrame{return nil,ErrInvalidRemoteAction}
 return frame,nil
}

func OpenRemoteGitDiffResult(ch *Channel,frame []byte,id string)(string,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
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
  typ!=remoteGitDiffResultType||receivedID!=id||!ok||available||
  len(content)>RemoteGitDiffMaxBytes||!utf8.ValidString(content){
  ch.Close();return "",ErrInvalidRemoteAction
 }
 sum:=sha256.Sum256([]byte(content))
 if hash!=hex.EncodeToString(sum[:]){ch.Close();return "",ErrInvalidRemoteAction}
 return content,nil
}
