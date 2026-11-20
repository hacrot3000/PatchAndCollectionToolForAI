package remoteportal

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "errors"
 "path/filepath"
 "sync/atomic"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
)

// A compromised PHP Portal cannot keep asking for a local TaskDeck password
// with a key approved for an account that has been deleted/recreated.
func TestAgentPreflightRequiresCurrentImmutableSQLiteUserID(t *testing.T) {
 a,err:=NewAgent("https://portal.example.com",
  filepath.Join(t.TempDir(),"private","agent.seed"),nil)
 if err!=nil{t.Fatal(err)}
 pub,priv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 encoded:=base64.StdEncoding.EncodeToString(pub)
 entry,err:=remoteclientkey.AddForUser(a.clientKeyRegistryPath,
  "alice","old-user-ID",encoded,"client approved by host")
 if err!=nil{t.Fatal(err)}
 binding:=sha256.Sum256([]byte("independent Agent-authenticated E2EE session"))
 proof:=base64.StdEncoding.EncodeToString(ed25519.Sign(priv,
  remoteclientkey.ProofMessage("alice",binding[:])))
 ctx:=context.Background()
 if a.verifyOwnerApprovedClientKey(ctx,"alice",encoded,proof,binding[:]){
  t.Fatal("P0: missing local SQLite verifier allowed password prompt")
 }
 var active atomic.Value
 active.Store("old-user-ID")
 lookup:=func(ctx context.Context,username string)(string,error){
  if ctx.Err()!=nil||username!="alice"{return "",errors.New("unknown TaskDeck user")}
  return active.Load().(string),nil
 }
 if err:=a.ConfigureClientKeyLocalUserLookup(lookup);err!=nil{t.Fatal(err)}
 if err:=a.ConfigureClientKeyLocalUserLookup(lookup);err==nil{
  t.Fatal("PHP-like second local identity lookup replaced first one")
 }
 if !a.verifyOwnerApprovedClientKey(ctx,"alice",encoded,proof,binding[:]){
  t.Fatal("valid key for current local account denied")
 }
 active.Store("new-user-ID")
 if a.verifyOwnerApprovedClientKey(ctx,"alice",encoded,proof,binding[:]){
  t.Fatal("P0: deleted/recreated local account triggered password prompt using old approved key")
 }
 if err:=remoteclientkey.Remove(a.clientKeyRegistryPath,entry.Fingerprint);err!=nil{t.Fatal(err)}
 if _,err:=remoteclientkey.AddForUser(a.clientKeyRegistryPath,"alice",
  "new-user-ID",encoded,"host explicitly re-approved");err!=nil{t.Fatal(err)}
 if !a.verifyOwnerApprovedClientKey(ctx,"alice",encoded,proof,binding[:]){
  t.Fatal("host's explicit new-ID approval not recognized")
 }
 if a.verifyOwnerApprovedClientKey(ctx,"alice",encoded,proof,make([]byte,32)){
  t.Fatal("P0: signed E2EE proof replayed on foreign session")
 }
 if a.verifyOwnerApprovedClientKey(ctx,"bob",encoded,proof,binding[:]){
  t.Fatal("P0: key crossed local SQLite users")
 }
}
