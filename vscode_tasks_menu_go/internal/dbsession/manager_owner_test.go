package dbsession

import (
 "encoding/json"
 "strings"
 "testing"
)

func TestDatabaseSessionOwnerImmutableAndNotSerialized(t *testing.T) {
 manager:=&Manager{sessions:map[string]*session{
  "connection-1":{meta:Metadata{ID:"connection-1",ProfileID:"profile-1"}},
 }}
 if err:=manager.SetOwner("connection-1","project-a","user-a");err!=nil{t.Fatal(err)}
 if err:=manager.SetOwner("connection-1","project-a","user-a");err!=nil{t.Fatalf("idempotent owner assignment: %v",err)}
 for _,tc:=range []struct{project,user string}{
  {"project-a","user-b"},{"project-b","user-a"},{"","user-a"},{"project-a",""},
 } {
  if err:=manager.SetOwner("connection-1",tc.project,tc.user);err==nil{
   t.Fatalf("session ownership changed to %q/%q",tc.project,tc.user)
  }
 }
 metadata,err:=manager.Get("connection-1")
 if err!=nil{t.Fatal(err)}
 if metadata.OwnerProjectID!="project-a"||metadata.OwnerUserID!="user-a"{
  t.Fatalf("unexpected owner metadata %+v",metadata)
 }
 list:=manager.List()
 if len(list)!=1||list[0].OwnerUserID!="user-a"{t.Fatalf("lost list ownership %+v",list)}
 encoded,err:=json.Marshal(metadata)
 if err!=nil{t.Fatal(err)}
 if strings.Contains(string(encoded),"user-a")||strings.Contains(string(encoded),"project-a")||
  strings.Contains(string(encoded),"OwnerUserID") {
  t.Fatalf("session owner leaked into browser JSON: %s",encoded)
 }
 if err:=manager.SetOwner("missing","project-a","user-a");err!=ErrSessionNotFound{
  t.Fatalf("missing database session: %v",err)
 }
}
