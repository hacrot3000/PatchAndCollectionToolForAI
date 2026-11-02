package remotetransport

import "encoding/json"

// An authenticated insertion SHA is scoped to one large-file line insertion.
// It is not interchangeable with the full-file replacement result type.
const remoteLargeLineInsertResultType="taskdeck-e2ee-large-line-insert-result-v1"

func SealRemoteLargeLineInsertResult(ch *Channel,id,sha string)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteReplacement("",sha){return nil,ErrInvalidRemoteAction}
 clear,err:=json.Marshal(remoteFileReplaceResultWire{
  Type:remoteLargeLineInsertResultType,RequestID:id,
  OK:true,SHA256:sha,RemoteControlAvailable:false,
 })
 if err!=nil||len(clear)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(clear)
}
func OpenRemoteLargeLineInsertResult(ch *Channel,frame []byte,id string)(string,error){
 if ch==nil||!ch.Established()||!validActionID(id){return "",ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return "",ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 raw,err:=decodeActionObject(clear,
  "type","request_id","ok","sha256","remote_control_available")
 if err!=nil{ch.Close();return "",ErrInvalidRemoteAction}
 var typ,gotID,sha string
 var ok,available bool
 if json.Unmarshal(raw["type"],&typ)!=nil||
  json.Unmarshal(raw["request_id"],&gotID)!=nil||
  json.Unmarshal(raw["sha256"],&sha)!=nil||
  json.Unmarshal(raw["ok"],&ok)!=nil||
  json.Unmarshal(raw["remote_control_available"],&available)!=nil||
  typ!=remoteLargeLineInsertResultType||gotID!=id||!ok||available||
  !validRemoteReplacement("",sha){
  ch.Close();return "",ErrInvalidRemoteAction
 }
 return sha,nil
}
