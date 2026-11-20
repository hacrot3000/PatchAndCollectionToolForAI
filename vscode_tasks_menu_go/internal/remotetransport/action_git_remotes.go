package remotetransport

import(
 "encoding/json"
 "strings"
)

// Port the main branch's offline Git remote name listing using a separate
// authenticated response envelope. No remote URLs, credentials or argv.
const RemoteGitRemotesResultProtocol="taskdeck-e2ee-git-remotes-result-v1"
const MaxRemoteGitRemotesNamesBytes=320

type remoteGitRemotesWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Remotes string `json:"remotes"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

// Reject the WHOLE response if it is truncated, unsafe, duplicated or
// unsorted. Host and client independently apply the same report rules.
func ValidRemoteGitRemotesReport(value string)bool{
 const prefix="GIT_REMOTES_V1\n"
 if len(value)>MaxRemoteGitRemotesNamesBytes||!strings.HasPrefix(value,prefix){return false}
 rest:=strings.TrimPrefix(value,prefix)
 if rest==""{return true}
 if !strings.HasSuffix(rest,"\n"){return false}
 names:=strings.Split(strings.TrimSuffix(rest,"\n"),"\n")
 if len(names)>16{return false}
 previous:=""
 for _,name:=range names{
  if !ValidRemoteGitRemoteName(name)||(previous!=""&&previous>=name){return false}
  previous=name
 }
 return true
}

// The same conservative ASCII subset was used by main's original read-only
// remote metadata operation. A malicious config cannot smuggle URLs/paths.
func ValidRemoteGitRemoteName(name string)bool{
 if len(name)<1||len(name)>64{return false}
 for i:=0;i<len(name);i++{
  b:=name[i]
  ascii:=(b>='A'&&b<='Z')||(b>='a'&&b<='z')||(b>='0'&&b<='9')
  if ascii{continue}
  if i==0||i==len(name)-1||(b!='-'&&b!='_'&&b!='.'){return false}
 }
 return !strings.Contains(name,"..")
}
func SealRemoteGitRemotesResult(ch *Channel,id,remotes string)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!ValidRemoteGitRemotesReport(remotes){
  return nil,ErrInvalidRemoteAction
 }
 raw,err:=json.Marshal(remoteGitRemotesWire{
  Type:RemoteGitRemotesResultProtocol,RequestID:id,
  Remotes:remotes,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteGitRemotesResult(ch *Channel,frame []byte,id string)(string,error){
 if ch==nil||!ch.Established()||!validActionID(id){return "",ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return "",ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","remotes","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return "",ErrInvalidRemoteAction
 }
 var typ,got,remotes string
 if json.Unmarshal(fields["type"],&typ)!=nil||
    json.Unmarshal(fields["request_id"],&got)!=nil||
    json.Unmarshal(fields["remotes"],&remotes)!=nil||
    typ!=RemoteGitRemotesResultProtocol||got!=id||
    !ValidRemoteGitRemotesReport(remotes){
  ch.Close();return "",ErrInvalidRemoteAction
 }
 return remotes,nil
}
