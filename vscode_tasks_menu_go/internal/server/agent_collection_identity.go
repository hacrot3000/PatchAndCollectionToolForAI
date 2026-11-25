package server

import (
 "encoding/json"
 "errors"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

type agentCollectionIdentityError struct{ Kind string }
func(e *agentCollectionIdentityError) Error()string{
 switch e.Kind{
 case "missing":return "missing project identity: project.key and project.id are required for standalone text COLLECT"
 case "foreign":return "project identity does not match this TaskDeck workspace (both key and id must match)"
 default:return "invalid project identity: project must be an object containing non-empty key and id"
 }
}
func agentCollectionIdentity(raw json.RawMessage,identity patchtool.ProjectIdentity,required bool)error{
 if len(raw)==0||string(raw)=="null"{
  if required{return &agentCollectionIdentityError{Kind:"missing"}}
  return nil
 }
 var obj map[string]json.RawMessage
 if json.Unmarshal(raw,&obj)!=nil||obj==nil{return &agentCollectionIdentityError{Kind:"malformed"}}
 var key,id string
 keyErr:=json.Unmarshal(obj["key"],&key)
 idErr:=json.Unmarshal(obj["id"],&id)
 // A partially supplied object containing a conflicting identity is still
 // foreign. Never silently bind it to the active workspace.
 if (keyErr==nil&&key!=""&&key!=identity.Key)||(idErr==nil&&id!=""&&id!=identity.ID){
  return &agentCollectionIdentityError{Kind:"foreign"}
 }
 if keyErr!=nil||idErr!=nil||strings.TrimSpace(key)==""||strings.TrimSpace(id)==""||len(obj)!=2{
  return &agentCollectionIdentityError{Kind:"malformed"}
 }
 return ensurePastedIdentity(raw,identity)
}
func agentCollectionIdentityKind(err error)string{
 var e *agentCollectionIdentityError
 if errors.As(err,&e){return e.Kind}
 return ""
}
