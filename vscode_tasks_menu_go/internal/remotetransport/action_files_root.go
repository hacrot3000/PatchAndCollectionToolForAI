package remotetransport

import "encoding/json"

const RemoteFilesRootProtocol="taskdeck-e2ee-files-root-v1"
const RemoteActionFilesRootNext="files.root.next"
const MaxRemoteFilesRootItems=8
type RemoteFilesRoot struct {Items []string; More bool}
type remoteFilesRootWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Items []string `json:"items"`
 More bool `json:"more"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func ValidRemoteFilesRootItem(v string)bool{
 if len(v)<3||len(v)>34||(v[:2]!="F:"&&v[:2]!="D:"){return false}
 for i:=2;i<len(v);i++{
  c:=v[i]
  if i==2&& !((c>='a'&&c<='z')||(c>='A'&&c<='Z')||(c>='0'&&c<='9')){return false}
  if !((c>='a'&&c<='z')||(c>='A'&&c<='Z')||(c>='0'&&c<='9')||c=='-'||c=='_'||c=='.'){return false}
 }
 return true
}
func ValidRemoteFilesRoot(v RemoteFilesRoot)bool{
 if len(v.Items)>MaxRemoteFilesRootItems{return false}
 previous:=""
 for _,item:=range v.Items{
  if !ValidRemoteFilesRootItem(item)||item[2:]<=previous{return false}
  previous=item[2:]
 }
 return true
}
func SealRemoteFilesRoot(ch *Channel,id string,v RemoteFilesRoot)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!ValidRemoteFilesRoot(v){return nil,ErrInvalidRemoteAction}
 if v.Items==nil{v.Items=[]string{}}
 raw,err:=json.Marshal(remoteFilesRootWire{Type:RemoteFilesRootProtocol,RequestID:id,Items:v.Items,
  More:v.More,RemoteControlAvailable:false})
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteFilesRoot(ch *Channel,frame []byte,id string)(RemoteFilesRoot,error){
 var denied RemoteFilesRoot
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","items","more","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"||
  (string(fields["more"])!="true"&&string(fields["more"])!="false"){ch.Close();return denied,ErrInvalidRemoteAction}
 var typ,requestID string
 var value RemoteFilesRoot
 if json.Unmarshal(fields["type"],&typ)!=nil||json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["items"],&value.Items)!=nil||json.Unmarshal(fields["more"],&value.More)!=nil||
  typ!=RemoteFilesRootProtocol||requestID!=id||value.Items==nil||!ValidRemoteFilesRoot(value){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return value,nil
}
