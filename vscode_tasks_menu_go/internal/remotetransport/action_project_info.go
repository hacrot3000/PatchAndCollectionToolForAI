package remotetransport

import (
 "encoding/json"
 "unicode"
 "unicode/utf8"
)

const RemoteProjectInfoResultProtocol = "taskdeck-e2ee-project-info-result-v1"

// Project metadata only. No raw host absolute path, task content, arbitrary
// server HTTP resource, filename, secret, credential or executable code.
type RemoteProjectInfo struct {
 ProjectName string
 WorkspaceName string
 HostOS string
}

type remoteProjectInfoResultWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 ProjectName string `json:"project_name"`
 WorkspaceName string `json:"workspace_name"`
 HostOS string `json:"host_os"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func validRemoteProjectInfoText(s string)bool{
 if s=="" || len(s)>96 || !utf8.ValidString(s){return false}
 for _,r:=range s{
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){
   return false
  }
 }
 return true
}

func validRemoteProjectInfo(info RemoteProjectInfo)bool{
 if !validRemoteProjectInfoText(info.ProjectName) ||
  !validRemoteProjectInfoText(info.WorkspaceName){return false}
 return info.HostOS=="linux" || info.HostOS=="darwin" || info.HostOS=="windows"
}

func SealRemoteActionProjectInfoResult(
 ch *Channel,id string,info RemoteProjectInfo,
)([]byte,error){
 if ch==nil || !ch.Established() || !validActionID(id) ||
  !validRemoteProjectInfo(info) {
  return nil,ErrInvalidRemoteAction
 }
 clear,err:=json.Marshal(remoteProjectInfoResultWire{
  Type:RemoteProjectInfoResultProtocol,RequestID:id,
  ProjectName:info.ProjectName,WorkspaceName:info.WorkspaceName,
  HostOS:info.HostOS,RemoteControlAvailable:false,
 })
 if err!=nil || len(clear)>maxRemoteActionPlaintext{
  return nil,ErrInvalidRemoteAction
 }
 return ch.Seal(clear)
}

func OpenRemoteActionProjectInfoResult(
 ch *Channel,frame []byte,wantID string,
)(RemoteProjectInfo,error){
 denied:=RemoteProjectInfo{}
 if ch==nil || !ch.Established() || !validActionID(wantID){
  return denied,ErrInvalidRemoteAction
 }
 clear,err:=ch.Open(frame)
 if err!=nil {return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 fields,err:=decodeActionObject(clear,"type","request_id","project_name",
  "workspace_name","host_os","remote_control_available")
 if err!=nil {
  ch.Close()
  return denied,ErrInvalidRemoteAction
 }
 var typ,id string
 var available bool
 var result RemoteProjectInfo
 if json.Unmarshal(fields["type"],&typ)!=nil ||
  json.Unmarshal(fields["request_id"],&id)!=nil ||
  json.Unmarshal(fields["project_name"],&result.ProjectName)!=nil ||
  json.Unmarshal(fields["workspace_name"],&result.WorkspaceName)!=nil ||
  json.Unmarshal(fields["host_os"],&result.HostOS)!=nil ||
  json.Unmarshal(fields["remote_control_available"],&available)!=nil ||
  typ!=RemoteProjectInfoResultProtocol || id!=wantID || available ||
  !validRemoteProjectInfo(result){
  ch.Close()
  return denied,ErrInvalidRemoteAction
 }
 return result,nil
}
