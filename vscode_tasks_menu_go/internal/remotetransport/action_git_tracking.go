package remotetransport

import("encoding/json";"strings")

const RemoteActionGitTrackingStart="git.tracking.start"
const RemoteActionGitTrackingNext="git.tracking.next"
const RemoteGitTrackingPageProtocol="taskdeck-e2ee-git-tracking-page-v1"
const MaxRemoteGitTrackingSnapshotBytes=4096
const MaxRemoteGitTrackingPageBytes=360
const MaxRemoteGitTrackingRefs=32

type RemoteGitTrackingPage struct{ Page int; Refs string; More bool }
type remoteGitTrackingPageWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Page int `json:"page"`
 Refs string `json:"refs"`
 More bool `json:"more"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func validGitTrackingLine(row string)(string,bool){
 i:=strings.LastIndexByte(row,' ')
 if i<1||i>=len(row)-1||strings.Contains(row[:i]," "){return "",false}
 name,oid:=row[:i],row[i+1:]
 if len(name)>160||strings.HasSuffix(name,"/HEAD")||(len(oid)!=40&&len(oid)!=64){return "",false}
 parts:=strings.Split(name,"/")
 if len(parts)<2||len(parts)>7{return "",false}
 for _,part:=range parts{
  if !ValidRemoteGitRemoteName(part)||strings.HasSuffix(part,".lock"){return "",false}
 }
 for i:=0;i<len(oid);i++{
  c:=oid[i]
  if (c<'0'||c>'9')&&(c<'a'||c>'f'){return "",false}
 }
 return name,true
}
func ValidRemoteGitTrackingSnapshot(value string)bool{
 const header="GIT_TRACKING_V1\n"
 if len(value)>MaxRemoteGitTrackingSnapshotBytes||!strings.HasPrefix(value,header){return false}
 tail:=strings.TrimPrefix(value,header)
 if tail==""{return true}
 if !strings.HasSuffix(tail,"\n"){return false}
 lines:=strings.Split(strings.TrimSuffix(tail,"\n"),"\n")
 if len(lines)>MaxRemoteGitTrackingRefs{return false}
 last:=""
 for _,line:=range lines{
  name,ok:=validGitTrackingLine(line)
  if !ok||(last!=""&&name<=last){return false}
  last=name
 }
 return true
}
func ValidRemoteGitTrackingPage(value RemoteGitTrackingPage)bool{
 if value.Page<1||value.Page>MaxRemoteGitTrackingRefs||
  len(value.Refs)>MaxRemoteGitTrackingPageBytes{return false}
 const prefix="GIT_TRACKING_V1\n"
 if value.Page==1{
  if !strings.HasPrefix(value.Refs,prefix){return false}
  if value.Refs==prefix{return !value.More}
  return ValidRemoteGitTrackingSnapshot(value.Refs)
 }
 if value.Refs==""||strings.HasPrefix(value.Refs,prefix){return false}
 return ValidRemoteGitTrackingSnapshot(prefix+value.Refs)
}
func marshalGitTrackingPage(id string,v RemoteGitTrackingPage)([]byte,error){
 b,err:=json.Marshal(remoteGitTrackingPageWire{
  Type:RemoteGitTrackingPageProtocol,RequestID:id,
  Page:v.Page,Refs:v.Refs,More:v.More,RemoteControlAvailable:false,
 })
 if err!=nil||len(b)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return b,nil
}
func BoundRemoteGitTrackingPage(id,all string,offset,page int)(RemoteGitTrackingPage,int,error){
 var zero RemoteGitTrackingPage
 if !validActionID(id)||!ValidRemoteGitTrackingSnapshot(all)||page<1||
  page>MaxRemoteGitTrackingRefs||offset<0||offset>=len(all)||
  (page==1)!=(offset==0)||(offset>0&&all[offset-1]!='\n'){
  return zero,0,ErrInvalidRemoteAction
 }
 upper:=offset+MaxRemoteGitTrackingPageBytes
 if upper>len(all){upper=len(all)}
 for end:=upper;end>offset;end--{
  if all[end-1]!='\n'{continue}
  v:=RemoteGitTrackingPage{Page:page,Refs:all[offset:end],More:end<len(all)}
  if !ValidRemoteGitTrackingPage(v){continue}
  if _,err:=marshalGitTrackingPage(id,v);err==nil{return v,end,nil}
 }
 return zero,0,ErrInvalidRemoteAction
}
func SealRemoteGitTrackingPage(ch *Channel,id string,v RemoteGitTrackingPage)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!ValidRemoteGitTrackingPage(v){
  return nil,ErrInvalidRemoteAction
 }
 raw,err:=marshalGitTrackingPage(id,v)
 if err!=nil{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteGitTrackingPage(ch *Channel,frame []byte,id string)(RemoteGitTrackingPage,error){
 var zero RemoteGitTrackingPage
 if ch==nil||!ch.Established()||!validActionID(id){return zero,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return zero,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","page","refs","more","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"||
  (string(fields["more"])!="true"&&string(fields["more"])!="false"){
  ch.Close();return zero,ErrInvalidRemoteAction
 }
 var typ,req string
 var v RemoteGitTrackingPage
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&req)!=nil||
  json.Unmarshal(fields["page"],&v.Page)!=nil||
  json.Unmarshal(fields["refs"],&v.Refs)!=nil||
  json.Unmarshal(fields["more"],&v.More)!=nil||
  typ!=RemoteGitTrackingPageProtocol||req!=id||!ValidRemoteGitTrackingPage(v){
  ch.Close();return zero,ErrInvalidRemoteAction
 }
 return v,nil
}
