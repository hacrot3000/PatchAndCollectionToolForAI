package remotetransport

import (
 "encoding/json"
 "strings"
 "unicode"
 "unicode/utf8"
)

const RemoteActionGitCommit="git.commit"
const RemoteGitCommitResultProtocol="taskdeck-e2ee-git-commit-result-v1"
const MaxRemoteGitCommitMessageBytes=120

// Only a short, single-line UTF-8 commit subject can cross E2EE. This is
// never a shell command, argv option, repository path or Git configuration.
func ValidRemoteGitCommitMessage(message string)bool{
 if len(message)<1||len(message)>MaxRemoteGitCommitMessageBytes||
  !utf8.ValidString(message)||strings.TrimSpace(message)!=message{return false}
 for _,r:=range message{
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){
   return false
  }
 }
 return true
}

type RemoteGitCommitResult struct{
 Before string
 After string
}

type remoteGitCommitResultWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Before string `json:"before"`
 After string `json:"after"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func validRemoteGitCommitResult(value RemoteGitCommitResult)bool{
 return gitPullObjectID.MatchString(value.Before)&&
  gitPullObjectID.MatchString(value.After)&&value.Before!=value.After
}

func SealRemoteGitCommitResult(ch *Channel,id string,value RemoteGitCommitResult)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteGitCommitResult(value){return nil,ErrInvalidRemoteAction}
 raw,err:=json.Marshal(remoteGitCommitResultWire{
  Type:RemoteGitCommitResultProtocol,RequestID:id,
  Before:value.Before,After:value.After,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}

func OpenRemoteGitCommitResult(ch *Channel,frame []byte,id string)(RemoteGitCommitResult,error){
 var denied RemoteGitCommitResult
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","before","after",
  "remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,requestID string
 var result RemoteGitCommitResult
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["before"],&result.Before)!=nil||
  json.Unmarshal(fields["after"],&result.After)!=nil||
  typ!=RemoteGitCommitResultProtocol||requestID!=id||
  !validRemoteGitCommitResult(result){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return result,nil
}
