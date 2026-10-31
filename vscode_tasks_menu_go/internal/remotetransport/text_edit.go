package remotetransport

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "strings"
 "unicode/utf8"
)

// Separate, host-opt-in 4KiB UTF-8 optimistic editor. It is NOT a file
// upload, terminal, generic HTTP body, patch engine or binary transfer.
const RemoteTextEditMaxBytes=4096
const RemoteTextEditMaxPlaintext=6144
const MaxEncryptedTextEditFrame=RemoteTextEditMaxPlaintext+9+16
const remoteTextEditResultType="taskdeck-e2ee-file-replace-text-result-v1"

func validRemoteTextEdit(content,sha string)bool{
 return len(content)<=RemoteTextEditMaxBytes&&utf8.ValidString(content)&&
  !strings.ContainsRune(content,'\x00')&&validFileWindowDigest(sha)
}

func SealRemoteTextEditResult(ch *Channel,id,sha string)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validFileWindowDigest(sha){return nil,ErrInvalidRemoteAction}
 clear,err:=json.Marshal(remoteFileReplaceResultWire{
  Type:remoteTextEditResultType,RequestID:id,OK:true,
  SHA256:sha,RemoteControlAvailable:false,
 })
 if err!=nil||len(clear)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(clear)
}
func OpenRemoteTextEditResult(ch *Channel,frame []byte,id string)(string,error){
 if ch==nil||!ch.Established()||!validActionID(id)||len(frame)<25||
  len(frame)>MaxEncryptedPingFrame{return "",ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return "",ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 raw,err:=decodeActionObject(clear,"type","request_id","ok","sha256","remote_control_available")
 if err!=nil{ch.Close();return "",ErrInvalidRemoteAction}
 var typ,gotID,sha string
 var ok,available bool
 if json.Unmarshal(raw["type"],&typ)!=nil||
  json.Unmarshal(raw["request_id"],&gotID)!=nil||
  json.Unmarshal(raw["sha256"],&sha)!=nil||
  json.Unmarshal(raw["ok"],&ok)!=nil||
  json.Unmarshal(raw["remote_control_available"],&available)!=nil||
  typ!=remoteTextEditResultType||gotID!=id||!ok||available||
  !validFileWindowDigest(sha) {
  ch.Close();return "",ErrInvalidRemoteAction
 }
 return sha,nil
}

// New content hash is an immutable SHA-256 of exactly what the host wrote.
func remoteEditedSHA(content []byte)string {
 sum:=sha256.Sum256(content)
 return hex.EncodeToString(sum[:])
}
