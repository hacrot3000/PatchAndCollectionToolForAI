package remotetransport

import (
 "bytes"
 "encoding/json"
 "unicode"
 "unicode/utf8"
)

const RemoteActionFilesReadStart="files.read.start"
const RemoteActionFilesReadNext="files.read.next"
const RemoteFilesReadPageProtocol="taskdeck-e2ee-files-read-page-v1"
const MaxRemoteFilesReadTextBytes=220
const MaxRemoteFilesReadSourceBytes=2048

// A fixed read-only plaintext page. The file name is supplied by the host
// response for display ONLY, never as a caller-selected E2EE request path.
type RemoteFilesReadPage struct {
 Page int
 Name string
 Text string
 More bool
}
type remoteFilesReadPageWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Page int `json:"page"`
 Name string `json:"name"`
 Text string `json:"text"`
 More bool `json:"more"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func ValidRemoteFilesReadText(value string)bool{
 if !utf8.ValidString(value){return false}
 for _,r:=range value{
  if r=='\n'||r=='\t'{continue}
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){
   return false
  }
 }
 return true
}
func ValidRemoteFilesReadPage(value RemoteFilesReadPage)bool{
 return value.Page>=1&&value.Page<=16&&
  ValidRemoteFilesRootItem("F:"+value.Name)&&
  len(value.Text)<=MaxRemoteFilesReadTextBytes&&
  ValidRemoteFilesReadText(value.Text)&&
  !(value.More&&len(value.Text)==0)&&
  !(value.Page>1&&len(value.Text)==0)
}
func marshalRemoteFilesReadPage(id string,value RemoteFilesReadPage)([]byte,error){
 var data bytes.Buffer
 encoder:=json.NewEncoder(&data)
 encoder.SetEscapeHTML(false)
 err:=encoder.Encode(remoteFilesReadPageWire{
  Type:RemoteFilesReadPageProtocol,RequestID:id,
  Page:value.Page,Name:value.Name,Text:value.Text,
  More:value.More,RemoteControlAvailable:false,
 })
 if err!=nil{return nil,ErrInvalidRemoteAction}
 return bytes.TrimSuffix(data.Bytes(),[]byte{'\n'}),nil
}
// Page selection never takes an offset or filename from PHP or the client.
// Validate the entire file BEFORE first disclosure; no partial misleading
// text is exposed when a later chunk has a forbidden control or binary byte.
func BoundRemoteFilesReadPage(
 id,name,whole string,offset,page int,
)(RemoteFilesReadPage,int,error){
 var denied RemoteFilesReadPage
 if !validActionID(id)||len(whole)>MaxRemoteFilesReadSourceBytes||
  !ValidRemoteFilesReadText(whole)||offset<0||offset>len(whole)||
  page<1||page>16||!ValidRemoteFilesRootItem("F:"+name)||
  (offset==len(whole)&&offset>0){
  return denied,0,ErrInvalidRemoteAction
 }
 if whole==""&&offset==0&&page==1{
  result:=RemoteFilesReadPage{Page:1,Name:name,Text:"",More:false}
  raw,err:=marshalRemoteFilesReadPage(id,result)
  if err==nil&&len(raw)<=maxRemoteActionPlaintext{return result,0,nil}
  return denied,0,ErrInvalidRemoteAction
 }
 upper:=offset+MaxRemoteFilesReadTextBytes
 if upper>len(whole){upper=len(whole)}
 for upper>offset&&!utf8.ValidString(whole[offset:upper]){upper--}
 for upper>offset{
  candidate:=RemoteFilesReadPage{
   Page:page,Name:name,Text:whole[offset:upper],More:upper<len(whole),
  }
  if ValidRemoteFilesReadPage(candidate){
   raw,err:=marshalRemoteFilesReadPage(id,candidate)
   if err==nil&&len(raw)<=maxRemoteActionPlaintext{
    return candidate,upper,nil
   }
  }
  _,width:=utf8.DecodeLastRuneInString(whole[offset:upper])
  if width<1{return denied,0,ErrInvalidRemoteAction}
  upper-=width
 }
 return denied,0,ErrInvalidRemoteAction
}
func SealRemoteFilesReadPage(ch *Channel,id string,value RemoteFilesReadPage)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !ValidRemoteFilesReadPage(value){return nil,ErrInvalidRemoteAction}
 raw,err:=marshalRemoteFilesReadPage(id,value)
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteFilesReadPage(ch *Channel,frame []byte,id string)(RemoteFilesReadPage,error){
 var denied RemoteFilesReadPage
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","page","name",
  "text","more","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"||
  (string(fields["more"])!="true"&&string(fields["more"])!="false"){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,requestID string
 var result RemoteFilesReadPage
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["page"],&result.Page)!=nil||
  json.Unmarshal(fields["name"],&result.Name)!=nil||
  json.Unmarshal(fields["text"],&result.Text)!=nil||
  json.Unmarshal(fields["more"],&result.More)!=nil||
  typ!=RemoteFilesReadPageProtocol||requestID!=id||
  !ValidRemoteFilesReadPage(result){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return result,nil
}
