package remotetransport

import (
 "bytes"
 "encoding/json"
 "unicode"
 "unicode/utf8"
)

const RemoteActionGitPatchStart="git.patch.start"
const RemoteActionGitPatchNext="git.patch.next"
const RemoteGitPatchPageProtocol="taskdeck-e2ee-git-patch-page-v1"
const MaxRemoteGitPatchPageBytes=320

// A read-only patch of already-tracked changes is paged by the host alone.
// PHP, browser and native client cannot supply path, revision or page number.
type RemoteGitPatchPage struct{
 Page int
 Patch string
 More bool
}

type remoteGitPatchPageWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Page int `json:"page"`
 Patch string `json:"patch"`
 More bool `json:"more"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func ValidRemoteGitPatchText(s string)bool{
 if s==""||len(s)>MaxRemoteGitPatchPageBytes||!utf8.ValidString(s){return false}
 for _,r:=range s{
  if r=='
'||r=='	'{continue}
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){
   return false
  }
 }
 return true
}
func marshalRemoteGitPatchPage(id string,v RemoteGitPatchPage)([]byte,error){
 var buf bytes.Buffer
 encoder:=json.NewEncoder(&buf)
 encoder.SetEscapeHTML(false)
 err:=encoder.Encode(remoteGitPatchPageWire{
  Type:RemoteGitPatchPageProtocol,RequestID:id,
  Page:v.Page,Patch:v.Patch,More:v.More,RemoteControlAvailable:false,
 })
 if err!=nil{return nil,ErrInvalidRemoteAction}
 return bytes.TrimSuffix(buf.Bytes(),[]byte{'
'}),nil
}
func ValidRemoteGitPatchPage(page RemoteGitPatchPage)bool{
 return page.Page>=1&&page.Page<=256&&ValidRemoteGitPatchText(page.Patch)
}
// BoundRemoteGitPatchPage selects the largest UTF-8 prefix that fits an
// unchanged 512-byte AEAD plaintext mailbox, without clipping a rune.
// Empty chunks and unsafe binary/terminal-control bytes fail closed.
func BoundRemoteGitPatchPage(id string,content string,offset,page int)(RemoteGitPatchPage,int,error){
 var denied RemoteGitPatchPage
 if !validActionID(id)||!utf8.ValidString(content)||
  offset<0||offset>=len(content)||page<1||page>256{
  return denied,0,ErrInvalidRemoteAction
 }
 upper:=offset+MaxRemoteGitPatchPageBytes
 if upper>len(content){upper=len(content)}
 for upper>offset&&!utf8.ValidString(content[offset:upper]){
  upper--
 }
 // Validate ALL data before sending any page. A control character on a
 // later page must not cause partial disclosure of a misleading diff.
 for _,r:=range content{
  if r=='
'||r=='	'{continue}
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){
   return denied,0,ErrInvalidRemoteAction
  }
 }
 for upper>offset{
  candidate:=RemoteGitPatchPage{
   Page:page,Patch:content[offset:upper],More:upper<len(content),
  }
  if ValidRemoteGitPatchPage(candidate){
   clear,err:=marshalRemoteGitPatchPage(id,candidate)
   if err==nil&&len(clear)<=maxRemoteActionPlaintext{
    return candidate,upper,nil
   }
  }
  _,n:=utf8.DecodeLastRuneInString(content[offset:upper])
  if n<1{return denied,0,ErrInvalidRemoteAction}
  upper-=n
 }
 return denied,0,ErrInvalidRemoteAction
}
func SealRemoteGitPatchPage(ch *Channel,id string,v RemoteGitPatchPage)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!ValidRemoteGitPatchPage(v){
  return nil,ErrInvalidRemoteAction
 }
 clear,err:=marshalRemoteGitPatchPage(id,v)
 if err!=nil||len(clear)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(clear)
}
func OpenRemoteGitPatchPage(ch *Channel,frame []byte,id string)(RemoteGitPatchPage,error){
 var denied RemoteGitPatchPage
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","page","patch",
  "more","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"||
  (string(fields["more"])!="true"&&string(fields["more"])!="false"){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,requestID string
 var page RemoteGitPatchPage
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["page"],&page.Page)!=nil||
  json.Unmarshal(fields["patch"],&page.Patch)!=nil||
  json.Unmarshal(fields["more"],&page.More)!=nil||
  typ!=RemoteGitPatchPageProtocol||requestID!=id||
  !ValidRemoteGitPatchPage(page){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return page,nil
}
