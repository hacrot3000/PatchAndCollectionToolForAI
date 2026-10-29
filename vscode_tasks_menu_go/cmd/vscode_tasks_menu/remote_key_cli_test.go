package main

import (
 "crypto/ed25519"
 "crypto/rand"
 "encoding/base64"
 "os"
 "path/filepath"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
)

func TestHostRemoteKeyCLIAddListRevokeAndDenyInvalidKey(t *testing.T){
 home:=t.TempDir()
 t.Setenv("XDG_CONFIG_HOME",home)
 workspace:=filepath.Join(t.TempDir(),"project")
 if err:=os.MkdirAll(filepath.Join(workspace,".vscode"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(workspace,".vscode","tasks.json"),[]byte("{}"),0600);err!=nil{t.Fatal(err)}
 args:=[]string{"--workspace",workspace}
 if err:=runHostRemoteKeyCLI(append(append([]string{},args...),"list"));err!=nil{t.Fatal(err)}
 pub,_,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 public:=base64.StdEncoding.EncodeToString(pub)
 path,err:=remoteclientkey.DefaultPath(workspace)
 if err!=nil{t.Fatal(err)}
 if err:=runHostRemoteKeyCLI(append(append([]string{},args...),"add","alice",public,"laptop"));err!=nil{t.Fatal(err)}
 entries,err:=remoteclientkey.List(path)
 if err!=nil||len(entries)!=1||entries[0].Username!="alice"{t.Fatalf("host CLI add failed: %v %+v",err,entries)}
 if err:=runHostRemoteKeyCLI(append(append([]string{},args...),"add","bob",public));err==nil{t.Fatal("P0: second user claimed same host-registered key")}
 if err:=runHostRemoteKeyCLI(append(append([]string{},args...),"revoke",entries[0].Fingerprint));err!=nil{t.Fatal(err)}
 remaining,err:=remoteclientkey.List(path)
 if err!=nil||len(remaining)!=0{t.Fatalf("revocation not persistent: %v %+v",err,remaining)}
 if err:=runHostRemoteKeyCLI(append(append([]string{},args...),"add","alice","not a public key"));err==nil{t.Fatal("invalid public key approved")}
}
