package remotetransport

import (
 "encoding/json"
 "unicode"
 "unicode/utf8"
)

const RemoteActionGitLog="git.log"
const RemoteGitLogResultProtocol="taskdeck-e2ee-git-log-result-v1"
const MaxRemoteGitLogText=240

type remoteGitLogWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Log string `json:"log"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func ValidRemoteGitLogText(s string)bool{
 if len(s)<1||len(s)>MaxRemoteGitLogText||!utf8.ValidString(s){return false}
 for _,r:=range s{
  if r=='\n'{continue}
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){
   return false
  }
 }
 return true
}
func SealRemoteGitLogResult(ch *Channel,id,log string)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !ValidRemoteGitLogText(log){return nil,ErrInvalidRemoteAction}
 raw,err:=json.Marshal(remoteGitLogWire{
  Type:RemoteGitLogResultProtocol,RequestID:id,
  Log:log,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteGitLogResult(ch *Channel,frame []byte,id string)(string,error){
 if ch==nil||!ch.Established()||!validActionID(id){return "",ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return "",ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","log",
  "remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return "",ErrInvalidRemoteAction
 }
 var typ,requestID,log string
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["log"],&log)!=nil||
  typ!=RemoteGitLogResultProtocol||requestID!=id||
  !ValidRemoteGitLogText(log){
  ch.Close();return "",ErrInvalidRemoteAction
 }
 return log,nil
}
