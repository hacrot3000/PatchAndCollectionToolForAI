package remotetransport

import (
 "encoding/json"
 "strings"
)

// Inventory of local fast-forward paths only; never an authorization to Pull.
const RemoteActionGitPullChanges="git.pull_changes"
const RemoteGitPullChangesProtocol="taskdeck-e2ee-git-pull-changes-v1"
const MaxRemoteGitPullChangedFiles=6

type RemoteGitPullChanges struct{
 Head string
 UpstreamHead string
 Files []string // A:path, M:path or D:path; no Git arguments or URLs
}
type remoteGitPullChangesWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Head string `json:"head"`
 UpstreamHead string `json:"upstream_head"`
 Files []string `json:"files"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func ValidRemoteGitPullChangedFile(value string)bool{
 if len(value)<4||len(value)>50||
  (value[:2]!="A:"&&value[:2]!="M:"&&value[:2]!="D:"){return false}
 parts:=strings.Split(value[2:],"/")
 if len(parts)>4{return false}
 for _,part:=range parts{
  if len(part)<1||len(part)>48{return false}
  for i:=0;i<len(part);i++{
   ch:=part[i]
   alpha:=(ch>='A'&&ch<='Z')||(ch>='a'&&ch<='z')
   digit:=ch>='0'&&ch<='9'
   if i==0&&!alpha&&!digit{return false}
   if !alpha&&!digit&&ch!='.'&&ch!='_'&&ch!='-'{return false}
  }
 }
 return true
}
func ValidRemoteGitPullChanges(value RemoteGitPullChanges)bool{
 if !remoteGitPullReviewOID.MatchString(value.Head)||
  !remoteGitPullReviewOID.MatchString(value.UpstreamHead)||
  len(value.Files)>MaxRemoteGitPullChangedFiles||
  (value.Head==value.UpstreamHead&&len(value.Files)!=0){return false}
 previous:=""
 for _,entry:=range value.Files{
  if !ValidRemoteGitPullChangedFile(entry)||entry[2:]<=previous{return false}
  previous=entry[2:]
 }
 return true
}
func SealRemoteGitPullChanges(ch *Channel,id string,value RemoteGitPullChanges)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!ValidRemoteGitPullChanges(value){
  return nil,ErrInvalidRemoteAction
 }
 if value.Files==nil{value.Files=[]string{}}
 data,err:=json.Marshal(remoteGitPullChangesWire{
  Type:RemoteGitPullChangesProtocol,RequestID:id,
  Head:value.Head,UpstreamHead:value.UpstreamHead,Files:value.Files,
  RemoteControlAvailable:false,
 })
 if err!=nil||len(data)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(data)
}
func OpenRemoteGitPullChanges(ch *Channel,frame []byte,id string)(RemoteGitPullChanges,error){
 var denied RemoteGitPullChanges
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 fields,err:=decodeActionObject(clear,"type","request_id","head","upstream_head","files",
  "remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,requestID string
 var value RemoteGitPullChanges
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["head"],&value.Head)!=nil||
  json.Unmarshal(fields["upstream_head"],&value.UpstreamHead)!=nil||
  json.Unmarshal(fields["files"],&value.Files)!=nil||
  typ!=RemoteGitPullChangesProtocol||requestID!=id||value.Files==nil||
  !ValidRemoteGitPullChanges(value){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return value,nil
}
