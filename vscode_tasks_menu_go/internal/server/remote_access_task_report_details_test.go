package server

import (
 "fmt"
 "strings"
 "testing"
)

func TestRemoteTaskFinishAuditDetailsStrictSchema(t *testing.T){
 valid:=`{"transport":"e2ee","task_id":123,"exit_code":-1,"duration_ms":1000}`
 parsed,ok:=decodeRemoteTaskFinishAuditDetails(valid)
 if !ok||parsed.TaskID!=123||parsed.ExitCode!= -1||parsed.DurationMS!=1000{
  t.Fatalf("valid host audit row refused: %+v %t",parsed,ok)
 }
 // Field order must not matter; serialization of future audit backends
 // may change while maintaining the exact authenticated schema.
 reordered:=`{"duration_ms":0,"exit_code":0,"task_id":12,"transport":"e2ee"}`
 parsed,ok=decodeRemoteTaskFinishAuditDetails(reordered)
 if !ok||parsed.TaskID!=12||parsed.ExitCode!=0||parsed.DurationMS!=0{
  t.Fatalf("valid reordered audit refused: %+v %t",parsed,ok)
 }
 invalid:=[]string{
  `{}`,`[]`,`null`,`{"transport":"e2ee"}`,
  `{"transport":"e2ee","task_id":1,"exit_code":0}`,
  `{"transport":"e2ee","task_id":1,"exit_code":0,"duration_ms":1,"command":"/bin/sh"}`,
  `{"transport":"e2ee","task_id":1,"task_id":2,"exit_code":0,"duration_ms":1}`,
  `{"transport":"e2ee","task_id":1,"exit_code":0,"exit_code":1,"duration_ms":1}`,
  `{"transport":"e2ee","task_id":1,"exit_code":0,"duration_ms":1,"duration_ms":2}`,
  `{"transport":"E2EE","task_id":1,"exit_code":0,"duration_ms":1}`,
  `{"transport":null,"task_id":1,"exit_code":0,"duration_ms":1}`,
  `{"transport":"e2ee","task_id":null,"exit_code":0,"duration_ms":1}`,
  `{"transport":"e2ee","task_id":"1","exit_code":0,"duration_ms":1}`,
  `{"transport":"e2ee","task_id":1.0,"exit_code":0,"duration_ms":1}`,
  `{"transport":"e2ee","task_id":1e1,"exit_code":0,"duration_ms":1}`,
  `{"transport":"e2ee","task_id":0,"exit_code":0,"duration_ms":1}`,
  `{"transport":"e2ee","task_id":2147483648,"exit_code":0,"duration_ms":1}`,
  `{"transport":"e2ee","task_id":1,"exit_code":-2,"duration_ms":1}`,
  `{"transport":"e2ee","task_id":1,"exit_code":256,"duration_ms":1}`,
  `{"transport":"e2ee","task_id":1,"exit_code":0,"duration_ms":-1}`,
  `{"transport":"e2ee","task_id":1,"exit_code":0,"duration_ms":86400001}`,
  valid+"true",valid+valid,strings.Repeat(" ",257),
 }
 for i,row:=range invalid{
  if result,ok:=decodeRemoteTaskFinishAuditDetails(row);ok{
   t.Fatalf("invalid audit row %d admitted: %+v (%s)",i,result,row)
  }
 }
 // Repeated JSON property keys must never override the host-scoped
 // task ID or invent extra result metadata inside local audit storage.
 for _,key:=range []string{"task_id","exit_code","duration_ms","transport"}{
  extra:=fmt.Sprintf(`,"%s":0`,key)
  if key=="transport"{extra=`,"transport":"e2ee"`}
  duplicated:=strings.TrimSuffix(valid,"}")+extra+"}"
  if _,ok:=decodeRemoteTaskFinishAuditDetails(duplicated);ok{
   t.Fatalf("duplicate %s admitted",key)
  }
 }
}
