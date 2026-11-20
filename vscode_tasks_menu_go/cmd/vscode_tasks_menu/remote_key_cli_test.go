package main

import (
 "crypto/ed25519"
 "context"
 "fmt"
 "io"
 "time"
 "bletonfc/vscode_tasks_menu/internal/identity"
 "crypto/rand"
 "encoding/base64"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
 "bletonfc/vscode_tasks_menu/internal/remoteportal"
)

func TestHostRemoteKeyCLIAddListRevokeAndDenyInvalidKey(t *testing.T){
 home:=t.TempDir()
 t.Setenv("XDG_CONFIG_HOME",home)
 workspace:=filepath.Join(t.TempDir(),"project")
 if err:=os.MkdirAll(filepath.Join(workspace,".vscode"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(workspace,".vscode","tasks.json"),[]byte("{}"),0600);err!=nil{t.Fatal(err)}
 // Host-only approval must independently look up a REAL local TaskDeck
 // user ID, rather than trusting a username supplied by PHP or a shell flag.
 dbPath:=filepath.Join(t.TempDir(),"local-identity","identity.db")
 ctx:=context.Background()
 store,err:=identity.OpenSQLiteStore(ctx,dbPath)
 if err!=nil{t.Fatal(err)}
 defer store.Close()
 hash,err:=identity.HashPassword(ctx,"local-admin-password-for-test-only")
 if err!=nil{t.Fatal(err)}
 admin,err:=store.BootstrapFirstAdmin(ctx,"host-project","alice",hash,time.Now().UTC())
 if err!=nil{t.Fatal(err)}
 ini:=fmt.Sprintf("[shared_server]\\nenabled=true\\nproject_id=host-project\\nidentity_db=%s\\n",dbPath)
 ini=strings.ReplaceAll(ini,"\\n","\n")
 if err:=os.WriteFile(filepath.Join(workspace,".vscode","vscode_tasks_menu.ini"),[]byte(ini),0600);err!=nil{t.Fatal(err)}
 args:=[]string{"--workspace",workspace}
 if err:=runHostRemoteKeyCLI(append(append([]string{},args...),"list"));err!=nil{t.Fatal(err)}
 pub,_,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 public:=base64.StdEncoding.EncodeToString(pub)
 path,err:=remoteclientkey.DefaultPath(workspace)
 if err!=nil{t.Fatal(err)}
 if err:=runHostRemoteKeyCLI(append(append([]string{},args...),"add","alice",public,"laptop"));err!=nil{t.Fatal(err)}
 entries,err:=remoteclientkey.List(path)
 if err!=nil||len(entries)!=1||entries[0].Username!="alice"||entries[0].UserID!=string(admin.UserID){t.Fatalf("host CLI add failed or did not bind immutable user ID: %v %+v",err,entries)}
 if err:=runHostRemoteKeyCLI(append(append([]string{},args...),"add","bob",public));err==nil{t.Fatal("P0: second user claimed same host-registered key")}
 if err:=runHostRemoteKeyCLI(append(append([]string{},args...),"revoke",entries[0].Fingerprint));err!=nil{t.Fatal(err)}
 remaining,err:=remoteclientkey.List(path)
 if err!=nil||len(remaining)!=0{t.Fatalf("revocation not persistent: %v %+v",err,remaining)}
 if err:=runHostRemoteKeyCLI(append(append([]string{},args...),"add","alice","not a public key"));err==nil{t.Fatal("invalid public key approved")}
}

func TestHostRemoteKeyCLIExportsExistingAgentPublicKeyWithoutGenerating(t *testing.T){
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 ws:=filepath.Join(t.TempDir(),"project")
 if err:=os.MkdirAll(filepath.Join(ws,".vscode"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(ws,".vscode","tasks.json"),[]byte("{}"),0600);err!=nil{t.Fatal(err)}
 args:=[]string{"--workspace",ws,"host-key"}
 path,err:=remoteportal.DefaultIdentityPath(ws)
 if err!=nil{t.Fatal(err)}
 if err:=runHostRemoteKeyCLI(args);err==nil{
  t.Fatal("P0: host-key silently generated a trusted Agent identity")
 }
 if _,err:=os.Stat(path);!os.IsNotExist(err){
  t.Fatalf("missing Agent seed was created by read-only host-key: %v",err)
 }
 enrolled,err:=remoteportal.LoadOrCreateIdentity(path)
 if err!=nil{t.Fatal(err)}
 oldOutput:=os.Stdout
 read,write,err:=os.Pipe()
 if err!=nil{t.Fatal(err)}
 os.Stdout=write
 cliErr:=runHostRemoteKeyCLI(args)
 _=write.Close()
 os.Stdout=oldOutput
 output,readErr:=io.ReadAll(read)
 _=read.Close()
 if cliErr!=nil || readErr!=nil {t.Fatalf("host-key: %v, output: %v",cliErr,readErr)}
 if !strings.Contains(string(output),enrolled.DeviceID()) ||
  !strings.Contains(string(output),enrolled.PublicKeyBase64()){
  t.Fatal("host-key did not print verified owner-controlled Agent public key and Device ID")
 }
 same,err:=remoteportal.LoadExistingIdentity(path)
 if err!=nil || same.DeviceID()!=enrolled.DeviceID(){
  t.Fatal("host-key rotated the persistent Agent identity")
 }
 if err:=os.Remove(path);err!=nil{t.Fatal(err)}
 if err:=runHostRemoteKeyCLI(args);err==nil{
  t.Fatal("P0: removed Agent key was silently regenerated")
 }
}
